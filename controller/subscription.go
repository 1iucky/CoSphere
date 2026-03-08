package controller

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ===================== Admin 订阅管理 API =====================

// GetAllSubscriptions 获取订阅列表（管理员）
// GET /api/admin/subscriptions
// 支持全量查询和组合筛选：user_id, plan_id, status
func GetAllSubscriptions(c *gin.Context) {
	var req dto.SubscriptionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 设置默认分页
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	startIdx := (req.Page - 1) * req.PageSize

	// 使用新的管理员查询方法，支持全量和组合筛选
	subs, total, err := model.GetAllSubscriptionsAdmin(req.UserID, req.PlanID, req.Status, startIdx, req.PageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为 DTO
	var subDTOs []dto.SubscriptionResponse
	for _, sub := range subs {
		subDTO := convertSubscriptionToDTO(sub)
		subDTOs = append(subDTOs, subDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    subDTOs,
		"total":   total,
	})
}

// GetSubscriptionAdmin 获取订阅详情（管理员）
// GET /api/admin/subscriptions/:id
func GetSubscriptionAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	sub, err := model.GetSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	subDTO := convertSubscriptionToDTO(sub)

	// 获取当前窗口使用量摘要
	usageSummary, err := getSubscriptionUsageSummary(id)
	if err == nil && len(usageSummary) > 0 {
		subDTO.UsageSummary = usageSummary
	}

	// 获取使用量历史记录
	usageHistory, err := getSubscriptionUsageHistory(id)
	if err == nil && len(usageHistory) > 0 {
		subDTO.UsageHistory = usageHistory
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    subDTO,
	})
}

// CancelSubscriptionAdmin 取消订阅（管理员）
// POST /api/admin/subscriptions/:id/cancel
func CancelSubscriptionAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	// 解析取消原因
	var req struct {
		Reason string `json:"reason" binding:"omitempty,max=500"`
	}
	// 允许无 body 的请求
	_ = c.ShouldBindJSON(&req)
	if req.Reason == "" {
		req.Reason = "管理员取消"
	}

	// 获取订阅信息
	sub, err := model.GetSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 状态校验：仅允许取消 active 或 pending 状态的订阅
	if sub.Status != common.SubscriptionStatusActive && sub.Status != common.SubscriptionStatusPending {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "只能取消活跃或待激活状态的订阅",
		})
		return
	}

	// 获取操作员信息
	operatorId, _ := c.Get("id")
	operatorIdInt64 := int64(operatorId.(int))

	// 使用带原因的取消方法
	originalSub, err := model.CancelSubscriptionWithReason(id, req.Reason)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 创建审计日志
	auditMetadata := map[string]interface{}{
		"reason":          req.Reason,
		"original_status": originalSub.Status,
		"original_end_at": originalSub.EndAt,
		"user_id":         originalSub.UserId,
		"plan_id":         originalSub.PlanId,
	}
	metadataJSON, _ := json.Marshal(auditMetadata)
	auditLog := &model.AuditLog{
		UserId:     &originalSub.UserId,
		OperatorId: &operatorIdInt64,
		ObjectType: "subscription",
		ObjectId:   &id,
		Action:     "cancel",
		IpAddress:  c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		Metadata:   string(metadataJSON),
	}
	_ = model.CreateAuditLog(auditLog)

	// 清除缓存
	cacheSvc := service.GetSubscriptionCacheService()
	_ = cacheSvc.InvalidateOnStatusChange(originalSub.UserId, originalSub.Id)
	service.GetSubscriptionPriorityService().InvalidateUserSubscriptionCache(originalSub.UserId)

	// 发送取消订阅通知
	go sendSubscriptionCancelNotification(originalSub, req.Reason)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// sendSubscriptionCancelNotification 发送订阅取消通知
func sendSubscriptionCancelNotification(sub *model.Subscription, reason string) {
	// 获取用户信息
	user, err := model.GetUserById(int(sub.UserId), false)
	if err != nil {
		common.SysLog("sendSubscriptionCancelNotification: 获取用户信息失败: " + err.Error())
		return
	}

	// 获取套餐名称
	planName := "订阅套餐"
	if sub.Plan != nil {
		planName = sub.Plan.Name
	}

	// 构建通知内容
	title := "订阅已取消"
	content := "您的订阅「" + planName + "」已被取消。\n\n取消原因：" + reason

	// 发送通知
	notify := dto.NewNotify("subscription_cancelled", title, content, nil)
	err = service.NotifyUser(user.Id, user.Email, user.GetSetting(), notify)
	if err != nil {
		common.SysLog("sendSubscriptionCancelNotification: 发送通知失败: " + err.Error())
	}
}

// UpdateSubscriptionPriorityAdmin 更新订阅优先级（管理员）
// PUT /api/admin/subscriptions/:id/priority
func UpdateSubscriptionPriorityAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	var req struct {
		Priority int `json:"priority" binding:"min=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	err = model.UpdateSubscriptionPriority(id, req.Priority)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ActivateSubscriptionAdmin 激活订阅（管理员）
// POST /api/admin/subscriptions/:id/activate
func ActivateSubscriptionAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	sub, err := model.GetSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	err = model.ActivateSubscription(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 缓存预热：订阅激活后主动预热缓存（而不是仅失效等待回源）
	cacheSvc := service.GetSubscriptionCacheService()
	go func() {
		_ = cacheSvc.WarmupCache(sub.UserId)
	}()
	service.GetSubscriptionPriorityService().InvalidateUserSubscriptionCache(sub.UserId)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// ExpireSubscriptionAdmin 过期订阅（管理员）
// POST /api/admin/subscriptions/:id/expire
func ExpireSubscriptionAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	sub, err := model.GetSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	err = model.ExpireSubscription(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	cacheSvc := service.GetSubscriptionCacheService()
	_ = cacheSvc.InvalidateOnStatusChange(sub.UserId, sub.Id)
	service.GetSubscriptionPriorityService().InvalidateUserSubscriptionCache(sub.UserId)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// convertSubscriptionToDTO 将订阅模型转换为 DTO
func convertSubscriptionToDTO(sub *model.Subscription) dto.SubscriptionResponse {
	subDTO := dto.SubscriptionResponse{
		ID:                 sub.Id,
		UserID:             sub.UserId,
		PlanID:             sub.PlanId,
		OrderID:            sub.OrderId,
		Status:             sub.Status,
		StartAt:            sub.StartAt,
		EndAt:              sub.EndAt,
		Priority:           sub.Priority,
		AutoWalletFallback: sub.AutoWalletFallback,
		RedemptionID:       sub.RedemptionId,
		CouponID:           sub.CouponId,
		RedeemOption:       sub.RedeemOption,
		CreatedAt:          sub.CreatedAt,
		UpdatedAt:          sub.UpdatedAt,
		RemainingDays:      int64(sub.GetRemainingDays()),
		IsExpiringSoon:     sub.GetRemainingDays() <= 7 && sub.GetRemainingDays() > 0,
	}

	if sub.BindChannelGroup != nil {
		subDTO.BindChannelGroup = *sub.BindChannelGroup
	}
	if sub.ModelWhitelistCache != nil {
		subDTO.ModelWhitelistCache = *sub.ModelWhitelistCache
	}
	if sub.Metadata != nil {
		subDTO.Metadata = *sub.Metadata
	}

	// 获取套餐名称
	if sub.Plan != nil {
		subDTO.PlanName = sub.Plan.Name
	}

	return subDTO
}

// getSubscriptionUsageSummary 获取订阅使用量摘要
func getSubscriptionUsageSummary(subscriptionId int64) ([]dto.SubscriptionUsagePeriodSummary, error) {
	usageSvc := service.GetSubscriptionUsageService()
	usages, err := usageSvc.GetCurrentUsage(subscriptionId)
	if err != nil {
		return nil, err
	}

	now := common.GetTimestamp()
	var summary []dto.SubscriptionUsagePeriodSummary
	for _, usage := range usages {
		usageRate := float64(0)
		if usage.LimitQuota > 0 {
			usageRate = float64(usage.UsedQuota) / float64(usage.LimitQuota) * 100
		}

		// 计算剩余额度
		remainingQuota := usage.LimitQuota - usage.UsedQuota
		if remainingQuota < 0 {
			remainingQuota = 0
		}

		// 计算距离窗口刷新的秒数（倒计时）
		secondsToRefresh := usage.WindowEnd - now
		if secondsToRefresh < 0 {
			secondsToRefresh = 0
		}

		// 计算 USD 展示值（quota / QuotaPerUnit = USD）
		usedQuotaUSD := float64(usage.UsedQuota) / common.QuotaPerUnit
		limitQuotaUSD := float64(usage.LimitQuota) / common.QuotaPerUnit
		remainingUSD := float64(remainingQuota) / common.QuotaPerUnit

		summary = append(summary, dto.SubscriptionUsagePeriodSummary{
			Period:           usage.Period,
			UsedQuota:        usage.UsedQuota,
			LimitQuota:       usage.LimitQuota,
			RemainingQuota:   remainingQuota,
			UsageRate:        usageRate,
			WindowStart:      usage.WindowStart,
			WindowEnd:        usage.WindowEnd,
			SecondsToRefresh: secondsToRefresh,
			IsOverLimit:      usage.UsedQuota >= usage.LimitQuota,
			UsedQuotaUSD:     usedQuotaUSD,
			LimitQuotaUSD:    limitQuotaUSD,
			RemainingUSD:     remainingUSD,
		})
	}
	return summary, nil
}

// getSubscriptionUsageHistory 获取订阅使用量历史记录
func getSubscriptionUsageHistory(subscriptionId int64) ([]dto.SubscriptionUsageHistoryItem, error) {
	// 获取所有使用量记录（最多返回最近 100 条）
	usages, _, err := model.GetAllUsagesBySubscription(subscriptionId, 0, 100)
	if err != nil {
		return nil, err
	}

	var history []dto.SubscriptionUsageHistoryItem
	for _, usage := range usages {
		usageRate := float64(0)
		if usage.LimitQuota > 0 {
			usageRate = float64(usage.UsedQuota) / float64(usage.LimitQuota) * 100
		}
		history = append(history, dto.SubscriptionUsageHistoryItem{
			ID:          usage.Id,
			Period:      usage.Period,
			UsedQuota:   usage.UsedQuota,
			LimitQuota:  usage.LimitQuota,
			UsageRate:   usageRate,
			WindowStart: usage.WindowStart,
			WindowEnd:   usage.WindowEnd,
			IsOverLimit: usage.UsedQuota >= usage.LimitQuota,
			CreatedAt:   usage.CreatedAt,
		})
	}
	return history, nil
}

// ===================== Admin 订阅退款 API =====================

// RefundSubscriptionAdmin 人工退款记录（管理员）
// POST /api/admin/subscriptions/:id/refund
// 注意：此接口仅记录退款流水，不实际增加用户余额（统一走线下处理）
func RefundSubscriptionAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	var req struct {
		Amount      int64  `json:"amount" binding:"required,min=1"`     // 退款金额（分）
		Reason      string `json:"reason" binding:"required,max=500"`   // 退款原因
		RefundType  string `json:"refund_type" binding:"omitempty,oneof=manual auto"` // 退款类型（移除 none）
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 设置默认退款类型
	if req.RefundType == "" {
		req.RefundType = "manual"
	}

	// 获取订阅信息
	sub, err := model.GetSubscriptionById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取操作员信息
	operatorId, _ := c.Get("id")
	operatorIdInt64 := int64(operatorId.(int))

	// 获取关联的订单信息（用于填充退款元数据）
	var order *model.SubscriptionOrder
	if sub.OrderId != nil && *sub.OrderId > 0 {
		order, _ = model.GetSubscriptionOrderById(*sub.OrderId)
	}

	// 执行退款记录（使用事务）
	var bill *model.UserBill
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// 在事务中获取用户当前余额（用于记录）
		var user model.User
		if err := tx.Select("id, quota").Where("id = ?", sub.UserId).First(&user).Error; err != nil {
			return err
		}

		// 构建退款账单参数
		billParams := &model.RefundBillParams{
			UserId:         sub.UserId,
			Amount:         req.Amount,
			BalanceBefore:  int64(user.Quota),
			RefundType:     req.RefundType,
			Description:    req.Reason,
			OperatorId:     &operatorIdInt64,
			RefundAmount:   req.Amount,     // 本次退款金额
			CouponRestored: false,          // 人工退款不自动恢复优惠券
		}

		// 如果有关联订单，填充订单相关元数据
		if order != nil {
			billParams.SourceType = common.BillSourceTypeSubscriptionOrder
			billParams.SourceId = order.Id
			billParams.CouponId = order.CouponId
			billParams.UserCouponId = order.UserCouponId
			billParams.OriginalAmount = order.PriceCents
			billParams.DiscountAmount = order.DiscountCents
			billParams.FinalAmount = order.FinalPriceCents
		} else {
			// 如果没有订单，使用订阅作为来源（类型和 ID 匹配）
			billParams.SourceType = "subscription"
			billParams.SourceId = id
		}

		// 创建退款账单记录（仅记录流水，不增加余额）
		var billErr error
		bill, billErr = model.CreateRefundBillWithMetadataWithTx(tx, billParams)
		if billErr != nil {
			return billErr
		}

		return nil
	})

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "退款记录失败: " + err.Error(),
		})
		return
	}

	// 创建审计日志
	auditMetadata := map[string]interface{}{
		"refund_amount":   req.Amount,
		"refund_type":     req.RefundType,
		"reason":          req.Reason,
		"user_id":         sub.UserId,
		"plan_id":         sub.PlanId,
		"bill_id":         bill.Id,
		"note":            "仅记录退款流水，实际退款走线下处理",
	}
	if order != nil {
		auditMetadata["order_id"] = order.Id
		auditMetadata["original_amount"] = order.PriceCents
		auditMetadata["discount_amount"] = order.DiscountCents
		auditMetadata["final_amount"] = order.FinalPriceCents
	}
	metadataJSON, _ := json.Marshal(auditMetadata)
	auditLog := &model.AuditLog{
		UserId:     &sub.UserId,
		OperatorId: &operatorIdInt64,
		ObjectType: "subscription",
		ObjectId:   &id,
		Action:     "refund",
		IpAddress:  c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		Metadata:   string(metadataJSON),
	}
	_ = model.CreateAuditLog(auditLog)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "退款记录已创建，实际退款请通过线下渠道处理",
		"data": gin.H{
			"refund_amount": req.Amount,
			"bill_id":       bill.Id,
		},
	})
}

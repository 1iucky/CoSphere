package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ===================== 用户订阅 API =====================

// GetUserSubscriptions 获取当前用户的订阅列表
// GET /api/subscription/self
func GetUserSubscriptions(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	startIdx := (page - 1) * pageSize

	subs, total, err := model.GetAllSubscriptionsByUser(int64(userId), startIdx, pageSize, status)
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

// GetUserActiveSubscriptions 获取当前用户的活跃订阅
// GET /api/subscription/self/active
func GetUserActiveSubscriptions(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	subs, err := model.GetActiveSubscriptionsByUser(int64(userId))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为 DTO 并填充 usage 摘要
	var subDTOs []dto.SubscriptionResponse
	for _, sub := range subs {
		subDTO := convertSubscriptionToDTO(sub)
		// 填充 usage 摘要（包含剩余额度、倒计时、USD 展示）
		usageSummary, err := getSubscriptionUsageSummary(sub.Id)
		if err == nil {
			subDTO.UsageSummary = usageSummary
		}
		subDTOs = append(subDTOs, subDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    subDTOs,
	})
}

// GetUserSubscriptionDetail 获取用户订阅详情
// GET /api/subscription/self/:id
func GetUserSubscriptionDetail(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	subId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	sub, err := model.GetSubscriptionById(subId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证订阅属于当前用户
	if sub.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此订阅",
		})
		return
	}

	subDTO := convertSubscriptionToDTO(sub)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    subDTO,
	})
}

// GetUserSubscriptionUsage 获取用户订阅使用量
// GET /api/subscription/self/:id/usage
func GetUserSubscriptionUsage(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	subId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	// 验证订阅属于当前用户
	sub, err := model.GetSubscriptionById(subId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if sub.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此订阅",
		})
		return
	}

	usages, err := model.GetCurrentUsageBySubscription(subId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	now := common.GetTimestamp()
	// 转换为响应格式，填充新增字段
	var usageDTOs []dto.SubscriptionUsageResponse
	for _, usage := range usages {
		// 计算剩余额度
		remainingQuota := usage.LimitQuota - usage.UsedQuota
		if remainingQuota < 0 {
			remainingQuota = 0
		}
		// 计算使用率
		usageRate := float64(0)
		if usage.LimitQuota > 0 {
			usageRate = float64(usage.UsedQuota) / float64(usage.LimitQuota) * 100
		}
		// 计算倒计时
		secondsToRefresh := usage.WindowEnd - now
		if secondsToRefresh < 0 {
			secondsToRefresh = 0
		}
		// 计算 USD 展示值
		usedQuotaUSD := float64(usage.UsedQuota) / common.QuotaPerUnit
		limitQuotaUSD := float64(usage.LimitQuota) / common.QuotaPerUnit
		remainingUSD := float64(remainingQuota) / common.QuotaPerUnit

		usageDTOs = append(usageDTOs, dto.SubscriptionUsageResponse{
			ID:               usage.Id,
			SubscriptionID:   usage.SubscriptionId,
			Period:           usage.Period,
			WindowStart:      usage.WindowStart,
			WindowEnd:        usage.WindowEnd,
			UsedQuota:        usage.UsedQuota,
			LimitQuota:       usage.LimitQuota,
			RemainingQuota:   remainingQuota,
			UsageRate:        usageRate,
			SecondsToRefresh: secondsToRefresh,
			IsOverLimit:      usage.UsedQuota >= usage.LimitQuota,
			UsedQuotaUSD:     usedQuotaUSD,
			LimitQuotaUSD:    limitQuotaUSD,
			RemainingUSD:     remainingUSD,
			CreatedAt:        usage.CreatedAt,
			UpdatedAt:        usage.UpdatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usageDTOs,
	})
}

// UpdateUserSubscriptionSettings 更新用户订阅设置
// PUT /api/subscription/self/:id/settings
func UpdateUserSubscriptionSettings(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	subId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	var req dto.SubscriptionUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取并验证订阅
	sub, err := model.GetSubscriptionById(subId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if sub.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权修改此订阅",
		})
		return
	}

	// 更新设置
	if req.AutoWalletFallback != nil {
		sub.AutoWalletFallback = *req.AutoWalletFallback
	}
	if req.BindChannelGroup != nil {
		// 校验 bind_channel_group：支持多分组（逗号分隔），每个分组都必须在套餐允许范围内
		requestedGroups := *req.BindChannelGroup
		if requestedGroups != "" {
			// 获取套餐信息以验证渠道组
			plan, err := model.GetSubscriptionPlanById(sub.PlanId)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "获取套餐信息失败: " + err.Error(),
				})
				return
			}

			// 验证每个渠道组是否在套餐允许范围内
			if plan.ChannelGroups != nil && *plan.ChannelGroups != "" {
				allowedGroups := strings.Split(*plan.ChannelGroups, ",")
				// 构建允许分组的 set
				allowedSet := make(map[string]bool)
				for _, group := range allowedGroups {
					allowedSet[strings.TrimSpace(group)] = true
				}

				// 检查请求的每个分组
				requestedList := strings.Split(requestedGroups, ",")
				var invalidGroups []string
				for _, group := range requestedList {
					trimmedGroup := strings.TrimSpace(group)
					if trimmedGroup != "" && !allowedSet[trimmedGroup] {
						invalidGroups = append(invalidGroups, trimmedGroup)
					}
				}
				if len(invalidGroups) > 0 {
					c.JSON(http.StatusOK, gin.H{
						"success": false,
						"message": fmt.Sprintf("渠道组 '%s' 不在套餐允许范围内，允许的渠道组: %s", strings.Join(invalidGroups, ","), *plan.ChannelGroups),
					})
					return
				}
			}
			// 如果套餐没有限制渠道组，则允许任何非空值
		}
		sub.BindChannelGroup = req.BindChannelGroup
	}

	err = model.UpdateSubscription(sub)
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

// ReorderUserSubscriptionPriorities 重新排序用户订阅优先级
// POST /api/subscription/self/reorder
func ReorderUserSubscriptionPriorities(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req struct {
		SubscriptionIds []int64 `json:"subscription_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	err := model.ReorderSubscriptionPriorities(int64(userId), req.SubscriptionIds)
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

// CancelUserSubscription 取消用户订阅
// POST /api/subscription/self/:id/cancel
func CancelUserSubscription(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	subId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	// 验证订阅属于当前用户
	sub, err := model.GetSubscriptionById(subId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if sub.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权取消此订阅",
		})
		return
	}

	err = model.CancelSubscription(subId)
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

// ===================== 用户订单 API =====================

// GetUserOrders 获取当前用户的订单列表
// GET /api/subscription-orders/self
func GetUserOrders(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	svc := service.GetSubscriptionOrderService()
	orders, total, err := svc.GetOrdersByUser(int64(userId), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为 DTO
	var orderDTOs []dto.SubscriptionOrderResponse
	for _, order := range orders {
		orderDTO := convertOrderToDTO(order)
		orderDTOs = append(orderDTOs, orderDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderDTOs,
		"total":   total,
	})
}

// GetUserOrderDetail 获取用户订单详情
// GET /api/subscription-orders/self/:id
func GetUserOrderDetail(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	orderId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	svc := service.GetSubscriptionOrderService()
	order, err := svc.GetOrderById(orderId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证订单属于当前用户
	if order.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此订单",
		})
		return
	}

	orderDTO := convertOrderToDTO(order)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderDTO,
	})
}

// PreviewOrder 预览订单（计算价格）
// POST /api/subscription-orders/preview
func PreviewOrder(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req struct {
		PlanID     int64   `json:"plan_id" binding:"required,min=1"`
		CouponCode *string `json:"coupon_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 处理优惠券：根据 coupon_code 查询 userCouponId
	var userCouponId *int64 = nil
	if req.CouponCode != nil && *req.CouponCode != "" {
		// 根据 code 查询用户优惠券
		userCoupon, err := model.GetUserCouponByCode(*req.CouponCode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "优惠券无效: " + err.Error(),
			})
			return
		}

		// 验证优惠券归属
		if userCoupon.UserId != int64(userId) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该优惠券不属于当前用户",
			})
			return
		}

		// 验证优惠券状态
		if userCoupon.Status != common.UserCouponStatusAvailable {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("优惠券状态无效: %s", userCoupon.Status),
			})
			return
		}

		userCouponId = &userCoupon.Id
	}

	svc := service.GetSubscriptionOrderService()
	result, err := svc.PreviewOrder(int64(userId), req.PlanID, userCouponId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	appliedCouponCode := ""
	if result.CouponApplicable && req.CouponCode != nil {
		appliedCouponCode = *req.CouponCode
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": dto.SubscriptionOrderPreviewResponse{
			PlanID:          result.PlanId,
			PlanName:        result.PlanName,
			PriceCents:      result.OriginalAmount,
			DiscountCents:   result.DiscountAmount,
			FinalPriceCents: result.FinalAmount,
			Currency:        result.Currency,
			CouponApplied:   result.CouponApplicable,
			CouponCode:      appliedCouponCode,
		},
	})
}

// CreateOrder 创建订单
// POST /api/subscription-orders
func CreateOrder(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req dto.SubscriptionOrderCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 处理优惠券：根据 coupon_code 查询 userCouponId
	var userCouponId *int64 = nil
	if req.CouponCode != nil && *req.CouponCode != "" {
		// 根据 code 查询用户优惠券
		userCoupon, err := model.GetUserCouponByCode(*req.CouponCode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "优惠券无效: " + err.Error(),
			})
			return
		}

		// 验证优惠券归属
		if userCoupon.UserId != int64(userId) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该优惠券不属于当前用户",
			})
			return
		}

		// 验证优惠券状态
		if userCoupon.Status != common.UserCouponStatusAvailable {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("优惠券状态无效: %s", userCoupon.Status),
			})
			return
		}

		userCouponId = &userCoupon.Id
	}

	svc := service.GetSubscriptionOrderService()
	result, err := svc.CreateOrder(int64(userId), req.PlanID, userCouponId, req.PaymentChannel)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 返回订单信息
	// 对于第三方支付渠道，需要说明下一步操作
	response := gin.H{
		"order_id":          result.Order.Id,
		"plan_id":           result.Order.PlanId,
		"final_price_cents": result.FinalAmount,
		"original_price":    result.OriginalAmount,
		"discount_amount":   result.DiscountAmount,
		"payment_channel":   result.Order.PaymentChannel,
		"status":            result.Order.Status,
		"is_idempotent":     result.IsIdempotent, // 是否为幂等返回（已有同参数订单）
	}

	// 对于第三方支付，提供支付指引
	switch req.PaymentChannel {
	case common.PaymentChannelWallet:
		response["next_step"] = "调用 POST /api/subscription-orders/{order_id}/pay 完成支付"
	case common.PaymentChannelAlipay, common.PaymentChannelWechat, common.PaymentChannelStripe, common.PaymentChannelPaypal:
		response["next_step"] = "调用第三方支付接口获取支付链接，支付完成后系统将自动回调确认"
		response["payment_timeout_seconds"] = OrderPaymentTimeoutSeconds // 订单有效期 30 分钟
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// PayOrder 支付订单
// POST /api/subscription-orders/:id/pay
func PayOrder(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	orderId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	var req struct {
		PaymentChannel string `json:"payment_channel" binding:"required"`
		TradeNo        string `json:"trade_no"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	svc := service.GetSubscriptionOrderService()
	result, err := svc.ProcessPayment(int64(userId), orderId, req.PaymentChannel, req.TradeNo)
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
		"data": gin.H{
			"subscription_id": result.Subscription.Id,
			"start_at":        result.Subscription.StartAt,
			"end_at":          result.Subscription.EndAt,
		},
	})
}

// CancelUserOrder 取消用户订单
// POST /api/subscription-orders/:id/cancel
func CancelUserOrder(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	orderId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)

	svc := service.GetSubscriptionOrderService()
	err = svc.CancelOrder(int64(userId), orderId, req.Reason)
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

// PurchaseSubscription 一键购买订阅（创建订单 + 支付）
// POST /api/subscription-orders/purchase
// 仅支持 wallet/free 支付方式（第三方支付需使用分步流程）
func PurchaseSubscription(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req struct {
		PlanID     int64   `json:"plan_id" binding:"required,min=1"`
		CouponCode *string `json:"coupon_code" binding:"omitempty,max=64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 处理优惠券：根据 coupon_code 查询 userCouponId
	var userCouponId *int64 = nil
	if req.CouponCode != nil && *req.CouponCode != "" {
		userCoupon, err := model.GetUserCouponByCode(*req.CouponCode)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "优惠券无效: " + err.Error(),
			})
			return
		}

		if userCoupon.UserId != int64(userId) {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "该优惠券不属于当前用户",
			})
			return
		}

		if userCoupon.Status != common.UserCouponStatusAvailable {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": fmt.Sprintf("优惠券状态无效: %s", userCoupon.Status),
			})
			return
		}

		userCouponId = &userCoupon.Id
	}

	svc := service.GetSubscriptionOrderService()

	// 1. 创建订单（默认使用 wallet 支付渠道）
	createResult, err := svc.CreateOrder(int64(userId), req.PlanID, userCouponId, common.PaymentChannelWallet)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "创建订单失败: " + err.Error(),
		})
		return
	}

	// 2. 确定支付方式：如果是 0 元订单使用 free，否则使用 wallet
	paymentChannel := common.PaymentChannelWallet
	if createResult.FinalAmount == 0 {
		paymentChannel = common.PaymentChannelFree
	}

	// 3. 立即支付
	payResult, err := svc.ProcessPayment(int64(userId), createResult.Order.Id, paymentChannel, "")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "支付失败: " + err.Error(),
			"data": gin.H{
				"order_id": createResult.Order.Id,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"order_id":        createResult.Order.Id,
			"subscription_id": payResult.Subscription.Id,
			"plan_id":         payResult.Subscription.PlanId,
			"start_at":        payResult.Subscription.StartAt,
			"end_at":          payResult.Subscription.EndAt,
			"amount_paid":     createResult.FinalAmount,
		},
	})
}

// ===================== 用户订阅配置 API =====================

// UpdateSubscriptionAutoWallet 切换订阅自动兜底开关
// PUT /api/user/subscriptions/:id/auto-wallet
func UpdateSubscriptionAutoWallet(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	subId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 校验 enabled 必填
	if req.Enabled == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "enabled 参数为必填项",
		})
		return
	}

	// 使用单字段更新，避免并发丢更新风险
	// 同时验证订阅归属（WHERE id AND user_id）
	err = model.UpdateSubscriptionAutoWalletFallback(subId, int64(userId), *req.Enabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 刷新缓存
	cacheSvc := service.GetSubscriptionCacheService()
	_ = cacheSvc.InvalidateOnStatusChange(int64(userId), subId)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"auto_wallet_fallback": *req.Enabled,
		},
	})
}

// BatchUpdateSubscriptionPriorities 批量更新订阅优先级
// PUT /api/user/subscriptions/priorities
func BatchUpdateSubscriptionPriorities(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req struct {
		SubscriptionIds []int64 `json:"subscription_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	if len(req.SubscriptionIds) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订阅 ID 列表不能为空",
		})
		return
	}

	// 使用 SubscriptionPriorityService 进行重排序
	prioritySvc := service.GetSubscriptionPriorityService()
	err := prioritySvc.ReorderSubscriptions(int64(userId), req.SubscriptionIds)
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

// GetUserAutoWalletFallback 获取用户级别兜底配置
// GET /api/user/settings/auto-wallet-fallback
func GetUserAutoWalletFallback(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	// 获取用户信息
	user, err := model.GetUserById(userId, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户信息失败: " + err.Error(),
		})
		return
	}

	// 获取系统默认值
	systemDefault := common.GetAutoWalletFallbackDefault()

	// 使用 GetEffectiveAutoWalletFallback 获取有效值和是否使用系统默认
	effectiveValue, usingSystemDefault := user.GetEffectiveAutoWalletFallback(systemDefault)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"user_setting":         user.AutoWalletFallback,       // 用户设置的值（数据库中的值）
			"system_default":       systemDefault,                 // 系统默认值
			"effective":            effectiveValue,                // 当前生效的值
			"is_explicit":          !usingSystemDefault,           // 是否为显式设置
			"using_system_default": usingSystemDefault,            // 是否使用系统默认
		},
	})
}

// UpdateUserAutoWalletFallbackRequest 更新用户自动兜底配置请求
type UpdateUserAutoWalletFallbackRequest struct {
	Enabled *bool `json:"enabled"` // 使用指针区分未提供和 false
	Reset   bool  `json:"reset"`   // 是否重置为继承系统默认
}

// UpdateUserAutoWalletFallback 更新用户级别兜底配置
// PUT /api/user/settings/auto-wallet-fallback
func UpdateUserAutoWalletFallback(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req UpdateUserAutoWalletFallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 验证参数：enabled 和 reset 不能同时提供
	if req.Reset && req.Enabled != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "enabled 和 reset 参数不能同时提供",
		})
		return
	}

	// 验证参数：至少提供一个
	if !req.Reset && req.Enabled == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请提供 enabled 或 reset 参数",
		})
		return
	}

	if req.Reset {
		// 重置为继承系统默认：清除 explicit 标记
		err := model.ResetAutoWalletFallback(userId)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "重置失败: " + err.Error(),
			})
			return
		}

		// 获取当前系统默认值
		systemDefault := common.GetAutoWalletFallbackDefault()
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data": gin.H{
				"auto_wallet_fallback": systemDefault,
				"is_explicit":          false,
				"using_system_default": true,
			},
		})
		return
	}

	// 显式设置：使用正确的函数，同时更新 setting JSON 和 explicit 标记
	err := model.UpdateAutoWalletFallback(userId, *req.Enabled)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"auto_wallet_fallback": *req.Enabled,
			"is_explicit":          true,
			"using_system_default": false,
		},
	})
}

// ===================== 公开 API（获取可购买套餐） =====================

// GetAvailablePlans 获取可购买的套餐列表
// GET /api/subscription-plans
func GetAvailablePlans(c *gin.Context) {
	svc := service.GetSubscriptionPlanService()
	plans, err := svc.GetActivePlans()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为 DTO，过滤不在有效期内的套餐
	now := common.GetTimestamp()
	var planDTOs []dto.SubscriptionPlanResponse
	for _, plan := range plans {
		// 跳过尚未生效的套餐
		if plan.StartAt != nil && *plan.StartAt > now {
			continue
		}
		// 跳过已停售的套餐
		if plan.EndAt != nil && *plan.EndAt < now {
			continue
		}
		// 使用用户端转换（过滤 disabled 限额）
		planDTO := convertPlanToDTOForUser(plan)
		planDTOs = append(planDTOs, planDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    planDTOs,
	})
}

// GetAvailablePlanDetail 获取可购买套餐详情（公开接口）
// GET /api/subscription-plans/:id
func GetAvailablePlanDetail(c *gin.Context) {
	idStr := c.Param("id")
	planId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的套餐 ID",
		})
		return
	}

	svc := service.GetSubscriptionPlanService()
	plan, err := svc.GetPlan(planId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 只返回已上架的套餐
	if plan.Status != common.PlanStatusActive {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐不可用",
		})
		return
	}

	// 校验套餐有效期
	now := common.GetTimestamp()
	if plan.StartAt != nil && *plan.StartAt > now {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐尚未生效",
		})
		return
	}
	if plan.EndAt != nil && *plan.EndAt < now {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "套餐已停售",
		})
		return
	}

	// 使用用户端转换（过滤 disabled 限额）
	planDTO := convertPlanToDTOForUser(plan)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    planDTO,
	})
}

// GetUserSubscriptionHistory 获取用户订阅历史（订单/兑换/取消记录）
// GET /api/subscription/self/:id/history
func GetUserSubscriptionHistory(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	subId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订阅 ID",
		})
		return
	}

	// 验证订阅属于当前用户
	sub, err := model.GetSubscriptionById(subId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	if sub.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此订阅",
		})
		return
	}

	// 构建历史记录
	history := make([]gin.H, 0)

	// 1. 获取关联订单信息
	if sub.OrderId != nil && *sub.OrderId > 0 {
		order, err := model.GetSubscriptionOrderById(*sub.OrderId)
		if err == nil && order != nil {
			history = append(history, gin.H{
				"type":           "order",
				"action":         "purchase",
				"timestamp":      order.CreatedAt,
				"amount":         order.FinalPriceCents,
				"original_amount": order.PriceCents,
				"discount_amount": order.DiscountCents,
				"payment_channel": order.PaymentChannel,
				"status":         order.Status,
			})
		}
	}

	// 2. 获取兑换记录
	if sub.RedemptionId != nil && *sub.RedemptionId > 0 {
		redemption, err := model.GetRedemptionById(int(*sub.RedemptionId))
		if err == nil && redemption != nil {
			history = append(history, gin.H{
				"type":         "redemption",
				"action":       sub.RedeemOption,
				"timestamp":    redemption.RedeemedTime,
				"redemption_id": redemption.Id,
			})
		}
	}

	// 3. 获取审计日志中的取消/状态变更记录
	auditLogs, err := model.GetAuditLogsByObject("subscription", subId, 0, 50)
	if err == nil {
		for _, log := range auditLogs {
			if log.Action == "cancel" || log.Action == "expire" || log.Action == "activate" {
				history = append(history, gin.H{
					"type":      "status_change",
					"action":    log.Action,
					"timestamp": log.CreatedAt,
					"metadata":  log.Metadata,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"subscription_id": subId,
			"history":         history,
		},
	})
}

package controller

import (
	"encoding/json"
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

// ===================== Admin 优惠券管理 API =====================

// GetAllSubscriptionCoupons 获取优惠券列表（支持筛选/分页）
// GET /api/admin/subscription-coupons
func GetAllSubscriptionCoupons(c *gin.Context) {
	var req dto.CouponListRequest
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

	offset := (req.Page - 1) * req.PageSize

	// ��建查询
	var coupons []*model.Coupon
	var total int64
	var err error

	// 支持组合筛选：keyword 可以与 status, scope, type 等条件同时使用
	keyword := req.Keyword
	if keyword == "" {
		keyword = c.Query("keyword") // 兼容查询参数
	}

	if keyword != "" {
		// 使用 keyword 搜索，同时支持其他筛选条件
		coupons, total, err = model.SearchCouponsWithFilters(keyword, req.Status, req.Scope, req.Type, req.DiscountType, offset, req.PageSize)
	} else {
		// 普通列表查询，支持多条件筛选
		coupons, total, err = model.GetAllCouponsWithFilters(req.Status, req.Scope, req.Type, req.DiscountType, offset, req.PageSize)
	}

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取优惠券列表失败: " + err.Error(),
		})
		return
	}

	// 转换为 DTO
	var couponDTOs []dto.CouponResponse
	for _, coupon := range coupons {
		couponDTO := convertCouponToDTO(coupon)
		couponDTOs = append(couponDTOs, couponDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    couponDTOs,
		"total":   total,
	})
}

// GetSubscriptionCoupon 获取优惠券详情
// GET /api/admin/subscription-coupons/:id
func GetSubscriptionCoupon(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	coupon, err := model.GetCouponById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	couponDTO := convertCouponToDTO(coupon)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    couponDTO,
	})
}

// CreateSubscriptionCoupon 创建优惠券
// POST /api/admin/subscription-coupons
func CreateSubscriptionCoupon(c *gin.Context) {
	var req dto.CouponCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取当前管理员 ID
	adminId := c.GetInt("id")
	if adminId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取当前用户 ID",
		})
		return
	}

	// 转换请求到模型
	coupon := &model.Coupon{
		Code:            strings.TrimSpace(req.Code),
		Name:            strings.TrimSpace(req.Name),
		Type:            req.Type,
		Scope:           req.Scope,
		DiscountValue:   req.DiscountValue,
		ThresholdAmount: req.ThresholdAmount,
		Currency:        req.Currency, // 必填字段，已由 binding 校验为 CNY
		TotalCount:      req.TotalCount,
		PerUserLimit:    req.PerUserLimit,
		ValidFrom:       req.ValidFrom,
		ValidTo:         req.ValidTo,
		Status:          common.CouponStatusActive,
		CreatedBy:       int64(adminId),
	}

	// 处理可选字段
	if req.Description != "" {
		coupon.Description = &req.Description
	}
	// 处理 ApplicablePlanIDs：将 []int64 转换为 JSON 字符串存储
	if len(req.ApplicablePlanIDs) > 0 {
		planIdsJson, err := json.Marshal(req.ApplicablePlanIDs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "适用套餐 IDs 格式错误: " + err.Error(),
			})
			return
		}
		planIdsStr := string(planIdsJson)
		coupon.ApplicablePlanIds = &planIdsStr
	}
	if req.Status != "" {
		coupon.Status = req.Status
	}
	if req.BindUserID != nil {
		coupon.BindUserId = req.BindUserID
	}

	// 根据 Type 推断 DiscountType（忽略请求中的 discount_type 字段，确保一致性）
	if req.Type == common.CouponTypeDiscount {
		coupon.DiscountType = common.DiscountTypePercentage
	} else {
		coupon.DiscountType = common.DiscountTypeFixed
	}

	// 创建优惠券
	err := model.CreateCoupon(coupon)
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
			"id":              coupon.Id,
			"code":            coupon.Code,
			"name":            coupon.Name,
			"remaining_count": coupon.GetRemainingCount(),
			"created_at":      coupon.CreatedAt,
		},
	})
}

// UpdateSubscriptionCoupon 更新优惠券
// PUT /api/admin/subscription-coupons/:id
func UpdateSubscriptionCoupon(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	var req dto.CouponUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取现有优惠券
	coupon, err := model.GetCouponById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 应用更新
	if req.Name != nil {
		coupon.Name = *req.Name
	}
	if req.Description != nil {
		coupon.Description = req.Description
	}
	if req.Type != nil {
		coupon.Type = *req.Type
		// 同步更新 discount_type（向后兼容）
		if *req.Type == common.CouponTypeDiscount {
			coupon.DiscountType = common.DiscountTypePercentage
		} else {
			coupon.DiscountType = common.DiscountTypeFixed
		}
	}
	if req.Scope != nil {
		coupon.Scope = *req.Scope
	}
	if req.DiscountValue != nil {
		coupon.DiscountValue = *req.DiscountValue
	}
	if req.ThresholdAmount != nil {
		coupon.ThresholdAmount = *req.ThresholdAmount
	}
	if req.Currency != nil {
		coupon.Currency = *req.Currency
	}
	// 处理 ApplicablePlanIDs：将 *[]int64 转换为 JSON 字符串存储
	if req.ApplicablePlanIDs != nil {
		if len(*req.ApplicablePlanIDs) > 0 {
			planIdsJson, err := json.Marshal(*req.ApplicablePlanIDs)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "适用套餐 IDs 格式错误: " + err.Error(),
				})
				return
			}
			planIdsStr := string(planIdsJson)
			coupon.ApplicablePlanIds = &planIdsStr
		} else {
			// 空数组表示清除适用套餐限制
			coupon.ApplicablePlanIds = nil
		}
	}
	if req.Status != nil {
		coupon.Status = *req.Status
	}
	if req.BindUserID != nil {
		coupon.BindUserId = req.BindUserID
	}
	if req.TotalCount != nil {
		// 校验：库存只能增加不能减少（不再支持 -1 无限库存）
		if *req.TotalCount < coupon.TotalCount {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "优惠券总库存只能增加，不能减少",
			})
			return
		}
		coupon.TotalCount = *req.TotalCount
	}
	if req.PerUserLimit != nil {
		coupon.PerUserLimit = *req.PerUserLimit
	}
	if req.ValidFrom != nil {
		coupon.ValidFrom = *req.ValidFrom
	}
	if req.ValidTo != nil {
		coupon.ValidTo = *req.ValidTo
	}

	// 更新优惠券
	err = model.UpdateCoupon(coupon)
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

// DeleteSubscriptionCoupon 删除优惠券
// DELETE /api/admin/subscription-coupons/:id
func DeleteSubscriptionCoupon(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	err = model.DeleteCoupon(id)
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

// BindCouponToRedemption 绑定优惠券到兑换码
// POST /api/admin/subscription-coupons/:id/bind-redemption
func BindCouponToRedemption(c *gin.Context) {
	idStr := c.Param("id")
	couponId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	var req dto.CouponBindRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取当前管理员 ID
	adminId := c.GetInt("id")
	if adminId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取当前用户 ID",
		})
		return
	}

	// 调用 Service 层绑定，传递 BoundUserID（专属用户）
	couponService := service.GetCouponService()
	result, err := couponService.BindCouponToRedemption(couponId, req.RedemptionID, req.BoundUserID, int64(adminId))
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
			"binding_id": result.BindingId,
		},
	})
}

// UnbindCouponFromRedemption 解绑优惠券与兑换码（一券多码时必须指定 redemption_id）
// DELETE /api/admin/subscription-coupons/:id/unbind
func UnbindCouponFromRedemption(c *gin.Context) {
	idStr := c.Param("id")
	couponId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	// 获取兑换码 ID（从请求体或查询参数，必填）
	var redemptionId int64
	redemptionIdStr := c.Query("redemption_id")
	if redemptionIdStr != "" {
		redemptionId, err = strconv.ParseInt(redemptionIdStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无效的兑换码 ID",
			})
			return
		}
	} else {
		// 从请求体获取
		var req struct {
			RedemptionID int64 `json:"redemption_id"`
		}
		if err := c.ShouldBindJSON(&req); err == nil && req.RedemptionID > 0 {
			redemptionId = req.RedemptionID
		}
	}

	// 一券多码场景下必须指定 redemption_id
	if redemptionId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "必须指定 redemption_id 参数（一券可能绑定多码，需明确指定解绑哪个）",
		})
		return
	}

	// 校验该兑换码是否属于路径里的优惠券
	binding, err := model.GetCouponRedemptionBindingByRedemption(redemptionId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询绑定记录失败: " + err.Error(),
		})
		return
	}
	if binding == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该兑换码未绑定任何优惠券",
		})
		return
	}
	if binding.CouponId != couponId {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "该兑换码不属于指定的优惠券，无法解绑",
		})
		return
	}

	// 获取当前管理员 ID
	adminId := c.GetInt("id")
	if adminId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取当前用户 ID",
		})
		return
	}

	// 调用 Service 层解绑
	couponService := service.GetCouponService()
	err = couponService.UnbindCoupon(redemptionId, int64(adminId))
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

// GetCouponBindings 获取优惠券绑定状态（支持一券多码）
// GET /api/admin/subscription-coupons/:id/bindings
func GetCouponBindings(c *gin.Context) {
	idStr := c.Param("id")
	couponId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	// 获取优惠券信息
	coupon, err := model.GetCouponById(couponId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取所有绑定记录（支持一券多码）
	bindings, err := model.GetAllCouponRedemptionBindingsByCoupon(couponId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "查询绑定记录失败: " + err.Error(),
		})
		return
	}

	// 构建响应
	response := gin.H{
		"coupon_id":      couponId,
		"coupon_code":    coupon.Code,
		"coupon_name":    coupon.Name,
		"is_bound":       len(bindings) > 0,
		"binding_count":  len(bindings),
	}

	if len(bindings) > 0 {
		var bindingInfos []gin.H
		for _, binding := range bindings {
			// 获取兑换码信息
			redemption, _ := model.GetRedemptionById(int(binding.RedemptionId))

			bindingInfo := gin.H{
				"binding_id":    binding.Id,
				"redemption_id": binding.RedemptionId,
				"status":        binding.Status,
				"locked_at":     binding.LockedAt,
				"created_at":    binding.CreatedAt,
				"updated_at":    binding.UpdatedAt,
			}

			if binding.UserId != nil {
				bindingInfo["user_id"] = *binding.UserId
			}

			if redemption != nil {
				bindingInfo["redemption_name"] = redemption.Name
				bindingInfo["redemption_status"] = redemption.Status
			}

			bindingInfos = append(bindingInfos, bindingInfo)
		}

		response["bindings"] = bindingInfos
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// ===================== 辅助方法 =====================

// convertCouponToDTO 将优惠券模型转换为 DTO
func convertCouponToDTO(coupon *model.Coupon) dto.CouponResponse {
	couponDTO := dto.CouponResponse{
		ID:              coupon.Id,
		Code:            coupon.Code,
		Name:            coupon.Name,
		Type:            coupon.Type,
		Scope:           coupon.Scope,
		DiscountValue:   coupon.DiscountValue,
		ThresholdAmount: coupon.ThresholdAmount,
		Currency:        coupon.Currency,
		TotalCount:      coupon.TotalCount,
		UsedCount:       coupon.UsedCount,
		PerUserLimit:    coupon.PerUserLimit,
		ValidFrom:       coupon.ValidFrom,
		ValidTo:         coupon.ValidTo,
		Status:          coupon.Status,
		BindUserID:      coupon.BindUserId,
		CreatedBy:       coupon.CreatedBy,
		CreatedAt:       coupon.CreatedAt,
		UpdatedAt:       coupon.UpdatedAt,
		IsAvailable:     coupon.IsValid(),
		RemainingCount:  coupon.GetRemainingCount(),
	}

	// 处理可选字段
	if coupon.Description != nil {
		couponDTO.Description = *coupon.Description
	}
	// 处理 ApplicablePlanIds：将 JSON 字符串解析为 []int64
	if coupon.ApplicablePlanIds != nil && *coupon.ApplicablePlanIds != "" {
		var planIds []int64
		if err := json.Unmarshal([]byte(*coupon.ApplicablePlanIds), &planIds); err != nil {
			// 记录解析错误以便排查数据问题，但列表接口仍返回空数组（与使用路径行为一致：使用时会报错）
			common.SysLog(fmt.Sprintf("优惠券 ID=%d 的 applicable_plan_ids 解析失败: %v，原始值: %s",
				coupon.Id, err, *coupon.ApplicablePlanIds))
		} else {
			couponDTO.ApplicablePlanIDs = planIds
		}
	}
	// 如果 ApplicablePlanIDs 为 nil，初始化为空数组
	if couponDTO.ApplicablePlanIDs == nil {
		couponDTO.ApplicablePlanIDs = []int64{}
	}
	// 兼容旧字段（用于向后兼容）
	if coupon.DiscountType != "" {
		couponDTO.DiscountType = coupon.DiscountType
	}

	return couponDTO
}

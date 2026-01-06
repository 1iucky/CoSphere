package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ===================== 用户优惠券 API =====================

// UserCouponResponse 用户优惠券响应结构
type UserCouponResponse struct {
	ID             int64  `json:"id"`
	CouponID       int64  `json:"coupon_id"`
	Code           string `json:"code"`
	Status         string `json:"status"`
	ClaimedAt      int64  `json:"claimed_at"`
	UsedAt         *int64 `json:"used_at"`
	OrderID        *int64 `json:"order_id"`
	DiscountAmount int64  `json:"discount_amount"`
	FinalAmount    int64  `json:"final_amount"`
	CreatedAt      int64  `json:"created_at"`

	// 优惠券模板详情
	CouponName        string `json:"coupon_name"`
	CouponType        string `json:"coupon_type"`
	CouponScope       string `json:"coupon_scope"`
	DiscountValue     int64  `json:"discount_value"`
	ThresholdAmount   int64  `json:"threshold_amount"`
	Currency          string `json:"currency"`
	ValidFrom         int64  `json:"valid_from"`
	ValidTo           int64  `json:"valid_to"`
	RemainingDays     int    `json:"remaining_days"`
	CanUse            bool   `json:"can_use"`
}

// ClaimCouponRequest 领取优惠券请求
type ClaimCouponRequest struct {
	ClaimCode string `json:"claim_code" binding:"required"`
}

// CouponPreviewRequest 优惠券预览请求
type CouponPreviewRequest struct {
	UserCouponID int64  `json:"user_coupon_id" binding:"required,min=1"`
	OrderAmount  int64  `json:"order_amount" binding:"required,min=1"`
	Scene        string `json:"scene" binding:"required,oneof=wallet subscription"`
	PlanID       *int64 `json:"plan_id"`
	Currency     string `json:"currency"`
}

// CouponPreviewResponse 优惠券预览响应
type CouponPreviewResponse struct {
	Valid          bool   `json:"valid"`
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	OriginalAmount int64  `json:"original_amount"`
	DiscountAmount int64  `json:"discount_amount"`
	FinalAmount    int64  `json:"final_amount"`
	CouponType     string `json:"coupon_type"`
	ThresholdMet   bool   `json:"threshold_met"`
}

// ClaimCoupon 用户领取优惠券
// POST /api/user/coupons/claim
func ClaimCoupon(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req ClaimCouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 使用 CouponService 领取优惠券
	svc := service.GetCouponService()
	userCoupon, err := svc.ClaimCoupon(int64(userId), req.ClaimCode)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 获取优惠券模板信息
	coupon, _ := model.GetCouponById(userCoupon.CouponId)

	response := convertUserCouponToResponse(userCoupon, coupon)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetUserCoupons 获取用户优惠券列表
// GET /api/user/coupons
func GetUserCoupons(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	// 解析查询参数
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 使用 CouponService 获取列表
	svc := service.GetCouponService()
	result, err := svc.GetUserCoupons(int64(userId), status, page, pageSize)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为响应格式
	var responses []UserCouponResponse
	for _, item := range result.Coupons {
		response := convertUserCouponToResponse(item.UserCoupon, item.CouponDetail)
		response.RemainingDays = item.RemainingDays
		response.CanUse = item.CanUse
		responses = append(responses, response)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    responses,
		"total":   result.Total,
	})
}

// GetUserCouponDetail 获取用户优惠券详情
// GET /api/user/coupons/:id
func GetUserCouponDetail(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	couponId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的优惠券 ID",
		})
		return
	}

	// 获取用户优惠券
	userCoupon, err := model.GetUserCouponById(couponId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证归属
	if userCoupon.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权访问此优惠券",
		})
		return
	}

	// 获取优惠券模板信息
	coupon, _ := model.GetCouponById(userCoupon.CouponId)

	response := convertUserCouponToResponse(userCoupon, coupon)

	// 计算可用性
	if coupon != nil {
		response.RemainingDays = coupon.GetRemainingValidDays()
		response.CanUse = userCoupon.Status == common.UserCouponStatusAvailable && !coupon.IsExpired()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// PreviewCouponUsage 优惠券使用预览
// POST /api/user/payment/coupon/preview
func PreviewCouponUsage(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	var req CouponPreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 使用 CouponService 预览
	svc := service.GetCouponService()
	result, err := svc.PreviewCouponUsageWithPlan(
		int64(userId),
		req.UserCouponID,
		req.OrderAmount,
		req.Scene,
		req.PlanID,
		req.Currency,
	)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	response := CouponPreviewResponse{
		Valid:          result.Valid,
		ErrorCode:      result.ErrorCode,
		ErrorMessage:   result.ErrorMessage,
		OriginalAmount: result.OriginalAmount,
		DiscountAmount: result.DiscountAmount,
		FinalAmount:    result.FinalAmount,
		CouponType:     result.CouponType,
		ThresholdMet:   result.ThresholdMet,
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    response,
	})
}

// GetAvailableCoupons 获取用户可领取的优惠券列表
// GET /api/user/coupons/available
func GetAvailableCoupons(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	// 获取所有有效的优惠券
	coupons, err := model.GetActiveCoupons()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 过滤掉用户已领取的优惠券
	var availableCoupons []gin.H
	for _, coupon := range coupons {
		// 检查用户是否已领取
		existingCoupon, _ := model.GetUserCouponByUserAndCoupon(int64(userId), coupon.Id)
		if existingCoupon != nil {
			continue // 已领取，跳过
		}

		// 检查是否绑定到其他用户
		if coupon.BindUserId != nil && *coupon.BindUserId != int64(userId) {
			continue // 绑定到其他用户，跳过
		}

		// 检查是否绑定到兑换码（绑定券只能通过兑换码流程领取）
		isBound, err := model.IsCouponBoundToRedemption(coupon.Id)
		if err != nil {
			common.SysLog(fmt.Sprintf("GetAvailableCoupons: failed to check coupon binding for coupon_id=%d, err=%v", coupon.Id, err))
			continue // 查询失败，保守处理跳过并记录日志
		}
		if isBound {
			continue // 绑定到兑换码，跳过
		}

		availableCoupons = append(availableCoupons, gin.H{
			"id":               coupon.Id,
			"code":             coupon.Code,
			"name":             coupon.Name,
			"description":      coupon.Description,
			"type":             coupon.Type,
			"scope":            coupon.Scope,
			"discount_value":   coupon.DiscountValue,
			"threshold_amount": coupon.ThresholdAmount,
			"currency":         coupon.Currency,
			"valid_from":       coupon.ValidFrom,
			"valid_to":         coupon.ValidTo,
			"remaining_count":  coupon.GetRemainingCount(),
			"remaining_days":   coupon.GetRemainingValidDays(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    availableCoupons,
	})
}

// ===================== 辅助函数 =====================

// convertUserCouponToResponse 转换用户优惠券为响应格式
func convertUserCouponToResponse(userCoupon *model.UserCoupon, coupon *model.Coupon) UserCouponResponse {
	response := UserCouponResponse{
		ID:             userCoupon.Id,
		CouponID:       userCoupon.CouponId,
		Code:           userCoupon.Code,
		Status:         userCoupon.Status,
		ClaimedAt:      userCoupon.ClaimedAt,
		UsedAt:         userCoupon.UsedAt,
		OrderID:        userCoupon.OrderId,
		DiscountAmount: userCoupon.DiscountAmount,
		FinalAmount:    userCoupon.FinalAmount,
		CreatedAt:      userCoupon.CreatedAt,
	}

	if coupon != nil {
		response.CouponName = coupon.Name
		response.CouponType = coupon.Type
		response.CouponScope = coupon.Scope
		response.DiscountValue = coupon.DiscountValue
		response.ThresholdAmount = coupon.ThresholdAmount
		response.Currency = coupon.Currency
		response.ValidFrom = coupon.ValidFrom
		response.ValidTo = coupon.ValidTo
		response.RemainingDays = coupon.GetRemainingValidDays()
		response.CanUse = userCoupon.Status == common.UserCouponStatusAvailable && !coupon.IsExpired()
	}

	return response
}

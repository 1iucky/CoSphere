package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ===================== 2.16.2 用户兑换订阅 API =====================

// UseSubscriptionRedemptionRequest 用户兑换订阅请求
type UseSubscriptionRedemptionRequest struct {
	RedemptionCode string `json:"redemption_code" binding:"required"` // 兑换码
	RedeemOption   string `json:"redeem_option"`                      // 兑换选项：stack/coexist/convert/replace/extend（可选，用于冲突时指定）
}

// UseSubscriptionRedemptionResponse 用户兑换订阅响应
type UseSubscriptionRedemptionResponse struct {
	Success        bool                     `json:"success"`
	SubscriptionId int64                    `json:"subscription_id,omitempty"`
	RedeemOption   string                   `json:"redeem_option,omitempty"`    // 实际使用的兑换选项
	ExtendedDays   int                      `json:"extended_days,omitempty"`    // 延长天数（stack/extend 场景）
	NewEndAt       int64                    `json:"new_end_at,omitempty"`       // 新结束时间
	CouponUsed     bool                     `json:"coupon_used,omitempty"`      // 是否使用了优惠券
	DiscountAmount int64                    `json:"discount_amount,omitempty"`  // 优惠金额
	Idempotent     bool                     `json:"idempotent,omitempty"`       // 幂等标识：重复请求时为 true
	ConversionInfo *ConversionInfoResponse  `json:"conversion_info,omitempty"`  // 折算信息（convert 场景）
	Conflict       *ConflictCheckResponse   `json:"conflict,omitempty"`         // 冲突信息（需要用户选择时返回）
	ErrorCode      string                   `json:"error_code,omitempty"`
	Message        string                   `json:"message"`
}

// ConversionInfoResponse 折算信息响应
type ConversionInfoResponse struct {
	OldSubscriptionId    int64   `json:"old_subscription_id"`
	OldPlanId            int64   `json:"old_plan_id"`
	OldRemainingDays     int     `json:"old_remaining_days"`
	OldDailyPrice        float64 `json:"old_daily_price"`       // 旧套餐日均价（元）
	OldRemainingValue    float64 `json:"old_remaining_value"`   // 旧套餐剩余价值（元）
	NewPlanId            int64   `json:"new_plan_id"`
	NewDailyPrice        float64 `json:"new_daily_price"`       // 新套餐日均价（元）
	ConvertedDays        int     `json:"converted_days"`        // 折算天数
	RemainderAmount      float64 `json:"remainder_amount"`      // 不足一天的金额（元）
	NewSubscriptionEndAt int64   `json:"new_subscription_end_at"`
}

// ConflictCheckResponse 冲突检测响应
type ConflictCheckResponse struct {
	HasConflict       bool                       `json:"has_conflict"`
	ConflictType      string                     `json:"conflict_type"`         // same_plan / more_expensive / different_plan
	ExistingPlanId    int64                      `json:"existing_plan_id,omitempty"`
	ExistingPlanName  string                     `json:"existing_plan_name,omitempty"`
	ExistingPlanPrice float64                    `json:"existing_plan_price,omitempty"` // 元
	NewPlanId         int64                      `json:"new_plan_id,omitempty"`
	NewPlanName       string                     `json:"new_plan_name,omitempty"`
	NewPlanPrice      float64                    `json:"new_plan_price,omitempty"`      // 元
	AvailableOptions  []string                   `json:"available_options"`
	RecommendedOption string                     `json:"recommended_option"`
	ConversionPreview *ConversionPreviewResponse `json:"conversion_preview,omitempty"`
}

// ConversionPreviewResponse 折算预览响应
type ConversionPreviewResponse struct {
	RemainingDays   int     `json:"remaining_days"`
	RemainingValue  float64 `json:"remaining_value"`   // 元
	ConvertedDays   int     `json:"converted_days"`
	RemainderAmount float64 `json:"remainder_amount"`  // 元
	NewEndAt        int64   `json:"new_end_at"`
}

// UseSubscriptionRedemption 用户兑换订阅
// POST /api/user/redemptions/use
func UseSubscriptionRedemption(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: "INVALID_USER",
			Message:   "无法获取用户信息",
		})
		return
	}

	var req UseSubscriptionRedemptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: "INVALID_REQUEST",
			Message:   "请求参数错误: " + err.Error(),
		})
		return
	}

	// 验证兑换选项合法性（如果提供）
	if req.RedeemOption != "" {
		validOptions := map[string]bool{
			common.RedeemOptionStack:   true,
			common.RedeemOptionCoexist: true,
			common.RedeemOptionConvert: true,
			common.RedeemOptionReplace: true,
			common.RedeemOptionExtend:  true,
		}
		if !validOptions[req.RedeemOption] {
			c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
				Success:   false,
				ErrorCode: "INVALID_REDEEM_OPTION",
				Message:   "无效的兑换选项，必须是 stack/coexist/convert/replace/extend 之一",
			})
			return
		}
	}

	// 调用 RedemptionService 执行兑换
	svc := service.GetRedemptionService()
	result, err := svc.RedeemWithCoupon(userId, req.RedemptionCode, req.RedeemOption)

	// 处理兑换结果
	response := buildUseRedemptionResponse(result, err)
	c.JSON(http.StatusOK, response)
}

// PreviewSubscriptionRedemption 预览兑换结果（检测冲突）
// POST /api/user/redemptions/preview
func PreviewSubscriptionRedemption(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: "INVALID_USER",
			Message:   "无法获取用户信息",
		})
		return
	}

	var req struct {
		RedemptionCode string `json:"redemption_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: "INVALID_REQUEST",
			Message:   "请求参数错误: " + err.Error(),
		})
		return
	}

	// 调用 RedemptionService 获取冲突信息
	svc := service.GetRedemptionService()
	conflictResult, err := svc.PromptUserChoice(int64(userId), req.RedemptionCode)
	if err != nil {
		c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: "REDEMPTION_CHECK_FAILED",
			Message:   err.Error(),
		})
		return
	}

	// 构建冲突响应
	conflictResponse := convertConflictToResponse(conflictResult)

	c.JSON(http.StatusOK, UseSubscriptionRedemptionResponse{
		Success:  true,
		Message:  "兑换预览成功",
		Conflict: conflictResponse,
	})
}

// ===================== 辅助函数 =====================

// buildUseRedemptionResponse 构建兑换响应
func buildUseRedemptionResponse(result *service.RedemptionResult, err error) UseSubscriptionRedemptionResponse {
	if result == nil {
		return UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: "REDEMPTION_FAILED",
			Message:   "兑换失败",
		}
	}

	// 兑换冲突场景：需要用户选择处理方式
	if result.ErrorCode == "REDEMPTION_CONFLICT" {
		return UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: result.ErrorCode,
			Message:   result.ErrorMessage,
		}
	}

	// 兑换失败
	if !result.Success || err != nil {
		errorMsg := result.ErrorMessage
		if err != nil && errorMsg == "" {
			errorMsg = err.Error()
		}
		return UseSubscriptionRedemptionResponse{
			Success:   false,
			ErrorCode: result.ErrorCode,
			Message:   errorMsg,
		}
	}

	// 兑换成功（包括幂等场景）
	response := UseSubscriptionRedemptionResponse{
		Success:        true,
		SubscriptionId: result.SubscriptionId,
		RedeemOption:   result.RedeemOption,
		ExtendedDays:   result.ExtendedDays,
		NewEndAt:       result.NewEndAt,
		CouponUsed:     result.CouponUsed,
		DiscountAmount: result.DiscountAmount,
		Idempotent:     result.Idempotent,
		Message:        buildSuccessMessage(result),
	}

	// 转换折算信息
	if result.ConversionInfo != nil {
		response.ConversionInfo = &ConversionInfoResponse{
			OldSubscriptionId:    result.ConversionInfo.OldSubscriptionId,
			OldPlanId:            result.ConversionInfo.OldPlanId,
			OldRemainingDays:     result.ConversionInfo.OldRemainingDays,
			OldDailyPrice:        result.ConversionInfo.OldDailyPrice.InexactFloat64() / 100, // 分转元
			OldRemainingValue:    result.ConversionInfo.OldRemainingValue.InexactFloat64() / 100,
			NewPlanId:            result.ConversionInfo.NewPlanId,
			NewDailyPrice:        result.ConversionInfo.NewDailyPrice.InexactFloat64() / 100,
			ConvertedDays:        result.ConversionInfo.ConvertedDays,
			RemainderAmount:      result.ConversionInfo.RemainderAmount.InexactFloat64() / 100,
			NewSubscriptionEndAt: result.ConversionInfo.NewSubscriptionEndAt,
		}
	}

	return response
}

// convertConflictToResponse 转换冲突检测结果为响应格式
func convertConflictToResponse(result *service.ConflictCheckResult) *ConflictCheckResponse {
	if result == nil {
		return nil
	}

	response := &ConflictCheckResponse{
		HasConflict:       result.HasConflict,
		ConflictType:      result.ConflictType,
		ExistingPlanId:    result.ExistingPlanId,
		ExistingPlanName:  result.ExistingPlanName,
		ExistingPlanPrice: float64(result.ExistingPlanPrice) / 100, // 分转元
		NewPlanId:         result.NewPlanId,
		NewPlanName:       result.NewPlanName,
		NewPlanPrice:      float64(result.NewPlanPrice) / 100,
		AvailableOptions:  result.AvailableOptions,
		RecommendedOption: result.RecommendedOption,
	}

	// 转换折算预览
	if result.ConversionPreview != nil {
		response.ConversionPreview = &ConversionPreviewResponse{
			RemainingDays:   result.ConversionPreview.RemainingDays,
			RemainingValue:  result.ConversionPreview.RemainingValue.InexactFloat64() / 100,
			ConvertedDays:   result.ConversionPreview.ConvertedDays,
			RemainderAmount: result.ConversionPreview.RemainderAmount.InexactFloat64() / 100,
			NewEndAt:        result.ConversionPreview.NewEndAt,
		}
	}

	return response
}

// buildSuccessMessage 构建成功消息
func buildSuccessMessage(result *service.RedemptionResult) string {
	switch result.RedeemOption {
	case common.RedeemOptionStack, common.RedeemOptionExtend:
		return "兑换成功，订阅已延长"
	case common.RedeemOptionCoexist:
		return "兑换成功，已创建新订阅"
	case common.RedeemOptionConvert:
		return "兑换成功，旧订阅已折算为新订阅"
	case common.RedeemOptionReplace:
		return "兑换成功，已替换旧订阅"
	default:
		return "兑换成功"
	}
}

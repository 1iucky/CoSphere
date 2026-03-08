package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// ===================== 2.7 Service 层 - 兑换码服务扩展 =====================

// RedemptionService 兑换码服务
// 负责订阅类型兑换码的核心业务逻辑
type RedemptionService struct {
	couponService   *CouponService
	priorityService *SubscriptionPriorityService
}

var (
	redemptionServiceInstance *RedemptionService
	redemptionServiceOnce     sync.Once
)

// GetRedemptionService 获取兑换码服务单例
func GetRedemptionService() *RedemptionService {
	redemptionServiceOnce.Do(func() {
		redemptionServiceInstance = &RedemptionService{
			couponService:   GetCouponService(),
			priorityService: GetSubscriptionPriorityService(),
		}
	})
	return redemptionServiceInstance
}

// ===================== 兑换结果定义 =====================

// RedemptionResult 兑换结果
type RedemptionResult struct {
	Success        bool   `json:"success"`
	RedeemOption   string `json:"redeem_option"`   // 最终使用的兑换选项
	SubscriptionId int64  `json:"subscription_id"` // 订阅 ID
	ErrorCode      string `json:"error_code,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`

	// 幂等性支持：当兑换码已被当前用户使用时返回 true
	Idempotent bool `json:"idempotent,omitempty"`

	// 叠加/延期场景
	ExtendedDays int   `json:"extended_days,omitempty"` // 延长天数
	NewEndAt     int64 `json:"new_end_at,omitempty"`    // 新结束时间

	// 折算场景
	ConversionInfo *ConversionInfo `json:"conversion_info,omitempty"`

	// 新订阅场景（coexist）
	NewSubscription *model.Subscription `json:"new_subscription,omitempty"`

	// 优惠券场景
	CouponUsed     bool  `json:"coupon_used"`
	DiscountAmount int64 `json:"discount_amount,omitempty"`
}

// ConversionInfo 折算信息
type ConversionInfo struct {
	OldSubscriptionId    int64           `json:"old_subscription_id"`
	OldPlanId            int64           `json:"old_plan_id"`
	OldRemainingDays     int             `json:"old_remaining_days"`
	OldDailyPrice        decimal.Decimal `json:"old_daily_price"`     // 旧套餐日均价（分）
	OldRemainingValue    decimal.Decimal `json:"old_remaining_value"` // 旧套餐剩余价值（分）
	NewPlanId            int64           `json:"new_plan_id"`
	NewDailyPrice        decimal.Decimal `json:"new_daily_price"`  // 新套餐日均价（分）
	ConvertedDays        int             `json:"converted_days"`   // 折算天数（向下取整）
	RemainderAmount      decimal.Decimal `json:"remainder_amount"` // 不足一天的金额（分，记入流水）
	NewSubscriptionEndAt int64           `json:"new_subscription_end_at"`
}

// ConflictCheckResult 冲突检测结果
type ConflictCheckResult struct {
	HasConflict       bool                     `json:"has_conflict"`
	ConflictType      string                   `json:"conflict_type"` // same_plan / more_expensive / different_plan
	ExistingPlanId    int64                    `json:"existing_plan_id,omitempty"`
	ExistingPlanName  string                   `json:"existing_plan_name,omitempty"`
	ExistingPlanPrice int64                    `json:"existing_plan_price,omitempty"`
	NewPlanId         int64                    `json:"new_plan_id,omitempty"`
	NewPlanName       string                   `json:"new_plan_name,omitempty"`
	NewPlanPrice      int64                    `json:"new_plan_price,omitempty"`
	AvailableOptions  []string                 `json:"available_options"`  // 可选的兑换选项
	RecommendedOption string                   `json:"recommended_option"` // 推荐的选项
	ConversionPreview *ConversionPreviewResult `json:"conversion_preview,omitempty"`
}

// ConversionPreviewResult 折算预览结果
type ConversionPreviewResult struct {
	RemainingDays   int             `json:"remaining_days"`
	RemainingValue  decimal.Decimal `json:"remaining_value"`
	ConvertedDays   int             `json:"converted_days"`
	RemainderAmount decimal.Decimal `json:"remainder_amount"`
	NewEndAt        int64           `json:"new_end_at"`
}

// ===================== 2.7.1 支持订阅类型兑换 =====================

// RedeemSubscription 兑换订阅（主入口）
// 支持所有兑换选项：stack/coexist/convert/replace/extend
// 支持幂等性：重复兑换同一兑换码时返回原兑换结果
func (s *RedemptionService) RedeemSubscription(userId int, key string, redeemOptionOverride string) (*RedemptionResult, error) {
	result := &RedemptionResult{
		Success: false,
	}

	if userId == 0 {
		result.ErrorCode = "INVALID_USER_ID"
		result.ErrorMessage = "用户 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}
	if key == "" {
		result.ErrorCode = "INVALID_KEY"
		result.ErrorMessage = "兑换码不能为空"
		return result, errors.New(result.ErrorMessage)
	}

	// 1. 获取并验证兑换码（事务外预检，减少锁竞争）
	redemption, err := model.GetRedemptionByKey(key)
	if err != nil {
		result.ErrorCode = "REDEMPTION_NOT_FOUND"
		result.ErrorMessage = common.MsgRedemptionNotFound
		return result, err
	}

	// 2. 预检：验证兑换码类型
	if !redemption.IsSubscriptionRedemption() {
		result.ErrorCode = "REDEMPTION_INVALID_TYPE"
		result.ErrorMessage = common.MsgRedemptionInvalidType
		return result, errors.New(result.ErrorMessage)
	}

	// 3. 预检：验证兑换码状态（带幂等性检查）
	isIdempotent, validationErr := s.validateRedemptionWithIdempotency(redemption, userId, result)
	if isIdempotent {
		// 幂等场景：兑换码已被当前用户使用，直接返回成功结果
		return result, nil
	}
	if validationErr != nil {
		return result, validationErr
	}

	// 4. 预检：验证专属用户（如果设置）
	if err := redemption.ValidateBoundUser(userId); err != nil {
		result.ErrorCode = "REDEMPTION_USER_MISMATCH"
		result.ErrorMessage = err.Error()
		return result, err
	}

	// 5. 事务内执行兑换
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// 5.1 获取兑换码并加锁
		lockedRedemption, err := model.GetRedemptionByKeyWithTx(tx, key, true)
		if err != nil {
			result.ErrorCode = "REDEMPTION_NOT_FOUND"
			result.ErrorMessage = common.MsgRedemptionNotFound
			return err
		}

		// 5.2 事务内重新校验类型（防止 TOCTOU）
		if !lockedRedemption.IsSubscriptionRedemption() {
			result.ErrorCode = "REDEMPTION_INVALID_TYPE"
			result.ErrorMessage = common.MsgRedemptionInvalidType
			return errors.New(result.ErrorMessage)
		}

		// 5.3 事务内重新校验状态（带幂等性检查，防止 TOCTOU）
		isIdempotent, validationErr := s.validateRedemptionWithIdempotency(lockedRedemption, userId, result)
		if isIdempotent {
			// 幂等场景：兑换码在并发中已被当前用户使用
			return nil
		}
		if validationErr != nil {
			return validationErr
		}

		// 5.4 事务内重新校验专属用户
		if err := lockedRedemption.ValidateBoundUser(userId); err != nil {
			result.ErrorCode = "REDEMPTION_USER_MISMATCH"
			result.ErrorMessage = err.Error()
			return err
		}

		// 5.5 事务内重新解析 payload（防止 TOCTOU）
		payload, err := lockedRedemption.GetSubscriptionPayload()
		if err != nil {
			result.ErrorCode = "REDEMPTION_PAYLOAD_INVALID"
			result.ErrorMessage = "兑换码数据无效"
			return err
		}

		// 5.6 获取套餐信息
		plan, err := model.GetSubscriptionPlanByIdWithTx(tx, payload.PlanId)
		if err != nil {
			result.ErrorCode = "PLAN_NOT_FOUND"
			result.ErrorMessage = common.MsgPlanNotFound
			return err
		}

		// 5.7 确定兑换选项（优先使用用户指定的，否则使用 payload 中的默认值）
		redeemOption := payload.RedeemOption
		if redeemOptionOverride != "" {
			redeemOption = redeemOptionOverride
		}

		// 5.8 检测套餐冲突
		conflictResult, err := s.checkSubscriptionConflictWithTx(tx, int64(userId), payload.PlanId)
		if err != nil {
			return err
		}

		// 5.9 根据冲突类型和兑换选项执行相应逻辑，并更新 result.RedeemOption 为实际执行的选项
		switch conflictResult.ConflictType {
		case "same_plan":
			// 同套餐：支持 stack 和 extend（两者行为相同，都是延长现有订阅）
			switch redeemOption {
			case common.RedeemOptionStack, common.RedeemOptionExtend, "":
				// 更新为实际执行的选项
				if redeemOption == "" {
					result.RedeemOption = common.RedeemOptionStack
				} else {
					result.RedeemOption = redeemOption
				}
				return s.executeStackRedemption(tx, result, int64(userId), lockedRedemption, plan, payload)
			default:
				// 不支持的选项，默认执行 stack，更新 RedeemOption 为实际执行的选项
				result.RedeemOption = common.RedeemOptionStack
				return s.executeStackRedemption(tx, result, int64(userId), lockedRedemption, plan, payload)
			}

		case "more_expensive":
			// 更贵套餐：支持 coexist、convert、replace
			switch redeemOption {
			case common.RedeemOptionCoexist:
				result.RedeemOption = common.RedeemOptionCoexist
				return s.executeCoexistRedemption(tx, result, int64(userId), lockedRedemption, plan, payload)
			case common.RedeemOptionConvert:
				result.RedeemOption = common.RedeemOptionConvert
				return s.executeConvertRedemption(tx, result, int64(userId), lockedRedemption, plan, payload, conflictResult.ExistingPlanId)
			case common.RedeemOptionReplace:
				result.RedeemOption = common.RedeemOptionReplace
				return s.executeReplaceRedemption(tx, result, int64(userId), lockedRedemption, plan, payload, conflictResult.ExistingPlanId)
			default:
				// 返回冲突错误码，前端需提示用户选择
				result.ErrorCode = "REDEMPTION_CONFLICT"
				result.ErrorMessage = "检测到套餐冲突，请选择处理方式：coexist（并存）、convert（折算）或 replace（替换）"
				return errors.New(result.ErrorMessage)
			}

		case "different_plan":
			// 不同套餐（更便宜或无关）：支持 coexist、replace
			switch redeemOption {
			case common.RedeemOptionReplace:
				result.RedeemOption = common.RedeemOptionReplace
				return s.executeReplaceRedemption(tx, result, int64(userId), lockedRedemption, plan, payload, conflictResult.ExistingPlanId)
			default:
				// 默认共存，更新 RedeemOption 为实际执行的选项
				result.RedeemOption = common.RedeemOptionCoexist
				return s.executeCoexistRedemption(tx, result, int64(userId), lockedRedemption, plan, payload)
			}

		default:
			// 无冲突：直接创建新订阅
			result.RedeemOption = common.RedeemOptionCoexist
			return s.executeCoexistRedemption(tx, result, int64(userId), lockedRedemption, plan, payload)
		}
	})

	if err != nil {
		if result.ErrorCode == "" {
			result.ErrorCode = "REDEMPTION_FAILED"
		}
		if result.ErrorMessage == "" {
			result.ErrorMessage = err.Error()
		}
		return result, err
	}

	// 事务提交成功后刷新缓存并预热
	s.priorityService.InvalidateUserSubscriptionCache(int64(userId))
	cacheSvc := GetSubscriptionCacheService()
	go func() {
		_ = cacheSvc.WarmupCache(int64(userId))
	}()

	result.Success = true
	return result, nil
}

// validateRedemptionWithDetailedError 验证兑换码状态，返回细分的错误码
func (s *RedemptionService) validateRedemptionWithDetailedError(redemption *model.Redemption, result *RedemptionResult) error {
	// 检查状态
	if redemption.Status == common.RedemptionCodeStatusUsed {
		result.ErrorCode = "REDEMPTION_USED"
		result.ErrorMessage = common.MsgRedemptionUsed
		return errors.New(result.ErrorMessage)
	}

	if redemption.Status == common.RedemptionCodeStatusDisabled {
		result.ErrorCode = "REDEMPTION_DISABLED"
		result.ErrorMessage = "该兑换码已被禁用"
		return errors.New(result.ErrorMessage)
	}

	if redemption.Status != common.RedemptionCodeStatusEnabled {
		result.ErrorCode = "REDEMPTION_INVALID"
		result.ErrorMessage = "兑换码状态无效"
		return errors.New(result.ErrorMessage)
	}

	// 检查过期
	if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
		result.ErrorCode = "REDEMPTION_EXPIRED"
		result.ErrorMessage = common.MsgRedemptionExpired
		return errors.New(result.ErrorMessage)
	}

	return nil
}

// validateRedemptionWithIdempotency 带幂等性检查的验证
// 若兑换码已被当前用户使用，返回原订阅结果（幂等）；若被其他用户使用，返回错误
// 修复 2.16.3：支持 stack/extend 场景的幂等性，通过审计日志查询订阅信息
func (s *RedemptionService) validateRedemptionWithIdempotency(redemption *model.Redemption, userId int, result *RedemptionResult) (isIdempotent bool, err error) {
	// 检查状态 - 已使用时检查幂等性
	if redemption.Status == common.RedemptionCodeStatusUsed {
		// 检查是否被当前用户使用
		if redemption.UsedUserId == userId {
			// 幂等场景1：通过 subscription.redemption_id 查询（coexist/replace/convert 场景）
			existingSub, err := model.GetSubscriptionByRedemptionId(int64(redemption.Id))
			if err != nil {
				result.ErrorCode = "IDEMPOTENT_CHECK_FAILED"
				result.ErrorMessage = "幂等性检查失败"
				return false, err
			}

			if existingSub != nil {
				// 填充幂等结果
				result.Success = true
				result.Idempotent = true
				result.SubscriptionId = existingSub.Id
				result.RedeemOption = existingSub.RedeemOption
				result.NewEndAt = existingSub.EndAt
				result.ErrorCode = ""
				result.ErrorMessage = ""
				return true, nil
			}

			// 幂等场景2：stack/extend 场景，通过审计日志查询订阅信息
			// 这种场景下 subscription.redemption_id 未设置，需要从审计日志中获取
			auditLog, err := model.GetRedemptionAuditLogByRedemptionId(int64(userId), int64(redemption.Id))
			if err != nil {
				result.ErrorCode = "IDEMPOTENT_CHECK_FAILED"
				result.ErrorMessage = "幂等性检查失败"
				return false, err
			}

			if auditLog != nil && auditLog.ObjectId != nil {
				// 从审计日志中获取订阅 ID
				subId := *auditLog.ObjectId
				sub, err := model.GetSubscriptionById(subId)
				if err == nil && sub != nil {
					// 解析审计日志 metadata 获取 redeem_option
					redeemOption := s.extractRedeemOptionFromAuditLog(auditLog)

					// 填充幂等结果
					result.Success = true
					result.Idempotent = true
					result.SubscriptionId = sub.Id
					result.RedeemOption = redeemOption
					result.NewEndAt = sub.EndAt
					result.ErrorCode = ""
					result.ErrorMessage = ""
					return true, nil
				}
			}
		}

		// 被其他用户使用
		result.ErrorCode = "REDEMPTION_USED"
		result.ErrorMessage = common.MsgRedemptionUsed
		return false, errors.New(result.ErrorMessage)
	}

	if redemption.Status == common.RedemptionCodeStatusDisabled {
		result.ErrorCode = "REDEMPTION_DISABLED"
		result.ErrorMessage = "该兑换码已被禁用"
		return false, errors.New(result.ErrorMessage)
	}

	if redemption.Status != common.RedemptionCodeStatusEnabled {
		result.ErrorCode = "REDEMPTION_INVALID"
		result.ErrorMessage = "兑换码状态无效"
		return false, errors.New(result.ErrorMessage)
	}

	// 检查过期
	if redemption.ExpiredTime != 0 && redemption.ExpiredTime < common.GetTimestamp() {
		result.ErrorCode = "REDEMPTION_EXPIRED"
		result.ErrorMessage = common.MsgRedemptionExpired
		return false, errors.New(result.ErrorMessage)
	}

	return false, nil
}

// ===================== 2.7.2 同套餐叠加逻辑 =====================

// ExtendSubscription 延长现有订阅的有效期
// 用于同套餐叠加场景
func (s *RedemptionService) ExtendSubscription(subscriptionId int64, additionalSeconds int64) error {
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}
	if additionalSeconds <= 0 {
		return errors.New("延长时间必须大于 0")
	}

	return model.DB.Transaction(func(tx *gorm.DB) error {
		return s.extendSubscriptionWithTx(tx, subscriptionId, additionalSeconds)
	})
}

// extendSubscriptionWithTx 在事务中延长订阅
func (s *RedemptionService) extendSubscriptionWithTx(tx *gorm.DB, subscriptionId int64, additionalSeconds int64) error {
	// 获取订阅并加锁
	sub, err := model.GetSubscriptionByIdWithTx(tx, subscriptionId, true)
	if err != nil {
		return err
	}

	// 更新结束时间
	sub.ExtendEndAt(additionalSeconds)

	// 保存更新
	return model.UpdateSubscriptionWithTx(tx, sub)
}

// executeStackRedemption 执行叠加兑换
func (s *RedemptionService) executeStackRedemption(tx *gorm.DB, result *RedemptionResult, userId int64, redemption *model.Redemption, plan *model.SubscriptionPlan, payload *model.RedemptionSubscriptionPayload) error {
	// 1. 获取用户该套餐的现有订阅
	existingSub, err := s.getUserSubscriptionByPlanWithTx(tx, userId, plan.Id)
	if err != nil {
		return fmt.Errorf("获取现有订阅失败: %w", err)
	}

	if existingSub == nil {
		// 没有现有订阅，创建新订阅
		return s.executeCoexistRedemption(tx, result, userId, redemption, plan, payload)
	}

	// 2. 计算延长时间
	durationSeconds := payload.GetDurationSeconds()
	extendedDays := int(durationSeconds / (24 * 60 * 60))

	// 3. 延长订阅（内部会加锁并更新）
	if err := s.extendSubscriptionWithTx(tx, existingSub.Id, durationSeconds); err != nil {
		return fmt.Errorf("延长订阅失败: %w", err)
	}

	// 修复：重新读取订阅以获取准确的 NewEndAt（避免并发下返回旧值）
	updatedSub, err := model.GetSubscriptionByIdWithTx(tx, existingSub.Id, false)
	if err != nil {
		return fmt.Errorf("读取更新后的订阅失败: %w", err)
	}

	// 4. 标记兑换码已使用
	if err := model.RedeemSubscriptionWithTx(tx, redemption, int(userId)); err != nil {
		return fmt.Errorf("标记兑换码失败: %w", err)
	}

	// 5. 记录账单流水
	description := fmt.Sprintf("兑换码叠加延长订阅 %d 天，套餐：%s", extendedDays, plan.Name)
	if err := s.recordRedemptionBillWithTx(tx, userId, redemption, plan, payload, description); err != nil {
		return fmt.Errorf("记录账单失败: %w", err)
	}

	// 6. 记录审计日志
	if err := s.logRedemptionAudit(tx, userId, redemption.Id, redemption.Key, plan.Id, existingSub.Id, "stack", common.RedeemOptionStack, nil); err != nil {
		return fmt.Errorf("记录审计日志失败: %w", err)
	}

	// 注意：缓存刷新移到事务提交后执行，避免读取到未提交的数据

	// 设置结果（使用重新读取的准确值）
	result.SubscriptionId = existingSub.Id
	result.ExtendedDays = extendedDays
	result.NewEndAt = updatedSub.EndAt

	return nil
}

// ===================== 2.7.3 更贵套餐二选一逻辑 =====================

// CheckSubscriptionConflict 检测套餐冲突
// 返回冲突类型和可用选项
func (s *RedemptionService) CheckSubscriptionConflict(userId int64, newPlanId int64) (*ConflictCheckResult, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if newPlanId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	return s.checkSubscriptionConflictWithTx(model.DB, userId, newPlanId)
}

// checkSubscriptionConflictWithTx 在事务中检测套餐冲突
// 修复：多订阅场景下，优先匹配相同套餐，否则取最贵套餐作为冲突基准
func (s *RedemptionService) checkSubscriptionConflictWithTx(tx *gorm.DB, userId int64, newPlanId int64) (*ConflictCheckResult, error) {
	result := &ConflictCheckResult{
		HasConflict:      false,
		NewPlanId:        newPlanId,
		AvailableOptions: []string{common.RedeemOptionCoexist},
	}

	// 获取新套餐信息
	newPlan, err := model.GetSubscriptionPlanByIdWithTx(tx, newPlanId)
	if err != nil {
		return nil, err
	}
	result.NewPlanName = newPlan.Name
	result.NewPlanPrice = newPlan.PriceCents

	// 获取用户所有活跃订阅
	activeSubs, err := model.GetActiveSubscriptionsByUserWithTx(tx, userId)
	if err != nil {
		return nil, err
	}

	if len(activeSubs) == 0 {
		// 无现有订阅，无冲突
		return result, nil
	}

	// 修复：多订阅场景下的基准选择逻辑
	// 1. 优先查找相同套餐的订阅（same_plan）
	// 2. 否则选择最贵的套餐作为冲突基准

	var samePlanSub *model.Subscription
	var mostExpensiveSub *model.Subscription
	var maxPrice int64 = -1

	for _, sub := range activeSubs {
		if sub.Plan == nil {
			continue
		}

		// 检查相同套餐
		if sub.PlanId == newPlanId {
			samePlanSub = sub
			break // 相同套餐优先级最高，直接返回
		}

		// 记录最贵套餐
		if sub.Plan.PriceCents > maxPrice {
			maxPrice = sub.Plan.PriceCents
			mostExpensiveSub = sub
		}
	}

	// 情况1: 存在相同套餐
	if samePlanSub != nil {
		result.HasConflict = true
		result.ConflictType = "same_plan"
		result.ExistingPlanId = samePlanSub.PlanId
		result.ExistingPlanName = samePlanSub.Plan.Name
		result.ExistingPlanPrice = samePlanSub.Plan.PriceCents
		result.AvailableOptions = []string{common.RedeemOptionStack, common.RedeemOptionExtend}
		result.RecommendedOption = common.RedeemOptionStack
		return result, nil
	}

	// 情况2: 无相同套餐，使用最贵套餐作为基准
	if mostExpensiveSub == nil {
		// 所有订阅都没有 Plan 信息，返回无冲突
		return result, nil
	}

	result.HasConflict = true
	result.ExistingPlanId = mostExpensiveSub.PlanId
	result.ExistingPlanName = mostExpensiveSub.Plan.Name
	result.ExistingPlanPrice = mostExpensiveSub.Plan.PriceCents

	if newPlan.PriceCents > mostExpensiveSub.Plan.PriceCents {
		// 新套餐比最贵的现有套餐更贵 - 用户可选择共存、折算或替换
		result.ConflictType = "more_expensive"
		result.AvailableOptions = []string{common.RedeemOptionCoexist, common.RedeemOptionConvert, common.RedeemOptionReplace}
		result.RecommendedOption = common.RedeemOptionCoexist

		// 计算折算预览（不含兑换码时长，仅预估折算天数）
		preview, err := s.calculateConversionPreview(mostExpensiveSub, newPlan, 0)
		if err == nil {
			result.ConversionPreview = preview
		}
		return result, nil
	}

	// 情况3: 新套餐不比现有最贵套餐贵 - 属于不同套餐，默认共存
	result.ConflictType = "different_plan"
	return result, nil
}

// PromptUserChoice 返回选择提示给前端
// 用于更贵套餐冲突场景，返回包含完整预览信息（含兑换码时长）
func (s *RedemptionService) PromptUserChoice(userId int64, redemptionKey string) (*ConflictCheckResult, error) {
	// 获取兑换码信息
	redemption, err := model.GetRedemptionByKey(redemptionKey)
	if err != nil {
		return nil, err
	}

	if !redemption.IsSubscriptionRedemption() {
		return nil, errors.New(common.MsgRedemptionInvalidType)
	}

	payload, err := redemption.GetSubscriptionPayload()
	if err != nil {
		return nil, err
	}

	// 调用带兑换码时长的冲突检测
	return s.CheckSubscriptionConflictWithDuration(userId, payload.PlanId, payload.GetDurationSeconds())
}

// CheckSubscriptionConflictWithDuration 检测套餐冲突（带兑换码时长）
// 用于返回包含完整预览信息的冲突结果
func (s *RedemptionService) CheckSubscriptionConflictWithDuration(userId int64, newPlanId int64, redeemDurationSeconds int64) (*ConflictCheckResult, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if newPlanId == 0 {
		return nil, errors.New("套餐 ID 不能为空")
	}

	return s.checkSubscriptionConflictWithDurationTx(model.DB, userId, newPlanId, redeemDurationSeconds)
}

// checkSubscriptionConflictWithDurationTx 在事务中检测套餐冲突（带兑换码时长）
func (s *RedemptionService) checkSubscriptionConflictWithDurationTx(tx *gorm.DB, userId int64, newPlanId int64, redeemDurationSeconds int64) (*ConflictCheckResult, error) {
	result := &ConflictCheckResult{
		HasConflict:      false,
		NewPlanId:        newPlanId,
		AvailableOptions: []string{common.RedeemOptionCoexist},
	}

	// 获取新套餐信息
	newPlan, err := model.GetSubscriptionPlanByIdWithTx(tx, newPlanId)
	if err != nil {
		return nil, err
	}
	result.NewPlanName = newPlan.Name
	result.NewPlanPrice = newPlan.PriceCents

	// 获取用户所有活跃订阅
	activeSubs, err := model.GetActiveSubscriptionsByUserWithTx(tx, userId)
	if err != nil {
		return nil, err
	}

	if len(activeSubs) == 0 {
		// 无现有订阅，无冲突
		return result, nil
	}

	// 修复：多订阅场景下的基准选择逻辑
	// 1. 优先查找相同套餐的订阅（same_plan）
	// 2. 否则选择最贵的套餐作为冲突基准

	var samePlanSub *model.Subscription
	var mostExpensiveSub *model.Subscription
	var maxPrice int64 = -1

	for _, sub := range activeSubs {
		if sub.Plan == nil {
			continue
		}

		// 检查相同套餐
		if sub.PlanId == newPlanId {
			samePlanSub = sub
			break // 相同套餐优先级最高，直接返回
		}

		// 记录最贵套餐
		if sub.Plan.PriceCents > maxPrice {
			maxPrice = sub.Plan.PriceCents
			mostExpensiveSub = sub
		}
	}

	// 情况1: 存在相同套餐
	if samePlanSub != nil {
		result.HasConflict = true
		result.ConflictType = "same_plan"
		result.ExistingPlanId = samePlanSub.PlanId
		result.ExistingPlanName = samePlanSub.Plan.Name
		result.ExistingPlanPrice = samePlanSub.Plan.PriceCents
		result.AvailableOptions = []string{common.RedeemOptionStack, common.RedeemOptionExtend}
		result.RecommendedOption = common.RedeemOptionStack
		return result, nil
	}

	// 情况2: 无相同套餐，使用最贵套餐作为基准
	if mostExpensiveSub == nil {
		// 所有订阅都没有 Plan 信息，返回无冲突
		return result, nil
	}

	result.HasConflict = true
	result.ExistingPlanId = mostExpensiveSub.PlanId
	result.ExistingPlanName = mostExpensiveSub.Plan.Name
	result.ExistingPlanPrice = mostExpensiveSub.Plan.PriceCents

	if newPlan.PriceCents > mostExpensiveSub.Plan.PriceCents {
		// 新套餐比最贵的现有套餐更贵 - 用户可选择共存、折算或替换
		result.ConflictType = "more_expensive"
		result.AvailableOptions = []string{common.RedeemOptionCoexist, common.RedeemOptionConvert, common.RedeemOptionReplace}
		result.RecommendedOption = common.RedeemOptionCoexist

		// 计算折算预览（含兑换码时长）
		preview, err := s.calculateConversionPreview(mostExpensiveSub, newPlan, redeemDurationSeconds)
		if err == nil {
			result.ConversionPreview = preview
		}
		return result, nil
	}

	// 情况3: 新套餐不比现有最贵套餐贵 - 属于不同套餐，默认共存
	result.ConflictType = "different_plan"
	return result, nil
}

// ===================== 2.7.4 折算逻辑（convert 选项） =====================

// CalculateRemainValue 计算订阅剩余价值
// 返回值单位：分
// 公式：剩余价值 = 剩余天数 × 日均价
// 日均价 = plan_price / cycle_days（按套餐计费周期计算，非订阅实际时长）
func (s *RedemptionService) CalculateRemainValue(subscription *model.Subscription, plan *model.SubscriptionPlan) (decimal.Decimal, error) {
	if subscription == nil || plan == nil {
		return decimal.Zero, errors.New("订阅或套餐不能为空")
	}

	// 计算套餐计费周期天数（按设计文档：monthly=30, yearly=365, custom=BillingCycleValue）
	billingCycleDays := s.getPlanBillingCycleDays(plan)
	if billingCycleDays <= 0 {
		return decimal.Zero, errors.New("套餐计费周期无效")
	}

	// 计算日均价（分）= plan_price / cycle_days
	dailyPrice := decimal.NewFromInt(plan.PriceCents).Div(decimal.NewFromInt(billingCycleDays))

	// 计算剩余天数
	now := common.GetTimestamp()
	remainingSeconds := subscription.EndAt - now
	if remainingSeconds <= 0 {
		return decimal.Zero, nil
	}
	remainingDays := decimal.NewFromInt(remainingSeconds).Div(decimal.NewFromInt(24 * 60 * 60))

	// 剩余价值 = 剩余天数 × 日均价
	remainingValue := remainingDays.Mul(dailyPrice)

	return remainingValue, nil
}

// getPlanBillingCycleDays 获取套餐计费周期天数
func (s *RedemptionService) getPlanBillingCycleDays(plan *model.SubscriptionPlan) int64 {
	if plan == nil {
		return 0
	}
	switch plan.BillingCycle {
	case common.BillingCycleMonthly:
		return 30
	case common.BillingCycleYearly:
		return 365
	case common.BillingCycleCustom:
		if plan.BillingCycleValue > 0 {
			return int64(plan.BillingCycleValue)
		}
		return 30 // 默认值
	default:
		return 30 // 默认按月
	}
}

// ConvertToDays 按新套餐日均价折算天数（向下取整）
func (s *RedemptionService) ConvertToDays(remainingValue decimal.Decimal, newPlan *model.SubscriptionPlan) (int, decimal.Decimal, error) {
	if newPlan == nil {
		return 0, decimal.Zero, errors.New("新套餐不能为空")
	}

	// 计算新套餐日均价
	// 根据计费周期计算天数
	var billingDays int64
	switch newPlan.BillingCycle {
	case common.BillingCycleMonthly:
		billingDays = 30
	case common.BillingCycleYearly:
		billingDays = 365
	case common.BillingCycleCustom:
		// 修复：防止除零，custom 周期 BillingCycleValue 必须 > 0
		if newPlan.BillingCycleValue <= 0 {
			return 0, remainingValue, errors.New("自定义计费周期天数必须大于 0")
		}
		billingDays = int64(newPlan.BillingCycleValue)
	default:
		billingDays = 30
	}

	newDailyPrice := decimal.NewFromInt(newPlan.PriceCents).Div(decimal.NewFromInt(billingDays))

	if newDailyPrice.LessThanOrEqual(decimal.Zero) {
		return 0, remainingValue, errors.New("新套餐日均价无效")
	}

	// 折算天数（向下取整）
	convertedDaysDecimal := remainingValue.Div(newDailyPrice).Floor()
	convertedDays := int(convertedDaysDecimal.IntPart())

	// 计算剩余金额（不足一天的部分）
	usedValue := convertedDaysDecimal.Mul(newDailyPrice)
	remainder := remainingValue.Sub(usedValue)

	return convertedDays, remainder, nil
}

// calculateConversionPreview 计算折算预览
// redeemDurationSeconds: 兑换码提供的时长（秒），预览时可传 0 表示只计算折算部分
func (s *RedemptionService) calculateConversionPreview(existingSub *model.Subscription, newPlan *model.SubscriptionPlan, redeemDurationSeconds int64) (*ConversionPreviewResult, error) {
	if existingSub == nil || existingSub.Plan == nil || newPlan == nil {
		return nil, errors.New("参数无效")
	}

	// 计算剩余价值
	remainingValue, err := s.CalculateRemainValue(existingSub, existingSub.Plan)
	if err != nil {
		return nil, err
	}

	// 折算天数
	convertedDays, remainder, err := s.ConvertToDays(remainingValue, newPlan)
	if err != nil {
		return nil, err
	}

	// 计算剩余天数
	now := common.GetTimestamp()
	remainingSeconds := existingSub.EndAt - now
	remainingDays := int(remainingSeconds / (24 * 60 * 60))

	// 修复：保留精确秒数计算，不截断时长
	// 折算天数转换为秒
	convertedSeconds := int64(convertedDays) * 24 * 60 * 60
	// 总秒数 = 折算秒数 + 兑换码时长秒数（精确保留）
	totalSeconds := convertedSeconds + redeemDurationSeconds

	return &ConversionPreviewResult{
		RemainingDays:   remainingDays,
		RemainingValue:  remainingValue,
		ConvertedDays:   convertedDays,
		RemainderAmount: remainder,
		NewEndAt:        now + totalSeconds,
	}, nil
}

// executeConvertRedemption 执行折算兑换
func (s *RedemptionService) executeConvertRedemption(tx *gorm.DB, result *RedemptionResult, userId int64, redemption *model.Redemption, newPlan *model.SubscriptionPlan, payload *model.RedemptionSubscriptionPayload, existingPlanId int64) error {
	// 1. 获取现有订阅
	existingSub, err := s.getUserSubscriptionByPlanWithTx(tx, userId, existingPlanId)
	if err != nil || existingSub == nil {
		return fmt.Errorf("获取现有订阅失败: %w", err)
	}

	// 2. 获取旧套餐信息
	oldPlan, err := model.GetSubscriptionPlanByIdWithTx(tx, existingSub.PlanId)
	if err != nil {
		return fmt.Errorf("获取旧套餐失败: %w", err)
	}

	// 3. 计算剩余价值
	remainingValue, err := s.CalculateRemainValue(existingSub, oldPlan)
	if err != nil {
		return fmt.Errorf("计算剩余价值失败: %w", err)
	}

	// 4. 折算天数
	convertedDays, remainder, err := s.ConvertToDays(remainingValue, newPlan)
	if err != nil {
		return fmt.Errorf("折算天数失败: %w", err)
	}

	// 4.1 业务规则：折算结果不足1天时拒绝（兑换码时长不参与最小单位校验）
	// 原因：折算价值过低时，用户应选择其他兑换方式（如 replace）
	if convertedDays < 1 {
		result.ErrorCode = "CONVERSION_INSUFFICIENT"
		result.ErrorMessage = "折算价值不足1天，无法进行折算兑换，请选择其他兑换方式"
		return errors.New(result.ErrorMessage)
	}

	// 5. 修复：使用精确秒数计算，不截断兑换码时长
	redeemDurationSeconds := payload.GetDurationSeconds()
	// 折算天数转换为秒
	convertedSeconds := int64(convertedDays) * 24 * 60 * 60
	// 总秒数 = 折算秒数 + 兑换码时长秒数（精确保留，不截断）
	totalSeconds := convertedSeconds + redeemDurationSeconds

	// 6. 取消旧订阅
	if err := model.CancelSubscriptionWithTx(tx, existingSub.Id); err != nil {
		return fmt.Errorf("取消旧订阅失败: %w", err)
	}

	// 7. 创建新订阅（使用精确的秒数计算 EndAt）
	now := common.GetTimestamp()
	newEndAt := now + totalSeconds // 使用精确秒数
	redemptionId := int64(redemption.Id)

	newSub := &model.Subscription{
		UserId:             userId,
		PlanId:             newPlan.Id,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now,
		EndAt:              newEndAt,
		CreatedAt:          now,                                                   // 设置 CreatedAt 用于 priority tie-break
		AutoWalletFallback: s.getEffectiveAutoWalletFallback(tx, userId), // 修复：按 "用户设置 > 系统默认" 继承
		RedemptionId:       &redemptionId,
		RedeemOption:       common.RedeemOptionConvert,
	}

	// 从套餐复制模型白名单
	if newPlan.ModelWhitelist != nil {
		newSub.ModelWhitelistCache = newPlan.ModelWhitelist
	}

	// 初始化优先级
	if err := s.priorityService.InitializeSubscriptionPriorityWithTx(tx, newSub); err != nil {
		return fmt.Errorf("初始化优先级失败: %w", err)
	}

	// 创建订阅
	if err := model.CreateSubscriptionWithTx(tx, newSub); err != nil {
		return fmt.Errorf("创建新订阅失败: %w", err)
	}

	// 8. 标记兑换码已使用
	if err := model.RedeemSubscriptionWithTx(tx, redemption, int(userId)); err != nil {
		return fmt.Errorf("标记兑换码失败: %w", err)
	}

	// 9. 记录折算流水（包括不足一天的金额）
	conversionInfo := &ConversionInfo{
		OldSubscriptionId:    existingSub.Id,
		OldPlanId:            oldPlan.Id,
		OldRemainingDays:     existingSub.GetRemainingDays(),
		OldDailyPrice:        decimal.NewFromInt(oldPlan.PriceCents).Div(decimal.NewFromInt(s.getPlanBillingCycleDays(oldPlan))),
		OldRemainingValue:    remainingValue,
		NewPlanId:            newPlan.Id,
		NewDailyPrice:        decimal.NewFromInt(newPlan.PriceCents).Div(decimal.NewFromInt(s.getPlanBillingCycleDays(newPlan))),
		ConvertedDays:        convertedDays,
		RemainderAmount:      remainder,
		NewSubscriptionEndAt: newEndAt,
	}

	// 记录折算账单
	if err := s.recordConversionBillWithTx(tx, userId, redemption, newPlan, payload, conversionInfo); err != nil {
		return fmt.Errorf("记录折算账单失败: %w", err)
	}

	// 10. 记录审计日志
	conversionMeta, _ := json.Marshal(conversionInfo)
	if err := s.logRedemptionAudit(tx, userId, redemption.Id, redemption.Key, newPlan.Id, newSub.Id, "convert", common.RedeemOptionConvert, conversionMeta); err != nil {
		return fmt.Errorf("记录审计日志失败: %w", err)
	}

	// 注意：缓存刷新移到事务提交后执行，避免读取到未提交的数据

	// 设置结果
	result.SubscriptionId = newSub.Id
	result.ConversionInfo = conversionInfo
	result.NewEndAt = newEndAt

	return nil
}

// ===================== 2.7.5 并存逻辑（coexist 选项） =====================

// executeCoexistRedemption 执行并存兑换
// 创建新订阅，保留旧订阅
func (s *RedemptionService) executeCoexistRedemption(tx *gorm.DB, result *RedemptionResult, userId int64, redemption *model.Redemption, plan *model.SubscriptionPlan, payload *model.RedemptionSubscriptionPayload) error {
	// 1. 修复：先锁定用户行，防止并发请求同时通过检查（解决 0 行无锁问题）
	// 用户行一定存在，锁定它确保同一用户的并发兑换请求串行化
	if err := model.LockUserRowForUpdateWithTx(tx, userId); err != nil {
		return fmt.Errorf("锁定用户失败: %w", err)
	}

	// 2. 检查用户订阅数量限制
	activeCount, err := model.CountActiveSubscriptionsByUserWithTxForUpdate(tx, userId)
	if err != nil {
		return fmt.Errorf("检查订阅数量失败: %w", err)
	}

	maxPerUser := int64(common.GetSubscriptionMaxPerUser())
	if activeCount >= maxPerUser {
		return fmt.Errorf("订阅数量已达上限（%d）", maxPerUser)
	}

	// 3. 计算订阅时间
	now := common.GetTimestamp()
	durationSeconds := payload.GetDurationSeconds()
	endAt := now + durationSeconds

	// 3. 创建新订阅
	redemptionId := int64(redemption.Id)
	newSub := &model.Subscription{
		UserId:             userId,
		PlanId:             plan.Id,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now,
		EndAt:              endAt,
		CreatedAt:          now,                                                // 设置 CreatedAt 用于 priority tie-break
		AutoWalletFallback: s.getEffectiveAutoWalletFallback(tx, userId), // 修复：按 "用户设置 > 系统默认" 继承
		RedemptionId:       &redemptionId,
		RedeemOption:       common.RedeemOptionCoexist,
	}

	// 从套餐复制模型白名单
	if plan.ModelWhitelist != nil {
		newSub.ModelWhitelistCache = plan.ModelWhitelist
	}

	// 初始化优先级（按 end_at 动态插入）
	if err := s.priorityService.InitializeSubscriptionPriorityWithTx(tx, newSub); err != nil {
		return fmt.Errorf("初始化优先级失败: %w", err)
	}

	// 创建订阅
	if err := model.CreateSubscriptionWithTx(tx, newSub); err != nil {
		return fmt.Errorf("创建订阅失败: %w", err)
	}

	// 4. 标记兑换码已使用
	if err := model.RedeemSubscriptionWithTx(tx, redemption, int(userId)); err != nil {
		return fmt.Errorf("标记兑换码失败: %w", err)
	}

	// 5. 记录账单流水
	extendedDays := int(durationSeconds / (24 * 60 * 60))
	description := fmt.Sprintf("兑换码创建订阅 %d 天，套餐：%s", extendedDays, plan.Name)
	if err := s.recordRedemptionBillWithTx(tx, userId, redemption, plan, payload, description); err != nil {
		return fmt.Errorf("记录账单失败: %w", err)
	}

	// 6. 记录审计日志
	if err := s.logRedemptionAudit(tx, userId, redemption.Id, redemption.Key, plan.Id, newSub.Id, "coexist", common.RedeemOptionCoexist, nil); err != nil {
		return fmt.Errorf("记录审计日志失败: %w", err)
	}

	// 注意：缓存刷新移到事务提交后执行，避免读取到未提交的数据

	// 设置结果
	result.SubscriptionId = newSub.Id
	result.NewSubscription = newSub
	result.ExtendedDays = extendedDays
	result.NewEndAt = endAt

	return nil
}

// executeReplaceRedemption 执行替换兑换
// 取消旧订阅，创建新订阅（不做价值折算）
func (s *RedemptionService) executeReplaceRedemption(tx *gorm.DB, result *RedemptionResult, userId int64, redemption *model.Redemption, plan *model.SubscriptionPlan, payload *model.RedemptionSubscriptionPayload, existingPlanId int64) error {
	// 1. 获取并取消旧订阅
	existingSub, err := s.getUserSubscriptionByPlanWithTx(tx, userId, existingPlanId)
	if err != nil {
		return fmt.Errorf("获取现有订阅失败: %w", err)
	}
	if existingSub != nil {
		if err := model.CancelSubscriptionWithTx(tx, existingSub.Id); err != nil {
			return fmt.Errorf("取消旧订阅失败: %w", err)
		}
	}

	// 2. 计算订阅时间
	now := common.GetTimestamp()
	durationSeconds := payload.GetDurationSeconds()
	endAt := now + durationSeconds

	// 3. 创建新订阅
	redemptionId := int64(redemption.Id)
	newSub := &model.Subscription{
		UserId:             userId,
		PlanId:             plan.Id,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now,
		EndAt:              endAt,
		CreatedAt:          now,
		AutoWalletFallback: s.getEffectiveAutoWalletFallback(tx, userId), // 修复：按 "用户设置 > 系统默认" 继承
		RedemptionId:       &redemptionId,
		RedeemOption:       common.RedeemOptionReplace,
	}

	// 从套餐复制模型白名单
	if plan.ModelWhitelist != nil {
		newSub.ModelWhitelistCache = plan.ModelWhitelist
	}

	// 初始化优先级
	if err := s.priorityService.InitializeSubscriptionPriorityWithTx(tx, newSub); err != nil {
		return fmt.Errorf("初始化优先级失败: %w", err)
	}

	// 创建订阅
	if err := model.CreateSubscriptionWithTx(tx, newSub); err != nil {
		return fmt.Errorf("创建订阅失败: %w", err)
	}

	// 4. 标记兑换码已使用
	if err := model.RedeemSubscriptionWithTx(tx, redemption, int(userId)); err != nil {
		return fmt.Errorf("标记兑换码失败: %w", err)
	}

	// 5. 记录账单流水
	extendedDays := int(durationSeconds / (24 * 60 * 60))
	description := fmt.Sprintf("兑换码替换订阅 %d 天，套餐：%s（旧订阅已取消）", extendedDays, plan.Name)
	if err := s.recordRedemptionBillWithTx(tx, userId, redemption, plan, payload, description); err != nil {
		return fmt.Errorf("记录账单失败: %w", err)
	}

	// 6. 记录审计日志
	if err := s.logRedemptionAudit(tx, userId, redemption.Id, redemption.Key, plan.Id, newSub.Id, "replace", common.RedeemOptionReplace, nil); err != nil {
		return fmt.Errorf("记录审计日志失败: %w", err)
	}

	// 设置结果
	result.SubscriptionId = newSub.Id
	result.NewSubscription = newSub
	result.ExtendedDays = extendedDays
	result.NewEndAt = endAt

	return nil
}

// ===================== 2.7.6 兑换时优惠券自动消费 =====================

// RedeemWithCoupon 兑换 + 消费绑定的优惠券
func (s *RedemptionService) RedeemWithCoupon(userId int, key string, redeemOptionOverride string) (*RedemptionResult, error) {
	result := &RedemptionResult{
		Success: false,
	}

	// 1. 先获取兑换码信息，检查是否有绑定的优惠券
	redemption, err := model.GetRedemptionByKey(key)
	if err != nil {
		result.ErrorCode = "REDEMPTION_NOT_FOUND"
		result.ErrorMessage = common.MsgRedemptionNotFound
		return result, err
	}

	// 2. 检查是否有绑定的优惠券（修复：不忽略查询错误）
	couponBinding, err := model.GetCouponRedemptionBindingByRedemption(int64(redemption.Id))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		// 查询绑定关系出错（非"未找到"错误），返回错误
		result.ErrorCode = "COUPON_BINDING_QUERY_FAILED"
		result.ErrorMessage = "查询优惠券绑定信息失败"
		return result, err
	}

	// 3. 如果有绑定的优惠券，使用带优惠券的兑换流程
	if couponBinding != nil {
		return s.redeemWithBoundCoupon(userId, key, redeemOptionOverride, couponBinding)
	}

	// 4. 没有绑定优惠券，使用普通兑换流程
	return s.RedeemSubscription(userId, key, redeemOptionOverride)
}

// redeemWithBoundCoupon 带绑定优惠券的兑换
// 修复：先检测冲突，再消费券；券消费在同一事务内
func (s *RedemptionService) redeemWithBoundCoupon(userId int, key string, redeemOptionOverride string, binding *model.CouponRedemptionBinding) (*RedemptionResult, error) {
	result := &RedemptionResult{
		Success: false,
	}

	// 验证用户 ID
	if userId == 0 {
		result.ErrorCode = "INVALID_USER_ID"
		result.ErrorMessage = "用户 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}

	// 使用事务执行整个流程
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 获取并锁定兑换码
		redemption, err := model.GetRedemptionByKeyWithTx(tx, key, true)
		if err != nil {
			result.ErrorCode = "REDEMPTION_NOT_FOUND"
			result.ErrorMessage = common.MsgRedemptionNotFound
			return err
		}

		// 2. 验证兑换码
		if !redemption.IsSubscriptionRedemption() {
			result.ErrorCode = "REDEMPTION_INVALID_TYPE"
			result.ErrorMessage = common.MsgRedemptionInvalidType
			return errors.New(result.ErrorMessage)
		}

		// 修复 2.16.3：使用带幂等性检查的验证，已成功兑换的重复请求返回原结果
		isIdempotent, validationErr := s.validateRedemptionWithIdempotency(redemption, userId, result)
		if isIdempotent {
			// 幂等场景：兑换码已被当前用户使用，直接返回成功结果（不再消费券）
			return nil
		}
		if validationErr != nil {
			return validationErr
		}

		if err := redemption.ValidateBoundUser(userId); err != nil {
			result.ErrorCode = "REDEMPTION_USER_MISMATCH"
			result.ErrorMessage = err.Error()
			return err
		}

		// 3. 解析 payload
		payload, err := redemption.GetSubscriptionPayload()
		if err != nil {
			result.ErrorCode = "REDEMPTION_PAYLOAD_INVALID"
			result.ErrorMessage = "兑换码数据无效"
			return err
		}

		// 4. 获取套餐
		plan, err := model.GetSubscriptionPlanByIdWithTx(tx, payload.PlanId)
		if err != nil {
			result.ErrorCode = "PLAN_NOT_FOUND"
			result.ErrorMessage = common.MsgPlanNotFound
			return err
		}

		// 5. 确定兑换选项
		redeemOption := payload.RedeemOption
		if redeemOptionOverride != "" {
			redeemOption = redeemOptionOverride
		}
		// 修复：不在此处设置 result.RedeemOption，而是在各分支中设置为实际执行的选项

		// 6. 先检测冲突（在消费券之前！）
		conflictResult, err := s.checkSubscriptionConflictWithTx(tx, int64(userId), payload.PlanId)
		if err != nil {
			return err
		}

		// 7. 如果有冲突且需要用户选择，直接返回（不消费券）
		if conflictResult.ConflictType == "more_expensive" &&
			redeemOption != common.RedeemOptionCoexist &&
			redeemOption != common.RedeemOptionConvert &&
			redeemOption != common.RedeemOptionReplace {
			result.ErrorCode = "REDEMPTION_CONFLICT"
			result.ErrorMessage = "检测到套餐冲突，请选择处理方式：coexist（并存）、convert（折算）或 replace（替换）"
			return errors.New(result.ErrorMessage)
		}

		// 8. 冲突检测通过后，消费绑定的优惠券（在同一事务内）
		// 修复：使用 UUID 生成唯一的 virtualOrderId，避免时间戳碰撞风险
		virtualOrderId := common.GenerateUniqueOrderId()
		// 修复：核销金额使用套餐的实际价格（plan.PriceCents），而非 payload.PricePaid
		// payload.PricePaid 是用户实际支付的金额，可能为 0（免费兑换码），会导致优惠券计算异常
		couponResult, err := s.couponService.UseBoundCouponWithPlanTx(
			tx,
			int64(userId),
			int64(redemption.Id),
			virtualOrderId,
			plan.PriceCents, // 使用套餐价格而非 payload.PricePaid
			"subscription",
			&plan.Id,
		)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			result.ErrorCode = "COUPON_USE_FAILED"
			result.ErrorMessage = err.Error()
			return err
		}

		if couponResult != nil && couponResult.CouponUsed {
			result.CouponUsed = true
			result.DiscountAmount = couponResult.DiscountAmount
		}

		// 9. 根据冲突类型执行相应逻辑，并更新 result.RedeemOption 为实际执行的选项
		switch conflictResult.ConflictType {
		case "same_plan":
			// 同套餐：支持 stack 和 extend（行为相同）
			switch redeemOption {
			case common.RedeemOptionStack, common.RedeemOptionExtend, "":
				// 更新为实际执行的选项
				if redeemOption == "" {
					result.RedeemOption = common.RedeemOptionStack
				} else {
					result.RedeemOption = redeemOption
				}
				return s.executeStackRedemption(tx, result, int64(userId), redemption, plan, payload)
			default:
				// 不支持的选项，默认执行 stack
				result.RedeemOption = common.RedeemOptionStack
				return s.executeStackRedemption(tx, result, int64(userId), redemption, plan, payload)
			}
		case "more_expensive":
			switch redeemOption {
			case common.RedeemOptionCoexist:
				result.RedeemOption = common.RedeemOptionCoexist
				return s.executeCoexistRedemption(tx, result, int64(userId), redemption, plan, payload)
			case common.RedeemOptionConvert:
				result.RedeemOption = common.RedeemOptionConvert
				return s.executeConvertRedemption(tx, result, int64(userId), redemption, plan, payload, conflictResult.ExistingPlanId)
			case common.RedeemOptionReplace:
				result.RedeemOption = common.RedeemOptionReplace
				return s.executeReplaceRedemption(tx, result, int64(userId), redemption, plan, payload, conflictResult.ExistingPlanId)
			default:
				result.ErrorCode = "REDEMPTION_CONFLICT"
				result.ErrorMessage = "检测到套餐冲突，请选择处理方式：coexist（并存）、convert（折算）或 replace（替换）"
				return errors.New(result.ErrorMessage)
			}
		case "different_plan":
			switch redeemOption {
			case common.RedeemOptionReplace:
				result.RedeemOption = common.RedeemOptionReplace
				return s.executeReplaceRedemption(tx, result, int64(userId), redemption, plan, payload, conflictResult.ExistingPlanId)
			default:
				// 默认共存
				result.RedeemOption = common.RedeemOptionCoexist
				return s.executeCoexistRedemption(tx, result, int64(userId), redemption, plan, payload)
			}
		default:
			// 无冲突：直接创建新订阅
			result.RedeemOption = common.RedeemOptionCoexist
			return s.executeCoexistRedemption(tx, result, int64(userId), redemption, plan, payload)
		}
	})

	if err != nil {
		if result.ErrorCode == "" {
			result.ErrorCode = "REDEMPTION_FAILED"
		}
		if result.ErrorMessage == "" {
			result.ErrorMessage = err.Error()
		}
		return result, err
	}

	// 事务提交成功后刷新缓存并预热
	s.priorityService.InvalidateUserSubscriptionCache(int64(userId))
	cacheSvc := GetSubscriptionCacheService()
	go func() {
		_ = cacheSvc.WarmupCache(int64(userId))
	}()

	result.Success = true
	return result, nil
}

// ===================== 辅助方法 =====================

// getUserSubscriptionByPlanWithTx 在事务中获取用户指定套餐的订阅
func (s *RedemptionService) getUserSubscriptionByPlanWithTx(tx *gorm.DB, userId int64, planId int64) (*model.Subscription, error) {
	activeSubs, err := model.GetActiveSubscriptionsByUserWithTx(tx, userId)
	if err != nil {
		return nil, err
	}

	for _, sub := range activeSubs {
		if sub.PlanId == planId {
			return sub, nil
		}
	}

	return nil, nil
}

// getEffectiveAutoWalletFallback 计算有效的 AutoWalletFallback 值
// 逻辑：按 "用户设置 > 系统默认" 继承
func (s *RedemptionService) getEffectiveAutoWalletFallback(tx *gorm.DB, userId int64) bool {
	// 获取用户
	user, err := model.GetUserByIdWithTx(tx, int(userId), false)
	if err != nil || user == nil {
		// 用户不存在时使用系统默认
		return common.GetAutoWalletFallbackDefault()
	}

	// 按 "用户设置 > 系统默认" 继承
	systemDefault := common.GetAutoWalletFallbackDefault()
	effectiveValue, _ := user.GetEffectiveAutoWalletFallback(systemDefault)
	return effectiveValue
}

// recordRedemptionBillWithTx 记录兑换账单
// 修复：OriginalAmount 使用套餐原价（plan.PriceCents），而非 payload.PricePaid（可能为 0）
// 语义：OriginalAmount = 市场价值（套餐价格），FinalAmount = 实付金额（兑换码为 0）
func (s *RedemptionService) recordRedemptionBillWithTx(tx *gorm.DB, userId int64, redemption *model.Redemption, plan *model.SubscriptionPlan, payload *model.RedemptionSubscriptionPayload, description string) error {
	sourceType := common.BillSourceTypeRedemption
	sourceId := int64(redemption.Id)
	paymentChannel := common.PaymentChannelRedemption

	bill := &model.UserBill{
		UserId:         userId,
		BillType:       common.BillTypeSubscription,
		Amount:         0, // 兑换码不扣款
		BalanceBefore:  0,
		BalanceAfter:   0,
		SourceType:     &sourceType,
		SourceId:       &sourceId,
		PaymentChannel: &paymentChannel,
		Description:    &description,
		OriginalAmount: plan.PriceCents, // 修复：使用套餐原价表示市场价值
		FinalAmount:    0,               // 兑换码免费
		CreatedAt:      common.GetTimestamp(),
	}

	return model.CreateUserBillWithTx(tx, bill)
}

// recordConversionBillWithTx 记录折算账单（包含剩余金额）
// 修复：OriginalAmount 使用套餐原价；剩余金额使用 Round() 而非 IntPart() 避免截断丢失
func (s *RedemptionService) recordConversionBillWithTx(tx *gorm.DB, userId int64, redemption *model.Redemption, plan *model.SubscriptionPlan, payload *model.RedemptionSubscriptionPayload, conversionInfo *ConversionInfo) error {
	sourceType := common.BillSourceTypeRedemption
	sourceId := int64(redemption.Id)
	paymentChannel := common.PaymentChannelRedemption

	// 构建折算元数据
	conversionMetadata, _ := json.Marshal(conversionInfo)
	conversionMetadataStr := string(conversionMetadata)

	description := fmt.Sprintf("套餐折算兑换，旧套餐剩余 %d 天折算为新套餐 %d 天，套餐：%s",
		conversionInfo.OldRemainingDays, conversionInfo.ConvertedDays, plan.Name)

	bill := &model.UserBill{
		UserId:             userId,
		BillType:           common.BillTypeSubscription,
		Amount:             0,
		BalanceBefore:      0,
		BalanceAfter:       0,
		SourceType:         &sourceType,
		SourceId:           &sourceId,
		PaymentChannel:     &paymentChannel,
		Description:        &description,
		OriginalAmount:     plan.PriceCents, // 修复：使用套餐原价表示市场价值
		FinalAmount:        0,
		ConversionMetadata: &conversionMetadataStr,
		CreatedAt:          common.GetTimestamp(),
	}

	// 如果有剩余金额，单独记录
	// 修复：使用 Round() 四舍五入而非 IntPart() 截断，避免丢失精度
	if conversionInfo.RemainderAmount.GreaterThan(decimal.Zero) {
		// 四舍五入到整数分
		roundedRemainder := conversionInfo.RemainderAmount.Round(0).IntPart()
		// 只有舍入后金额 > 0 才记录账单（避免 0 分账单）
		if roundedRemainder > 0 {
			remainderDescription := fmt.Sprintf("套餐折算剩余金额（不足一天），金额：%d 分（原值：%s 分）",
				roundedRemainder, conversionInfo.RemainderAmount.StringFixed(2))
			remainderBill := &model.UserBill{
				UserId:             userId,
				BillType:           common.BillTypeAdjustment,
				Amount:             roundedRemainder,
				BalanceBefore:      0,
				BalanceAfter:       0,
				SourceType:         &sourceType,
				SourceId:           &sourceId,
				Description:        &remainderDescription,
				ConversionMetadata: &conversionMetadataStr,
				CreatedAt:          common.GetTimestamp(),
			}
			if err := model.CreateUserBillWithTx(tx, remainderBill); err != nil {
				return err
			}
		}
	}

	return model.CreateUserBillWithTx(tx, bill)
}

// logRedemptionAudit 记录兑换审计日志
// 修复：补充 redemption_code/plan_id/redeem_option 审计字段
func (s *RedemptionService) logRedemptionAudit(tx *gorm.DB, userId int64, redemptionId int, redemptionCode string, planId int64, subscriptionId int64, action string, redeemOption string, extraMetadata []byte) error {
	metadata := map[string]interface{}{
		"redemption_id":   redemptionId,
		"redemption_code": redemptionCode, // 修复：补充兑换码
		"plan_id":         planId,         // 修复：补充套餐ID
		"subscription_id": subscriptionId,
		"action":          action,
		"redeem_option":   redeemOption, // 修复：补充兑换选项
	}

	if extraMetadata != nil {
		var extra map[string]interface{}
		if err := json.Unmarshal(extraMetadata, &extra); err == nil {
			metadata["extra"] = extra
		}
	}

	metadataJSON, _ := json.Marshal(metadata)

	objectId := subscriptionId
	auditLog := &model.AuditLog{
		UserId:     &userId,
		OperatorId: &userId,
		ObjectType: "subscription_redemption",
		ObjectId:   &objectId,
		Action:     "redeem_" + action,
		Metadata:   string(metadataJSON),
		CreatedAt:  common.GetTimestamp(),
	}

	return model.CreateAuditLogWithTx(tx, auditLog)
}

// extractRedeemOptionFromAuditLog 从审计日志 metadata 中提取 redeem_option
// 用于 stack/extend 场景的幂等性返回
func (s *RedemptionService) extractRedeemOptionFromAuditLog(auditLog *model.AuditLog) string {
	if auditLog == nil || auditLog.Metadata == "" {
		return common.RedeemOptionStack // 默认返回 stack
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(auditLog.Metadata), &metadata); err != nil {
		return common.RedeemOptionStack
	}

	if option, ok := metadata["redeem_option"].(string); ok && option != "" {
		return option
	}

	return common.RedeemOptionStack
}

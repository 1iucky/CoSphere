package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

// ===================== Service 层 - 优惠券领取服务 =====================

// CouponService 优惠券服务
type CouponService struct {
	// 可以添加依赖项，如缓存、日志等
}

var (
	couponServiceInstance *CouponService
	couponServiceOnce     sync.Once
)

// GetCouponService 获取优惠券服务单例
func GetCouponService() *CouponService {
	couponServiceOnce.Do(func() {
		couponServiceInstance = &CouponService{}
	})
	return couponServiceInstance
}

// ===================== 优惠券校验结果 =====================

// CouponValidationResult 优惠券校验结果
type CouponValidationResult struct {
	Valid              bool   // 是否可领取
	ErrorCode          string // 错误码（如果不可领取）
	ErrorMessage       string // 错误消息（如果不可领取）
	RemainingCount     int64  // 剩余库存
	UserAlreadyClaimed bool   // 用户是否已领取
}

// UserClaimLimitResult 用户领取限制检查结果
type UserClaimLimitResult struct {
	CanClaim            bool   // 是否可领取
	ErrorCode           string // 错误码（如果不可领取）
	ErrorMessage        string // 错误消息（如果不可领取）
	AlreadyClaimedCount int64  // 已领取次数
	PerUserLimit        int64  // 每用户限制
	UserAlreadyClaimed  bool   // 用户是否已领取
}

// ===================== 优惠券领取服务方法 =====================

// ClaimCoupon 领取优惠券（Service 层封装）
// 提供业务逻辑封装、验证、审计日志等功能
func (s *CouponService) ClaimCoupon(userId int64, claimCode string) (*model.UserCoupon, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if claimCode == "" {
		return nil, errors.New("领取码不能为空")
	}

	// 0. 领券频率限制：10次/分钟（滑动窗口）
	// 防止恶意用户高频调用领券接口
	if common.RDB != nil {
		ctx := context.Background()
		rateLimitKey := fmt.Sprintf("coupon:claim:rate:%d", userId)
		rl := limiter.New(ctx, common.RDB)
		// Capacity=10（每分钟最多10次），Rate=10（每分钟恢复10个令牌）
		allowed, err := rl.Allow(ctx, rateLimitKey,
			limiter.WithCapacity(10),
			limiter.WithRate(10),
			limiter.WithRequested(1),
		)
		if err != nil {
			// 限流器异常时记录日志但不阻断业务（降级策略）
			common.SysLog(fmt.Sprintf("领券限流检查失败: userId=%d, err=%v", userId, err))
		} else if !allowed {
			return nil, errors.New("领券过于频繁，请稍后再试（限制：10次/分钟）")
		}
	}

	// 1. 前置校验：检查优惠券是否可领取
	validation := s.ValidateCouponClaimable(claimCode)
	if !validation.Valid {
		return nil, errors.New(validation.ErrorMessage)
	}

	// 2. 检查用户领取限制
	limitCheck := s.CheckUserClaimLimit(userId, claimCode)
	if !limitCheck.CanClaim {
		return nil, errors.New(limitCheck.ErrorMessage)
	}

	// 3. 调用 Model 层领取优惠券（包含乐观锁和事务）
	userCoupon, err := model.ClaimCoupon(userId, claimCode)
	if err != nil {
		// 映射 Model 层错误到 Service 层错误
		return nil, s.mapModelError(err)
	}

	// 4. 写入审计日志（异步）
	go s.logCouponClaim(userId, claimCode, userCoupon.Id, true, "")

	return userCoupon, nil
}

// ValidateCouponClaimable 校验优惠券是否可领取
// 检查优惠券存在性、状态、时间、库存等
func (s *CouponService) ValidateCouponClaimable(claimCode string) *CouponValidationResult {
	result := &CouponValidationResult{
		Valid: false,
	}

	if claimCode == "" {
		result.ErrorCode = "INVALID_CLAIM_CODE"
		result.ErrorMessage = "领取码不能为空"
		return result
	}

	// 1. 查询优惠券
	coupon, err := model.GetCouponByCode(claimCode)
	if err != nil {
		result.ErrorCode = "COUPON_NOT_FOUND"
		result.ErrorMessage = common.MsgCouponNotFound
		return result
	}

	// 2. 检查优惠券状态
	if coupon.Status != common.CouponStatusActive {
		result.ErrorCode = "COUPON_INVALID_STATUS"
		result.ErrorMessage = common.MsgCouponInvalidStatus
		return result
	}

	// 2.1 修复 2.16：检查优惠券是否绑定到兑换码（绑定券不可通过普通领取流程使用）
	// 绑定关系存储在 coupon_redemption_bindings 表
	isBound, err := model.IsCouponBoundToRedemption(coupon.Id)
	if err != nil {
		result.ErrorCode = "COUPON_BINDING_CHECK_FAILED"
		result.ErrorMessage = "检查优惠券绑定状态失败"
		return result
	}
	if isBound {
		result.ErrorCode = "COUPON_BOUND_TO_REDEMPTION"
		result.ErrorMessage = "该优惠券仅限随兑换码一起使用"
		return result
	}

	// 3. 检查时间有效性
	now := common.GetTimestamp()
	if now < coupon.ValidFrom {
		result.ErrorCode = "COUPON_NOT_YET_VALID"
		result.ErrorMessage = "优惠券尚未生效"
		return result
	}
	if now > coupon.ValidTo {
		result.ErrorCode = "COUPON_EXPIRED"
		result.ErrorMessage = common.MsgCouponExpired
		return result
	}

	// 4. 检查库存
	remainingCount := coupon.GetRemainingCount()
	result.RemainingCount = remainingCount
	if remainingCount == 0 {
		result.ErrorCode = "COUPON_EXHAUSTED"
		result.ErrorMessage = common.MsgCouponExhausted
		return result
	}

	// 所有检查通过
	result.Valid = true
	return result
}

// CheckUserClaimLimit 检查用户领取限制
// 检查用户是否已领取、是否超过每用户限制
func (s *CouponService) CheckUserClaimLimit(userId int64, claimCode string) *UserClaimLimitResult {
	result := &UserClaimLimitResult{
		CanClaim: false,
	}

	if userId == 0 {
		result.ErrorCode = "INVALID_USER_ID"
		result.ErrorMessage = "用户 ID 不能为空"
		return result
	}
	if claimCode == "" {
		result.ErrorCode = "INVALID_CLAIM_CODE"
		result.ErrorMessage = "领取码不能为空"
		return result
	}

	// 1. 查询优惠券
	coupon, err := model.GetCouponByCode(claimCode)
	if err != nil {
		result.ErrorCode = "COUPON_NOT_FOUND"
		result.ErrorMessage = common.MsgCouponNotFound
		return result
	}

	result.PerUserLimit = coupon.PerUserLimit

	// 2. 检查用户是否已领取该优惠券
	userCoupon, err := model.GetUserCouponByUserAndCoupon(userId, coupon.Id)
	if err != nil {
		result.ErrorCode = "DATABASE_ERROR"
		result.ErrorMessage = "查询用户优惠券失败"
		return result
	}

	// 如果用户已领取，则不能再次领取
	if userCoupon != nil {
		result.ErrorCode = "COUPON_ALREADY_CLAIMED"
		result.ErrorMessage = "您已领取过该优惠券"
		result.AlreadyClaimedCount = 1
		result.UserAlreadyClaimed = true
		return result
	}

	// 3. 所有检查通过，用户可以领取
	result.CanClaim = true
	result.AlreadyClaimedCount = 0
	return result
}

// ===================== 辅助方法 =====================

// mapModelError 将 Model 层错误映射为 Service 层错误
func (s *CouponService) mapModelError(err error) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()

	// 映射常见错误
	switch errMsg {
	case common.MsgCouponNotFound:
		return errors.New(common.MsgCouponNotFound)
	case common.MsgCouponExpired:
		return errors.New(common.MsgCouponExpired)
	case "优惠券库存不足":
		return errors.New(common.MsgCouponInsufficientQuota)
	case "您已领取过该优惠券":
		return errors.New("您已领取过该优惠券，无法重复领取")
	case "优惠券尚未生效":
		return errors.New("优惠券尚未生效，请稍后再试")
	default:
		// 保留原始错误消息
		return err
	}
}

// couponClaimLogData 优惠券领取审计日志数据结构
type couponClaimLogData struct {
	ClaimCode    string `json:"claim_code"`
	UserCouponId int64  `json:"user_coupon_id"`
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}

// logCouponClaim 记录优惠券领取审计日志
func (s *CouponService) logCouponClaim(userId int64, claimCode string, userCouponId int64, success bool, errorMsg string) {
	// 使用结构体安全构建 JSON，避免注入风险
	logData := couponClaimLogData{
		ClaimCode:    claimCode,
		UserCouponId: userCouponId,
		Success:      success,
	}
	if !success && errorMsg != "" {
		logData.Error = errorMsg
	}

	// 安全序列化 JSON
	metadataBytes, err := json.Marshal(logData)
	if err != nil {
		// JSON 序列化失败时使用空对象
		metadataBytes = []byte("{}")
	}

	// 写入审计日志（使用现有审计日志系统）
	auditLog := &model.AuditLog{
		OperatorId: &userId, // 领取时操作者即用户本人
		ObjectType: "user_coupon",
		ObjectId:   &userCouponId,
		Action:     "claim_coupon",
		Metadata:   string(metadataBytes),
		CreatedAt:  common.GetTimestamp(),
	}

	// 异步写入日志，忽略错误（不影响主流程）
	_ = model.CreateAuditLog(auditLog)
}

// ===================== 2.6.2 优惠券核销服务方法 =====================

// CouponUsageResult 优惠券核销结果（Service 层）
type CouponUsageResult struct {
	UserCouponId   int64  `json:"user_coupon_id"`  // 用户优惠券 ID
	CouponId       int64  `json:"coupon_id"`       // 优惠券模板 ID
	OriginalAmount int64  `json:"original_amount"` // 原价（分）
	DiscountAmount int64  `json:"discount_amount"` // 优惠金额（分）
	FinalAmount    int64  `json:"final_amount"`    // 最终应付金额（分）
	CouponType     string `json:"coupon_type"`     // 优惠券类型
	IsIdempotent   bool   `json:"is_idempotent"`   // 是否为幂等返回
}

// DiscountPreviewResult 优惠金额预览结果
type DiscountPreviewResult struct {
	Valid          bool   `json:"valid"`           // 是否可用
	ErrorCode      string `json:"error_code"`      // 错误码
	ErrorMessage   string `json:"error_message"`   // 错误消息
	OriginalAmount int64  `json:"original_amount"` // 原价（分）
	DiscountAmount int64  `json:"discount_amount"` // 优惠金额（分）
	FinalAmount    int64  `json:"final_amount"`    // 最终应付金额（分）
	CouponType     string `json:"coupon_type"`     // 优惠券类型
	ThresholdMet   bool   `json:"threshold_met"`   // 是否满足阈值
}

// UseCoupon 核销优惠券（Service 层封装）
// 包含幂等校验、行锁、折扣计算、审计日志
func (s *CouponService) UseCoupon(userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string) (*CouponUsageResult, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if userCouponId == 0 {
		return nil, errors.New("用户优惠券 ID 不能为空")
	}
	if orderId == 0 {
		return nil, errors.New("订单 ID 不能为空")
	}
	if orderAmount <= 0 {
		return nil, errors.New("订单金额必须大于 0")
	}
	if scene == "" {
		return nil, errors.New("使用场景不能为空")
	}

	// 调用 Model 层核销（包含幂等校验、行锁、折扣计算）
	modelResult, err := model.UseCoupon(userId, userCouponId, orderId, orderAmount, scene)
	if err != nil {
		// 记录失败审计日志
		go s.logCouponUsage(userId, userCouponId, orderId, 0, 0, false, err.Error())
		return nil, s.mapModelError(err)
	}

	// 获取用户优惠券详情以获取 couponId
	userCoupon, _ := model.GetUserCouponById(userCouponId)
	var couponId int64
	var couponType string
	if userCoupon != nil {
		couponId = userCoupon.CouponId
		// 获取优惠券模板类型
		if coupon, err := model.GetCouponById(couponId); err == nil {
			couponType = coupon.Type
		}
	}

	result := &CouponUsageResult{
		UserCouponId:   userCouponId,
		CouponId:       couponId,
		OriginalAmount: orderAmount,
		DiscountAmount: modelResult.DiscountAmount,
		FinalAmount:    modelResult.FinalAmount,
		CouponType:     couponType,
		IsIdempotent:   modelResult.IsIdempotent, // 从 Model 层传递幂等标记
	}

	// 记录成功审计日志
	go s.logCouponUsage(userId, userCouponId, orderId, modelResult.DiscountAmount, modelResult.FinalAmount, true, "")

	return result, nil
}

// UseCouponWithTx 在事务中核销优惠券（Service 层封装）
// 与 UseCoupon 相同，但使用外部传入的事务，保证事务原子性
func (s *CouponService) UseCouponWithTx(tx *gorm.DB, userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string) (*CouponUsageResult, error) {
	if tx == nil {
		return nil, errors.New("事务不能为空")
	}
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if userCouponId == 0 {
		return nil, errors.New("用户优惠券 ID 不能为空")
	}
	if orderId == 0 {
		return nil, errors.New("订单 ID 不能为空")
	}
	if orderAmount <= 0 {
		return nil, errors.New("订单金额必须大于 0")
	}
	if scene == "" {
		return nil, errors.New("使用场景不能为空")
	}

	// 调用 Model 层核销（使用传入的事务，保证原子性）
	modelResult, err := model.UseCouponWithTx(tx, userId, userCouponId, orderId, orderAmount, scene)
	if err != nil {
		// 记录失败审计日志（异步，不影响事务）
		go s.logCouponUsage(userId, userCouponId, orderId, 0, 0, false, err.Error())
		return nil, s.mapModelError(err)
	}

	// 获取用户优惠券详情以获取 couponId（使用事务内查询）
	var userCoupon model.UserCoupon
	var couponId int64
	var couponType string
	if err := tx.Where("id = ?", userCouponId).First(&userCoupon).Error; err == nil {
		couponId = userCoupon.CouponId
		// 获取优惠券模板类型
		var coupon model.Coupon
		if err := tx.Where("id = ?", couponId).First(&coupon).Error; err == nil {
			couponType = coupon.Type
		}
	}

	result := &CouponUsageResult{
		UserCouponId:   userCouponId,
		CouponId:       couponId,
		OriginalAmount: orderAmount,
		DiscountAmount: modelResult.DiscountAmount,
		FinalAmount:    modelResult.FinalAmount,
		CouponType:     couponType,
		IsIdempotent:   modelResult.IsIdempotent, // 从 Model 层传递幂等标记
	}

	// 记录成功审计日志（异步，不影响事务）
	go s.logCouponUsage(userId, userCouponId, orderId, modelResult.DiscountAmount, modelResult.FinalAmount, true, "")

	return result, nil
}

// UseCouponWithPlanTx 在事务中核销优惠券（带套餐校验，Service 层封装）
// - planId 为 nil 时跳过套餐校验（保持与 Model 层一致）
// - 注意：币种校验仍由 Model 层处理（当前固定 CNY）
func (s *CouponService) UseCouponWithPlanTx(tx *gorm.DB, userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string, planId *int64) (*CouponUsageResult, error) {
	if tx == nil {
		return nil, errors.New("事务不能为空")
	}
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if userCouponId == 0 {
		return nil, errors.New("用户优惠券 ID 不能为空")
	}
	if orderId == 0 {
		return nil, errors.New("订单 ID 不能为空")
	}
	if orderAmount <= 0 {
		return nil, errors.New("订单金额必须大于 0")
	}
	if scene == "" {
		return nil, errors.New("使用场景不能为空")
	}

	modelResult, err := model.UseCouponWithPlanTx(tx, userId, userCouponId, orderId, orderAmount, scene, planId)
	if err != nil {
		go s.logCouponUsage(userId, userCouponId, orderId, 0, 0, false, err.Error())
		return nil, s.mapModelError(err)
	}

	// 获取 couponId/couponType（使用事务内查询）
	var userCoupon model.UserCoupon
	var couponId int64
	var couponType string
	if err := tx.Where("id = ?", userCouponId).First(&userCoupon).Error; err == nil {
		couponId = userCoupon.CouponId
		var coupon model.Coupon
		if err := tx.Where("id = ?", couponId).First(&coupon).Error; err == nil {
			couponType = coupon.Type
		}
	}

	result := &CouponUsageResult{
		UserCouponId:   userCouponId,
		CouponId:       couponId,
		OriginalAmount: orderAmount,
		DiscountAmount: modelResult.DiscountAmount,
		FinalAmount:    modelResult.FinalAmount,
		CouponType:     couponType,
		IsIdempotent:   modelResult.IsIdempotent,
	}

	go s.logCouponUsage(userId, userCouponId, orderId, modelResult.DiscountAmount, modelResult.FinalAmount, true, "")
	return result, nil
}

// CalculateDiscount 计算优惠金额（不实际核销）
// 用于预览或计算折扣
func (s *CouponService) CalculateDiscount(couponId int64, orderAmount int64) (int64, error) {
	if couponId == 0 {
		return 0, errors.New("优惠券 ID 不能为空")
	}
	if orderAmount <= 0 {
		return 0, errors.New("订单金额必须大于 0")
	}

	// 获取优惠券模板
	coupon, err := model.GetCouponById(couponId)
	if err != nil {
		return 0, err
	}

	// 根据优惠券类型计算折扣
	var discountAmount int64
	switch coupon.Type {
	case common.CouponTypeDiscount: // 折扣券（百分比）
		discountAmount = orderAmount * coupon.DiscountValue / 100
	case common.CouponTypeFullReduction: // 满减券
		if orderAmount >= coupon.ThresholdAmount {
			discountAmount = coupon.DiscountValue
		} else {
			return 0, errors.New("订单金额未达满减阈值")
		}
	case common.CouponTypeInstantReduction: // 立减券
		if coupon.DiscountValue > orderAmount {
			discountAmount = orderAmount
		} else {
			discountAmount = coupon.DiscountValue
		}
	default:
		return 0, errors.New("优惠券类型无效")
	}

	return discountAmount, nil
}

// CheckCouponThreshold 检查优惠券阈值
// 返回是否满足使用条件
func (s *CouponService) CheckCouponThreshold(couponId int64, orderAmount int64) (bool, string, error) {
	if couponId == 0 {
		return false, "INVALID_COUPON_ID", errors.New("优惠券 ID 不能为空")
	}

	// 获取优惠券模板
	coupon, err := model.GetCouponById(couponId)
	if err != nil {
		return false, "COUPON_NOT_FOUND", err
	}

	// 满减券需要检查阈值
	if coupon.Type == common.CouponTypeFullReduction {
		if orderAmount < coupon.ThresholdAmount {
			return false, "COUPON_THRESHOLD_NOT_MET", errors.New("订单金额未达满减阈值")
		}
	}

	return true, "", nil
}

// couponUsageLogData 优惠券核销审计日志数据结构
type couponUsageLogData struct {
	UserCouponId   int64  `json:"user_coupon_id"`
	OrderId        int64  `json:"order_id"`
	DiscountAmount int64  `json:"discount_amount"`
	FinalAmount    int64  `json:"final_amount"`
	Success        bool   `json:"success"`
	Error          string `json:"error,omitempty"`
}

// logCouponUsage 记录优惠券核销审计日志
func (s *CouponService) logCouponUsage(userId int64, userCouponId int64, orderId int64, discountAmount int64, finalAmount int64, success bool, errorMsg string) {
	logData := couponUsageLogData{
		UserCouponId:   userCouponId,
		OrderId:        orderId,
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
		Success:        success,
	}
	if !success && errorMsg != "" {
		logData.Error = errorMsg
	}

	metadataBytes, err := json.Marshal(logData)
	if err != nil {
		metadataBytes = []byte("{}")
	}

	auditLog := &model.AuditLog{
		OperatorId: &userId, // 核销时操作者即用户本人
		ObjectType: "user_coupon",
		ObjectId:   &userCouponId,
		Action:     "use_coupon",
		Metadata:   string(metadataBytes),
		CreatedAt:  common.GetTimestamp(),
	}

	_ = model.CreateAuditLog(auditLog)
}

// ===================== 2.6.3 绑定/解绑/专属用户服务方法 =====================

// CouponBindingResult 优惠券绑定结果
type CouponBindingResult struct {
	Success      bool   `json:"success"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	BindingId    int64  `json:"binding_id,omitempty"`
}

// BindCouponToRedemption 绑定优惠券到兑换码
// 用于第三方售卖场景
func (s *CouponService) BindCouponToRedemption(couponId int64, redemptionId int64, boundUserId *int64, operatorId int64) (*CouponBindingResult, error) {
	if couponId == 0 {
		return &CouponBindingResult{Success: false, ErrorCode: "INVALID_COUPON_ID", ErrorMessage: "优惠券 ID 不能为空"}, errors.New("优惠券 ID 不能为空")
	}
	if redemptionId == 0 {
		return &CouponBindingResult{Success: false, ErrorCode: "INVALID_REDEMPTION_ID", ErrorMessage: "兑换码 ID 不能为空"}, errors.New("兑换码 ID 不能为空")
	}
	if operatorId == 0 {
		return &CouponBindingResult{Success: false, ErrorCode: "INVALID_OPERATOR_ID", ErrorMessage: "操作员 ID 不能为空"}, errors.New("操作员 ID 不能为空")
	}

	// 调用 Model 层绑定（model 层已在事务内写审计日志）
	binding, err := model.BindCouponToRedemption(couponId, redemptionId, boundUserId, operatorId)
	if err != nil {
		// 仅记录失败日志（model 层事务失败不会写日志）
		go s.logCouponBinding(operatorId, couponId, redemptionId, boundUserId, "bind", false, err.Error())
		return &CouponBindingResult{Success: false, ErrorCode: "BINDING_FAILED", ErrorMessage: err.Error()}, err
	}

	// 成功日志已由 model 层写入，无需重复记录

	return &CouponBindingResult{
		Success:   true,
		BindingId: binding.Id,
	}, nil
}

// UnbindCoupon 解绑优惠券
// 仅当绑定状态为 reserved 时允许解绑
func (s *CouponService) UnbindCoupon(redemptionId int64, operatorId int64) error {
	if redemptionId == 0 {
		return errors.New("兑换码 ID 不能为空")
	}
	if operatorId == 0 {
		return errors.New("操作员 ID 不能为空")
	}

	// 调用 Model 层解绑（使用统一的 coupon_redemption_bindings 表）
	err := model.UnbindFromRedemption(redemptionId, operatorId)
	if err != nil {
		return err
	}

	return nil
}

// ValidateBoundUser 校验专属用户
// 检查用户是否有权使用绑定的优惠券
func (s *CouponService) ValidateBoundUser(redemptionId int64, userId int64) (bool, string, error) {
	if redemptionId == 0 {
		return false, "INVALID_REDEMPTION_ID", errors.New("兑换码 ID 不能为空")
	}
	if userId == 0 {
		return false, "INVALID_USER_ID", errors.New("用户 ID 不能为空")
	}

	// 获取绑定记录
	binding, err := model.GetCouponRedemptionBindingByRedemption(redemptionId)
	if err != nil {
		// 查询错误
		return false, "DATABASE_ERROR", err
	}
	if binding == nil {
		// 无绑定记录，允许使用
		return true, "", nil
	}

	// 检查专属用户
	if binding.UserId != nil && *binding.UserId != userId {
		return false, "COUPON_USER_MISMATCH", errors.New("该优惠券仅限指定用户使用")
	}

	// 检查绑定状态
	if binding.Status == common.CouponBindingStatusLocked {
		return false, "COUPON_REDEMPTION_LOCKED", errors.New("优惠券绑定已锁定")
	}

	return true, "", nil
}

// UseBoundCouponResult 使用绑定优惠券结果
type UseBoundCouponResult struct {
	HasBinding     bool   `json:"has_binding"`     // 是否有绑定的优惠券
	CouponUsed     bool   `json:"coupon_used"`     // 优惠券是否使用成功
	UserCouponId   int64  `json:"user_coupon_id"`  // 用户优惠券 ID
	DiscountAmount int64  `json:"discount_amount"` // 优惠金额
	FinalAmount    int64  `json:"final_amount"`    // 最终金额
	ErrorMessage   string `json:"error_message"`   // 错误消息（如果有）
}

// UseBoundCoupon 兑换时自动消费绑定的优惠券（事务原子操作）
// 设计流程：
// 1. 获取绑定记录并加行锁
// 2. 校验专属用户
// 3. 锁定绑定记录（reserved → locked）
// 4. 创建或获取用户优惠券并锁定（available → locked）
// 5. 核销优惠券（locked → used）
// 6. 写入审计日志
// 失败时回滚所有操作
func (s *CouponService) UseBoundCoupon(userId int64, redemptionId int64, orderId int64, orderAmount int64, scene string) (*UseBoundCouponResult, error) {
	result := &UseBoundCouponResult{
		HasBinding:  false,
		CouponUsed:  false,
		FinalAmount: orderAmount,
	}

	if redemptionId == 0 {
		return result, nil // 无兑换码，直接返回
	}

	// 先检查是否有绑定（不加锁，快速判断）
	binding, err := model.GetCouponRedemptionBindingByRedemption(redemptionId)
	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}
	if binding == nil {
		return result, nil // 无绑定记录，正常返回
	}
	result.HasBinding = true

	// 使用事务保证原子性
	var userCouponId int64
	var discountAmount, finalAmount int64

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 获取绑定记录并加行锁（FOR UPDATE）
		bindingLocked, err := model.GetCouponRedemptionBindingByRedemptionWithTx(tx, redemptionId, true)
		if err != nil {
			return err
		}
		if bindingLocked == nil {
			return errors.New(common.MsgCouponBindingNotFound)
		}

		// 2. 校验专属用户
		if bindingLocked.UserId != nil && *bindingLocked.UserId != userId {
			return errors.New("该优惠券仅限指定用户使用")
		}

		// 3. 检查绑定状态并锁定
		if bindingLocked.Status == common.CouponBindingStatusLocked {
			return errors.New("优惠券绑定已锁定")
		}

		// 4. 锁定绑定记录（reserved → locked）
		err = model.LockBindingWithTx(tx, bindingLocked.Id, userId)
		if err != nil {
			return err
		}

		// 5. 查找或创建用户优惠券
		var userCoupon *model.UserCoupon
		userCoupon, _ = model.GetUserCouponByUserAndCouponWithTx(tx, userId, bindingLocked.CouponId)

		if userCoupon == nil {
			// 创建用户优惠券实例（状态为 locked，直接准备核销）
			userCoupon = &model.UserCoupon{
				CouponId:  bindingLocked.CouponId,
				UserId:    userId,
				Code:      "UC-" + common.GetUUID(),
				Status:    common.UserCouponStatusLocked, // 直接设置为 locked
				ClaimedAt: common.GetTimestamp(),
				CreatedAt: common.GetTimestamp(),
				UpdatedAt: common.GetTimestamp(),
			}
			if err := model.CreateUserCouponWithTx(tx, userCoupon); err != nil {
				return err
			}
		} else {
			// 锁定已有的用户优惠券（available → locked）
			if userCoupon.Status == common.UserCouponStatusAvailable {
				err = model.LockUserCouponWithTx(tx, userCoupon.Id)
				if err != nil {
					return err
				}
			} else if userCoupon.Status != common.UserCouponStatusLocked {
				return errors.New("用户优惠券状态无效")
			}
		}

		// 6. 核销优惠券（locked → used）
		usageResult, err := model.UseCouponWithTx(tx, userId, userCoupon.Id, orderId, orderAmount, scene)
		if err != nil {
			return err
		}

		userCouponId = userCoupon.Id
		discountAmount = usageResult.DiscountAmount
		finalAmount = usageResult.FinalAmount

		// 7. 写入审计日志
		now := common.GetTimestamp()
		metadata := map[string]interface{}{
			"user_id":         userId,
			"redemption_id":   redemptionId,
			"coupon_id":       bindingLocked.CouponId,
			"user_coupon_id":  userCoupon.Id,
			"order_id":        orderId,
			"discount_amount": discountAmount,
			"final_amount":    finalAmount,
		}
		metadataJSON, _ := json.Marshal(metadata)
		metadataStr := string(metadataJSON)

		auditLog := &model.AuditLog{
			OperatorId: &userId,
			Action:     "coupon_use_bound",
			ObjectType: "user_coupon",
			ObjectId:   &userCoupon.Id,
			Metadata:   metadataStr,
			CreatedAt:  now,
		}
		return tx.Create(auditLog).Error
	})

	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	result.CouponUsed = true
	result.UserCouponId = userCouponId
	result.DiscountAmount = discountAmount
	result.FinalAmount = finalAmount

	return result, nil
}

// UseBoundCouponWithPlan 兑换时自动消费绑定的优惠券（带套餐校验）
// 与 UseBoundCoupon 相同，但增加了对 applicable_plan_ids 的校验
// planId 为 nil 时跳过套餐校验
func (s *CouponService) UseBoundCouponWithPlan(userId int64, redemptionId int64, orderId int64, orderAmount int64, scene string, planId *int64) (*UseBoundCouponResult, error) {
	result := &UseBoundCouponResult{
		HasBinding:  false,
		CouponUsed:  false,
		FinalAmount: orderAmount,
	}

	if redemptionId == 0 {
		return result, nil // 无兑换码，直接返回
	}

	// 先检查是否有绑定（不加锁，快速判断）
	binding, err := model.GetCouponRedemptionBindingByRedemption(redemptionId)
	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}
	if binding == nil {
		return result, nil // 无绑定记录，正常返回
	}
	result.HasBinding = true

	// 使用事务保证原子性
	var userCouponId int64
	var discountAmount, finalAmount int64

	err = model.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 获取绑定记录并加行锁（FOR UPDATE）
		bindingLocked, err := model.GetCouponRedemptionBindingByRedemptionWithTx(tx, redemptionId, true)
		if err != nil {
			return err
		}
		if bindingLocked == nil {
			return errors.New(common.MsgCouponBindingNotFound)
		}

		// 2. 校验专属用户
		if bindingLocked.UserId != nil && *bindingLocked.UserId != userId {
			return errors.New("该优惠券仅限指定用户使用")
		}

		// 3. 检查绑定状态并锁定
		if bindingLocked.Status == common.CouponBindingStatusLocked {
			return errors.New("优惠券绑定已锁定")
		}

		// 4. 锁定绑定记录（reserved → locked）
		err = model.LockBindingWithTx(tx, bindingLocked.Id, userId)
		if err != nil {
			return err
		}

		// 5. 查找或创建用户优惠券
		var userCoupon *model.UserCoupon
		userCoupon, _ = model.GetUserCouponByUserAndCouponWithTx(tx, userId, bindingLocked.CouponId)

		if userCoupon == nil {
			// 创建用户优惠券实例（状态为 locked，直接准备核销）
			userCoupon = &model.UserCoupon{
				CouponId:  bindingLocked.CouponId,
				UserId:    userId,
				Code:      "UC-" + common.GetUUID(),
				Status:    common.UserCouponStatusLocked,
				ClaimedAt: common.GetTimestamp(),
				CreatedAt: common.GetTimestamp(),
				UpdatedAt: common.GetTimestamp(),
			}
			if err := model.CreateUserCouponWithTx(tx, userCoupon); err != nil {
				return err
			}
		} else {
			// 锁定已有的用户优惠券（available → locked）
			if userCoupon.Status == common.UserCouponStatusAvailable {
				err = model.LockUserCouponWithTx(tx, userCoupon.Id)
				if err != nil {
					return err
				}
			} else if userCoupon.Status != common.UserCouponStatusLocked {
				return errors.New("用户优惠券状态无效")
			}
		}

		// 6. 核销优惠券（带套餐校验）
		usageResult, err := model.UseCouponWithPlanTx(tx, userId, userCoupon.Id, orderId, orderAmount, scene, planId)
		if err != nil {
			return err
		}

		userCouponId = userCoupon.Id
		discountAmount = usageResult.DiscountAmount
		finalAmount = usageResult.FinalAmount

		// 7. 写入审计日志
		now := common.GetTimestamp()
		metadata := map[string]interface{}{
			"user_id":         userId,
			"redemption_id":   redemptionId,
			"coupon_id":       bindingLocked.CouponId,
			"user_coupon_id":  userCoupon.Id,
			"order_id":        orderId,
			"discount_amount": discountAmount,
			"final_amount":    finalAmount,
		}
		if planId != nil {
			metadata["plan_id"] = *planId
		}
		metadataJSON, _ := json.Marshal(metadata)
		metadataStr := string(metadataJSON)

		auditLog := &model.AuditLog{
			OperatorId: &userId,
			Action:     "coupon_use_bound",
			ObjectType: "user_coupon",
			ObjectId:   &userCoupon.Id,
			Metadata:   metadataStr,
			CreatedAt:  now,
		}
		return tx.Create(auditLog).Error
	})

	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	result.CouponUsed = true
	result.UserCouponId = userCouponId
	result.DiscountAmount = discountAmount
	result.FinalAmount = finalAmount

	return result, nil
}

// UseBoundCouponWithPlanTx 兑换时自动消费绑定的优惠券（带套餐校验，使用外部事务）
// 与 UseBoundCouponWithPlan 相同，但使用外部传入的事务，保证原子性
func (s *CouponService) UseBoundCouponWithPlanTx(tx *gorm.DB, userId int64, redemptionId int64, orderId int64, orderAmount int64, scene string, planId *int64) (*UseBoundCouponResult, error) {
	result := &UseBoundCouponResult{
		HasBinding:  false,
		CouponUsed:  false,
		FinalAmount: orderAmount,
	}

	if redemptionId == 0 {
		return result, nil // 无兑换码，直接返回
	}

	// 1. 获取绑定记录并加行锁（FOR UPDATE）
	bindingLocked, err := model.GetCouponRedemptionBindingByRedemptionWithTx(tx, redemptionId, true)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return result, nil // 无绑定记录，正常返回
		}
		result.ErrorMessage = err.Error()
		return result, err
	}
	if bindingLocked == nil {
		return result, nil // 无绑定记录，正常返回
	}
	result.HasBinding = true

	// 2. 校验专属用户
	if bindingLocked.UserId != nil && *bindingLocked.UserId != userId {
		return result, errors.New("该优惠券仅限指定用户使用")
	}

	// 3. 检查绑定状态并锁定
	if bindingLocked.Status == common.CouponBindingStatusLocked {
		return result, errors.New("优惠券绑定已锁定")
	}

	// 4. 锁定绑定记录（reserved → locked）
	err = model.LockBindingWithTx(tx, bindingLocked.Id, userId)
	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	// 5. 查找或创建用户优惠券
	var userCoupon *model.UserCoupon
	userCoupon, _ = model.GetUserCouponByUserAndCouponWithTx(tx, userId, bindingLocked.CouponId)

	if userCoupon == nil {
		// 创建用户优惠券实例（状态为 locked，直接准备核销）
		userCoupon = &model.UserCoupon{
			CouponId:  bindingLocked.CouponId,
			UserId:    userId,
			Code:      "UC-" + common.GetUUID(),
			Status:    common.UserCouponStatusLocked,
			ClaimedAt: common.GetTimestamp(),
			CreatedAt: common.GetTimestamp(),
			UpdatedAt: common.GetTimestamp(),
		}
		if err := model.CreateUserCouponWithTx(tx, userCoupon); err != nil {
			result.ErrorMessage = err.Error()
			return result, err
		}
	} else {
		// 锁定已有的用户优惠券（available → locked）
		if userCoupon.Status == common.UserCouponStatusAvailable {
			err = model.LockUserCouponWithTx(tx, userCoupon.Id)
			if err != nil {
				result.ErrorMessage = err.Error()
				return result, err
			}
		} else if userCoupon.Status != common.UserCouponStatusLocked {
			return result, errors.New("用户优惠券状态无效")
		}
	}

	// 6. 核销优惠券（带套餐校验）
	usageResult, err := model.UseCouponWithPlanTx(tx, userId, userCoupon.Id, orderId, orderAmount, scene, planId)
	if err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	// 7. 写入审计日志
	now := common.GetTimestamp()
	metadata := map[string]interface{}{
		"user_id":         userId,
		"redemption_id":   redemptionId,
		"coupon_id":       bindingLocked.CouponId,
		"user_coupon_id":  userCoupon.Id,
		"order_id":        orderId,
		"discount_amount": usageResult.DiscountAmount,
		"final_amount":    usageResult.FinalAmount,
	}
	if planId != nil {
		metadata["plan_id"] = *planId
	}
	metadataJSON, _ := json.Marshal(metadata)

	auditLog := &model.AuditLog{
		OperatorId: &userId,
		Action:     "coupon_use_bound",
		ObjectType: "user_coupon",
		ObjectId:   &userCoupon.Id,
		Metadata:   string(metadataJSON),
		CreatedAt:  now,
	}
	if err := tx.Create(auditLog).Error; err != nil {
		result.ErrorMessage = err.Error()
		return result, err
	}

	result.CouponUsed = true
	result.UserCouponId = userCoupon.Id
	result.DiscountAmount = usageResult.DiscountAmount
	result.FinalAmount = usageResult.FinalAmount

	return result, nil
}

// couponBindingLogData 优惠券绑定审计日志数据结构
type couponBindingLogData struct {
	CouponId     int64  `json:"coupon_id"`
	RedemptionId int64  `json:"redemption_id"`
	BoundUserId  *int64 `json:"bound_user_id,omitempty"`
	Action       string `json:"action"` // bind / unbind
	Success      bool   `json:"success"`
	Error        string `json:"error,omitempty"`
}

// logCouponBinding 记录优惠券绑定审计日志
func (s *CouponService) logCouponBinding(operatorId int64, couponId int64, redemptionId int64, boundUserId *int64, action string, success bool, errorMsg string) {
	logData := couponBindingLogData{
		CouponId:     couponId,
		RedemptionId: redemptionId,
		BoundUserId:  boundUserId,
		Action:       action,
		Success:      success,
	}
	if !success && errorMsg != "" {
		logData.Error = errorMsg
	}

	metadataBytes, _ := json.Marshal(logData)

	auditLog := &model.AuditLog{
		OperatorId: &operatorId,
		ObjectType: "coupon_binding",
		ObjectId:   &couponId,
		Action:     "coupon_" + action,
		Metadata:   string(metadataBytes),
		CreatedAt:  common.GetTimestamp(),
	}

	_ = model.CreateAuditLog(auditLog)
}

// ===================== 2.6.4 列表与预览服务方法 =====================

// UserCouponListResult 用户优惠券列表结果
type UserCouponListResult struct {
	Coupons []*UserCouponWithDetail `json:"coupons"`
	Total   int64                   `json:"total"`
}

// UserCouponWithDetail 带详情的用户优惠券
type UserCouponWithDetail struct {
	UserCoupon    *model.UserCoupon `json:"user_coupon"`
	CouponDetail  *model.Coupon     `json:"coupon_detail"`
	RemainingDays int               `json:"remaining_days"`
	CanUse        bool              `json:"can_use"`
}

// GetUserCoupons 获取用户优惠券列表（支持状态筛选）
func (s *CouponService) GetUserCoupons(userId int64, status string, page int, pageSize int) (*UserCouponListResult, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize

	// 获取用户优惠券列表
	userCoupons, total, err := model.GetUserCouponsByUser(userId, status, offset, pageSize)
	if err != nil {
		return nil, err
	}

	// 组装详情
	result := &UserCouponListResult{
		Coupons: make([]*UserCouponWithDetail, 0, len(userCoupons)),
		Total:   total,
	}

	for _, uc := range userCoupons {
		detail := &UserCouponWithDetail{
			UserCoupon: uc,
			CanUse:     uc.Status == common.UserCouponStatusAvailable,
		}

		// 获取优惠券模板详情
		if coupon, err := model.GetCouponById(uc.CouponId); err == nil {
			detail.CouponDetail = coupon
			detail.RemainingDays = coupon.GetRemainingValidDays()
			// 更新可用状态（考虑过期）
			if detail.CanUse && coupon.IsExpired() {
				detail.CanUse = false
			}
		}

		result.Coupons = append(result.Coupons, detail)
	}

	return result, nil
}

// GetCouponByCode 根据领取码获取优惠券信息
func (s *CouponService) GetCouponByCode(code string) (*model.Coupon, error) {
	if code == "" {
		return nil, errors.New("优惠码不能为空")
	}
	return model.GetCouponByCode(code)
}

// PreviewCouponUsage 预览优惠券使用效果
// 不实际核销，仅计算折扣金额
func (s *CouponService) PreviewCouponUsage(userId int64, userCouponId int64, orderAmount int64, scene string) (*DiscountPreviewResult, error) {
	result := &DiscountPreviewResult{
		Valid:          false,
		OriginalAmount: orderAmount,
	}

	if userId == 0 {
		result.ErrorCode = "INVALID_USER_ID"
		result.ErrorMessage = "用户 ID 不能为空"
		return result, nil
	}
	if userCouponId == 0 {
		result.ErrorCode = "INVALID_USER_COUPON_ID"
		result.ErrorMessage = "用户优惠券 ID 不能为空"
		return result, nil
	}
	if orderAmount <= 0 {
		result.ErrorCode = "INVALID_ORDER_AMOUNT"
		result.ErrorMessage = "订单金额必须大于 0"
		return result, nil
	}

	// 1. 获取用户优惠券
	userCoupon, err := model.GetUserCouponById(userCouponId)
	if err != nil {
		result.ErrorCode = "COUPON_NOT_FOUND"
		result.ErrorMessage = common.MsgCouponNotFound
		return result, nil
	}

	// 2. 检查归属
	if userCoupon.UserId != userId {
		result.ErrorCode = "COUPON_USER_MISMATCH"
		result.ErrorMessage = "优惠券不属于该用户"
		return result, nil
	}

	// 3. 检查状态
	if userCoupon.Status != common.UserCouponStatusAvailable {
		result.ErrorCode = "COUPON_STATUS_INVALID"
		result.ErrorMessage = "优惠券状态无效"
		return result, nil
	}

	// 4. 获取优惠券模板
	coupon, err := model.GetCouponById(userCoupon.CouponId)
	if err != nil {
		result.ErrorCode = "COUPON_TEMPLATE_NOT_FOUND"
		result.ErrorMessage = "优惠券模板不存在"
		return result, nil
	}
	result.CouponType = coupon.Type

	// 4.1 修复 2.16：检查优惠券是否绑定到兑换码（绑定券不可通过普通预览/核销流程使用）
	// 绑定关系存储在 coupon_redemption_bindings 表
	isBound, err := model.IsCouponBoundToRedemption(coupon.Id)
	if err != nil {
		result.ErrorCode = "COUPON_BINDING_CHECK_FAILED"
		result.ErrorMessage = "检查优惠券绑定状态失败"
		return result, nil
	}
	if isBound {
		result.ErrorCode = "COUPON_BOUND_TO_REDEMPTION"
		result.ErrorMessage = "该优惠券仅限随兑换码一起使用"
		return result, nil
	}

	// 5. 检查时间有效性
	now := common.GetTimestamp()
	if now < coupon.ValidFrom {
		result.ErrorCode = "COUPON_NOT_YET_VALID"
		result.ErrorMessage = "优惠券尚未生效"
		return result, nil
	}
	if now > coupon.ValidTo {
		result.ErrorCode = "COUPON_EXPIRED"
		result.ErrorMessage = common.MsgCouponExpired
		return result, nil
	}

	// 6. 检查作用域
	if !isScopeMatch(coupon.Scope, scene) {
		result.ErrorCode = "COUPON_SCOPE_MISMATCH"
		result.ErrorMessage = "优惠券不适用于当前场景"
		return result, nil
	}

	// 7. 计算优惠金额
	discountAmount, err := s.CalculateDiscount(coupon.Id, orderAmount)
	if err != nil {
		result.ErrorCode = "COUPON_THRESHOLD_NOT_MET"
		result.ErrorMessage = err.Error()
		result.ThresholdMet = false
		return result, nil
	}
	result.ThresholdMet = true

	// 8. 计算最终金额
	finalAmount := orderAmount - discountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}

	result.Valid = true
	result.DiscountAmount = discountAmount
	result.FinalAmount = finalAmount

	return result, nil
}

// PreviewCouponUsageWithPlan 预览优惠券使用效果（带套餐/币种校验）
// - planId 为 nil 时跳过 applicable_plan_ids 校验
// - orderCurrency 为空时跳过币种校验
func (s *CouponService) PreviewCouponUsageWithPlan(userId int64, userCouponId int64, orderAmount int64, scene string, planId *int64, orderCurrency string) (*DiscountPreviewResult, error) {
	result := &DiscountPreviewResult{
		Valid:          false,
		OriginalAmount: orderAmount,
	}

	if userId == 0 {
		result.ErrorCode = "INVALID_USER_ID"
		result.ErrorMessage = "用户 ID 不能为空"
		return result, nil
	}
	if userCouponId == 0 {
		result.ErrorCode = "INVALID_USER_COUPON_ID"
		result.ErrorMessage = "用户优惠券 ID 不能为空"
		return result, nil
	}
	if orderAmount <= 0 {
		result.ErrorCode = "INVALID_ORDER_AMOUNT"
		result.ErrorMessage = "订单金额必须大于 0"
		return result, nil
	}

	// 1. 获取用户优惠券
	userCoupon, err := model.GetUserCouponById(userCouponId)
	if err != nil {
		result.ErrorCode = "COUPON_NOT_FOUND"
		result.ErrorMessage = common.MsgCouponNotFound
		return result, nil
	}

	// 2. 检查归属
	if userCoupon.UserId != userId {
		result.ErrorCode = "COUPON_USER_MISMATCH"
		result.ErrorMessage = "优惠券不属于该用户"
		return result, nil
	}

	// 3. 检查状态
	if userCoupon.Status != common.UserCouponStatusAvailable {
		result.ErrorCode = "COUPON_STATUS_INVALID"
		result.ErrorMessage = "优惠券状态无效"
		return result, nil
	}

	// 4. 获取优惠券模板
	coupon, err := model.GetCouponById(userCoupon.CouponId)
	if err != nil {
		result.ErrorCode = "COUPON_TEMPLATE_NOT_FOUND"
		result.ErrorMessage = "优惠券模板不存在"
		return result, nil
	}
	result.CouponType = coupon.Type

	// 4.1 修复 2.16：检查优惠券是否绑定到兑换码（绑定券不可通过普通预览/核销流程使用）
	// 绑定关系存储在 coupon_redemption_bindings 表
	isBound, err := model.IsCouponBoundToRedemption(coupon.Id)
	if err != nil {
		result.ErrorCode = "COUPON_BINDING_CHECK_FAILED"
		result.ErrorMessage = "检查优惠券绑定状态失败"
		return result, nil
	}
	if isBound {
		result.ErrorCode = "COUPON_BOUND_TO_REDEMPTION"
		result.ErrorMessage = "该优惠券仅限随兑换码一起使用"
		return result, nil
	}

	// 5. 检查时间有效性
	now := common.GetTimestamp()
	if now < coupon.ValidFrom {
		result.ErrorCode = "COUPON_NOT_YET_VALID"
		result.ErrorMessage = "优惠券尚未生效"
		return result, nil
	}
	if now > coupon.ValidTo {
		result.ErrorCode = "COUPON_EXPIRED"
		result.ErrorMessage = common.MsgCouponExpired
		return result, nil
	}

	// 6. 检查作用域
	if !isScopeMatch(coupon.Scope, scene) {
		result.ErrorCode = "COUPON_SCOPE_MISMATCH"
		result.ErrorMessage = "优惠券不适用于当前场景"
		return result, nil
	}

	// 7. 校验适用套餐
	if planId != nil {
		applicable, err := coupon.IsApplicableToPlan(*planId)
		if err != nil {
			result.ErrorCode = "COUPON_USE_FAILED"
			result.ErrorMessage = err.Error()
			return result, nil
		}
		if !applicable {
			result.ErrorCode = "COUPON_NOT_APPLICABLE"
			result.ErrorMessage = common.MsgCouponPlanMismatch
			return result, nil
		}
	}

	// 8. 币种校验（订单币种与优惠券币种一致）
	if orderCurrency != "" && strings.ToUpper(coupon.Currency) != strings.ToUpper(orderCurrency) {
		result.ErrorCode = "COUPON_CURRENCY_MISMATCH"
		result.ErrorMessage = "优惠券币种与订单不匹配"
		return result, nil
	}

	// 9. 计算优惠金额（基于订单原价）
	var discountAmount int64
	switch coupon.Type {
	case common.CouponTypeDiscount:
		result.ThresholdMet = true
		discountAmount = orderAmount * coupon.DiscountValue / 100
	case common.CouponTypeFullReduction:
		if orderAmount >= coupon.ThresholdAmount {
			result.ThresholdMet = true
			discountAmount = coupon.DiscountValue
		} else {
			result.ErrorCode = "COUPON_THRESHOLD_NOT_MET"
			result.ErrorMessage = "订单金额未达满减阈值"
			result.ThresholdMet = false
			return result, nil
		}
	case common.CouponTypeInstantReduction:
		result.ThresholdMet = true
		if coupon.DiscountValue > orderAmount {
			discountAmount = orderAmount
		} else {
			discountAmount = coupon.DiscountValue
		}
	default:
		result.ErrorCode = "COUPON_TYPE_INVALID"
		result.ErrorMessage = "优惠券类型无效"
		return result, nil
	}

	finalAmount := orderAmount - discountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}

	result.Valid = true
	result.DiscountAmount = discountAmount
	result.FinalAmount = finalAmount

	return result, nil
}

// PreviewCouponUsageWithPlanWithTx 预览优惠券使用效果（带套餐/币种校验，使用外部事务）
// 用于订单创建等需要事务内一致性读的场景，避免事务内再开连接导致死锁/不一致。
func (s *CouponService) PreviewCouponUsageWithPlanWithTx(tx *gorm.DB, userId int64, userCouponId int64, orderAmount int64, scene string, planId *int64, orderCurrency string) (*DiscountPreviewResult, error) {
	if tx == nil {
		return nil, errors.New("事务不能为空")
	}

	result := &DiscountPreviewResult{
		Valid:          false,
		OriginalAmount: orderAmount,
	}

	if userId == 0 {
		result.ErrorCode = "INVALID_USER_ID"
		result.ErrorMessage = "用户 ID 不能为空"
		return result, nil
	}
	if userCouponId == 0 {
		result.ErrorCode = "INVALID_USER_COUPON_ID"
		result.ErrorMessage = "用户优惠券 ID 不能为空"
		return result, nil
	}
	if orderAmount <= 0 {
		result.ErrorCode = "INVALID_ORDER_AMOUNT"
		result.ErrorMessage = "订单金额必须大于 0"
		return result, nil
	}

	// 1. 获取用户优惠券（事务内读取）
	userCoupon, err := model.GetUserCouponByIdWithTx(tx, userCouponId, false)
	if err != nil {
		result.ErrorCode = "COUPON_NOT_FOUND"
		result.ErrorMessage = common.MsgCouponNotFound
		return result, nil
	}

	// 2. 检查归属
	if userCoupon.UserId != userId {
		result.ErrorCode = "COUPON_USER_MISMATCH"
		result.ErrorMessage = "优惠券不属于该用户"
		return result, nil
	}

	// 3. 检查状态
	if userCoupon.Status != common.UserCouponStatusAvailable {
		result.ErrorCode = "COUPON_STATUS_INVALID"
		result.ErrorMessage = "优惠券状态无效"
		return result, nil
	}

	// 4. 获取优惠券模板（事务内读取）
	coupon, err := model.GetCouponByIdWithTx(tx, userCoupon.CouponId, false)
	if err != nil {
		result.ErrorCode = "COUPON_TEMPLATE_NOT_FOUND"
		result.ErrorMessage = "优惠券模板不存在"
		return result, nil
	}
	result.CouponType = coupon.Type

	// 4.1 修复 2.16：检查优惠券是否绑定到兑换码（绑定券不可通过普通预览/核销流程使用）
	// 绑定关系存储在 coupon_redemption_bindings 表
	isBound, err := model.IsCouponBoundToRedemptionWithTx(tx, coupon.Id)
	if err != nil {
		result.ErrorCode = "COUPON_BINDING_CHECK_FAILED"
		result.ErrorMessage = "检查优惠券绑定状态失败"
		return result, nil
	}
	if isBound {
		result.ErrorCode = "COUPON_BOUND_TO_REDEMPTION"
		result.ErrorMessage = "该优惠券仅限随兑换码一起使用"
		return result, nil
	}

	// 5. 检查时间有效性
	now := common.GetTimestamp()
	if now < coupon.ValidFrom {
		result.ErrorCode = "COUPON_NOT_YET_VALID"
		result.ErrorMessage = "优惠券尚未生效"
		return result, nil
	}
	if now > coupon.ValidTo {
		result.ErrorCode = "COUPON_EXPIRED"
		result.ErrorMessage = common.MsgCouponExpired
		return result, nil
	}

	// 6. 检查作用域
	if !isScopeMatch(coupon.Scope, scene) {
		result.ErrorCode = "COUPON_SCOPE_MISMATCH"
		result.ErrorMessage = "优惠券不适用于当前场景"
		return result, nil
	}

	// 7. 校验适用套餐
	if planId != nil {
		applicable, err := coupon.IsApplicableToPlan(*planId)
		if err != nil {
			result.ErrorCode = "COUPON_USE_FAILED"
			result.ErrorMessage = err.Error()
			return result, nil
		}
		if !applicable {
			result.ErrorCode = "COUPON_NOT_APPLICABLE"
			result.ErrorMessage = common.MsgCouponPlanMismatch
			return result, nil
		}
	}

	// 8. 币种校验（订单币种与优惠券币种一致）
	if orderCurrency != "" && strings.ToUpper(coupon.Currency) != strings.ToUpper(orderCurrency) {
		result.ErrorCode = "COUPON_CURRENCY_MISMATCH"
		result.ErrorMessage = "优惠券币种与订单不匹配"
		return result, nil
	}

	// 9. 计算优惠金额（基于订单原价）
	var discountAmount int64
	switch coupon.Type {
	case common.CouponTypeDiscount:
		result.ThresholdMet = true
		discountAmount = orderAmount * coupon.DiscountValue / 100
	case common.CouponTypeFullReduction:
		if orderAmount >= coupon.ThresholdAmount {
			result.ThresholdMet = true
			discountAmount = coupon.DiscountValue
		} else {
			result.ErrorCode = "COUPON_THRESHOLD_NOT_MET"
			result.ErrorMessage = "订单金额未达满减阈值"
			result.ThresholdMet = false
			return result, nil
		}
	case common.CouponTypeInstantReduction:
		result.ThresholdMet = true
		if coupon.DiscountValue > orderAmount {
			discountAmount = orderAmount
		} else {
			discountAmount = coupon.DiscountValue
		}
	default:
		result.ErrorCode = "COUPON_TYPE_INVALID"
		result.ErrorMessage = "优惠券类型无效"
		return result, nil
	}

	finalAmount := orderAmount - discountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}

	result.Valid = true
	result.DiscountAmount = discountAmount
	result.FinalAmount = finalAmount

	return result, nil
}

// isScopeMatch 校验优惠券作用域是否匹配场景
func isScopeMatch(scope string, scene string) bool {
	switch scope {
	case common.CouponScopeWallet:
		return scene == "wallet"
	case common.CouponScopeSubscription:
		return scene == "subscription"
	case common.CouponScopeWalletSubscription:
		return scene == "wallet" || scene == "subscription"
	default:
		return false
	}
}

// ===================== 2.6.5 退款处理服务方法 =====================

// RefundWithCouponRequest 带优惠券的退款请求
type RefundWithCouponRequest struct {
	OrderId        int64  `json:"order_id"`        // 订单 ID
	UserId         int64  `json:"user_id"`         // ��户 ID
	UserCouponId   int64  `json:"user_coupon_id"`  // 用户优惠券 ID（可选）
	CouponId       int64  `json:"coupon_id"`       // 优惠券模板 ID（可选）
	OriginalAmount int64  `json:"original_amount"` // 原价（分）
	DiscountAmount int64  `json:"discount_amount"` // 优惠金额（分）
	FinalAmount    int64  `json:"final_amount"`    // 实付金额（分）= 退款金额
	RefundType     string `json:"refund_type"`     // 退款类型：manual / auto
	RefundReason   string `json:"refund_reason"`   // 退款原因
	OperatorId     int64  `json:"operator_id"`     // 操作员 ID
}

// RefundWithCouponResult 带优惠券的退款结果
type RefundWithCouponResult struct {
	Success        bool   `json:"success"`
	BillId         int64  `json:"bill_id"`         // 退款账单 ID
	RefundAmount   int64  `json:"refund_amount"`   // 退款金额
	CouponRestored bool   `json:"coupon_restored"` // 优惠券是否恢复（始终为 false）
	ErrorMessage   string `json:"error_message"`   // 错误消息
}

// ProcessRefundWithCoupon 处理带优惠券的退款
// 核心规则：优惠券一旦核销（状态 used），不可恢复
// 退款金额 = 使用优惠券后的实际支付金额（final_amount）
// 设计要点：
//  1. 根据订单ID查找原账单
//  2. 检查是否已退款（防止重复退款）
//  3. 更新原账单的 refund_type 字段
//  4. 记录 refund metadata（含 coupon_restored=false）
func (s *CouponService) ProcessRefundWithCoupon(req *RefundWithCouponRequest) (*RefundWithCouponResult, error) {
	result := &RefundWithCouponResult{
		Success:        false,
		CouponRestored: false, // 优惠券永不恢复
	}

	// 参数校验
	if req.OrderId == 0 {
		result.ErrorMessage = "订单 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}
	if req.UserId == 0 {
		result.ErrorMessage = "用户 ID 不能为空"
		return result, errors.New(result.ErrorMessage)
	}
	if req.FinalAmount < 0 {
		result.ErrorMessage = "退款金额不能为负"
		return result, errors.New(result.ErrorMessage)
	}
	if req.RefundType != "manual" && req.RefundType != "auto" {
		result.ErrorMessage = "无效的退款类型"
		return result, errors.New(result.ErrorMessage)
	}

	// 使用事务保证原子性
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		return s.ProcessRefundWithCouponWithTx(tx, req, result)
	})

	if err != nil {
		return result, err
	}

	result.Success = true
	return result, nil
}

// ProcessRefundWithCouponWithTx 在事务中处理带优惠券的退款
// 支持外部事务，用于与订单退款保证原子性
func (s *CouponService) ProcessRefundWithCouponWithTx(tx *gorm.DB, req *RefundWithCouponRequest, result *RefundWithCouponResult) error {
	// 1. 根据订单ID查找原订阅账单（加行锁）
	// 注意：只查询主订阅账单类型，避免匹配到 coupon_discount 等附属账单
	var originalBill model.UserBill
	err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("source_id = ? AND user_id = ? AND bill_type IN ?",
			req.OrderId, req.UserId,
			[]string{common.BillTypeSubscription, common.BillTypeSubscriptionRenew}).
		First(&originalBill).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result.ErrorMessage = "原账单不存在"
			return errors.New(result.ErrorMessage)
		}
		result.ErrorMessage = "查询原账单失败: " + err.Error()
		return err
	}

	// 2. 检查是否已退款（防止重复退款）
	if originalBill.RefundType != nil && *originalBill.RefundType != "" && *originalBill.RefundType != "none" {
		result.ErrorMessage = "账单已退款，不可重复退款"
		return errors.New(result.ErrorMessage)
	}

	// 3. 计算退款金额（使用原账单的 final_amount）
	refundAmount := originalBill.FinalAmount
	if req.FinalAmount > 0 {
		// 校验退款金额不能超过原支付金额
		if req.FinalAmount > originalBill.FinalAmount {
			result.ErrorMessage = "退款金额不能超过原支付金额"
			return errors.New(result.ErrorMessage)
		}
		refundAmount = req.FinalAmount // 允许指定退款金额（部分退款场景）
	}
	result.RefundAmount = refundAmount

	// 4. 构建 refund metadata
	refundMetadata := map[string]interface{}{
		"original_amount": originalBill.OriginalAmount,
		"refund_amount":   refundAmount,
		"refund_reason":   req.RefundReason,
		"refund_type":     req.RefundType,
		"coupon_restored": false, // 优惠券永不恢复
	}
	if originalBill.CouponId != nil {
		refundMetadata["coupon_id"] = *originalBill.CouponId
	}
	if originalBill.UserCouponId != nil {
		refundMetadata["user_coupon_id"] = *originalBill.UserCouponId
	}
	if originalBill.DiscountAmount > 0 {
		refundMetadata["discount_amount"] = originalBill.DiscountAmount
	}
	metadataBytes, _ := json.Marshal(refundMetadata)
	metadataStr := string(metadataBytes)

	// 5. 更新原账单的退款状态（条件更新防止并发）
	updateResult := tx.Model(&model.UserBill{}).
		Where("id = ? AND (refund_type IS NULL OR refund_type = '' OR refund_type = 'none')", originalBill.Id).
		Updates(map[string]interface{}{
			"refund_type": req.RefundType,
			"metadata":    metadataStr,
		})
	if updateResult.Error != nil {
		result.ErrorMessage = "更新账单退款状态失败: " + updateResult.Error.Error()
		return updateResult.Error
	}
	if updateResult.RowsAffected == 0 {
		result.ErrorMessage = "账单并发冲突或已退款"
		return errors.New(result.ErrorMessage)
	}

	result.BillId = originalBill.Id
	return nil
}

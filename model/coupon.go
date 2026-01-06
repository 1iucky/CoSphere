package model

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// Coupon 优惠券
type Coupon struct {
	Id                int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	Code              string  `json:"code" gorm:"type:varchar(64);uniqueIndex;not null"` // 领取码: COUPON + YYYYMMDD + 6位随机大写字母数字
	Name              string  `json:"name" gorm:"type:varchar(128);not null"`
	Description       *string `json:"description" gorm:"type:text"`
	// 优惠券类型: discount(折扣), full_reduction(满减), instant_reduction(立减)
	Type              string  `json:"type" gorm:"type:varchar(32);not null;default:'discount'"`
	// 适用范围: wallet(余额充值), subscription(订阅), wallet_subscription(充值+订阅均可)
	Scope             string  `json:"scope" gorm:"type:varchar(32);not null;default:'wallet_subscription'"`
	// 折扣值: 百分比(1-100)或金额(分)
	DiscountValue     int64   `json:"discount_value" gorm:"bigint;not null"`
	// 满减阈值(分): 订单金额需达到此阈值才能使用满减券
	ThresholdAmount   int64   `json:"threshold_amount" gorm:"bigint;not null;default:0"`
	// 币种: CNY(人民币), USD(美元), EUR(欧元)
	Currency          string  `json:"currency" gorm:"type:varchar(8);not null;default:'CNY'"`
	// 乐观锁版本号: 用于防止并发超发
	Version           int64   `json:"version" gorm:"bigint;not null;default:0"`
	// 兼容旧字段: 折扣类型 percentage(百分比) / fixed(固定金额)
	DiscountType      string  `json:"discount_type" gorm:"type:varchar(32);default:'percentage'"`
	ApplicablePlanIds *string `json:"applicable_plan_ids" gorm:"type:text"`
	TotalCount        int64   `json:"total_count" gorm:"bigint;default:1"`
	UsedCount         int64   `json:"used_count" gorm:"bigint;default:0"`
	PerUserLimit      int64   `json:"per_user_limit" gorm:"bigint;default:1"`
	ValidFrom         int64   `json:"valid_from" gorm:"bigint;not null"`
	ValidTo           int64   `json:"valid_to" gorm:"bigint;not null;index"`
	Status            string  `json:"status" gorm:"type:varchar(32);default:'active';index"`
	BindUserId        *int64  `json:"bind_user_id" gorm:"index"`
	BindRedemptionId  *int64  `json:"bind_redemption_id" gorm:"index"`
	BindLockedAt      *int64  `json:"bind_locked_at" gorm:"bigint"`
	CreatedBy         int64   `json:"created_by" gorm:"bigint;not null"`
	CreatedAt         int64   `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt         int64   `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (Coupon) TableName() string {
	return "coupons"
}

// ===================== 优惠券 CRUD 方法 =====================

// CreateCoupon 创建优惠券
func CreateCoupon(coupon *Coupon) error {
	if err := validateCoupon(coupon); err != nil {
		return err
	}

	return DB.Create(coupon).Error
}

// CreateCouponWithTx 在事务中创建优惠券
func CreateCouponWithTx(tx *gorm.DB, coupon *Coupon) error {
	if err := validateCoupon(coupon); err != nil {
		return err
	}

	return tx.Create(coupon).Error
}

// UpdateCoupon 更新优惠券
func UpdateCoupon(coupon *Coupon) error {
	if coupon.Id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	if err := validateCoupon(coupon); err != nil {
		return err
	}

	return DB.Model(coupon).Updates(map[string]interface{}{
		"name":                coupon.Name,
		"description":         coupon.Description,
		"type":                coupon.Type,
		"scope":               coupon.Scope,
		"discount_value":      coupon.DiscountValue,
		"threshold_amount":    coupon.ThresholdAmount,
		"currency":            coupon.Currency,
		"discount_type":       coupon.DiscountType,
		"applicable_plan_ids": coupon.ApplicablePlanIds,
		"total_count":         coupon.TotalCount,
		"per_user_limit":      coupon.PerUserLimit,
		"valid_from":          coupon.ValidFrom,
		"valid_to":            coupon.ValidTo,
		"status":              coupon.Status,
		"bind_user_id":        coupon.BindUserId,
	}).Error
}

// GetCouponById 根据 ID 获取优惠券
func GetCouponById(id int64) (*Coupon, error) {
	if id == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var coupon Coupon
	err := DB.First(&coupon, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &coupon, nil
}

// GetCouponByIdWithTx 在事务中根据 ID 获取优惠券（带行锁）
func GetCouponByIdWithTx(tx *gorm.DB, id int64, forUpdate bool) (*Coupon, error) {
	if id == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var coupon Coupon
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.First(&coupon, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &coupon, nil
}

// GetCouponByCode 根据优惠码获取优惠券
func GetCouponByCode(code string) (*Coupon, error) {
	if code == "" {
		return nil, errors.New("优惠码不能为空")
	}

	var coupon Coupon
	err := DB.First(&coupon, "code = ?", code).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &coupon, nil
}

// GetCouponByCodeWithTx 在事务中根据优惠码获取优惠券（带行锁）
func GetCouponByCodeWithTx(tx *gorm.DB, code string, forUpdate bool) (*Coupon, error) {
	if code == "" {
		return nil, errors.New("优惠码不能为空")
	}

	var coupon Coupon
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.First(&coupon, "code = ?", code).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &coupon, nil
}

// GetAllCoupons 获取所有优惠券（分页）
func GetAllCoupons(startIdx int, num int, status string) (coupons []*Coupon, total int64, err error) {
	query := DB.Model(&Coupon{})

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&coupons).Error
	if err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}

// SearchCoupons 搜索优惠券
func SearchCoupons(keyword string, startIdx int, num int) (coupons []*Coupon, total int64, err error) {
	query := DB.Model(&Coupon{})

	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("code LIKE ? OR name LIKE ? OR description LIKE ?", likeKeyword, likeKeyword, likeKeyword)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&coupons).Error
	if err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}

// GetAllCouponsWithFilters 获取所有优惠券（支持多条件筛选）
func GetAllCouponsWithFilters(status, scope, couponType, discountType string, startIdx, num int) (coupons []*Coupon, total int64, err error) {
	query := DB.Model(&Coupon{})

	// 应用筛选条件
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if couponType != "" {
		query = query.Where("type = ?", couponType)
	}
	if discountType != "" {
		query = query.Where("discount_type = ?", discountType)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&coupons).Error
	if err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}

// SearchCouponsWithFilters 搜索优惠券（支持keyword + 多条件筛选）
func SearchCouponsWithFilters(keyword, status, scope, couponType, discountType string, startIdx, num int) (coupons []*Coupon, total int64, err error) {
	query := DB.Model(&Coupon{})

	// 关键词搜索
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("code LIKE ? OR name LIKE ? OR description LIKE ?", likeKeyword, likeKeyword, likeKeyword)
	}

	// 应用额外筛选条件
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	if couponType != "" {
		query = query.Where("type = ?", couponType)
	}
	if discountType != "" {
		query = query.Where("discount_type = ?", discountType)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&coupons).Error
	if err != nil {
		return nil, 0, err
	}

	return coupons, total, nil
}

// GetActiveCoupons 获取有效的优惠券
func GetActiveCoupons() ([]*Coupon, error) {
	var coupons []*Coupon
	now := common.GetTimestamp()

	err := DB.Where("status = ?", common.CouponStatusActive).
		Where("valid_from <= ?", now).
		Where("valid_to >= ?", now).
		Where("used_count < total_count").
		Order("created_at desc").
		Find(&coupons).Error

	if err != nil {
		return nil, err
	}

	return coupons, nil
}

// GetCouponsByUser 获取用户绑定的优惠券
func GetCouponsByUser(userId int64) ([]*Coupon, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	var coupons []*Coupon
	err := DB.Where("bind_user_id = ?", userId).
		Order("created_at desc").
		Find(&coupons).Error

	if err != nil {
		return nil, err
	}

	return coupons, nil
}

// GetExpiredCoupons 获取已过期的优惠券（用于状态更新）
func GetExpiredCoupons(limit int) ([]*Coupon, error) {
	now := common.GetTimestamp()

	var coupons []*Coupon
	err := DB.Where("status = ?", common.CouponStatusActive).
		Where("valid_to < ?", now).
		Limit(limit).
		Find(&coupons).Error

	if err != nil {
		return nil, err
	}

	return coupons, nil
}

// DeleteCoupon 删除优惠券
func DeleteCoupon(id int64) error {
	if id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	coupon, err := GetCouponById(id)
	if err != nil {
		return err
	}

	// 检查是否有使用记录
	if coupon.UsedCount > 0 {
		return errors.New("优惠券已被使用，无法删除")
	}

	// 检查是否有用户已领取记录（user_coupons 表）
	var userCouponCount int64
	err = DB.Model(&UserCoupon{}).Where("coupon_id = ?", id).Count(&userCouponCount).Error
	if err != nil {
		return err
	}
	if userCouponCount > 0 {
		return errors.New("优惠券已被用户领取，无法删除")
	}

	// 检查是否有绑定记录（coupon_redemption_bindings 表）
	var bindingCount int64
	err = DB.Model(&CouponRedemptionBinding{}).Where("coupon_id = ?", id).Count(&bindingCount).Error
	if err != nil {
		return err
	}
	if bindingCount > 0 {
		return errors.New("优惠券已绑定到兑换码，无法删除")
	}

	return DB.Delete(&Coupon{}, id).Error
}

// ===================== 优惠券使用方法 =====================

// 注意：UseCoupon 和 UseCouponWithTx 已迁移到 user_coupon.go
// 旧的实现已删除，请使用新的签名：
// func UseCoupon(userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string) (*CouponUsageResult, error)

// RefundCouponUsage 退还优惠券使用次数
func RefundCouponUsage(id int64) error {
	if id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	result := DB.Model(&Coupon{}).
		Where("id = ? AND used_count > 0", id).
		Update("used_count", gorm.Expr("used_count - 1"))

	if result.Error != nil {
		return result.Error
	}

	return nil
}

// RefundCouponUsageWithTx 在事务中退还优惠券使用次数
func RefundCouponUsageWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	result := tx.Model(&Coupon{}).
		Where("id = ? AND used_count > 0", id).
		Update("used_count", gorm.Expr("used_count - 1"))

	if result.Error != nil {
		return result.Error
	}

	return nil
}

// ===================== 绑定方法 =====================

// ReserveCouponToUser 预留优惠券给用户（可解绑）
// 绑定后状态变为 reserved，兑换前可解绑
func ReserveCouponToUser(couponId int64, userId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	result := DB.Model(&Coupon{}).
		Where("id = ? AND bind_user_id IS NULL AND bind_locked_at IS NULL AND status = ?", couponId, common.CouponStatusActive).
		Updates(map[string]interface{}{
			"bind_user_id": userId,
			"status":       common.CouponStatusReserved,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New(common.MsgCouponBindingLocked)
	}

	return nil
}

// ReserveCouponToUserWithTx 在事务中预留优惠券给用户
func ReserveCouponToUserWithTx(tx *gorm.DB, couponId int64, userId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	result := tx.Model(&Coupon{}).
		Where("id = ? AND bind_user_id IS NULL AND bind_locked_at IS NULL AND status = ?", couponId, common.CouponStatusActive).
		Updates(map[string]interface{}{
			"bind_user_id": userId,
			"status":       common.CouponStatusReserved,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New(common.MsgCouponBindingLocked)
	}

	return nil
}

// BindCouponToUser 绑定优惠券到用户（兼容旧接口，立即锁定）
func BindCouponToUser(couponId int64, userId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	now := common.GetTimestamp()

	result := DB.Model(&Coupon{}).
		Where("id = ? AND bind_user_id IS NULL AND bind_locked_at IS NULL", couponId).
		Updates(map[string]interface{}{
			"bind_user_id":   userId,
			"bind_locked_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New(common.MsgCouponBindingLocked)
	}

	return nil
}

// BindCouponToUserWithTx 在事务中绑定优惠券到用户
func BindCouponToUserWithTx(tx *gorm.DB, couponId int64, userId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	now := common.GetTimestamp()

	result := tx.Model(&Coupon{}).
		Where("id = ? AND bind_user_id IS NULL AND bind_locked_at IS NULL", couponId).
		Updates(map[string]interface{}{
			"bind_user_id":   userId,
			"bind_locked_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New(common.MsgCouponBindingLocked)
	}

	return nil
}

// 注意：BindCouponToRedemption 和 BindCouponToRedemptionWithTx 已迁移到 coupon_redemption_binding.go
// 旧的实现已删除，请使用新的签名：
// func BindCouponToRedemption(couponId int64, redemptionId int64, userId *int64, operatorId int64) (*CouponRedemptionBinding, error)
// func BindCouponToRedemptionWithTx(tx *gorm.DB, couponId int64, redemptionId int64, userId *int64, operatorId int64) (*CouponRedemptionBinding, error)

// UnbindCouponFromUser 解绑优惠券（仅 reserved 状态可解绑）
func UnbindCouponFromUser(couponId int64, userId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	result := DB.Model(&Coupon{}).
		Where("id = ? AND bind_user_id = ? AND bind_locked_at IS NULL AND status = ?", couponId, userId, common.CouponStatusReserved).
		Updates(map[string]interface{}{
			"bind_user_id": nil,
			"status":       common.CouponStatusActive,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("无法解绑：优惠券已锁定或不属于该用户")
	}

	return nil
}

// UnbindCouponFromUserWithTx 在事务中解绑优惠券
func UnbindCouponFromUserWithTx(tx *gorm.DB, couponId int64, userId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	result := tx.Model(&Coupon{}).
		Where("id = ? AND bind_user_id = ? AND bind_locked_at IS NULL AND status = ?", couponId, userId, common.CouponStatusReserved).
		Updates(map[string]interface{}{
			"bind_user_id": nil,
			"status":       common.CouponStatusActive,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("无法解绑：优惠券已锁定或不属于该用户")
	}

	return nil
}

// LockCoupon 锁定优惠券（兑换时调用，锁定后不可解绑）
func LockCoupon(couponId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	now := common.GetTimestamp()

	result := DB.Model(&Coupon{}).
		Where("id = ? AND bind_locked_at IS NULL AND status IN ?", couponId, []string{common.CouponStatusActive, common.CouponStatusReserved}).
		Update("bind_locked_at", now)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券已锁定或状态无效")
	}

	return nil
}

// LockCouponWithTx 在事务中锁定优惠券
func LockCouponWithTx(tx *gorm.DB, couponId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	now := common.GetTimestamp()

	result := tx.Model(&Coupon{}).
		Where("id = ? AND bind_locked_at IS NULL AND status IN ?", couponId, []string{common.CouponStatusActive, common.CouponStatusReserved}).
		Update("bind_locked_at", now)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券已锁定或状态无效")
	}

	return nil
}

// ===================== 状态管理方法 =====================

// DeactivateCoupon 停用优惠券
func DeactivateCoupon(id int64) error {
	if id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	return DB.Model(&Coupon{}).Where("id = ?", id).
		Update("status", common.CouponStatusInactive).Error
}

// ActivateCoupon 激活优惠券
func ActivateCoupon(id int64) error {
	if id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	return DB.Model(&Coupon{}).Where("id = ?", id).
		Update("status", common.CouponStatusActive).Error
}

// ExpireCoupon 过期优惠券
func ExpireCoupon(id int64) error {
	if id == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	return DB.Model(&Coupon{}).Where("id = ?", id).
		Update("status", common.CouponStatusExpired).Error
}

// ===================== 验证方法 =====================

// couponCodeRegex 优惠券码格式正则：COUPON + YYYYMMDD + 6位大写字母数字
// 示例：COUPON20250109AB12CD
var couponCodeRegex = regexp.MustCompile(`^COUPON\d{8}[A-Z0-9]{6}$`)

// validateCoupon 验证优惠券数据
func validateCoupon(coupon *Coupon) error {
	if strings.TrimSpace(coupon.Code) == "" {
		return errors.New("优惠码不能为空")
	}

	// 验证优惠券码格式：COUPON + YYYYMMDD + 6位大写字母数字
	if !couponCodeRegex.MatchString(coupon.Code) {
		return errors.New("优惠码格式无效：必须为 COUPON + YYYYMMDD + 6位大写字母数字，如 COUPON20250109AB12CD")
	}

	if strings.TrimSpace(coupon.Name) == "" {
		return errors.New("优惠券名称不能为空")
	}

	// 验证优惠券类型
	validTypes := map[string]bool{
		common.CouponTypeDiscount:         true,
		common.CouponTypeFullReduction:    true,
		common.CouponTypeInstantReduction: true,
	}
	if !validTypes[coupon.Type] {
		return errors.New("无效的优惠券类型：必须是 discount/full_reduction/instant_reduction")
	}

	// 验证适用范围
	validScopes := map[string]bool{
		common.CouponScopeWallet:              true,
		common.CouponScopeSubscription:        true,
		common.CouponScopeWalletSubscription:  true,
	}
	if !validScopes[coupon.Scope] {
		return errors.New("无效的优惠券作用域：必须是 wallet/subscription/wallet_subscription")
	}

	// 验证币种：当前固定 CNY
	if coupon.Currency != "CNY" {
		return errors.New("无效的币种：当前仅支持 CNY")
	}

	// 验证满减阈值
	if coupon.ThresholdAmount < 0 {
		return errors.New("满减阈值不能为负数")
	}

	if coupon.DiscountValue <= 0 {
		return errors.New("折扣值必须大于 0")
	}

	// 折扣券类型验证折扣值范围 (1-100)
	if coupon.Type == common.CouponTypeDiscount && (coupon.DiscountValue < 1 || coupon.DiscountValue > 100) {
		return errors.New("折扣券的折扣值必须在 1-100 之间（百分比）")
	}

	// 兼容旧的 DiscountType 验证
	if coupon.DiscountType != "" {
		validDiscountTypes := map[string]bool{
			common.DiscountTypePercentage: true,
			common.DiscountTypeFixed:      true,
		}
		if !validDiscountTypes[coupon.DiscountType] {
			return errors.New("无效的折扣类型")
		}

		if coupon.DiscountType == common.DiscountTypePercentage && coupon.DiscountValue > 100 {
			return errors.New("百分比折扣不能超过 100")
		}
	}

	// TotalCount: 必须 >= 1（不再支持 -1 无限库存）
	if coupon.TotalCount < 1 {
		return errors.New("优惠券总数量至少为 1")
	}

	// PerUserLimit: 必须 >= 1（不再支持 -1 无限）
	if coupon.PerUserLimit < 1 {
		return errors.New("每用户限制至少为 1")
	}

	if coupon.ValidFrom == 0 {
		return errors.New("生效时间不能为空")
	}

	if coupon.ValidTo == 0 {
		return errors.New("过期时间不能为空")
	}

	if coupon.ValidTo <= coupon.ValidFrom {
		return errors.New("过期时间必须晚于生效时间")
	}

	if coupon.CreatedBy == 0 {
		return errors.New("创建者 ID 不能为空")
	}

	validStatuses := map[string]bool{
		common.CouponStatusActive:   true,
		common.CouponStatusInactive: true,
		common.CouponStatusExpired:  true,
		common.CouponStatusReserved: true,
	}
	if coupon.Status != "" && !validStatuses[coupon.Status] {
		return errors.New("无效的优惠券状态")
	}

	return nil
}

// ===================== 辅助方法 =====================

// IsValid 检查优惠券是否有效
func (c *Coupon) IsValid() bool {
	if c.Status != common.CouponStatusActive {
		return false
	}

	now := common.GetTimestamp()
	if now < c.ValidFrom || now > c.ValidTo {
		return false
	}

	// 检查库存是否用完
	if c.UsedCount >= c.TotalCount {
		return false
	}

	return true
}

// IsExpired 检查优惠券是否已过期
func (c *Coupon) IsExpired() bool {
	now := common.GetTimestamp()
	return now > c.ValidTo
}

// GetRemainingCount 获取剩余使用次数
// 返回值：0 表示已用完，>0 表示剩余数量
func (c *Coupon) GetRemainingCount() int64 {
	if c.TotalCount <= c.UsedCount {
		return 0
	}
	return c.TotalCount - c.UsedCount
}

// CanBeUsedByUser 检查用户是否可以使用此优惠券
func (c *Coupon) CanBeUsedByUser(userId int64) bool {
	// 检查是否绑定到特定用户
	if c.BindUserId != nil && *c.BindUserId != userId {
		return false
	}

	return c.IsValid()
}

// GetApplicablePlanIds 获取适用的套餐 ID 列表
func (c *Coupon) GetApplicablePlanIds() ([]int64, error) {
	if c.ApplicablePlanIds == nil || *c.ApplicablePlanIds == "" {
		return []int64{}, nil
	}

	var planIds []int64
	err := json.Unmarshal([]byte(*c.ApplicablePlanIds), &planIds)
	if err != nil {
		return nil, err
	}

	return planIds, nil
}

// SetApplicablePlanIds 设置适用的套餐 ID 列表
func (c *Coupon) SetApplicablePlanIds(planIds []int64) error {
	if len(planIds) == 0 {
		c.ApplicablePlanIds = nil
		return nil
	}

	data, err := json.Marshal(planIds)
	if err != nil {
		return err
	}
	str := string(data)
	c.ApplicablePlanIds = &str
	return nil
}

// IsApplicableToPlan 检查优惠券是否适用于指定套餐
func (c *Coupon) IsApplicableToPlan(planId int64) (bool, error) {
	planIds, err := c.GetApplicablePlanIds()
	if err != nil {
		return false, err
	}

	// 空列表表示适用于所有套餐
	if len(planIds) == 0 {
		return true, nil
	}

	for _, id := range planIds {
		if id == planId {
			return true, nil
		}
	}

	return false, nil
}

// CalculateDiscount 计算折扣金额
func (c *Coupon) CalculateDiscount(originalAmount int64) int64 {
	switch c.DiscountType {
	case common.DiscountTypePercentage:
		// 百分比折扣
		discount := originalAmount * c.DiscountValue / 100
		return discount
	case common.DiscountTypeFixed:
		// 固定金额折扣
		if c.DiscountValue > originalAmount {
			return originalAmount
		}
		return c.DiscountValue
	default:
		return 0
	}
}

// GetFinalPrice 计算最终价格
func (c *Coupon) GetFinalPrice(originalAmount int64) int64 {
	discount := c.CalculateDiscount(originalAmount)
	finalPrice := originalAmount - discount
	if finalPrice < 0 {
		return 0
	}
	return finalPrice
}

// IsBound 检查优惠券是否已绑定
func (c *Coupon) IsBound() bool {
	return c.BindUserId != nil || c.BindRedemptionId != nil
}

// IsReserved 检查优惠券是否为预留状态（已绑定但未锁定）
func (c *Coupon) IsReserved() bool {
	return c.Status == common.CouponStatusReserved && c.BindLockedAt == nil
}

// IsLocked 检查优惠券是否已锁定（已兑换使用）
func (c *Coupon) IsLocked() bool {
	return c.BindLockedAt != nil
}

// CanUnbind 检查优惠券是否可以解绑
func (c *Coupon) CanUnbind() bool {
	return c.IsReserved() && !c.IsLocked()
}

// GetRemainingValidDays 获取剩余有效天数
func (c *Coupon) GetRemainingValidDays() int {
	now := common.GetTimestamp()
	if now > c.ValidTo {
		return 0
	}

	remainingSeconds := c.ValidTo - now
	return int(remainingSeconds / (24 * 60 * 60))
}

// ===================== 用户使用记录跟踪 =====================

// CouponUsage 优惠券使用记录（用于跟踪每用户使用次数）
type CouponUsage struct {
	Id        int64 `json:"id" gorm:"primaryKey;autoIncrement"`
	CouponId  int64 `json:"coupon_id" gorm:"not null;uniqueIndex:idx_coupon_user,priority:1"`
	UserId    int64 `json:"user_id" gorm:"not null;uniqueIndex:idx_coupon_user,priority:2"`
	UsedCount int64 `json:"used_count" gorm:"bigint;default:0"`
	CreatedAt int64 `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt int64 `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (CouponUsage) TableName() string {
	return "coupon_usages"
}

// GetOrCreateCouponUsage 获取或创建用户优惠券使用记录
func GetOrCreateCouponUsage(couponId int64, userId int64) (*CouponUsage, error) {
	var usage CouponUsage

	err := DB.Where("coupon_id = ? AND user_id = ?", couponId, userId).First(&usage).Error
	if err == nil {
		return &usage, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	usage = CouponUsage{
		CouponId:  couponId,
		UserId:    userId,
		UsedCount: 0,
	}

	err = DB.Create(&usage).Error
	if err != nil {
		// 可能是并发创建，尝试重新获取
		err = DB.Where("coupon_id = ? AND user_id = ?", couponId, userId).First(&usage).Error
		if err != nil {
			return nil, err
		}
	}

	return &usage, nil
}

// IncrementCouponUsage 增加用户优惠券使用次数
func IncrementCouponUsage(couponId int64, userId int64, perUserLimit int64) error {
	usage, err := GetOrCreateCouponUsage(couponId, userId)
	if err != nil {
		return err
	}

	if usage.UsedCount >= perUserLimit {
		return errors.New(common.MsgCouponUserLimitReached)
	}

	return DB.Model(&CouponUsage{}).
		Where("id = ? AND used_count < ?", usage.Id, perUserLimit).
		Update("used_count", gorm.Expr("used_count + 1")).Error
}

// IncrementCouponUsageWithTx 在事务中增加用户优惠券使用次数
func IncrementCouponUsageWithTx(tx *gorm.DB, couponId int64, userId int64, perUserLimit int64) error {
	var usage CouponUsage

	err := tx.Where("coupon_id = ? AND user_id = ?", couponId, userId).First(&usage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		usage = CouponUsage{
			CouponId:  couponId,
			UserId:    userId,
			UsedCount: 1,
		}
		return tx.Create(&usage).Error
	}

	if err != nil {
		return err
	}

	if usage.UsedCount >= perUserLimit {
		return errors.New(common.MsgCouponUserLimitReached)
	}

	return tx.Model(&CouponUsage{}).
		Where("id = ? AND used_count < ?", usage.Id, perUserLimit).
		Update("used_count", gorm.Expr("used_count + 1")).Error
}

// GetUserCouponUsage 获取用户对特定优惠券的使用次数
func GetUserCouponUsage(couponId int64, userId int64) (int64, error) {
	var usage CouponUsage

	err := DB.Where("coupon_id = ? AND user_id = ?", couponId, userId).First(&usage).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}

	if err != nil {
		return 0, err
	}

	return usage.UsedCount, nil
}

// ===================== 优惠券解绑与锁定方法 =====================

// UnbindCouponFromRedemption 解绑优惠券与兑换码（仅当未锁定时允许）
// 只能解绑 status=reserved 且 bind_locked_at IS NULL 的优惠券
func UnbindCouponFromRedemption(couponId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	result := DB.Model(&Coupon{}).
		Where("id = ? AND bind_redemption_id IS NOT NULL AND bind_locked_at IS NULL AND status = ?",
			couponId, common.CouponStatusReserved).
		Updates(map[string]interface{}{
			"bind_redemption_id": nil,
			"status":             common.CouponStatusActive,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("无法解绑：优惠券可能已锁定或状态无效")
	}

	return nil
}

// UnbindCouponFromRedemptionWithTx 在事务中解绑优惠券与兑换码
func UnbindCouponFromRedemptionWithTx(tx *gorm.DB, couponId int64) error {
	if couponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	result := tx.Model(&Coupon{}).
		Where("id = ? AND bind_redemption_id IS NOT NULL AND bind_locked_at IS NULL AND status = ?",
			couponId, common.CouponStatusReserved).
		Updates(map[string]interface{}{
			"bind_redemption_id": nil,
			"status":             common.CouponStatusActive,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("无法解绑：优惠券可能已锁定或状态无效")
	}

	return nil
}

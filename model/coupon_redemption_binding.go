package model

import (
	"encoding/json"
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// CouponRedemptionBinding 优惠券兑换码绑定关系
// 设计约束：一个兑换码只能绑定一张优惠券（redemption_id 唯一）
type CouponRedemptionBinding struct {
	Id           int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CouponId     int64  `json:"coupon_id" gorm:"bigint;not null;index:idx_binding_coupon"`
	RedemptionId int64  `json:"redemption_id" gorm:"bigint;not null;uniqueIndex:uq_binding_redemption"`
	UserId       *int64 `json:"user_id" gorm:"bigint;index:idx_binding_user"`               // 专属用户ID（可选，NULL 表示不限用户）
	Status       string `json:"status" gorm:"type:varchar(32);not null;default:'reserved'"` // reserved/locked
	LockedAt     *int64 `json:"locked_at" gorm:"bigint"`                                     // 锁定时间
	CreatedAt    int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (CouponRedemptionBinding) TableName() string {
	return "coupon_redemption_bindings"
}

// ===================== 绑定 CRUD 方法 =====================

// CreateCouponRedemptionBinding 创建绑定记录
func CreateCouponRedemptionBinding(binding *CouponRedemptionBinding) error {
	if err := validateCouponRedemptionBinding(binding); err != nil {
		return err
	}

	now := common.GetTimestamp()
	if binding.CreatedAt == 0 {
		binding.CreatedAt = now
	}
	if binding.UpdatedAt == 0 {
		binding.UpdatedAt = now
	}

	return DB.Create(binding).Error
}

// CreateCouponRedemptionBindingWithTx 在事务中创建绑定记录
func CreateCouponRedemptionBindingWithTx(tx *gorm.DB, binding *CouponRedemptionBinding) error {
	if err := validateCouponRedemptionBinding(binding); err != nil {
		return err
	}

	now := common.GetTimestamp()
	if binding.CreatedAt == 0 {
		binding.CreatedAt = now
	}
	if binding.UpdatedAt == 0 {
		binding.UpdatedAt = now
	}

	return tx.Create(binding).Error
}

// GetCouponRedemptionBindingById 根据 ID 获取绑定记录
func GetCouponRedemptionBindingById(id int64) (*CouponRedemptionBinding, error) {
	if id == 0 {
		return nil, errors.New("绑定记录 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	err := DB.First(&binding, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponBindingNotFound)
		}
		return nil, err
	}

	return &binding, nil
}

// GetCouponRedemptionBindingByIdWithTx 在事务中根据 ID 获取绑定记录（带行锁）
func GetCouponRedemptionBindingByIdWithTx(tx *gorm.DB, id int64, forUpdate bool) (*CouponRedemptionBinding, error) {
	if id == 0 {
		return nil, errors.New("绑定记录 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.First(&binding, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponBindingNotFound)
		}
		return nil, err
	}

	return &binding, nil
}

// GetCouponRedemptionBindingByRedemption 根据兑换码 ID 获取绑定记录
func GetCouponRedemptionBindingByRedemption(redemptionId int64) (*CouponRedemptionBinding, error) {
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	err := DB.Where("redemption_id = ?", redemptionId).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil
		}
		return nil, err
	}

	return &binding, nil
}

// GetCouponRedemptionBindingByRedemptionWithTx 在事务中根据兑换码 ID 获取绑定记录（带行锁）
func GetCouponRedemptionBindingByRedemptionWithTx(tx *gorm.DB, redemptionId int64, forUpdate bool) (*CouponRedemptionBinding, error) {
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.Where("redemption_id = ?", redemptionId).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil
		}
		return nil, err
	}

	return &binding, nil
}

// GetCouponRedemptionBindingByCoupon 根据优惠券 ID 获取绑定记录（单条，向后兼容）
func GetCouponRedemptionBindingByCoupon(couponId int64) (*CouponRedemptionBinding, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	err := DB.Where("coupon_id = ?", couponId).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil
		}
		return nil, err
	}

	return &binding, nil
}

// GetAllCouponRedemptionBindingsByCoupon 根据优惠券 ID 获取所有绑定记录（一券多码支持）
func GetAllCouponRedemptionBindingsByCoupon(couponId int64) ([]*CouponRedemptionBinding, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var bindings []*CouponRedemptionBinding
	err := DB.Where("coupon_id = ?", couponId).Order("created_at DESC").Find(&bindings).Error
	if err != nil {
		return nil, err
	}

	return bindings, nil
}

// IsCouponBoundToRedemption 检查优惠券是否绑定到任何兑换码
// 修复 2.16：绑定关系存储在 coupon_redemption_bindings 表，而非 coupon.bind_redemption_id
// 返回 true 表示优惠券已绑定到兑换码，不可通过普通领取/预览/核销流程使用
func IsCouponBoundToRedemption(couponId int64) (bool, error) {
	if couponId == 0 {
		return false, nil
	}

	var count int64
	err := DB.Model(&CouponRedemptionBinding{}).
		Where("coupon_id = ?", couponId).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// IsCouponBoundToRedemptionWithTx 在事务中检查优惠券是否绑定到任何兑换码
func IsCouponBoundToRedemptionWithTx(tx *gorm.DB, couponId int64) (bool, error) {
	if couponId == 0 {
		return false, nil
	}

	var count int64
	err := tx.Model(&CouponRedemptionBinding{}).
		Where("coupon_id = ?", couponId).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetCouponRedemptionBindingByCouponWithTx 在事务中根据优惠券 ID 获取绑定记录（带行锁）
func GetCouponRedemptionBindingByCouponWithTx(tx *gorm.DB, couponId int64, forUpdate bool) (*CouponRedemptionBinding, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.Where("coupon_id = ?", couponId).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil
		}
		return nil, err
	}

	return &binding, nil
}

// GetCouponRedemptionBindingByCouponAndRedemption 根据优惠券 ID 和兑换码 ID 获取绑定记录
func GetCouponRedemptionBindingByCouponAndRedemption(couponId int64, redemptionId int64) (*CouponRedemptionBinding, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}

	var binding CouponRedemptionBinding
	err := DB.Where("coupon_id = ? AND redemption_id = ?", couponId, redemptionId).First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil
		}
		return nil, err
	}

	return &binding, nil
}

// UpdateCouponRedemptionBindingStatus 更新绑定记录状态
func UpdateCouponRedemptionBindingStatus(id int64, status string) error {
	if id == 0 {
		return errors.New("绑定记录 ID 不能为空")
	}
	if status == "" {
		return errors.New("状态不能为空")
	}

	// 验证状态合法性
	validStatuses := map[string]bool{
		common.CouponBindingStatusReserved: true,
		common.CouponBindingStatusLocked:   true,
	}
	if !validStatuses[status] {
		return errors.New("无效的绑定状态")
	}

	now := common.GetTimestamp()
	return DB.Model(&CouponRedemptionBinding{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		}).Error
}

// UpdateCouponRedemptionBindingStatusWithTx 在事务中更新绑定记录状态
func UpdateCouponRedemptionBindingStatusWithTx(tx *gorm.DB, id int64, status string) error {
	if id == 0 {
		return errors.New("绑定记录 ID 不能为空")
	}
	if status == "" {
		return errors.New("状态不能为空")
	}

	// 验证状态合法性
	validStatuses := map[string]bool{
		common.CouponBindingStatusReserved: true,
		common.CouponBindingStatusLocked:   true,
	}
	if !validStatuses[status] {
		return errors.New("无效的绑定状态")
	}

	now := common.GetTimestamp()
	return tx.Model(&CouponRedemptionBinding{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		}).Error
}

// ===================== 绑定业务方法 =====================

// BindCouponToRedemption 绑定优惠券到兑换码
// 校验优惠券存在且状态为 active，创建绑定记录（status = 'reserved'），写入审计日志
func BindCouponToRedemption(couponId int64, redemptionId int64, userId *int64, operatorId int64) (*CouponRedemptionBinding, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}
	if operatorId == 0 {
		return nil, errors.New("操作者 ID 不能为空")
	}

	var binding *CouponRedemptionBinding

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1. 查询优惠券并验证状态
		var coupon Coupon
		err := tx.Where("id = ?", couponId).First(&coupon).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgCouponNotFound)
			}
			return err
		}

		// 验证优惠券状态为 active
		if coupon.Status != common.CouponStatusActive {
			return errors.New("优惠券状态无效，只能绑定 active 状态的优惠券")
		}

		// 2. 查询兑换码并验证状态
		var redemption Redemption
		err = tx.Where("id = ?", redemptionId).First(&redemption).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgRedemptionNotFound)
			}
			return err
		}

		// 验证兑换码状态为 enabled
		if redemption.Status != common.RedemptionCodeStatusEnabled {
			return errors.New("兑换码状态无效，只能绑定 enabled 状态的兑换码")
		}

		// 3. 检查是否已存在绑定（一个兑换码只能绑定一张优惠券）
		var existingBinding CouponRedemptionBinding
		err = tx.Where("redemption_id = ?", redemptionId).First(&existingBinding).Error
		if err == nil {
			return errors.New("该兑换码已绑定优惠券")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// 4. 创建绑定记录
		now := common.GetTimestamp()
		binding = &CouponRedemptionBinding{
			CouponId:     couponId,
			RedemptionId: redemptionId,
			UserId:       userId,
			Status:       common.CouponBindingStatusReserved,
			CreatedAt:    now,
			UpdatedAt:    now,
		}

		err = tx.Create(binding).Error
		if err != nil {
			return err
		}

		// 5. 写入审计日志
		metadata := map[string]interface{}{
			"coupon_id":     couponId,
			"redemption_id": redemptionId,
			"user_id":       userId,
			"binding_id":    binding.Id,
		}
		metadataJSON, _ := json.Marshal(metadata)
		metadataStr := string(metadataJSON)

		auditLog := &AuditLog{
			OperatorId: &operatorId,
			Action:     "coupon_bind",
			ObjectType: "coupon_redemption_binding",
			ObjectId:   &binding.Id,
			Metadata:   metadataStr,
			CreatedAt:  now,
		}

		err = tx.Create(auditLog).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return binding, nil
}

// BindCouponToRedemptionWithTx 在事务中绑定优惠券到兑换码
func BindCouponToRedemptionWithTx(tx *gorm.DB, couponId int64, redemptionId int64, userId *int64, operatorId int64) (*CouponRedemptionBinding, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}
	if operatorId == 0 {
		return nil, errors.New("操作者 ID 不能为空")
	}

	// 1. 查询优惠券并验证状态
	var coupon Coupon
	err := tx.Where("id = ?", couponId).First(&coupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	// 验证优惠券状态为 active
	if coupon.Status != common.CouponStatusActive {
		return nil, errors.New("优惠券状态无效，只能绑定 active 状态的优惠券")
	}

	// 2. 查询兑换码并验证状态
	var redemption Redemption
	err = tx.Where("id = ?", redemptionId).First(&redemption).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgRedemptionNotFound)
		}
		return nil, err
	}

	// 验证兑换码状态为 enabled
	if redemption.Status != common.RedemptionCodeStatusEnabled {
		return nil, errors.New("兑换码状态无效，只能绑定 enabled 状态的兑换码")
	}

	// 3. 检查是否已存在绑定（一个兑换码只能绑定一张优惠券）
	var existingBinding CouponRedemptionBinding
	err = tx.Where("redemption_id = ?", redemptionId).First(&existingBinding).Error
	if err == nil {
		return nil, errors.New("该兑换码已绑定优惠券")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 4. 创建绑定记录
	now := common.GetTimestamp()
	binding := &CouponRedemptionBinding{
		CouponId:     couponId,
		RedemptionId: redemptionId,
		UserId:       userId,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = tx.Create(binding).Error
	if err != nil {
		return nil, err
	}

	// 5. 写入审计日志
	metadata := map[string]interface{}{
		"coupon_id":     couponId,
		"redemption_id": redemptionId,
		"user_id":       userId,
		"binding_id":    binding.Id,
	}
	metadataJSON, _ := json.Marshal(metadata)
	metadataStr := string(metadataJSON)

	auditLog := &AuditLog{
		OperatorId: &operatorId,
		Action:     "coupon_bind",
		ObjectType: "coupon_redemption_binding",
		ObjectId:   &binding.Id,
		Metadata:   metadataStr,
		CreatedAt:  now,
	}

	err = tx.Create(auditLog).Error
	if err != nil {
		return nil, err
	}

	return binding, nil
}

// LockBinding 锁定绑定记录（reserved → locked）
// 获取绑定记录并加行锁，校验专属用户（若 user_id 不为空），原子更新状态为 locked
func LockBinding(id int64, userId int64) error {
	if id == 0 {
		return errors.New("绑定记录 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1. 获取绑定记录并加行锁（FOR UPDATE）
		var binding CouponRedemptionBinding
		err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ?", id).
			First(&binding).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgCouponBindingNotFound)
			}
			return err
		}

		// 2. 校验状态必须为 reserved
		if binding.Status != common.CouponBindingStatusReserved {
			return errors.New("绑定记录状态无效，只能锁定 reserved 状态的绑定")
		}

		// 3. 校验专属用户（若 user_id 不为空）
		if binding.UserId != nil && *binding.UserId != userId {
			return errors.New("该优惠券为专属用户优惠券，无法使用")
		}

		// 4. 原子更新状态为 locked
		now := common.GetTimestamp()
		result := tx.Model(&CouponRedemptionBinding{}).
			Where("id = ? AND status = ?", id, common.CouponBindingStatusReserved).
			Updates(map[string]interface{}{
				"status":     common.CouponBindingStatusLocked,
				"locked_at":  now,
				"updated_at": now,
			})

		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return errors.New("绑定记录状态并发冲突")
		}

		return nil
	})

	return err
}

// LockBindingWithTx 在事务中锁定绑定记录
func LockBindingWithTx(tx *gorm.DB, id int64, userId int64) error {
	if id == 0 {
		return errors.New("绑定记录 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	// 1. 获取绑定记录并加行锁（FOR UPDATE）
	var binding CouponRedemptionBinding
	err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ?", id).
		First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(common.MsgCouponBindingNotFound)
		}
		return err
	}

	// 2. 校验状态必须为 reserved
	if binding.Status != common.CouponBindingStatusReserved {
		return errors.New("绑定记录状态无效，只能锁定 reserved 状态的绑定")
	}

	// 3. 校验专属用户（若 user_id 不为空）
	if binding.UserId != nil && *binding.UserId != userId {
		return errors.New("该优惠券为专属用户优惠券，无法使用")
	}

	// 4. 原子更新状态为 locked
	now := common.GetTimestamp()
	result := tx.Model(&CouponRedemptionBinding{}).
		Where("id = ? AND status = ?", id, common.CouponBindingStatusReserved).
		Updates(map[string]interface{}{
			"status":     common.CouponBindingStatusLocked,
			"locked_at":  now,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("绑定记录状态并发冲突")
	}

	return nil
}

// UnbindFromRedemption 解绑优惠券与兑换码
// 仅当绑定状态为 reserved 时允许解绑，删除 coupon_redemption_bindings 表记录
func UnbindFromRedemption(redemptionId int64, operatorId int64) error {
	if redemptionId == 0 {
		return errors.New("兑换码 ID 不能为空")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		// 1. 获取绑定记录并加行锁
		var binding CouponRedemptionBinding
		err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("redemption_id = ?", redemptionId).
			First(&binding).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgCouponBindingNotFound)
			}
			return err
		}

		// 2. 检查状态：只有 reserved 状态才能解绑
		if binding.Status != common.CouponBindingStatusReserved {
			return errors.New("无法解绑：绑定已锁定")
		}

		// 3. 删除绑定记录
		result := tx.Delete(&CouponRedemptionBinding{}, "id = ?", binding.Id)
		if result.Error != nil {
			return result.Error
		}

		// 4. 写入审计日志
		now := common.GetTimestamp()
		metadata := map[string]interface{}{
			"coupon_id":     binding.CouponId,
			"redemption_id": redemptionId,
			"binding_id":    binding.Id,
		}
		metadataJSON, _ := json.Marshal(metadata)
		metadataStr := string(metadataJSON)

		auditLog := &AuditLog{
			OperatorId: &operatorId,
			Action:     "coupon_unbind",
			ObjectType: "coupon_redemption_binding",
			ObjectId:   &binding.Id,
			Metadata:   metadataStr,
			CreatedAt:  now,
		}

		return tx.Create(auditLog).Error
	})
}

// UnbindFromRedemptionWithTx 在事务中解绑优惠券与兑换码
func UnbindFromRedemptionWithTx(tx *gorm.DB, redemptionId int64, operatorId int64) error {
	if redemptionId == 0 {
		return errors.New("兑换码 ID 不能为空")
	}

	// 1. 获取绑定记录并加行锁
	var binding CouponRedemptionBinding
	err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("redemption_id = ?", redemptionId).
		First(&binding).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New(common.MsgCouponBindingNotFound)
		}
		return err
	}

	// 2. 检查状态：只有 reserved 状态才能解绑
	if binding.Status != common.CouponBindingStatusReserved {
		return errors.New("无法解绑：绑定已锁定")
	}

	// 3. 删除绑定记录
	result := tx.Delete(&CouponRedemptionBinding{}, "id = ?", binding.Id)
	if result.Error != nil {
		return result.Error
	}

	// 4. 写入审计日志
	now := common.GetTimestamp()
	metadata := map[string]interface{}{
		"coupon_id":     binding.CouponId,
		"redemption_id": redemptionId,
		"binding_id":    binding.Id,
	}
	metadataJSON, _ := json.Marshal(metadata)
	metadataStr := string(metadataJSON)

	auditLog := &AuditLog{
		OperatorId: &operatorId,
		Action:     "coupon_unbind",
		ObjectType: "coupon_redemption_binding",
		ObjectId:   &binding.Id,
		Metadata:   metadataStr,
		CreatedAt:  now,
	}

	return tx.Create(auditLog).Error
}

// ===================== 辅助方法 =====================

// validateCouponRedemptionBinding 验证绑定记录数据
func validateCouponRedemptionBinding(binding *CouponRedemptionBinding) error {
	if binding.CouponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}

	if binding.RedemptionId == 0 {
		return errors.New("兑换码 ID 不能为空")
	}

	// 验证状态合法性
	validStatuses := map[string]bool{
		common.CouponBindingStatusReserved: true,
		common.CouponBindingStatusLocked:   true,
	}
	if binding.Status != "" && !validStatuses[binding.Status] {
		return errors.New("无效的绑定状态")
	}

	return nil
}

// IsReserved 检查绑定是否为预留状态
func (b *CouponRedemptionBinding) IsReserved() bool {
	return b.Status == common.CouponBindingStatusReserved
}

// IsLocked 检查绑定是否已锁定
func (b *CouponRedemptionBinding) IsLocked() bool {
	return b.Status == common.CouponBindingStatusLocked
}

// HasExclusiveUser 检查绑定是否指定专属用户
func (b *CouponRedemptionBinding) HasExclusiveUser() bool {
	return b.UserId != nil
}

// GetExclusiveUserId 获取专属用户ID
func (b *CouponRedemptionBinding) GetExclusiveUserId() *int64 {
	return b.UserId
}

// IsUserAllowed 检查用户是否允许使用此绑定
func (b *CouponRedemptionBinding) IsUserAllowed(userId int64) bool {
	// 如果未指定专属用户，所有用户都可使用
	if b.UserId == nil {
		return true
	}
	// 如果指定了专属用户，只有该用户可使用
	return *b.UserId == userId
}

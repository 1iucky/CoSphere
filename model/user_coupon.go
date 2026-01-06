package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// UserCoupon 用户优惠券实例
type UserCoupon struct {
	Id             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	CouponId       int64  `json:"coupon_id" gorm:"bigint;not null;index:idx_uc_coupon;uniqueIndex:uq_user_coupon,priority:2"`
	UserId         int64  `json:"user_id" gorm:"bigint;not null;index:idx_uc_user_status,priority:1;uniqueIndex:uq_user_coupon,priority:1"`
	Code           string `json:"code" gorm:"type:varchar(64);not null;uniqueIndex"` // 用户专属核销码 UC-UUID
	Status         string `json:"status" gorm:"type:varchar(32);not null;default:'available';index:idx_uc_user_status,priority:2"`
	ClaimedAt      int64  `json:"claimed_at" gorm:"bigint;not null"`
	UsedAt         *int64 `json:"used_at" gorm:"bigint"`
	OrderId        *int64 `json:"order_id" gorm:"bigint"`                // 核销关联订单
	DiscountAmount int64  `json:"discount_amount" gorm:"bigint;default:0"` // 优惠金额（分）
	FinalAmount    int64  `json:"final_amount" gorm:"bigint;default:0"`    // 最终支付金额（分）
	CreatedAt      int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint"`
}

func (UserCoupon) TableName() string {
	return "user_coupons"
}

// ===================== 用户优惠券 CRUD 方法 =====================

// CreateUserCoupon 创建用户优惠券
func CreateUserCoupon(userCoupon *UserCoupon) error {
	if userCoupon.CouponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userCoupon.UserId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if userCoupon.Code == "" {
		return errors.New("用户优惠券核销码不能为空")
	}

	now := common.GetTimestamp()
	if userCoupon.ClaimedAt == 0 {
		userCoupon.ClaimedAt = now
	}
	if userCoupon.CreatedAt == 0 {
		userCoupon.CreatedAt = now
	}
	if userCoupon.UpdatedAt == 0 {
		userCoupon.UpdatedAt = now
	}

	return DB.Create(userCoupon).Error
}

// CreateUserCouponWithTx 在事务中创建用户优惠券
func CreateUserCouponWithTx(tx *gorm.DB, userCoupon *UserCoupon) error {
	if userCoupon.CouponId == 0 {
		return errors.New("优惠券 ID 不能为空")
	}
	if userCoupon.UserId == 0 {
		return errors.New("用户 ID 不能为空")
	}
	if userCoupon.Code == "" {
		return errors.New("用户优惠券核销码不能为空")
	}

	now := common.GetTimestamp()
	if userCoupon.ClaimedAt == 0 {
		userCoupon.ClaimedAt = now
	}
	if userCoupon.CreatedAt == 0 {
		userCoupon.CreatedAt = now
	}
	if userCoupon.UpdatedAt == 0 {
		userCoupon.UpdatedAt = now
	}

	return tx.Create(userCoupon).Error
}

// GetUserCouponById 根据 ID 获取用户优惠券
func GetUserCouponById(id int64) (*UserCoupon, error) {
	if id == 0 {
		return nil, errors.New("用户优惠券 ID 不能为空")
	}

	var userCoupon UserCoupon
	err := DB.First(&userCoupon, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &userCoupon, nil
}

// GetUserCouponByIdWithTx 在事务中根据 ID 获取用户优惠券（带行锁）
func GetUserCouponByIdWithTx(tx *gorm.DB, id int64, forUpdate bool) (*UserCoupon, error) {
	if id == 0 {
		return nil, errors.New("用户优惠券 ID 不能为空")
	}

	var userCoupon UserCoupon
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.First(&userCoupon, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &userCoupon, nil
}

// GetUserCouponByCode 根据用户券核销码查询
func GetUserCouponByCode(code string) (*UserCoupon, error) {
	if code == "" {
		return nil, errors.New("用户优惠券核销码不能为空")
	}

	var userCoupon UserCoupon
	err := DB.Where("code = ?", code).First(&userCoupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	return &userCoupon, nil
}

// GetUserCouponByUserAndCoupon 根据用户ID和优惠券ID查询
func GetUserCouponByUserAndCoupon(userId int64, couponId int64) (*UserCoupon, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var userCoupon UserCoupon
	err := DB.Where("user_id = ? AND coupon_id = ?", userId, couponId).First(&userCoupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil 而不是错误
		}
		return nil, err
	}

	return &userCoupon, nil
}

// GetUserCouponByUserAndCouponWithTx 在事务中根据用户ID和优惠券ID查询
func GetUserCouponByUserAndCouponWithTx(tx *gorm.DB, userId int64, couponId int64) (*UserCoupon, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var userCoupon UserCoupon
	err := tx.Where("user_id = ? AND coupon_id = ?", userId, couponId).First(&userCoupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到返回 nil 而不是错误
		}
		return nil, err
	}

	return &userCoupon, nil
}

// GetUserCouponsByUser 获取用户的优惠券列表
func GetUserCouponsByUser(userId int64, status string, startIdx int, num int) (userCoupons []*UserCoupon, total int64, err error) {
	if userId == 0 {
		return nil, 0, errors.New("用户 ID 不能为空")
	}

	query := DB.Model(&UserCoupon{}).Where("user_id = ?", userId)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("claimed_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&userCoupons).Error
	if err != nil {
		return nil, 0, err
	}

	return userCoupons, total, nil
}

// UpdateUserCouponStatus 更新用户优惠券状态
func UpdateUserCouponStatus(id int64, status string) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}
	if status == "" {
		return errors.New("状态不能为空")
	}

	// 验证状态合法性
	validStatuses := map[string]bool{
		common.UserCouponStatusAvailable: true,
		common.UserCouponStatusLocked:    true,
		common.UserCouponStatusUsed:      true,
		common.UserCouponStatusExpired:   true,
		common.UserCouponStatusInvalid:   true,
	}
	if !validStatuses[status] {
		return errors.New("无效的用户优惠券状态")
	}

	now := common.GetTimestamp()
	return DB.Model(&UserCoupon{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		}).Error
}

// UpdateUserCouponStatusWithTx 在事务中更新用户优惠券状态
func UpdateUserCouponStatusWithTx(tx *gorm.DB, id int64, status string) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}
	if status == "" {
		return errors.New("状态不能为空")
	}

	// 验证状态合法性
	validStatuses := map[string]bool{
		common.UserCouponStatusAvailable: true,
		common.UserCouponStatusLocked:    true,
		common.UserCouponStatusUsed:      true,
		common.UserCouponStatusExpired:   true,
		common.UserCouponStatusInvalid:   true,
	}
	if !validStatuses[status] {
		return errors.New("无效的用户优惠券状态")
	}

	now := common.GetTimestamp()
	return tx.Model(&UserCoupon{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": now,
		}).Error
}

// ===================== 领券方法（包含乐观锁） =====================

// ClaimCoupon 领取优惠券（乐观锁防止超发）
func ClaimCoupon(userId int64, claimCode string) (*UserCoupon, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}
	if claimCode == "" {
		return nil, errors.New("领取码不能为空")
	}

	var userCoupon *UserCoupon

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1. 查询优惠券模板并加载版本号（乐观锁）
		var coupon Coupon
		err := tx.Where("code = ? AND status = ?", claimCode, common.CouponStatusActive).
			First(&coupon).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgCouponNotFound)
			}
			return err
		}

		// 2. 修复 2.16：检查优惠券是否绑定到兑换码（绑定券不可通过普通领取流程使用）
		// 绑定关系存储在 coupon_redemption_bindings 表
		isBound, err := IsCouponBoundToRedemptionWithTx(tx, coupon.Id)
		if err != nil {
			return fmt.Errorf("检查优惠券绑定状态失败: %w", err)
		}
		if isBound {
			return errors.New("该优惠券仅限随兑换码一起使用")
		}

		// 3. 校验时间、库存、状态
		now := common.GetTimestamp()
		if now < coupon.ValidFrom {
			return errors.New("优惠券尚未生效")
		}
		if now > coupon.ValidTo {
			return errors.New(common.MsgCouponExpired)
		}
		// 检查库存
		if coupon.UsedCount >= coupon.TotalCount {
			return errors.New("优惠券库存不足")
		}

		// 4. 校验用户领取限制（同一用户同一优惠券只能领取一次）
		var count int64
		err = tx.Model(&UserCoupon{}).
			Where("user_id = ? AND coupon_id = ?", userId, coupon.Id).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count > 0 {
			return errors.New("您已领取过该优惠券")
		}

		// 5. 乐观锁更新库存（并发安全）
		// 需要同时检查 used_count < total_count
		result := tx.Model(&Coupon{}).
			Where("id = ? AND version = ? AND used_count < total_count",
				coupon.Id, coupon.Version).
			Updates(map[string]interface{}{
				"used_count": gorm.Expr("used_count + 1"),
				"version":    gorm.Expr("version + 1"),
			})

		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return errors.New("优惠券库存不足")
		}

		// 6. 插入用户优惠券实例
		userCoupon = &UserCoupon{
			CouponId:  coupon.Id,
			UserId:    userId,
			Code:      "UC-" + common.GetUUID(), // UC- + UUID 格式
			Status:    common.UserCouponStatusAvailable,
			ClaimedAt: now,
			CreatedAt: now,
			UpdatedAt: now,
		}
		err = tx.Create(userCoupon).Error
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return userCoupon, nil
}

// ===================== 核销方法（包含幂等性检查） =====================

// CouponUsageResult 优惠券核销结果
type CouponUsageResult struct {
	DiscountAmount int64 // 优惠金额（分）
	FinalAmount    int64 // 最终应付金额（分）
	IsIdempotent   bool  // 是否为幂等返回（相同订单号重复核销时为 true）
}

// UseCoupon 核销优惠券（幂等性保证）
func UseCoupon(userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string) (*CouponUsageResult, error) {
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

	var result *CouponUsageResult

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 1. 获取用户优惠券并加行锁（FOR UPDATE）
		var userCoupon UserCoupon
		err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND user_id = ?", userCouponId, userId).
			First(&userCoupon).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgCouponNotFound)
			}
			return err
		}

		// 2. 幂等性检查：如果已使用且订单号相同，直接返回成功
		if userCoupon.Status == common.UserCouponStatusUsed {
			if userCoupon.OrderId != nil && *userCoupon.OrderId == orderId {
				// 幂等返回，已成功核销过
				result = &CouponUsageResult{
					DiscountAmount: userCoupon.DiscountAmount,
					FinalAmount:    userCoupon.FinalAmount,
					IsIdempotent:   true, // 标记为幂等返回
				}
				return nil
			}
			return errors.New("优惠券已使用")
		}

		// 3. 状态校验：必须为 available
		if userCoupon.Status != common.UserCouponStatusAvailable {
			return errors.New("优惠券状态无效")
		}

		// 4. 获取优惠券模板信息
		var coupon Coupon
		err = tx.Where("id = ?", userCoupon.CouponId).First(&coupon).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New(common.MsgCouponNotFound)
			}
			return err
		}

		// 4.1 绑定券校验：绑定到兑换码的优惠券只能通过兑换码流程核销
		isBound, err := IsCouponBoundToRedemptionWithTx(tx, coupon.Id)
		if err != nil {
			return fmt.Errorf("检查优惠券绑定状态失败: %w", err)
		}
		if isBound {
			return errors.New("该优惠券仅限随兑换码一起使用")
		}

		// 5. 时间校验
		now := common.GetTimestamp()
		if now < coupon.ValidFrom {
			return errors.New("优惠券尚未生效")
		}
		if now > coupon.ValidTo {
			return errors.New(common.MsgCouponExpired)
		}

		// 6. 作用域校验（scope 与场景匹配）
		if !isScopeMatch(coupon.Scope, scene) {
			return errors.New("优惠券不适用于当前场景")
		}

		// 7. 币种校验（当前固定 CNY）
		if coupon.Currency != "CNY" {
			return errors.New("优惠券币种与订单不匹配")
		}

		// 8. 计算优惠金额（基于订单原价 orderAmount）
		var discountAmount int64
		switch coupon.Type {
		case common.CouponTypeDiscount: // 折扣券（百分比）
			discountAmount = orderAmount * coupon.DiscountValue / 100
		case common.CouponTypeFullReduction: // 满减券
			if orderAmount >= coupon.ThresholdAmount {
				discountAmount = coupon.DiscountValue
			} else {
				return errors.New("订单金额未达满减阈值")
			}
		case common.CouponTypeInstantReduction: // 立减券
			discountAmount = min(coupon.DiscountValue, orderAmount)
		default:
			return errors.New("优惠券类型无效")
		}

		// 9. 计算最终应付金额（不允许为负）
		finalAmount := max(0, orderAmount-discountAmount)

		// 10. 原子更新用户优惠券状态为 used（条件更新防止并发）
		now = common.GetTimestamp()
		updateResult := tx.Model(&UserCoupon{}).
			Where("id = ? AND status = ?", userCouponId, common.UserCouponStatusAvailable).
			Updates(map[string]interface{}{
				"status":          common.UserCouponStatusUsed,
				"used_at":         now,
				"order_id":        orderId,
				"discount_amount": discountAmount,
				"final_amount":    finalAmount,
				"updated_at":      now,
			})

		if updateResult.Error != nil {
			return updateResult.Error
		}

		if updateResult.RowsAffected == 0 {
			return errors.New("优惠券并发冲突")
		}

		// 11. 写入 user_bills 账单记录
		sourceType := common.BillSourceTypeCouponUsage
		description := "优惠券核销"
		bill := &UserBill{
			UserId:         userId,
			BillType:       common.BillTypeCouponDiscount,
			Amount:         -discountAmount, // 优惠金额为负数（表示用户获得折扣）
			BalanceBefore:  0,               // 优惠券核销不影响余额
			BalanceAfter:   0,
			SourceType:     &sourceType,
			SourceId:       &orderId,
			CouponId:       &coupon.Id,
			UserCouponId:   &userCouponId,
			DiscountAmount: discountAmount,
			OriginalAmount: orderAmount,
			FinalAmount:    finalAmount,
			Description:    &description,
			CreatedAt:      now,
		}
		if err := tx.Create(bill).Error; err != nil {
			return err
		}

		result = &CouponUsageResult{
			DiscountAmount: discountAmount,
			FinalAmount:    finalAmount,
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

// UseCouponWithTx 在事务中核销优惠券（幂等性保证）
// 用于需要在外部事务中执行优惠券核销的场景
func UseCouponWithTx(tx *gorm.DB, userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string) (*CouponUsageResult, error) {
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

	// 1. 获取用户优惠券并加行锁（FOR UPDATE）
	var userCoupon UserCoupon
	err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ? AND user_id = ?", userCouponId, userId).
		First(&userCoupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	// 2. 幂等性检查：如果已使用且订单号相同，直接返回成功
	if userCoupon.Status == common.UserCouponStatusUsed {
		if userCoupon.OrderId != nil && *userCoupon.OrderId == orderId {
			// 幂等返回，已成功核销过
			return &CouponUsageResult{
				DiscountAmount: userCoupon.DiscountAmount,
				FinalAmount:    userCoupon.FinalAmount,
				IsIdempotent:   true, // 标记为幂等返回
			}, nil
		}
		return nil, errors.New("优惠券已使用")
	}

	// 3. 状态校验：必须为 available 或 locked
	if userCoupon.Status != common.UserCouponStatusAvailable && userCoupon.Status != common.UserCouponStatusLocked {
		return nil, errors.New("优惠券状态无效")
	}

	// 4. 获取优惠券模板信息
	var coupon Coupon
	err = tx.Where("id = ?", userCoupon.CouponId).First(&coupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	// 4.1 绑定券校验：绑定到兑换码的优惠券只能通过兑换码流程核销
	// 注意：如果优惠券已是 locked 状态，说明是通过兑换码流程锁定的，允许继续核销
	if userCoupon.Status != common.UserCouponStatusLocked {
		isBound, err := IsCouponBoundToRedemptionWithTx(tx, coupon.Id)
		if err != nil {
			return nil, fmt.Errorf("检查优惠券绑定状态失败: %w", err)
		}
		if isBound {
			return nil, errors.New("该优惠券仅限随兑换码一起使用")
		}
	}

	// 5. 时间校验
	now := common.GetTimestamp()
	if now < coupon.ValidFrom {
		return nil, errors.New("优惠券尚未生效")
	}
	if now > coupon.ValidTo {
		return nil, errors.New(common.MsgCouponExpired)
	}

	// 6. 作用域校验（scope 与场景匹配）
	if !isScopeMatch(coupon.Scope, scene) {
		return nil, errors.New("优惠券不适用于当前场景")
	}

	// 7. 币种校验（当前固定 CNY）
	if coupon.Currency != "CNY" {
		return nil, errors.New("优惠券币种与订单不匹配")
	}

	// 8. 计算优惠金额（基于订单原价 orderAmount）
	var discountAmount int64
	switch coupon.Type {
	case common.CouponTypeDiscount: // 折扣券（百分比）
		discountAmount = orderAmount * coupon.DiscountValue / 100
	case common.CouponTypeFullReduction: // 满减券
		if orderAmount >= coupon.ThresholdAmount {
			discountAmount = coupon.DiscountValue
		} else {
			return nil, errors.New("订单金额未达满减阈值")
		}
	case common.CouponTypeInstantReduction: // 立减券
		discountAmount = min(coupon.DiscountValue, orderAmount)
	default:
		return nil, errors.New("优惠券类型无效")
	}

	// 9. 计算最终应付金额（不允许为负）
	finalAmount := max(0, orderAmount-discountAmount)

	// 10. 原子更新用户优惠券状态为 used（条件更新防止并发）
	now = common.GetTimestamp()
	updateResult := tx.Model(&UserCoupon{}).
		Where("id = ? AND status IN ?", userCouponId, []string{common.UserCouponStatusAvailable, common.UserCouponStatusLocked}).
		Updates(map[string]interface{}{
			"status":          common.UserCouponStatusUsed,
			"used_at":         now,
			"order_id":        orderId,
			"discount_amount": discountAmount,
			"final_amount":    finalAmount,
			"updated_at":      now,
		})

	if updateResult.Error != nil {
		return nil, updateResult.Error
	}

	if updateResult.RowsAffected == 0 {
		return nil, errors.New("优惠券并发冲突")
	}

	// 11. 写入 user_bills 账单记录
	sourceType := common.BillSourceTypeCouponUsage
	description := "优惠券核销"
	bill := &UserBill{
		UserId:         userId,
		BillType:       common.BillTypeCouponDiscount,
		Amount:         -discountAmount, // 优惠金额为负数（表示用户获得折扣）
		BalanceBefore:  0,               // 优惠券核销不影响余额
		BalanceAfter:   0,
		SourceType:     &sourceType,
		SourceId:       &orderId,
		CouponId:       &coupon.Id,
		UserCouponId:   &userCouponId,
		DiscountAmount: discountAmount,
		OriginalAmount: orderAmount,
		FinalAmount:    finalAmount,
		Description:    &description,
		CreatedAt:      now,
	}
	if err := tx.Create(bill).Error; err != nil {
		return nil, err
	}

	return &CouponUsageResult{
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
	}, nil
}

// UseCouponWithPlanTx 在事务中核销优惠券（带套餐校验）
// 与 UseCouponWithTx 类似，但增加了对 applicable_plan_ids 的校验
// planId 为 nil 时跳过套餐校验
func UseCouponWithPlanTx(tx *gorm.DB, userId int64, userCouponId int64, orderId int64, orderAmount int64, scene string, planId *int64) (*CouponUsageResult, error) {
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

	// 1. 获取用户优惠券并加行锁（FOR UPDATE）
	var userCoupon UserCoupon
	err := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ? AND user_id = ?", userCouponId, userId).
		First(&userCoupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	// 2. 幂等性检查：如果已使用且订单号相同，直接返回成功
	if userCoupon.Status == common.UserCouponStatusUsed {
		if userCoupon.OrderId != nil && *userCoupon.OrderId == orderId {
			return &CouponUsageResult{
				DiscountAmount: userCoupon.DiscountAmount,
				FinalAmount:    userCoupon.FinalAmount,
				IsIdempotent:   true, // 标记为幂等返回
			}, nil
		}
		return nil, errors.New("优惠券已使用")
	}

	// 3. 状态校验：必须为 available 或 locked
	if userCoupon.Status != common.UserCouponStatusAvailable && userCoupon.Status != common.UserCouponStatusLocked {
		return nil, errors.New("优惠券状态无效")
	}

	// 4. 获取优惠券模板信息
	var coupon Coupon
	err = tx.Where("id = ?", userCoupon.CouponId).First(&coupon).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgCouponNotFound)
		}
		return nil, err
	}

	// 4.1 修复 2.16：检查优惠券是否绑定到兑换码
	// 绑定券只能通过 UseBoundCouponWithPlanTx（兑换流程）使用，不能通过普通核销流程使用
	// 但如果用户优惠券状态为 locked（已被兑换流程锁定），则允许继续核销
	// 绑定关系存储在 coupon_redemption_bindings 表
	if userCoupon.Status != common.UserCouponStatusLocked {
		isBound, err := IsCouponBoundToRedemptionWithTx(tx, coupon.Id)
		if err != nil {
			return nil, fmt.Errorf("检查优惠券绑定状态失败: %w", err)
		}
		if isBound {
			return nil, errors.New("该优惠券仅限随兑换码一起使用")
		}
	}

	// 5. 时间校验
	now := common.GetTimestamp()
	if now < coupon.ValidFrom {
		return nil, errors.New("优惠券尚未生效")
	}
	if now > coupon.ValidTo {
		return nil, errors.New(common.MsgCouponExpired)
	}

	// 6. 作用域校验（scope 与场景匹配）
	if !isScopeMatch(coupon.Scope, scene) {
		return nil, errors.New("优惠券不适用于当前场景")
	}

	// 7. 适用套餐校验（仅当 planId 非空时）
	if planId != nil {
		applicable, err := coupon.IsApplicableToPlan(*planId)
		if err != nil {
			return nil, err
		}
		if !applicable {
			return nil, errors.New(common.MsgCouponPlanMismatch)
		}
	}

	// 8. 币种校验（当前固定 CNY）
	if coupon.Currency != "CNY" {
		return nil, errors.New("优惠券币种与订单不匹配")
	}

	// 9. 计算优惠金额（基于订单原价 orderAmount）
	var discountAmount int64
	switch coupon.Type {
	case common.CouponTypeDiscount: // 折扣券（百分比）
		discountAmount = orderAmount * coupon.DiscountValue / 100
	case common.CouponTypeFullReduction: // 满减券
		if orderAmount >= coupon.ThresholdAmount {
			discountAmount = coupon.DiscountValue
		} else {
			return nil, errors.New("订单金额未达满减阈值")
		}
	case common.CouponTypeInstantReduction: // 立减券
		discountAmount = min(coupon.DiscountValue, orderAmount)
	default:
		return nil, errors.New("优惠券类型无效")
	}

	// 10. 计算最终应付金额（不允许为负）
	finalAmount := max(0, orderAmount-discountAmount)

	// 11. 原子更新用户优惠券状态为 used（条件更新防止并发）
	now = common.GetTimestamp()
	updateResult := tx.Model(&UserCoupon{}).
		Where("id = ? AND status IN ?", userCouponId, []string{common.UserCouponStatusAvailable, common.UserCouponStatusLocked}).
		Updates(map[string]interface{}{
			"status":          common.UserCouponStatusUsed,
			"used_at":         now,
			"order_id":        orderId,
			"discount_amount": discountAmount,
			"final_amount":    finalAmount,
			"updated_at":      now,
		})

	if updateResult.Error != nil {
		return nil, updateResult.Error
	}

	if updateResult.RowsAffected == 0 {
		return nil, errors.New("优惠券并发冲突")
	}

	// 12. 写入 user_bills 账单记录
	sourceType := common.BillSourceTypeCouponUsage
	description := "优惠券核销"
	bill := &UserBill{
		UserId:         userId,
		BillType:       common.BillTypeCouponDiscount,
		Amount:         -discountAmount,
		BalanceBefore:  0,
		BalanceAfter:   0,
		SourceType:     &sourceType,
		SourceId:       &orderId,
		CouponId:       &coupon.Id,
		UserCouponId:   &userCouponId,
		DiscountAmount: discountAmount,
		OriginalAmount: orderAmount,
		FinalAmount:    finalAmount,
		Description:    &description,
		CreatedAt:      now,
	}
	if err := tx.Create(bill).Error; err != nil {
		return nil, err
	}

	return &CouponUsageResult{
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
	}, nil
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

// min 返回两个数的最小值
func min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// max 返回两个数的最大值
func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// ===================== 状态转换方法 =====================

// LockUserCoupon 锁定用户优惠券
func LockUserCoupon(id int64) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}

	now := common.GetTimestamp()
	result := DB.Model(&UserCoupon{}).
		Where("id = ? AND status = ?", id, common.UserCouponStatusAvailable).
		Updates(map[string]interface{}{
			"status":     common.UserCouponStatusLocked,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券状态无效或已被使用")
	}

	return nil
}

// LockUserCouponWithTx 在事务中锁定用户优惠券
func LockUserCouponWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}

	now := common.GetTimestamp()
	result := tx.Model(&UserCoupon{}).
		Where("id = ? AND status = ?", id, common.UserCouponStatusAvailable).
		Updates(map[string]interface{}{
			"status":     common.UserCouponStatusLocked,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券状态无效或已被使用")
	}

	return nil
}

// UseUserCoupon 将用户优惠券标记为已使用
func UseUserCoupon(id int64, orderId int64, discountAmount int64, finalAmount int64) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}
	if orderId == 0 {
		return errors.New("订单 ID 不能为空")
	}

	now := common.GetTimestamp()
	result := DB.Model(&UserCoupon{}).
		Where("id = ? AND status IN ?", id, []string{common.UserCouponStatusAvailable, common.UserCouponStatusLocked}).
		Updates(map[string]interface{}{
			"status":          common.UserCouponStatusUsed,
			"used_at":         now,
			"order_id":        orderId,
			"discount_amount": discountAmount,
			"final_amount":    finalAmount,
			"updated_at":      now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券状态无效或已被使用")
	}

	return nil
}

// UseUserCouponWithTx 在事务中将用户优惠券标记为已使用
func UseUserCouponWithTx(tx *gorm.DB, id int64, orderId int64, discountAmount int64, finalAmount int64) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}
	if orderId == 0 {
		return errors.New("订单 ID 不能为空")
	}

	now := common.GetTimestamp()
	result := tx.Model(&UserCoupon{}).
		Where("id = ? AND status IN ?", id, []string{common.UserCouponStatusAvailable, common.UserCouponStatusLocked}).
		Updates(map[string]interface{}{
			"status":          common.UserCouponStatusUsed,
			"used_at":         now,
			"order_id":        orderId,
			"discount_amount": discountAmount,
			"final_amount":    finalAmount,
			"updated_at":      now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券状态无效或已被使用")
	}

	return nil
}

// ExpireUserCoupon 将用户优惠券标记为已过期
func ExpireUserCoupon(id int64) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}

	now := common.GetTimestamp()
	result := DB.Model(&UserCoupon{}).
		Where("id = ? AND status = ?", id, common.UserCouponStatusAvailable).
		Updates(map[string]interface{}{
			"status":     common.UserCouponStatusExpired,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券状态无效或已被使用")
	}

	return nil
}

// InvalidateUserCoupon 作废用户优惠券
func InvalidateUserCoupon(id int64) error {
	if id == 0 {
		return errors.New("用户优惠券 ID 不能为空")
	}

	now := common.GetTimestamp()
	result := DB.Model(&UserCoupon{}).
		Where("id = ? AND status IN ?", id, []string{common.UserCouponStatusAvailable, common.UserCouponStatusLocked}).
		Updates(map[string]interface{}{
			"status":     common.UserCouponStatusInvalid,
			"updated_at": now,
		})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("优惠券状态无效或已被使用")
	}

	return nil
}

// ===================== 批量过期处理 =====================

// ExpireUserCouponsInBatch 批量标记过期优惠券
func ExpireUserCouponsInBatch(limit int) (int64, error) {
	now := common.GetTimestamp()

	// 查询需要过期的用户优惠券 ID
	var expiredIds []int64
	err := DB.Model(&UserCoupon{}).
		Select("user_coupons.id").
		Joins("INNER JOIN coupons ON user_coupons.coupon_id = coupons.id").
		Where("user_coupons.status = ?", common.UserCouponStatusAvailable).
		Where("coupons.valid_to < ?", now).
		Limit(limit).
		Pluck("user_coupons.id", &expiredIds).Error

	if err != nil {
		return 0, err
	}

	if len(expiredIds) == 0 {
		return 0, nil
	}

	// 批量更新状态
	result := DB.Model(&UserCoupon{}).
		Where("id IN ?", expiredIds).
		Updates(map[string]interface{}{
			"status":     common.UserCouponStatusExpired,
			"updated_at": now,
		})

	return result.RowsAffected, result.Error
}

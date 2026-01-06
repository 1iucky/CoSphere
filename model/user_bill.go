package model

import (
	"encoding/json"
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// UserBill 用户账单流水表
type UserBill struct {
	Id                 int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId             int64   `json:"user_id" gorm:"bigint;not null;index:idx_user_bills_user_id_created_at,priority:1"`
	BillType           string  `json:"bill_type" gorm:"type:varchar(64);not null"`
	Amount             int64   `json:"amount" gorm:"bigint;not null"`
	BalanceBefore      int64   `json:"balance_before" gorm:"bigint;not null"`
	BalanceAfter       int64   `json:"balance_after" gorm:"bigint;not null"`
	SourceType         *string `json:"source_type" gorm:"type:varchar(64);index:idx_user_bills_source,priority:1"`
	SourceId           *int64  `json:"source_id" gorm:"bigint;index:idx_user_bills_source,priority:2"`
	PaymentChannel     *string `json:"payment_channel" gorm:"type:varchar(64)"`
	TradeNo            *string `json:"trade_no" gorm:"type:varchar(255);index"`
	Description        *string `json:"description" gorm:"type:text"`
	// 优惠券相关字段
	CouponId           *int64  `json:"coupon_id" gorm:"bigint;index:idx_bill_coupon"`         // 使用的优惠券模板ID
	UserCouponId       *int64  `json:"user_coupon_id" gorm:"bigint;index:idx_bill_user_coupon"` // 使用的用户优惠券ID
	DiscountAmount     int64   `json:"discount_amount" gorm:"bigint;default:0"`               // 优惠金额（分）
	OriginalAmount     int64   `json:"original_amount" gorm:"bigint;default:0"`               // 原价（分）
	FinalAmount        int64   `json:"final_amount" gorm:"bigint;default:0"`                  // 实付金额（分）
	// 退款和兑换元数据
	RefundType         *string `json:"refund_type" gorm:"type:varchar(32)"`
	ConversionMetadata *string `json:"conversion_metadata" gorm:"type:text"`
	Metadata           *string `json:"metadata" gorm:"type:text"`
	OperatorId         *int64  `json:"operator_id" gorm:"bigint"`
	CreatedAt          int64   `json:"created_at" gorm:"bigint;not null;index:idx_user_bills_user_id_created_at,priority:2"`
}

func (UserBill) TableName() string {
	return "user_bills"
}

// ===================== 账单 CRUD 方法 =====================

// CreateUserBill 创建账单记录
func CreateUserBill(bill *UserBill) error {
	if err := validateUserBill(bill); err != nil {
		return err
	}

	if bill.CreatedAt == 0 {
		bill.CreatedAt = common.GetTimestamp()
	}

	return DB.Create(bill).Error
}

// CreateUserBillWithTx 在事务中创建账单记录
func CreateUserBillWithTx(tx *gorm.DB, bill *UserBill) error {
	if err := validateUserBill(bill); err != nil {
		return err
	}

	if bill.CreatedAt == 0 {
		bill.CreatedAt = common.GetTimestamp()
	}

	return tx.Create(bill).Error
}

// GetUserBillById 根据 ID 获取账单
func GetUserBillById(id int64) (*UserBill, error) {
	if id == 0 {
		return nil, errors.New("账单 ID 不能为空")
	}

	var bill UserBill
	err := DB.First(&bill, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgBillNotFound)
		}
		return nil, err
	}

	return &bill, nil
}

// GetUserBillsByUser 获取用户的账单列表（分页）
func GetUserBillsByUser(userId int64, startIdx int, num int, billType string) (bills []*UserBill, total int64, err error) {
	if userId == 0 {
		return nil, 0, errors.New("用户 ID 不能为空")
	}

	query := DB.Model(&UserBill{}).Where("user_id = ?", userId)

	if billType != "" {
		query = query.Where("bill_type = ?", billType)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据（按创建时间倒序）
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&bills).Error
	if err != nil {
		return nil, 0, err
	}

	return bills, total, nil
}

// GetAllUserBills 获取所有账单（管理员，分页）
func GetAllUserBills(startIdx int, num int, billType string) (bills []*UserBill, total int64, err error) {
	query := DB.Model(&UserBill{})

	if billType != "" {
		query = query.Where("bill_type = ?", billType)
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
		Find(&bills).Error
	if err != nil {
		return nil, 0, err
	}

	return bills, total, nil
}

// SearchUserBills 搜索账单
func SearchUserBills(userId int64, billType string, sourceType string, startTime int64, endTime int64, startIdx int, num int) (bills []*UserBill, total int64, err error) {
	query := DB.Model(&UserBill{})

	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if billType != "" {
		query = query.Where("bill_type = ?", billType)
	}
	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if startTime > 0 {
		query = query.Where("created_at >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("created_at <= ?", endTime)
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
		Find(&bills).Error
	if err != nil {
		return nil, 0, err
	}

	return bills, total, nil
}

// GetBillsBySource 根据来源获取账单
func GetBillsBySource(sourceType string, sourceId int64) ([]*UserBill, error) {
	if sourceType == "" || sourceId == 0 {
		return nil, errors.New("来源类型和来源 ID 不能为空")
	}

	var bills []*UserBill
	err := DB.Where("source_type = ? AND source_id = ?", sourceType, sourceId).
		Order("created_at desc").
		Find(&bills).Error

	if err != nil {
		return nil, err
	}

	return bills, nil
}

// GetBillByTradeNo 根据交易号获取账单
func GetBillByTradeNo(tradeNo string) (*UserBill, error) {
	if tradeNo == "" {
		return nil, errors.New("交易号不能为空")
	}

	var bill UserBill
	err := DB.Where("trade_no = ?", tradeNo).First(&bill).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &bill, nil
}

// ===================== 账单创建便捷方法 =====================

// CreateSubscriptionBill 创建订阅相关账单
func CreateSubscriptionBill(userId int64, billType string, amount int64, balanceBefore int64, orderId int64, paymentChannel string, description string) (*UserBill, error) {
	sourceType := common.BillSourceTypeSubscriptionOrder
	bill := &UserBill{
		UserId:         userId,
		BillType:       billType,
		Amount:         amount,
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceBefore - amount,
		SourceType:     &sourceType,
		SourceId:       &orderId,
		PaymentChannel: &paymentChannel,
		Description:    &description,
		CreatedAt:      common.GetTimestamp(),
	}

	err := CreateUserBill(bill)
	if err != nil {
		return nil, err
	}

	return bill, nil
}

// CreateSubscriptionBillWithTx 在事务中创建订阅账单
func CreateSubscriptionBillWithTx(tx *gorm.DB, userId int64, billType string, amount int64, balanceBefore int64, orderId int64, paymentChannel string, description string) (*UserBill, error) {
	sourceType := common.BillSourceTypeSubscriptionOrder
	bill := &UserBill{
		UserId:         userId,
		BillType:       billType,
		Amount:         amount,
		BalanceBefore:  balanceBefore,
		BalanceAfter:   balanceBefore - amount,
		SourceType:     &sourceType,
		SourceId:       &orderId,
		PaymentChannel: &paymentChannel,
		Description:    &description,
		CreatedAt:      common.GetTimestamp(),
	}

	err := CreateUserBillWithTx(tx, bill)
	if err != nil {
		return nil, err
	}

	return bill, nil
}

// CreateRefundBill 创建退款账单
func CreateRefundBill(userId int64, amount int64, balanceBefore int64, sourceType string, sourceId int64, refundType string, description string, operatorId *int64) (*UserBill, error) {
	bill := &UserBill{
		UserId:        userId,
		BillType:      common.BillTypeRefund,
		Amount:        -amount, // 退款为负数
		BalanceBefore: balanceBefore,
		BalanceAfter:  balanceBefore + amount,
		SourceType:    &sourceType,
		SourceId:      &sourceId,
		RefundType:    &refundType,
		Description:   &description,
		OperatorId:    operatorId,
		CreatedAt:     common.GetTimestamp(),
	}

	err := CreateUserBill(bill)
	if err != nil {
		return nil, err
	}

	return bill, nil
}

// CreateRefundBillWithTx 在事务中创建退款账单
func CreateRefundBillWithTx(tx *gorm.DB, userId int64, amount int64, balanceBefore int64, sourceType string, sourceId int64, refundType string, description string, operatorId *int64) (*UserBill, error) {
	bill := &UserBill{
		UserId:        userId,
		BillType:      common.BillTypeRefund,
		Amount:        -amount,
		BalanceBefore: balanceBefore,
		BalanceAfter:  balanceBefore + amount,
		SourceType:    &sourceType,
		SourceId:      &sourceId,
		RefundType:    &refundType,
		Description:   &description,
		OperatorId:    operatorId,
		CreatedAt:     common.GetTimestamp(),
	}

	err := CreateUserBillWithTx(tx, bill)
	if err != nil {
		return nil, err
	}

	return bill, nil
}

// RefundBillParams 退款账单参数（用于完整记录退款元数据）
type RefundBillParams struct {
	UserId         int64
	Amount         int64   // 退款金额（分）- 仅用于记录，不影响余额
	BalanceBefore  int64   // 退款前余额（额度）
	SourceType     string  // 来源类型
	SourceId       int64   // 来源 ID（应为订单 ID）
	RefundType     string  // 退款类型（manual/auto）
	Description    string  // 退款描述/原因
	OperatorId     *int64  // 操作员 ID
	CouponId       *int64  // 优惠券 ID
	UserCouponId   *int64  // 用户优惠券 ID
	OriginalAmount int64   // 原价（分）- 订单原价，用于 metadata 记录
	DiscountAmount int64   // 优惠金额（分）- 订单优惠，用于 metadata 记录
	FinalAmount    int64   // 订单实付金额（分）- 仅用于 metadata，实际写入账单时会被 RefundAmount 覆盖
	RefundAmount   int64   // 本次退款金额（分）- 实际写入 user_bills.final_amount 的值
	CouponRestored bool    // 优惠券是否已恢复
}

// CreateRefundBillWithMetadataWithTx 在事务中创建退款账单（包含完整元数据）
// 注意：此方法仅记录退款流水，不实际增加用户余额
// Amount 字段设为 0，避免污染统计；退款金额通过 FinalAmount 和 metadata 记录
func CreateRefundBillWithMetadataWithTx(tx *gorm.DB, params *RefundBillParams) (*UserBill, error) {
	if params == nil {
		return nil, errors.New("退款参数不能为空")
	}

	// 构建退款元数据（包含完整的订单和优惠券信息）
	refundMetadata := map[string]interface{}{
		"refund_amount":   params.RefundAmount,   // 本次退款金额（分）
		"original_amount": params.OriginalAmount, // 原价（分）
		"discount_amount": params.DiscountAmount, // 优惠金额（分）
		"coupon_restored": params.CouponRestored,
		"is_manual":       params.RefundType == "manual",
		"note":            "仅记录退款流水，实际退款走线下处理",
	}
	// 添加优惠券信息（如有）
	if params.CouponId != nil {
		refundMetadata["coupon_id"] = *params.CouponId
	}
	if params.UserCouponId != nil {
		refundMetadata["user_coupon_id"] = *params.UserCouponId
	}
	metadataJSON, _ := json.Marshal(refundMetadata)
	metadataStr := string(metadataJSON)

	bill := &UserBill{
		UserId:         params.UserId,
		BillType:       common.BillTypeRefund,
		Amount:         0, // 不实际增加余额，Amount 设为 0 避免污染统计
		BalanceBefore:  params.BalanceBefore,
		BalanceAfter:   params.BalanceBefore, // 余额不变
		SourceType:     &params.SourceType,
		SourceId:       &params.SourceId,
		RefundType:     &params.RefundType,
		Description:    &params.Description,
		OperatorId:     params.OperatorId,
		CouponId:       params.CouponId,
		UserCouponId:   params.UserCouponId,
		OriginalAmount: params.OriginalAmount,
		DiscountAmount: params.DiscountAmount,
		FinalAmount:    params.RefundAmount, // 使用 FinalAmount 记录本次退款金额
		Metadata:       &metadataStr,
		CreatedAt:      common.GetTimestamp(),
	}

	err := CreateUserBillWithTx(tx, bill)
	if err != nil {
		return nil, err
	}

	return bill, nil
}

// ===================== 验证方法 =====================

// validateUserBill 验证账单数据
func validateUserBill(bill *UserBill) error {
	if bill.UserId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	if bill.BillType == "" {
		return errors.New("账单类型不能为空")
	}

	validBillTypes := map[string]bool{
		common.BillTypeSubscription:       true,
		common.BillTypeSubscriptionRenew:  true,
		common.BillTypeRefund:             true,
		common.BillTypeRecharge:           true,
		common.BillTypeConsume:            true,
		common.BillTypeAdjustment:         true,
		common.BillTypeCouponDiscount:     true,
	}
	if !validBillTypes[bill.BillType] {
		return errors.New("无效的账单类型")
	}

	return nil
}

// ===================== 统计方法 =====================

// GetUserBillStats 获取用户账单统计
func GetUserBillStats(userId int64) (map[string]int64, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	stats := make(map[string]int64)

	// 总支出
	var totalSpend int64
	err := DB.Model(&UserBill{}).
		Where("user_id = ? AND amount < 0", userId).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&totalSpend).Error
	if err != nil {
		return nil, err
	}
	stats["total_spend"] = totalSpend

	// 总充值
	var totalRecharge int64
	err = DB.Model(&UserBill{}).
		Where("user_id = ? AND bill_type = ?", userId, common.BillTypeRecharge).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalRecharge).Error
	if err != nil {
		return nil, err
	}
	stats["total_recharge"] = totalRecharge

	// 总退款（使用 final_amount 统计，因为人工退款的 amount=0，退款金额记录在 final_amount）
	// 注意：单位为分（cents），与其他配额单位字段不同
	var totalRefundCents int64
	err = DB.Model(&UserBill{}).
		Where("user_id = ? AND bill_type = ?", userId, common.BillTypeRefund).
		Select("COALESCE(SUM(final_amount), 0)").
		Scan(&totalRefundCents).Error
	if err != nil {
		return nil, err
	}
	stats["total_refund_cents"] = totalRefundCents

	// 订阅支出
	var subscriptionSpend int64
	err = DB.Model(&UserBill{}).
		Where("user_id = ? AND bill_type IN ?", userId, []string{common.BillTypeSubscription, common.BillTypeSubscriptionRenew}).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&subscriptionSpend).Error
	if err != nil {
		return nil, err
	}
	stats["subscription_spend"] = subscriptionSpend

	return stats, nil
}

// GetBillStatsByDateRange 获取日期范围内的账单统计
func GetBillStatsByDateRange(startTime int64, endTime int64) (map[string]int64, error) {
	stats := make(map[string]int64)

	// 总收入
	var totalIncome int64
	err := DB.Model(&UserBill{}).
		Where("created_at >= ? AND created_at <= ? AND amount > 0", startTime, endTime).
		Select("COALESCE(SUM(amount), 0)").
		Scan(&totalIncome).Error
	if err != nil {
		return nil, err
	}
	stats["total_income"] = totalIncome

	// 订阅收入
	var subscriptionIncome int64
	err = DB.Model(&UserBill{}).
		Where("created_at >= ? AND created_at <= ? AND bill_type IN ?", startTime, endTime, []string{common.BillTypeSubscription, common.BillTypeSubscriptionRenew}).
		Select("COALESCE(SUM(ABS(amount)), 0)").
		Scan(&subscriptionIncome).Error
	if err != nil {
		return nil, err
	}
	stats["subscription_income"] = subscriptionIncome

	// 退款金额（使用 final_amount 统计，因为人工退款的 amount=0，退款金额记录在 final_amount）
	// 注意：单位为分（cents），与其他配额单位字段不同
	var refundAmountCents int64
	err = DB.Model(&UserBill{}).
		Where("created_at >= ? AND created_at <= ? AND bill_type = ?", startTime, endTime, common.BillTypeRefund).
		Select("COALESCE(SUM(final_amount), 0)").
		Scan(&refundAmountCents).Error
	if err != nil {
		return nil, err
	}
	stats["refund_amount_cents"] = refundAmountCents

	// 账单数量
	var billCount int64
	err = DB.Model(&UserBill{}).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Count(&billCount).Error
	if err != nil {
		return nil, err
	}
	stats["bill_count"] = billCount

	return stats, nil
}

// ===================== 辅助方法 =====================

// GetMetadataAsMap 获取元数据映射
func (b *UserBill) GetMetadataAsMap() (map[string]interface{}, error) {
	if b.Metadata == nil || *b.Metadata == "" {
		return map[string]interface{}{}, nil
	}

	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(*b.Metadata), &metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// SetMetadataFromMap 设置元数据
func (b *UserBill) SetMetadataFromMap(metadata map[string]interface{}) error {
	if len(metadata) == 0 {
		b.Metadata = nil
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	str := string(data)
	b.Metadata = &str
	return nil
}

// GetConversionMetadataAsMap 获取兑换元数据映射
func (b *UserBill) GetConversionMetadataAsMap() (map[string]interface{}, error) {
	if b.ConversionMetadata == nil || *b.ConversionMetadata == "" {
		return map[string]interface{}{}, nil
	}

	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(*b.ConversionMetadata), &metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// SetConversionMetadataFromMap 设置兑换元数据
func (b *UserBill) SetConversionMetadataFromMap(metadata map[string]interface{}) error {
	if len(metadata) == 0 {
		b.ConversionMetadata = nil
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	str := string(data)
	b.ConversionMetadata = &str
	return nil
}

// IsDebit 检查是否为支出账单
func (b *UserBill) IsDebit() bool {
	return b.Amount < 0
}

// IsCredit 检查是否为收入账单
func (b *UserBill) IsCredit() bool {
	return b.Amount > 0
}

// GetAbsAmount 获取绝对金额
func (b *UserBill) GetAbsAmount() int64 {
	if b.Amount < 0 {
		return -b.Amount
	}
	return b.Amount
}

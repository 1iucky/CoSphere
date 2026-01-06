package model

import (
	"encoding/json"
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// SubscriptionOrder 订阅订单
type SubscriptionOrder struct {
	Id              int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId          int64   `json:"user_id" gorm:"not null;index"`
	PlanId          int64   `json:"plan_id" gorm:"not null;index"`
	PlanSnapshot    string  `json:"plan_snapshot" gorm:"type:text;not null"`
	CouponId        *int64  `json:"coupon_id" gorm:"index"`
	UserCouponId    *int64  `json:"user_coupon_id" gorm:"index"`
	CouponSnapshot  *string `json:"coupon_snapshot" gorm:"type:text"`
	PaymentChannel  string  `json:"payment_channel" gorm:"type:varchar(32);not null"`
	PriceCents      int64   `json:"price_cents" gorm:"bigint;not null"`
	DiscountCents   int64   `json:"discount_cents" gorm:"bigint;default:0"`
	FinalPriceCents int64   `json:"final_price_cents" gorm:"bigint;not null"`
	RedemptionId    *int64  `json:"redemption_id" gorm:"index"`
	BillId          *int64  `json:"bill_id" gorm:"index"`
	SubscriptionId  *int64  `json:"subscription_id" gorm:"index"`
	TradeNo         *string `json:"trade_no" gorm:"type:varchar(64);uniqueIndex"`
	PaidAt          *int64  `json:"paid_at" gorm:"bigint"`
	Status          string  `json:"status" gorm:"type:varchar(32);default:'pending';index"`
	Metadata        *string `json:"metadata" gorm:"type:text"`
	CreatedAt       int64   `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt       int64   `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (SubscriptionOrder) TableName() string {
	return "subscription_orders"
}

// PlanSnapshotData 套餐快照数据结构
type PlanSnapshotData struct {
	Id                  int64                   `json:"id"`
	SKU                 string                  `json:"sku"`
	Name                string                  `json:"name"`
	Description         string                  `json:"description"`
	PriceCents          int64                   `json:"price_cents"`
	Currency            string                  `json:"currency"`
	BillingCycle        string                  `json:"billing_cycle"`
	BillingCycleValue   int                     `json:"billing_cycle_value"`
	AllowWalletFallback bool                    `json:"allow_wallet_fallback"`
	ModelWhitelist      []string                `json:"model_whitelist"`
	ChannelGroups       []string                `json:"channel_groups"`
	Limits              []PlanLimitSnapshotData `json:"limits"`
}

// PlanLimitSnapshotData 套餐限额快照数据
type PlanLimitSnapshotData struct {
	Period         string `json:"period"`
	Quota          int64  `json:"quota"`
	Unit           string `json:"unit"`
	WindowStrategy string `json:"window_strategy"`
}

// CouponSnapshotData 优惠券快照数据结构
type CouponSnapshotData struct {
	Id                int64   `json:"id"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Type              string  `json:"type"` // 优惠券类型: discount/full_reduction/instant_reduction
	DiscountType      string  `json:"discount_type"`
	DiscountValue     int64   `json:"discount_value"`
	ThresholdAmount   int64   `json:"threshold_amount,omitempty"`
	Currency          string  `json:"currency,omitempty"`
	Scope             string  `json:"scope"`
	ApplicablePlanIds []int64 `json:"applicable_plan_ids,omitempty"`
}

// ===================== 订单 CRUD 方法 =====================

// CreateSubscriptionOrder 创建订单
func CreateSubscriptionOrder(order *SubscriptionOrder) error {
	if err := validateSubscriptionOrder(order); err != nil {
		return err
	}

	return DB.Create(order).Error
}

// CreateSubscriptionOrderWithTx 在事务中创建订单
func CreateSubscriptionOrderWithTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if err := validateSubscriptionOrder(order); err != nil {
		return err
	}

	return tx.Create(order).Error
}

// UpdateSubscriptionOrder 更新订单
func UpdateSubscriptionOrder(order *SubscriptionOrder) error {
	if order.Id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	// 更新所有可变字段，包括支付相关信息
	return DB.Model(order).Updates(map[string]interface{}{
		"status":          order.Status,
		"bill_id":         order.BillId,
		"metadata":        order.Metadata,
		"paid_at":         order.PaidAt,         // 支付时间
		"payment_channel": order.PaymentChannel, // 支付渠道
		"trade_no":        order.TradeNo,        // 交易号
		"subscription_id": order.SubscriptionId, // 关联的订阅ID
	}).Error
}

// UpdateSubscriptionOrderWithTx 在事务中更新订单
func UpdateSubscriptionOrderWithTx(tx *gorm.DB, order *SubscriptionOrder) error {
	if order.Id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	// 更新所有可变字段，包括支付相关信息
	return tx.Model(order).Updates(map[string]interface{}{
		"status":          order.Status,
		"bill_id":         order.BillId,
		"metadata":        order.Metadata,
		"paid_at":         order.PaidAt,         // 支付时间
		"payment_channel": order.PaymentChannel, // 支付渠道
		"trade_no":        order.TradeNo,        // 交易号
		"subscription_id": order.SubscriptionId, // 关联的订阅ID
	}).Error
}

// GetSubscriptionOrderById 根据 ID 获取订单
func GetSubscriptionOrderById(id int64) (*SubscriptionOrder, error) {
	if id == 0 {
		return nil, errors.New("订单 ID 不能为空")
	}

	var order SubscriptionOrder
	err := DB.First(&order, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgOrderNotFound)
		}
		return nil, err
	}

	return &order, nil
}

// GetSubscriptionOrderByIdWithTx 在事务中根据 ID 获取订单（带行锁）
func GetSubscriptionOrderByIdWithTx(tx *gorm.DB, id int64, forUpdate bool) (*SubscriptionOrder, error) {
	if id == 0 {
		return nil, errors.New("订单 ID 不能为空")
	}

	var order SubscriptionOrder
	query := tx
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.First(&order, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgOrderNotFound)
		}
		return nil, err
	}

	return &order, nil
}

// GetSubscriptionOrderByTradeNo 根据交易号获取订单
func GetSubscriptionOrderByTradeNo(tradeNo string) (*SubscriptionOrder, error) {
	if tradeNo == "" {
		return nil, errors.New("交易号不能为空")
	}

	var order SubscriptionOrder
	err := DB.First(&order, "trade_no = ?", tradeNo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgOrderNotFound)
		}
		return nil, err
	}

	return &order, nil
}

// GetSubscriptionOrdersByUser 获取用户的订单列表（分页）
func GetSubscriptionOrdersByUser(userId int64, startIdx int, num int, status string) (orders []*SubscriptionOrder, total int64, err error) {
	if userId == 0 {
		return nil, 0, errors.New("用户 ID 不能为空")
	}

	query := DB.Model(&SubscriptionOrder{}).Where("user_id = ?", userId)

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
		Find(&orders).Error
	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// GetAllSubscriptionOrders 获取所有订单（管理员，分页）
func GetAllSubscriptionOrders(startIdx int, num int, status string) (orders []*SubscriptionOrder, total int64, err error) {
	query := DB.Model(&SubscriptionOrder{})

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
		Find(&orders).Error
	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// SearchSubscriptionOrders 搜索订单
func SearchSubscriptionOrders(userId int64, planId int64, status string, startTime int64, endTime int64, startIdx int, num int) (orders []*SubscriptionOrder, total int64, err error) {
	query := DB.Model(&SubscriptionOrder{})

	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if planId > 0 {
		query = query.Where("plan_id = ?", planId)
	}
	if status != "" {
		query = query.Where("status = ?", status)
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
		Find(&orders).Error
	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// GetOrdersByPlan 获取套餐的订单统计
func GetOrdersByPlan(planId int64, startIdx int, num int) (orders []*SubscriptionOrder, total int64, err error) {
	if planId == 0 {
		return nil, 0, errors.New("套餐 ID 不能为空")
	}

	query := DB.Model(&SubscriptionOrder{}).Where("plan_id = ?", planId)

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&orders).Error
	if err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// GetOrdersByCoupon 获取优惠券的订单
func GetOrdersByCoupon(couponId int64) ([]*SubscriptionOrder, error) {
	if couponId == 0 {
		return nil, errors.New("优惠券 ID 不能为空")
	}

	var orders []*SubscriptionOrder
	err := DB.Where("coupon_id = ?", couponId).
		Order("created_at desc").
		Find(&orders).Error

	if err != nil {
		return nil, err
	}

	return orders, nil
}

// GetPendingOrders 获取待支付订单（用于超时处理）
func GetPendingOrders(timeoutSeconds int64, limit int) ([]*SubscriptionOrder, error) {
	cutoffTime := common.GetTimestamp() - timeoutSeconds

	var orders []*SubscriptionOrder
	err := DB.Where("status = ?", common.OrderStatusPending).
		Where("created_at < ?", cutoffTime).
		Limit(limit).
		Find(&orders).Error

	if err != nil {
		return nil, err
	}

	return orders, nil
}

// GetUserPendingOrderByPlan 获取用户对某套餐的待支付订单（未过期）
// 用于幂等性检查，防止重复创建订单
func GetUserPendingOrderByPlan(userId int64, planId int64) (*SubscriptionOrder, error) {
	if userId == 0 || planId == 0 {
		return nil, nil
	}

	// 计算 30 分钟前的时间戳（与 IsExpired 逻辑一致）
	expireThreshold := common.GetTimestamp() - 30*60

	var order SubscriptionOrder
	err := DB.Where("user_id = ?", userId).
		Where("plan_id = ?", planId).
		Where("status = ?", common.OrderStatusPending).
		Where("created_at >= ?", expireThreshold). // 只查询未过期的订单
		Order("created_at desc").
		First(&order).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &order, nil
}

// GetUserPendingOrderByPlanWithTx 在事务中查询用户对某套餐的未过期待支付订单（支持行锁）
func GetUserPendingOrderByPlanWithTx(tx *gorm.DB, userId int64, planId int64, forUpdate bool) (*SubscriptionOrder, error) {
	if userId == 0 || planId == 0 {
		return nil, nil
	}

	// 计算 30 分钟前的时间戳（与 IsExpired 逻辑一致）
	expireThreshold := common.GetTimestamp() - 30*60

	query := tx.Where("user_id = ?", userId).
		Where("plan_id = ?", planId).
		Where("status = ?", common.OrderStatusPending).
		Where("created_at >= ?", expireThreshold). // 只查询未过期的订单
		Order("created_at desc")

	// 如果需要行锁，添加 FOR UPDATE
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	var order SubscriptionOrder
	err := query.First(&order).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &order, nil
}

// ===================== 状态转换方法 =====================

// PayOrder 支付订单
func PayOrder(id int64, billId int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return DB.Model(&SubscriptionOrder{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":  common.OrderStatusPaid,
			"bill_id": billId,
		}).Error
}

// PayOrderWithTx 在事务中支付订单
func PayOrderWithTx(tx *gorm.DB, id int64, billId int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return tx.Model(&SubscriptionOrder{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":  common.OrderStatusPaid,
			"bill_id": billId,
		}).Error
}

// CancelOrder 取消订单
func CancelOrder(id int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	order, err := GetSubscriptionOrderById(id)
	if err != nil {
		return err
	}

	if order.Status != common.OrderStatusPending {
		return errors.New("只能取消待支付的订单")
	}

	return DB.Model(&SubscriptionOrder{}).Where("id = ?", id).
		Update("status", common.OrderStatusCancelled).Error
}

// CancelOrderWithTx 在事务中取消订单
func CancelOrderWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return tx.Model(&SubscriptionOrder{}).
		Where("id = ? AND status = ?", id, common.OrderStatusPending).
		Update("status", common.OrderStatusCancelled).Error
}

// ExpireOrder 订单过期
func ExpireOrder(id int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return DB.Model(&SubscriptionOrder{}).
		Where("id = ? AND status = ?", id, common.OrderStatusPending).
		Update("status", common.OrderStatusExpired).Error
}

// ExpireOrderWithTx 在事务中标记订单过期
func ExpireOrderWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return tx.Model(&SubscriptionOrder{}).
		Where("id = ? AND status = ?", id, common.OrderStatusPending).
		Update("status", common.OrderStatusExpired).Error
}

// RefundOrder 退款订单
func RefundOrder(id int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	order, err := GetSubscriptionOrderById(id)
	if err != nil {
		return err
	}

	if order.Status != common.OrderStatusPaid {
		return errors.New("只能退款已支付的订单")
	}

	return DB.Model(&SubscriptionOrder{}).Where("id = ?", id).
		Update("status", common.OrderStatusRefunded).Error
}

// RefundOrderWithTx 在事务中退款订单
func RefundOrderWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("订单 ID 不能为空")
	}

	return tx.Model(&SubscriptionOrder{}).
		Where("id = ? AND status = ?", id, common.OrderStatusPaid).
		Update("status", common.OrderStatusRefunded).Error
}

// ===================== 快照方法 =====================

// CreatePlanSnapshot 从套餐创建快照
func CreatePlanSnapshot(plan *SubscriptionPlan) (string, error) {
	if plan == nil {
		return "", errors.New("套餐不能为空")
	}

	modelWhitelist, _ := plan.GetModelWhitelistAsSlice()
	channelGroups, _ := plan.GetChannelGroupsAsSlice()

	var limits []PlanLimitSnapshotData
	for _, limit := range plan.GetEnabledLimits() {
		limits = append(limits, PlanLimitSnapshotData{
			Period:         limit.Period,
			Quota:          limit.Quota,
			Unit:           limit.Unit,
			WindowStrategy: limit.WindowStrategy,
		})
	}

	snapshot := PlanSnapshotData{
		Id:                  plan.Id,
		Name:                plan.Name,
		Description:         safeString(plan.Description),
		PriceCents:          plan.PriceCents,
		Currency:            plan.Currency,
		BillingCycle:        plan.BillingCycle,
		BillingCycleValue:   plan.BillingCycleValue,
		AllowWalletFallback: plan.AllowWalletFallback,
		ModelWhitelist:      modelWhitelist,
		ChannelGroups:       channelGroups,
		Limits:              limits,
	}

	if plan.SKU != nil {
		snapshot.SKU = *plan.SKU
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// GetPlanSnapshotData 解析套餐快照
func (o *SubscriptionOrder) GetPlanSnapshotData() (*PlanSnapshotData, error) {
	if o.PlanSnapshot == "" {
		return nil, errors.New("套餐快照为空")
	}

	var snapshot PlanSnapshotData
	err := json.Unmarshal([]byte(o.PlanSnapshot), &snapshot)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// CreateCouponSnapshot 从优惠券创建快照
func CreateCouponSnapshot(coupon *Coupon) (string, error) {
	if coupon == nil {
		return "", nil
	}

	applicablePlanIds, err := coupon.GetApplicablePlanIds()
	if err != nil {
		return "", err
	}

	snapshot := CouponSnapshotData{
		Id:                coupon.Id,
		Code:              coupon.Code,
		Name:              coupon.Name,
		Type:              coupon.Type,
		DiscountType:      coupon.DiscountType,
		DiscountValue:     coupon.DiscountValue,
		ThresholdAmount:   coupon.ThresholdAmount,
		Currency:          coupon.Currency,
		Scope:             coupon.Scope,
		ApplicablePlanIds: applicablePlanIds,
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// GetCouponSnapshotData 解析优惠券快照
func (o *SubscriptionOrder) GetCouponSnapshotData() (*CouponSnapshotData, error) {
	if o.CouponSnapshot == nil || *o.CouponSnapshot == "" {
		return nil, nil
	}

	var snapshot CouponSnapshotData
	err := json.Unmarshal([]byte(*o.CouponSnapshot), &snapshot)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// ===================== 验证方法 =====================

// validateSubscriptionOrder 验证订单数据
func validateSubscriptionOrder(order *SubscriptionOrder) error {
	if order.UserId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	if order.PlanId == 0 {
		return errors.New("套餐 ID 不能为空")
	}

	if order.PlanSnapshot == "" {
		return errors.New("套餐快照不能为空")
	}

	if order.PaymentChannel == "" {
		return errors.New("支付渠道不能为空")
	}

	validChannels := map[string]bool{
		common.PaymentChannelWallet:     true,
		common.PaymentChannelRedemption: true,
		common.PaymentChannelAlipay:     true,
		common.PaymentChannelWechat:     true,
		common.PaymentChannelStripe:     true,
		common.PaymentChannelFree:       true,
	}
	if !validChannels[order.PaymentChannel] {
		return errors.New("无效的支付渠道")
	}

	if order.PriceCents < 0 {
		return errors.New(common.MsgOrderInvalidAmount)
	}

	if order.DiscountCents < 0 {
		return errors.New("折扣金额不能为负数")
	}

	if order.FinalPriceCents < 0 {
		return errors.New(common.MsgOrderInvalidAmount)
	}

	validStatuses := map[string]bool{
		common.OrderStatusPending:   true,
		common.OrderStatusPaid:      true,
		common.OrderStatusCancelled: true,
		common.OrderStatusExpired:   true,
		common.OrderStatusRefunded:  true,
	}
	if order.Status != "" && !validStatuses[order.Status] {
		return errors.New(common.MsgOrderInvalidStatus)
	}

	return nil
}

// ===================== 统计方法 =====================

// CountOrdersByUser 统计用户订单数量
func CountOrdersByUser(userId int64, status string) (int64, error) {
	if userId == 0 {
		return 0, errors.New("用户 ID 不能为空")
	}

	query := DB.Model(&SubscriptionOrder{}).Where("user_id = ?", userId)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	var count int64
	err := query.Count(&count).Error

	return count, err
}

// SumOrderAmountByUser 统计用户订单总金额
func SumOrderAmountByUser(userId int64) (int64, error) {
	if userId == 0 {
		return 0, errors.New("用户 ID 不能为空")
	}

	var totalAmount int64
	err := DB.Model(&SubscriptionOrder{}).
		Where("user_id = ?", userId).
		Where("status = ?", common.OrderStatusPaid).
		Select("COALESCE(SUM(final_price_cents), 0)").
		Scan(&totalAmount).Error

	return totalAmount, err
}

// GetOrderStatsByPlan 获取套餐订单统计
func GetOrderStatsByPlan(planId int64) (paidCount int64, totalRevenue int64, err error) {
	if planId == 0 {
		return 0, 0, errors.New("套餐 ID 不能为空")
	}

	// 已支付订单数
	err = DB.Model(&SubscriptionOrder{}).
		Where("plan_id = ?", planId).
		Where("status = ?", common.OrderStatusPaid).
		Count(&paidCount).Error
	if err != nil {
		return 0, 0, err
	}

	// 总收入
	err = DB.Model(&SubscriptionOrder{}).
		Where("plan_id = ?", planId).
		Where("status = ?", common.OrderStatusPaid).
		Select("COALESCE(SUM(final_price_cents), 0)").
		Scan(&totalRevenue).Error
	if err != nil {
		return 0, 0, err
	}

	return paidCount, totalRevenue, nil
}

// ===================== 辅助方法 =====================

// GetMetadataAsMap 获取元数据映射
func (o *SubscriptionOrder) GetMetadataAsMap() (map[string]interface{}, error) {
	if o.Metadata == nil || *o.Metadata == "" {
		return map[string]interface{}{}, nil
	}

	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(*o.Metadata), &metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// SetMetadataFromMap 设置元数据
func (o *SubscriptionOrder) SetMetadataFromMap(metadata map[string]interface{}) error {
	if len(metadata) == 0 {
		o.Metadata = nil
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	str := string(data)
	o.Metadata = &str
	return nil
}

// IsPending 检查订单是否待支付
func (o *SubscriptionOrder) IsPending() bool {
	return o.Status == common.OrderStatusPending
}

// IsPaid 检查订单是否已支付
func (o *SubscriptionOrder) IsPaid() bool {
	return o.Status == common.OrderStatusPaid
}

// CanBePaid 检查订单是否可以支付
func (o *SubscriptionOrder) CanBePaid() bool {
	return o.Status == common.OrderStatusPending
}

// CanBeRefunded 检查订单是否可以退款
func (o *SubscriptionOrder) CanBeRefunded() bool {
	return o.Status == common.OrderStatusPaid
}

// GetActualDiscount 获取实际折扣金额
func (o *SubscriptionOrder) GetActualDiscount() int64 {
	return o.PriceCents - o.FinalPriceCents
}

// safeString 安全获取字符串指针的值
func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetExpiredPendingOrders 获取超时未支付的订单（供定时任务调用）
// 默认超时时间为 30 分钟
func GetExpiredPendingOrders(limit int) ([]*SubscriptionOrder, error) {
	if limit <= 0 {
		limit = 100
	}

	now := common.GetTimestamp()
	expireThreshold := now - 30*60 // 30分钟前创建且未支付的订单

	var orders []*SubscriptionOrder
	err := DB.Where("status = ?", common.OrderStatusPending).
		Where("created_at < ?", expireThreshold).
		Limit(limit).
		Find(&orders).Error

	if err != nil {
		return nil, err
	}

	return orders, nil
}

// ===================== 订单辅助方法 =====================

// IsExpired 检查订单是否已过期（30分钟未支付）
func (o *SubscriptionOrder) IsExpired() bool {
	if o.Status != common.OrderStatusPending {
		return false
	}
	now := common.GetTimestamp()
	expireThreshold := o.CreatedAt + 30*60 // 30分钟
	return now > expireThreshold
}

// SetPlanSnapshotFromData 从数据结构设置套餐快照
func (o *SubscriptionOrder) SetPlanSnapshotFromData(data *PlanSnapshotData) error {
	if data == nil {
		return errors.New("套餐快照数据不能为空")
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	o.PlanSnapshot = string(jsonData)
	return nil
}

// GetPlanSnapshotAsData 获取套餐快照数据结构
func (o *SubscriptionOrder) GetPlanSnapshotAsData() (*PlanSnapshotData, error) {
	if o.PlanSnapshot == "" {
		return nil, errors.New("套餐快照为空")
	}

	var snapshot PlanSnapshotData
	err := json.Unmarshal([]byte(o.PlanSnapshot), &snapshot)
	if err != nil {
		return nil, err
	}

	return &snapshot, nil
}

// SetCouponSnapshotFromData 从数据结构设置优惠券快照
func (o *SubscriptionOrder) SetCouponSnapshotFromData(data *CouponSnapshotData) error {
	if data == nil {
		o.CouponSnapshot = nil
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	str := string(jsonData)
	o.CouponSnapshot = &str
	return nil
}

package model

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Subscription 用户订阅
type Subscription struct {
	Id                  int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId              int64   `json:"user_id" gorm:"not null;index:idx_subscriptions_user_status,priority:1;index:idx_subscriptions_user_priority,priority:1"`
	PlanId              int64   `json:"plan_id" gorm:"not null;index"`
	OrderId             *int64  `json:"order_id" gorm:"index"`
	Status              string  `json:"status" gorm:"type:varchar(32);default:'pending';index:idx_subscriptions_user_status,priority:2"`
	StartAt             int64   `json:"start_at" gorm:"bigint;not null"`
	EndAt               int64   `json:"end_at" gorm:"bigint;not null;index:idx_subscriptions_end_at_status,priority:1"`
	Priority            int     `json:"priority" gorm:"not null;default:1;index:idx_subscriptions_user_priority,priority:2"`
	AutoWalletFallback  bool    `json:"auto_wallet_fallback" gorm:"default:false"`
	BindChannelGroup    *string `json:"bind_channel_group" gorm:"type:varchar(64)"`
	ModelWhitelistCache *string `json:"model_whitelist_cache" gorm:"type:text"`
	RedemptionId        *int64  `json:"redemption_id" gorm:"index"`
	CouponId            *int64  `json:"coupon_id" gorm:"index"`
	RedeemOption        string  `json:"redeem_option" gorm:"type:varchar(32);default:'stack'"`
	Metadata            *string `json:"metadata" gorm:"type:text"`
	CreatedAt           int64   `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt           int64   `json:"updated_at" gorm:"bigint;autoUpdateTime"`

	// 关联（非数据库字段）
	Plan *SubscriptionPlan `json:"plan,omitempty" gorm:"foreignKey:PlanId;references:Id"`
}

func (Subscription) TableName() string {
	return "subscriptions"
}

// BeforeCreate GORM hook：创建前确保 priority 有有效值
func (sub *Subscription) BeforeCreate(tx *gorm.DB) error {
	// 如果 priority 未设置（默认为 0），设为 1
	if sub.Priority == 0 {
		sub.Priority = 1
	}
	return nil
}

// ===================== 订阅 CRUD 方法 =====================

// CreateSubscription 创建订阅
func CreateSubscription(sub *Subscription) error {
	if err := validateSubscription(sub); err != nil {
		return err
	}

	return DB.Create(sub).Error
}

// CreateSubscriptionWithTx 在事务中创建订阅
func CreateSubscriptionWithTx(tx *gorm.DB, sub *Subscription) error {
	if err := validateSubscription(sub); err != nil {
		return err
	}

	return tx.Create(sub).Error
}

// UpdateSubscription 更新订阅
func UpdateSubscription(sub *Subscription) error {
	if sub.Id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return DB.Model(sub).Updates(map[string]interface{}{
		"status":               sub.Status,
		"start_at":             sub.StartAt,
		"end_at":               sub.EndAt,
		"priority":             sub.Priority,
		"auto_wallet_fallback": sub.AutoWalletFallback,
		"bind_channel_group":   sub.BindChannelGroup,
		"model_whitelist_cache": sub.ModelWhitelistCache,
		"redeem_option":        sub.RedeemOption,
		"metadata":             sub.Metadata,
	}).Error
}

// UpdateSubscriptionWithTx 在事务中更新订阅
func UpdateSubscriptionWithTx(tx *gorm.DB, sub *Subscription) error {
	if sub.Id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return tx.Model(sub).Updates(map[string]interface{}{
		"status":               sub.Status,
		"start_at":             sub.StartAt,
		"end_at":               sub.EndAt,
		"priority":             sub.Priority,
		"auto_wallet_fallback": sub.AutoWalletFallback,
		"bind_channel_group":   sub.BindChannelGroup,
		"model_whitelist_cache": sub.ModelWhitelistCache,
		"redeem_option":        sub.RedeemOption,
		"metadata":             sub.Metadata,
	}).Error
}

// UpdateSubscriptionAutoWalletFallback 只更新订阅的 auto_wallet_fallback 字段
// 使用 WHERE id AND user_id 限定，避免并发更新时覆盖其他字段
// 支持幂等：同值更新也返回成功（MySQL/SQLite 同值更新 RowsAffected=0）
func UpdateSubscriptionAutoWalletFallback(id int64, userId int64, enabled bool) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	result := DB.Model(&Subscription{}).
		Where("id = ? AND user_id = ?", id, userId).
		Update("auto_wallet_fallback", enabled)

	if result.Error != nil {
		return result.Error
	}

	// MySQL/SQLite 同值更新时 RowsAffected=0，需额外校验订阅是否存在且属于用户
	if result.RowsAffected == 0 {
		var count int64
		if err := DB.Model(&Subscription{}).Where("id = ? AND user_id = ?", id, userId).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return errors.New("订阅不存在或无权修改")
		}
		// 订阅存在且属于用户，说明是同值更新，返回成功（幂等）
	}
	return nil
}

// GetSubscriptionById 根据 ID 获取订阅
func GetSubscriptionById(id int64) (*Subscription, error) {
	if id == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	var sub Subscription
	err := DB.Preload("Plan").First(&sub, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgSubscriptionNotFound)
		}
		return nil, err
	}

	return &sub, nil
}

// GetSubscriptionByIdWithTx 在事务中根据 ID 获取订阅（带行锁）
func GetSubscriptionByIdWithTx(tx *gorm.DB, id int64, forUpdate bool) (*Subscription, error) {
	if id == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	var sub Subscription
	query := tx.Preload("Plan")
	if forUpdate {
		query = query.Set("gorm:query_option", "FOR UPDATE")
	}

	err := query.First(&sub, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New(common.MsgSubscriptionNotFound)
		}
		return nil, err
	}

	return &sub, nil
}

// GetActiveSubscriptionsByUser 获取用户的有效订阅（按优先级排序）
func GetActiveSubscriptionsByUser(userId int64) ([]*Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	var subs []*Subscription
	now := common.GetTimestamp()

	err := DB.Preload("Plan").
		Where("user_id = ?", userId).
		Where("status = ?", common.SubscriptionStatusActive).
		Where("start_at <= ?", now).
		Where("end_at >= ?", now).
		Order("priority asc").
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// GetHighestPriorityActiveSubscription 获取用户优先级最高的有效订阅
func GetHighestPriorityActiveSubscription(userId int64) (*Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	var sub Subscription
	now := common.GetTimestamp()

	err := DB.Preload("Plan").
		Where("user_id = ?", userId).
		Where("status = ?", common.SubscriptionStatusActive).
		Where("start_at <= ?", now).
		Where("end_at >= ?", now).
		Order("priority asc").
		First(&sub).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 没有有效订阅不是错误
		}
		return nil, err
	}

	return &sub, nil
}

// GetAllSubscriptionsByUser 获取用户的所有订阅（分页）
func GetAllSubscriptionsByUser(userId int64, startIdx int, num int, status string) (subs []*Subscription, total int64, err error) {
	if userId == 0 {
		return nil, 0, errors.New("用户 ID 不能为空")
	}

	query := DB.Model(&Subscription{}).Where("user_id = ?", userId)

	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Preload("Plan").
		Order("priority asc, created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&subs).Error
	if err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

// GetSubscriptionsByPlan 获取套餐下的所有订阅
func GetSubscriptionsByPlan(planId int64, startIdx int, num int) (subs []*Subscription, total int64, err error) {
	if planId == 0 {
		return nil, 0, errors.New("套餐 ID 不能为空")
	}

	query := DB.Model(&Subscription{}).Where("plan_id = ?", planId)

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&subs).Error
	if err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

// GetAllSubscriptionsAdmin 获取全量订阅列表（管理员）
// 支持组合筛选条件：userId, planId, status
func GetAllSubscriptionsAdmin(userId *int64, planId *int64, status string, startIdx int, num int) (subs []*Subscription, total int64, err error) {
	query := DB.Model(&Subscription{})

	// 组合筛选条件
	if userId != nil && *userId > 0 {
		query = query.Where("user_id = ?", *userId)
	}
	if planId != nil && *planId > 0 {
		query = query.Where("plan_id = ?", *planId)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据（预加载 Plan）
	err = query.Preload("Plan").
		Order("created_at desc").
		Limit(num).
		Offset(startIdx).
		Find(&subs).Error
	if err != nil {
		return nil, 0, err
	}

	return subs, total, nil
}

// CountActiveSubscriptionsByUser 获取用户有效订阅数量
func CountActiveSubscriptionsByUser(userId int64) (int64, error) {
	if userId == 0 {
		return 0, errors.New("用户 ID 不能为空")
	}

	var count int64
	now := common.GetTimestamp()

	err := DB.Model(&Subscription{}).
		Where("user_id = ?", userId).
		Where("status = ?", common.SubscriptionStatusActive).
		Where("start_at <= ?", now).
		Where("end_at >= ?", now).
		Count(&count).Error

	return count, err
}

// CountActiveSubscriptionsByUserWithTxForUpdate 在事务中获取用户有效订阅数量（带行锁）
// 使用 FOR UPDATE 锁定相关行，防止并发创建导致超限
func CountActiveSubscriptionsByUserWithTxForUpdate(tx *gorm.DB, userId int64) (int64, error) {
	if userId == 0 {
		return 0, errors.New("用户 ID 不能为空")
	}

	now := common.GetTimestamp()

	// 先锁定用户的所有活跃订阅行，防止并发插入
	var subs []*Subscription
	err := tx.Where("user_id = ?", userId).
		Where("status = ?", common.SubscriptionStatusActive).
		Where("start_at <= ?", now).
		Where("end_at >= ?", now).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Find(&subs).Error

	if err != nil {
		return 0, err
	}

	return int64(len(subs)), nil
}

// GetSubscriptionByOrder 根据订单 ID 获取订阅
func GetSubscriptionByOrder(orderId int64) (*Subscription, error) {
	if orderId == 0 {
		return nil, errors.New("订单 ID 不能为空")
	}

	var sub Subscription
	err := DB.Preload("Plan").First(&sub, "order_id = ?", orderId).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &sub, nil
}

// GetExpiringSubscriptions 获取即将过期的订阅（用于提醒）
func GetExpiringSubscriptions(expiryDays int) ([]*Subscription, error) {
	now := common.GetTimestamp()
	expiryTimestamp := now + int64(expiryDays*24*60*60)

	var subs []*Subscription
	err := DB.Preload("Plan").
		Where("status = ?", common.SubscriptionStatusActive).
		Where("end_at > ?", now).
		Where("end_at <= ?", expiryTimestamp).
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// GetExpiredSubscriptions 获取已过期的订阅（用于状态更新）
func GetExpiredSubscriptions(limit int) ([]*Subscription, error) {
	now := common.GetTimestamp()

	var subs []*Subscription
	err := DB.Where("status = ?", common.SubscriptionStatusActive).
		Where("end_at < ?", now).
		Limit(limit).
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// ===================== 状态转换方法 =====================

// ActivateSubscription 激活订阅
func ActivateSubscription(id int64) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	sub, err := GetSubscriptionById(id)
	if err != nil {
		return err
	}

	if sub.Status == common.SubscriptionStatusActive {
		return errors.New("订阅已是激活状态")
	}

	if sub.Status == common.SubscriptionStatusCancelled {
		return errors.New(common.MsgSubscriptionCancelled)
	}

	return DB.Model(&Subscription{}).Where("id = ?", id).
		Update("status", common.SubscriptionStatusActive).Error
}

// ActivateSubscriptionWithTx 在事务中激活订阅
func ActivateSubscriptionWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return tx.Model(&Subscription{}).Where("id = ?", id).
		Update("status", common.SubscriptionStatusActive).Error
}

// ExpireSubscription 过期订阅
func ExpireSubscription(id int64) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return DB.Model(&Subscription{}).Where("id = ?", id).
		Update("status", common.SubscriptionStatusExpired).Error
}

// ExpireSubscriptionWithTx 在事务中过期订阅
func ExpireSubscriptionWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return tx.Model(&Subscription{}).Where("id = ?", id).
		Update("status", common.SubscriptionStatusExpired).Error
}

// CancelSubscription 取消订阅
func CancelSubscription(id int64) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	sub, err := GetSubscriptionById(id)
	if err != nil {
		return err
	}

	if sub.Status == common.SubscriptionStatusCancelled {
		return errors.New("订阅已取消")
	}

	now := common.GetTimestamp()
	return DB.Model(&Subscription{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status": common.SubscriptionStatusCancelled,
			"end_at": now, // 取消时立即终止
		}).Error
}

// CancelSubscriptionWithTx 在事务中取消订阅
func CancelSubscriptionWithTx(tx *gorm.DB, id int64) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()
	return tx.Model(&Subscription{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status": common.SubscriptionStatusCancelled,
			"end_at": now, // 取消时立即终止
		}).Error
}

// CancelSubscriptionWithReason 取消订阅（带原因）
// 返回取消前的订阅信息用于审计
func CancelSubscriptionWithReason(id int64, reason string) (*Subscription, error) {
	if id == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	sub, err := GetSubscriptionById(id)
	if err != nil {
		return nil, err
	}

	if sub.Status == common.SubscriptionStatusCancelled {
		return nil, errors.New("订阅已取消")
	}

	// 保存取消前的信息用于返回
	originalSub := *sub

	now := common.GetTimestamp()

	// 将取消原因存入 metadata
	metadata, _ := sub.GetMetadataAsMap()
	metadata["cancel_reason"] = reason
	metadata["cancel_time"] = now
	metadata["original_end_at"] = sub.EndAt
	sub.SetMetadataFromMap(metadata)

	err = DB.Model(&Subscription{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":   common.SubscriptionStatusCancelled,
			"end_at":   now,
			"metadata": sub.Metadata,
		}).Error
	if err != nil {
		return nil, err
	}

	return &originalSub, nil
}

// ===================== 优先级管理方法 =====================

// UpdateSubscriptionPriority 更新订阅优先级
func UpdateSubscriptionPriority(id int64, priority int) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	if priority < 1 {
		return errors.New("优先级必须为正整数")
	}

	return DB.Model(&Subscription{}).Where("id = ?", id).
		Update("priority", priority).Error
}

// UpdateSubscriptionPriorityWithTx 在事务中更新订阅优先级
func UpdateSubscriptionPriorityWithTx(tx *gorm.DB, id int64, priority int) error {
	if id == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	if priority < 1 {
		return errors.New("优先级必须为正整数")
	}

	return tx.Model(&Subscription{}).Where("id = ?", id).
		Update("priority", priority).Error
}

// GetNextAvailablePriority 获取用户下一个可用的优先级（最低优先级 + 1）
func GetNextAvailablePriority(userId int64) (int, error) {
	if userId == 0 {
		return 0, errors.New("用户 ID 不能为空")
	}

	var maxPriority int
	err := DB.Model(&Subscription{}).
		Where("user_id = ?", userId).
		Where("status IN ?", []string{common.SubscriptionStatusPending, common.SubscriptionStatusActive}).
		Select("COALESCE(MAX(priority), 0)").
		Scan(&maxPriority).Error

	if err != nil {
		return 0, err
	}

	return maxPriority + 1, nil
}

// GetNextAvailablePriorityWithTx 在事务中获取用户下一个可用的优先级
func GetNextAvailablePriorityWithTx(tx *gorm.DB, userId int64) (int, error) {
	if userId == 0 {
		return 0, errors.New("用户 ID 不能为空")
	}

	var maxPriority int
	err := tx.Model(&Subscription{}).
		Where("user_id = ?", userId).
		Where("status IN ?", []string{common.SubscriptionStatusPending, common.SubscriptionStatusActive}).
		Select("COALESCE(MAX(priority), 0)").
		Scan(&maxPriority).Error

	if err != nil {
		return 0, err
	}

	return maxPriority + 1, nil
}

// GetActiveSubscriptionsByUserWithTx 在事务中获取用户所有有效订阅
func GetActiveSubscriptionsByUserWithTx(tx *gorm.DB, userId int64) ([]*Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	var subs []*Subscription
	now := common.GetTimestamp()

	err := tx.Preload("Plan").
		Where("user_id = ?", userId).
		Where("status = ?", common.SubscriptionStatusActive).
		Where("start_at <= ?", now).
		Where("end_at >= ?", now).
		Order("priority asc").
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// GetActiveSubscriptionsByUserWithTxForUpdate 在事务中获取用户所有有效订阅（带行锁）
// 使用 FOR UPDATE 锁定行，防止并发修改
func GetActiveSubscriptionsByUserWithTxForUpdate(tx *gorm.DB, userId int64) ([]*Subscription, error) {
	if userId == 0 {
		return nil, errors.New("用户 ID 不能为空")
	}

	var subs []*Subscription
	now := common.GetTimestamp()

	err := tx.Preload("Plan").
		Where("user_id = ?", userId).
		Where("status = ?", common.SubscriptionStatusActive).
		Where("start_at <= ?", now).
		Where("end_at >= ?", now).
		Order("priority asc").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Find(&subs).Error

	if err != nil {
		return nil, err
	}

	return subs, nil
}

// ReorderSubscriptionPriorities 重新排序用户订阅的优先级
func ReorderSubscriptionPriorities(userId int64, subscriptionIds []int64) error {
	if userId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	if len(subscriptionIds) == 0 {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		for i, subId := range subscriptionIds {
			if err := tx.Model(&Subscription{}).
				Where("id = ? AND user_id = ?", subId, userId).
				Update("priority", i+1).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ===================== 验证方法 =====================

// validateSubscription 验证订阅数据
func validateSubscription(sub *Subscription) error {
	if sub.UserId == 0 {
		return errors.New("用户 ID 不能为空")
	}

	if sub.PlanId == 0 {
		return errors.New("套餐 ID 不能为空")
	}

	if sub.StartAt == 0 {
		return errors.New("订阅开始时间不能为空")
	}

	if sub.EndAt == 0 {
		return errors.New("订阅结束时间不能为空")
	}

	if sub.EndAt <= sub.StartAt {
		return errors.New("订阅结束时间必须晚于开始时间")
	}

	validStatuses := map[string]bool{
		common.SubscriptionStatusPending:   true,
		common.SubscriptionStatusActive:    true,
		common.SubscriptionStatusExpired:   true,
		common.SubscriptionStatusCancelled: true,
	}
	if sub.Status != "" && !validStatuses[sub.Status] {
		return errors.New("无效的订阅状态")
	}

	validRedeemOptions := map[string]bool{
		common.RedeemOptionStack:   true,
		common.RedeemOptionCoexist: true,
		common.RedeemOptionConvert: true,
		common.RedeemOptionReplace: true,
		common.RedeemOptionExtend:  true,
	}
	if sub.RedeemOption != "" && !validRedeemOptions[sub.RedeemOption] {
		return errors.New("无效的兑换选项")
	}

	if sub.Priority < 0 {
		return errors.New("优先级不能为负数")
	}

	return nil
}

// ===================== 辅助方法 =====================

// IsActive 检查订阅是否当前有效
func (s *Subscription) IsActive() bool {
	if s.Status != common.SubscriptionStatusActive {
		return false
	}

	now := common.GetTimestamp()
	return s.StartAt <= now && s.EndAt >= now
}

// IsExpired 检查订阅是否已过期
func (s *Subscription) IsExpired() bool {
	now := common.GetTimestamp()
	return s.EndAt < now
}

// GetRemainingDays 获取订阅剩余天数
func (s *Subscription) GetRemainingDays() int {
	if !s.IsActive() {
		return 0
	}

	now := common.GetTimestamp()
	remainingSeconds := s.EndAt - now
	if remainingSeconds <= 0 {
		return 0
	}

	return int(remainingSeconds / (24 * 60 * 60))
}

// GetModelWhitelistAsSlice 获取模型白名单切片
func (s *Subscription) GetModelWhitelistAsSlice() ([]string, error) {
	if s.ModelWhitelistCache == nil || *s.ModelWhitelistCache == "" {
		return []string{}, nil
	}

	var models []string
	err := json.Unmarshal([]byte(*s.ModelWhitelistCache), &models)
	if err != nil {
		return nil, err
	}

	return models, nil
}

// SetModelWhitelistFromSlice 设置模型白名单
func (s *Subscription) SetModelWhitelistFromSlice(models []string) error {
	if len(models) == 0 {
		s.ModelWhitelistCache = nil
		return nil
	}

	data, err := json.Marshal(models)
	if err != nil {
		return err
	}
	str := string(data)
	s.ModelWhitelistCache = &str
	return nil
}

// GetMetadataAsMap 获取元数据映射
func (s *Subscription) GetMetadataAsMap() (map[string]interface{}, error) {
	if s.Metadata == nil || *s.Metadata == "" {
		return map[string]interface{}{}, nil
	}

	var metadata map[string]interface{}
	err := json.Unmarshal([]byte(*s.Metadata), &metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// GetPlanSnapshotFromMetadata 从订阅 metadata 中解析套餐快照（用于保持套餐变更不影响订阅）
func (s *Subscription) GetPlanSnapshotFromMetadata() (*PlanSnapshotData, error) {
	metadata, err := s.GetMetadataAsMap()
	if err != nil {
		return nil, err
	}

	raw, ok := metadata["plan_snapshot"]
	if !ok || raw == nil {
		return nil, nil
	}

	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil, nil
		}
		var snapshot PlanSnapshotData
		if err := json.Unmarshal([]byte(v), &snapshot); err != nil {
			return nil, err
		}
		return &snapshot, nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var snapshot PlanSnapshotData
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return nil, err
		}
		return &snapshot, nil
	}
}

// SetMetadataFromMap 设置元数据
func (s *Subscription) SetMetadataFromMap(metadata map[string]interface{}) error {
	if len(metadata) == 0 {
		s.Metadata = nil
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	str := string(data)
	s.Metadata = &str
	return nil
}

// GetDurationDescription 获取订阅时长描述
func (s *Subscription) GetDurationDescription() string {
	totalSeconds := s.EndAt - s.StartAt
	days := totalSeconds / (24 * 60 * 60)

	if days >= 365 {
		years := days / 365
		return fmt.Sprintf("%d 年", years)
	} else if days >= 30 {
		months := days / 30
		return fmt.Sprintf("%d 个月", months)
	} else if days >= 7 {
		weeks := days / 7
		return fmt.Sprintf("%d 周", weeks)
	} else {
		return fmt.Sprintf("%d 天", days)
	}
}

// ExtendEndAt 延长订阅结束时间
func (s *Subscription) ExtendEndAt(additionalSeconds int64) {
	s.EndAt += additionalSeconds
}

// CanUseModel 检查订阅是否可以使用指定模型
func (s *Subscription) CanUseModel(modelName string) (bool, error) {
	models, err := s.GetModelWhitelistAsSlice()
	if err != nil {
		return false, err
	}

	// 空白名单表示可用所有模型
	if len(models) == 0 {
		return true, nil
	}

	// 检查模型是否在白名单中
	for _, m := range models {
		if m == modelName {
			return true, nil
		}
	}

	return false, nil
}

// GetSubscriptionByRedemptionId 根据兑换码 ID 获取订阅
// 用于幂等性检查：若兑换码已被使用，返回对应的订阅
func GetSubscriptionByRedemptionId(redemptionId int64) (*Subscription, error) {
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}

	var sub Subscription
	err := DB.Preload("Plan").Where("redemption_id = ?", redemptionId).First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到不是错误，返回 nil
		}
		return nil, err
	}

	return &sub, nil
}

// GetSubscriptionByRedemptionIdWithTx 在事务中根据兑换码 ID 获取订阅
func GetSubscriptionByRedemptionIdWithTx(tx *gorm.DB, redemptionId int64) (*Subscription, error) {
	if redemptionId == 0 {
		return nil, errors.New("兑换码 ID 不能为空")
	}

	var sub Subscription
	err := tx.Preload("Plan").Where("redemption_id = ?", redemptionId).First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 未找到不是错误，返回 nil
		}
		return nil, err
	}

	return &sub, nil
}

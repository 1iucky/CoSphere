package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SubscriptionUsage 订阅使用量记录（滚动窗口）
type SubscriptionUsage struct {
	Id             int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	SubscriptionId int64  `json:"subscription_id" gorm:"not null;index;uniqueIndex:idx_subscription_usage_unique,priority:1"`
	Period         string `json:"period" gorm:"type:varchar(32);not null;uniqueIndex:idx_subscription_usage_unique,priority:2"`
	WindowStart    int64  `json:"window_start" gorm:"bigint;not null;uniqueIndex:idx_subscription_usage_unique,priority:3"`
	WindowEnd      int64  `json:"window_end" gorm:"bigint;not null"`
	UsedQuota      int64  `json:"used_quota" gorm:"bigint;not null;default:0"`
	LimitQuota     int64  `json:"limit_quota" gorm:"bigint;not null"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;autoUpdateTime"`
}

func (SubscriptionUsage) TableName() string {
	return "subscription_usages"
}

// ===================== 使用量 CRUD 方法 =====================

// CreateSubscriptionUsage 创建使用量记录
func CreateSubscriptionUsage(usage *SubscriptionUsage) error {
	if err := validateSubscriptionUsage(usage); err != nil {
		return err
	}

	return DB.Create(usage).Error
}

// CreateSubscriptionUsageWithTx 在事务中创建使用量记录
func CreateSubscriptionUsageWithTx(tx *gorm.DB, usage *SubscriptionUsage) error {
	if err := validateSubscriptionUsage(usage); err != nil {
		return err
	}

	return tx.Create(usage).Error
}

// GetOrCreateSubscriptionUsage 获取或创建使用量记录（已弃用，建议使用 GetOrCreateActiveUsageWindow）
// Deprecated: 使用 GetOrCreateActiveUsageWindow 替代，可指定窗口策略
func GetOrCreateSubscriptionUsage(subscriptionId int64, period string, limitQuota int64) (*SubscriptionUsage, error) {
	// 默认使用滚动窗口策略
	return GetOrCreateActiveUsageWindow(subscriptionId, period, limitQuota, common.WindowStrategyRolling)
}

// GetOrCreateSubscriptionUsageWithTx 在事务中获取或创建使用量记录（已弃用，建议使用 GetOrCreateActiveUsageWindowWithTx）
// Deprecated: 使用 GetOrCreateActiveUsageWindowWithTx 替代，可指定窗口策略
func GetOrCreateSubscriptionUsageWithTx(tx *gorm.DB, subscriptionId int64, period string, limitQuota int64) (*SubscriptionUsage, error) {
	// 默认使用滚动窗口策略
	return GetOrCreateActiveUsageWindowWithTx(tx, subscriptionId, period, limitQuota, common.WindowStrategyRolling)
}

// GetSubscriptionUsageById 根据 ID 获取使用量记录
func GetSubscriptionUsageById(id int64) (*SubscriptionUsage, error) {
	if id == 0 {
		return nil, errors.New("使用量记录 ID 不能为空")
	}

	var usage SubscriptionUsage
	err := DB.First(&usage, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("使用量记录不存在")
		}
		return nil, err
	}

	return &usage, nil
}

// GetCurrentUsageBySubscription 获取订阅当前周期的所有使用量
func GetCurrentUsageBySubscription(subscriptionId int64) ([]*SubscriptionUsage, error) {
	if subscriptionId == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()
	var usages []*SubscriptionUsage

	err := DB.Where("subscription_id = ?", subscriptionId).
		Where("window_start <= ? AND window_end >= ?", now, now).
		Find(&usages).Error

	if err != nil {
		return nil, err
	}

	return usages, nil
}

// GetAllUsagesBySubscription 获取订阅的所有使用量记录（分页）
func GetAllUsagesBySubscription(subscriptionId int64, startIdx int, num int) (usages []*SubscriptionUsage, total int64, err error) {
	if subscriptionId == 0 {
		return nil, 0, errors.New("订阅 ID 不能为空")
	}

	query := DB.Model(&SubscriptionUsage{}).Where("subscription_id = ?", subscriptionId)

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 获取分页数据
	err = query.Order("window_start desc").
		Limit(num).
		Offset(startIdx).
		Find(&usages).Error
	if err != nil {
		return nil, 0, err
	}

	return usages, total, nil
}

// DeleteUsagesBySubscription 删除订阅的所有使用量记录
func DeleteUsagesBySubscription(subscriptionId int64) error {
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return DB.Where("subscription_id = ?", subscriptionId).Delete(&SubscriptionUsage{}).Error
}

// DeleteUsagesBySubscriptionWithTx 在事务中删除订阅的所有使用量记录
func DeleteUsagesBySubscriptionWithTx(tx *gorm.DB, subscriptionId int64) error {
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	return tx.Where("subscription_id = ?", subscriptionId).Delete(&SubscriptionUsage{}).Error
}

// ===================== 额度消耗方法 =====================

// ConsumeQuota 消耗额度（原子操作）- 已弃用，建议使用 ConsumeQuotaWithStrategy
// Deprecated: 使用 ConsumeQuotaWithStrategy 替代，可指定窗口策略
func ConsumeQuota(subscriptionId int64, period string, amount int64, limitQuota int64) error {
	// 默认使用滚动窗口策略
	return ConsumeQuotaWithStrategy(subscriptionId, period, amount, limitQuota, common.WindowStrategyRolling)
}

// ConsumeQuotaWithTx 在事务中消耗额度
func ConsumeQuotaWithTx(tx *gorm.DB, usageId int64, amount int64) error {
	if amount <= 0 {
		return errors.New("消耗额度必须为正数")
	}

	result := tx.Model(&SubscriptionUsage{}).
		Where("id = ? AND used_quota + ? <= limit_quota", usageId, amount).
		Update("used_quota", gorm.Expr("used_quota + ?", amount))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New(common.MsgSubscriptionQuotaExhausted)
	}

	return nil
}

// CheckAndConsumeQuota 检查并消耗额度（返回是否成功）- 已弃用，建议使用 CheckAndConsumeQuotaWithStrategy
// Deprecated: 使用 CheckAndConsumeQuotaWithStrategy 替代，可指定窗口策略
func CheckAndConsumeQuota(subscriptionId int64, period string, amount int64, limitQuota int64) (bool, error) {
	// 默认使用滚动窗口策略
	return CheckAndConsumeQuotaWithStrategy(subscriptionId, period, amount, limitQuota, common.WindowStrategyRolling)
}

// GetRemainingQuota 获取剩余额度（支持滚动窗口和固定窗口）
func GetRemainingQuota(subscriptionId int64, period string) (int64, error) {
	// 使用 GetActiveUsageWindow 以支持滚动窗口
	usage, err := GetActiveUsageWindow(subscriptionId, period)
	if err != nil {
		return 0, err
	}

	if usage == nil {
		return -1, nil // -1 表示没有活跃窗口（可能是无限额度或未初始化）
	}

	return usage.LimitQuota - usage.UsedQuota, nil
}

// GetUsagePercentage 获取使用量百分比（支持滚动窗口和固定窗口）
func GetUsagePercentage(subscriptionId int64, period string) (float64, error) {
	// 使用 GetActiveUsageWindow 以支持滚动窗口
	usage, err := GetActiveUsageWindow(subscriptionId, period)
	if err != nil {
		return 0, err
	}

	if usage == nil || usage.LimitQuota == 0 {
		return 0, nil
	}

	return float64(usage.UsedQuota) / float64(usage.LimitQuota), nil
}

// RefundQuota 退还额度
func RefundQuota(usageId int64, amount int64) error {
	if amount <= 0 {
		return errors.New("退还额度必须为正数")
	}

	result := DB.Model(&SubscriptionUsage{}).
		Where("id = ? AND used_quota >= ?", usageId, amount).
		Update("used_quota", gorm.Expr("used_quota - ?", amount))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("退还额度失败：可能超出已使用额度")
	}

	return nil
}

// RefundQuotaWithTx 在事务中退还额度
func RefundQuotaWithTx(tx *gorm.DB, usageId int64, amount int64) error {
	if amount <= 0 {
		return errors.New("退还额度必须为正数")
	}

	result := tx.Model(&SubscriptionUsage{}).
		Where("id = ? AND used_quota >= ?", usageId, amount).
		Update("used_quota", gorm.Expr("used_quota - ?", amount))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("退还额度失败：可能超出已使用额度")
	}

	return nil
}

// ===================== 窗口计算方法 =====================

// CalculateWindowBounds 计算固定窗口边界（仅用于自然日等固定窗口场景）
// 注意：滚动窗口应使用 GetOrCreateActiveUsageWindow，窗口开始时间由首次使用触发
func CalculateWindowBounds(period string, timestamp int64) (windowStart int64, windowEnd int64) {
	return CalculateFixedWindowBounds(period, timestamp)
}

// CalculateFixedWindowBounds 计算固定/自然窗口边界
// 仅用于固定窗口场景（如自然日、自然周、自然月）
// 滚动窗口的开始时间由用户首次使用触发，不通过此函数计算
func CalculateFixedWindowBounds(period string, timestamp int64) (windowStart int64, windowEnd int64) {
	t := time.Unix(timestamp, 0).UTC()

	switch period {
	case common.LimitPeriodFiveHours:
		// 固定窗口：以整5小时为边界
		hour := t.Hour()
		windowHour := (hour / 5) * 5
		windowStart = time.Date(t.Year(), t.Month(), t.Day(), windowHour, 0, 0, 0, time.UTC).Unix()
		windowEnd = windowStart + 5*60*60 - 1

	case common.LimitPeriodDay:
		// 每日窗口：自然日（UTC 00:00 - 23:59:59）
		windowStart = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix()
		windowEnd = windowStart + 24*60*60 - 1

	case common.LimitPeriodWeek:
		// 固定窗口：周一 00:00 - 周日 23:59:59
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7 // 周日作为第7天
		}
		daysSinceMonday := weekday - 1
		monday := t.AddDate(0, 0, -daysSinceMonday)
		windowStart = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, time.UTC).Unix()
		windowEnd = windowStart + 7*24*60*60 - 1

	case common.LimitPeriodMonth:
		// 固定窗口：自然月
		windowStart = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
		nextMonth := t.AddDate(0, 1, 0)
		windowEnd = time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, time.UTC).Unix() - 1

	default:
		// 默认使用每日窗口
		windowStart = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix()
		windowEnd = windowStart + 24*60*60 - 1
	}

	return windowStart, windowEnd
}

// CalculateRollingWindowEnd 计算滚动窗口的结束时间
// 滚动窗口：窗口开始时间 = 用户首次使用的时间，窗口结束时间 = 开始时间 + 周期时长
func CalculateRollingWindowEnd(period string, windowStart int64) int64 {
	duration := GetWindowDurationSeconds(period)
	return windowStart + duration - 1
}

// GetWindowDurationSeconds 获取窗口时长（秒）
func GetWindowDurationSeconds(period string) int64 {
	switch period {
	case common.LimitPeriodFiveHours:
		return 5 * 60 * 60
	case common.LimitPeriodDay:
		return 24 * 60 * 60
	case common.LimitPeriodWeek:
		return 7 * 24 * 60 * 60
	case common.LimitPeriodMonth:
		return 30 * 24 * 60 * 60 // 近似值
	default:
		return 24 * 60 * 60
	}
}

// ===================== 滚动窗口按需触发方法 =====================
//
// 滚动窗口核心逻辑：
// 1. 窗口开始时间 = 用户首次实际使用计费的时间（不是订阅开始时间）
// 2. 窗口结束时间 = 窗口开始时间 + 周期时长
// 3. 窗口结束后不会立即开始新窗口
// 4. 新窗口开始时间 = 上个窗口结束后，用户下次首次使用计费的时间
//
// 例如：
// - 用户11点开通订阅，15点首次使用
// - 5小时窗口的 window_start = 15:00, window_end = 20:00
// - 用户在 20:00 后（比如21:00）再次使用时，创建新窗口
// - 新窗口的 window_start = 21:00, window_end = 02:00（次日）

// GetActiveUsageWindow 获取当前活跃的使用量窗口（不创建）
// 返回 nil 表示没有活跃窗口
func GetActiveUsageWindow(subscriptionId int64, period string) (*SubscriptionUsage, error) {
	if subscriptionId == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()
	var usage SubscriptionUsage

	// 查找活跃窗口：window_end >= now
	err := DB.Where("subscription_id = ? AND period = ? AND window_end >= ?",
		subscriptionId, period, now).
		Order("window_start DESC"). // 取最新的
		First(&usage).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 没有活跃窗口
		}
		return nil, err
	}

	return &usage, nil
}

// GetActiveUsageWindowWithTx 在事务中获取当前活跃的使用量窗口（不创建）
func GetActiveUsageWindowWithTx(tx *gorm.DB, subscriptionId int64, period string) (*SubscriptionUsage, error) {
	if subscriptionId == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()
	var usage SubscriptionUsage

	err := tx.Where("subscription_id = ? AND period = ? AND window_end >= ?",
		subscriptionId, period, now).
		Order("window_start DESC").
		First(&usage).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return &usage, nil
}

// GetOrCreateActiveUsageWindow 获取或创建活跃的使用量窗口（滚动窗口按需触发）
// strategy: 窗口策略 (rolling/fixed)
//   - rolling: 滚动窗口 - 窗口开始时间为首次使用时间
//   - fixed: 固定窗口 - 窗口开始时间为自然边界（如自然日、自然周）
func GetOrCreateActiveUsageWindow(subscriptionId int64, period string, limitQuota int64, strategy string) (*SubscriptionUsage, error) {
	if subscriptionId == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()

	// 1. 尝试获取活跃窗口
	usage, err := GetActiveUsageWindow(subscriptionId, period)
	if err != nil {
		return nil, err
	}

	if usage != nil {
		return usage, nil
	}

	// 2. 没有活跃窗口，创建新窗口
	var windowStart, windowEnd int64
	if strategy == common.WindowStrategyRolling {
		// 滚动窗口：window_start = 当前时间（首次使用触发）
		windowStart = now
		windowEnd = CalculateRollingWindowEnd(period, windowStart)
	} else {
		// 固定窗口：使用自然边界
		windowStart, windowEnd = CalculateFixedWindowBounds(period, now)
	}

	newUsage := SubscriptionUsage{
		SubscriptionId: subscriptionId,
		Period:         period,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		UsedQuota:      0,
		LimitQuota:     limitQuota,
	}

	// 使用 upsert 避免并发冲突
	err = DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscription_id"}, {Name: "period"}, {Name: "window_start"}},
		DoNothing: true,
	}).Create(&newUsage).Error

	if err != nil {
		return nil, err
	}

	// 重新获取记录（可能是并发创建的）
	return GetActiveUsageWindow(subscriptionId, period)
}

// GetOrCreateActiveUsageWindowWithTx 在事务中获取或创建活跃的使用量窗口
// 修复并发问题：
// 1. 先用 FOR UPDATE 锁定活跃窗口查询
// 2. 若无活跃窗口，锁定 subscriptions 表对应行，确保同一订阅的窗口创建串行化
// 3. 锁定后重新计算 now，避免跨时间边界问题
func GetOrCreateActiveUsageWindowWithTx(tx *gorm.DB, subscriptionId int64, period string, limitQuota int64, strategy string) (*SubscriptionUsage, error) {
	if subscriptionId == 0 {
		return nil, errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()

	// 1. 使用 FOR UPDATE 锁定查询，防止并发创建多个窗口
	// 查询当前周期是否存在活跃窗口（window_end >= now）
	var usage SubscriptionUsage
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("subscription_id = ? AND period = ? AND window_end >= ?",
			subscriptionId, period, now).
		Order("window_start DESC").
		First(&usage).Error

	if err == nil {
		// 找到活跃窗口，直接返回
		return &usage, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		// 其他错误
		return nil, err
	}

	// 2. 没有活跃窗口时，先锁定 subscriptions 表对应订阅行
	// 这确保同一订阅的窗口创建操作串行化，避免多活窗口问题
	var subscription Subscription
	lockErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", subscriptionId).
		First(&subscription).Error
	if lockErr != nil {
		// 如果找不到订阅，仍然尝试创建（可能是外部调用）
		if !errors.Is(lockErr, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("锁定订阅行失败: %w", lockErr)
		}
	}

	// 3. 锁定后重新计算 now，避免等待锁期间跨越时间边界导致窗口计算错误
	// 特别是 fixed 策略下的日/周/月边界
	now = common.GetTimestamp()

	// 4. 再次检查是否有活跃窗口（可能在等待锁期间被其他事务创建）
	// 使用更新后的 now 进行检查
	err = tx.Where("subscription_id = ? AND period = ? AND window_end >= ?",
		subscriptionId, period, now).
		Order("window_start DESC").
		First(&usage).Error
	if err == nil {
		// 其他事务已创建，直接返回
		return &usage, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// 5. 确认没有活跃窗口，创建新窗口（使用更新后的 now）
	var windowStart, windowEnd int64
	if strategy == common.WindowStrategyRolling {
		// 滚动窗口：window_start = 当前时间（首次使用触发）
		windowStart = now
		windowEnd = CalculateRollingWindowEnd(period, windowStart)
	} else {
		// 固定窗口：使用自然边界
		windowStart, windowEnd = CalculateFixedWindowBounds(period, now)
	}

	newUsage := SubscriptionUsage{
		SubscriptionId: subscriptionId,
		Period:         period,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		UsedQuota:      0,
		LimitQuota:     limitQuota,
	}

	// 使用 upsert 作为额外保护（主要依赖上面的锁机制）
	err = tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "subscription_id"}, {Name: "period"}, {Name: "window_start"}},
		DoNothing: true,
	}).Create(&newUsage).Error

	if err != nil {
		return nil, err
	}

	// 重新获取记录（确保返回数据库中的记录，而非本地创建的）
	err = tx.Where("subscription_id = ? AND period = ? AND window_end >= ?",
		subscriptionId, period, now).
		Order("window_start DESC").
		First(&usage).Error

	if err != nil {
		return nil, fmt.Errorf("获取新创建的窗口失败: %w", err)
	}

	return &usage, nil
}

// ConsumeQuotaWithStrategy 带策略的额度消耗（原子操作）
// strategy: 窗口策略 (rolling/fixed)
func ConsumeQuotaWithStrategy(subscriptionId int64, period string, amount int64, limitQuota int64, strategy string) error {
	if amount <= 0 {
		return errors.New("消耗额度必须为正数")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		usage, err := GetOrCreateActiveUsageWindowWithTx(tx, subscriptionId, period, limitQuota, strategy)
		if err != nil {
			return err
		}

		if usage == nil {
			return errors.New("无法创建使用量窗口")
		}

		// 检查是否超额
		if usage.UsedQuota+amount > usage.LimitQuota {
			return errors.New(common.MsgSubscriptionQuotaExhausted)
		}

		// 原子更新
		result := tx.Model(&SubscriptionUsage{}).
			Where("id = ? AND used_quota + ? <= limit_quota", usage.Id, amount).
			Update("used_quota", gorm.Expr("used_quota + ?", amount))

		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return errors.New(common.MsgSubscriptionQuotaExhausted)
		}

		return nil
	})
}

// CheckAndConsumeQuotaWithStrategy 检查并消耗额度（返回是否成功）
func CheckAndConsumeQuotaWithStrategy(subscriptionId int64, period string, amount int64, limitQuota int64, strategy string) (bool, error) {
	err := ConsumeQuotaWithStrategy(subscriptionId, period, amount, limitQuota, strategy)
	if err != nil {
		if err.Error() == common.MsgSubscriptionQuotaExhausted {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsWindowExpired 检查窗口是否已过期
func IsWindowExpired(usage *SubscriptionUsage) bool {
	if usage == nil {
		return true
	}
	now := common.GetTimestamp()
	return now > usage.WindowEnd
}

// GetTimeUntilWindowEnd 获取距离窗口结束的剩余时间（秒）
func GetTimeUntilWindowEnd(usage *SubscriptionUsage) int64 {
	if usage == nil {
		return 0
	}
	now := common.GetTimestamp()
	remaining := usage.WindowEnd - now
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IsWindowActive 检查窗口是否在有效期内
func (u *SubscriptionUsage) IsWindowActive() bool {
	now := common.GetTimestamp()
	return u.WindowStart <= now && u.WindowEnd >= now
}

// ===================== 验证方法 =====================

// validateSubscriptionUsage 验证使用量记录数据
func validateSubscriptionUsage(usage *SubscriptionUsage) error {
	if usage.SubscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	validPeriods := map[string]bool{
		common.LimitPeriodFiveHours: true,
		common.LimitPeriodDay:       true,
		common.LimitPeriodWeek:      true,
		common.LimitPeriodMonth:     true,
	}
	if !validPeriods[usage.Period] {
		return errors.New("无效的周期类型")
	}

	if usage.WindowStart == 0 {
		return errors.New("窗口开始时间不能为空")
	}

	if usage.WindowEnd == 0 {
		return errors.New("窗口结束时间不能为空")
	}

	if usage.WindowEnd <= usage.WindowStart {
		return errors.New("窗口结束时间必须晚于开始时间")
	}

	if usage.LimitQuota < 0 {
		return errors.New("限额不能为负数")
	}

	if usage.UsedQuota < 0 {
		return errors.New("已用额度不能为负数")
	}

	return nil
}

// ===================== 辅助方法 =====================

// HasAvailableQuota 检查是否有可用额度
func (u *SubscriptionUsage) HasAvailableQuota(amount int64) bool {
	return u.UsedQuota+amount <= u.LimitQuota
}

// GetRemainingQuotaValue 获取剩余额度值
func (u *SubscriptionUsage) GetRemainingQuotaValue() int64 {
	if u.LimitQuota <= u.UsedQuota {
		return 0
	}
	return u.LimitQuota - u.UsedQuota
}

// GetUsageRatio 获取使用率
func (u *SubscriptionUsage) GetUsageRatio() float64 {
	if u.LimitQuota == 0 {
		return 0
	}
	return float64(u.UsedQuota) / float64(u.LimitQuota)
}

// IsQuotaLow 检查额度是否低于阈值
func (u *SubscriptionUsage) IsQuotaLow(threshold float64) bool {
	if u.LimitQuota == 0 {
		return false
	}
	return u.GetUsageRatio() >= (1 - threshold)
}

// CleanupExpiredUsages 清理过期的使用量记录
func CleanupExpiredUsages(retentionDays int) (int64, error) {
	if retentionDays < 1 {
		retentionDays = 30 // 默认保留30天
	}

	cutoffTime := common.GetTimestamp() - int64(retentionDays*24*60*60)

	result := DB.Where("window_end < ?", cutoffTime).Delete(&SubscriptionUsage{})
	if result.Error != nil {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}

// GetUsageSummaryBySubscription 获取订阅的使用量汇总
func GetUsageSummaryBySubscription(subscriptionId int64) (map[string]*SubscriptionUsage, error) {
	usages, err := GetCurrentUsageBySubscription(subscriptionId)
	if err != nil {
		return nil, err
	}

	summary := make(map[string]*SubscriptionUsage)
	for _, u := range usages {
		summary[u.Period] = u
	}

	return summary, nil
}

// BatchGetCurrentUsages 批量获取多个订阅的当前使用量
func BatchGetCurrentUsages(subscriptionIds []int64) (map[int64][]*SubscriptionUsage, error) {
	if len(subscriptionIds) == 0 {
		return map[int64][]*SubscriptionUsage{}, nil
	}

	now := common.GetTimestamp()
	var usages []*SubscriptionUsage

	err := DB.Where("subscription_id IN ?", subscriptionIds).
		Where("window_start <= ? AND window_end >= ?", now, now).
		Find(&usages).Error

	if err != nil {
		return nil, err
	}

	result := make(map[int64][]*SubscriptionUsage)
	for _, u := range usages {
		result[u.SubscriptionId] = append(result[u.SubscriptionId], u)
	}

	return result, nil
}

// ===================== 额外管理方法 =====================

// UpdateUsageWindow 更新使用量窗口（用于管理员手动调整）
func UpdateUsageWindow(usageId int64, usedQuota int64, limitQuota int64) error {
	if usageId == 0 {
		return errors.New("使用量记录 ID 不能为空")
	}

	updates := map[string]interface{}{}
	if usedQuota >= 0 {
		updates["used_quota"] = usedQuota
	}
	if limitQuota > 0 {
		updates["limit_quota"] = limitQuota
	}

	if len(updates) == 0 {
		return errors.New("没有可更新的字段")
	}

	return DB.Model(&SubscriptionUsage{}).Where("id = ?", usageId).Updates(updates).Error
}

// UpdateUsageWindowWithTx 在事务中更新使用量窗口
func UpdateUsageWindowWithTx(tx *gorm.DB, usageId int64, usedQuota int64, limitQuota int64) error {
	if usageId == 0 {
		return errors.New("使用量记录 ID 不能为空")
	}

	updates := map[string]interface{}{}
	if usedQuota >= 0 {
		updates["used_quota"] = usedQuota
	}
	if limitQuota > 0 {
		updates["limit_quota"] = limitQuota
	}

	if len(updates) == 0 {
		return errors.New("没有可更新的字段")
	}

	return tx.Model(&SubscriptionUsage{}).Where("id = ?", usageId).Updates(updates).Error
}

// ResetUsageWindow 重置使用量窗口（清零使用量）
func ResetUsageWindow(subscriptionId int64, period string) error {
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()
	return DB.Model(&SubscriptionUsage{}).
		Where("subscription_id = ? AND period = ? AND window_end >= ?", subscriptionId, period, now).
		Update("used_quota", 0).Error
}

// ResetUsageWindowWithTx 在事务中重置使用量窗口
func ResetUsageWindowWithTx(tx *gorm.DB, subscriptionId int64, period string) error {
	if subscriptionId == 0 {
		return errors.New("订阅 ID 不能为空")
	}

	now := common.GetTimestamp()
	return tx.Model(&SubscriptionUsage{}).
		Where("subscription_id = ? AND period = ? AND window_end >= ?", subscriptionId, period, now).
		Update("used_quota", 0).Error
}

// AtomicIncrementUsage 原子增加使用量（直接操作，不检查限额）
func AtomicIncrementUsage(usageId int64, amount int64) error {
	if usageId == 0 {
		return errors.New("使用量记录 ID 不能为空")
	}
	if amount <= 0 {
		return errors.New("增量必须为正数")
	}

	return DB.Model(&SubscriptionUsage{}).
		Where("id = ?", usageId).
		Update("used_quota", gorm.Expr("used_quota + ?", amount)).Error
}

// AtomicIncrementUsageWithTx 在事务中原子增加使用量
func AtomicIncrementUsageWithTx(tx *gorm.DB, usageId int64, amount int64) error {
	if usageId == 0 {
		return errors.New("使用量记录 ID 不能为空")
	}
	if amount <= 0 {
		return errors.New("增量必须为正数")
	}

	return tx.Model(&SubscriptionUsage{}).
		Where("id = ?", usageId).
		Update("used_quota", gorm.Expr("used_quota + ?", amount)).Error
}

// AtomicDecrementUsage 原子减少使用量（用于退款等场景）
func AtomicDecrementUsage(usageId int64, amount int64) error {
	if usageId == 0 {
		return errors.New("使用量记录 ID 不能为空")
	}
	if amount <= 0 {
		return errors.New("减量必须为正数")
	}

	result := DB.Model(&SubscriptionUsage{}).
		Where("id = ? AND used_quota >= ?", usageId, amount).
		Update("used_quota", gorm.Expr("used_quota - ?", amount))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("减少失败：可能超出已使用额度")
	}

	return nil
}

// AtomicDecrementUsageWithTx 在事务中原子减少使用量
func AtomicDecrementUsageWithTx(tx *gorm.DB, usageId int64, amount int64) error {
	if usageId == 0 {
		return errors.New("使用量记录 ID 不能为空")
	}
	if amount <= 0 {
		return errors.New("减量必须为正数")
	}

	result := tx.Model(&SubscriptionUsage{}).
		Where("id = ? AND used_quota >= ?", usageId, amount).
		Update("used_quota", gorm.Expr("used_quota - ?", amount))

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("减少失败：可能超出已使用额度")
	}

	return nil
}

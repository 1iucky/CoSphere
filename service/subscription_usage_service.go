package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"gorm.io/gorm"
)

// SubscriptionUsageService 使用量服务（滚动窗口核心逻辑）
type SubscriptionUsageService struct {
	// 预扣上下文缓存（用于回滚）
	preConsumeCache sync.Map // key: contextId, value: *PreConsumeContext
	// contextId 序列号（确保唯一性）
	contextSeq uint64
}

var usageService = &SubscriptionUsageService{}

// GetSubscriptionUsageService 获取使用量服务实例
func GetSubscriptionUsageService() *SubscriptionUsageService {
	return usageService
}

// ===================== 预扣上下文 =====================

// PeriodConfig 周期配置（支持每周期不同策略）
type PeriodConfig struct {
	LimitQuota int64  `json:"limit_quota"` // 限额
	Strategy   string `json:"strategy"`    // 窗口策略 (rolling/fixed/natural)
}

// PreConsumeContext 预扣上下文（用于后续回滚或调整）
type PreConsumeContext struct {
	ContextId      string                       `json:"context_id"`       // 上下文 ID
	SubscriptionId int64                        `json:"subscription_id"`  // 订阅 ID
	Amount         int64                        `json:"amount"`           // 预扣额度
	Timestamp      int64                        `json:"timestamp"`        // 预扣时间
	Usages         map[string]*UsagePreConsume  `json:"usages"`           // 各周期的预扣记录
}

// UsagePreConsume 单个周期的预扣记录
type UsagePreConsume struct {
	UsageId     int64  `json:"usage_id"`     // 使用量记录 ID
	Period      string `json:"period"`       // 周期类型
	Amount      int64  `json:"amount"`       // 预扣额度
	PreUsed     int64  `json:"pre_used"`     // 预扣前已用额度
	WindowStart int64  `json:"window_start"` // 窗口开始时间
	WindowEnd   int64  `json:"window_end"`   // 窗口结束时间
}

// ===================== Redis 分布式上下文存储 =====================

const (
	// preConsumeRedisKeyPrefix Redis 存储预扣上下文的 key 前缀
	preConsumeRedisKeyPrefix = "pre_consume:"
	// preConsumeContextTTL 预扣上下文默认过期时间（5分钟）
	preConsumeContextTTL = 5 * time.Minute

	// usageCacheKeyPrefix Redis 存储使用量缓存的 key 前缀
	// 格式: subscription:usage:<subscription_id>
	usageCacheKeyPrefix = "subscription:usage:"
	// usageCacheDefaultTTL 使用量缓存默认过期时间（1小时，会被 end_at 覆盖）
	usageCacheDefaultTTL = 1 * time.Hour

	// activeSubCacheKeyPrefix Redis 存储活跃订阅列表的 key 前缀
	// 格式: subscription:active:<user_id>
	activeSubCacheKeyPrefix = "subscription:active:"
)

// UsageCacheEntry 使用量缓存条目
type UsageCacheEntry struct {
	WindowStart int64 `json:"window_start"`
	WindowEnd   int64 `json:"window_end"`
	UsedQuota   int64 `json:"used"`
	LimitQuota  int64 `json:"limit"`
}

// refreshUsageCache 刷新订阅使用量缓存
// 在 usage 更新后调用，TTL 与 window_end 对齐
func (s *SubscriptionUsageService) refreshUsageCache(subscriptionId int64, usages []*model.SubscriptionUsage) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}

	if len(usages) == 0 {
		return
	}

	// 构建缓存数据：period -> UsageCacheEntry
	cacheData := make(map[string]UsageCacheEntry)
	var maxWindowEnd int64 = 0

	for _, usage := range usages {
		cacheData[usage.Period] = UsageCacheEntry{
			WindowStart: usage.WindowStart,
			WindowEnd:   usage.WindowEnd,
			UsedQuota:   usage.UsedQuota,
			LimitQuota:  usage.LimitQuota,
		}
		// 找到最大的 window_end 用于设置 TTL
		if usage.WindowEnd > maxWindowEnd {
			maxWindowEnd = usage.WindowEnd
		}
	}

	// 序列化
	data, err := json.Marshal(cacheData)
	if err != nil {
		common.SysError(fmt.Sprintf("序列化使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
		return
	}

	// 计算 TTL：与最大 window_end 对齐，至少 1 分钟
	now := common.GetTimestamp()
	ttlSeconds := maxWindowEnd - now
	if ttlSeconds < 60 {
		ttlSeconds = 60
	}
	// 最大不超过 30 天
	if ttlSeconds > 30*24*3600 {
		ttlSeconds = 30 * 24 * 3600
	}

	// 写入 Redis
	redisKey := fmt.Sprintf("%s%d", usageCacheKeyPrefix, subscriptionId)
	if err := common.RedisSet(redisKey, string(data), time.Duration(ttlSeconds)*time.Second); err != nil {
		common.SysError(fmt.Sprintf("写入使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
	}
}

// getUsageFromCache 从缓存获取使用量
// 返回 nil 表示缓存未命中，需要从数据库读取
func (s *SubscriptionUsageService) getUsageFromCache(subscriptionId int64) (map[string]UsageCacheEntry, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, false
	}

	redisKey := fmt.Sprintf("%s%d", usageCacheKeyPrefix, subscriptionId)
	data, err := common.RedisGet(redisKey)
	if err != nil || data == "" {
		return nil, false
	}

	var cacheData map[string]UsageCacheEntry
	if err := json.Unmarshal([]byte(data), &cacheData); err != nil {
		common.SysError(fmt.Sprintf("反序列化使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
		return nil, false
	}

	// 验证缓存数据有效性（窗口未过期）
	now := common.GetTimestamp()
	validData := make(map[string]UsageCacheEntry)
	for period, entry := range cacheData {
		if entry.WindowEnd >= now {
			validData[period] = entry
		}
	}

	if len(validData) == 0 {
		return nil, false
	}

	return validData, true
}

// invalidateUsageCache 清除订阅使用量缓存
func (s *SubscriptionUsageService) invalidateUsageCache(subscriptionId int64) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}

	redisKey := fmt.Sprintf("%s%d", usageCacheKeyPrefix, subscriptionId)
	if err := common.RedisDel(redisKey); err != nil {
		common.SysError(fmt.Sprintf("清除使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
	}
}

// ===================== 活跃订阅缓存 (subscription:active:<user_id>) =====================
// 注意：活跃订阅缓存由 SubscriptionCacheService 统一管理
// 本服务仅提供兼容性方法，实际操作委托给 SubscriptionCacheService

// ActiveSubCacheEntry 活跃订阅缓存条目（兼容性类型别名）
// 实际缓存结构为 CachedSubscription，此类型仅用于向后兼容
type ActiveSubCacheEntry = CachedSubscription

// RefreshActiveSubCache 刷新用户的活跃订阅缓存
// 委托给 SubscriptionCacheService，确保缓存结构和 TTL 策略统一
func (s *SubscriptionUsageService) RefreshActiveSubCache(userId int, subscriptions []*model.Subscription) {
	// 委托给 SubscriptionCacheService 统一管理
	GetSubscriptionCacheService().InvalidateUserCache(int64(userId))
}

// GetActiveSubFromCache 从缓存获取用户的活跃订阅列表
// 委托给 SubscriptionCacheService，返回已过滤过期订阅的数据
func (s *SubscriptionUsageService) GetActiveSubFromCache(userId int) ([]ActiveSubCacheEntry, bool) {
	items, fromCache, _ := GetSubscriptionPriorityService().GetCachedActiveSubscriptions(int64(userId))
	return items, fromCache
}

// InvalidateActiveSubCache 刷新用户的活跃订阅缓存
// 注意：此方法实际执行的是"刷新"（从数据库重新加载并写入缓存），而非简单删除
// 方法名保留 Invalidate 是为了向后兼容，语义上等同于 RefreshActiveSubCache
// 委托给 SubscriptionCacheService
func (s *SubscriptionUsageService) InvalidateActiveSubCache(userId int) {
	GetSubscriptionCacheService().InvalidateUserCache(int64(userId))
}

// storeContext 存储预扣上下文
// 当 Redis 启用时，必须成功写入 Redis，否则返回错误（保证跨实例回滚一致性）
// 当 Redis 未启用时，使用内存存储（单实例场景）
func (s *SubscriptionUsageService) storeContext(contextId string, ctx *PreConsumeContext) error {
	// 优先使用 Redis（支持跨实例）
	if common.RedisEnabled && common.RDB != nil {
		data, err := json.Marshal(ctx)
		if err != nil {
			common.SysError(fmt.Sprintf("序列化预扣上下文失败 [%s]: %v", contextId, err))
			// 序列化失败是代码 bug，返回错误
			return fmt.Errorf("序列化预扣上下文失败: %w", err)
		}
		redisKey := preConsumeRedisKeyPrefix + contextId
		if err := common.RedisSet(redisKey, string(data), preConsumeContextTTL); err != nil {
			common.SysError(fmt.Sprintf("Redis 存储预扣上下文失败 [%s]: %v", contextId, err))
			// Redis 启用但写入失败时，返回错误而非降级
			// 原因：如果降级到内存，其他实例无法通过 contextId 回滚，
			// 这会导致跨实例回滚失败，破坏数据一致性
			return fmt.Errorf("Redis 存储预扣上下文失败: %w", err)
		}
		return nil
	}
	// Redis 不可用时使用内存（单实例场景）
	s.preConsumeCache.Store(contextId, ctx)
	return nil
}

// loadContext 加载预扣上下文（优先从 Redis，降级从内存）
func (s *SubscriptionUsageService) loadContext(contextId string) (*PreConsumeContext, bool) {
	// 优先从 Redis 加载
	if common.RedisEnabled && common.RDB != nil {
		redisKey := preConsumeRedisKeyPrefix + contextId
		data, err := common.RedisGet(redisKey)
		if err == nil && data != "" {
			var ctx PreConsumeContext
			if jsonErr := json.Unmarshal([]byte(data), &ctx); jsonErr == nil {
				return &ctx, true
			} else {
				common.SysError(fmt.Sprintf("反序列化预扣上下文失败 [%s]: %v", contextId, jsonErr))
			}
		}
		// Redis 中找不到，尝试从内存降级读取（兼容旧数据）
	}
	// 从内存加载
	val, ok := s.preConsumeCache.Load(contextId)
	if !ok {
		return nil, false
	}
	ctx, ok := val.(*PreConsumeContext)
	return ctx, ok
}

// deleteContext 删除预扣上下文（同时删除 Redis 和内存）
func (s *SubscriptionUsageService) deleteContext(contextId string) {
	// 删除 Redis 中的数据
	if common.RedisEnabled && common.RDB != nil {
		redisKey := preConsumeRedisKeyPrefix + contextId
		if err := common.RedisDel(redisKey); err != nil {
			common.SysError(fmt.Sprintf("Redis 删除预扣上下文失败 [%s]: %v", contextId, err))
		}
	}
	// 同时删除内存中的数据（确保一致性）
	s.preConsumeCache.Delete(contextId)
}

// ===================== 窗口计算方法 =====================

// CalculateWindowStart 计算窗口开始时间（支持滚动和固定窗口）
func (s *SubscriptionUsageService) CalculateWindowStart(period string, strategy string, timestamp int64) int64 {
	if strategy == common.WindowStrategyRolling {
		// 滚动窗口：开始时间 = 触发时间（用户首次使用）
		return timestamp
	}

	// 固定窗口：使用自然边界
	windowStart, _ := model.CalculateFixedWindowBounds(period, timestamp)
	return windowStart
}

// CalculateWindowEnd 计算窗口结束时间
func (s *SubscriptionUsageService) CalculateWindowEnd(period string, strategy string, windowStart int64) int64 {
	if strategy == common.WindowStrategyRolling {
		// 滚动窗口：结束时间 = 开始时间 + 周期时长
		return model.CalculateRollingWindowEnd(period, windowStart)
	}

	// 固定窗口：使用自然边界
	_, windowEnd := model.CalculateFixedWindowBounds(period, windowStart)
	return windowEnd
}

// ===================== 窗口状态判定 =====================

// IsWindowExpired 检查窗口是否已过期
func (s *SubscriptionUsageService) IsWindowExpired(usage *model.SubscriptionUsage) bool {
	return model.IsWindowExpired(usage)
}

// ShouldResetWindow 是否需要刷新窗口（窗口已过期）
func (s *SubscriptionUsageService) ShouldResetWindow(usage *model.SubscriptionUsage) bool {
	return s.IsWindowExpired(usage)
}

// GetTimeUntilWindowEnd 获取距离窗口结束的剩余时间（秒）
func (s *SubscriptionUsageService) GetTimeUntilWindowEnd(usage *model.SubscriptionUsage) int64 {
	return model.GetTimeUntilWindowEnd(usage)
}

// ===================== 原子预扣接口 =====================

// PeriodPriority 周期优先级顺序（按需求文档：按周期顺序预扣）
// 从小周期到大周期，先扣小周期
var PeriodPriority = []string{
	common.LimitPeriodFiveHours, // 5小时
	common.LimitPeriodDay,       // 日
	common.LimitPeriodWeek,      // 周
	common.LimitPeriodMonth,     // 月
}

// getSortedPeriods 获取按优先级排序的周期列表（用于 map[string]int64）
func (s *SubscriptionUsageService) getSortedPeriods(periods map[string]int64) []string {
	// 按 PeriodPriority 顺序返回存在的周期
	var sorted []string
	for _, p := range PeriodPriority {
		if _, exists := periods[p]; exists {
			sorted = append(sorted, p)
		}
	}
	// 追加不在标准列表中的周期（如有）
	for p := range periods {
		found := false
		for _, sp := range sorted {
			if sp == p {
				found = true
				break
			}
		}
		if !found {
			sorted = append(sorted, p)
		}
	}
	return sorted
}

// getSortedPeriodConfigs 获取按优先级排序的周期列表（用于 map[string]PeriodConfig）
func (s *SubscriptionUsageService) getSortedPeriodConfigs(periodConfigs map[string]PeriodConfig) []string {
	var sorted []string
	for _, p := range PeriodPriority {
		if _, exists := periodConfigs[p]; exists {
			sorted = append(sorted, p)
		}
	}
	// 追加不在标准列表中的周期
	for p := range periodConfigs {
		found := false
		for _, sp := range sorted {
			if sp == p {
				found = true
				break
			}
		}
		if !found {
			sorted = append(sorted, p)
		}
	}
	return sorted
}

// generateContextId 生成唯一的上下文 ID
// 使用原子序列号 + 时间戳 + 随机字节，确保高并发下的唯一性
func (s *SubscriptionUsageService) generateContextId(subscriptionId int64) string {
	seq := atomic.AddUint64(&s.contextSeq, 1)
	randBytes := make([]byte, 4)
	if _, err := rand.Read(randBytes); err != nil {
		// 极端情况：随机数生成失败，使用序列号的额外变体作为后备
		// 这种情况在正常运行时几乎不会发生（系统熵池耗尽），但仍需防御
		common.SysError(fmt.Sprintf("generateContextId: rand.Read 失败: %v，使用后备方案", err))
		// 使用更多序列号位作为后备随机性
		fallbackSeq := atomic.AddUint64(&s.contextSeq, 1)
		return fmt.Sprintf("ctx_%d_%d_%d_fb%016x", subscriptionId, time.Now().UnixNano(), seq, fallbackSeq)
	}
	return fmt.Sprintf("ctx_%d_%d_%d_%s", subscriptionId, time.Now().UnixMilli(), seq, hex.EncodeToString(randBytes))
}

// TryPreConsume 尝试预扣额度（原子操作，支持多周期）
// 返回预扣上下文，用于后续回滚或调整
func (s *SubscriptionUsageService) TryPreConsume(
	subscriptionId int64,
	amount int64,
	periods map[string]int64, // period -> limitQuota
	strategy string,
) (*PreConsumeContext, error) {
	if amount <= 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("预扣额度必须为正数"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	if len(periods) == 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("至少需要一个限额周期"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	// 生成唯一上下文 ID（使用毫秒时间戳 + 序列号 + 随机字节）
	contextId := s.generateContextId(subscriptionId)

	ctx := &PreConsumeContext{
		ContextId:      contextId,
		SubscriptionId: subscriptionId,
		Amount:         amount,
		Timestamp:      common.GetTimestamp(),
		Usages:         make(map[string]*UsagePreConsume),
	}

	// 在事务中执行预扣
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 按周期优先级顺序执行预扣（需求：按周期顺序预扣，从小周期到大周期）
		sortedPeriods := s.getSortedPeriods(periods)
		for _, period := range sortedPeriods {
			limitQuota := periods[period]
			// 获取或创建活跃窗口
			usage, err := model.GetOrCreateActiveUsageWindowWithTx(tx, subscriptionId, period, limitQuota, strategy)
			if err != nil {
				return fmt.Errorf("获取窗口失败 [%s]: %w", period, err)
			}

			if usage == nil {
				return fmt.Errorf("无法创建使用量窗口 [%s]", period)
			}

			// 检查是否超额
			if usage.UsedQuota+amount > usage.LimitQuota {
				return types.NewErrorWithStatusCode(
					fmt.Errorf("周期 [%s] 额度不足: 已用 %d, 限额 %d, 请求 %d",
						period, usage.UsedQuota, usage.LimitQuota, amount),
					types.ErrorCodeUsageQuotaExceeded,
					http.StatusTooManyRequests,
				)
			}

			// 记录预扣前状态
			preUsed := usage.UsedQuota

			// 原子更新
			result := tx.Model(&model.SubscriptionUsage{}).
				Where("id = ? AND used_quota + ? <= limit_quota", usage.Id, amount).
				Update("used_quota", gorm.Expr("used_quota + ?", amount))

			if result.Error != nil {
				return fmt.Errorf("预扣失败 [%s]: %w", period, result.Error)
			}

			if result.RowsAffected == 0 {
				return types.NewErrorWithStatusCode(
					fmt.Errorf("周期 [%s] %s", period, common.MsgSubscriptionQuotaExhausted),
					types.ErrorCodeUsageQuotaExceeded,
					http.StatusTooManyRequests,
				)
			}

			// 记录预扣上下文
			ctx.Usages[period] = &UsagePreConsume{
				UsageId:     usage.Id,
				Period:      period,
				Amount:      amount,
				PreUsed:     preUsed,
				WindowStart: usage.WindowStart,
				WindowEnd:   usage.WindowEnd,
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 缓存上下文（用于后续回滚）- 使用 Redis 支持跨实例
	if err := s.storeContext(contextId, ctx); err != nil {
		// 存储上下文失败，数据库已扣费但无法回滚，这是严重错误
		// 需要回滚数据库事务中的扣费操作
		common.SysError(fmt.Sprintf("预扣上下文存储失败，尝试回滚数据库扣费: %v", err))
		// 回滚已执行的数据库扣费
		rollbackErr := model.DB.Transaction(func(tx *gorm.DB) error {
			for period, preConsume := range ctx.Usages {
				if refundErr := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount); refundErr != nil {
					return fmt.Errorf("回滚周期 [%s] 失败: %w", period, refundErr)
				}
			}
			return nil
		})
		if rollbackErr != nil {
			common.SysError(fmt.Sprintf("预扣回滚也失败，数据可能不一致: %v", rollbackErr))
		}
		return nil, fmt.Errorf("预扣上下文存储失败: %w", err)
	}

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(subscriptionId)

	return ctx, nil
}

// TryPreConsumeWithStrategies 尝试预扣额度（支持每周期不同策略）
// periodConfigs: period -> PeriodConfig (包含 limitQuota 和 strategy)
func (s *SubscriptionUsageService) TryPreConsumeWithStrategies(
	subscriptionId int64,
	amount int64,
	periodConfigs map[string]PeriodConfig,
) (*PreConsumeContext, error) {
	if amount <= 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("预扣额度必须为正数"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	if len(periodConfigs) == 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("至少需要一个限额周期"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	contextId := s.generateContextId(subscriptionId)

	ctx := &PreConsumeContext{
		ContextId:      contextId,
		SubscriptionId: subscriptionId,
		Amount:         amount,
		Timestamp:      common.GetTimestamp(),
		Usages:         make(map[string]*UsagePreConsume),
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 按周期优先级顺序执行预扣（需求：按周期顺序预扣，从小周期到大周期）
		sortedPeriods := s.getSortedPeriodConfigs(periodConfigs)
		for _, period := range sortedPeriods {
			config := periodConfigs[period]
			// 每个周期使用各自的策略
			strategy := config.Strategy
			if strategy == "" {
				strategy = common.WindowStrategyRolling
			}

			usage, err := model.GetOrCreateActiveUsageWindowWithTx(tx, subscriptionId, period, config.LimitQuota, strategy)
			if err != nil {
				return fmt.Errorf("获取窗口失败 [%s]: %w", period, err)
			}

			if usage == nil {
				return fmt.Errorf("无法创建使用量窗口 [%s]", period)
			}

			if usage.UsedQuota+amount > usage.LimitQuota {
				return types.NewErrorWithStatusCode(
					fmt.Errorf("周期 [%s] 额度不足: 已用 %d, 限额 %d, 请求 %d",
						period, usage.UsedQuota, usage.LimitQuota, amount),
					types.ErrorCodeUsageQuotaExceeded,
					http.StatusTooManyRequests,
				)
			}

			preUsed := usage.UsedQuota

			result := tx.Model(&model.SubscriptionUsage{}).
				Where("id = ? AND used_quota + ? <= limit_quota", usage.Id, amount).
				Update("used_quota", gorm.Expr("used_quota + ?", amount))

			if result.Error != nil {
				return fmt.Errorf("预扣失败 [%s]: %w", period, result.Error)
			}

			if result.RowsAffected == 0 {
				return types.NewErrorWithStatusCode(
					fmt.Errorf("周期 [%s] %s", period, common.MsgSubscriptionQuotaExhausted),
					types.ErrorCodeUsageQuotaExceeded,
					http.StatusTooManyRequests,
				)
			}

			ctx.Usages[period] = &UsagePreConsume{
				UsageId:     usage.Id,
				Period:      period,
				Amount:      amount,
				PreUsed:     preUsed,
				WindowStart: usage.WindowStart,
				WindowEnd:   usage.WindowEnd,
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 缓存上下文（用于后续回滚）- 使用 Redis 支持跨实例
	if err := s.storeContext(contextId, ctx); err != nil {
		// 存储上下文失败，需要回滚数据库扣费
		common.SysError(fmt.Sprintf("预扣上下文存储失败，尝试回滚数据库扣费: %v", err))
		rollbackErr := model.DB.Transaction(func(tx *gorm.DB) error {
			for period, preConsume := range ctx.Usages {
				if refundErr := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount); refundErr != nil {
					return fmt.Errorf("回滚周期 [%s] 失败: %w", period, refundErr)
				}
			}
			return nil
		})
		if rollbackErr != nil {
			common.SysError(fmt.Sprintf("预扣回滚也失败，数据可能不一致: %v", rollbackErr))
		}
		return nil, fmt.Errorf("预扣上下文存储失败: %w", err)
	}

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(subscriptionId)

	return ctx, nil
}

// TryPreConsumeWithTx 在事务中尝试预扣额度
// ⚠️ 重要：此方法不会自动写入缓存，调用方需要在事务提交成功后调用 StorePreConsumeContext(ctx)
// 这样可以避免事务回滚时缓存中遗留无效数据
// 如果需要通过 contextId 回滚，请在事务提交后先调用 StorePreConsumeContext
func (s *SubscriptionUsageService) TryPreConsumeWithTx(
	tx *gorm.DB,
	subscriptionId int64,
	amount int64,
	periods map[string]int64,
	strategy string,
) (*PreConsumeContext, error) {
	if amount <= 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("预扣额度必须为正数"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	if len(periods) == 0 {
		return nil, types.NewErrorWithStatusCode(
			errors.New("至少需要一个限额周期"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	// 使用与 TryPreConsume 相同的唯一 ID 生成方式
	contextId := s.generateContextId(subscriptionId)

	ctx := &PreConsumeContext{
		ContextId:      contextId,
		SubscriptionId: subscriptionId,
		Amount:         amount,
		Timestamp:      common.GetTimestamp(),
		Usages:         make(map[string]*UsagePreConsume),
	}

	// 按周期优先级顺序执行预扣（需求：按周期顺序预扣，从小周期到大周期）
	sortedPeriods := s.getSortedPeriods(periods)
	for _, period := range sortedPeriods {
		limitQuota := periods[period]
		usage, err := model.GetOrCreateActiveUsageWindowWithTx(tx, subscriptionId, period, limitQuota, strategy)
		if err != nil {
			return nil, fmt.Errorf("获取窗口失败 [%s]: %w", period, err)
		}

		if usage == nil {
			return nil, fmt.Errorf("无法创建使用量窗口 [%s]", period)
		}

		if usage.UsedQuota+amount > usage.LimitQuota {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("周期 [%s] 额度不足: 已用 %d, 限额 %d, 请求 %d",
					period, usage.UsedQuota, usage.LimitQuota, amount),
				types.ErrorCodeUsageQuotaExceeded,
				http.StatusTooManyRequests,
			)
		}

		preUsed := usage.UsedQuota

		result := tx.Model(&model.SubscriptionUsage{}).
			Where("id = ? AND used_quota + ? <= limit_quota", usage.Id, amount).
			Update("used_quota", gorm.Expr("used_quota + ?", amount))

		if result.Error != nil {
			return nil, fmt.Errorf("预扣失败 [%s]: %w", period, result.Error)
		}

		if result.RowsAffected == 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("周期 [%s] %s", period, common.MsgSubscriptionQuotaExhausted),
				types.ErrorCodeUsageQuotaExceeded,
				http.StatusTooManyRequests,
			)
		}

		ctx.Usages[period] = &UsagePreConsume{
			UsageId:     usage.Id,
			Period:      period,
			Amount:      amount,
			PreUsed:     preUsed,
			WindowStart: usage.WindowStart,
			WindowEnd:   usage.WindowEnd,
		}
	}

	// 注意：不在此处写入缓存，调用方需要在事务提交后调用 StorePreConsumeContext

	return ctx, nil
}

// ===================== 回滚接口 =====================

// RollbackPreConsume 回滚预扣（恢复额度）
func (s *SubscriptionUsageService) RollbackPreConsume(contextId string) error {
	// 从缓存获取上下文（支持 Redis 跨实例）
	ctx, ok := s.loadContext(contextId)
	if !ok {
		return fmt.Errorf("预扣上下文不存在或已过期: %s", contextId)
	}

	// 在事务中回滚所有周期
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for period, preConsume := range ctx.Usages {
			err := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount)
			if err != nil {
				return fmt.Errorf("回滚失败 [%s]: %w", period, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 清理缓存
	s.deleteContext(contextId)

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)

	common.SysLog(fmt.Sprintf("预扣回滚成功: 订阅 %d, 上下文 %s", ctx.SubscriptionId, contextId))
	return nil
}

// RollbackPreConsumeWithTx 在事务中回滚预扣
func (s *SubscriptionUsageService) RollbackPreConsumeWithTx(tx *gorm.DB, contextId string) error {
	ctx, ok := s.loadContext(contextId)
	if !ok {
		return fmt.Errorf("预扣上下文不存在或已过期: %s", contextId)
	}

	for period, preConsume := range ctx.Usages {
		err := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount)
		if err != nil {
			return fmt.Errorf("回滚失败 [%s]: %w", period, err)
		}
	}

	s.deleteContext(contextId)
	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)
	return nil
}

// RollbackPreConsumeByContext 通过上下文对象直接回滚（不依赖缓存）
// 适用于调用方持有上下文对象但不确定是否在缓存中的场景
func (s *SubscriptionUsageService) RollbackPreConsumeByContext(ctx *PreConsumeContext) error {
	if ctx == nil {
		return types.NewErrorWithStatusCode(
			errors.New("预扣上下文不能为空"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	// 在事务中回滚所有周期
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for period, preConsume := range ctx.Usages {
			err := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount)
			if err != nil {
				return fmt.Errorf("回滚失败 [%s]: %w", period, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 如果缓存中存在，也清理掉
	s.deleteContext(ctx.ContextId)

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)

	common.SysLog(fmt.Sprintf("预扣回滚成功: 订阅 %d, 上下文 %s", ctx.SubscriptionId, ctx.ContextId))
	return nil
}

// RollbackPreConsumeByContextWithTx 在事务中通过上下文对象直接回滚（不依赖缓存）
func (s *SubscriptionUsageService) RollbackPreConsumeByContextWithTx(tx *gorm.DB, ctx *PreConsumeContext) error {
	if ctx == nil {
		return types.NewErrorWithStatusCode(
			errors.New("预扣上下文不能为空"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	for period, preConsume := range ctx.Usages {
		err := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount)
		if err != nil {
			return fmt.Errorf("回滚失败 [%s]: %w", period, err)
		}
	}

	// 如果缓存中存在，也清理掉
	s.deleteContext(ctx.ContextId)
	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)
	return nil
}

// PartialRollbackPreConsume 部分回滚（只回滚指定周期）
func (s *SubscriptionUsageService) PartialRollbackPreConsume(contextId string, periods []string) error {
	ctx, ok := s.loadContext(contextId)
	if !ok {
		return fmt.Errorf("预扣上下文不存在或已过期: %s", contextId)
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, period := range periods {
			preConsume, exists := ctx.Usages[period]
			if !exists {
				continue
			}

			err := model.RefundQuotaWithTx(tx, preConsume.UsageId, preConsume.Amount)
			if err != nil {
				return fmt.Errorf("回滚失败 [%s]: %w", period, err)
			}

			// 从上下文中移除
			delete(ctx.Usages, period)
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 如果所有周期都已回滚，清理缓存；否则更新缓存中的上下文
	if len(ctx.Usages) == 0 {
		s.deleteContext(contextId)
	} else {
		// 部分回滚后仍有剩余周期，需要回写 Redis（确保跨实例一致性）
		if storeErr := s.storeContext(contextId, ctx); storeErr != nil {
			// ⚠️ 严重：Redis 回写失败，但 DB 已回滚成功
			// 此时 Redis 中仍是旧上下文，若后续按同一 contextId 再次回滚会导致二次退款
			// RefundQuotaWithTx 不是幂等的（每次调用都会增加 used_quota）
			// 必须返回错误，让调用方感知并处理此不一致状态
			common.SysError(fmt.Sprintf("部分回滚后回写上下文失败 [%s]: %v - DB已回滚但Redis未更新，存在二次退款风险", contextId, storeErr))
			// 尝试删除 Redis 上下文以防止二次回滚（宁可丢失剩余周期回滚能力，也不要二次退款）
			s.deleteContext(contextId)
			// 使用量已变化，清除缓存
			s.invalidateUsageCache(ctx.SubscriptionId)
			return fmt.Errorf("部分回滚成功但上下文回写失败，剩余周期无法再回滚: %w", storeErr)
		}
	}

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)

	return nil
}

// ===================== 差额调整接口 =====================

// AdjustUsage 调整使用量（根据实际消耗调整，支持增加或减少）
// delta > 0: 补扣（实际消耗大于预扣）
// delta < 0: 返还（实际消耗小于预扣）
func (s *SubscriptionUsageService) AdjustUsage(contextId string, actualAmount int64) error {
	ctx, ok := s.loadContext(contextId)
	if !ok {
		return fmt.Errorf("预扣上下文不存在或已过期: %s", contextId)
	}

	delta := actualAmount - ctx.Amount

	if delta == 0 {
		// 实际消耗等于预扣，无需调整
		s.deleteContext(contextId)
		// 使用量确认完成，清除缓存以确保一致性
		s.invalidateUsageCache(ctx.SubscriptionId)
		return nil
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for period, preConsume := range ctx.Usages {
			if delta > 0 {
				// 补扣：实际消耗大于预扣
				err := model.ConsumeQuotaWithTx(tx, preConsume.UsageId, delta)
				if err != nil {
					return fmt.Errorf("补扣失败 [%s]: %w", period, err)
				}
			} else {
				// 返还：实际消耗小于预扣
				err := model.RefundQuotaWithTx(tx, preConsume.UsageId, -delta)
				if err != nil {
					return fmt.Errorf("返还失败 [%s]: %w", period, err)
				}
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	s.deleteContext(contextId)
	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)
	common.SysLog(fmt.Sprintf("额度调整成功: 订阅 %d, 预扣 %d, 实际 %d, 差额 %d",
		ctx.SubscriptionId, ctx.Amount, actualAmount, delta))

	return nil
}

// AdjustUsageWithTx 在事务中调整使用量
func (s *SubscriptionUsageService) AdjustUsageWithTx(tx *gorm.DB, contextId string, actualAmount int64) error {
	ctx, ok := s.loadContext(contextId)
	if !ok {
		return fmt.Errorf("预扣上下文不存在或已过期: %s", contextId)
	}

	delta := actualAmount - ctx.Amount

	if delta == 0 {
		s.deleteContext(contextId)
		// 使用量确认完成，清除缓存以确保一致性
		s.invalidateUsageCache(ctx.SubscriptionId)
		return nil
	}

	for period, preConsume := range ctx.Usages {
		if delta > 0 {
			err := model.ConsumeQuotaWithTx(tx, preConsume.UsageId, delta)
			if err != nil {
				return fmt.Errorf("补扣失败 [%s]: %w", period, err)
			}
		} else {
			err := model.RefundQuotaWithTx(tx, preConsume.UsageId, -delta)
			if err != nil {
				return fmt.Errorf("返还失败 [%s]: %w", period, err)
			}
		}
	}

	s.deleteContext(contextId)
	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)
	return nil
}

// ===================== 窗口刷新逻辑 =====================

// RefreshWindow 刷新窗口（供定时任务调用）
// 对于固定窗口：检查并归档过期窗口
// 对于滚动窗口：窗口由用户首次使用触发，此方法仅清理过期记录
// 性能优化：使用批量查询统计，避免全量加载和逐条日志
func (s *SubscriptionUsageService) RefreshWindow(ctx context.Context) error {
	now := common.GetTimestamp()

	// 使用 COUNT 获取过期窗口数量（避免全量加载）
	var expiredCount int64
	err := model.DB.Model(&model.SubscriptionUsage{}).
		Where("window_end < ?", now).
		Count(&expiredCount).Error
	if err != nil {
		common.SysError(fmt.Sprintf("统计过期窗口失败: %v", err))
		return err
	}

	if expiredCount == 0 {
		return nil
	}

	// 获取过期窗口的汇总统计信息（按订阅和周期分组，避免逐条日志）
	type ExpiredSummary struct {
		SubscriptionId int64
		Period         string
		TotalUsed      int64
		TotalLimit     int64
		WindowCount    int64
	}
	var summaries []ExpiredSummary
	err = model.DB.Model(&model.SubscriptionUsage{}).
		Select("subscription_id, period, SUM(used_quota) as total_used, SUM(limit_quota) as total_limit, COUNT(*) as window_count").
		Where("window_end < ?", now).
		Group("subscription_id, period").
		Scan(&summaries).Error
	if err != nil {
		common.SysError(fmt.Sprintf("获取过期窗口汇总失败: %v", err))
		// 继续执行标记操作，不影响主流程
	} else if len(summaries) > 0 {
		// 仅记录汇总信息，避免逐条日志的性能开销
		common.SysLog(fmt.Sprintf("窗口归档汇总: %d 个订阅/周期组合, 共 %d 条记录",
			len(summaries), expiredCount))
	}

	// 批量标记过期窗口（仅更新未标记的记录，避免重复写入）
	// 使用 updated_at = now 作为标记，后续清理任务会处理这些记录
	result := model.DB.Model(&model.SubscriptionUsage{}).
		Where("window_end < ? AND updated_at < ?", now, now).
		Update("updated_at", now)

	if result.Error != nil {
		common.SysError(fmt.Sprintf("窗口归档更新失败: %v", result.Error))
		return result.Error
	}

	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("窗口刷新任务完成: 新归档 %d 条记录", result.RowsAffected))
	}
	return nil
}

// BatchRefreshExpiredWindows 批量刷新指定订阅的过期窗口
// 注意：此方法直接查询过期窗口，而非通过 GetCurrentUsageBySubscription（只返回活跃窗口）
// 性能优化：使用批量查询和批量更新，避免循环查询和逐条日志
func (s *SubscriptionUsageService) BatchRefreshExpiredWindows(subscriptionIds []int64) error {
	if len(subscriptionIds) == 0 {
		return nil
	}

	now := common.GetTimestamp()

	// 批量统计过期窗口数量（按订阅分组，用于日志）
	type ExpiredCount struct {
		SubscriptionId int64
		Count          int64
	}
	var expiredCounts []ExpiredCount
	err := model.DB.Model(&model.SubscriptionUsage{}).
		Select("subscription_id, COUNT(*) as count").
		Where("subscription_id IN ? AND window_end < ?", subscriptionIds, now).
		Group("subscription_id").
		Scan(&expiredCounts).Error

	if err != nil {
		common.SysError(fmt.Sprintf("批量查询过期窗口失败: %v", err))
		return err
	}

	// 计算总数
	var totalExpired int64
	for _, ec := range expiredCounts {
		totalExpired += ec.Count
	}

	if totalExpired == 0 {
		return nil
	}

	// 批量标记过期窗口（与 RefreshWindow 逻辑一致）
	// 仅更新未标记的记录，避免重复更新
	result := model.DB.Model(&model.SubscriptionUsage{}).
		Where("subscription_id IN ? AND window_end < ? AND updated_at < ?", subscriptionIds, now, now).
		Update("updated_at", now)

	if result.Error != nil {
		common.SysError(fmt.Sprintf("批量标记过期窗口失败: %v", result.Error))
		return result.Error
	}

	// 清除受影响订阅的使用量缓存
	for _, ec := range expiredCounts {
		s.invalidateUsageCache(ec.SubscriptionId)
	}

	if result.RowsAffected > 0 {
		common.SysLog(fmt.Sprintf("批量刷新窗口完成: %d 个订阅，新归档 %d 条记录",
			len(expiredCounts), result.RowsAffected))
	}

	return nil
}

// StartPeriodicCleanup 启动周期性清理任务
// 返回一个取消函数，调用后停止清理任务
func (s *SubscriptionUsageService) StartPeriodicCleanup(ctx context.Context, interval time.Duration) context.CancelFunc {
	// 验证 interval，避免 time.NewTicker panic
	if interval <= 0 {
		common.SysError(fmt.Sprintf("StartPeriodicCleanup: interval 必须为正数，收到: %v，使用默认值 5 分钟", interval))
		interval = 5 * time.Minute
	}

	cleanupCtx, cancel := context.WithCancel(ctx)

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		common.SysLog(fmt.Sprintf("预扣上下文清理任务已启动，间隔: %v", interval))

		for {
			select {
			case <-cleanupCtx.Done():
				common.SysLog("预扣上下文清理任务已停止")
				return
			case <-ticker.C:
				count := s.ClearExpiredContexts()
				if count > 0 {
					common.SysLog(fmt.Sprintf("定时清理完成: 清除 %d 个过期上下文", count))
				}
			}
		}
	}()

	return cancel
}

// ===================== 直接消耗接口（不经过预扣）=====================

// ConsumeQuotaDirect 直接消耗额度（不经过预扣，用于同步场景）
func (s *SubscriptionUsageService) ConsumeQuotaDirect(
	subscriptionId int64,
	amount int64,
	periods map[string]int64,
	strategy string,
) error {
	if amount <= 0 {
		return types.NewErrorWithStatusCode(
			errors.New("消耗额度必须为正数"),
			types.ErrorCodeInvalidRequestParams,
			http.StatusBadRequest,
		)
	}

	err := model.DB.Transaction(func(tx *gorm.DB) error {
		// 按周期优先级顺序执行消耗（保持与预扣一致）
		sortedPeriods := s.getSortedPeriods(periods)
		for _, period := range sortedPeriods {
			limitQuota := periods[period]
			err := model.ConsumeQuotaWithStrategy(subscriptionId, period, amount, limitQuota, strategy)
			if err != nil {
				return fmt.Errorf("消耗失败 [%s]: %w", period, err)
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(subscriptionId)
	return nil
}

// ===================== 查询接口 =====================

// GetCurrentUsage 获取订阅当前所有周期的使用量
// 优先从 Redis 缓存读取，缓存未命中时从数据库读取并刷新缓存
func (s *SubscriptionUsageService) GetCurrentUsage(subscriptionId int64) ([]*model.SubscriptionUsage, error) {
	// 尝试从缓存读取
	if cacheData, ok := s.getUsageFromCache(subscriptionId); ok {
		// 将缓存数据转换为 model.SubscriptionUsage 格式
		usages := make([]*model.SubscriptionUsage, 0, len(cacheData))
		for period, entry := range cacheData {
			usages = append(usages, &model.SubscriptionUsage{
				SubscriptionId: subscriptionId,
				Period:         period,
				WindowStart:    entry.WindowStart,
				WindowEnd:      entry.WindowEnd,
				UsedQuota:      entry.UsedQuota,
				LimitQuota:     entry.LimitQuota,
			})
		}
		return usages, nil
	}

	// 缓存未命中，从数据库读取
	usages, err := model.GetCurrentUsageBySubscription(subscriptionId)
	if err != nil {
		return nil, err
	}

	// 刷新缓存（异步，不影响返回）
	if len(usages) > 0 {
		s.refreshUsageCache(subscriptionId, usages)
	}

	return usages, nil
}

// GetUsageSummary 获取订阅使用量汇总（按周期分组）
func (s *SubscriptionUsageService) GetUsageSummary(subscriptionId int64) (map[string]*model.SubscriptionUsage, error) {
	return model.GetUsageSummaryBySubscription(subscriptionId)
}

// GetRemainingQuota 获取剩余额度
func (s *SubscriptionUsageService) GetRemainingQuota(subscriptionId int64, period string) (int64, error) {
	return model.GetRemainingQuota(subscriptionId, period)
}

// GetUsagePercentage 获取使用率
func (s *SubscriptionUsageService) GetUsagePercentage(subscriptionId int64, period string) (float64, error) {
	return model.GetUsagePercentage(subscriptionId, period)
}

// BatchGetCurrentUsages 批量获取多个订阅的当前使用量
func (s *SubscriptionUsageService) BatchGetCurrentUsages(subscriptionIds []int64) (map[int64][]*model.SubscriptionUsage, error) {
	return model.BatchGetCurrentUsages(subscriptionIds)
}

// ===================== 管理接口 =====================

// UpdateUsageWindow 更新使用量窗口（管理员操作）
func (s *SubscriptionUsageService) UpdateUsageWindow(usageId int64, usedQuota int64, limitQuota int64) error {
	// 先获取 usage 记录以获取 subscriptionId（用于缓存失效）
	var usage model.SubscriptionUsage
	if err := model.DB.Select("subscription_id").Where("id = ?", usageId).First(&usage).Error; err != nil {
		return fmt.Errorf("usage 记录不存在: %w", err)
	}

	err := model.UpdateUsageWindow(usageId, usedQuota, limitQuota)
	if err != nil {
		return err
	}

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(usage.SubscriptionId)

	common.SysLog(fmt.Sprintf("使用量窗口已更新: ID %d, 已用 %d, 限额 %d", usageId, usedQuota, limitQuota))
	return nil
}

// ResetUsageWindow 重置使用量窗口（清零）
func (s *SubscriptionUsageService) ResetUsageWindow(subscriptionId int64, period string) error {
	err := model.ResetUsageWindow(subscriptionId, period)
	if err != nil {
		return err
	}

	// 使用量已变化，清除 Redis 使用量缓存
	s.invalidateUsageCache(subscriptionId)

	common.SysLog(fmt.Sprintf("使用量窗口已重置: 订阅 %d, 周期 %s", subscriptionId, period))
	return nil
}

// CleanupExpiredUsages 清理过期的使用量记录
func (s *SubscriptionUsageService) CleanupExpiredUsages(retentionDays int) (int64, error) {
	count, err := model.CleanupExpiredUsages(retentionDays)
	if err != nil {
		return 0, err
	}

	common.SysLog(fmt.Sprintf("清理过期使用量记录: 删除 %d 条记录", count))
	return count, nil
}

// ===================== 上下文管理 =====================

// StorePreConsumeContext 存储预扣上下文到缓存
// 在使用 TryPreConsumeWithTx 后，事务提交成功后调用此方法
// 这样可以确保只有成功提交的预扣才会被缓存
// 返回 error：Redis 启用时，存储失败会返回错误
func (s *SubscriptionUsageService) StorePreConsumeContext(ctx *PreConsumeContext) error {
	if ctx == nil {
		return nil
	}
	err := s.storeContext(ctx.ContextId, ctx)
	if err != nil {
		return err
	}
	// 使用量已变化（TryPreConsumeWithTx 已更新 DB），清除 Redis 使用量缓存
	s.invalidateUsageCache(ctx.SubscriptionId)
	return nil
}

// GetPreConsumeContext 获取预扣上下文
func (s *SubscriptionUsageService) GetPreConsumeContext(contextId string) (*PreConsumeContext, error) {
	ctx, ok := s.loadContext(contextId)
	if !ok {
		return nil, fmt.Errorf("预扣上下文不存在或已过期: %s", contextId)
	}
	return ctx, nil
}

// SerializeContext 序列化预扣上下文（用于跨进程传递）
func (s *SubscriptionUsageService) SerializeContext(ctx *PreConsumeContext) (string, error) {
	data, err := json.Marshal(ctx)
	if err != nil {
		return "", fmt.Errorf("序列化上下文失败: %w", err)
	}
	return string(data), nil
}

// DeserializeContext 反序列化预扣上下文
func (s *SubscriptionUsageService) DeserializeContext(data string) (*PreConsumeContext, error) {
	var ctx PreConsumeContext
	err := json.Unmarshal([]byte(data), &ctx)
	if err != nil {
		return nil, fmt.Errorf("反序列化上下文失败: %w", err)
	}

	// 恢复到缓存（支持 Redis 跨实例）
	if err := s.storeContext(ctx.ContextId, &ctx); err != nil {
		return nil, fmt.Errorf("恢复上下文到缓存失败: %w", err)
	}

	return &ctx, nil
}

// ClearExpiredContexts 清理过期的预扣上下文（供定时任务调用）
// 注意：Redis 存储的上下文有自动 TTL 过期，此函数主要清理内存缓存
func (s *SubscriptionUsageService) ClearExpiredContexts() int {
	count := 0
	now := common.GetTimestamp()

	s.preConsumeCache.Range(func(key, value interface{}) bool {
		contextId, ok := key.(string)
		if !ok {
			s.preConsumeCache.Delete(key)
			count++
			return true
		}

		ctx, ok := value.(*PreConsumeContext)
		if !ok {
			s.deleteContext(contextId)
			count++
			return true
		}

		// 超过 10 分钟的上下文视为过期
		if now-ctx.Timestamp > 600 {
			s.deleteContext(contextId)
			count++
		}

		return true
	})

	if count > 0 {
		common.SysLog(fmt.Sprintf("清理过期预扣上下文: %d 个", count))
	}

	return count
}

// ===================== 统计接口 =====================

// GetUsageStats 获取使用量统计信息
func (s *SubscriptionUsageService) GetUsageStats(subscriptionId int64) (map[string]interface{}, error) {
	usages, err := s.GetCurrentUsage(subscriptionId)
	if err != nil {
		return nil, err
	}

	stats := make(map[string]interface{})
	stats["subscription_id"] = subscriptionId
	stats["periods"] = make(map[string]interface{})

	for _, usage := range usages {
		periodStats := map[string]interface{}{
			"used_quota":     usage.UsedQuota,
			"limit_quota":    usage.LimitQuota,
			"remaining":      usage.GetRemainingQuotaValue(),
			"usage_ratio":    usage.GetUsageRatio(),
			"window_start":   usage.WindowStart,
			"window_end":     usage.WindowEnd,
			"time_remaining": s.GetTimeUntilWindowEnd(usage),
		}
		stats["periods"].(map[string]interface{})[usage.Period] = periodStats
	}

	return stats, nil
}

// CheckQuotaLow 检查额度是否低于阈值（触发通知）
func (s *SubscriptionUsageService) CheckQuotaLow(subscriptionId int64, threshold float64) (map[string]bool, error) {
	usages, err := s.GetCurrentUsage(subscriptionId)
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, usage := range usages {
		result[usage.Period] = usage.IsQuotaLow(threshold)
	}

	return result, nil
}

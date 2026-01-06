package service

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ===================== Service 层 - 订阅缓存服务 =====================
// 负责管理订阅相关的 Redis 缓存，提升计费链路性能

// SubscriptionCacheService 订阅缓存服务
type SubscriptionCacheService struct {
	priorityService *SubscriptionPriorityService
	mu              sync.RWMutex // 保护缓存统计
	cacheHits       int64        // 缓存命中次数
	cacheMisses     int64        // 缓存未命中次数
}

var (
	subscriptionCacheService     *SubscriptionCacheService
	subscriptionCacheServiceOnce sync.Once
)

// GetSubscriptionCacheService 获取订阅缓存服务单例
func GetSubscriptionCacheService() *SubscriptionCacheService {
	subscriptionCacheServiceOnce.Do(func() {
		subscriptionCacheService = &SubscriptionCacheService{
			priorityService: GetSubscriptionPriorityService(),
			cacheHits:       0,
			cacheMisses:     0,
		}
	})
	return subscriptionCacheService
}

// ===================== 缓存键定义 =====================

const (
	// 用户活跃订阅列表缓存键前缀
	// 格式: subscription:active:<user_id>
	activeSubscriptionsCacheKeyPrefix = "subscription:active:"

	// 订阅使用量缓存键前缀
	// 格式: subscription:usage:<subscription_id>
	subscriptionUsageCacheKeyPrefix = "subscription:usage:"

	// 默认缓存 TTL（1小时）
	defaultCacheTTL = 1 * time.Hour

	// 最大缓存 TTL（30天）
	maxCacheTTL = 30 * 24 * time.Hour
)

// ===================== 缓存数据结构 =====================

// CachedSubscription 缓存的订阅数据（精简版）
// 导出此类型供 subscription_priority_service 使用
type CachedSubscription struct {
	Id                  int64   `json:"id"`
	PlanId              int64   `json:"plan_id"`
	Status              string  `json:"status"`
	StartAt             int64   `json:"start_at"`
	EndAt               int64   `json:"end_at"`
	Priority            int     `json:"priority"`
	AutoWalletFallback  bool    `json:"auto_wallet_fallback"`
	BindChannelGroup    *string `json:"bind_channel_group,omitempty"`
	ModelWhitelistCache *string `json:"model_whitelist_cache,omitempty"`
	Metadata            *string `json:"metadata,omitempty"` // 包含 plan_snapshot 等元数据
}

// ActiveSubscriptionsCacheData 活跃订阅缓存包装结构
// 导出此类型供 subscription_priority_service 使用
type ActiveSubscriptionsCacheData struct {
	Subscriptions []CachedSubscription `json:"subscriptions"`
	UpdatedAt     int64                `json:"updated_at"`
}

// CachedUsage 缓存的使用量数据
// 字段命名与 subscription_usage_service.go 的 UsageCacheEntry 保持一致
type CachedUsage struct {
	WindowStart int64 `json:"window_start"`
	WindowEnd   int64 `json:"window_end"`
	UsedQuota   int64 `json:"used"`  // 注意：使用 "used" 而不是 "used_quota"
	LimitQuota  int64 `json:"limit"` // 注意：使用 "limit" 而不是 "limit_quota"
}

// ===================== 缓存写入方法 =====================

// CacheActiveSubscriptions 缓存用户活跃订阅列表
// TTL 与最早到期的订阅 end_at 对齐
func (s *SubscriptionCacheService) CacheActiveSubscriptions(userId int64, subscriptions []*model.Subscription) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil // Redis 未启用，跳过
	}

	if userId == 0 {
		return nil
	}

	// 转换为缓存数据结构
	var cachedSubs []CachedSubscription
	var minEndAt int64 = 0
	now := common.GetTimestamp()

	for _, sub := range subscriptions {
		if sub.Status != common.SubscriptionStatusActive {
			continue
		}

		cachedSub := CachedSubscription{
			Id:                  sub.Id,
			PlanId:              sub.PlanId,
			Status:              sub.Status,
			StartAt:             sub.StartAt,
			EndAt:               sub.EndAt,
			Priority:            sub.Priority,
			AutoWalletFallback:  sub.AutoWalletFallback,
			BindChannelGroup:    sub.BindChannelGroup,
			ModelWhitelistCache: sub.ModelWhitelistCache,
			Metadata:            sub.Metadata,
		}

		cachedSubs = append(cachedSubs, cachedSub)

		// 找到最早到期时间
		if minEndAt == 0 || sub.EndAt < minEndAt {
			minEndAt = sub.EndAt
		}
	}

	// 使用包装结构
	cacheData := ActiveSubscriptionsCacheData{
		Subscriptions: cachedSubs,
		UpdatedAt:     now,
	}

	// 序列化
	data, err := json.Marshal(cacheData)
	if err != nil {
		common.SysError(fmt.Sprintf("序列化订阅缓存失败 [user_id=%d]: %v", userId, err))
		return err
	}

	// 计算 TTL
	ttl := s.calculateTTL(minEndAt)

	// 写入 Redis
	redisKey := fmt.Sprintf("%s%d", activeSubscriptionsCacheKeyPrefix, userId)
	if err := common.RedisSet(redisKey, string(data), ttl); err != nil {
		common.SysError(fmt.Sprintf("写入订阅缓存失败 [user_id=%d]: %v", userId, err))
		return err
	}

	return nil
}

// CacheSubscriptionUsage 缓存订阅使用量数据
// TTL 与窗口 end_at 对齐
func (s *SubscriptionCacheService) CacheSubscriptionUsage(subscriptionId int64, usages []*model.SubscriptionUsage) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	if subscriptionId == 0 || len(usages) == 0 {
		return nil
	}

	// 转换为缓存数据结构（period 作为 map 的 key）
	cachedUsages := make(map[string]CachedUsage)
	var maxWindowEnd int64 = 0

	for _, usage := range usages {
		cachedUsages[usage.Period] = CachedUsage{
			WindowStart: usage.WindowStart,
			WindowEnd:   usage.WindowEnd,
			UsedQuota:   usage.UsedQuota,
			LimitQuota:  usage.LimitQuota,
		}

		if usage.WindowEnd > maxWindowEnd {
			maxWindowEnd = usage.WindowEnd
		}
	}

	// 序列化
	data, err := json.Marshal(cachedUsages)
	if err != nil {
		common.SysError(fmt.Sprintf("序列化使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
		return err
	}

	// 计算 TTL
	ttl := s.calculateTTL(maxWindowEnd)

	// 写入 Redis
	redisKey := fmt.Sprintf("%s%d", subscriptionUsageCacheKeyPrefix, subscriptionId)
	if err := common.RedisSet(redisKey, string(data), ttl); err != nil {
		common.SysError(fmt.Sprintf("写入使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
		return err
	}

	return nil
}

// RefreshCacheTTL 刷新缓存 TTL
// 通过重新获取缓存值并设置新 TTL 实现
func (s *SubscriptionCacheService) RefreshCacheTTL(userId int64, newEndAt int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	redisKey := fmt.Sprintf("%s%d", activeSubscriptionsCacheKeyPrefix, userId)

	// 获取当前缓存值
	data, err := common.RedisGet(redisKey)
	if err != nil || data == "" {
		return nil // 缓存不存在，无需刷新
	}

	// 计算新 TTL 并重新设置
	ttl := s.calculateTTL(newEndAt)
	return common.RedisSet(redisKey, data, ttl)
}

// calculateTTL 计算缓存 TTL
func (s *SubscriptionCacheService) calculateTTL(endAt int64) time.Duration {
	if endAt == 0 {
		return defaultCacheTTL
	}

	now := common.GetTimestamp()
	ttlSeconds := endAt - now

	// 至少 1 分钟
	if ttlSeconds < 60 {
		return 1 * time.Minute
	}

	// 最大 30 天
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl > maxCacheTTL {
		return maxCacheTTL
	}

	return ttl
}

// ===================== 缓存读取方法 =====================

// GetActiveSubscriptionsFromCache 从缓存获取用户活跃订阅
// 返回 nil, false 表示缓存未命中
func (s *SubscriptionCacheService) GetActiveSubscriptionsFromCache(userId int64) ([]CachedSubscription, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, false
	}

	redisKey := fmt.Sprintf("%s%d", activeSubscriptionsCacheKeyPrefix, userId)
	data, err := common.RedisGet(redisKey)
	if err != nil || data == "" {
		// 缓存未命中
		s.recordCacheMiss()
		return nil, false
	}

	// 解析包装结构
	var cacheData ActiveSubscriptionsCacheData
	if err := json.Unmarshal([]byte(data), &cacheData); err != nil {
		common.SysError(fmt.Sprintf("反序列化订阅缓存失败 [user_id=%d]: %v", userId, err))
		s.recordCacheMiss()
		_ = common.RedisDel(redisKey)
		return nil, false
	}

	// 过滤有效的订阅（已过期 || 未开始）
	now := common.GetTimestamp()
	var validSubs []CachedSubscription
	hasExpired := false
	for _, sub := range cacheData.Subscriptions {
		// 检查状态、开始时间、结束时间
		if sub.Status == common.SubscriptionStatusActive && sub.StartAt <= now && sub.EndAt >= now {
			validSubs = append(validSubs, sub)
		} else {
			hasExpired = true
		}
	}

	// 缓存存在订阅但全部无效，视为未命中
	if len(validSubs) == 0 && len(cacheData.Subscriptions) > 0 {
		s.recordCacheMiss()
		_ = common.RedisDel(redisKey)
		return nil, false
	}

	if hasExpired {
		go func() {
			_, _ = s.FallbackToDatabase(userId)
		}()
	}

	// 缓存命中
	s.recordCacheHit()
	return validSubs, true
}

// GetUsageFromCache 从缓存获取订阅使用量
// 返回 nil, false 表示缓存��命中
// 过滤已过期的窗口（window_end < now）
func (s *SubscriptionCacheService) GetUsageFromCache(subscriptionId int64) (map[string]CachedUsage, bool) {
	if !common.RedisEnabled || common.RDB == nil {
		return nil, false
	}

	redisKey := fmt.Sprintf("%s%d", subscriptionUsageCacheKeyPrefix, subscriptionId)
	data, err := common.RedisGet(redisKey)
	if err != nil || data == "" {
		// 缓存未命中
		s.recordCacheMiss()
		return nil, false
	}

	var cachedUsages map[string]CachedUsage
	if err := json.Unmarshal([]byte(data), &cachedUsages); err != nil {
		common.SysError(fmt.Sprintf("反序列化使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
		s.recordCacheMiss()
		return nil, false
	}

	// 过滤过期的窗口
	now := common.GetTimestamp()
	validUsages := make(map[string]CachedUsage)
	for period, usage := range cachedUsages {
		if usage.WindowEnd >= now {
			validUsages[period] = usage
		}
	}

	// 缓存命中
	s.recordCacheHit()
	return validUsages, true
}

// FallbackToDatabase 缓存未命中时从数据库获取并缓存
func (s *SubscriptionCacheService) FallbackToDatabase(userId int64) ([]*model.Subscription, error) {
	// 从数据库获取
	subscriptions, err := s.priorityService.GetSortedActiveSubscriptions(userId)
	if err != nil {
		return nil, err
	}

	// 异步写入缓存
	go func() {
		_ = s.CacheActiveSubscriptions(userId, subscriptions)
	}()

	return subscriptions, nil
}

// ===================== 缓存失效方法 =====================

// InvalidateOnStatusChange 订阅状态变更时失效缓存
func (s *SubscriptionCacheService) InvalidateOnStatusChange(userId int64, subscriptionId int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	// 删除用户订阅列表缓存
	userCacheKey := fmt.Sprintf("%s%d", activeSubscriptionsCacheKeyPrefix, userId)
	if err := common.RedisDel(userCacheKey); err != nil {
		common.SysError(fmt.Sprintf("删除用户订阅缓存失败 [user_id=%d]: %v", userId, err))
	}

	// 删除订阅使用量缓存
	usageCacheKey := fmt.Sprintf("%s%d", subscriptionUsageCacheKeyPrefix, subscriptionId)
	if err := common.RedisDel(usageCacheKey); err != nil {
		common.SysError(fmt.Sprintf("删除使用量缓存失败 [subscription_id=%d]: %v", subscriptionId, err))
	}

	return nil
}

// InvalidateOnPriorityChange 优先级调整时失效用户订阅缓存
func (s *SubscriptionCacheService) InvalidateOnPriorityChange(userId int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	redisKey := fmt.Sprintf("%s%d", activeSubscriptionsCacheKeyPrefix, userId)
	return common.RedisDel(redisKey)
}

// InvalidateOnUsageUpdate 使用量更新时同步缓存
// 可以选择删除或更新缓存
func (s *SubscriptionCacheService) InvalidateOnUsageUpdate(subscriptionId int64, usages []*model.SubscriptionUsage) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	// 更新缓存（而不是删除，以保持缓存热度）
	return s.CacheSubscriptionUsage(subscriptionId, usages)
}

// InvalidateUserCache 失效用户所有订阅相关缓存
func (s *SubscriptionCacheService) InvalidateUserCache(userId int64) error {
	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	redisKey := fmt.Sprintf("%s%d", activeSubscriptionsCacheKeyPrefix, userId)
	return common.RedisDel(redisKey)
}

// ===================== 缓存预热方法 =====================

// WarmupCache 预热用户缓存
func (s *SubscriptionCacheService) WarmupCache(userId int64) error {
	// 获取用户活跃订阅
	subscriptions, err := s.priorityService.GetSortedActiveSubscriptions(userId)
	if err != nil {
		return err
	}

	// 缓存订阅列表
	if err := s.CacheActiveSubscriptions(userId, subscriptions); err != nil {
		return err
	}

	// 缓存每个订阅的使用量
	for _, sub := range subscriptions {
		usages, err := model.GetCurrentUsageBySubscription(sub.Id)
		if err != nil {
			continue
		}
		_ = s.CacheSubscriptionUsage(sub.Id, usages)
	}

	return nil
}

// ===================== 高层 API =====================

// GetActiveSubscriptions 获取用户活跃订阅（优先从缓存）
// 这是计费链路的主要入口
func (s *SubscriptionCacheService) GetActiveSubscriptions(userId int64) ([]*model.Subscription, error) {
	// 1. 尝试从缓存获取
	cachedSubs, hit := s.GetActiveSubscriptionsFromCache(userId)
	if hit {
		// 将缓存数据转换为完整模型（需要关联套餐信息时再查数据库）
		return s.convertCachedToModel(userId, cachedSubs)
	}

	// 2. 缓存未命中，回源数据库
	return s.FallbackToDatabase(userId)
}

// convertCachedToModel 将缓存的订阅转换为完整模型
// 注意：此方法返回的订阅包含 metadata（其中包含 plan_snapshot），确保套餐变更不影响已购订阅
func (s *SubscriptionCacheService) convertCachedToModel(userId int64, cached []CachedSubscription) ([]*model.Subscription, error) {
	var subs []*model.Subscription

	for _, c := range cached {
		sub := &model.Subscription{
			Id:                  c.Id,
			UserId:              userId,
			PlanId:              c.PlanId,
			Status:              c.Status,
			StartAt:             c.StartAt,
			EndAt:               c.EndAt,
			Priority:            c.Priority,
			AutoWalletFallback:  c.AutoWalletFallback,
			BindChannelGroup:    c.BindChannelGroup,
			ModelWhitelistCache: c.ModelWhitelistCache,
			Metadata:            c.Metadata,
		}

		subs = append(subs, sub)
	}

	return subs, nil
}

// ===================== 性能监控 =====================

// CacheStats 缓存统计
type CacheStats struct {
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Total   int64   `json:"total"`
	HitRate float64 `json:"hit_rate"`
}

// recordCacheHit 记录缓存命中
func (s *SubscriptionCacheService) recordCacheHit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheHits++
}

// recordCacheMiss 记录缓存未命中
func (s *SubscriptionCacheService) recordCacheMiss() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheMisses++
}

// GetCacheStats 获取缓存统计信息
func (s *SubscriptionCacheService) GetCacheStats() CacheStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := s.cacheHits + s.cacheMisses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(s.cacheHits) / float64(total) * 100
	}

	return CacheStats{
		Hits:    s.cacheHits,
		Misses:  s.cacheMisses,
		Total:   total,
		HitRate: hitRate,
	}
}

// ResetCacheStats 重置缓存统计（用于测试或定期重置）
func (s *SubscriptionCacheService) ResetCacheStats() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheHits = 0
	s.cacheMisses = 0
}

// 注意：此实现为基础监控，生产环境建议集成 Prometheus 指标
// Prometheus 集成示例：
// - 使用 prometheus.CounterVec 记录 cache_hits_total{type="subscription"}
// - 使用 prometheus.CounterVec 记录 cache_misses_total{type="subscription"}
// - 使用 prometheus.HistogramVec 记录 cache_access_duration_seconds{type="subscription"}

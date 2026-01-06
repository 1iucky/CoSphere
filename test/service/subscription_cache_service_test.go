package service

import (
	"testing"

	"github.com/QuantumNous/new-api/service"
)

// ===================== 缓存服务单例测试 =====================

func TestGetSubscriptionCacheService_Singleton(t *testing.T) {
	svc1 := service.GetSubscriptionCacheService()
	svc2 := service.GetSubscriptionCacheService()

	if svc1 != svc2 {
		t.Error("期望获取相同的服务实例（单例模式）")
	}
}

// ===================== CacheActiveSubscriptions 测试 =====================

func TestCacheActiveSubscriptions_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.CacheActiveSubscriptions(1, nil)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

func TestCacheActiveSubscriptions_InvalidUserId(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 用户 ID 为 0 时应该返回 nil（跳过缓存）
	err := svc.CacheActiveSubscriptions(0, nil)
	if err != nil {
		t.Errorf("用户 ID 为 0 时不应该返回错误，但得到: %v", err)
	}
}

// ===================== CacheSubscriptionUsage 测试 =====================

func TestCacheSubscriptionUsage_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.CacheSubscriptionUsage(1, nil)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

func TestCacheSubscriptionUsage_InvalidParams(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 订阅 ID 为 0 时应该返回 nil（跳过缓存）
	err := svc.CacheSubscriptionUsage(0, nil)
	if err != nil {
		t.Errorf("订阅 ID 为 0 时不应该返回错误，但得到: %v", err)
	}
}

// ===================== RefreshCacheTTL 测试 =====================

func TestRefreshCacheTTL_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.RefreshCacheTTL(1, 1735689600)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

// ===================== GetActiveSubscriptionsFromCache 测试 =====================

func TestGetActiveSubscriptionsFromCache_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该返回 nil, false
	subs, hit := svc.GetActiveSubscriptionsFromCache(1)
	if hit {
		t.Error("Redis 未启用时不应该命中缓存")
	}
	if subs != nil {
		t.Error("Redis 未启用时应该返回 nil")
	}
}

// ===================== GetUsageFromCache 测试 =====================

func TestGetUsageFromCache_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该返回 nil, false
	usages, hit := svc.GetUsageFromCache(1)
	if hit {
		t.Error("Redis 未启用时不应该命中缓存")
	}
	if usages != nil {
		t.Error("Redis 未启用时应该返回 nil")
	}
}

// ===================== InvalidateOnStatusChange 测试 =====================

func TestInvalidateOnStatusChange_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.InvalidateOnStatusChange(1, 1)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

// ===================== InvalidateOnPriorityChange 测试 =====================

func TestInvalidateOnPriorityChange_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.InvalidateOnPriorityChange(1)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

// ===================== InvalidateOnUsageUpdate 测试 =====================

func TestInvalidateOnUsageUpdate_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.InvalidateOnUsageUpdate(1, nil)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

// ===================== InvalidateUserCache 测试 =====================

func TestInvalidateUserCache_RedisDisabled(t *testing.T) {
	svc := service.GetSubscriptionCacheService()

	// 当 Redis 未启用时，应该直接返回 nil
	err := svc.InvalidateUserCache(1)
	if err != nil {
		t.Errorf("Redis 未启用时不应该返回错误，但得到: %v", err)
	}
}

// ===================== CachedSubscription 结构测试 =====================

func TestCachedSubscriptionStruct(t *testing.T) {
	// 测试 CachedSubscription 结构可以正确创建
	cached := service.CachedSubscription{
		Id:                 1,
		PlanId:             100,
		Status:             "active",
		StartAt:            1704067200,
		EndAt:              1735689600,
		Priority:           1,
		AutoWalletFallback: true,
	}

	if cached.Id != 1 {
		t.Errorf("期望 Id=1，但得到 %d", cached.Id)
	}
	if cached.PlanId != 100 {
		t.Errorf("期望 PlanId=100，但得到 %d", cached.PlanId)
	}
	if cached.Status != "active" {
		t.Errorf("期望 Status=active，但得到 %s", cached.Status)
	}
	if cached.Priority != 1 {
		t.Errorf("期望 Priority=1，但得到 %d", cached.Priority)
	}
	if !cached.AutoWalletFallback {
		t.Error("期望 AutoWalletFallback=true")
	}
}

// ===================== CachedUsage 结构测试 =====================

func TestCachedUsageStruct(t *testing.T) {
	// 测试 CachedUsage 结构可以正确创建
	// 注意：Period 作为 map 的 key，不在 CachedUsage 结构中
	cached := service.CachedUsage{
		WindowStart: 1704067200,
		WindowEnd:   1704153600,
		UsedQuota:   500,
		LimitQuota:  1000,
	}

	if cached.WindowStart != 1704067200 {
		t.Errorf("期望 WindowStart=1704067200，但得到 %d", cached.WindowStart)
	}
	if cached.UsedQuota != 500 {
		t.Errorf("期望 UsedQuota=500，但得到 %d", cached.UsedQuota)
	}
	if cached.LimitQuota != 1000 {
		t.Errorf("期望 LimitQuota=1000，但得到 %d", cached.LimitQuota)
	}
}

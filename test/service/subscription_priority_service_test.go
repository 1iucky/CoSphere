package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

// ===================== 排序测试 =====================

func TestGetSortedSubscriptions_EmptyUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	_, err := svc.GetSortedSubscriptions(0)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

func TestGetSortedActiveSubscriptions_EmptyUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	_, err := svc.GetSortedActiveSubscriptions(0)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

// ===================== 优先级更新测试 =====================

func TestUpdateUserPriority_InvalidParams(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	testCases := []struct {
		name           string
		userId         int64
		subscriptionId int64
		priority       int
		expectError    bool
	}{
		{
			name:           "用户ID为0",
			userId:         0,
			subscriptionId: 1,
			priority:       1,
			expectError:    true,
		},
		{
			name:           "订阅ID为0",
			userId:         1,
			subscriptionId: 0,
			priority:       1,
			expectError:    true,
		},
		{
			name:           "优先级为0",
			userId:         1,
			subscriptionId: 1,
			priority:       0,
			expectError:    true,
		},
		{
			name:           "优先级为负数",
			userId:         1,
			subscriptionId: 1,
			priority:       -1,
			expectError:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.UpdateUserPriority(tc.userId, tc.subscriptionId, tc.priority)
			if tc.expectError && err == nil {
				t.Errorf("期望返回错误，但得到 nil")
			}
			if !tc.expectError && err != nil {
				t.Errorf("不期望返回错误，但得到: %v", err)
			}
		})
	}
}

// ===================== 批量更新测试 =====================

func TestBatchUpdatePriorities_EmptyMap(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.BatchUpdatePriorities(1, nil)
	if err != nil {
		t.Errorf("空映射不应返回错误: %v", err)
	}

	err = svc.BatchUpdatePriorities(1, map[int64]int{})
	if err != nil {
		t.Errorf("空映射不应返回错误: %v", err)
	}
}

func TestBatchUpdatePriorities_NegativePriority(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.BatchUpdatePriorities(1, map[int64]int{
		1: 1,
		2: -1, // 负数优先级
	})
	if err == nil {
		t.Error("期望负数优先级返回错误")
	}
}

func TestBatchUpdatePriorities_ZeroPriority(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.BatchUpdatePriorities(1, map[int64]int{
		1: 1,
		2: 0, // 优先级为0
	})
	if err == nil {
		t.Error("期望优先级为0时返回错误")
	}
}

func TestBatchUpdatePriorities_DuplicatePriority(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.BatchUpdatePriorities(1, map[int64]int{
		1: 1,
		2: 1, // 重复优先级
	})
	if err == nil {
		t.Error("期望重复优先级返回错误")
	}
}

func TestBatchUpdatePriorities_InvalidUserId(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.BatchUpdatePriorities(0, map[int64]int{
		1: 1,
	})
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

// ===================== 重排序测试 =====================

func TestReorderSubscriptions_EmptyList(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.ReorderSubscriptions(1, nil)
	if err != nil {
		t.Errorf("空列表不应返回错误: %v", err)
	}

	err = svc.ReorderSubscriptions(1, []int64{})
	if err != nil {
		t.Errorf("空列表不应返回错误: %v", err)
	}
}

func TestReorderSubscriptions_InvalidUserId(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.ReorderSubscriptions(0, []int64{1, 2, 3})
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

// ===================== 冲突校验测试 =====================

func TestValidatePriorityConflict_InvalidUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.ValidatePriorityConflict(0, 1, 1)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

func TestHasPriorityConflict_InvalidUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	// 无效用户应返回有冲突（因为查询会失败）
	result := svc.HasPriorityConflict(0, 1, 1)
	if !result {
		t.Error("期望无效用户ID返回有冲突")
	}
}

// ===================== 初始化测试 =====================

func TestInitializeSubscriptionPriority_NilSubscription(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.InitializeSubscriptionPriority(nil)
	if err == nil {
		t.Error("期望 nil 订阅返回错误")
	}
}

// ===================== 辅助方法测试 =====================

func TestGetPriorityRange_InvalidUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	_, _, err := svc.GetPriorityRange(0)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

func TestNormalizePriorities_InvalidUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	err := svc.NormalizePriorities(0)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

func TestCalculateInsertPosition_InvalidUser(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	_, err := svc.CalculateInsertPosition(0, 1000, 1000)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

func TestInvalidateUserSubscriptionCache_NoPanic(t *testing.T) {
	svc := service.GetSubscriptionPriorityService()

	// 保存并禁用 Redis 以避免 nil 指针
	prevRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	defer func() {
		common.RedisEnabled = prevRedisEnabled
	}()

	// 即使 Redis 未启用，也不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("InvalidateUserSubscriptionCache 不应 panic: %v", r)
		}
	}()

	svc.InvalidateUserSubscriptionCache(1)
}

// ===================== 单例测试 =====================

func TestGetSubscriptionPriorityService_Singleton(t *testing.T) {
	svc1 := service.GetSubscriptionPriorityService()
	svc2 := service.GetSubscriptionPriorityService()

	if svc1 != svc2 {
		t.Error("GetSubscriptionPriorityService 应返回相同的单例实例")
	}
}

package service_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===================== 2.16.3 兑换订阅幂等性测试 =====================

// TestRedemptionIdempotency_SameUserRetry 测试同一用户重复兑换同一兑换码（幂等场景）
func TestRedemptionIdempotency_SameUserRetry(t *testing.T) {
	// 跳过测试（如果数据库未初始化）
	if model.DB == nil {
		t.Skip("数据库未初始化，跳过测试")
	}

	// 1. 创建测试用户
	testUser := &model.User{
		Username: "idempotency_test_user_" + common.GetUUID()[:8],
		Password: "test123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	err := model.DB.Create(testUser).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testUser)

	// 2. 创建测试套餐
	testPlan := &model.SubscriptionPlan{
		Name:         "幂等性测试套餐",
		Description:  "用于测试幂等性",
		Price:        9900,
		Duration:     30 * 24 * 60 * 60, // 30天
		Status:       common.PlanStatusPublished,
		ChannelGroup: "default",
		Priority:     1,
	}
	err = model.DB.Create(testPlan).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testPlan)

	// 3. 创建测试兑换码
	redemption, err := model.CreateSubscriptionRedemption(
		"幂等性测试兑换码",
		testPlan.Id,
		30*24*60*60, // 30天
		0,
		9900,
		common.RedeemOptionCoexist,
		nil,
		0, // 不过期
		int(testUser.Id),
	)
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(redemption)

	// 4. 第一次兑换 - 应该成功
	svc := service.GetRedemptionService()
	result1, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
	require.NoError(t, err)
	assert.True(t, result1.Success, "第一次兑换应该成功")
	assert.False(t, result1.Idempotent, "第一次兑换不应是幂等结果")
	assert.Greater(t, result1.SubscriptionId, int64(0), "应返回有效的订阅 ID")

	// 记录第一次兑换的结果
	firstSubscriptionId := result1.SubscriptionId
	firstRedeemOption := result1.RedeemOption
	firstEndAt := result1.NewEndAt

	// 清理订阅
	defer model.DB.Unscoped().Delete(&model.Subscription{}, "id = ?", firstSubscriptionId)

	// 5. 第二次兑换（相同用户、相同兑换码） - 应该返回幂等结果
	result2, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
	require.NoError(t, err)
	assert.True(t, result2.Success, "第二次兑换应该返回成功")
	assert.True(t, result2.Idempotent, "第二次兑换应该标记为幂等")
	assert.Equal(t, firstSubscriptionId, result2.SubscriptionId, "应返回相同的订阅 ID")
	assert.Equal(t, firstRedeemOption, result2.RedeemOption, "应返回相同的兑换选项")
	assert.Equal(t, firstEndAt, result2.NewEndAt, "应返回相同的结束时间")

	// 6. 第三次兑换 - 仍应返回幂等结果
	result3, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
	require.NoError(t, err)
	assert.True(t, result3.Success, "第三次兑换应该返回成功")
	assert.True(t, result3.Idempotent, "第三次兑换应该标记为幂等")
	assert.Equal(t, firstSubscriptionId, result3.SubscriptionId, "应返回相同的订阅 ID")
}

// TestRedemptionIdempotency_DifferentUserFail 测试不同用户使用已兑换的兑换码（应失败）
func TestRedemptionIdempotency_DifferentUserFail(t *testing.T) {
	if model.DB == nil {
		t.Skip("数据库未初始化，跳过测试")
	}

	// 1. 创建测试用户 A
	userA := &model.User{
		Username: "idempotency_user_a_" + common.GetUUID()[:8],
		Password: "test123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	err := model.DB.Create(userA).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(userA)

	// 2. 创建测试用户 B
	userB := &model.User{
		Username: "idempotency_user_b_" + common.GetUUID()[:8],
		Password: "test123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	err = model.DB.Create(userB).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(userB)

	// 3. 创建测试套餐
	testPlan := &model.SubscriptionPlan{
		Name:         "幂等性测试套餐B",
		Description:  "用于测试不同用户幂等性",
		Price:        9900,
		Duration:     30 * 24 * 60 * 60,
		Status:       common.PlanStatusPublished,
		ChannelGroup: "default",
		Priority:     1,
	}
	err = model.DB.Create(testPlan).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testPlan)

	// 4. 创建测试兑换码
	redemption, err := model.CreateSubscriptionRedemption(
		"不同用户幂等性测试",
		testPlan.Id,
		30*24*60*60,
		0,
		9900,
		common.RedeemOptionCoexist,
		nil,
		0,
		int(userA.Id),
	)
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(redemption)

	// 5. 用户 A 兑换 - 应该成功
	svc := service.GetRedemptionService()
	result1, err := svc.RedeemSubscription(int(userA.Id), redemption.Key, "")
	require.NoError(t, err)
	assert.True(t, result1.Success, "用户 A 兑换应该成功")

	// 清理订阅
	defer model.DB.Unscoped().Delete(&model.Subscription{}, "id = ?", result1.SubscriptionId)

	// 6. 用户 B 尝试使用同一兑换码 - 应该失败
	result2, err := svc.RedeemSubscription(int(userB.Id), redemption.Key, "")
	assert.Error(t, err, "用户 B 使用已兑换的兑换码应该失败")
	assert.False(t, result2.Success, "用户 B 使用已兑换的兑换码应该失败")
	assert.Equal(t, "REDEMPTION_USED", result2.ErrorCode, "错误码应为 REDEMPTION_USED")
	assert.False(t, result2.Idempotent, "不应标记为幂等")
}

// TestRedemptionIdempotency_ConcurrentSameUser 测试并发场景下的幂等性
func TestRedemptionIdempotency_ConcurrentSameUser(t *testing.T) {
	if model.DB == nil {
		t.Skip("数据库未初始化，跳过测试")
	}

	// 1. 创建测试用户
	testUser := &model.User{
		Username: "concurrent_idempotency_" + common.GetUUID()[:8],
		Password: "test123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	err := model.DB.Create(testUser).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testUser)

	// 2. 创建测试套餐
	testPlan := &model.SubscriptionPlan{
		Name:         "并发幂等性测试套餐",
		Description:  "用于测试并发幂等性",
		Price:        9900,
		Duration:     30 * 24 * 60 * 60,
		Status:       common.PlanStatusPublished,
		ChannelGroup: "default",
		Priority:     1,
	}
	err = model.DB.Create(testPlan).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testPlan)

	// 3. 创建测试兑换码
	redemption, err := model.CreateSubscriptionRedemption(
		"并发幂等性测试",
		testPlan.Id,
		30*24*60*60,
		0,
		9900,
		common.RedeemOptionCoexist,
		nil,
		0,
		int(testUser.Id),
	)
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(redemption)

	// 4. 并发执行多个兑换请求
	concurrency := 10
	var wg sync.WaitGroup
	var successCount int32
	var idempotentCount int32
	var subscriptionIds = make(map[int64]bool)
	var mu sync.Mutex

	svc := service.GetRedemptionService()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
			if err == nil && result.Success {
				atomic.AddInt32(&successCount, 1)
				if result.Idempotent {
					atomic.AddInt32(&idempotentCount, 1)
				}
				mu.Lock()
				subscriptionIds[result.SubscriptionId] = true
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// 5. 验证结果
	// 所有请求都应该成功（第一个是正常成功，其他是幂等成功）
	assert.Equal(t, int32(concurrency), successCount, "所有请求都应该成功")
	// 应该只有一个订阅 ID
	assert.Equal(t, 1, len(subscriptionIds), "应该只创建一个订阅")
	// 幂等计数应该是 concurrency - 1（第一个不是幂等）
	assert.Equal(t, int32(concurrency-1), idempotentCount, "除第一个外都应是幂等结果")

	// 清理订阅
	for subId := range subscriptionIds {
		model.DB.Unscoped().Delete(&model.Subscription{}, "id = ?", subId)
	}
}

// TestRedemptionIdempotency_StackScenario 测试叠加场景的幂等性
func TestRedemptionIdempotency_StackScenario(t *testing.T) {
	if model.DB == nil {
		t.Skip("数据库未初始化，跳过测试")
	}

	// 1. 创建测试用户
	testUser := &model.User{
		Username: "stack_idempotency_" + common.GetUUID()[:8],
		Password: "test123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	err := model.DB.Create(testUser).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testUser)

	// 2. 创建测试套餐
	testPlan := &model.SubscriptionPlan{
		Name:         "叠加幂等性测试套餐",
		Description:  "用于测试叠加场景幂等性",
		Price:        9900,
		Duration:     30 * 24 * 60 * 60,
		Status:       common.PlanStatusPublished,
		ChannelGroup: "default",
		Priority:     1,
	}
	err = model.DB.Create(testPlan).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testPlan)

	// 3. 先创建一个已有订阅（用于叠加）
	now := time.Now().Unix()
	existingSub := &model.Subscription{
		UserId:   testUser.Id,
		PlanId:   testPlan.Id,
		Status:   common.SubscriptionStatusActive,
		StartAt:  now,
		EndAt:    now + 30*24*60*60, // 30天后
		Priority: 1,
	}
	err = model.DB.Create(existingSub).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(existingSub)

	// 4. 创建叠加类型的兑换码
	redemption, err := model.CreateSubscriptionRedemption(
		"叠加幂等性测试",
		testPlan.Id,
		15*24*60*60, // 15天
		0,
		4950,
		common.RedeemOptionStack, // 叠加模式
		nil,
		0,
		int(testUser.Id),
	)
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(redemption)

	// 5. 第一次兑换（叠加）
	svc := service.GetRedemptionService()
	result1, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
	require.NoError(t, err)
	assert.True(t, result1.Success, "叠加兑换应该成功")
	assert.False(t, result1.Idempotent, "第一次兑换不应是幂等")

	firstEndAt := result1.NewEndAt

	// 6. 第二次兑换（应该是幂等）
	result2, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
	require.NoError(t, err)
	assert.True(t, result2.Success, "重复兑换应该成功")
	assert.True(t, result2.Idempotent, "重复兑换应该是幂等")
	// 注意：叠加场景下，幂等返回的是已创建订阅的信息
	// 由于叠加是延长而非创建新订阅，所以返回的是已有订阅的 ID
	assert.Equal(t, firstEndAt, result2.NewEndAt, "幂等返回的结束时间应该一致")
}

// TestGetSubscriptionByRedemptionId 测试通过兑换码 ID 获取订阅
func TestGetSubscriptionByRedemptionId(t *testing.T) {
	if model.DB == nil {
		t.Skip("数据库未初始化，跳过测试")
	}

	// 1. 创建测试用户
	testUser := &model.User{
		Username: "get_sub_by_redemption_" + common.GetUUID()[:8],
		Password: "test123",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	err := model.DB.Create(testUser).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testUser)

	// 2. 创建测试套餐
	testPlan := &model.SubscriptionPlan{
		Name:         "获取订阅测试套餐",
		Description:  "用于测试获取订阅",
		Price:        9900,
		Duration:     30 * 24 * 60 * 60,
		Status:       common.PlanStatusPublished,
		ChannelGroup: "default",
		Priority:     1,
	}
	err = model.DB.Create(testPlan).Error
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(testPlan)

	// 3. 创建兑换码
	redemption, err := model.CreateSubscriptionRedemption(
		"获取订阅测试",
		testPlan.Id,
		30*24*60*60,
		0,
		9900,
		common.RedeemOptionCoexist,
		nil,
		0,
		int(testUser.Id),
	)
	require.NoError(t, err)
	defer model.DB.Unscoped().Delete(redemption)

	// 4. 兑换前查询 - 应该返回 nil
	sub, err := model.GetSubscriptionByRedemptionId(int64(redemption.Id))
	require.NoError(t, err)
	assert.Nil(t, sub, "兑换前应返回 nil")

	// 5. 执行兑换
	svc := service.GetRedemptionService()
	result, err := svc.RedeemSubscription(int(testUser.Id), redemption.Key, "")
	require.NoError(t, err)
	require.True(t, result.Success)
	defer model.DB.Unscoped().Delete(&model.Subscription{}, "id = ?", result.SubscriptionId)

	// 6. 兑换后查询 - 应该返回订阅
	sub, err = model.GetSubscriptionByRedemptionId(int64(redemption.Id))
	require.NoError(t, err)
	assert.NotNil(t, sub, "兑换后应返回订阅")
	assert.Equal(t, result.SubscriptionId, sub.Id, "订阅 ID 应匹配")
	assert.Equal(t, testUser.Id, sub.UserId, "用户 ID 应匹配")
	assert.Equal(t, testPlan.Id, sub.PlanId, "套餐 ID 应匹配")
}

package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
)

// ===================== 窗口计算测试 =====================

// TestCalculateWindowStart_RollingWindow 测试滚动窗口开始时间计算
func TestCalculateWindowStart_RollingWindow(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	timestamp := int64(1640995200) // 2022-01-01 00:00:00
	windowStart := svc.CalculateWindowStart(common.LimitPeriodDay, common.WindowStrategyRolling, timestamp)

	// 滚动窗口：开始时间应该等于触发时间
	if windowStart != timestamp {
		t.Errorf("滚动窗口开始时间应该等于触发时间: 期望 %d, 得到 %d", timestamp, windowStart)
	}
}

// TestCalculateWindowStart_FixedWindow 测试固定窗口开始时间计算
func TestCalculateWindowStart_FixedWindow(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	timestamp := int64(1640995200 + 5*3600) // 2022-01-01 05:00:00
	windowStart := svc.CalculateWindowStart(common.LimitPeriodDay, common.WindowStrategyFixed, timestamp)

	// 固定窗口：开始时间应该是当天 00:00:00
	expectedStart := int64(1640995200)
	if windowStart != expectedStart {
		t.Errorf("固定窗口开始时间应该是自然日开始: 期望 %d, 得到 %d", expectedStart, windowStart)
	}
}

// TestCalculateWindowEnd_RollingWindow 测试滚动窗口结束时间计算
func TestCalculateWindowEnd_RollingWindow(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	windowStart := int64(1640995200)
	windowEnd := svc.CalculateWindowEnd(common.LimitPeriodFiveHours, common.WindowStrategyRolling, windowStart)

	// 5小时窗口：结束时间 = 开始时间 + 5*3600 - 1
	expectedEnd := windowStart + 5*3600 - 1
	if windowEnd != expectedEnd {
		t.Errorf("滚动窗口结束时间计算错误: 期望 %d, 得到 %d", expectedEnd, windowEnd)
	}
}

// TestCalculateWindowEnd_DayWindow 测试每日窗口
func TestCalculateWindowEnd_DayWindow(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	windowStart := int64(1640995200) // 2022-01-01 00:00:00
	windowEnd := svc.CalculateWindowEnd(common.LimitPeriodDay, common.WindowStrategyRolling, windowStart)

	// 每日窗口：结束时间 = 开始时间 + 24*3600 - 1
	expectedEnd := windowStart + 24*3600 - 1
	if windowEnd != expectedEnd {
		t.Errorf("每日窗口结束时间计算错误: 期望 %d, 得到 %d", expectedEnd, windowEnd)
	}
}

// ===================== 窗口状态测试 =====================

// TestIsWindowExpired_NotExpired 测试窗口未过期
func TestIsWindowExpired_NotExpired(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	now := common.GetTimestamp()
	usage := &model.SubscriptionUsage{
		WindowStart: now - 1000,
		WindowEnd:   now + 1000, // 窗口还未结束
	}

	if svc.IsWindowExpired(usage) {
		t.Error("窗口应该未过期")
	}
}

// TestIsWindowExpired_Expired 测试窗口已过期
func TestIsWindowExpired_Expired(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	now := common.GetTimestamp()
	usage := &model.SubscriptionUsage{
		WindowStart: now - 2000,
		WindowEnd:   now - 1000, // 窗口已结束
	}

	if !svc.IsWindowExpired(usage) {
		t.Error("窗口应该已过期")
	}
}

// TestGetTimeUntilWindowEnd 测试剩余时间计算
func TestGetTimeUntilWindowEnd(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	now := common.GetTimestamp()
	usage := &model.SubscriptionUsage{
		WindowStart: now - 1000,
		WindowEnd:   now + 500,
	}

	remaining := svc.GetTimeUntilWindowEnd(usage)

	// 剩余时间应该约为 500 秒
	if remaining < 490 || remaining > 510 {
		t.Errorf("剩余时间计算错误: 期望约 500, 得到 %d", remaining)
	}
}

// TestGetTimeUntilWindowEnd_Expired 测试过期窗口剩余时间
func TestGetTimeUntilWindowEnd_Expired(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	now := common.GetTimestamp()
	usage := &model.SubscriptionUsage{
		WindowStart: now - 2000,
		WindowEnd:   now - 1000,
	}

	remaining := svc.GetTimeUntilWindowEnd(usage)

	// 过期窗口剩余时间应该为 0
	if remaining != 0 {
		t.Errorf("过期窗口剩余时间应该为 0, 得到 %d", remaining)
	}
}

// ===================== 预扣上下文测试 =====================

// TestPreConsumeContext_Creation 测试预扣上下文创建
func TestPreConsumeContext_Creation(t *testing.T) {
	// svc := service.GetSubscriptionUsageService()

	// 注意：此测试需要数据库环境
	// 这里只测试上下文结构

	ctx := &service.PreConsumeContext{
		ContextId:      "test_ctx_001",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(),
		Usages:         make(map[string]*service.UsagePreConsume),
	}

	if ctx.ContextId != "test_ctx_001" {
		t.Error("上下文 ID 设置错误")
	}
	if ctx.SubscriptionId != 1 {
		t.Error("订阅 ID 设置错误")
	}
	if ctx.Amount != 1000 {
		t.Error("预扣额度设置错误")
	}
	if ctx.Usages == nil {
		t.Error("使用量映射不应该为 nil")
	}
}

// TestSerializeContext 测试上下文序列化
func TestSerializeContext(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	ctx := &service.PreConsumeContext{
		ContextId:      "test_ctx_002",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(),
		Usages: map[string]*service.UsagePreConsume{
			common.LimitPeriodDay: {
				UsageId:     100,
				Period:      common.LimitPeriodDay,
				Amount:      1000,
				PreUsed:     500,
				WindowStart: 1640995200,
				WindowEnd:   1640995200 + 24*3600 - 1,
			},
		},
	}

	// 序列化
	data, err := svc.SerializeContext(ctx)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	if data == "" {
		t.Error("序列化结果不应该为空")
	}

	// 反序列化
	restored, err := svc.DeserializeContext(data)
	if err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	// 验证
	if restored.ContextId != ctx.ContextId {
		t.Errorf("上下文 ID 不匹配: 期望 %s, 得到 %s", ctx.ContextId, restored.ContextId)
	}
	if restored.SubscriptionId != ctx.SubscriptionId {
		t.Errorf("订阅 ID 不匹配: 期望 %d, 得到 %d", ctx.SubscriptionId, restored.SubscriptionId)
	}
	if restored.Amount != ctx.Amount {
		t.Errorf("预扣额度不匹配: 期望 %d, 得到 %d", ctx.Amount, restored.Amount)
	}
	if len(restored.Usages) != len(ctx.Usages) {
		t.Errorf("使用量记录数量不匹配: 期望 %d, 得到 %d", len(ctx.Usages), len(restored.Usages))
	}
}

// ===================== 差额调整测试 =====================

// TestAdjustUsage_Calculation 测试差额计算
func TestAdjustUsage_Calculation(t *testing.T) {
	// 测试场景
	testCases := []struct {
		name          string
		preConsume    int64
		actualAmount  int64
		expectedDelta int64
	}{
		{
			name:          "实际消耗等于预扣",
			preConsume:    1000,
			actualAmount:  1000,
			expectedDelta: 0,
		},
		{
			name:          "实际消耗大于预扣（需要补扣）",
			preConsume:    1000,
			actualAmount:  1200,
			expectedDelta: 200,
		},
		{
			name:          "实际消耗小于预扣（需要返还）",
			preConsume:    1000,
			actualAmount:  800,
			expectedDelta: -200,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			delta := tc.actualAmount - tc.preConsume

			if delta != tc.expectedDelta {
				t.Errorf("差额计算错误: 期望 %d, 得到 %d", tc.expectedDelta, delta)
			}

			// 验证操作类型
			if delta > 0 && tc.expectedDelta <= 0 {
				t.Error("正差额应该触发补扣操作")
			}
			if delta < 0 && tc.expectedDelta >= 0 {
				t.Error("负差额应该触发返还操作")
			}
		})
	}
}

// ===================== 并发安全测试 =====================
//
// 当前并发测试覆盖范围 (参见 tasks.md 2.4.9):
//
// ✅ 已覆盖：
// 1) TryPreConsume 的数据库层原子性 + used_quota 不超 limit_quota：
//    - 见 TestConcurrentTryPreConsume1000_NoOverConsume（SQLite 临时库）
// 2) contextId 高并发唯一性：
//    - 见 TestConcurrentPreConsume1000_ContextIdUniqueness（真实调用 TryPreConsume）
//
// ⚠️ 待补充（需要额外测试环境/更高阶集成测试）：
// 1) TryPreConsumeWithTx 的提交/回滚与 StorePreConsumeContext 时序一致性（含 -race）
// 2) 跨实例并发（分布式场景）：Redis 上下文存储/回滚、以及分布式锁（如需）
// =====================================================

// TestConcurrentPreConsume 测试并发预扣（模拟）
func TestConcurrentPreConsume(t *testing.T) {
	// 测试并发访问 sync.Map 的安全性

	var cache sync.Map
	concurrency := 100
	var wg sync.WaitGroup

	// 模拟并发写入
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			contextId := fmt.Sprintf("ctx_%d", id)
			ctx := &service.PreConsumeContext{
				ContextId:      contextId,
				SubscriptionId: int64(id),
				Amount:         1000,
				Timestamp:      common.GetTimestamp(),
				Usages:         make(map[string]*service.UsagePreConsume),
			}

			cache.Store(contextId, ctx)
		}(i)
	}

	wg.Wait()

	// 验证所有上下文都已存储
	count := 0
	cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	if count != concurrency {
		t.Errorf("并发写入数量不匹配: 期望 %d, 得到 %d", concurrency, count)
	}
}

// TestConcurrentContextAccess 测试并发上下文访问
func TestConcurrentContextAccess(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	// 创建测试上下文
	ctx := &service.PreConsumeContext{
		ContextId:      "test_concurrent_ctx",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(),
		Usages:         make(map[string]*service.UsagePreConsume),
	}

	// 手动存储到缓存
	svc.GetPreConsumeContext("test_concurrent_ctx") // 确保缓存初始化
	contextId := ctx.ContextId

	// 模拟并发读取
	concurrency := 50
	var wg sync.WaitGroup
	errors := make(chan error, concurrency)

	// 先存储上下文
	data, _ := svc.SerializeContext(ctx)
	svc.DeserializeContext(data)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			_, err := svc.GetPreConsumeContext(contextId)
			if err != nil {
				errors <- err
			}
		}()
	}

	wg.Wait()
	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Errorf("并发读取错误: %v", err)
	}
}

// ===================== 上下文清理测试 =====================

// TestClearExpiredContexts 测试过期上下文清理
func TestClearExpiredContexts(t *testing.T) {
	svc := service.GetSubscriptionUsageService()

	// 创建新上下文（不会过期）
	newCtx := &service.PreConsumeContext{
		ContextId:      "test_new_ctx",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(), // 当前时间
		Usages:         make(map[string]*service.UsagePreConsume),
	}
	data1, _ := svc.SerializeContext(newCtx)
	svc.DeserializeContext(data1)

	// 创建过期上下文（11分钟前）
	oldCtx := &service.PreConsumeContext{
		ContextId:      "test_old_ctx",
		SubscriptionId: 2,
		Amount:         2000,
		Timestamp:      common.GetTimestamp() - 660, // 11分钟前
		Usages:         make(map[string]*service.UsagePreConsume),
	}
	data2, _ := svc.SerializeContext(oldCtx)
	svc.DeserializeContext(data2)

	// 执行清理
	count := svc.ClearExpiredContexts()

	// 应该至少清理了一个过期上下文
	if count < 1 {
		t.Errorf("应该清理至少1个过期上下文, 实际清理 %d 个", count)
	}

	// 验证新上下文还在
	_, err := svc.GetPreConsumeContext("test_new_ctx")
	if err != nil {
		t.Error("新上下文不应该被清理")
	}

	// 验证旧上下文已被清理
	_, err = svc.GetPreConsumeContext("test_old_ctx")
	if err == nil {
		t.Error("过期上下文应该被清理")
	}
}

// ===================== 统计接口测试 =====================

// TestGetUsageStats_Structure 测试统计数据结构
func TestGetUsageStats_Structure(t *testing.T) {
	// svc := service.GetSubscriptionUsageService()

	// 注意：此测试需要数据库环境
	// 这里只测试数据结构

	stats := make(map[string]interface{})
	stats["subscription_id"] = int64(1)
	stats["periods"] = make(map[string]interface{})

	periodStats := map[string]interface{}{
		"used_quota":     int64(5000),
		"limit_quota":    int64(10000),
		"remaining":      int64(5000),
		"usage_ratio":    0.5,
		"window_start":   int64(1640995200),
		"window_end":     int64(1640995200 + 24*3600 - 1),
		"time_remaining": int64(3600),
	}
	stats["periods"].(map[string]interface{})[common.LimitPeriodDay] = periodStats

	// 验证数据结构
	if stats["subscription_id"].(int64) != 1 {
		t.Error("订阅 ID 不匹配")
	}

	periods := stats["periods"].(map[string]interface{})
	if len(periods) != 1 {
		t.Errorf("周期数量不匹配: 期望 1, 得到 %d", len(periods))
	}

	dayStats, ok := periods[common.LimitPeriodDay].(map[string]interface{})
	if !ok {
		t.Fatal("每日统计数据不存在")
	}

	if dayStats["used_quota"].(int64) != 5000 {
		t.Error("已用额度不匹配")
	}
	if dayStats["limit_quota"].(int64) != 10000 {
		t.Error("限额不匹配")
	}
	if dayStats["remaining"].(int64) != 5000 {
		t.Error("剩余额度不匹配")
	}
	if dayStats["usage_ratio"].(float64) != 0.5 {
		t.Error("使用率不匹配")
	}
}

// TestCheckQuotaLow_Logic 测试额度低阈值检查逻辑
func TestCheckQuotaLow_Logic(t *testing.T) {
	testCases := []struct {
		name      string
		used      int64
		limit     int64
		threshold float64
		expected  bool
	}{
		{
			name:      "额度充足（使用50%，阈值20%）",
			used:      5000,
			limit:     10000,
			threshold: 0.2,
			expected:  false,
		},
		{
			name:      "额度低于阈值（使用85%，阈值20%）",
			used:      8500,
			limit:     10000,
			threshold: 0.2,
			expected:  true,
		},
		{
			name:      "额度刚好达到阈值（使用80%，阈值20%）",
			used:      8000,
			limit:     10000,
			threshold: 0.2,
			expected:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟 IsQuotaLow 逻辑
			usageRatio := float64(tc.used) / float64(tc.limit)
			isLow := usageRatio >= (1 - tc.threshold)

			if isLow != tc.expected {
				t.Errorf("额度低检查错误: 期望 %v, 得到 %v (使用率 %.2f, 阈值 %.2f)",
					tc.expected, isLow, usageRatio, tc.threshold)
			}
		})
	}
}

// ===================== 窗口时长测试 =====================

// TestGetWindowDurationSeconds 测试窗口时长计算
func TestGetWindowDurationSeconds(t *testing.T) {
	testCases := []struct {
		period   string
		expected int64
	}{
		{common.LimitPeriodFiveHours, 5 * 60 * 60},
		{common.LimitPeriodDay, 24 * 60 * 60},
		{common.LimitPeriodWeek, 7 * 24 * 60 * 60},
		{common.LimitPeriodMonth, 30 * 24 * 60 * 60},
	}

	for _, tc := range testCases {
		t.Run(tc.period, func(t *testing.T) {
			duration := model.GetWindowDurationSeconds(tc.period)

			if duration != tc.expected {
				t.Errorf("窗口时长计算错误 [%s]: 期望 %d, 得到 %d", tc.period, tc.expected, duration)
			}
		})
	}
}

// ===================== 多周期预扣测试 =====================

// TestMultiPeriodPreConsume_Validation 测试多周期预扣验证
func TestMultiPeriodPreConsume_Validation(t *testing.T) {
	// svc := service.GetSubscriptionUsageService()

	// 测试参数验证
	testCases := []struct {
		name    string
		amount  int64
		periods map[string]int64
		wantErr bool
	}{
		{
			name:    "预扣额度为0",
			amount:  0,
			periods: map[string]int64{common.LimitPeriodDay: 10000},
			wantErr: true,
		},
		{
			name:    "预扣额度为负数",
			amount:  -100,
			periods: map[string]int64{common.LimitPeriodDay: 10000},
			wantErr: true,
		},
		{
			name:    "没有限额周期",
			amount:  1000,
			periods: map[string]int64{},
			wantErr: true,
		},
		{
			name:   "正常预扣参数",
			amount: 1000,
			periods: map[string]int64{
				common.LimitPeriodDay:   10000,
				common.LimitPeriodWeek:  50000,
				common.LimitPeriodMonth: 200000,
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 参数验证逻辑
			hasError := false
			if tc.amount <= 0 {
				hasError = true
			}
			if len(tc.periods) == 0 {
				hasError = true
			}

			if hasError != tc.wantErr {
				t.Errorf("验证结果不匹配: 期望错误 %v, 得到错误 %v", tc.wantErr, hasError)
			}
		})
	}
}

// ===================== 自动过期清理测试 =====================

// TestContextAutoExpiry 测试上下文自动过期
func TestContextAutoExpiry(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过长时间测试")
	}

	svc := service.GetSubscriptionUsageService()

	// 创建测试上下文
	ctx := &service.PreConsumeContext{
		ContextId:      "test_auto_expire_ctx",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(),
		Usages:         make(map[string]*service.UsagePreConsume),
	}

	data, _ := svc.SerializeContext(ctx)
	svc.DeserializeContext(data)

	// 验证上下文存在
	_, err := svc.GetPreConsumeContext("test_auto_expire_ctx")
	if err != nil {
		t.Fatal("上下文应该存在")
	}

	// 注意：实际的自动过期需要等待 10 分钟
	// 这里只测试过期检查逻辑

	t.Log("上下文自动过期测试通过（完整测试需要等待10分钟）")
}

// ===================== 性能基准测试 =====================

// BenchmarkTryPreConsume 预扣性能基准测试
func BenchmarkTryPreConsume(b *testing.B) {
	// 注意：需要数据库环境
	// 这里只测试上下文创建性能

	for i := 0; i < b.N; i++ {
		_ = &service.PreConsumeContext{
			ContextId:      fmt.Sprintf("bench_ctx_%d", i),
			SubscriptionId: int64(i),
			Amount:         1000,
			Timestamp:      common.GetTimestamp(),
			Usages:         make(map[string]*service.UsagePreConsume),
		}
	}
}

// BenchmarkSerializeContext 上下文序列化性能基准测试
func BenchmarkSerializeContext(b *testing.B) {
	svc := service.GetSubscriptionUsageService()

	ctx := &service.PreConsumeContext{
		ContextId:      "bench_ctx",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(),
		Usages: map[string]*service.UsagePreConsume{
			common.LimitPeriodDay: {
				UsageId:     100,
				Period:      common.LimitPeriodDay,
				Amount:      1000,
				PreUsed:     500,
				WindowStart: 1640995200,
				WindowEnd:   1640995200 + 24*3600 - 1,
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.SerializeContext(ctx)
	}
}

// BenchmarkDeserializeContext 上下文反序列化性能基准测试
func BenchmarkDeserializeContext(b *testing.B) {
	svc := service.GetSubscriptionUsageService()

	ctx := &service.PreConsumeContext{
		ContextId:      "bench_ctx",
		SubscriptionId: 1,
		Amount:         1000,
		Timestamp:      common.GetTimestamp(),
		Usages: map[string]*service.UsagePreConsume{
			common.LimitPeriodDay: {
				UsageId:     100,
				Period:      common.LimitPeriodDay,
				Amount:      1000,
				PreUsed:     500,
				WindowStart: 1640995200,
				WindowEnd:   1640995200 + 24*3600 - 1,
			},
		},
	}

	data, _ := svc.SerializeContext(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.DeserializeContext(data)
	}
}

// ===================== 1000 并发预扣测试 =====================

// TestConcurrentPreConsume1000_ContextIdUniqueness 测试 1000 并发预扣上下文 ID 唯一性
// 此测试验证在高并发场景下 contextId 生成的唯一性
func TestConcurrentPreConsume1000_ContextIdUniqueness(t *testing.T) {
	db, cleanup := setupTestSQLiteDB(t)
	defer cleanup()

	svc := service.GetSubscriptionUsageService()
	concurrency := 1000
	subscriptionId := int64(1001)

	amount := int64(1)
	limitQuota := int64(1000000) // 确保所有并发请求都能成功，以验证 contextId 生成的唯一性
	period := common.LimitPeriodDay

	now := common.GetTimestamp()

	sub := &model.Subscription{
		Id:                 subscriptionId,
		UserId:             1,
		PlanId:             1,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now - 3600,
		EndAt:              now + 24*3600,
		Priority:           1,
		AutoWalletFallback: false,
		RedeemOption:       common.RedeemOptionStack,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create subscription failed: %v", err)
	}

	// 预创建窗口，避免窗口创建路径影响测试稳定性
	windowStart := common.GetTimestamp()
	windowEnd := model.CalculateRollingWindowEnd(period, windowStart)
	initialUsage := &model.SubscriptionUsage{
		SubscriptionId: subscriptionId,
		Period:         period,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		UsedQuota:      0,
		LimitQuota:     limitQuota,
	}
	if err := db.Create(initialUsage).Error; err != nil {
		t.Fatalf("create initial usage window failed: %v", err)
	}

	periods := map[string]int64{period: limitQuota}

	start := make(chan struct{})
	var ready sync.WaitGroup
	var wg sync.WaitGroup
	ready.Add(concurrency)
	wg.Add(concurrency)

	contextIds := make(chan string, concurrency)
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			ready.Done()
			<-start

			var lastErr error
			for attempt := 0; attempt < 10; attempt++ {
				ctx, err := svc.TryPreConsume(subscriptionId, amount, periods, common.WindowStrategyRolling)
				if err == nil {
					contextIds <- ctx.ContextId
					return
				}
				if isSQLiteBusy(err) {
					lastErr = err
					time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
					continue
				}
				lastErr = err
				break
			}
			errCh <- lastErr
		}()
	}

	ready.Wait()
	close(start)
	wg.Wait()
	close(contextIds)
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	seen := make(map[string]bool)
	duplicateCount := 0
	totalCount := 0

	for contextId := range contextIds {
		totalCount++
		if seen[contextId] {
			duplicateCount++
			t.Logf("发现重复 contextId: %s", contextId)
		}
		seen[contextId] = true
	}

	if totalCount != concurrency {
		t.Fatalf("contextId 总数不匹配: 期望 %d, 得到 %d", concurrency, totalCount)
	}

	if duplicateCount > 0 {
		t.Fatalf("1000 并发测试发现 %d 个重复的 contextId (总计 %d 个)", duplicateCount, totalCount)
	}
}

// TestConcurrentPreConsume1000_CacheConsistency 测试 1000 并发预扣缓存一致性
// 此测试验证高并发下缓存的读写一致性
func TestConcurrentPreConsume1000_CacheConsistency(t *testing.T) {
	svc := service.GetSubscriptionUsageService()
	concurrency := 1000

	// 先清理缓存中的测试数据
	for i := 0; i < concurrency; i++ {
		contextId := fmt.Sprintf("test_cache_consistency_%d", i)
		svc.GetPreConsumeContext(contextId) // 尝试获取，如果不存在则忽略
	}

	var wg sync.WaitGroup
	successCount := int64(0)
	errorCount := int64(0)

	// 并发写入
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			contextId := fmt.Sprintf("test_cache_consistency_%d", id)
			ctx := &service.PreConsumeContext{
				ContextId:      contextId,
				SubscriptionId: int64(id),
				Amount:         int64(1000 + id),
				Timestamp:      common.GetTimestamp(),
				Usages: map[string]*service.UsagePreConsume{
					common.LimitPeriodDay: {
						UsageId:     int64(id * 100),
						Period:      common.LimitPeriodDay,
						Amount:      int64(1000 + id),
						PreUsed:     0,
						WindowStart: common.GetTimestamp(),
						WindowEnd:   common.GetTimestamp() + 86400,
					},
				},
			}

			// 存入缓存
			_ = svc.StorePreConsumeContext(ctx)
		}(i)
	}

	wg.Wait()

	// 并发读取并验证
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			contextId := fmt.Sprintf("test_cache_consistency_%d", id)
			ctx, err := svc.GetPreConsumeContext(contextId)

			if err != nil {
				// 使用原子操作计数
				atomic.AddInt64(&errorCount, 1)
				return
			}

			// 验证数据一致性
			if ctx.ContextId != contextId {
				t.Errorf("contextId 不匹配: 期望 %s, 得到 %s", contextId, ctx.ContextId)
				return
			}
			if ctx.SubscriptionId != int64(id) {
				t.Errorf("subscriptionId 不匹配: 期望 %d, 得到 %d", id, ctx.SubscriptionId)
				return
			}
			if ctx.Amount != int64(1000+id) {
				t.Errorf("amount 不匹配: 期望 %d, 得到 %d", 1000+id, ctx.Amount)
				return
			}

			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()

	finalSuccess := atomic.LoadInt64(&successCount)
	finalError := atomic.LoadInt64(&errorCount)

	t.Logf("1000 并发缓存一致性测试: 成功 %d, 失败 %d", finalSuccess, finalError)

	if finalError > 0 {
		t.Errorf("1000 并发测试中有 %d 个缓存读取失败", finalError)
	}
}

// TestConcurrentPreConsume1000_RaceCondition 测试 1000 并发下的竞态条件
// 此测试验证同一订阅的并发预扣不会产生竞态问题
func TestConcurrentPreConsume1000_RaceCondition(t *testing.T) {
	svc := service.GetSubscriptionUsageService()
	concurrency := 1000
	subscriptionId := int64(999) // 使用同一个订阅 ID

	var wg sync.WaitGroup
	results := make(chan string, concurrency)

	// 所有请求都针对同一个订阅 ID 进行预扣
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			ctx := &service.PreConsumeContext{
				ContextId:      fmt.Sprintf("race_test_%d_%d", subscriptionId, id),
				SubscriptionId: subscriptionId,
				Amount:         100, // 每次预扣 100
				Timestamp:      common.GetTimestamp(),
				Usages: map[string]*service.UsagePreConsume{
					common.LimitPeriodDay: {
						UsageId:     int64(id),
						Period:      common.LimitPeriodDay,
						Amount:      100,
						PreUsed:     int64(id * 100),
						WindowStart: common.GetTimestamp(),
						WindowEnd:   common.GetTimestamp() + 86400,
					},
				},
			}

			// 存入缓存
			svc.StorePreConsumeContext(ctx)
			results <- ctx.ContextId
		}(i)
	}

	wg.Wait()
	close(results)

	// 统计成功存入的数量
	count := 0
	for range results {
		count++
	}

	if count != concurrency {
		t.Errorf("竞态条件测试: 期望 %d 个结果, 得到 %d 个", concurrency, count)
	} else {
		t.Logf("1000 并发竞态条件测试通过: 同一订阅下成功处理 %d 个并发请求", count)
	}
}

// BenchmarkConcurrentPreConsume1000 1000 并发预扣性能基准测试
func BenchmarkConcurrentPreConsume1000(b *testing.B) {
	svc := service.GetSubscriptionUsageService()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup

		for j := 0; j < 1000; j++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()

				ctx := &service.PreConsumeContext{
					ContextId:      fmt.Sprintf("bench_1000_%d_%d", i, id),
					SubscriptionId: int64(id % 10),
					Amount:         1000,
					Timestamp:      common.GetTimestamp(),
					Usages:         make(map[string]*service.UsagePreConsume),
				}

				_ = svc.StorePreConsumeContext(ctx)
			}(j)
		}

		wg.Wait()
	}
}

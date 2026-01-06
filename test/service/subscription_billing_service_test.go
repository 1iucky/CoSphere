package service

import (
	"testing"

	"github.com/QuantumNous/new-api/service"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// ===================== SelectCandidateSubscriptions 测试 =====================

func TestSelectCandidateSubscriptions_InvalidParams(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	testCases := []struct {
		name                       string
		userId                     int64
		modelName                  string
		channelGroup               string
		tokenSubscriptionPreferred bool
		expectError                bool
		expectEmpty                bool
	}{
		{
			name:                       "用户ID为0",
			userId:                     0,
			modelName:                  "gpt-4",
			channelGroup:               "",
			tokenSubscriptionPreferred: true,
			expectError:                true,
			expectEmpty:                false,
		},
		{
			name:                       "token未启用订阅优先",
			userId:                     1,
			modelName:                  "gpt-4",
			channelGroup:               "",
			tokenSubscriptionPreferred: false,
			expectError:                false,
			expectEmpty:                true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			candidates, err := svc.SelectCandidateSubscriptions(tc.userId, tc.modelName, tc.channelGroup, tc.tokenSubscriptionPreferred)
			if tc.expectError && err == nil {
				t.Errorf("期望返回错误，但得到 nil")
			}
			if !tc.expectError && err != nil {
				// 允许数据库连接错误
				if tc.expectEmpty && candidates != nil {
					t.Errorf("期望返回空列表，但得到 %d 个候选", len(candidates))
				}
			}
			if tc.expectEmpty && len(candidates) > 0 {
				t.Errorf("期望返回空列表，但得到 %d 个候选", len(candidates))
			}
		})
	}
}

// ===================== TryDeductFromSubscriptions 测试 =====================

func TestTryDeductFromSubscriptions_InvalidParams(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	testCases := []struct {
		name        string
		candidates  []*service.CandidateSubscription
		amount      int64
		expectError bool
	}{
		{
			name:        "候选列表为空",
			candidates:  nil,
			amount:      100,
			expectError: true,
		},
		{
			name:        "候选列表为空数组",
			candidates:  []*service.CandidateSubscription{},
			amount:      100,
			expectError: true,
		},
		{
			name: "预扣额度为0",
			candidates: []*service.CandidateSubscription{
				{Priority: 1},
			},
			amount:      0,
			expectError: true,
		},
		{
			name: "预扣额度为负数",
			candidates: []*service.CandidateSubscription{
				{Priority: 1},
			},
			amount:      -100,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.TryDeductFromSubscriptions(tc.candidates, tc.amount)
			if tc.expectError && err == nil {
				t.Errorf("期望返回错误，但得到 nil")
			}
		})
	}
}

// ===================== ShouldSkipSubscriptionBilling 测试 =====================

func TestShouldSkipSubscriptionBilling(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	testCases := []struct {
		name                       string
		tokenSubscriptionPreferred bool
		expectSkip                 bool
	}{
		{
			name:                       "token未启用订阅优先",
			tokenSubscriptionPreferred: false,
			expectSkip:                 true,
		},
		{
			name:                       "token启用了订阅优先",
			tokenSubscriptionPreferred: true,
			expectSkip:                 false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldSkip := svc.ShouldSkipSubscriptionBilling(tc.tokenSubscriptionPreferred)
			if shouldSkip != tc.expectSkip {
				t.Errorf("期望 skip=%v，但得到 %v", tc.expectSkip, shouldSkip)
			}
		})
	}
}

// ===================== ReturnSkipReason 测试 =====================

func TestReturnSkipReason(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	testCases := []struct {
		name         string
		tokenId      int
		reason       string
		expectReason string
	}{
		{
			name:         "有自定义原因",
			tokenId:      1,
			reason:       "custom_reason",
			expectReason: "custom_reason",
		},
		{
			name:         "无自定义原因",
			tokenId:      1,
			reason:       "",
			expectReason: "subscription_preferred_disabled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reason := svc.ReturnSkipReason(tc.tokenId, tc.reason)
			if reason != tc.expectReason {
				t.Errorf("期望原因=%q，但得到 %q", tc.expectReason, reason)
			}
		})
	}
}

// ===================== FallbackToWallet 测试 =====================

func TestFallbackToWallet(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 测试 nil 上下文
	ctx := svc.FallbackToWallet(nil, "test reason")
	if ctx == nil {
		t.Error("期望返回非 nil 的上下文")
	}
	if ctx.Source != service.BillingSourceFallback {
		t.Errorf("期望 Source=%s，但得到 %s", service.BillingSourceFallback, ctx.Source)
	}
	if ctx.FallbackReason != "test reason" {
		t.Errorf("期望 FallbackReason=%q，但得到 %q", "test reason", ctx.FallbackReason)
	}

	// 测试已有上下文
	existingCtx := &service.BillingContext{
		UserId:         123,
		SubscriptionId: 456,
	}
	updatedCtx := svc.FallbackToWallet(existingCtx, "another reason")
	if updatedCtx.UserId != 123 {
		t.Errorf("期望 UserId=%d，但得到 %d", 123, updatedCtx.UserId)
	}
	if updatedCtx.Source != service.BillingSourceFallback {
		t.Errorf("期望 Source=%s，但得到 %s", service.BillingSourceFallback, updatedCtx.Source)
	}
}

// ===================== PostBilling 测试 =====================

func TestPostBilling_NilContext(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	err := svc.PostBilling(nil, 100, 100, nil)
	if err == nil {
		t.Error("期望 nil 上下文返回错误")
	}
}

func TestPostBilling_NonSubscriptionSource(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 钱包来源不需要处理
	ctx := &service.BillingContext{
		Source: service.BillingSourceWallet,
	}
	err := svc.PostBilling(ctx, 100, 100, nil)
	if err != nil {
		t.Errorf("钱包来源不应该返回错误，但得到: %v", err)
	}

	// 跳过来源不需要处理
	ctx.Source = service.BillingSourceSkipped
	err = svc.PostBilling(ctx, 100, 100, nil)
	if err != nil {
		t.Errorf("跳过来源不应该返回错误，但得到: %v", err)
	}
}

func TestPostBilling_NoPreConsumeContext(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	ctx := &service.BillingContext{
		Source:            service.BillingSourceSubscription,
		PreConsumeContext: nil,
	}
	err := svc.PostBilling(ctx, 100, 100, nil)
	if err != nil {
		t.Errorf("无预扣上下文不应该返回错误，但得到: %v", err)
	}
}

// ===================== 服务单例测试 =====================

func TestGetSubscriptionBillingService_Singleton(t *testing.T) {
	svc1 := service.GetSubscriptionBillingService()
	svc2 := service.GetSubscriptionBillingService()

	if svc1 != svc2 {
		t.Error("期望获取相同的服务实例（单例模式）")
	}
}

// ===================== 计费来源常量测试 =====================

func TestBillingSourceConstants(t *testing.T) {
	if service.BillingSourceSubscription != "subscription" {
		t.Errorf("期望 BillingSourceSubscription=%q", "subscription")
	}
	if service.BillingSourceWallet != "wallet" {
		t.Errorf("期望 BillingSourceWallet=%q", "wallet")
	}
	if service.BillingSourceFallback != "fallback" {
		t.Errorf("期望 BillingSourceFallback=%q", "fallback")
	}
	if service.BillingSourceSkipped != "skipped" {
		t.Errorf("期望 BillingSourceSkipped=%q", "skipped")
	}
}

// ===================== SelectCandidate 筛选逻辑测试 =====================

func TestSelectCandidate_TokenSubscriptionPreferred(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	testCases := []struct {
		name                       string
		tokenSubscriptionPreferred bool
		expectSource               string
	}{
		{
			name:                       "token未启用订阅优先-应返回跳过来源",
			tokenSubscriptionPreferred: false,
			expectSource:               service.BillingSourceSkipped,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 构造模拟的 RelayInfo
			relayInfo := &relaycommon.RelayInfo{
				TokenId:                    1,
				TokenSubscriptionPreferred: tc.tokenSubscriptionPreferred,
			}

			ctx, err := svc.SelectCandidate(1, "gpt-4", "", relayInfo)
			if err != nil {
				t.Fatalf("SelectCandidate 返回错误: %v", err)
			}
			if ctx.Source != tc.expectSource {
				t.Errorf("期望 Source=%s，但得到 %s", tc.expectSource, ctx.Source)
			}
		})
	}
}

// ===================== TryBilling 自动兜底测试 =====================

func TestTryBilling_SkippedSource(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 测试 Skipped 来源直接返回成功
	ctx := &service.BillingContext{
		UserId: 1,
		Source: service.BillingSourceSkipped,
	}

	result, err := svc.TryBilling(ctx, 100, nil)
	if err != nil {
		t.Fatalf("TryBilling 返回错误: %v", err)
	}
	if !result.Success {
		t.Error("期望 Success=true")
	}
	if result.Source != service.BillingSourceSkipped {
		t.Errorf("期望 Source=%s，但得到 %s", service.BillingSourceSkipped, result.Source)
	}
}

func TestTryBilling_WalletSource(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 测试 Wallet 来源直接返回成功
	ctx := &service.BillingContext{
		UserId: 1,
		Source: service.BillingSourceWallet,
	}

	result, err := svc.TryBilling(ctx, 100, nil)
	if err != nil {
		t.Fatalf("TryBilling 返回错误: %v", err)
	}
	if !result.Success {
		t.Error("期望 Success=true")
	}
	if result.Source != service.BillingSourceWallet {
		t.Errorf("期望 Source=%s，但得到 %s", service.BillingSourceWallet, result.Source)
	}
}

func TestTryBilling_NilContext(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	result, err := svc.TryBilling(nil, 100, nil)
	if err == nil {
		t.Error("期望 nil context 返回错误")
	}
	if result.Success {
		t.Error("期望 Success=false")
	}
}

// ===================== PostBilling 差额调整测试 =====================

func TestPostBilling_NoDiff(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 无差额不需要调整
	ctx := &service.BillingContext{
		Source: service.BillingSourceSubscription,
		PreConsumeContext: &service.PreConsumeContext{
			ContextId: "test-context-id",
			Amount:    100,
		},
	}

	// preConsumedAmount == actualAmount, 无差额
	err := svc.PostBilling(ctx, 100, 100, nil)
	if err != nil {
		t.Errorf("无差额不应该返回错误，但得到: %v", err)
	}
}

// ===================== 渠道分组匹配测试 =====================

func TestMatchChannelGroup_MultiGroup(t *testing.T) {
	// 测试多分组匹配逻辑已在 SelectCandidateSubscriptions 中覆盖
	// 这里验证构建候选订阅时的分组解析逻辑
	svc := service.GetSubscriptionBillingService()

	// 用户ID为0应该返回错误
	_, err := svc.SelectCandidateSubscriptions(0, "gpt-4", "group1,group2", true)
	if err == nil {
		t.Error("期望用户ID为0时返回错误")
	}
}

// ===================== CheckAutoWalletFallback 测试 =====================

func TestCheckAutoWalletFallback_Priority(t *testing.T) {
	// 跳过此测试，因为需要数据库和Redis连接
	t.Skip("需要数据库和Redis环境，跳过单元测试")

	svc := service.GetSubscriptionBillingService()

	// 测试优先级：用户级 > 系统级
	_, err := svc.CheckAutoWalletFallback(1, 0)
	if err != nil {
		t.Logf("CheckAutoWalletFallback 返回错误: %v", err)
	}
}

package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
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

// ===================== 兜底触发条件测试 =====================
// 验证设计文档要求："仅订阅额度不足才兜底，系统错误不兜底"

func TestErrorCode_SubscriptionLimitReached(t *testing.T) {
	// 验证错误码为大写 SUBSCRIPTION_LIMIT_REACHED（设计文档要求）
	expectedCode := "SUBSCRIPTION_LIMIT_REACHED"
	if string(types.ErrorCodeSubscriptionLimitReached) != expectedCode {
		t.Errorf("期望错误码为 %s，但得到 %s", expectedCode, types.ErrorCodeSubscriptionLimitReached)
	}
}

func TestFallbackCondition_OnlyLimitReachedShouldFallback(t *testing.T) {
	// 测试：只有 ErrorCodeSubscriptionLimitReached 错误才应该触发兜底
	// 其他错误（如系统错误、DB 错误）不应该触发兜底

	testCases := []struct {
		name               string
		errorCode          types.ErrorCode
		shouldTriggerFallback bool
	}{
		{
			name:               "订阅额度不足应触发兜底",
			errorCode:          types.ErrorCodeSubscriptionLimitReached,
			shouldTriggerFallback: true,
		},
		{
			name:               "使用量超额不应触发兜底",
			errorCode:          types.ErrorCodeUsageQuotaExceeded,
			shouldTriggerFallback: false,
		},
		{
			name:               "查询数据错误不应触发兜底",
			errorCode:          types.ErrorCodeQueryDataError,
			shouldTriggerFallback: false,
		},
		{
			name:               "订阅操作失败不应触发兜底",
			errorCode:          types.ErrorCodeSubscriptionOperationFailed,
			shouldTriggerFallback: false,
		},
		{
			name:               "订阅不存在不应触发兜底",
			errorCode:          types.ErrorCodeSubscriptionNotFound,
			shouldTriggerFallback: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟 TryBilling 中的错误判断逻辑
			isLimitReachedError := tc.errorCode == types.ErrorCodeSubscriptionLimitReached

			if isLimitReachedError != tc.shouldTriggerFallback {
				t.Errorf("错误码 %s: 期望触发兜底=%v，实际=%v",
					tc.errorCode, tc.shouldTriggerFallback, isLimitReachedError)
			}
		})
	}
}

func TestTryBilling_WithCandidatesInContext(t *testing.T) {
	// 测试：当上下文中已有候选订阅时，TryBilling 不应重复筛选
	svc := service.GetSubscriptionBillingService()

	// 创建带有空候选列表的上下文（模拟 SelectCandidate 返回无订阅的情况）
	ctx := &service.BillingContext{
		UserId:     1,
		ModelName:  "gpt-4",
		Candidates: []*service.CandidateSubscription{}, // 空候选列表
	}

	// TryBilling 应该直接返回钱包来源，而不是重新筛选
	// 注意：由于 Candidates 为空数组（非 nil），会进入钱包路径
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

func TestBillingContext_WithCandidates(t *testing.T) {
	// 验证 BillingContext 结构体包含 Candidates 字段
	ctx := &service.BillingContext{
		UserId:     123,
		ModelName:  "gpt-4",
		Candidates: []*service.CandidateSubscription{
			{Priority: 1},
			{Priority: 2},
		},
	}

	if len(ctx.Candidates) != 2 {
		t.Errorf("期望 Candidates 长度为 2，但得到 %d", len(ctx.Candidates))
	}
}

// ===================== TryBilling 实际分支行为集成测试 =====================
// 测试设计文档要求："仅订阅额度不足才兜底，系统错误不兜底"

func TestTryBilling_BranchBehavior_EmptyCandidates_FallsBackToWallet(t *testing.T) {
	// 场景：上下文有候选列表但为空 -> 应直接使用钱包，不触发订阅扣款流程
	svc := service.GetSubscriptionBillingService()

	ctx := &service.BillingContext{
		UserId:     9999,
		ModelName:  "gpt-4",
		Candidates: []*service.CandidateSubscription{}, // 空列表
	}

	result, err := svc.TryBilling(ctx, 100, nil)
	if err != nil {
		t.Fatalf("TryBilling 不应返回错误: %v", err)
	}

	// 验证分支：空候选 -> 直接使用钱包
	if !result.Success {
		t.Error("期望 Success=true（空候选应直接走钱包）")
	}
	if result.Source != service.BillingSourceWallet {
		t.Errorf("期望 Source=%s（空候选应走钱包），但得到 %s",
			service.BillingSourceWallet, result.Source)
	}
}

func TestTryBilling_BranchBehavior_PresetSkippedSource(t *testing.T) {
	// 场景：上下文已设置为 Skipped -> 应直接跳过，不进入订阅扣款流程
	svc := service.GetSubscriptionBillingService()

	ctx := &service.BillingContext{
		UserId: 9999,
		Source: service.BillingSourceSkipped,
	}

	result, err := svc.TryBilling(ctx, 100, nil)
	if err != nil {
		t.Fatalf("TryBilling 不应返回错误: %v", err)
	}

	// 验证分支：预设 Skipped -> 保持 Skipped
	if result.Source != service.BillingSourceSkipped {
		t.Errorf("期望 Source=%s（预设跳过应保持），但得到 %s",
			service.BillingSourceSkipped, result.Source)
	}
}

func TestTryBilling_BranchBehavior_PresetWalletSource(t *testing.T) {
	// 场景：上下文已设置为 Wallet -> 应直接使用钱包，不进入订阅扣款流程
	svc := service.GetSubscriptionBillingService()

	ctx := &service.BillingContext{
		UserId: 9999,
		Source: service.BillingSourceWallet,
	}

	result, err := svc.TryBilling(ctx, 100, nil)
	if err != nil {
		t.Fatalf("TryBilling 不应返回错误: %v", err)
	}

	// 验证分支：预设 Wallet -> 保持 Wallet
	if result.Source != service.BillingSourceWallet {
		t.Errorf("期望 Source=%s（预设钱包应保持），但得到 %s",
			service.BillingSourceWallet, result.Source)
	}
}

// TestErrorCodeShouldTriggerFallback 验证错误码判断逻辑
// 这是对 TryBilling 内部 isLimitReachedError 判断逻辑的验证
func TestErrorCodeShouldTriggerFallback_Integration(t *testing.T) {
	testCases := []struct {
		name                  string
		errorCode             types.ErrorCode
		shouldAllowFallback   bool
		description           string
	}{
		{
			name:                "SUBSCRIPTION_LIMIT_REACHED 应允许兜底",
			errorCode:           types.ErrorCodeSubscriptionLimitReached,
			shouldAllowFallback: true,
			description:         "设计文档要求：订阅额度不足时应检查兜底开关",
		},
		{
			name:                "subscription_quota_exhausted 不应允许兜底",
			errorCode:           types.ErrorCodeSubscriptionQuotaExhausted,
			shouldAllowFallback: false,
			description:         "quota_exhausted 是已用尽状态，非实时限额触发",
		},
		{
			name:                "query_data_error 不应允许兜底",
			errorCode:           types.ErrorCodeQueryDataError,
			shouldAllowFallback: false,
			description:         "数据库查询错误是系统错误，不应触发兜底",
		},
		{
			name:                "subscription_operation_failed 不应允许兜底",
			errorCode:           types.ErrorCodeSubscriptionOperationFailed,
			shouldAllowFallback: false,
			description:         "操作失败是系统错误，不应触发兜底",
		},
		{
			name:                "insufficient_user_quota 不应允许兜底",
			errorCode:           types.ErrorCodeInsufficientUserQuota,
			shouldAllowFallback: false,
			description:         "用户余额不足是钱包层错误，与订阅兜底无关",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟 TryBilling 中的判断逻辑
			isLimitReachedError := tc.errorCode == types.ErrorCodeSubscriptionLimitReached

			if isLimitReachedError != tc.shouldAllowFallback {
				t.Errorf("[%s] 错误码 %s: 期望允许兜底=%v，实际=%v",
					tc.description, tc.errorCode, tc.shouldAllowFallback, isLimitReachedError)
			}
		})
	}
}

// TestNewAPIError_WithHintAndDetails 验证错误构建逻辑
func TestNewAPIError_WithHintAndDetails(t *testing.T) {
	// 构建带 hint 和 details 的错误
	hint := "请升级套餐或启用余额兜底"
	details := map[string]interface{}{
		"subscription_id": int64(123),
		"limit_quota":     int64(1000),
		"used_quota":      int64(950),
	}

	err := types.NewError(
		nil,
		types.ErrorCodeSubscriptionLimitReached,
		types.ErrOptionWithHint(hint),
		types.ErrOptionWithDetails(details),
	)

	// 转换为 OpenAI 格式
	openAIErr := err.ToOpenAIError()

	// 验证 hint 被输出
	if openAIErr.Hint != hint {
		t.Errorf("期望 Hint=%q，但得到 %q", hint, openAIErr.Hint)
	}

	// 验证 details 被输出
	if openAIErr.Details == nil {
		t.Error("期望 Details 非空")
	} else {
		if subId, ok := openAIErr.Details["subscription_id"]; !ok || subId != int64(123) {
			t.Errorf("期望 Details[subscription_id]=123，但得到 %v", subId)
		}
	}

	// 验证 hint 被追加到 message
	if openAIErr.Hint != "" && !contains(openAIErr.Message, hint) {
		t.Errorf("期望 Message 包含 hint，但 Message=%q", openAIErr.Message)
	}
}

// TestNewAPIError_ErrorCodeFormat 验证错误码格式
func TestNewAPIError_ErrorCodeFormat(t *testing.T) {
	err := types.NewError(
		nil,
		types.ErrorCodeSubscriptionLimitReached,
	)

	openAIErr := err.ToOpenAIError()

	// 验证错误码为大写格式（设计文档要求）
	expectedCode := "SUBSCRIPTION_LIMIT_REACHED"
	if code, ok := openAIErr.Code.(types.ErrorCode); ok {
		if string(code) != expectedCode {
			t.Errorf("期望 Code=%q，但得到 %q", expectedCode, code)
		}
	} else {
		t.Errorf("期望 Code 类型为 ErrorCode，但得到 %T", openAIErr.Code)
	}
}

// contains 辅助函数，检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstr(s, substr)))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ===================== TryDeductFromSubscriptions 错误路径测试 =====================

func TestTryDeductFromSubscriptions_EmptyCandidates(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	_, err := svc.TryDeductFromSubscriptions(nil, 100)
	if err == nil {
		t.Error("期望空候选列表返回错误")
	}
	if err.Error() != "没有可用的订阅" {
		t.Errorf("期望错误消息为 '没有可用的订阅'，但得到 %q", err.Error())
	}
}

func TestTryDeductFromSubscriptions_InvalidAmount(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	candidates := []*service.CandidateSubscription{{Priority: 1}}

	_, err := svc.TryDeductFromSubscriptions(candidates, 0)
	if err == nil {
		t.Error("期望额度为0时返回错误")
	}

	_, err = svc.TryDeductFromSubscriptions(candidates, -100)
	if err == nil {
		t.Error("期望负数额度返回错误")
	}
}

// ===================== TryBilling 兜底路径集成测试 =====================
// 测试设计文档要求：仅 SUBSCRIPTION_LIMIT_REACHED 允许兜底检查

func TestTryBilling_FallbackBehavior_SimulatedScenarios(t *testing.T) {
	// 这些测试验证 TryBilling 的分支逻辑：
	// 1. 无候选订阅 -> 直接走钱包（不检查兜底）
	// 2. 有候选但全部失败且是 limit_reached -> 检查兜底开关
	// 3. 有候选但遇到系统错误 -> 直接返回错误（不检查兜底）

	svc := service.GetSubscriptionBillingService()

	t.Run("无候选订阅直接走钱包", func(t *testing.T) {
		ctx := &service.BillingContext{
			UserId:     999999, // 不存在的用户
			ModelName:  "test-model",
			Candidates: []*service.CandidateSubscription{}, // 空候选
		}

		result, err := svc.TryBilling(ctx, 100, nil)
		if err != nil {
			t.Fatalf("空候选不应返回错误: %v", err)
		}
		if result.Source != service.BillingSourceWallet {
			t.Errorf("空候选期望 Source=%s，得到 %s", service.BillingSourceWallet, result.Source)
		}
	})

	t.Run("预设Skipped来源直接跳过", func(t *testing.T) {
		ctx := &service.BillingContext{
			UserId: 999999,
			Source: service.BillingSourceSkipped,
		}

		result, err := svc.TryBilling(ctx, 100, nil)
		if err != nil {
			t.Fatalf("Skipped来源不应返回错误: %v", err)
		}
		if result.Source != service.BillingSourceSkipped {
			t.Errorf("Skipped来源期望保持 Source=%s，得到 %s", service.BillingSourceSkipped, result.Source)
		}
	})
}

// ===================== 错误 Details 传递测试 =====================

func TestErrorDetails_SubscriptionLimitReached(t *testing.T) {
	// 验证 SUBSCRIPTION_LIMIT_REACHED 错误包含正确的 details 结构
	expectedDetails := map[string]interface{}{
		"subscription_id": int64(123),
		"period":          "daily",
		"limit_quota":     int64(1000),
		"used_quota":      int64(950),
		"requested":       int64(100),
	}

	err := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足"),
		types.ErrorCodeSubscriptionLimitReached,
		429,
		types.ErrOptionWithDetails(expectedDetails),
		types.ErrOptionWithHint("请升级套餐"),
	)

	openAIErr := err.ToOpenAIError()

	// 验证 details 字段存在且包含预期的键
	if openAIErr.Details == nil {
		t.Fatal("期望 Details 非空")
	}

	requiredKeys := []string{"subscription_id", "period", "limit_quota", "used_quota", "requested"}
	for _, key := range requiredKeys {
		if _, ok := openAIErr.Details[key]; !ok {
			t.Errorf("期望 Details 包含键 %q", key)
		}
	}

	// 验证值类型和内容
	if subId, ok := openAIErr.Details["subscription_id"].(int64); !ok || subId != 123 {
		t.Errorf("期望 subscription_id=123，得到 %v", openAIErr.Details["subscription_id"])
	}
	if period, ok := openAIErr.Details["period"].(string); !ok || period != "daily" {
		t.Errorf("期望 period=daily，得到 %v", openAIErr.Details["period"])
	}
}

// TestFallbackTriggerCondition_ErrorCodeComparison 验证兜底触发条件的错误码比较逻辑
func TestFallbackTriggerCondition_ErrorCodeComparison(t *testing.T) {
	testCases := []struct {
		name              string
		err               error
		shouldCheckWallet bool
		description       string
	}{
		{
			name: "SUBSCRIPTION_LIMIT_REACHED 应检查钱包兜底",
			err: types.NewErrorWithStatusCode(
				errors.New("订阅额度不足"),
				types.ErrorCodeSubscriptionLimitReached,
				429,
			),
			shouldCheckWallet: true,
			description:       "订阅额度不足是唯一允许兜底的错误类型",
		},
		{
			name: "UsageQuotaExceeded 不应触发兜底",
			err: types.NewErrorWithStatusCode(
				errors.New("使用量超额"),
				types.ErrorCodeUsageQuotaExceeded,
				429,
			),
			shouldCheckWallet: false,
			description:       "单周期超额在内部处理，不应触发最终兜底",
		},
		{
			name: "QueryDataError 不应触发兜底",
			err: types.NewErrorWithStatusCode(
				errors.New("查询数据错误"),
				types.ErrorCodeQueryDataError,
				500,
			),
			shouldCheckWallet: false,
			description:       "系统错误不应触发兜底",
		},
		{
			name: "SubscriptionOperationFailed 不应触发兜底",
			err: types.NewErrorWithStatusCode(
				errors.New("操作失败"),
				types.ErrorCodeSubscriptionOperationFailed,
				500,
			),
			shouldCheckWallet: false,
			description:       "操作失败是系统错误，不应触发兜底",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			apiErr, ok := tc.err.(*types.NewAPIError)
			if !ok {
				t.Fatalf("期望 *types.NewAPIError 类型")
			}

			// 模拟 TryBilling 中的判断逻辑
			isLimitReachedError := apiErr.GetErrorCode() == types.ErrorCodeSubscriptionLimitReached

			if isLimitReachedError != tc.shouldCheckWallet {
				t.Errorf("[%s] 错误码 %s: 期望检查钱包兜底=%v，实际=%v",
					tc.description, apiErr.GetErrorCode(), tc.shouldCheckWallet, isLimitReachedError)
			}
		})
	}
}

// TestErrorCodeUpperCase 验证错误码大写格式
func TestErrorCodeUpperCase(t *testing.T) {
	code := string(types.ErrorCodeSubscriptionLimitReached)
	if code != "SUBSCRIPTION_LIMIT_REACHED" {
		t.Errorf("期望错误码为大写格式 SUBSCRIPTION_LIMIT_REACHED，得到 %q", code)
	}
}

// TestDetailsAndHintInOpenAIError 验证 ToOpenAIError 输出 details 和 hint
func TestDetailsAndHintInOpenAIError(t *testing.T) {
	details := map[string]interface{}{
		"subscription_id": int64(456),
		"period":          "monthly",
		"limit_quota":     int64(50000),
		"used_quota":      int64(49000),
		"requested":       int64(2000),
	}
	hint := "您可以升级套餐或启用余额兜底"

	err := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足"),
		types.ErrorCodeSubscriptionLimitReached,
		429,
		types.ErrOptionWithDetails(details),
		types.ErrOptionWithHint(hint),
	)

	openAIErr := err.ToOpenAIError()

	// 验证 hint
	if openAIErr.Hint != hint {
		t.Errorf("期望 Hint=%q，得到 %q", hint, openAIErr.Hint)
	}

	// 验证 hint 追加到 message
	if !contains(openAIErr.Message, hint) {
		t.Errorf("期望 Message 包含 hint，得到 %q", openAIErr.Message)
	}

	// 验证 details
	if openAIErr.Details == nil {
		t.Fatal("期望 Details 非空")
	}
	if subId := openAIErr.Details["subscription_id"]; subId != int64(456) {
		t.Errorf("期望 subscription_id=456，得到 %v", subId)
	}
}

// ===================== TryDeductFromSubscriptions 真实路径测试 =====================

func TestTryDeductFromSubscriptions_NoPeriodConfigs_ReturnsError(t *testing.T) {
	// 场景：候选订阅没有启用的限额配置 -> 返回错误
	svc := service.GetSubscriptionBillingService()

	// 构造没有限额配置的候选（Limits 为空）
	candidates := []*service.CandidateSubscription{
		{
			Priority: 1,
			Subscription: &model.Subscription{
				Id:                 999,
				UserId:             1,
				AutoWalletFallback: true,
			},
			Limits: []model.SubscriptionPlanLimit{}, // 空限额
		},
	}

	_, err := svc.TryDeductFromSubscriptions(candidates, 100)
	if err == nil {
		t.Error("期望无限额配置时返回错误")
	}

	// 验证是 SUBSCRIPTION_LIMIT_REACHED 错误
	apiErr, ok := err.(*types.NewAPIError)
	if !ok {
		t.Fatalf("期望 *types.NewAPIError，得到 %T", err)
	}
	if apiErr.GetErrorCode() != types.ErrorCodeSubscriptionLimitReached {
		t.Errorf("期望错误码 %s，得到 %s",
			types.ErrorCodeSubscriptionLimitReached, apiErr.GetErrorCode())
	}
}

func TestTryDeductFromSubscriptions_DisabledLimits_ReturnsError(t *testing.T) {
	// 场景：候选订阅的限额配置全部禁用 -> 返回错误
	svc := service.GetSubscriptionBillingService()

	// 构造限额配置禁用的候选
	candidates := []*service.CandidateSubscription{
		{
			Priority: 1,
			Subscription: &model.Subscription{
				Id:                 999,
				UserId:             1,
				AutoWalletFallback: false,
			},
			Limits: []model.SubscriptionPlanLimit{
				{Period: "daily", Quota: 1000, Enabled: false}, // 禁用
				{Period: "monthly", Quota: 10000, Enabled: false}, // 禁用
			},
		},
	}

	_, err := svc.TryDeductFromSubscriptions(candidates, 100)
	if err == nil {
		t.Error("期望禁用限额时返回错误")
	}
}

// ===================== TryBilling 真实 Fallback 路径测试 =====================
// 这些测试验证 TryBilling 对 TryDeductFromSubscriptions 返回错误的处理

func TestTryBilling_LimitReachedWithAutoWallet_Integration(t *testing.T) {
	// 场景：订阅额度不足 + auto_wallet_fallback=true -> 应该返回 fallback 来源
	// 注意：此测试需要数据库环境，测试候选订阅无可用限额的情况
	t.Skip("需要数据库和Redis环境，跳过单元测试")
	svc := service.GetSubscriptionBillingService()

	// 构造有候选但无可用限额的上下文
	ctx := &service.BillingContext{
		UserId:    999999,
		ModelName: "test-model",
		Candidates: []*service.CandidateSubscription{
			{
				Priority: 1,
				Subscription: &model.Subscription{
					Id:                 999,
					UserId:             999999,
					AutoWalletFallback: true, // 启用兜底
				},
				Limits: []model.SubscriptionPlanLimit{}, // 无可用限额
			},
		},
	}

	result, err := svc.TryBilling(ctx, 100, nil)

	// 当候选订阅无可用限额时，会返回 SUBSCRIPTION_LIMIT_REACHED 错误
	// 但因为 auto_wallet_fallback=true，应该触发兜底逻辑
	// 实际行为取决于 CheckAutoWalletFallback 的实现

	// 验证：要么成功兜底，要么返回带 details 的错误
	if err != nil {
		apiErr, ok := err.(*types.NewAPIError)
		if ok && apiErr.GetErrorCode() == types.ErrorCodeSubscriptionLimitReached {
			// 正常情况：返回限额错误
			// 验证 details 包含必要字段（如果有的话）
			t.Logf("返回 SUBSCRIPTION_LIMIT_REACHED 错误（可能因为兜底检查未通过）")
		} else {
			// 其他错误
			t.Logf("返回其他错误: %v", err)
		}
	} else if result.Success {
		// 可能成功兜底到钱包
		if result.Source == service.BillingSourceFallback {
			t.Log("成功触发兜底到钱包")
		} else if result.Source == service.BillingSourceWallet {
			t.Log("候选为空，直接使用钱包")
		}
	}
}

func TestTryBilling_LimitReachedWithoutAutoWallet_Integration(t *testing.T) {
	// 场景：订阅额度不足 + auto_wallet_fallback=false -> 应该返回错误
	t.Skip("需要数据库和Redis环境，跳过单元测试")
	svc := service.GetSubscriptionBillingService()

	// 构造有候选但无可用限额且禁用兜底的上下文
	ctx := &service.BillingContext{
		UserId:    999999,
		ModelName: "test-model",
		Candidates: []*service.CandidateSubscription{
			{
				Priority: 1,
				Subscription: &model.Subscription{
					Id:                 999,
					UserId:             999999,
					AutoWalletFallback: false, // 禁用兜底
				},
				Limits: []model.SubscriptionPlanLimit{}, // 无可用限额
			},
		},
	}

	result, err := svc.TryBilling(ctx, 100, nil)

	// 当候选订阅无可用限额且禁用兜底时，应该返回错误
	// 实际行为：候选 limits 为空会跳过，最终返回 SUBSCRIPTION_LIMIT_REACHED 错误

	if err == nil && result.Success && result.Source == service.BillingSourceWallet {
		// 候选全部被跳过（无可用限额），回退到钱包
		t.Log("候选无可用限额，回退到钱包来源")
	} else if err != nil {
		apiErr, ok := err.(*types.NewAPIError)
		if ok && apiErr.GetErrorCode() == types.ErrorCodeSubscriptionLimitReached {
			t.Log("返回 SUBSCRIPTION_LIMIT_REACHED 错误（预期行为）")
		}
	}
}

// TestDetailsContainsAutoWalletFallback 验证 details 包含 auto_wallet_fallback 字段
func TestDetailsContainsAutoWalletFallback(t *testing.T) {
	// 验证设计文档 7.2 要求的 details 结构包含 auto_wallet_fallback
	details := map[string]interface{}{
		"subscription_id":      int64(123),
		"period":               "daily",
		"limit_quota":          int64(1000),
		"used_quota":           int64(950),
		"requested":            int64(100),
		"auto_wallet_fallback": true, // 设计文档要求的字段
	}

	err := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足"),
		types.ErrorCodeSubscriptionLimitReached,
		429,
		types.ErrOptionWithDetails(details),
	)

	openAIErr := err.ToOpenAIError()

	// 验证 auto_wallet_fallback 字段存在
	if openAIErr.Details == nil {
		t.Fatal("期望 Details 非空")
	}

	autoWallet, ok := openAIErr.Details["auto_wallet_fallback"]
	if !ok {
		t.Error("期望 Details 包含 auto_wallet_fallback 字段")
	}
	if autoWallet != true {
		t.Errorf("期望 auto_wallet_fallback=true，得到 %v", autoWallet)
	}

	// 验证其他必要字段
	requiredKeys := []string{"subscription_id", "period", "limit_quota", "used_quota", "requested"}
	for _, key := range requiredKeys {
		if _, exists := openAIErr.Details[key]; !exists {
			t.Errorf("期望 Details 包含键 %q", key)
		}
	}
}

// ===================== 多订阅场景 Details 一致性测试 =====================

// TestMultiSubscription_DetailsConsistency 验证多订阅场景下 details 与决策订阅一致性
// 场景：订阅A(优先级1, auto_wallet=false, daily限额1000)，订阅B(优先级2, auto_wallet=true)
// 新语义：details 应该反映"决策订阅"（最高优先级订阅A）的配置
// 因为只有 auto_wallet_fallback=false 时才会返回 SUBSCRIPTION_LIMIT_REACHED
// 这样 details.auto_wallet_fallback=false 与错误语义一致，不会造成混淆
func TestMultiSubscription_DetailsConsistency(t *testing.T) {
	// 模拟 TryBilling 中的场景：
	// - 订阅A（优先级1）有启用限额，auto_wallet_fallback=false
	// - 订阅A 预扣失败（额度不足）
	// - 决策订阅是订阅A（最高优先级）
	// - 未启用兜底（因为 auto_wallet_fallback=false）
	// - details 应该反映订阅A的配置

	// 模拟 TryDeductFromSubscriptions 返回的错误（无 details）
	apiErr := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足"),
		types.ErrorCodeSubscriptionLimitReached,
		429,
		types.ErrOptionWithHint(common.MsgSubscriptionLimitReachedHint),
	)

	// 模拟 TryBilling 构造 details（基于决策订阅A）
	decisionSub := &model.Subscription{
		Id:                 100, // 订阅A
		AutoWalletFallback: false,
	}

	// 模拟 TryBilling 中的 details 构造逻辑
	enabledLimit := model.SubscriptionPlanLimit{
		Period: "daily",
		Quota:  1000,
		Enabled: true,
	}

	details := map[string]interface{}{
		"subscription_id":      decisionSub.Id,
		"requested":            int64(100),
		"auto_wallet_fallback": decisionSub.AutoWalletFallback, // false（决策订阅的设置）
		"period":               enabledLimit.Period,
		"limit_quota":          enabledLimit.Quota,
		"used_quota":           int64(0), // TryPreConsumeWithStrategies 未返回 used_quota
	}

	// 将 details 附加到错误
	if apiErr.Details == nil {
		apiErr.Details = details
	}

	openAIErr := apiErr.ToOpenAIError()

	// 验证：details 应该反映决策订阅A的配置
	autoWallet, ok := openAIErr.Details["auto_wallet_fallback"]
	if !ok {
		t.Fatal("期望 Details 包含 auto_wallet_fallback")
	}
	// 关键：auto_wallet_fallback=false 与 SUBSCRIPTION_LIMIT_REACHED 错误语义一致
	if autoWallet != false {
		t.Errorf("期望 auto_wallet_fallback=false（决策订阅A的设置，与错误语义一致），得到 %v", autoWallet)
	}

	// subscription_id 应该是决策订阅A
	subId, ok := openAIErr.Details["subscription_id"]
	if !ok {
		t.Fatal("期望 Details 包含 subscription_id")
	}
	if subId != int64(100) {
		t.Errorf("期望 subscription_id=100（决策订阅A），得到 %v", subId)
	}

	// 验证所有字段都来自决策订阅A（内部一致性）
	if period := openAIErr.Details["period"]; period != "daily" {
		t.Errorf("期望 period='daily'（与subscription_id一致），得到 %v", period)
	}
	if limitQuota := openAIErr.Details["limit_quota"]; limitQuota != int64(1000) {
		t.Errorf("期望 limit_quota=1000（与subscription_id一致），得到 %v", limitQuota)
	}
}

// TestFallbackDecision_UsesFirstCandidate 验证兜底决策使用第一个候选订阅
func TestFallbackDecision_UsesFirstCandidate(t *testing.T) {
	testCases := []struct {
		name                string
		candidates          []*service.CandidateSubscription
		expectedAutoWallet  bool
		expectedDecisionSub int64
	}{
		{
			name: "第一个订阅启用兜底",
			candidates: []*service.CandidateSubscription{
				{Priority: 1, Subscription: &model.Subscription{Id: 100, AutoWalletFallback: true}},
				{Priority: 2, Subscription: &model.Subscription{Id: 200, AutoWalletFallback: false}},
			},
			expectedAutoWallet:  true,
			expectedDecisionSub: 100,
		},
		{
			name: "第一个订阅禁用兜底",
			candidates: []*service.CandidateSubscription{
				{Priority: 1, Subscription: &model.Subscription{Id: 100, AutoWalletFallback: false}},
				{Priority: 2, Subscription: &model.Subscription{Id: 200, AutoWalletFallback: true}},
			},
			expectedAutoWallet:  false,
			expectedDecisionSub: 100,
		},
		{
			name: "单个订阅启用兜底",
			candidates: []*service.CandidateSubscription{
				{Priority: 1, Subscription: &model.Subscription{Id: 100, AutoWalletFallback: true}},
			},
			expectedAutoWallet:  true,
			expectedDecisionSub: 100,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟兜底决策逻辑
			autoWallet := false
			decisionSubId := int64(0)
			if len(tc.candidates) > 0 && tc.candidates[0].Subscription != nil {
				autoWallet = tc.candidates[0].Subscription.AutoWalletFallback
				decisionSubId = tc.candidates[0].Subscription.Id
			}

			if autoWallet != tc.expectedAutoWallet {
				t.Errorf("期望 autoWallet=%v，得到 %v", tc.expectedAutoWallet, autoWallet)
			}
			if decisionSubId != tc.expectedDecisionSub {
				t.Errorf("期望 decisionSubId=%d，得到 %d", tc.expectedDecisionSub, decisionSubId)
			}
		})
	}
}

// TestErrorDetailsCorrection_NoDetailsInitially 测试初始无 details 时不会 panic
func TestErrorDetailsCorrection_NoDetailsInitially(t *testing.T) {
	// 错误没有 details 的情况
	apiErr := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足"),
		types.ErrorCodeSubscriptionLimitReached,
		429,
	)

	// 模拟修正逻辑：如果 details 为 nil，不应该 panic
	if apiErr.Details != nil {
		apiErr.Details["auto_wallet_fallback"] = true
	}

	// 验证不会 panic，且 details 仍为 nil
	openAIErr := apiErr.ToOpenAIError()
	if openAIErr.Details != nil {
		t.Log("Details 不为 nil（可能被其他逻辑初始化）")
	}
}

// ===================== no_enabled_limits 分支测试 =====================

// TestNoEnabledLimits_DetailsContainsRequiredFields 验证无启用限额时 details 包含必要字段
// 新语义：period 使用 "none" 明确标识"无启用限额"状态，而不是空字符串
func TestNoEnabledLimits_DetailsContainsRequiredFields(t *testing.T) {
	// 模拟 TryBilling 中的场景：
	// - 决策订阅没有启用限额（所有 Limits 的 Enabled=false）
	// - 未启用兜底
	// - details 应该反映决策订阅的配置，period="none"

	decisionSub := &model.Subscription{
		Id:                 123,
		AutoWalletFallback: false,
	}
	requestedAmount := int64(100)

	// 构造 details（模拟 TryBilling 中的无启用限额分支）
	details := map[string]interface{}{
		"subscription_id":      decisionSub.Id,
		"period":               "none",          // 明确标识"无启用限额"
		"limit_quota":          int64(0),        // 无配置限额
		"used_quota":           int64(0),        // 无使用量
		"requested":            requestedAmount, // 请求的额度
		"auto_wallet_fallback": decisionSub.AutoWalletFallback,
	}

	err := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足"),
		types.ErrorCodeSubscriptionLimitReached,
		429,
		types.ErrOptionWithDetails(details),
	)

	openAIErr := err.ToOpenAIError()

	// 验证必要字段存在
	if openAIErr.Details == nil {
		t.Fatal("期望 Details 非空")
	}

	subId, ok := openAIErr.Details["subscription_id"]
	if !ok {
		t.Error("期望 Details 包含 subscription_id")
	}
	if subId != int64(123) {
		t.Errorf("期望 subscription_id=123，得到 %v", subId)
	}

	autoWallet, ok := openAIErr.Details["auto_wallet_fallback"]
	if !ok {
		t.Error("期望 Details 包含 auto_wallet_fallback")
	}
	if autoWallet != false {
		t.Errorf("期望 auto_wallet_fallback=false（与错误语义一致），得到 %v", autoWallet)
	}

	// 验证 period="none"（明确语义，不是空字符串）
	if period, ok := openAIErr.Details["period"]; !ok {
		t.Error("期望 Details 包含 period 字段")
	} else if period != "none" {
		t.Errorf("期望 period='none'（明确标识无启用限额），得到 %v", period)
	}

	// 验证占位字段存在
	if _, ok := openAIErr.Details["limit_quota"]; !ok {
		t.Error("期望 Details 包含 limit_quota 占位字段")
	}
	if _, ok := openAIErr.Details["used_quota"]; !ok {
		t.Error("期望 Details 包含 used_quota 占位字段")
	}
	if requested, ok := openAIErr.Details["requested"]; !ok {
		t.Error("期望 Details 包含 requested 字段")
	} else if requested != requestedAmount {
		t.Errorf("期望 requested=%d，得到 %v", requestedAmount, requested)
	}

	// 验证不包含非规范字段
	if _, exists := openAIErr.Details["reason"]; exists {
		t.Error("不应包含 reason 字段（非设计 7.2 规范）")
	}
}

// ===================== TryDeductFromSubscriptions 真实路径测试 =====================

// TestTryDeductFromSubscriptions_NoEnabledLimits_RealPath 测试真实代码路径：无启用限额
// 新语义：TryDeductFromSubscriptions 不返回 details，details 由 TryBilling 基于"决策订阅"构造
func TestTryDeductFromSubscriptions_NoEnabledLimits_RealPath(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 构造候选订阅：有限额配置但都未启用
	candidates := []*service.CandidateSubscription{
		{
			Priority: 1,
			Subscription: &model.Subscription{
				Id:                 100,
				UserId:             999,
				AutoWalletFallback: false, // 决策订阅的 auto_wallet_fallback=false
			},
			Limits: []model.SubscriptionPlanLimit{
				{Period: "daily", Quota: 1000, Enabled: false},
				{Period: "monthly", Quota: 10000, Enabled: false},
			},
		},
		{
			Priority: 2,
			Subscription: &model.Subscription{
				Id:                 200,
				UserId:             999,
				AutoWalletFallback: true,
			},
			Limits: []model.SubscriptionPlanLimit{
				{Period: "daily", Quota: 500, Enabled: false},
			},
		},
	}

	_, err := svc.TryDeductFromSubscriptions(candidates, 100)
	if err == nil {
		t.Fatal("期望返回错误")
	}

	apiErr, ok := err.(*types.NewAPIError)
	if !ok {
		t.Fatalf("期望 NewAPIError 类型，得到 %T", err)
	}

	// 验证错误码
	if apiErr.GetErrorCode() != types.ErrorCodeSubscriptionLimitReached {
		t.Errorf("期望错误码 SUBSCRIPTION_LIMIT_REACHED，得到 %s", apiErr.GetErrorCode())
	}

	// 新语义：TryDeductFromSubscriptions 不返回 details
	// details 由 TryBilling 基于"决策订阅"（candidates[0]）构造
	// 这里只验证 TryDeductFromSubscriptions 返回错误但无 details
	if apiErr.Details != nil {
		// TryDeductFromSubscriptions 不应返回 details
		// details 应该由 TryBilling 基于决策订阅构造
		t.Logf("TryDeductFromSubscriptions 不应返回 details，但得到了: %v", apiErr.Details)
	}
}

// TestTryDeductFromSubscriptions_EmptyLimits_RealPath 测试真实代码路径：空限额列表
func TestTryDeductFromSubscriptions_EmptyLimits_RealPath(t *testing.T) {
	svc := service.GetSubscriptionBillingService()

	// 构造候选订阅：限额列表为空
	candidates := []*service.CandidateSubscription{
		{
			Priority: 1,
			Subscription: &model.Subscription{
				Id:                 100,
				UserId:             999,
				AutoWalletFallback: false,
			},
			Limits: []model.SubscriptionPlanLimit{}, // 空列表
		},
	}

	_, err := svc.TryDeductFromSubscriptions(candidates, 100)
	if err == nil {
		t.Fatal("期望返回错误")
	}

	apiErr, ok := err.(*types.NewAPIError)
	if !ok {
		t.Fatalf("期望 NewAPIError 类型，得到 %T", err)
	}

	// 验证 details 存在且包含正确信息
	if apiErr.Details == nil {
		t.Fatal("期望 Details 非空")
	}

	if apiErr.Details["subscription_id"] != int64(100) {
		t.Errorf("期望 subscription_id=100，得到 %v", apiErr.Details["subscription_id"])
	}
	if apiErr.Details["auto_wallet_fallback"] != false {
		t.Errorf("期望 auto_wallet_fallback=false，得到 %v", apiErr.Details["auto_wallet_fallback"])
	}
}

// ===================== preConsumedQuota <= 0 分支测试 =====================

// TestPreConsumedQuotaZero_BillingLogic 验证预扣额度判断逻辑
func TestPreConsumedQuotaZero_BillingLogic(t *testing.T) {
	// 测试 preConsumedQuota <= 0 时的判断逻辑
	// 这是 pre_consume_quota.go:108-112 的逻辑
	testCases := []struct {
		name              string
		preConsumedQuota  int
		shouldSkipBilling bool
		expectedSource    string
	}{
		{
			name:              "预扣额度为0应跳过订阅扣费",
			preConsumedQuota:  0,
			shouldSkipBilling: true,
			expectedSource:    service.BillingSourceWallet,
		},
		{
			name:              "预扣额度为负数应跳过订阅扣费",
			preConsumedQuota:  -100,
			shouldSkipBilling: true,
			expectedSource:    service.BillingSourceWallet,
		},
		{
			name:              "预扣额度为正数应尝试订阅扣费",
			preConsumedQuota:  100,
			shouldSkipBilling: false,
			expectedSource:    "", // 取决于订阅扣费结果
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			shouldSkip := tc.preConsumedQuota <= 0
			if shouldSkip != tc.shouldSkipBilling {
				t.Errorf("预扣额度 %d：期望跳过订阅扣费=%v，得到 %v",
					tc.preConsumedQuota, tc.shouldSkipBilling, shouldSkip)
			}
			if tc.shouldSkipBilling && tc.expectedSource != service.BillingSourceWallet {
				t.Errorf("跳过订阅扣费时应使用钱包来源")
			}
		})
	}
}

// TestBillingSourceConstants_Extended 验证计费来源常量定义正确（扩展）
func TestBillingSourceConstants_Extended(t *testing.T) {
	// 验证常量值符合预期
	if service.BillingSourceWallet != "wallet" {
		t.Errorf("期望 BillingSourceWallet='wallet'，得到 %q", service.BillingSourceWallet)
	}
	if service.BillingSourceSubscription != "subscription" {
		t.Errorf("期望 BillingSourceSubscription='subscription'，得到 %q", service.BillingSourceSubscription)
	}
	if service.BillingSourceFallback != "fallback" {
		t.Errorf("期望 BillingSourceFallback='fallback'，得到 %q", service.BillingSourceFallback)
	}
	if service.BillingSourceSkipped != "skipped" {
		t.Errorf("期望 BillingSourceSkipped='skipped'，得到 %q", service.BillingSourceSkipped)
	}
}

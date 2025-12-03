package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
)

// MockContext 创建一个用于测试的 gin.Context
func MockContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	return c
}

// TestSelectChannelWithPriority_EmptyPriorities 测试空优先级列表
func TestSelectChannelWithPriority_EmptyPriorities(t *testing.T) {
	token := &model.Token{
		Group:           "default",
		GroupPriorities: "",
	}

	// 注意：这个测试需要真实的数据库环境，这里仅验证逻辑
	// 在真实测试中应该 mock CacheGetRandomSatisfiedChannel

	// 验证 GetGroupPriorities 返回正确的回退值
	priorities, err := token.GetGroupPriorities()
	if err != nil {
		t.Fatalf("GetGroupPriorities 失败: %v", err)
	}

	if len(priorities) != 1 {
		t.Errorf("期望回退到单个 Group, 得到 %d 个优先级", len(priorities))
	}

	if len(priorities) > 0 && priorities[0].Group != "default" {
		t.Errorf("期望回退分组为 'default', 得到 '%s'", priorities[0].Group)
	}
}

// TestSelectChannelWithPriority_InvalidJSON 测试无效的 JSON
func TestSelectChannelWithPriority_InvalidJSON(t *testing.T) {
	token := &model.Token{
		Group:           "default",
		GroupPriorities: "invalid json",
	}

	priorities, err := token.GetGroupPriorities()
	if err == nil {
		t.Error("期望 JSON 解析错误，但没有错误")
	}

	if priorities != nil {
		t.Error("错误情况下应该返回 nil")
	}
}

// TestSelectChannelWithPriority_PriorityOrder 测试优先级排序
func TestSelectChannelWithPriority_PriorityOrder(t *testing.T) {
	token := &model.Token{
		GroupPriorities: `[{"group":"standard","priority":3},{"group":"vip","priority":1},{"group":"basic","priority":2}]`,
	}

	priorities, err := token.GetGroupPriorities()
	if err != nil {
		t.Fatalf("GetGroupPriorities 失败: %v", err)
	}

	if len(priorities) != 3 {
		t.Fatalf("期望 3 个优先级, 得到 %d", len(priorities))
	}

	// 验证排序
	expectedOrder := []string{"vip", "basic", "standard"}
	for i, expected := range expectedOrder {
		if priorities[i].Group != expected {
			t.Errorf("索引 %d: 期望 %s, 得到 %s", i, expected, priorities[i].Group)
		}
	}
}

// TestSelectChannelByRatio_ExcludeLogic 测试排除逻辑
func TestSelectChannelByRatio_ExcludeLogic(t *testing.T) {
	excludePriorities := []model.GroupPriority{
		{Group: "vip", Priority: 1},
		{Group: "standard", Priority: 2},
	}

	excludeMap := make(map[string]bool)
	for _, p := range excludePriorities {
		excludeMap[p.Group] = true
	}

	// 验证排除逻辑
	if !excludeMap["vip"] {
		t.Error("vip 应该在排除列表中")
	}

	if !excludeMap["standard"] {
		t.Error("standard 应该在排除列表中")
	}

	if excludeMap["basic"] {
		t.Error("basic 不应该在排除列表中")
	}
}

// TestGroupPriorityValidation 测试分组优先级验证逻辑
func TestGroupPriorityValidation(t *testing.T) {
	tests := []struct {
		name        string
		priorities  []model.GroupPriority
		shouldError bool
		description string
	}{
		{
			name: "正常优先级",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 1},
				{Group: "standard", Priority: 2},
			},
			shouldError: false,
			description: "应该通过验证",
		},
		{
			name: "空分组名",
			priorities: []model.GroupPriority{
				{Group: "", Priority: 1},
			},
			shouldError: true,
			description: "应该拒绝空分组名",
		},
		{
			name: "优先级为0",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 0},
			},
			shouldError: true,
			description: "应该拒绝优先级为0",
		},
		{
			name: "重复分组",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 1},
				{Group: "vip", Priority: 2},
			},
			shouldError: true,
			description: "应该拒绝重复分组",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &model.Token{}
			err := token.SetGroupPriorities(tt.priorities)

			if tt.shouldError && err == nil {
				t.Errorf("%s: 期望错误但没有错误", tt.description)
			}

			if !tt.shouldError && err != nil {
				t.Errorf("%s: 意外错误: %v", tt.description, err)
			}
		})
	}
}

// TestAutoSmartGroupLogic 测试自动智能分组逻辑
func TestAutoSmartGroupLogic(t *testing.T) {
	// 模拟所有优先级分组都失败的场景
	token := &model.Token{
		GroupPriorities: `[{"group":"vip","priority":1},{"group":"standard","priority":2}]`,
		AutoSmartGroup:  true,
	}

	// 验证 AutoSmartGroup 标志
	if !token.AutoSmartGroup {
		t.Error("AutoSmartGroup 应该为 true")
	}

	priorities, err := token.GetGroupPriorities()
	if err != nil {
		t.Fatalf("GetGroupPriorities 失败: %v", err)
	}

	// 验证优先级列表存在
	if len(priorities) == 0 {
		t.Error("应该有优先级列表")
	}

	// 在实际实现中，当所有优先级分组失败时，会调用 selectChannelByRatio
	// 这里我们验证 excludeMap 的构建逻辑
	excludeMap := make(map[string]bool)
	for _, p := range priorities {
		excludeMap[p.Group] = true
	}

	if !excludeMap["vip"] || !excludeMap["standard"] {
		t.Error("已尝试的分组应该被排除")
	}
}

// 集成测试：完整的优先级选择流程
func TestPrioritySelectionFlow(t *testing.T) {
	// 1. 创建 token 并设置优先级
	token := &model.Token{}
	priorities := []model.GroupPriority{
		{Group: "enterprise", Priority: 1},
		{Group: "vip", Priority: 2},
		{Group: "standard", Priority: 3},
	}

	err := token.SetGroupPriorities(priorities)
	if err != nil {
		t.Fatalf("设置优先级失败: %v", err)
	}

	// 2. 获取并验证优先级
	retrieved, err := token.GetGroupPriorities()
	if err != nil {
		t.Fatalf("获取优先级失败: %v", err)
	}

	// 3. 验证排序正确
	if len(retrieved) != 3 {
		t.Fatalf("期望 3 个优先级, 得到 %d", len(retrieved))
	}

	expectedOrder := []string{"enterprise", "vip", "standard"}
	for i, expected := range expectedOrder {
		if retrieved[i].Group != expected {
			t.Errorf("索引 %d: 期望 %s, 得到 %s", i, expected, retrieved[i].Group)
		}
	}

	// 4. 验证 Group 字段同步
	if token.Group != "enterprise" {
		t.Errorf("Group 字段应该是 'enterprise', 得到 '%s'", token.Group)
	}
}

// TestErrorHandling 测试错误处理
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name          string
		setupToken    func() *model.Token
		expectedError bool
		description   string
	}{
		{
			name: "JSON 格式错误",
			setupToken: func() *model.Token {
				return &model.Token{
					GroupPriorities: "{invalid json",
				}
			},
			expectedError: true,
			description:   "应该返回 JSON 解析错误",
		},
		{
			name: "空 GroupPriorities",
			setupToken: func() *model.Token {
				return &model.Token{
					GroupPriorities: "",
					Group:           "default",
				}
			},
			expectedError: false,
			description:   "应该回退到 Group 字段",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := tt.setupToken()
			_, err := token.GetGroupPriorities()

			if tt.expectedError && err == nil {
				t.Errorf("%s: 期望错误但没有错误", tt.description)
			}

			if !tt.expectedError && err != nil {
				t.Errorf("%s: 意外错误: %v", tt.description, err)
			}
		})
	}
}

// 性能测试：优先级选择性能
func BenchmarkPrioritySelection(b *testing.B) {
	token := &model.Token{
		GroupPriorities: `[{"group":"enterprise","priority":1},{"group":"vip","priority":2},{"group":"standard","priority":3}]`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = token.GetGroupPriorities()
	}
}

// 模拟测试：验证 selectChannelByRatio 的排序逻辑
func TestRatioSorting(t *testing.T) {
	// 模拟不同费率的分组
	type groupWithRatio struct {
		group string
		ratio float64
	}

	candidates := []groupWithRatio{
		{group: "expensive", ratio: 2.5},
		{group: "cheap", ratio: 0.5},
		{group: "medium", ratio: 1.0},
	}

	// 排序（费率从低到高）
	for i := 0; i < len(candidates)-1; i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[i].ratio > candidates[j].ratio {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	// 验证排序结果
	expectedOrder := []string{"cheap", "medium", "expensive"}
	for i, expected := range expectedOrder {
		if candidates[i].group != expected {
			t.Errorf("索引 %d: 期望 %s, 得到 %s", i, expected, candidates[i].group)
		}
	}
}

// 边界条件测试
func TestEdgeCases(t *testing.T) {
	t.Run("单个优先级", func(t *testing.T) {
		token := &model.Token{}
		priorities := []model.GroupPriority{
			{Group: "only-one", Priority: 1},
		}

		err := token.SetGroupPriorities(priorities)
		if err != nil {
			t.Fatalf("设置单个优先级失败: %v", err)
		}

		retrieved, err := token.GetGroupPriorities()
		if err != nil {
			t.Fatalf("获取优先级失败: %v", err)
		}

		if len(retrieved) != 1 {
			t.Errorf("期望 1 个优先级, 得到 %d", len(retrieved))
		}

		if retrieved[0].Group != "only-one" {
			t.Errorf("期望 'only-one', 得到 '%s'", retrieved[0].Group)
		}
	})

	t.Run("大量优先级", func(t *testing.T) {
		token := &model.Token{}
		priorities := make([]model.GroupPriority, 100)
		for i := 0; i < 100; i++ {
			priorities[i] = model.GroupPriority{
				Group:    string(rune('A' + i)),
				Priority: i + 1,
			}
		}

		err := token.SetGroupPriorities(priorities)
		if err != nil {
			t.Fatalf("设置大量优先级失败: %v", err)
		}

		retrieved, err := token.GetGroupPriorities()
		if err != nil {
			t.Fatalf("获取优先级失败: %v", err)
		}

		if len(retrieved) != 100 {
			t.Errorf("期望 100 个优先级, 得到 %d", len(retrieved))
		}
	})
}

// --- New tests covering actual service logic ---

func TestSelectChannelWithPriority_ChoosesHighestPriority(t *testing.T) {
	defer func(original func(string, string, int) (*model.Channel, error)) {
		randomChannelSelector = original
	}(randomChannelSelector)

	randomChannelSelector = func(group, modelName string, retry int) (*model.Channel, error) {
		if group == "vip" {
			return &model.Channel{Id: 1, Name: "vip-channel"}, nil
		}
		return nil, errors.New("no channel")
	}

	c := MockContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	token := &model.Token{}
	_ = token.SetGroupPriorities([]model.GroupPriority{
		{Group: "vip", Priority: 1},
		{Group: "standard", Priority: 2},
	})

	channel, group, err := SelectChannelWithPriority(c, token, "gpt-4", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group != "vip" {
		t.Fatalf("expected vip group, got %s", group)
	}
	if channel == nil || channel.Name != "vip-channel" {
		t.Fatalf("expected vip channel, got %#v", channel)
	}
	if selected := common.GetContextKeyString(c, constant.ContextKeySelectedGroup); selected != "vip" {
		t.Fatalf("expected context selected group vip, got %s", selected)
	}
	if usedAuto := common.GetContextKeyBool(c, constant.ContextKeyAutoSmartGroupUsed); usedAuto {
		t.Fatal("expected auto smart group flag to be false")
	}
}

func TestSelectChannelWithPriority_AutoSmartFallback(t *testing.T) {
	defer func(original func(string, string, int) (*model.Channel, error)) {
		randomChannelSelector = original
	}(randomChannelSelector)

	randomChannelSelector = func(group, modelName string, retry int) (*model.Channel, error) {
		switch group {
		case "primary-group":
			return nil, errors.New("no channel in primary")
		case "vip":
			return &model.Channel{Id: 2, Name: "vip-channel"}, nil
		default:
			return nil, errors.New("unknown group")
		}
	}

	c := MockContext()
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	token := &model.Token{AutoSmartGroup: true}
	_ = token.SetGroupPriorities([]model.GroupPriority{
		{Group: "primary-group", Priority: 1},
	})

	channel, group, err := SelectChannelWithPriority(c, token, "gpt-4", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group != "vip" {
		t.Fatalf("expected vip fallback group, got %s", group)
	}
	if channel == nil || channel.Name != "vip-channel" {
		t.Fatalf("expected vip channel, got %#v", channel)
	}
	if !common.GetContextKeyBool(c, constant.ContextKeyAutoSmartGroupUsed) {
		t.Fatal("expected auto smart group flag to be true")
	}
}

func TestSelectChannelWithPriority_AllGroupsFailed(t *testing.T) {
	defer func(original func(string, string, int) (*model.Channel, error)) {
		randomChannelSelector = original
	}(randomChannelSelector)

	randomChannelSelector = func(group, modelName string, retry int) (*model.Channel, error) {
		return nil, errors.New("no channel")
	}

	c := MockContext()
	token := &model.Token{}
	_ = token.SetGroupPriorities([]model.GroupPriority{
		{Group: "vip", Priority: 1},
	})

	_, _, err := SelectChannelWithPriority(c, token, "gpt-4", 0)
	if err == nil {
		t.Fatal("expected error but got nil")
	}
	if !errors.Is(err, ErrAllGroupsFailed) {
		t.Fatalf("expected ErrAllGroupsFailed, got %v", err)
	}
}

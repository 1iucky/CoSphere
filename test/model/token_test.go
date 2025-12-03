package model_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/model"
)

// TestGetGroupPriorities 测试获取分组优先级
func TestGetGroupPriorities(t *testing.T) {
	tests := []struct {
		name          string
		token         model.Token
		expectedLen   int
		expectedFirst string
		expectedError bool
		description   string
	}{
		{
			name: "正常的优先级列表",
			token: model.Token{
				GroupPriorities: `[{"group":"vip","priority":1},{"group":"standard","priority":2}]`,
			},
			expectedLen:   2,
			expectedFirst: "vip",
			expectedError: false,
			description:   "应该正确解析并按优先级排序",
		},
		{
			name: "优先级乱序",
			token: model.Token{
				GroupPriorities: `[{"group":"standard","priority":3},{"group":"vip","priority":1},{"group":"basic","priority":2}]`,
			},
			expectedLen:   3,
			expectedFirst: "vip",
			expectedError: false,
			description:   "应该按优先级从低到高排序",
		},
		{
			name: "空的优先级列表",
			token: model.Token{
				GroupPriorities: "",
				Group:           "default",
			},
			expectedLen:   1,
			expectedFirst: "default",
			expectedError: false,
			description:   "应该回退到 Group 字段",
		},
		{
			name: "空优先级且空Group",
			token: model.Token{
				GroupPriorities: "",
				Group:           "",
			},
			expectedLen:   0,
			expectedError: false,
			description:   "应该返回空列表",
		},
		{
			name: "无效的JSON",
			token: model.Token{
				GroupPriorities: `invalid json`,
			},
			expectedError: true,
			description:   "应该返回错误",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priorities, err := tt.token.GetGroupPriorities()

			if tt.expectedError {
				if err == nil {
					t.Errorf("%s: 期望错误但没有错误", tt.description)
				}
				return
			}

			if err != nil {
				t.Errorf("%s: 意外错误: %v", tt.description, err)
				return
			}

			if len(priorities) != tt.expectedLen {
				t.Errorf("%s: 长度不匹配, 期望 %d, 得到 %d", tt.description, tt.expectedLen, len(priorities))
				return
			}

			if tt.expectedLen > 0 && priorities[0].Group != tt.expectedFirst {
				t.Errorf("%s: 第一个分组不匹配, 期望 %s, 得到 %s", tt.description, tt.expectedFirst, priorities[0].Group)
			}
		})
	}
}

// TestSetGroupPriorities 测试设置分组优先级
func TestSetGroupPriorities(t *testing.T) {
	tests := []struct {
		name          string
		priorities    []model.GroupPriority
		expectedError bool
		expectedGroup string
		description   string
	}{
		{
			name: "正常设置优先级",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 1},
				{Group: "standard", Priority: 2},
			},
			expectedError: false,
			expectedGroup: "vip",
			description:   "应该成功设置并同步 Group 字段",
		},
		{
			name: "优先级乱序",
			priorities: []model.GroupPriority{
				{Group: "standard", Priority: 3},
				{Group: "vip", Priority: 1},
				{Group: "basic", Priority: 2},
			},
			expectedError: false,
			expectedGroup: "vip",
			description:   "应该自动排序并设置最高优先级分组到 Group 字段",
		},
		{
			name:          "空列表",
			priorities:    []model.GroupPriority{},
			expectedError: false,
			expectedGroup: "",
			description:   "应该清空 GroupPriorities",
		},
		{
			name: "分组名称为空",
			priorities: []model.GroupPriority{
				{Group: "", Priority: 1},
			},
			expectedError: true,
			description:   "应该返回错误",
		},
		{
			name: "优先级小于1",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 0},
			},
			expectedError: true,
			description:   "应该返回错误",
		},
		{
			name: "重复的分组",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 1},
				{Group: "vip", Priority: 2},
			},
			expectedError: true,
			description:   "应该返回错误",
		},
		{
			name: "超过限制数量",
			priorities: func() []model.GroupPriority {
				items := make([]model.GroupPriority, model.MaxGroupPriorities+1)
				for i := range items {
					items[i] = model.GroupPriority{Group: fmt.Sprintf("group-%d", i), Priority: i + 1}
				}
				return items
			}(),
			expectedError: true,
			description:   "超过最大分组数量应该失败",
		},
		{
			name: "自动修剪分组名称",
			priorities: []model.GroupPriority{
				{Group: " vip ", Priority: 2},
				{Group: " basic ", Priority: 1},
			},
			expectedError: false,
			expectedGroup: "basic",
			description:   "应该修剪并重新排序优先级",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := &model.Token{}
			err := token.SetGroupPriorities(tt.priorities)

			if tt.expectedError {
				if err == nil {
					t.Errorf("%s: 期望错误但没有错误", tt.description)
				}
				return
			}

			if err != nil {
				t.Errorf("%s: 意外错误: %v", tt.description, err)
				return
			}

			// 验证 GroupPriorities JSON
			if len(tt.priorities) == 0 {
				if token.GroupPriorities != "" {
					t.Errorf("%s: 期望空字符串, 得到 %s", tt.description, token.GroupPriorities)
				}
			} else {
				var parsed []model.GroupPriority
				err := json.Unmarshal([]byte(token.GroupPriorities), &parsed)
				if err != nil {
					t.Errorf("%s: 无法解析 JSON: %v", tt.description, err)
					return
				}

				if len(parsed) != len(tt.priorities) {
					t.Errorf("%s: 长度不匹配, 期望 %d, 得到 %d", tt.description, len(tt.priorities), len(parsed))
				}
			}

			// 验证 Group 字段同步
			if token.Group != tt.expectedGroup {
				t.Errorf("%s: Group 字段不匹配, 期望 %s, 得到 %s", tt.description, tt.expectedGroup, token.Group)
			}
		})
	}
}

// TestGroupPrioritiesRoundTrip 测试完整的设置和获取流程
func TestGroupPrioritiesRoundTrip(t *testing.T) {
	token := &model.Token{}
	original := []model.GroupPriority{
		{Group: "enterprise", Priority: 1},
		{Group: "vip", Priority: 2},
		{Group: "standard", Priority: 3},
	}

	// 设置优先级
	err := token.SetGroupPriorities(original)
	if err != nil {
		t.Fatalf("设置优先级失败: %v", err)
	}

	// 获取优先级
	retrieved, err := token.GetGroupPriorities()
	if err != nil {
		t.Fatalf("获取优先级失败: %v", err)
	}

	// 验证长度
	if len(retrieved) != len(original) {
		t.Fatalf("长度不匹配, 期望 %d, 得到 %d", len(original), len(retrieved))
	}

	// 验证内容和排序
	for i, expected := range original {
		if retrieved[i].Group != expected.Group {
			t.Errorf("索引 %d: 分组不匹配, 期望 %s, 得到 %s", i, expected.Group, retrieved[i].Group)
		}
		if retrieved[i].Priority != expected.Priority {
			t.Errorf("索引 %d: 优先级不匹配, 期望 %d, 得到 %d", i, expected.Priority, retrieved[i].Priority)
		}
	}

	// 验证 Group 字段
	if token.Group != "enterprise" {
		t.Errorf("Group 字段应该是最高优先级分组, 期望 enterprise, 得到 %s", token.Group)
	}
}

// BenchmarkGetGroupPriorities 性能测试
func BenchmarkGetGroupPriorities(b *testing.B) {
	token := &model.Token{
		GroupPriorities: `[{"group":"vip","priority":1},{"group":"standard","priority":2},{"group":"basic","priority":3}]`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = token.GetGroupPriorities()
	}
}

// BenchmarkSetGroupPriorities 性能测试
func BenchmarkSetGroupPriorities(b *testing.B) {
	priorities := []model.GroupPriority{
		{Group: "vip", Priority: 1},
		{Group: "standard", Priority: 2},
		{Group: "basic", Priority: 3},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		token := &model.Token{}
		_ = token.SetGroupPriorities(priorities)
	}
}

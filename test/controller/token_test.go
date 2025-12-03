package controller_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// setupTestRouter 创建测试用的 Gin 路由
func setupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	return r
}

// mockUserContext 模拟用户上下文
func mockUserContext(c *gin.Context, userId int) {
	c.Set("id", userId)
}

// TestTokenRequest_JSONParsing 测试 TokenRequest JSON 解析
func TestTokenRequest_JSONParsing(t *testing.T) {
	tests := []struct {
		name        string
		jsonData    string
		expectError bool
		description string
	}{
		{
			name: "正常的分组优先级数组",
			jsonData: `{
				"name": "test-token",
				"group": "default",
				"group_priorities_array": [
					{"group": "vip", "priority": 1},
					{"group": "standard", "priority": 2}
				],
				"auto_smart_group": true
			}`,
			expectError: false,
			description: "应该成功解析 group_priorities_array",
		},
		{
			name: "空的分组优先级数组",
			jsonData: `{
				"name": "test-token",
				"group": "default",
				"group_priorities_array": []
			}`,
			expectError: false,
			description: "应该接受空数组",
		},
		{
			name: "没有分组优先级字段",
			jsonData: `{
				"name": "test-token",
				"group": "default"
			}`,
			expectError: false,
			description: "应该接受没有 group_priorities_array 的请求（向后兼容）",
		},
		{
			name: "无效的 JSON",
			jsonData: `{
				"name": "test-token",
				"group_priorities_array": invalid
			}`,
			expectError: true,
			description: "应该拒绝无效 JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tokenReq controller.TokenRequest
			err := json.Unmarshal([]byte(tt.jsonData), &tokenReq)

			if tt.expectError && err == nil {
				t.Errorf("%s: 期望错误但没有错误", tt.description)
			}

			if !tt.expectError && err != nil {
				t.Errorf("%s: 意外错误: %v", tt.description, err)
			}

			if !tt.expectError && err == nil {
				// 验证解析结果
				if tt.name == "正常的分组优先级数组" {
					if len(tokenReq.GroupPrioritiesArray) != 2 {
						t.Errorf("期望 2 个优先级, 得到 %d", len(tokenReq.GroupPrioritiesArray))
					}
					if tokenReq.AutoSmartGroup != true {
						t.Errorf("期望 AutoSmartGroup 为 true, 得到 %v", tokenReq.AutoSmartGroup)
					}
				}
			}
		})
	}
}

// TestAddToken_RequestValidation 测试 AddToken 请求验证逻辑
func TestAddToken_RequestValidation(t *testing.T) {
	t.Run("令牌名称过长", func(t *testing.T) {
		router := setupTestRouter()
		router.POST("/api/token", func(c *gin.Context) {
			mockUserContext(c, 1)
			controller.AddToken(c)
		})

		tokenReq := controller.TokenRequest{
			Token: model.Token{
				Name: "这是一个非常非常非常非常非常非常长的令牌名称超过三十个字符",
			},
		}

		jsonData, _ := json.Marshal(tokenReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/token", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("期望状态码 %d, 得到 %d", http.StatusOK, w.Code)
		}

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		if err != nil {
			t.Fatalf("解析响应失败: %v", err)
		}

		success, _ := response["success"].(bool)
		if success {
			t.Errorf("期望 success=false, 得到 success=true")
		}

		message, ok := response["message"].(string)
		if !ok || message != "令牌名称过长" {
			t.Errorf("期望错误消息 '令牌名称过长', 得到 '%s'", message)
		}
	})

	// 注意：其他需要数据库的测试跳过，应该在集成测试中完成
	t.Run("正常令牌创建（需要数据库）", func(t *testing.T) {
		t.Skip("需要数据库环境，跳过单元测试")
	})
}

// TestGroupPrioritiesValidation 测试分组优先级验证逻辑
func TestGroupPrioritiesValidation(t *testing.T) {
	// 只测试验证会失败的情况（这些不需要数据库）
	failureCases := []struct {
		name          string
		priorities    []model.GroupPriority
		expectedError string
		description   string
	}{
		{
			name: "空分组名",
			priorities: []model.GroupPriority{
				{Group: "", Priority: 1},
			},
			expectedError: "分组优先级设置失败",
			description:   "应该拒绝空分组名",
		},
		{
			name: "优先级为0",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 0},
			},
			expectedError: "分组优先级设置失败",
			description:   "应该拒绝优先级为0",
		},
		{
			name: "重复分组",
			priorities: []model.GroupPriority{
				{Group: "vip", Priority: 1},
				{Group: "vip", Priority: 2},
			},
			expectedError: "分组优先级设置失败",
			description:   "应该拒绝重复分组",
		},
	}

	for _, tt := range failureCases {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.POST("/api/token", func(c *gin.Context) {
				mockUserContext(c, 1)
				controller.AddToken(c)
			})

			tokenReq := controller.TokenRequest{
				Token: model.Token{
					Name: "test-token",
				},
				GroupPrioritiesArray: tt.priorities,
			}

			jsonData, _ := json.Marshal(tokenReq)
			req, _ := http.NewRequest(http.MethodPost, "/api/token", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}

			success, _ := response["success"].(bool)
			if success {
				t.Errorf("%s: 期望验证失败, 但返回成功", tt.description)
			}

			message, ok := response["message"].(string)
			if !ok || message == "" {
				t.Errorf("%s: 期望错误消息, 但没有消息", tt.description)
			}
		})
	}

	// 成功的情况需要数据库
	t.Run("正常的优先级（需要数据库）", func(t *testing.T) {
		t.Skip("需要数据库环境，跳过单元测试")
	})

	t.Run("无权访问分组", func(t *testing.T) {
		router := setupTestRouter()
		router.POST("/api/token", func(c *gin.Context) {
			mockUserContext(c, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			controller.AddToken(c)
		})

		tokenReq := controller.TokenRequest{
			Token: model.Token{
				Name: "test-token",
			},
			GroupPrioritiesArray: []model.GroupPriority{{Group: "unknown-group", Priority: 1}},
		}

		jsonData, _ := json.Marshal(tokenReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/token", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var response map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &response)
		if success, _ := response["success"].(bool); success {
			t.Fatalf("expected failure due to unauthorized group, got success")
		}
		if message, _ := response["message"].(string); message == "" || message != "无权访问分组: unknown-group" {
			t.Fatalf("unexpected error message: %s", message)
		}
	})

	t.Run("单分组权限校验", func(t *testing.T) {
		router := setupTestRouter()
		router.POST("/api/token", func(c *gin.Context) {
			mockUserContext(c, 1)
			common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
			controller.AddToken(c)
		})

		tokenReq := controller.TokenRequest{
			Token: model.Token{
				Name:  "legacy-token",
				Group: "unknown-group",
			},
		}

		jsonData, _ := json.Marshal(tokenReq)
		req, _ := http.NewRequest(http.MethodPost, "/api/token", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		var response map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &response)
		if success, _ := response["success"].(bool); success {
			t.Fatalf("expected unauthorized error for single group, got success")
		}
		if message, _ := response["message"].(string); message == "" || message != "无权访问分组: unknown-group" {
			t.Fatalf("unexpected error message: %s", message)
		}
	})
}

// TestUpdateToken_GroupPriorities 测试更新分组优先级
func TestUpdateToken_GroupPriorities(t *testing.T) {
	// UpdateToken 需要数据库环境才能完整测试
	t.Run("更新分组优先级（需要数据库）", func(t *testing.T) {
		t.Skip("需要数据库环境，跳过单元测试")
	})
}

// TestBackwardCompatibility 测试向后兼容性
func TestBackwardCompatibility(t *testing.T) {
	t.Run("不带 group_priorities_array 的请求解析", func(t *testing.T) {
		// 只测试 JSON 解析，不涉及数据库
		tokenReq := controller.TokenRequest{
			Token: model.Token{
				Name:  "legacy-token",
				Group: "default",
			},
		}

		jsonData, err := json.Marshal(tokenReq)
		if err != nil {
			t.Fatalf("JSON 序列化失败: %v", err)
		}

		var parsed controller.TokenRequest
		err = json.Unmarshal(jsonData, &parsed)
		if err != nil {
			t.Fatalf("JSON 解析失败: %v", err)
		}

		if parsed.Name != "legacy-token" {
			t.Errorf("期望 Name='legacy-token', 得到 '%s'", parsed.Name)
		}

		if parsed.Group != "default" {
			t.Errorf("期望 Group='default', 得到 '%s'", parsed.Group)
		}

		// 验证没有 group_priorities_array 时不会出错
		if parsed.GroupPrioritiesArray != nil && len(parsed.GroupPrioritiesArray) > 0 {
			t.Errorf("期望 GroupPrioritiesArray 为空, 得到 %v", parsed.GroupPrioritiesArray)
		}
	})

	t.Run("旧版本请求完整流程（需要数据库）", func(t *testing.T) {
		t.Skip("需要数据库环境，跳过单元测试")
	})
}

// TestResponseFormat 测试响应格式
func TestResponseFormat(t *testing.T) {
	tests := []struct {
		name           string
		handler        gin.HandlerFunc
		method         string
		path           string
		body           interface{}
		expectedFields []string
		description    string
	}{
		{
			name:    "AddToken 响应格式",
			handler: func(c *gin.Context) { mockUserContext(c, 1); controller.AddToken(c) },
			method:  http.MethodPost,
			path:    "/api/token",
			body: controller.TokenRequest{
				Token: model.Token{
					Name: "这是一个超过三十个字符的非常长的令牌名称会被拒绝",
				},
			},
			expectedFields: []string{"success", "message"},
			description:    "应该返回标准的响应格式",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupTestRouter()
			router.Handle(tt.method, tt.path, tt.handler)

			jsonData, _ := json.Marshal(tt.body)
			req, _ := http.NewRequest(tt.method, tt.path, bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("解析响应失败: %v", err)
			}

			// 验证响应包含所有必需字段
			for _, field := range tt.expectedFields {
				if _, exists := response[field]; !exists {
					t.Errorf("%s: 响应缺少字段 '%s'", tt.description, field)
				}
			}
		})
	}
}

// 性能测试：JSON 序列化
func BenchmarkTokenRequestJSON(b *testing.B) {
	tokenReq := controller.TokenRequest{
		Token: model.Token{
			Name:  "benchmark-token",
			Group: "default",
		},
		GroupPrioritiesArray: []model.GroupPriority{
			{Group: "vip", Priority: 1},
			{Group: "standard", Priority: 2},
			{Group: "basic", Priority: 3},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(tokenReq)
	}
}

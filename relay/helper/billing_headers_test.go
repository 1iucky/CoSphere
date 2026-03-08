package helper

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetBillingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("设置完整的计费响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		SetBillingHeaders(c, "subscription", "")

		assert.Equal(t, "subscription", w.Header().Get("X-New-Api-Billing-Source"))
		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("设置跳过原因响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		SetBillingHeaders(c, "skipped", "subscription_preferred_disabled")

		assert.Equal(t, "skipped", w.Header().Get("X-New-Api-Billing-Source"))
		assert.Equal(t, "subscription_preferred_disabled", w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("空计费来源不设置响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		SetBillingHeaders(c, "", "")

		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Source"))
		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("防止重复设置响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 第一次设置
		SetBillingHeaders(c, "wallet", "")
		assert.Equal(t, "wallet", w.Header().Get("X-New-Api-Billing-Source"))

		// 第二次尝试设置（应被忽略）
		SetBillingHeaders(c, "subscription", "some_reason")
		// 响应头应保持第一次设置的值
		assert.Equal(t, "wallet", w.Header().Get("X-New-Api-Billing-Source"))
		// skip_reason 也不应被设置（因为标志已存在）
		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("设置 fallback 来源", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		SetBillingHeaders(c, "fallback", "")

		assert.Equal(t, "fallback", w.Header().Get("X-New-Api-Billing-Source"))
	})

	t.Run("仅设置 skip_reason 而不设置 source", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 这种情况在实际中不应发生，但代码应正确处理
		SetBillingHeaders(c, "", "subscription_preferred_disabled")

		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Source"))
		assert.Equal(t, "subscription_preferred_disabled", w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})
}

func TestSetBillingHeaders_WithStreamHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("流式响应时同时设置 EventStream 和 Billing 响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 先设置流式响应头
		SetEventStreamHeaders(c)
		// 再设置计费响应头
		SetBillingHeaders(c, "subscription", "")

		// 验证流式响应头
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
		assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

		// 验证计费响应头
		assert.Equal(t, "subscription", w.Header().Get("X-New-Api-Billing-Source"))
	})
}

// TestStreamingHandler_EndToEnd_BillingHeaders 端到端集成测试
// 模拟真实流式 handler 的完整路径，验证计费响应头在 SSE 响应中正确设置
func TestStreamingHandler_EndToEnd_BillingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("模拟 OpenAI 流式 handler 完整路径", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 1. 模拟 PreConsumeQuota 在 context 中存储 billing 信息
		c.Set(string(constant.ContextKeyBillingSource), "subscription")

		// 2. 模拟流式 handler 调用 SetEventStreamHeaders（会自动设置 billing headers）
		SetEventStreamHeaders(c)

		// 3. 模拟发送流式数据
		_ = StringData(c, `{"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}`)
		_ = StringData(c, `{"id":"chatcmpl-123","object":"chat.completion.chunk","choices":[{"delta":{"content":" World"}}]}`)
		Done(c)

		// 4. 验证响应头
		assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"),
			"Content-Type 应为 text/event-stream")
		assert.Equal(t, "subscription", w.Header().Get("X-New-Api-Billing-Source"),
			"流式响应应包含计费来源响应头")

		// 5. 验证响应体包含流式数据
		body := w.Body.String()
		assert.Contains(t, body, "data: ", "响应体应包含 SSE data 前缀")
		assert.Contains(t, body, "[DONE]", "响应体应包含结束标记")
	})

	t.Run("模拟 wallet fallback 流式响应", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 模拟订阅优先但降级到钱包的场景
		c.Set(string(constant.ContextKeyBillingSource), "fallback")
		c.Set(string(constant.ContextKeyBillingSkipReason), "subscription_quota_exceeded")

		SetEventStreamHeaders(c)
		_ = StringData(c, `{"content":"test"}`)
		Done(c)

		assert.Equal(t, "fallback", w.Header().Get("X-New-Api-Billing-Source"),
			"应返回 fallback 计费来源")
		assert.Equal(t, "subscription_quota_exceeded", w.Header().Get("X-New-Api-Billing-Skip-Reason"),
			"应返回跳过订阅的原因")
	})

	t.Run("模拟 skipped 流式响应", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 模拟订阅被跳过的场景
		c.Set(string(constant.ContextKeyBillingSource), "skipped")
		c.Set(string(constant.ContextKeyBillingSkipReason), "subscription_preferred_disabled")

		SetEventStreamHeaders(c)
		Done(c)

		assert.Equal(t, "skipped", w.Header().Get("X-New-Api-Billing-Source"))
		assert.Equal(t, "subscription_preferred_disabled", w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("无 billing 信息时不设置响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 不在 context 中设置任何 billing 信息
		SetEventStreamHeaders(c)
		Done(c)

		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Source"),
			"无 billing 信息时不应设置计费来源响应头")
		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Skip-Reason"),
			"无 billing 信息时不应设置跳过原因响应头")
	})
}

// TestStreamingHandler_Integration_WithGinRouter 使用 Gin 路由器的集成测试
// 模拟完整的 HTTP 请求-响应周期
func TestStreamingHandler_Integration_WithGinRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("通过 Gin 路由器验证流式响应头", func(t *testing.T) {
		router := gin.New()

		// 注册一个模拟的流式端点
		router.GET("/v1/chat/completions/stream", func(c *gin.Context) {
			// 模拟 PreConsumeQuota 设置 billing 信息
			c.Set(string(constant.ContextKeyBillingSource), "subscription")

			// 模拟流式 handler
			SetEventStreamHeaders(c)
			_ = StringData(c, `{"choices":[{"delta":{"content":"Hello"}}]}`)
			Done(c)
		})

		// 发送请求
		req := httptest.NewRequest("GET", "/v1/chat/completions/stream", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// 验证响应
		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
		assert.Equal(t, "subscription", resp.Header.Get("X-New-Api-Billing-Source"),
			"通过路由器的流式响应应包含计费响应头")

		// 验证响应体
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "data: ")
		assert.Contains(t, string(body), "[DONE]")
	})

	t.Run("通过 Gin 路由器验证 Claude 流式响应头", func(t *testing.T) {
		router := gin.New()

		router.POST("/v1/messages/stream", func(c *gin.Context) {
			c.Set(string(constant.ContextKeyBillingSource), "wallet")

			SetEventStreamHeaders(c)
			// 模拟 Claude 格式的流式响应
			_ = StringData(c, `{"type":"message_start"}`)
			Done(c)
		})

		req := httptest.NewRequest("POST", "/v1/messages/stream", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		resp := w.Result()
		defer resp.Body.Close()

		assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
		assert.Equal(t, "wallet", resp.Header.Get("X-New-Api-Billing-Source"))
	})
}

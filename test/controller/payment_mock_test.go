package controller_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/form"
)

// ===================== Epay Mock 配置 =====================

// EpayConfig 保存 Epay 原始配置
type EpayConfig struct {
	PayAddress string
	EpayId     string
	EpayKey    string
}

// SetupEpayMockConfig 设置 Epay mock 配置，返回恢复函数
func SetupEpayMockConfig() func() {
	// 保存原始配置
	original := EpayConfig{
		PayAddress: operation_setting.PayAddress,
		EpayId:     operation_setting.EpayId,
		EpayKey:    operation_setting.EpayKey,
	}

	// 设置 mock 配置
	// Epay 的 Purchase 方法只生成 URL 和参数，不实际发起 HTTP 请求
	// 所以我们只需要设置任意有效值即可
	operation_setting.PayAddress = "https://mock-epay.test"
	operation_setting.EpayId = "mock_epay_id"
	operation_setting.EpayKey = "mock_epay_key"

	// 返回恢复函数
	return func() {
		operation_setting.PayAddress = original.PayAddress
		operation_setting.EpayId = original.EpayId
		operation_setting.EpayKey = original.EpayKey
	}
}

// ===================== Stripe Mock Backend =====================

// MockStripeBackend 实现 stripe.Backend 接口用于测试
type MockStripeBackend struct {
	mu        sync.Mutex
	responses map[string]interface{}
}

// NewMockStripeBackend 创建新的 mock backend
func NewMockStripeBackend() *MockStripeBackend {
	return &MockStripeBackend{
		responses: make(map[string]interface{}),
	}
}

// Call 实现 Backend.Call 接口
func (m *MockStripeBackend) Call(method, path, key string, params stripe.ParamsContainer, v stripe.LastResponseSetter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 模拟 Checkout Session 创建响应
	if method == http.MethodPost && path == "/v1/checkout/sessions" {
		resp := &stripe.CheckoutSession{
			ID:  "cs_test_mock_session_id",
			URL: "https://checkout.stripe.com/mock/test_session",
		}
		// 将 mock 响应写入 v
		respBytes, _ := json.Marshal(resp)
		apiResp := &stripe.APIResponse{
			Header:     http.Header{},
			RawJSON:    respBytes,
			StatusCode: http.StatusOK,
		}
		v.SetLastResponse(apiResp)

		// 通过 JSON 反序列化来设置值
		if session, ok := v.(*stripe.CheckoutSession); ok {
			session.ID = resp.ID
			session.URL = resp.URL
		}
	}
	return nil
}

// CallStreaming 实现 Backend.CallStreaming 接口
func (m *MockStripeBackend) CallStreaming(method, path, key string, params stripe.ParamsContainer, v stripe.StreamingLastResponseSetter) error {
	return nil
}

// CallRaw 实现 Backend.CallRaw 接口
func (m *MockStripeBackend) CallRaw(method, path, key string, body *form.Values, params *stripe.Params, v stripe.LastResponseSetter) error {
	return nil
}

// CallMultipart 实现 Backend.CallMultipart 接口
func (m *MockStripeBackend) CallMultipart(method, path, key, boundary string, body *bytes.Buffer, params *stripe.Params, v stripe.LastResponseSetter) error {
	return nil
}

// SetMaxNetworkRetries 实现 Backend.SetMaxNetworkRetries 接口
func (m *MockStripeBackend) SetMaxNetworkRetries(maxNetworkRetries int64) {}

// StripeConfig 保存 Stripe 原始配置
type StripeConfig struct {
	ApiSecret      string
	WebhookSecret  string
	OriginalBackend stripe.Backend
}

// SetupStripeMockConfig 设置 Stripe mock 配置，返回恢复函数
func SetupStripeMockConfig() func() {
	// 保存原始配置
	original := StripeConfig{
		ApiSecret:       setting.StripeApiSecret,
		WebhookSecret:   setting.StripeWebhookSecret,
		OriginalBackend: stripe.GetBackend(stripe.APIBackend),
	}

	// 设置 mock 配置
	// 使用 sk_test_ 前缀的测试密钥
	setting.StripeApiSecret = "sk_test_mock_api_secret_for_testing"
	setting.StripeWebhookSecret = "whsec_test_mock_webhook_secret"

	// 设置 mock backend
	mockBackend := NewMockStripeBackend()
	stripe.SetBackend(stripe.APIBackend, mockBackend)

	// 返回恢复函数
	return func() {
		setting.StripeApiSecret = original.ApiSecret
		setting.StripeWebhookSecret = original.WebhookSecret
		stripe.SetBackend(stripe.APIBackend, original.OriginalBackend)
	}
}

// ===================== 辅助函数 =====================

// CreateMockEpayServer 创建模拟 Epay 服务器（如果需要验证回调等）
func CreateMockEpayServer() *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 模拟 Epay 响应
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": 1,
			"msg":  "success",
		})
	})
	return httptest.NewServer(handler)
}

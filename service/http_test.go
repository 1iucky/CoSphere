package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetBillingHeadersFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("从 context 读取并设置 billing_source", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 模拟 PreConsumeQuota 设置 billing 信息到 context
		c.Set(string(constant.ContextKeyBillingSource), "subscription")

		SetBillingHeadersFromContext(c)

		assert.Equal(t, "subscription", w.Header().Get("X-New-Api-Billing-Source"))
	})

	t.Run("从 context 读取并设置 billing_source 和 skip_reason", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set(string(constant.ContextKeyBillingSource), "skipped")
		c.Set(string(constant.ContextKeyBillingSkipReason), "subscription_preferred_disabled")

		SetBillingHeadersFromContext(c)

		assert.Equal(t, "skipped", w.Header().Get("X-New-Api-Billing-Source"))
		assert.Equal(t, "subscription_preferred_disabled", w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("context 无 billing 信息时不设置响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 不设置任何 billing 信息
		SetBillingHeadersFromContext(c)

		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Source"))
		assert.Empty(t, w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})

	t.Run("防止重复设置响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set(string(constant.ContextKeyBillingSource), "wallet")
		SetBillingHeadersFromContext(c)

		assert.Equal(t, "wallet", w.Header().Get("X-New-Api-Billing-Source"))

		// 修改 context 中的值
		c.Set(string(constant.ContextKeyBillingSource), "subscription")
		// 再次调用（应被忽略）
		SetBillingHeadersFromContext(c)

		// 响应头应保持第一次设置的值
		assert.Equal(t, "wallet", w.Header().Get("X-New-Api-Billing-Source"))
	})

	t.Run("设置 wallet 来源", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set(string(constant.ContextKeyBillingSource), "wallet")

		SetBillingHeadersFromContext(c)

		assert.Equal(t, "wallet", w.Header().Get("X-New-Api-Billing-Source"))
	})

	t.Run("设置 fallback 来源", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set(string(constant.ContextKeyBillingSource), "fallback")
		c.Set(string(constant.ContextKeyBillingSkipReason), "subscription_quota_exceeded")

		SetBillingHeadersFromContext(c)

		assert.Equal(t, "fallback", w.Header().Get("X-New-Api-Billing-Source"))
		assert.Equal(t, "subscription_quota_exceeded", w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})
}

func TestIOCopyBytesGracefully_WithBillingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("IOCopyBytesGracefully 应设置计费响应头", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// 模拟 PreConsumeQuota 设置 billing 信息
		c.Set(string(constant.ContextKeyBillingSource), "subscription")

		// 调用 IOCopyBytesGracefully
		IOCopyBytesGracefully(c, nil, []byte(`{"test": "data"}`))

		// 验证计费响应头
		assert.Equal(t, "subscription", w.Header().Get("X-New-Api-Billing-Source"))
	})

	t.Run("IOCopyBytesGracefully 设置 skipped 来源和原因", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		c.Set(string(constant.ContextKeyBillingSource), "skipped")
		c.Set(string(constant.ContextKeyBillingSkipReason), "subscription_preferred_disabled")

		IOCopyBytesGracefully(c, nil, []byte(`{"test": "data"}`))

		assert.Equal(t, "skipped", w.Header().Get("X-New-Api-Billing-Source"))
		assert.Equal(t, "subscription_preferred_disabled", w.Header().Get("X-New-Api-Billing-Skip-Reason"))
	})
}

package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// SetBillingHeadersFromContext 从 context 读取 billing 信息并设置响应头
// 这是一个公共函数，可以被其他需要直接写入响应的 handler 调用
func SetBillingHeadersFromContext(c *gin.Context) {
	// 检查是否已经设置过计费响应头
	if _, exists := c.Get("billing_headers_set"); exists {
		return
	}

	// 从 context 读取 billing 信息
	billingSource := ""
	if source, exists := c.Get(string(constant.ContextKeyBillingSource)); exists {
		if s, ok := source.(string); ok {
			billingSource = s
		}
	}

	skipReason := ""
	if reason, exists := c.Get(string(constant.ContextKeyBillingSkipReason)); exists {
		if r, ok := reason.(string); ok {
			skipReason = r
		}
	}

	// 如果没有 billing 信息，不设置响应头
	if billingSource == "" && skipReason == "" {
		return
	}

	// 设置标志，表示计费响应头已经设置过
	c.Set("billing_headers_set", true)

	// 设置计费来源响应头
	if billingSource != "" {
		c.Writer.Header().Set("X-New-Api-Billing-Source", billingSource)
	}

	// 设置跳过原因响应头（仅当有跳过原因时）
	if skipReason != "" {
		c.Writer.Header().Set("X-New-Api-Billing-Skip-Reason", skipReason)
	}
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			// avoid setting Content-Length
			if k == "Content-Length" {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// 从 context 读取并设置计费响应头（集中处理，避免遗漏）
	SetBillingHeadersFromContext(c)

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
}

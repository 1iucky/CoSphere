package helper

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func FlushWriter(c *gin.Context) error {
	if c.Writer == nil {
		return nil
	}
	if flusher, ok := c.Writer.(http.Flusher); ok {
		flusher.Flush()
		return nil
	}
	return errors.New("streaming error: flusher not found")
}

func SetEventStreamHeaders(c *gin.Context) {
	// 检查是否已经设置过头部
	if _, exists := c.Get("event_stream_headers_set"); exists {
		return
	}

	// 设置标志，表示头部已经设置过
	c.Set("event_stream_headers_set", true)

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("Transfer-Encoding", "chunked")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	// 流式响应统一设置计费头（从 context 读取）
	SetBillingHeaders(c, "", "")
}

// SetBillingHeaders 设置计费相关的响应头
// 响应头名称：
//   - X-New-Api-Billing-Source: 计费来源（subscription/wallet/fallback/skipped）
//   - X-New-Api-Billing-Skip-Reason: 跳过订阅扣费的原因（仅当跳过时设置）
//
// 这些响应头用于向调用方提示计费信息，不会影响 OpenAI 格式的兼容性
// 如果传入参数为空，会尝试从 context 读取 billing 信息
func SetBillingHeaders(c *gin.Context, billingSource, skipReason string) {
	// 检查是否已经设置过计费响应头
	if _, exists := c.Get("billing_headers_set"); exists {
		return
	}

	// 如果传入参数为空，尝试从 context 读取
	if billingSource == "" {
		if source, exists := c.Get(string(constant.ContextKeyBillingSource)); exists {
			if s, ok := source.(string); ok {
				billingSource = s
			}
		}
	}
	if skipReason == "" {
		if reason, exists := c.Get(string(constant.ContextKeyBillingSkipReason)); exists {
			if r, ok := reason.(string); ok {
				skipReason = r
			}
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

func ClaudeData(c *gin.Context, resp dto.ClaudeResponse) error {
	rewritten := relaycommon.RewriteResponseObjectModel(relaycommon.GetRelayInfo(c), resp)
	jsonData, err := common.Marshal(rewritten)
	if err != nil {
		common.SysError("error marshalling stream response: " + err.Error())
	} else {
		c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
		c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonData)})
	}
	_ = FlushWriter(c)
	return nil
}

func ClaudeChunkData(c *gin.Context, resp dto.ClaudeResponse, data string) {
	if info := relaycommon.GetRelayInfo(c); info != nil {
		if rewritten, err := relaycommon.RewriteJSONModel(info, []byte(data), []string{"model"}, []string{"message", "model"}); err == nil {
			data = string(rewritten)
		}
	}
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("data: %s\n", data)})
	_ = FlushWriter(c)
}

func ResponseChunkData(c *gin.Context, resp dto.ResponsesStreamResponse, data string) {
	if info := relaycommon.GetRelayInfo(c); info != nil {
		if rewritten, err := relaycommon.RewriteJSONModel(info, []byte(data), []string{"response", "model"}); err == nil {
			data = string(rewritten)
		}
	}
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("event: %s\n", resp.Type)})
	c.Render(-1, common.CustomEvent{Data: fmt.Sprintf("data: %s", data)})
	_ = FlushWriter(c)
}

func StringData(c *gin.Context, str string) error {
	if info := relaycommon.GetRelayInfo(c); info != nil {
		if rewritten, err := relaycommon.RewriteJSONModel(info, []byte(str), []string{"model"}, []string{"response", "model"}, []string{"message", "model"}); err == nil {
			str = string(rewritten)
		}
	}
	//str = strings.TrimPrefix(str, "data: ")
	//str = strings.TrimSuffix(str, "\r")
	c.Render(-1, common.CustomEvent{Data: "data: " + str})
	_ = FlushWriter(c)
	return nil
}

func PingData(c *gin.Context) error {
	c.Writer.Write([]byte(": PING\n\n"))
	_ = FlushWriter(c)
	return nil
}

func ObjectData(c *gin.Context, object interface{}) error {
	if object == nil {
		return errors.New("object is nil")
	}
	rewritten := relaycommon.RewriteResponseObjectModel(relaycommon.GetRelayInfo(c), object)
	jsonData, err := common.Marshal(rewritten)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	return StringData(c, string(jsonData))
}

func Done(c *gin.Context) {
	_ = StringData(c, "[DONE]")
}

func WssString(c *gin.Context, ws *websocket.Conn, str string) error {
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", str))
	return ws.WriteMessage(1, []byte(str))
}

func WssObject(c *gin.Context, ws *websocket.Conn, object interface{}) error {
	jsonData, err := common.Marshal(object)
	if err != nil {
		return fmt.Errorf("error marshalling object: %w", err)
	}
	if ws == nil {
		logger.LogError(c, "websocket connection is nil")
		return errors.New("websocket connection is nil")
	}
	//common.LogInfo(c, fmt.Sprintf("sending message: %s", jsonData))
	return ws.WriteMessage(1, jsonData)
}

func WssError(c *gin.Context, ws *websocket.Conn, openaiError types.OpenAIError) {
	if ws == nil {
		return
	}
	errorObj := &dto.RealtimeEvent{
		Type:    "error",
		EventId: GetLocalRealtimeID(c),
		Error:   &openaiError,
	}
	_ = WssObject(c, ws, errorObj)
}

func GetResponseID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("chatcmpl-%s", logID)
}

func GetLocalRealtimeID(c *gin.Context) string {
	logID := c.GetString(common.RequestIdKey)
	return fmt.Sprintf("evt_%s", logID)
}

func GenerateStartEmptyResponse(id string, createAt int64, model string, systemFingerprint *string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: systemFingerprint,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Role:    "assistant",
					Content: common.GetPointer(""),
				},
			},
		},
	}
}

func GenerateStopResponse(id string, createAt int64, model string, finishReason string) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices: []dto.ChatCompletionsStreamResponseChoice{
			{
				FinishReason: &finishReason,
			},
		},
	}
}

func GenerateFinalUsageResponse(id string, createAt int64, model string, usage dto.Usage) *dto.ChatCompletionsStreamResponse {
	return &dto.ChatCompletionsStreamResponse{
		Id:                id,
		Object:            "chat.completion.chunk",
		Created:           createAt,
		Model:             model,
		SystemFingerprint: nil,
		Choices:           make([]dto.ChatCompletionsStreamResponseChoice, 0),
		Usage:             &usage,
	}
}

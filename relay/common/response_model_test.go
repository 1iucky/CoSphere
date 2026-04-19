package common

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseModelName(t *testing.T) {
	info := &RelayInfo{OriginModelName: "gpt-4o", ChannelMeta: &ChannelMeta{UpstreamModelName: "claude-3-7-sonnet", IsModelMapped: true}}

	assert.Equal(t, "gpt-4o", ResponseModelName(info, "claude-3-7-sonnet"))
	assert.Equal(t, "gpt-4o", DisplayResponseModelName(info))
	assert.Equal(t, "claude-3-7-sonnet", ResponseModelName(nil, "claude-3-7-sonnet"))
	assert.Equal(t, "claude-3-7-sonnet", ResponseModelName(&RelayInfo{OriginModelName: "gpt-4o"}, "claude-3-7-sonnet"))
}

func TestRewriteJSONModel(t *testing.T) {
	info := &RelayInfo{OriginModelName: "gpt-4o", ChannelMeta: &ChannelMeta{IsModelMapped: true}}
	payload := []byte(`{"model":"claude-3-7-sonnet","response":{"model":"claude-3-7-sonnet"},"message":{"model":"claude-3-7-sonnet"}}`)

	rewritten, err := RewriteJSONModel(info, payload, []string{"model"}, []string{"response", "model"}, []string{"message", "model"})
	require.NoError(t, err)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rewritten, &body))
	assert.Equal(t, "gpt-4o", body["model"])
	assert.Equal(t, "gpt-4o", body["response"].(map[string]interface{})["model"])
	assert.Equal(t, "gpt-4o", body["message"].(map[string]interface{})["model"])

	unchanged, err := RewriteJSONModel(info, []byte(`{"id":"resp_123"}`), []string{"model"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"resp_123"}`, string(unchanged))
}

func TestRewriteResponseObjectModel(t *testing.T) {
	info := &RelayInfo{OriginModelName: "gpt-4o", ChannelMeta: &ChannelMeta{IsModelMapped: true}}

	streamResponse := RewriteResponseObjectModel(info, dto.ChatCompletionsStreamResponse{Model: "claude-3-7-sonnet"}).(dto.ChatCompletionsStreamResponse)
	assert.Equal(t, "gpt-4o", streamResponse.Model)

	claudeResponse := RewriteResponseObjectModel(info, dto.ClaudeResponse{
		Model: "claude-3-7-sonnet",
		Message: &dto.ClaudeMediaMessage{
			Model: "claude-3-7-sonnet",
		},
	}).(dto.ClaudeResponse)
	assert.Equal(t, "gpt-4o", claudeResponse.Model)
	require.NotNil(t, claudeResponse.Message)
	assert.Equal(t, "gpt-4o", claudeResponse.Message.Model)

	responsesEvent := RewriteResponseObjectModel(info, dto.ResponsesStreamResponse{
		Response: &dto.OpenAIResponsesResponse{Model: "claude-3-7-sonnet"},
	}).(dto.ResponsesStreamResponse)
	require.NotNil(t, responsesEvent.Response)
	assert.Equal(t, "gpt-4o", responsesEvent.Response.Model)
}

func TestRelayInfoContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	info := &RelayInfo{OriginModelName: "gpt-4o", ChannelMeta: &ChannelMeta{IsModelMapped: true}}

	SetRelayInfo(ctx, info)
	assert.Same(t, info, GetRelayInfo(ctx))
	assert.Equal(t, "gpt-4o", ResponseModelNameFromContext(ctx, "claude-3-7-sonnet"))
}

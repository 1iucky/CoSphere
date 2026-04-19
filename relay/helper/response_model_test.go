package helper

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMappedContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	relaycommon.SetRelayInfo(c, &relaycommon.RelayInfo{
		OriginModelName: "gpt-4o",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "claude-3-7-sonnet",
			IsModelMapped:     true,
		},
	})
	return c, w
}

func TestStringData_RewritesTopLevelModel(t *testing.T) {
	c, w := newMappedContext()

	require.NoError(t, StringData(c, `{"id":"chatcmpl_123","model":"claude-3-7-sonnet"}`))
	assert.Contains(t, w.Body.String(), `"model":"gpt-4o"`)
}

func TestStringData_RewritesResponsesModel(t *testing.T) {
	c, w := newMappedContext()

	require.NoError(t, StringData(c, `{"type":"response.completed","response":{"id":"resp_123","model":"claude-3-7-sonnet"}}`))
	assert.Contains(t, w.Body.String(), `"model":"gpt-4o"`)
}

func TestObjectData_RewritesStreamModel(t *testing.T) {
	c, w := newMappedContext()

	err := ObjectData(c, dto.ChatCompletionsStreamResponse{Model: "claude-3-7-sonnet"})
	require.NoError(t, err)
	assert.Contains(t, w.Body.String(), `"model":"gpt-4o"`)
}

func TestClaudeChunkData_RewritesMessageModel(t *testing.T) {
	c, w := newMappedContext()

	ClaudeChunkData(c, dto.ClaudeResponse{Type: "message_start"}, `{"type":"message_start","message":{"model":"claude-3-7-sonnet"}}`)
	assert.Contains(t, w.Body.String(), `"model":"gpt-4o"`)
	assert.Contains(t, w.Body.String(), `event: message_start`)
}

func TestResponseChunkData_RewritesNestedModel(t *testing.T) {
	c, w := newMappedContext()

	ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.completed"}, `{"type":"response.completed","response":{"model":"claude-3-7-sonnet"}}`)
	assert.Contains(t, w.Body.String(), `"model":"gpt-4o"`)
	assert.Contains(t, w.Body.String(), `event: response.completed`)
}

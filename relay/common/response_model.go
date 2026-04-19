package common

import (
	basecommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
)

const RelayInfoContextKey = "relay_info"

func SetRelayInfo(c *gin.Context, info *RelayInfo) {
	if c == nil || info == nil {
		return
	}
	c.Set(RelayInfoContextKey, info)
}

func GetRelayInfo(c *gin.Context) *RelayInfo {
	if c == nil {
		return nil
	}
	value, exists := c.Get(RelayInfoContextKey)
	if !exists {
		return nil
	}
	info, ok := value.(*RelayInfo)
	if !ok {
		return nil
	}
	return info
}

func ResponseModelName(info *RelayInfo, current string) string {
	if info == nil || info.ChannelMeta == nil || !info.IsModelMapped || info.OriginModelName == "" {
		return current
	}
	return info.OriginModelName
}

func ResponseModelNameFromContext(c *gin.Context, current string) string {
	return ResponseModelName(GetRelayInfo(c), current)
}

func DisplayResponseModelName(info *RelayInfo) string {
	if info == nil {
		return ""
	}
	if info.ChannelMeta == nil {
		return info.OriginModelName
	}
	return ResponseModelName(info, info.UpstreamModelName)
}

func DisplayResponseModelNameFromContext(c *gin.Context) string {
	return DisplayResponseModelName(GetRelayInfo(c))
}

func RewriteTextResponseModel(info *RelayInfo, response *dto.TextResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
}

func RewriteOpenAITextResponseModel(info *RelayInfo, response *dto.OpenAITextResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
}

func RewriteChatCompletionsStreamResponseModel(info *RelayInfo, response *dto.ChatCompletionsStreamResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
}

func RewriteOpenAIResponsesResponseModel(info *RelayInfo, response *dto.OpenAIResponsesResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
}

func RewriteResponsesStreamResponseModel(info *RelayInfo, response *dto.ResponsesStreamResponse) {
	if response == nil || response.Response == nil {
		return
	}
	RewriteOpenAIResponsesResponseModel(info, response.Response)
}

func RewriteClaudeResponseModel(info *RelayInfo, response *dto.ClaudeResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
	if response.Message != nil {
		response.Message.Model = ResponseModelName(info, response.Message.Model)
	}
}

func RewriteOpenAIEmbeddingResponseModel(info *RelayInfo, response *dto.OpenAIEmbeddingResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
}

func RewriteFlexibleEmbeddingResponseModel(info *RelayInfo, response *dto.FlexibleEmbeddingResponse) {
	if response == nil {
		return
	}
	response.Model = ResponseModelName(info, response.Model)
}

func RewriteResponseObjectModel(info *RelayInfo, object interface{}) interface{} {
	switch value := object.(type) {
	case dto.TextResponse:
		RewriteTextResponseModel(info, &value)
		return value
	case *dto.TextResponse:
		if value == nil {
			return object
		}
		copied := *value
		RewriteTextResponseModel(info, &copied)
		return &copied
	case dto.OpenAITextResponse:
		RewriteOpenAITextResponseModel(info, &value)
		return value
	case *dto.OpenAITextResponse:
		if value == nil {
			return object
		}
		copied := *value
		RewriteOpenAITextResponseModel(info, &copied)
		return &copied
	case dto.ChatCompletionsStreamResponse:
		RewriteChatCompletionsStreamResponseModel(info, &value)
		return value
	case *dto.ChatCompletionsStreamResponse:
		if value == nil {
			return object
		}
		copied := *value
		RewriteChatCompletionsStreamResponseModel(info, &copied)
		return &copied
	case dto.OpenAIResponsesResponse:
		RewriteOpenAIResponsesResponseModel(info, &value)
		return value
	case *dto.OpenAIResponsesResponse:
		if value == nil {
			return object
		}
		copied := *value
		RewriteOpenAIResponsesResponseModel(info, &copied)
		return &copied
	case dto.ResponsesStreamResponse:
		if value.Response != nil {
			copiedResponse := *value.Response
			value.Response = &copiedResponse
		}
		RewriteResponsesStreamResponseModel(info, &value)
		return value
	case *dto.ResponsesStreamResponse:
		if value == nil {
			return object
		}
		copied := *value
		if value.Response != nil {
			copiedResponse := *value.Response
			copied.Response = &copiedResponse
		}
		RewriteResponsesStreamResponseModel(info, &copied)
		return &copied
	case dto.ClaudeResponse:
		if value.Message != nil {
			copiedMessage := *value.Message
			value.Message = &copiedMessage
		}
		RewriteClaudeResponseModel(info, &value)
		return value
	case *dto.ClaudeResponse:
		if value == nil {
			return object
		}
		copied := *value
		if value.Message != nil {
			copiedMessage := *value.Message
			copied.Message = &copiedMessage
		}
		RewriteClaudeResponseModel(info, &copied)
		return &copied
	case dto.OpenAIEmbeddingResponse:
		RewriteOpenAIEmbeddingResponseModel(info, &value)
		return value
	case *dto.OpenAIEmbeddingResponse:
		if value == nil {
			return object
		}
		copied := *value
		RewriteOpenAIEmbeddingResponseModel(info, &copied)
		return &copied
	case dto.FlexibleEmbeddingResponse:
		RewriteFlexibleEmbeddingResponseModel(info, &value)
		return value
	case *dto.FlexibleEmbeddingResponse:
		if value == nil {
			return object
		}
		copied := *value
		RewriteFlexibleEmbeddingResponseModel(info, &copied)
		return &copied
	default:
		return object
	}
}

func RewriteJSONModel(info *RelayInfo, payload []byte, paths ...[]string) ([]byte, error) {
	if info == nil || !info.IsModelMapped || info.OriginModelName == "" || len(payload) == 0 {
		return payload, nil
	}

	var body map[string]interface{}
	if err := basecommon.Unmarshal(payload, &body); err != nil {
		return nil, err
	}

	changed := false
	for _, path := range paths {
		if setNestedString(body, path, info.OriginModelName) {
			changed = true
		}
	}
	if !changed {
		return payload, nil
	}

	return basecommon.Marshal(body)
}

func setNestedString(body map[string]interface{}, path []string, value string) bool {
	if len(path) == 0 {
		return false
	}

	current := map[string]interface{}(body)
	for i, key := range path {
		if i == len(path)-1 {
			if _, exists := current[key]; !exists {
				return false
			}
			current[key] = value
			return true
		}

		next, exists := current[key]
		if !exists || next == nil {
			return false
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return false
		}
		current = nextMap
	}

	return false
}

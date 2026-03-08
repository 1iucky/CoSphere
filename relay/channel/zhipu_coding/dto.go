package zhipu_coding

import (
	"time"

	"github.com/QuantumNous/new-api/dto"
)

type ZhipuCodingResponse struct {
	Id                  string                         `json:"id"`
	Created             int64                          `json:"created"`
	Model               string                         `json:"model"`
	TextResponseChoices []dto.OpenAITextResponseChoice `json:"choices"`
	Usage               dto.Usage                      `json:"usage"`
	Error               dto.OpenAIError                `json:"error"`
}

type ZhipuCodingStreamResponse struct {
	Id      string                                    `json:"id"`
	Created int64                                     `json:"created"`
	Choices []dto.ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage   dto.Usage                                 `json:"usage"`
}

type tokenData struct {
	Token      string
	ExpiryTime time.Time
}
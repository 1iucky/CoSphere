package xunfei_coding

import (
	"github.com/QuantumNous/new-api/dto"
)

type XunfeiCodingResponse struct {
	Id                  string                         `json:"id"`
	Created             int64                          `json:"created"`
	Model               string                         `json:"model"`
	TextResponseChoices []dto.OpenAITextResponseChoice `json:"choices"`
	Usage               dto.Usage                      `json:"usage"`
	Error               dto.OpenAIError                `json:"error"`
}

type XunfeiCodingStreamResponse struct {
	Id      string                                    `json:"id"`
	Created int64                                     `json:"created"`
	Choices []dto.ChatCompletionsStreamResponseChoice `json:"choices"`
	Usage   dto.Usage                                 `json:"usage"`
}

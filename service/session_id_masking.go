package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	// maskedSessionTTL 伪装会话 ID 的缓存有效期（15分钟）
	maskedSessionTTLSec = 900
)

// GetOrCreateMaskedSessionID 获取或创建渠道的伪装会话 ID
// 伪装 ID 在 15 分钟内保持不变，过期后重新生成
func GetOrCreateMaskedSessionID(channelID int) (string, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return generateRandomUUID(), nil
	}

	key := fmt.Sprintf("channel_masked_session:%d", channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 尝试获取已有的伪装 ID
	val, err := common.RDB.Get(ctx, key).Result()
	if err == nil && val != "" {
		// 刷新 TTL
		common.RDB.Expire(ctx, key, time.Duration(maskedSessionTTLSec)*time.Second)
		return val, nil
	}

	// 生成新的伪装 ID
	newID := generateRandomUUID()
	if err := common.RDB.Set(ctx, key, newID, time.Duration(maskedSessionTTLSec)*time.Second).Err(); err != nil {
		// 设置失败仍然返回生成的 ID（非致命错误）
		return newID, nil
	}
	return newID, nil
}

// generateRandomUUID 生成随机 UUID v4 格式字符串
func generateRandomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		h := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		b = h[:16]
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// ApplyClaudeSessionIDMasking 对 Claude 格式请求体应用会话 ID 伪装
// 精确替换 metadata.user_id 字段，不修改其他字段（如 thinking 块等）
// 参考 sub2api 的 RewriteUserIDWithMasking 实现
func ApplyClaudeSessionIDMasking(body []byte, channelID int) []byte {
	maskedID, err := GetOrCreateMaskedSessionID(channelID)
	if err != nil || maskedID == "" {
		return body
	}
	result, setErr := sjson.SetBytes(body, "metadata.user_id", maskedID)
	if setErr != nil {
		return body
	}
	return result
}

// ApplyOpenAISessionIDMasking 对 OpenAI 格式请求体应用会话 ID 伪装
// 精确替换 user 字段
func ApplyOpenAISessionIDMasking(body []byte, channelID int) []byte {
	maskedID, err := GetOrCreateMaskedSessionID(channelID)
	if err != nil || maskedID == "" {
		return body
	}
	result, setErr := sjson.SetBytes(body, "user", maskedID)
	if setErr != nil {
		return body
	}
	return result
}

// ExtractClaudeSessionID 从 Claude 格式请求体中提取 metadata.user_id 作为会话标识
func ExtractClaudeSessionID(body []byte) string {
	result := gjson.GetBytes(body, "metadata.user_id")
	if result.Exists() && result.Type.String() == "String" {
		return result.String()
	}
	return ""
}

// ExtractOpenAISessionID 从 OpenAI 格式请求体中提取 user 字段作为会话标识
func ExtractOpenAISessionID(body []byte) string {
	result := gjson.GetBytes(body, "user")
	if result.Exists() && result.Type.String() == "String" {
		return result.String()
	}
	return ""
}

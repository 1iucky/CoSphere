package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// CheckChannelRPM 检查渠道 RPM 限制
// 返回 (是否允许, 错误)
// channelID: 渠道 ID
// settings: 渠道配额设置
// isSticky: 当前请求是否为粘性会话（连续对话）
func CheckChannelRPM(channelID int, settings *dto.ChannelSettings, isSticky bool) (bool, error) {
	if !settings.HasRPMConfig() {
		return true, nil
	}

	if !common.RedisEnabled || common.RDB == nil {
		// Redis 不可用时降级为不限流
		return true, nil
	}

	baseRPM := *settings.BaseRPM
	stickyBuffer := settings.GetRPMStickyBuffer()

	// 粘性会话有额外缓冲配额
	effectiveRPM := baseRPM
	if isSticky {
		effectiveRPM += stickyBuffer
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 使用当前分钟作为时间窗口 key
	now := time.Now()
	windowKey := fmt.Sprintf("channel_rpm:%d:%d", channelID, now.Unix()/60)

	// INCR + EXPIRE 原子操作
	count, err := common.RDB.Incr(ctx, windowKey).Result()
	if err != nil {
		// Redis 错误时降级为允许
		return true, fmt.Errorf("redis incr failed: %w", err)
	}

	// 设置过期时间为 120 秒（确保当前分钟窗口过期后自动清理）
	if count == 1 {
		common.RDB.Expire(ctx, windowKey, 120*time.Second)
	}

	return count <= int64(effectiveRPM), nil
}

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/go-redis/redis/v8"
)

// RegisterSession 注册一个活跃会话
// 返回 (是否允许, 错误)
// 当活跃会话数超过限制时返回 false
func RegisterSession(channelID int, sessionID string, settings *dto.ChannelSettings) (bool, error) {
	if !settings.HasSessionConfig() || sessionID == "" {
		return true, nil
	}

	if !common.RedisEnabled || common.RDB == nil {
		return true, nil
	}

	maxSessions := *settings.MaxSessions
	idleTimeout := settings.GetSessionIdleTimeout()
	key := fmt.Sprintf("channel_sessions:%d", channelID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := float64(time.Now().Unix())

	// 1. 清理过期会话（超时未活跃的）
	maxScore := float64(time.Now().Add(-time.Duration(idleTimeout) * time.Minute).Unix())
	common.RDB.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%.0f", maxScore))

	// 2. 添加或更新当前会话（score 为当前时间戳）
	common.RDB.ZAdd(ctx, key, &redis.Z{
		Score:  now,
		Member: sessionID,
	})

	// 3. 设置 key 过期时间（idleTimeout * 2，确保有足够缓冲）
	common.RDB.Expire(ctx, key, time.Duration(idleTimeout*2)*time.Minute)

	// 4. 检查活跃会话数
	count, err := common.RDB.ZCard(ctx, key).Result()
	if err != nil {
		return true, fmt.Errorf("redis zcard failed: %w", err)
	}

	// 当前会话已计入，所以允许数包含当前会话
	return count <= int64(maxSessions), nil
}

// RenewSession 续约会话活跃时间
func RenewSession(channelID int, sessionID string) error {
	if sessionID == "" {
		return nil
	}

	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	key := fmt.Sprintf("channel_sessions:%d", channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := float64(time.Now().Unix())
	return common.RDB.ZAdd(ctx, key, &redis.Z{
		Score:  now,
		Member: sessionID,
	}).Err()
}

// UnregisterSession 注销一个会话
func UnregisterSession(channelID int, sessionID string) error {
	if sessionID == "" {
		return nil
	}

	if !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	key := fmt.Sprintf("channel_sessions:%d", channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return common.RDB.ZRem(ctx, key, sessionID).Err()
}

// GetActiveSessionCount 获取渠道活跃会话数
func GetActiveSessionCount(channelID int, settings *dto.ChannelSettings) (int64, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, nil
	}

	key := fmt.Sprintf("channel_sessions:%d", channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// 先清理过期会话
	if settings != nil {
		idleTimeout := settings.GetSessionIdleTimeout()
		maxScore := float64(time.Now().Add(-time.Duration(idleTimeout) * time.Minute).Unix())
		common.RDB.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%.0f", maxScore))
	}

	return common.RDB.ZCard(ctx, key).Result()
}

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// AcquireChannelConcurrency 尝试获取渠道并发令牌
// 返回 (释放函数, 是否获取成功, 错误)
// 调用方必须在请求完成后调用 release() 释放令牌
func AcquireChannelConcurrency(channelID int, settings *dto.ChannelSettings) (release func(), ok bool, err error) {
	if !settings.HasConcurrencyConfig() {
		return func() {}, true, nil
	}

	if !common.RedisEnabled || common.RDB == nil {
		// Redis 不可用时降级为不限制
		return func() {}, true, nil
	}

	maxConcurrency := *settings.MaxConcurrency
	key := fmt.Sprintf("channel_concurrency:%d", channelID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// INCR 原子递增当前并发数
	count, err := common.RDB.Incr(ctx, key).Result()
	if err != nil {
		return func() {}, true, fmt.Errorf("redis incr failed: %w", err)
	}

	// 超过限制则回退并拒绝
	if count > int64(maxConcurrency) {
		common.RDB.Decr(ctx, key)
		return func() {}, false, nil
	}

	// 设置 TTL 防止泄漏（5分钟，足够任何请求完成）
	if count == 1 {
		common.RDB.Expire(ctx, key, 300*time.Second)
	}

	// 返回释放函数
	released := false
	release = func() {
		if released {
			return
		}
		released = true
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer releaseCancel()
		common.RDB.Decr(releaseCtx, key)
	}

	return release, true, nil
}

// GetChannelConcurrency 获取渠道当前并发数
func GetChannelConcurrency(channelID int) (int64, error) {
	if !common.RedisEnabled || common.RDB == nil {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	key := fmt.Sprintf("channel_concurrency:%d", channelID)
	val, err := common.RDB.Get(ctx, key).Int64()
	if err != nil {
		// key 不存在时返回 0
		return 0, nil
	}
	return val, nil
}

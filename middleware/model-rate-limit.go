package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
)

// getRateLimitIdentifier 根据配置的粒度返回限流标识符
func getRateLimitIdentifier(c *gin.Context) string {
	if setting.ModelRequestRateLimitScope == setting.RateLimitScopeToken {
		tokenId := c.GetInt("token_id")
		if tokenId > 0 {
			return strconv.Itoa(tokenId)
		}
	}
	// 默认使用用户粒度
	return strconv.Itoa(c.GetInt("id"))
}

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64, durationMinutes int) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(timeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().Format(timeFormat)
	nowTime, err := time.Parse(timeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(durationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int, durationMinutes int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().Format(timeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(durationMinutes)*time.Minute)
}

// redisRateLimitHandlerWithIdentity 使用指定标识符的Redis限流处理器
func redisRateLimitHandlerWithIdentity(identifier string, duration int64, durationMinutes int, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		rdb := common.RDB

		// 1. 检查成功请求数限制
		successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, identifier)
		allowed, err := checkRedisRateLimit(ctx, rdb, successKey, successMaxCount, duration, durationMinutes)
		if err != nil {
			fmt.Println("检查成功请求数限制失败:", err.Error())
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", durationMinutes, successMaxCount))
			return
		}

		// 2. 检查总请求数限制并记录总请求（当totalMaxCount为0时会自动跳过，使用令牌桶限流器
		if totalMaxCount > 0 {
			totalKey := fmt.Sprintf("rateLimit:%s", identifier)
			// 初始化
			tb := limiter.New(ctx, rdb)
			allowed, err = tb.Allow(
				ctx,
				totalKey,
				limiter.WithCapacity(int64(totalMaxCount)*duration),
				limiter.WithRate(int64(totalMaxCount)),
				limiter.WithRequested(duration),
			)

			if err != nil {
				fmt.Println("检查总请求数限制失败:", err.Error())
				abortWithOpenAiMessage(c, http.StatusInternalServerError, "rate_limit_check_failed")
				return
			}

			if !allowed {
				abortWithOpenAiMessage(c, http.StatusTooManyRequests, fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", durationMinutes, totalMaxCount))
			}
		}

		// 4. 处理请求
		c.Next()

		// 5. 如果请求成功，记录成功请求
		if c.Writer.Status() < 400 {
			recordRedisRequest(ctx, rdb, successKey, successMaxCount, durationMinutes)
		}
	}
}

// redisRateLimitHandler 使用系统配置标识符的Redis限流处理器
func redisRateLimitHandler(duration int64, durationMinutes int, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := getRateLimitIdentifier(c)
		redisRateLimitHandlerWithIdentity(identifier, duration, durationMinutes, totalMaxCount, successMaxCount)(c)
	}
}

// memoryRateLimitHandlerWithIdentity 使用指定标识符的内存限流处理器
func memoryRateLimitHandlerWithIdentity(identifier string, duration int64, durationMinutes int, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	inMemoryRateLimiter.Init(time.Duration(durationMinutes) * time.Minute)

	return func(c *gin.Context) {
		totalKey := ModelRequestRateLimitCountMark + identifier
		successKey := ModelRequestRateLimitSuccessCountMark + identifier

		// 1. 检查总请求数限制（当totalMaxCount为0时跳过）
		if totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, totalMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 2. 检查成功请求数限制
		checkKey := successKey + "_check"
		if !inMemoryRateLimiter.Request(checkKey, successMaxCount, duration) {
			c.Status(http.StatusTooManyRequests)
			c.Abort()
			return
		}

		// 3. 处理请求
		c.Next()

		// 4. 如果请求成功，记录到实际的成功请求计数中
		if c.Writer.Status() < 400 {
			inMemoryRateLimiter.Request(successKey, successMaxCount, duration)
		}
	}
}

// memoryRateLimitHandler 使用系统配置标识符的内存限流处理器
func memoryRateLimitHandler(duration int64, durationMinutes int, totalMaxCount, successMaxCount int) gin.HandlerFunc {
	return func(c *gin.Context) {
		identifier := getRateLimitIdentifier(c)
		memoryRateLimitHandlerWithIdentity(identifier, duration, durationMinutes, totalMaxCount, successMaxCount)(c)
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 令牌级别速率限制（优先于系统级别）
		if tokenObj, exists := c.Get("token"); exists && tokenObj != nil {
			if token, ok := tokenObj.(*model.Token); ok && token.RateLimitEnabled {
				duration := int64(token.RateLimitDurationMinutes * 60)
				if common.RedisEnabled {
					redisRateLimitHandlerWithIdentity(
						strconv.Itoa(token.Id), duration, token.RateLimitDurationMinutes,
						token.RateLimitCount, token.RateLimitSuccessCount,
					)(c)
				} else {
					memoryRateLimitHandlerWithIdentity(
						strconv.Itoa(token.Id), duration, token.RateLimitDurationMinutes,
						token.RateLimitCount, token.RateLimitSuccessCount,
					)(c)
				}
				return
			}
		}

		// 在每个请求时检查是否启用限流
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}

		// 计算限流参数
		duration := int64(setting.ModelRequestRateLimitDurationMinutes * 60)
		totalMaxCount := setting.ModelRequestRateLimitCount
		successMaxCount := setting.ModelRequestRateLimitSuccessCount

		// 获取分组
		group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if group == "" {
			group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		}

		//获取分组的限流配置
		groupTotalCount, groupSuccessCount, found := setting.GetGroupRateLimit(group)
		if found {
			totalMaxCount = groupTotalCount
			successMaxCount = groupSuccessCount
		}

		// 根据存储类型选择并执行限流处理器
		if common.RedisEnabled {
			redisRateLimitHandler(duration, setting.ModelRequestRateLimitDurationMinutes, totalMaxCount, successMaxCount)(c)
		} else {
			memoryRateLimitHandler(duration, setting.ModelRequestRateLimitDurationMinutes, totalMaxCount, successMaxCount)(c)
		}
	}
}

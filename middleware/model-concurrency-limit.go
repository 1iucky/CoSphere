package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	modelRequestConcurrencyUserKeyPrefix   = "concurrency:mr:user:"
	modelRequestConcurrencyTokenKeyPrefix  = "concurrency:mr:token:"
	defaultConcurrencyLeaseSeconds         = 300
	defaultConcurrencyRenewIntervalSeconds = 30
	defaultConcurrencyCleanupGraceSeconds  = 30
)

type concurrencyScope struct {
	key   string
	limit int
}

type concurrencyKeys struct {
	userKey  string
	tokenKey string
}

type concurrencyLeaseConfig struct {
	leaseSeconds        int64
	renewInterval       time.Duration
	cleanupGraceSeconds int64
}

type inMemoryConcurrencyLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func (l *inMemoryConcurrencyLimiter) Acquire(scopes ...*concurrencyScope) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.counts == nil {
		l.counts = make(map[string]int)
	}

	for _, scope := range scopes {
		if scope == nil || scope.limit <= 0 {
			continue
		}
		if l.counts[scope.key] >= scope.limit {
			return false
		}
	}

	for _, scope := range scopes {
		if scope == nil || scope.limit <= 0 {
			continue
		}
		l.counts[scope.key]++
	}

	return true
}

func (l *inMemoryConcurrencyLimiter) Release(keys []string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.counts == nil {
		return
	}

	for _, key := range keys {
		if key == "" {
			continue
		}
		if current, ok := l.counts[key]; ok {
			if current <= 1 {
				delete(l.counts, key)
			} else {
				l.counts[key] = current - 1
			}
		}
	}
}

var modelRequestConcurrencyLimiter inMemoryConcurrencyLimiter

var concurrencyAcquireScript = redis.NewScript(`
local userLimit = tonumber(ARGV[1])
local tokenLimit = tonumber(ARGV[2])
local requestId = ARGV[3]
local expireAt = tonumber(ARGV[4])
local now = tonumber(ARGV[5])
local ttlMs = tonumber(ARGV[6])

if userLimit ~= nil and userLimit > 0 then
  redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", now)
  local current = tonumber(redis.call("ZCARD", KEYS[1]) or "0")
  if current >= userLimit then
    return 0
  end
end

if tokenLimit ~= nil and tokenLimit > 0 then
  redis.call("ZREMRANGEBYSCORE", KEYS[2], "-inf", now)
  local current = tonumber(redis.call("ZCARD", KEYS[2]) or "0")
  if current >= tokenLimit then
    return 0
  end
end

if userLimit ~= nil and userLimit > 0 then
  redis.call("ZADD", KEYS[1], expireAt, requestId)
  if ttlMs ~= nil and ttlMs > 0 then
    redis.call("PEXPIRE", KEYS[1], ttlMs)
  end
end

if tokenLimit ~= nil and tokenLimit > 0 then
  redis.call("ZADD", KEYS[2], expireAt, requestId)
  if ttlMs ~= nil and ttlMs > 0 then
    redis.call("PEXPIRE", KEYS[2], ttlMs)
  end
end

return 1
`)

var concurrencyRenewScript = redis.NewScript(`
local requestId = ARGV[1]
local now = tonumber(ARGV[2])
local expireAt = tonumber(ARGV[3])
local ttlMs = tonumber(ARGV[4])

for i = 1, #KEYS do
  local key = KEYS[i]
  if key ~= "" then
    redis.call("ZREMRANGEBYSCORE", key, "-inf", now)
    local exists = redis.call("ZSCORE", key, requestId)
    if exists then
      redis.call("ZADD", key, expireAt, requestId)
      if ttlMs ~= nil and ttlMs > 0 then
        redis.call("PEXPIRE", key, ttlMs)
      end
    end
  end
end

return 1
`)

var concurrencyReleaseScript = redis.NewScript(`
local requestId = ARGV[1]
local now = tonumber(ARGV[2])

for i = 1, #KEYS do
  local key = KEYS[i]
  if key ~= "" then
    redis.call("ZREM", key, requestId)
    redis.call("ZREMRANGEBYSCORE", key, "-inf", now)
    local count = tonumber(redis.call("ZCARD", key) or "0")
    if count <= 0 then
      redis.call("DEL", key)
    end
  end
end

return 1
`)

func ModelRequestConcurrencyLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if !setting.ModelRequestConcurrencyLimitEnabled {
			c.Next()
			return
		}

		if common.GetContextKeyInt(c, constant.ContextKeyChannelId) == 0 {
			c.Next()
			return
		}

		group := strings.TrimSpace(common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		if group == "" {
			c.Next()
			return
		}

		userId := common.GetContextKeyInt(c, constant.ContextKeyUserId)
		tokenId := common.GetContextKeyInt(c, constant.ContextKeyTokenId)

		var userScope *concurrencyScope
		var tokenScope *concurrencyScope

		// 令牌级别并发限制（优先于系统分组配置）
		tokenHasOwnConcurrencyLimit := false
		if tokenId > 0 {
			if tokenObj, exists := c.Get("token"); exists && tokenObj != nil {
				if token, ok := tokenObj.(*model.Token); ok && token.ConcurrencyLimit > 0 {
					tokenHasOwnConcurrencyLimit = true
					tokenScope = &concurrencyScope{
						key:   buildConcurrencyKey(modelRequestConcurrencyTokenKeyPrefix, strconv.Itoa(tokenId), group),
						limit: token.ConcurrencyLimit,
					}
				}
			}
		}

		if userId > 0 {
			if limit, found := setting.GetUserGroupConcurrencyLimit(group); found && limit > 0 {
				userScope = &concurrencyScope{
					key:   buildConcurrencyKey(modelRequestConcurrencyUserKeyPrefix, strconv.Itoa(userId), group),
					limit: limit,
				}
			}
		}

		if !tokenHasOwnConcurrencyLimit && tokenId > 0 {
			if limit, found := setting.GetTokenGroupConcurrencyLimit(group); found && limit > 0 {
				tokenScope = &concurrencyScope{
					key:   buildConcurrencyKey(modelRequestConcurrencyTokenKeyPrefix, strconv.Itoa(tokenId), group),
					limit: limit,
				}
			}
		}

		if userScope == nil && tokenScope == nil {
			c.Next()
			return
		}

		requestId := getOrCreateRequestId(c)
		leaseConfig := getConcurrencyLeaseConfig()
		allowed, err := acquireConcurrency(c.Request.Context(), requestId, leaseConfig, userScope, tokenScope)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "concurrency_limit_check_failed")
			return
		}
		if !allowed {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests, buildConcurrencyDeniedMessage(group, userScope, tokenScope))
			return
		}

		keys := buildConcurrencyKeys(userScope, tokenScope)
		defer releaseConcurrency(c.Request.Context(), requestId, keys)
		if common.RedisEnabled && common.RDB != nil && leaseConfig.renewInterval > 0 {
			stopRenew := startConcurrencyLeaseRenewal(c.Request.Context(), requestId, leaseConfig, keys)
			if stopRenew != nil {
				defer stopRenew()
			}
		}
		c.Next()
	}
}

func acquireConcurrency(ctx context.Context, requestId string, leaseConfig concurrencyLeaseConfig, userScope, tokenScope *concurrencyScope) (bool, error) {
	if common.RedisEnabled && common.RDB != nil {
		return acquireRedisConcurrency(ctx, common.RDB, requestId, leaseConfig, userScope, tokenScope)
	}
	return modelRequestConcurrencyLimiter.Acquire(userScope, tokenScope), nil
}

func releaseConcurrency(ctx context.Context, requestId string, keys concurrencyKeys) {
	if keys.isEmpty() {
		return
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	if common.RedisEnabled && common.RDB != nil {
		releaseRedisConcurrency(ctx, common.RDB, requestId, keys)
		return
	}
	modelRequestConcurrencyLimiter.Release(keys.slice())
}

func acquireRedisConcurrency(ctx context.Context, rdb *redis.Client, requestId string, leaseConfig concurrencyLeaseConfig, userScope, tokenScope *concurrencyScope) (bool, error) {
	userKey := ""
	tokenKey := ""
	userLimit := int64(0)
	tokenLimit := int64(0)

	if userScope != nil && userScope.limit > 0 {
		userKey = userScope.key
		userLimit = int64(userScope.limit)
	}
	if tokenScope != nil && tokenScope.limit > 0 {
		tokenKey = tokenScope.key
		tokenLimit = int64(tokenScope.limit)
	}

	nowMs := time.Now().UnixMilli()
	expireAt := nowMs + leaseConfig.leaseSeconds*1000
	ttlMs := (leaseConfig.leaseSeconds + leaseConfig.cleanupGraceSeconds) * 1000

	result, err := concurrencyAcquireScript.Run(ctx, rdb, []string{userKey, tokenKey}, userLimit, tokenLimit, requestId, expireAt, nowMs, ttlMs).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

func releaseRedisConcurrency(ctx context.Context, rdb *redis.Client, requestId string, keys concurrencyKeys) {
	if requestId == "" {
		return
	}
	nowMs := time.Now().UnixMilli()
	_, _ = concurrencyReleaseScript.Run(ctx, rdb, []string{keys.userKey, keys.tokenKey}, requestId, nowMs).Result()
}

func buildConcurrencyKey(prefix, identifier, group string) string {
	return fmt.Sprintf("%s%s:%s", prefix, identifier, group)
}

func buildConcurrencyKeys(userScope, tokenScope *concurrencyScope) concurrencyKeys {
	var keys concurrencyKeys
	if userScope != nil && userScope.limit > 0 {
		keys.userKey = userScope.key
	}
	if tokenScope != nil && tokenScope.limit > 0 {
		keys.tokenKey = tokenScope.key
	}
	return keys
}

func buildConcurrencyDeniedMessage(group string, userScope, tokenScope *concurrencyScope) string {
	if userScope != nil && tokenScope == nil {
		return fmt.Sprintf("User concurrency limit reached: group %s allows %d concurrent requests", group, userScope.limit)
	}
	if tokenScope != nil && userScope == nil {
		return fmt.Sprintf("Token concurrency limit reached: group %s allows %d concurrent requests", group, tokenScope.limit)
	}
	return fmt.Sprintf("Concurrency limit reached for group %s", group)
}

func getOrCreateRequestId(c *gin.Context) string {
	if id := c.GetString(common.RequestIdKey); id != "" {
		return id
	}
	id := common.GetTimeString() + common.GetRandomString(8)
	c.Set(common.RequestIdKey, id)
	ctx := context.WithValue(c.Request.Context(), common.RequestIdKey, id)
	c.Request = c.Request.WithContext(ctx)
	c.Header(common.RequestIdKey, id)
	return id
}

func (keys concurrencyKeys) isEmpty() bool {
	return keys.userKey == "" && keys.tokenKey == ""
}

func (keys concurrencyKeys) slice() []string {
	result := make([]string, 0, 2)
	if keys.userKey != "" {
		result = append(result, keys.userKey)
	}
	if keys.tokenKey != "" {
		result = append(result, keys.tokenKey)
	}
	return result
}

func getConcurrencyLeaseConfig() concurrencyLeaseConfig {
	leaseSeconds := int64(defaultConcurrencyLeaseSeconds)
	if common.RelayTimeout > 0 && int64(common.RelayTimeout) > leaseSeconds {
		leaseSeconds = int64(common.RelayTimeout)
	}
	if constant.StreamingTimeout > 0 && int64(constant.StreamingTimeout) > leaseSeconds {
		leaseSeconds = int64(constant.StreamingTimeout)
	}
	if leaseSeconds < 30 {
		leaseSeconds = 30
	}

	renewInterval := time.Duration(defaultConcurrencyRenewIntervalSeconds) * time.Second
	maxRenew := time.Duration(leaseSeconds-5) * time.Second
	if maxRenew > 0 && renewInterval > maxRenew {
		renewInterval = maxRenew / 2
	}
	if renewInterval < 5*time.Second {
		renewInterval = 5 * time.Second
	}

	return concurrencyLeaseConfig{
		leaseSeconds:        leaseSeconds,
		renewInterval:       renewInterval,
		cleanupGraceSeconds: defaultConcurrencyCleanupGraceSeconds,
	}
}

func startConcurrencyLeaseRenewal(ctx context.Context, requestId string, leaseConfig concurrencyLeaseConfig, keys concurrencyKeys) func() {
	if requestId == "" || keys.isEmpty() || leaseConfig.renewInterval <= 0 || !common.RedisEnabled || common.RDB == nil {
		return nil
	}

	stop := make(chan struct{})
	var once sync.Once

	ticker := time.NewTicker(leaseConfig.renewInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := refreshRedisConcurrency(ctx, common.RDB, requestId, leaseConfig, keys); err != nil {
					if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
						common.SysLog("failed to renew concurrency lease: " + err.Error())
					}
				}
			case <-stop:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(stop)
		})
	}
}

func refreshRedisConcurrency(ctx context.Context, rdb *redis.Client, requestId string, leaseConfig concurrencyLeaseConfig, keys concurrencyKeys) error {
	if requestId == "" || keys.isEmpty() {
		return nil
	}
	nowMs := time.Now().UnixMilli()
	expireAt := nowMs + leaseConfig.leaseSeconds*1000
	ttlMs := (leaseConfig.leaseSeconds + leaseConfig.cleanupGraceSeconds) * 1000
	_, err := concurrencyRenewScript.Run(ctx, rdb, []string{keys.userKey, keys.tokenKey}, requestId, nowMs, expireAt, ttlMs).Result()
	return err
}

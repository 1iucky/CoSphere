package setting

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const (
	modelRequestConcurrencyLimitUserGroupRedisKey  = "config:model_request_concurrency_limit:user_group"
	modelRequestConcurrencyLimitTokenGroupRedisKey = "config:model_request_concurrency_limit:token_group"
)

var ModelRequestConcurrencyLimitEnabled = false
var ModelRequestConcurrencyLimitUserGroup = map[string]int{}
var ModelRequestConcurrencyLimitTokenGroup = map[string]int{}
var modelRequestConcurrencyLimitMutex sync.RWMutex

func ModelRequestConcurrencyLimitUserGroup2JSONString() string {
	modelRequestConcurrencyLimitMutex.RLock()
	defer modelRequestConcurrencyLimitMutex.RUnlock()

	jsonBytes, err := json.Marshal(ModelRequestConcurrencyLimitUserGroup)
	if err != nil {
		common.SysLog("error marshalling model request concurrency limit user group: " + err.Error())
	}
	return string(jsonBytes)
}

func ModelRequestConcurrencyLimitTokenGroup2JSONString() string {
	modelRequestConcurrencyLimitMutex.RLock()
	defer modelRequestConcurrencyLimitMutex.RUnlock()

	jsonBytes, err := json.Marshal(ModelRequestConcurrencyLimitTokenGroup)
	if err != nil {
		common.SysLog("error marshalling model request concurrency limit token group: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelRequestConcurrencyLimitUserGroupByJSONString(jsonStr string) error {
	limits, err := parseConcurrencyLimitMap(jsonStr)
	if err != nil {
		return err
	}

	modelRequestConcurrencyLimitMutex.Lock()
	ModelRequestConcurrencyLimitUserGroup = limits
	modelRequestConcurrencyLimitMutex.Unlock()

	syncConcurrencyLimitMapToRedis(modelRequestConcurrencyLimitUserGroupRedisKey, limits)
	return nil
}

func UpdateModelRequestConcurrencyLimitTokenGroupByJSONString(jsonStr string) error {
	limits, err := parseConcurrencyLimitMap(jsonStr)
	if err != nil {
		return err
	}

	modelRequestConcurrencyLimitMutex.Lock()
	ModelRequestConcurrencyLimitTokenGroup = limits
	modelRequestConcurrencyLimitMutex.Unlock()

	syncConcurrencyLimitMapToRedis(modelRequestConcurrencyLimitTokenGroupRedisKey, limits)
	return nil
}

func SyncModelRequestConcurrencyLimitToRedis() {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}

	modelRequestConcurrencyLimitMutex.RLock()
	userLimits := cloneConcurrencyLimitMap(ModelRequestConcurrencyLimitUserGroup)
	tokenLimits := cloneConcurrencyLimitMap(ModelRequestConcurrencyLimitTokenGroup)
	modelRequestConcurrencyLimitMutex.RUnlock()

	syncConcurrencyLimitMapToRedis(modelRequestConcurrencyLimitUserGroupRedisKey, userLimits)
	syncConcurrencyLimitMapToRedis(modelRequestConcurrencyLimitTokenGroupRedisKey, tokenLimits)
}

func GetUserGroupConcurrencyLimit(group string) (int, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0, false
	}
	if common.RedisEnabled && common.RDB != nil {
		limit, found, err := getConcurrencyLimitFromRedis(modelRequestConcurrencyLimitUserGroupRedisKey, group)
		if err == nil {
			if found {
				return limit, true
			}
		} else {
			common.SysLog("failed to load user concurrency limit from redis: " + err.Error())
		}
	}

	modelRequestConcurrencyLimitMutex.RLock()
	defer modelRequestConcurrencyLimitMutex.RUnlock()

	if ModelRequestConcurrencyLimitUserGroup == nil {
		return 0, false
	}
	limit, ok := ModelRequestConcurrencyLimitUserGroup[group]
	return limit, ok
}

func GetTokenGroupConcurrencyLimit(group string) (int, bool) {
	group = strings.TrimSpace(group)
	if group == "" {
		return 0, false
	}
	if common.RedisEnabled && common.RDB != nil {
		limit, found, err := getConcurrencyLimitFromRedis(modelRequestConcurrencyLimitTokenGroupRedisKey, group)
		if err == nil {
			if found {
				return limit, true
			}
		} else {
			common.SysLog("failed to load token concurrency limit from redis: " + err.Error())
		}
	}

	modelRequestConcurrencyLimitMutex.RLock()
	defer modelRequestConcurrencyLimitMutex.RUnlock()

	if ModelRequestConcurrencyLimitTokenGroup == nil {
		return 0, false
	}
	limit, ok := ModelRequestConcurrencyLimitTokenGroup[group]
	return limit, ok
}

func CheckModelRequestConcurrencyLimitUserGroup(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}

	checkMap := make(map[string]int)
	if err := json.Unmarshal([]byte(jsonStr), &checkMap); err != nil {
		return err
	}

	for rawGroup, limit := range checkMap {
		group := strings.TrimSpace(rawGroup)
		if group == "" {
			return fmt.Errorf("group name is empty")
		}
		if limit < 0 {
			return fmt.Errorf("group %s has negative concurrency limit: %d", rawGroup, limit)
		}
		if limit > math.MaxInt32 {
			return fmt.Errorf("group %s has max concurrency limit value 2147483647", rawGroup)
		}
	}

	return nil
}

func CheckModelRequestConcurrencyLimitTokenGroup(jsonStr string) error {
	if strings.TrimSpace(jsonStr) == "" {
		return nil
	}

	checkMap := make(map[string]int)
	if err := json.Unmarshal([]byte(jsonStr), &checkMap); err != nil {
		return err
	}

	for rawGroup, limit := range checkMap {
		group := strings.TrimSpace(rawGroup)
		if group == "" {
			return fmt.Errorf("group name is empty")
		}
		if limit < 0 {
			return fmt.Errorf("group %s has negative concurrency limit: %d", rawGroup, limit)
		}
		if limit > math.MaxInt32 {
			return fmt.Errorf("group %s has max concurrency limit value 2147483647", rawGroup)
		}
	}

	return nil
}

func parseConcurrencyLimitMap(jsonStr string) (map[string]int, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return map[string]int{}, nil
	}

	parsed := make(map[string]int)
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return nil, err
	}

	normalized := make(map[string]int)
	for rawGroup, limit := range parsed {
		group := strings.TrimSpace(rawGroup)
		if group == "" {
			continue
		}
		normalized[group] = limit
	}

	return normalized, nil
}

func cloneConcurrencyLimitMap(source map[string]int) map[string]int {
	if source == nil {
		return map[string]int{}
	}
	clone := make(map[string]int, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func syncConcurrencyLimitMapToRedis(redisKey string, limits map[string]int) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}

	ctx := context.Background()
	txn := common.RDB.TxPipeline()
	txn.Del(ctx, redisKey)
	if len(limits) > 0 {
		fields := make(map[string]interface{}, len(limits))
		for group, limit := range limits {
			fields[group] = strconv.Itoa(limit)
		}
		txn.HSet(ctx, redisKey, fields)
	}

	if _, err := txn.Exec(ctx); err != nil {
		common.SysLog("failed to sync model request concurrency limits to redis: " + err.Error())
	}
}

func getConcurrencyLimitFromRedis(redisKey, group string) (int, bool, error) {
	ctx := context.Background()
	value, err := common.RDB.HGet(ctx, redisKey, group).Result()
	if err != nil {
		if err == redis.Nil {
			return 0, false, nil
		}
		return 0, false, err
	}
	limit, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, err
	}
	return limit, true, nil
}

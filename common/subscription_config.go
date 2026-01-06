package common

import (
	"strconv"
	"sync"
)

// 订阅系统配置缓存
var (
	subscriptionConfig     SubscriptionConfig
	subscriptionConfigLock sync.RWMutex
)

func init() {
	// 确保在 OptionMap 尚未加载时也有合理默认值，避免出现 MaxPerUser=0 等零值导致业务不可用。
	InitSubscriptionConfig()
}

// SubscriptionConfig 订阅系统配置结构
type SubscriptionConfig struct {
	AutoWalletFallbackDefault bool    // 自动兜底默认值
	ExpiryNoticeDays          int     // 到期提醒天数
	MaxPerUser                int     // 每用户最大订阅数
	QuotaLowThreshold         float64 // 额度低阈值
	V2Enabled                 bool    // 订阅系统启用开关
}

// InitSubscriptionConfig 初始化订阅系统配置（从 OptionMap 加载）
func InitSubscriptionConfig() {
	subscriptionConfigLock.Lock()
	defer subscriptionConfigLock.Unlock()

	// 从 OptionMap 读取配置，如果不存在则使用默认值
	subscriptionConfig = SubscriptionConfig{
		AutoWalletFallbackDefault: GetBoolOptionWithDefault(OptionKeySubscriptionAutoWalletDefault, DefaultSubscriptionAutoWalletFallback),
		ExpiryNoticeDays:          GetIntOptionWithDefault(OptionKeySubscriptionExpiryNoticeDays, DefaultSubscriptionExpiryNoticeDays),
		MaxPerUser:                GetIntOptionWithDefault(OptionKeySubscriptionMaxPerUser, DefaultSubscriptionMaxPerUser),
		QuotaLowThreshold:         GetFloatOptionWithDefault(OptionKeySubscriptionQuotaLowThreshold, DefaultSubscriptionQuotaLowThreshold),
		V2Enabled:                 GetBoolOptionWithDefault(OptionKeySubscriptionV2Enabled, DefaultSubscriptionV2Enabled),
	}
}

// GetSubscriptionConfig 获取订阅系统配置（线程安全）
func GetSubscriptionConfig() SubscriptionConfig {
	subscriptionConfigLock.RLock()
	defer subscriptionConfigLock.RUnlock()
	return subscriptionConfig
}

// IsSubscriptionEnabled 判断订阅系统是否启用
func IsSubscriptionEnabled() bool {
	return GetSubscriptionConfig().V2Enabled
}

// GetSubscriptionMaxPerUser 获取每用户最大订阅数
func GetSubscriptionMaxPerUser() int {
	return GetSubscriptionConfig().MaxPerUser
}

// GetSubscriptionExpiryNoticeDays 获取到期提醒天数
func GetSubscriptionExpiryNoticeDays() int {
	return GetSubscriptionConfig().ExpiryNoticeDays
}

// GetSubscriptionQuotaLowThreshold 获取额度低阈值
func GetSubscriptionQuotaLowThreshold() float64 {
	return GetSubscriptionConfig().QuotaLowThreshold
}

// GetAutoWalletFallbackDefault 获取自动兜底默认值
func GetAutoWalletFallbackDefault() bool {
	return GetSubscriptionConfig().AutoWalletFallbackDefault
}

// UpdateSubscriptionConfig 更新订阅系统配置缓存（在配置变更时调用）
func UpdateSubscriptionConfig(key string, value string) {
	subscriptionConfigLock.Lock()
	defer subscriptionConfigLock.Unlock()

	switch key {
	case OptionKeySubscriptionAutoWalletDefault:
		if boolValue, err := strconv.ParseBool(value); err == nil {
			subscriptionConfig.AutoWalletFallbackDefault = boolValue
		}
	case OptionKeySubscriptionExpiryNoticeDays:
		if intValue, err := strconv.Atoi(value); err == nil {
			subscriptionConfig.ExpiryNoticeDays = intValue
		}
	case OptionKeySubscriptionMaxPerUser:
		if intValue, err := strconv.Atoi(value); err == nil {
			subscriptionConfig.MaxPerUser = intValue
		}
	case OptionKeySubscriptionQuotaLowThreshold:
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			subscriptionConfig.QuotaLowThreshold = floatValue
		}
	case OptionKeySubscriptionV2Enabled:
		if boolValue, err := strconv.ParseBool(value); err == nil {
			subscriptionConfig.V2Enabled = boolValue
		}
	}
}

// GetBoolOptionWithDefault 从 OptionMap 获取布尔配置，不存在时返回默认值
func GetBoolOptionWithDefault(key string, defaultValue bool) bool {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()

	if value, exists := OptionMap[key]; exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

// GetIntOptionWithDefault 从 OptionMap 获取整数配置，不存在时返回默认值
func GetIntOptionWithDefault(key string, defaultValue int) int {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()

	if value, exists := OptionMap[key]; exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetFloatOptionWithDefault 从 OptionMap 获取浮点数配置，不存在时返回默认值
func GetFloatOptionWithDefault(key string, defaultValue float64) float64 {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()

	if value, exists := OptionMap[key]; exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

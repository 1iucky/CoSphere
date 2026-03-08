package common

import (
	"strconv"
	"sync"
)

// 订阅系统配置缓存
var (
	subscriptionConfig     SubscriptionConfig
	subscriptionConfigLock sync.RWMutex

	// Feature flag 变更回调函数
	// 签名: func(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool)
	// v2EnabledChanged: 全局开关是否变化（用于处理灰度已开启时全局开关切换的场景）
	featureFlagChangeCallback func(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool)
	callbackLock              sync.RWMutex
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
	GrayscaleThreshold        int     // 灰度阈值（0-100），0表示关闭灰度
	GrayscaleMode             string  // 灰度模式：off/percentage/user_id
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
		GrayscaleThreshold:        GetIntOptionWithDefault(OptionKeySubscriptionGrayscaleThreshold, DefaultSubscriptionGrayscaleThreshold),
		GrayscaleMode:             GetStringOptionWithDefault(OptionKeySubscriptionGrayscaleMode, DefaultSubscriptionGrayscaleMode),
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

// RegisterFeatureFlagChangeCallback 注册 feature flag 变更回调
// 当 V2Enabled、灰度阈值或灰度模式变更时会调用此回调
// 用于通知缓存服务清除相关缓存
// 参数说明:
//   - oldEnabled: 变更前的有效启用状态
//   - newEnabled: 变更后的有效启用状态
//   - v2EnabledChanged: 全局开关是否变化（用于处理灰度已开启时全局开关切换的场景）
//   - grayscaleChanged: 灰度设置是否变化
func RegisterFeatureFlagChangeCallback(callback func(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool)) {
	callbackLock.Lock()
	defer callbackLock.Unlock()
	featureFlagChangeCallback = callback
}

// notifyFeatureFlagChange 通知 feature flag 变更
func notifyFeatureFlagChange(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool) {
	callbackLock.RLock()
	callback := featureFlagChangeCallback
	callbackLock.RUnlock()

	if callback != nil {
		// 异步调用回调，避免阻塞配置更新
		go callback(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged)
	}
}

// UpdateSubscriptionConfig 更新订阅系统配置缓存（在配置变更时调用）
func UpdateSubscriptionConfig(key string, value string) {
	subscriptionConfigLock.Lock()

	// 记录旧值，用于判断是否需要触发缓存刷新
	oldV2Enabled := subscriptionConfig.V2Enabled
	oldGrayscaleThreshold := subscriptionConfig.GrayscaleThreshold
	oldGrayscaleMode := subscriptionConfig.GrayscaleMode

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
	case OptionKeySubscriptionGrayscaleThreshold:
		if intValue, err := strconv.Atoi(value); err == nil {
			// 确保阈值在 0-100 范围内
			if intValue < 0 {
				intValue = 0
			}
			if intValue > 100 {
				intValue = 100
			}
			subscriptionConfig.GrayscaleThreshold = intValue
		}
	case OptionKeySubscriptionGrayscaleMode:
		// 验证模式值有效性
		if value == GrayscaleModeOff || value == GrayscaleModePercentage || value == GrayscaleModeUserID {
			subscriptionConfig.GrayscaleMode = value
		}
	}

	// 获取新值
	newV2Enabled := subscriptionConfig.V2Enabled
	newGrayscaleThreshold := subscriptionConfig.GrayscaleThreshold
	newGrayscaleMode := subscriptionConfig.GrayscaleMode

	subscriptionConfigLock.Unlock()

	// 判断是否需要触发缓存刷新回调
	v2EnabledChanged := oldV2Enabled != newV2Enabled
	grayscaleChanged := oldGrayscaleThreshold != newGrayscaleThreshold || oldGrayscaleMode != newGrayscaleMode

	if v2EnabledChanged || grayscaleChanged {
		// 计算旧的有效启用状态（考虑灰度）
		oldEffectiveEnabled := oldV2Enabled || (oldGrayscaleMode != GrayscaleModeOff && oldGrayscaleThreshold > 0)
		// 计算新的有效启用状态
		newEffectiveEnabled := newV2Enabled || (newGrayscaleMode != GrayscaleModeOff && newGrayscaleThreshold > 0)

		notifyFeatureFlagChange(oldEffectiveEnabled, newEffectiveEnabled, v2EnabledChanged, grayscaleChanged)
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

// GetStringOptionWithDefault 从 OptionMap 获取字符串配置，不存在时返回默认值
func GetStringOptionWithDefault(key string, defaultValue string) string {
	OptionMapRWMutex.RLock()
	defer OptionMapRWMutex.RUnlock()

	if value, exists := OptionMap[key]; exists && value != "" {
		return value
	}
	return defaultValue
}

// GetGrayscaleThreshold 获取灰度阈值
func GetGrayscaleThreshold() int {
	return GetSubscriptionConfig().GrayscaleThreshold
}

// GetGrayscaleMode 获取灰度模式
func GetGrayscaleMode() string {
	return GetSubscriptionConfig().GrayscaleMode
}

// IsSubscriptionEnabledForUser 判断订阅系统是否对指定用户启用
// 该函数综合考虑全局开关、灰度模式和灰度阈值
// 参数:
//   - userID: 用户ID，用于灰度判断
//
// 返回:
//   - enabled: 是否启用
//   - reason: 启用/禁用原因（用于日志和调试）
func IsSubscriptionEnabledForUser(userID int) (enabled bool, reason string) {
	config := GetSubscriptionConfig()

	// 1. 首先检查全局开关
	if config.V2Enabled {
		// 全局开启，直接返回启用
		return true, "global_enabled"
	}

	// 2. 全局关闭时，检查灰度设置
	if config.GrayscaleMode == GrayscaleModeOff || config.GrayscaleThreshold <= 0 {
		// 灰度关闭或阈值为0，返回禁用
		return false, "global_disabled_no_grayscale"
	}

	// 3. 按灰度模式判断
	switch config.GrayscaleMode {
	case GrayscaleModeUserID:
		// 按用户ID灰度: user_id % 100 < threshold
		if userID%100 < config.GrayscaleThreshold {
			return true, "grayscale_user_id_hit"
		}
		return false, "grayscale_user_id_miss"

	case GrayscaleModePercentage:
		// 按百分比灰度: 对于同一用户应该保持一致性
		// 使用 user_id 取模来保证同一用户的判断结果一致
		if userID%100 < config.GrayscaleThreshold {
			return true, "grayscale_percentage_hit"
		}
		return false, "grayscale_percentage_miss"

	default:
		// 未知模式，返回禁用
		return false, "unknown_grayscale_mode"
	}
}

// IsInGrayscale 检查指定用户是否在灰度范围内（不考虑全局开关）
// 这个函数用于纯粹的灰度判断，不受 V2Enabled 影响
func IsInGrayscale(userID int, threshold int, mode string) bool {
	if threshold <= 0 || mode == GrayscaleModeOff {
		return false
	}

	switch mode {
	case GrayscaleModeUserID, GrayscaleModePercentage:
		return userID%100 < threshold
	default:
		return false
	}
}

// GrayscaleStatus 灰度状态信息
type GrayscaleStatus struct {
	V2Enabled          bool   `json:"v2_enabled"`           // 全局开关
	GrayscaleMode      string `json:"grayscale_mode"`       // 灰度模式
	GrayscaleThreshold int    `json:"grayscale_threshold"`  // 灰度阈值
	EffectiveEnabled   bool   `json:"effective_enabled"`    // 对于该用户是否生效
	Reason             string `json:"reason"`               // 原因
}

// GetGrayscaleStatusForUser 获取指定用户的灰度状态详情
func GetGrayscaleStatusForUser(userID int) GrayscaleStatus {
	config := GetSubscriptionConfig()
	enabled, reason := IsSubscriptionEnabledForUser(userID)

	return GrayscaleStatus{
		V2Enabled:          config.V2Enabled,
		GrayscaleMode:      config.GrayscaleMode,
		GrayscaleThreshold: config.GrayscaleThreshold,
		EffectiveEnabled:   enabled,
		Reason:             reason,
	}
}

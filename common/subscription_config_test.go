package common

import (
	"sync"
	"testing"
)

func TestIsSubscriptionEnabledForUser_GlobalEnabled(t *testing.T) {
	// 保存原始配置
	origConfig := GetSubscriptionConfig()
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
	}()

	// 设置全局开启
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          true,
		GrayscaleMode:      GrayscaleModeOff,
		GrayscaleThreshold: 0,
	}
	subscriptionConfigLock.Unlock()

	// 任何用户都应该启用
	for _, userID := range []int{1, 50, 99, 100, 200} {
		enabled, reason := IsSubscriptionEnabledForUser(userID)
		if !enabled {
			t.Errorf("用户 %d: 期望 enabled=true，实际 enabled=%v", userID, enabled)
		}
		if reason != "global_enabled" {
			t.Errorf("用户 %d: 期望 reason=global_enabled，实际 reason=%s", userID, reason)
		}
	}
}

func TestIsSubscriptionEnabledForUser_GlobalDisabled_NoGrayscale(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
	}()

	// 全局关闭，无灰度
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeOff,
		GrayscaleThreshold: 0,
	}
	subscriptionConfigLock.Unlock()

	for _, userID := range []int{1, 50, 99, 100, 200} {
		enabled, reason := IsSubscriptionEnabledForUser(userID)
		if enabled {
			t.Errorf("用户 %d: 期望 enabled=false，实际 enabled=%v", userID, enabled)
		}
		if reason != "global_disabled_no_grayscale" {
			t.Errorf("用户 %d: 期望 reason=global_disabled_no_grayscale，实际 reason=%s", userID, reason)
		}
	}
}

func TestIsSubscriptionEnabledForUser_Grayscale_UserID_Threshold50(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
	}()

	// 全局关闭，user_id 灰度 50%
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeUserID,
		GrayscaleThreshold: 50,
	}
	subscriptionConfigLock.Unlock()

	testCases := []struct {
		userID         int
		expectEnabled  bool
		expectReason   string
	}{
		{0, true, "grayscale_user_id_hit"},      // 0 % 100 = 0 < 50
		{1, true, "grayscale_user_id_hit"},      // 1 % 100 = 1 < 50
		{49, true, "grayscale_user_id_hit"},     // 49 % 100 = 49 < 50
		{50, false, "grayscale_user_id_miss"},   // 50 % 100 = 50 >= 50
		{99, false, "grayscale_user_id_miss"},   // 99 % 100 = 99 >= 50
		{100, true, "grayscale_user_id_hit"},    // 100 % 100 = 0 < 50
		{149, true, "grayscale_user_id_hit"},    // 149 % 100 = 49 < 50
		{150, false, "grayscale_user_id_miss"},  // 150 % 100 = 50 >= 50
	}

	for _, tc := range testCases {
		enabled, reason := IsSubscriptionEnabledForUser(tc.userID)
		if enabled != tc.expectEnabled {
			t.Errorf("用户 %d: 期望 enabled=%v，实际 enabled=%v", tc.userID, tc.expectEnabled, enabled)
		}
		if reason != tc.expectReason {
			t.Errorf("用户 %d: 期望 reason=%s，实际 reason=%s", tc.userID, tc.expectReason, reason)
		}
	}
}

func TestIsSubscriptionEnabledForUser_Grayscale_Percentage_Threshold30(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
	}()

	// 全局关闭，percentage 灰度 30%
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModePercentage,
		GrayscaleThreshold: 30,
	}
	subscriptionConfigLock.Unlock()

	testCases := []struct {
		userID         int
		expectEnabled  bool
		expectReason   string
	}{
		{0, true, "grayscale_percentage_hit"},      // 0 % 100 = 0 < 30
		{29, true, "grayscale_percentage_hit"},     // 29 % 100 = 29 < 30
		{30, false, "grayscale_percentage_miss"},   // 30 % 100 = 30 >= 30
		{99, false, "grayscale_percentage_miss"},   // 99 % 100 = 99 >= 30
		{129, true, "grayscale_percentage_hit"},    // 129 % 100 = 29 < 30
	}

	for _, tc := range testCases {
		enabled, reason := IsSubscriptionEnabledForUser(tc.userID)
		if enabled != tc.expectEnabled {
			t.Errorf("用户 %d: 期望 enabled=%v，实际 enabled=%v", tc.userID, tc.expectEnabled, enabled)
		}
		if reason != tc.expectReason {
			t.Errorf("用户 %d: 期望 reason=%s，实际 reason=%s", tc.userID, tc.expectReason, reason)
		}
	}
}

func TestIsSubscriptionEnabledForUser_Grayscale_BoundaryThreshold(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
	}()

	// 边界测试: threshold = 0
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeUserID,
		GrayscaleThreshold: 0,
	}
	subscriptionConfigLock.Unlock()

	enabled, reason := IsSubscriptionEnabledForUser(0)
	if enabled {
		t.Error("threshold=0 时应该禁用所有用户")
	}
	if reason != "global_disabled_no_grayscale" {
		t.Errorf("期望 reason=global_disabled_no_grayscale，实际 reason=%s", reason)
	}

	// 边界测试: threshold = 100
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeUserID,
		GrayscaleThreshold: 100,
	}
	subscriptionConfigLock.Unlock()

	for _, userID := range []int{0, 50, 99} {
		enabled, reason := IsSubscriptionEnabledForUser(userID)
		if !enabled {
			t.Errorf("threshold=100 时用户 %d 应该启用", userID)
		}
		if reason != "grayscale_user_id_hit" {
			t.Errorf("用户 %d: 期望 reason=grayscale_user_id_hit，实际 reason=%s", userID, reason)
		}
	}
}

func TestIsInGrayscale(t *testing.T) {
	testCases := []struct {
		name      string
		userID    int
		threshold int
		mode      string
		expected  bool
	}{
		{"off mode", 10, 50, GrayscaleModeOff, false},
		{"threshold 0", 10, 0, GrayscaleModeUserID, false},
		{"user_id hit", 10, 50, GrayscaleModeUserID, true},
		{"user_id miss", 60, 50, GrayscaleModeUserID, false},
		{"percentage hit", 25, 30, GrayscaleModePercentage, true},
		{"percentage miss", 35, 30, GrayscaleModePercentage, false},
		{"unknown mode", 10, 50, "unknown", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsInGrayscale(tc.userID, tc.threshold, tc.mode)
			if result != tc.expected {
				t.Errorf("IsInGrayscale(%d, %d, %s): 期望 %v，实际 %v",
					tc.userID, tc.threshold, tc.mode, tc.expected, result)
			}
		})
	}
}

func TestUpdateSubscriptionConfig_TriggersCallback(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	origCallback := featureFlagChangeCallback
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
		callbackLock.Lock()
		featureFlagChangeCallback = origCallback
		callbackLock.Unlock()
	}()

	// 初始化配置
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeOff,
		GrayscaleThreshold: 0,
	}
	subscriptionConfigLock.Unlock()

	// 设置回调捕获
	var callbackCalled bool
	var capturedOldEnabled, capturedNewEnabled, capturedV2EnabledChanged, capturedGrayscaleChanged bool
	var wg sync.WaitGroup
	wg.Add(1)

	RegisterFeatureFlagChangeCallback(func(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool) {
		callbackCalled = true
		capturedOldEnabled = oldEnabled
		capturedNewEnabled = newEnabled
		capturedV2EnabledChanged = v2EnabledChanged
		capturedGrayscaleChanged = grayscaleChanged
		wg.Done()
	})

	// 更新 V2Enabled 从 false 到 true
	UpdateSubscriptionConfig(OptionKeySubscriptionV2Enabled, "true")

	// 等待回调
	wg.Wait()

	if !callbackCalled {
		t.Error("回调应该被调用")
	}
	if capturedOldEnabled {
		t.Error("oldEnabled 应该为 false")
	}
	if !capturedNewEnabled {
		t.Error("newEnabled 应该为 true")
	}
	if !capturedV2EnabledChanged {
		t.Error("v2EnabledChanged 应该为 true")
	}
	if capturedGrayscaleChanged {
		t.Error("grayscaleChanged 应该为 false")
	}
}

func TestUpdateSubscriptionConfig_GrayscaleChange(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	origCallback := featureFlagChangeCallback
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
		callbackLock.Lock()
		featureFlagChangeCallback = origCallback
		callbackLock.Unlock()
	}()

	// 初始化配置
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeOff,
		GrayscaleThreshold: 0,
	}
	subscriptionConfigLock.Unlock()

	var capturedGrayscaleChanged bool
	var wg sync.WaitGroup
	wg.Add(1)

	RegisterFeatureFlagChangeCallback(func(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool) {
		capturedGrayscaleChanged = grayscaleChanged
		wg.Done()
	})

	// 更新灰度阈值
	UpdateSubscriptionConfig(OptionKeySubscriptionGrayscaleThreshold, "50")

	wg.Wait()

	if !capturedGrayscaleChanged {
		t.Error("grayscaleChanged 应该为 true")
	}
}

// TestUpdateSubscriptionConfig_V2EnabledChangeWithGrayscaleActive 测试边界场景：
// 灰度已开启时，切换全局开关应该触发 v2EnabledChanged=true
func TestUpdateSubscriptionConfig_V2EnabledChangeWithGrayscaleActive(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	origCallback := featureFlagChangeCallback
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
		callbackLock.Lock()
		featureFlagChangeCallback = origCallback
		callbackLock.Unlock()
	}()

	// 初始化配置：灰度已开启，全局关闭
	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeUserID,
		GrayscaleThreshold: 50,
	}
	subscriptionConfigLock.Unlock()

	var capturedOldEnabled, capturedNewEnabled, capturedV2EnabledChanged, capturedGrayscaleChanged bool
	var wg sync.WaitGroup
	wg.Add(1)

	RegisterFeatureFlagChangeCallback(func(oldEnabled, newEnabled, v2EnabledChanged, grayscaleChanged bool) {
		capturedOldEnabled = oldEnabled
		capturedNewEnabled = newEnabled
		capturedV2EnabledChanged = v2EnabledChanged
		capturedGrayscaleChanged = grayscaleChanged
		wg.Done()
	})

	// 更新 V2Enabled 从 false 到 true（灰度已开启的情况下）
	UpdateSubscriptionConfig(OptionKeySubscriptionV2Enabled, "true")

	wg.Wait()

	// 关键断言：即使 effectiveEnabled 不变（都是 true），v2EnabledChanged 应该为 true
	if capturedOldEnabled != true {
		t.Error("oldEnabled 应该为 true（因为灰度已开启）")
	}
	if capturedNewEnabled != true {
		t.Error("newEnabled 应该为 true")
	}
	if !capturedV2EnabledChanged {
		t.Error("v2EnabledChanged 应该为 true（全局开关发生了变化）")
	}
	if capturedGrayscaleChanged {
		t.Error("grayscaleChanged 应该为 false（灰度设置未变）")
	}
}

func TestGetGrayscaleStatusForUser(t *testing.T) {
	origConfig := GetSubscriptionConfig()
	defer func() {
		subscriptionConfigLock.Lock()
		subscriptionConfig = origConfig
		subscriptionConfigLock.Unlock()
	}()

	subscriptionConfigLock.Lock()
	subscriptionConfig = SubscriptionConfig{
		V2Enabled:          false,
		GrayscaleMode:      GrayscaleModeUserID,
		GrayscaleThreshold: 50,
	}
	subscriptionConfigLock.Unlock()

	status := GetGrayscaleStatusForUser(25)

	if status.V2Enabled != false {
		t.Error("V2Enabled 应该为 false")
	}
	if status.GrayscaleMode != GrayscaleModeUserID {
		t.Errorf("GrayscaleMode 应该为 %s，实际为 %s", GrayscaleModeUserID, status.GrayscaleMode)
	}
	if status.GrayscaleThreshold != 50 {
		t.Errorf("GrayscaleThreshold 应该为 50，实际为 %d", status.GrayscaleThreshold)
	}
	if !status.EffectiveEnabled {
		t.Error("用户 25 应该被启用（25 % 100 = 25 < 50）")
	}
	if status.Reason != "grayscale_user_id_hit" {
		t.Errorf("Reason 应该为 grayscale_user_id_hit，实际为 %s", status.Reason)
	}
}

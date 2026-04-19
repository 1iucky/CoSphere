package service

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
)

func intPtr(v int) *int    { return &v }
func int64Ptr(v int64) *int64 { return &v }

// ========== ChannelSettings 辅助方法测试 ==========

func TestChannelSettings_HasRPMConfig(t *testing.T) {
	tests := []struct {
		name     string
		settings *dto.ChannelSettings
		want     bool
	}{
		{"nil settings", nil, false},
		{"empty BaseRPM", &dto.ChannelSettings{}, false},
		{"BaseRPM is 0", &dto.ChannelSettings{BaseRPM: intPtr(0)}, false},
		{"BaseRPM is 10", &dto.ChannelSettings{BaseRPM: intPtr(10)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.HasRPMConfig(); got != tt.want {
				t.Errorf("HasRPMConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelSettings_HasConcurrencyConfig(t *testing.T) {
	tests := []struct {
		name     string
		settings *dto.ChannelSettings
		want     bool
	}{
		{"nil settings", nil, false},
		{"empty MaxConcurrency", &dto.ChannelSettings{}, false},
		{"MaxConcurrency is 0", &dto.ChannelSettings{MaxConcurrency: intPtr(0)}, false},
		{"MaxConcurrency is 5", &dto.ChannelSettings{MaxConcurrency: intPtr(5)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.HasConcurrencyConfig(); got != tt.want {
				t.Errorf("HasConcurrencyConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelSettings_HasSessionConfig(t *testing.T) {
	tests := []struct {
		name     string
		settings *dto.ChannelSettings
		want     bool
	}{
		{"nil settings", nil, false},
		{"empty MaxSessions", &dto.ChannelSettings{}, false},
		{"MaxSessions is 10", &dto.ChannelSettings{MaxSessions: intPtr(10)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.HasSessionConfig(); got != tt.want {
				t.Errorf("HasSessionConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelSettings_GetSessionIdleTimeout(t *testing.T) {
	tests := []struct {
		name     string
		settings *dto.ChannelSettings
		want     int
	}{
		{"nil SessionIdleTimeout defaults to 5", &dto.ChannelSettings{}, 5},
		{"zero SessionIdleTimeout defaults to 5", &dto.ChannelSettings{SessionIdleTimeout: intPtr(0)}, 5},
		{"custom value 10", &dto.ChannelSettings{SessionIdleTimeout: intPtr(10)}, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.GetSessionIdleTimeout(); got != tt.want {
				t.Errorf("GetSessionIdleTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelSettings_GetRPMStickyBuffer(t *testing.T) {
	tests := []struct {
		name     string
		settings *dto.ChannelSettings
		want     int
	}{
		{"nil defaults to 3", &dto.ChannelSettings{}, 3},
		{"zero defaults to 3", &dto.ChannelSettings{RPMStickyBuffer: intPtr(0)}, 3},
		{"custom value 5", &dto.ChannelSettings{RPMStickyBuffer: intPtr(5)}, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.settings.GetRPMStickyBuffer(); got != tt.want {
				t.Errorf("GetRPMStickyBuffer() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ========== JSON 序列化/反序列化测试 ==========

func TestChannelSettings_JSONRoundTrip(t *testing.T) {
	original := dto.ChannelSettings{
		ForceFormat:            true,
		Proxy:                  "socks5://localhost:1080",
		BaseRPM:                intPtr(60),
		RPMStickyBuffer:       intPtr(5),
		MaxConcurrency:         intPtr(10),
		MaxSessions:            intPtr(20),
		SessionIdleTimeout:    intPtr(10),
		EnableTLSFingerprint:  true,
		TLSFingerprintProfileID: int64Ptr(1),
		EnableSessionIDMasking: true,
	}

	// 序列化
	jsonBytes, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// 反序列化
	var decoded dto.ChannelSettings
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// 验证各字段
	if decoded.ForceFormat != original.ForceFormat {
		t.Errorf("ForceFormat mismatch: got %v, want %v", decoded.ForceFormat, original.ForceFormat)
	}
	if decoded.Proxy != original.Proxy {
		t.Errorf("Proxy mismatch: got %v, want %v", decoded.Proxy, original.Proxy)
	}
	if *decoded.BaseRPM != *original.BaseRPM {
		t.Errorf("BaseRPM mismatch: got %v, want %v", *decoded.BaseRPM, *original.BaseRPM)
	}
	if *decoded.MaxConcurrency != *original.MaxConcurrency {
		t.Errorf("MaxConcurrency mismatch: got %v, want %v", *decoded.MaxConcurrency, *original.MaxConcurrency)
	}
	if *decoded.MaxSessions != *original.MaxSessions {
		t.Errorf("MaxSessions mismatch: got %v, want %v", *decoded.MaxSessions, *original.MaxSessions)
	}
	if decoded.EnableTLSFingerprint != original.EnableTLSFingerprint {
		t.Errorf("EnableTLSFingerprint mismatch: got %v, want %v", decoded.EnableTLSFingerprint, original.EnableTLSFingerprint)
	}
	if decoded.EnableSessionIDMasking != original.EnableSessionIDMasking {
		t.Errorf("EnableSessionIDMasking mismatch: got %v, want %v", decoded.EnableSessionIDMasking, original.EnableSessionIDMasking)
	}
	if *decoded.TLSFingerprintProfileID != *original.TLSFingerprintProfileID {
		t.Errorf("TLSFingerprintProfileID mismatch: got %v, want %v", *decoded.TLSFingerprintProfileID, *decoded.TLSFingerprintProfileID)
	}
}

func TestChannelSettings_EmptyJSONBackwardCompatible(t *testing.T) {
	// 模拟旧版本数据（不含新字段）
	oldJSON := `{"force_format":true,"proxy":"http://proxy:8080"}`

	var settings dto.ChannelSettings
	if err := json.Unmarshal([]byte(oldJSON), &settings); err != nil {
		t.Fatalf("Unmarshal old JSON failed: %v", err)
	}

	// 旧字段正常
	if !settings.ForceFormat {
		t.Error("ForceFormat should be true")
	}
	if settings.Proxy != "http://proxy:8080" {
		t.Errorf("Proxy mismatch: got %v", settings.Proxy)
	}

	// 新字段为默认值（不触发配额控制）
	if settings.HasRPMConfig() {
		t.Error("HasRPMConfig should be false for old data")
	}
	if settings.HasConcurrencyConfig() {
		t.Error("HasConcurrencyConfig should be false for old data")
	}
	if settings.HasSessionConfig() {
		t.Error("HasSessionConfig should be false for old data")
	}
	if settings.EnableTLSFingerprint {
		t.Error("EnableTLSFingerprint should be false for old data")
	}
	if settings.EnableSessionIDMasking {
		t.Error("EnableSessionIDMasking should be false for old data")
	}
}

package dto

type ChannelSettings struct {
	ForceFormat            bool   `json:"force_format,omitempty"`
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`
	Proxy                  string `json:"proxy"`
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"`
	SystemPrompt           string `json:"system_prompt,omitempty"`
	SystemPromptOverride   bool   `json:"system_prompt_override,omitempty"`

	// 配额控制
	MaxSessions             *int   `json:"max_sessions,omitempty"`                 // 最大会话数（null=不限）
	SessionIdleTimeout      *int   `json:"session_idle_timeout,omitempty"`         // 会话空闲超时（分钟，默认5）
	BaseRPM                 *int   `json:"base_rpm,omitempty"`                     // 每分钟请求限制（null=不限）
	RPMStickyBuffer         *int   `json:"rpm_sticky_buffer,omitempty"`            // 粘性会话RPM缓冲
	EnableTLSFingerprint    bool   `json:"enable_tls_fingerprint,omitempty"`       // 启用TLS指纹模拟
	TLSFingerprintProfileID *int64 `json:"tls_fingerprint_profile_id,omitempty"`  // TLS指纹配置ID（null=使用默认Node.js 24.x指纹）
	EnableSessionIDMasking  bool   `json:"enable_session_id_masking,omitempty"`    // 启用会话ID伪装
	MaxConcurrency          *int   `json:"max_concurrency,omitempty"`              // 最大并发数（null=不限）
}

// HasRPMConfig 是否配置了 RPM 限制
func (s *ChannelSettings) HasRPMConfig() bool {
	return s != nil && s.BaseRPM != nil && *s.BaseRPM > 0
}

// HasConcurrencyConfig 是否配置了并发限制
func (s *ChannelSettings) HasConcurrencyConfig() bool {
	return s != nil && s.MaxConcurrency != nil && *s.MaxConcurrency > 0
}

// HasSessionConfig 是否配置了会话数限制
func (s *ChannelSettings) HasSessionConfig() bool {
	return s != nil && s.MaxSessions != nil && *s.MaxSessions > 0
}

// GetSessionIdleTimeout 获取会话空闲超时（分钟），默认5
func (s *ChannelSettings) GetSessionIdleTimeout() int {
	if s.SessionIdleTimeout != nil && *s.SessionIdleTimeout > 0 {
		return *s.SessionIdleTimeout
	}
	return 5
}

// GetRPMStickyBuffer 获取RPM粘性缓冲，默认3
func (s *ChannelSettings) GetRPMStickyBuffer() int {
	if s.RPMStickyBuffer != nil && *s.RPMStickyBuffer > 0 {
		return *s.RPMStickyBuffer
	}
	return 3
}

type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"
	VertexKeyTypeAPIKey VertexKeyType = "api_key"
)

type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk" // 默认
	AwsKeyTypeApiKey AwsKeyType = "api_key"
)

type ChannelOtherSettings struct {
	AzureResponsesVersion string        `json:"azure_responses_version,omitempty"`
	VertexKeyType         VertexKeyType `json:"vertex_key_type,omitempty"` // "json" or "api_key"
	OpenRouterEnterprise  *bool         `json:"openrouter_enterprise,omitempty"`
	AllowServiceTier      bool          `json:"allow_service_tier,omitempty"`      // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	DisableStore          bool          `json:"disable_store,omitempty"`           // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowSafetyIdentifier bool          `json:"allow_safety_identifier,omitempty"` // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	AwsKeyType            AwsKeyType    `json:"aws_key_type,omitempty"`
}

func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}

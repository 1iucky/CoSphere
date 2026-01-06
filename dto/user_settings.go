package dto

type UserSetting struct {
	NotifyType            string  `json:"notify_type,omitempty"`                    // QuotaWarningType 额度预警类型
	QuotaWarningThreshold float64 `json:"quota_warning_threshold,omitempty"`        // QuotaWarningThreshold 额度预警阈值
	WebhookUrl            string  `json:"webhook_url,omitempty"`                    // WebhookUrl webhook地址
	WebhookSecret         string  `json:"webhook_secret,omitempty"`                 // WebhookSecret webhook密钥
	NotificationEmail     string  `json:"notification_email,omitempty"`             // NotificationEmail 通知邮箱地址
	BarkUrl               string  `json:"bark_url,omitempty"`                       // BarkUrl Bark推送URL
	GotifyUrl             string  `json:"gotify_url,omitempty"`                     // GotifyUrl Gotify服务器地址
	GotifyToken           string  `json:"gotify_token,omitempty"`                   // GotifyToken Gotify应用令牌
	GotifyPriority        int     `json:"gotify_priority"`                          // GotifyPriority Gotify消息优先级
	AcceptUnsetRatioModel bool    `json:"accept_unset_model_ratio_model,omitempty"` // AcceptUnsetRatioModel 是否接受未设置价格的模型
	RecordIpLog           bool    `json:"record_ip_log,omitempty"`                  // 是否记录请求和错误日志IP
	SidebarModules        string  `json:"sidebar_modules,omitempty"`                // SidebarModules 左侧边栏模块配置

	// ===================== 订阅系统相关设置 =====================
	AutoWalletFallback         bool                     `json:"auto_wallet_fallback"`                // 是否启用自动余额兜底（订阅额度用尽时自动使用余额）
	AutoWalletFallbackExplicit *bool                    `json:"auto_wallet_fallback_explicit,omitempty"` // 用户是否显式设置过自动兜底（nil=未设置，继承系统默认；非nil=已显式设置）
	NotificationPreferences    *NotificationPreferences `json:"notification_preferences,omitempty"` // 通知偏好设置
	Language                   string                   `json:"language,omitempty"`                 // 用户语言偏好
	Timezone                   string                   `json:"timezone,omitempty"`                 // 用户时区
}

// NotificationPreferences 通知偏好设置
type NotificationPreferences struct {
	Email bool `json:"email"` // 是否启用邮件通知
	InApp bool `json:"in_app"` // 是否启用站内通知
}

var (
	NotifyTypeEmail   = "email"   // Email 邮件
	NotifyTypeWebhook = "webhook" // Webhook
	NotifyTypeBark    = "bark"    // Bark 推送
	NotifyTypeGotify  = "gotify"  // Gotify 推送
)

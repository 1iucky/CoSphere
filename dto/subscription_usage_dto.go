package dto

// SubscriptionUsageRecordRequest 记录使用量请求
type SubscriptionUsageRecordRequest struct {
	SubscriptionID int64  `json:"subscription_id" binding:"required,min=1"`
	Period         string `json:"period" binding:"required,oneof=five_hours day week month"`
	QuotaUsed      int64  `json:"quota_used" binding:"required,min=1"`
}

// SubscriptionUsageResponse 使用量响应
type SubscriptionUsageResponse struct {
	ID               int64   `json:"id"`
	SubscriptionID   int64   `json:"subscription_id"`
	Period           string  `json:"period"`
	WindowStart      int64   `json:"window_start"`
	WindowEnd        int64   `json:"window_end"`
	UsedQuota        int64   `json:"used_quota"`
	LimitQuota       int64   `json:"limit_quota"`
	RemainingQuota   int64   `json:"remaining_quota"`    // 剩余额度
	UsageRate        float64 `json:"usage_rate"`         // 使用率，百分比（0-100）
	SecondsToRefresh int64   `json:"seconds_to_refresh"` // 距离窗口刷新的秒数
	IsOverLimit      bool    `json:"is_over_limit"`      // 是否超限
	UsedQuotaUSD     float64 `json:"used_quota_usd"`     // 已用额度折算 USD
	LimitQuotaUSD    float64 `json:"limit_quota_usd"`    // 限额折算 USD
	RemainingUSD     float64 `json:"remaining_usd"`      // 剩余额度折算 USD
	CreatedAt        int64   `json:"created_at"`
	UpdatedAt        int64   `json:"updated_at"`
}

// SubscriptionUsageListRequest 使用量列表查询请求
type SubscriptionUsageListRequest struct {
	SubscriptionID *int64 `form:"subscription_id" binding:"omitempty,min=1"`
	Period         string `form:"period" binding:"omitempty,oneof=five_hours day week month"`
	StartTime      *int64 `form:"start_time" binding:"omitempty,min=0"`
	EndTime        *int64 `form:"end_time" binding:"omitempty,min=0"`
	Page           int    `form:"page" binding:"omitempty,min=1"`
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// SubscriptionUsageSummaryResponse 使用量汇总响应
type SubscriptionUsageSummaryResponse struct {
	SubscriptionID int64                               `json:"subscription_id"`
	PlanName       string                              `json:"plan_name"`
	Periods        []SubscriptionUsagePeriodSummary    `json:"periods"`
}

// SubscriptionUsagePeriodSummary 单个周期的使用量汇总
type SubscriptionUsagePeriodSummary struct {
	Period          string  `json:"period"`
	UsedQuota       int64   `json:"used_quota"`
	LimitQuota      int64   `json:"limit_quota"`
	RemainingQuota  int64   `json:"remaining_quota"`  // 剩余额度
	UsageRate       float64 `json:"usage_rate"`       // 使用率（0-100）
	WindowStart     int64   `json:"window_start"`
	WindowEnd       int64   `json:"window_end"`
	SecondsToRefresh int64  `json:"seconds_to_refresh"` // 距离窗口刷新的秒数（倒计时）
	IsOverLimit     bool    `json:"is_over_limit"`
	UsedQuotaUSD    float64 `json:"used_quota_usd"`     // 已用额度折算 USD（展示用）
	LimitQuotaUSD   float64 `json:"limit_quota_usd"`    // 限额折算 USD（展示用）
	RemainingUSD    float64 `json:"remaining_usd"`      // 剩余额度折算 USD（展示用）
}

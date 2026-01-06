package dto

// SubscriptionPlanRequest 创建或更新套餐请求
type SubscriptionPlanRequest struct {
	SKU                 *string `json:"sku" binding:"omitempty,max=64"`
	Name                string  `json:"name" binding:"required,min=1,max=128"`
	Description         string  `json:"description" binding:"omitempty"`
	PriceCents          int64   `json:"price_cents" binding:"required,min=1"` // 价格必须 > 0（单位：分）
	Currency            string  `json:"currency" binding:"omitempty,len=3"`
	BillingCycle        string  `json:"billing_cycle" binding:"required,oneof=monthly yearly custom"` // 计费周期（不含限额周期）
	BillingCycleValue   int     `json:"billing_cycle_value" binding:"omitempty,min=1"`
	AllowWalletFallback *bool   `json:"allow_wallet_fallback" binding:"omitempty"`
	Status              string  `json:"status" binding:"omitempty,oneof=draft active archived"`
	StartAt             *int64  `json:"start_at" binding:"omitempty"`
	EndAt               *int64  `json:"end_at" binding:"omitempty"`
	ModelWhitelist      string  `json:"model_whitelist" binding:"omitempty"`
	ChannelGroups       string  `json:"channel_groups" binding:"omitempty"`
	Extra               string  `json:"extra" binding:"omitempty"`
	Limits              []SubscriptionPlanLimitRequest `json:"limits" binding:"omitempty,dive"`
}

// SubscriptionPlanLimitRequest 套餐限额配置请求
type SubscriptionPlanLimitRequest struct {
	Period         string `json:"period" binding:"required,oneof=five_hours day week month"`
	Quota          int64  `json:"quota" binding:"required,min=1"` // 限额必须 > 0
	Unit           string `json:"unit" binding:"omitempty,oneof=quota tokens requests"`
	Enabled        *bool  `json:"enabled" binding:"omitempty"`
	WindowStrategy string `json:"window_strategy" binding:"omitempty,oneof=rolling fixed natural"` // 支持 natural
}

// SubscriptionPlanResponse 套餐响应（包含限额配置）
type SubscriptionPlanResponse struct {
	ID                  int64                            `json:"id"`
	SKU                 string                           `json:"sku"`
	Name                string                           `json:"name"`
	Description         string                           `json:"description"`
	PriceCents          int64                            `json:"price_cents"`
	Currency            string                           `json:"currency"`
	BillingCycle        string                           `json:"billing_cycle"`
	BillingCycleValue   int                              `json:"billing_cycle_value"`
	AllowWalletFallback bool                             `json:"allow_wallet_fallback"`
	Status              string                           `json:"status"`
	StartAt             int64                            `json:"start_at"`
	EndAt               int64                            `json:"end_at"`
	ModelWhitelist      string                           `json:"model_whitelist"`
	ChannelGroups       string                           `json:"channel_groups"`
	Extra               string                           `json:"extra"`
	CreatedAt           int64                            `json:"created_at"`
	UpdatedAt           int64                            `json:"updated_at"`
	Limits              []SubscriptionPlanLimitResponse  `json:"limits"`
}

// SubscriptionPlanLimitResponse 套餐限额配置响应
type SubscriptionPlanLimitResponse struct {
	ID             int64  `json:"id"`
	PlanID         int64  `json:"plan_id"`
	Period         string `json:"period"`
	Quota          int64  `json:"quota"`
	Unit           string `json:"unit"`
	Enabled        bool   `json:"enabled"`
	WindowStrategy string `json:"window_strategy"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// SubscriptionPlanListRequest 套餐列表查询请求
type SubscriptionPlanListRequest struct {
	Status        string `form:"status" binding:"omitempty,oneof=draft active archived"`
	BillingCycle  string `form:"billing_cycle" binding:"omitempty"`
	MinPrice      *int64 `form:"min_price" binding:"omitempty,min=0"`
	MaxPrice      *int64 `form:"max_price" binding:"omitempty,min=0"`
	Page          int    `form:"page" binding:"omitempty,min=1"`
	PageSize      int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

package dto

// SubscriptionResponse 订阅响应
type SubscriptionResponse struct {
	ID                   int64  `json:"id"`
	UserID               int64  `json:"user_id"`
	PlanID               int64  `json:"plan_id"`
	OrderID              *int64 `json:"order_id"`
	Status               string `json:"status"`
	StartAt              int64  `json:"start_at"`
	EndAt                int64  `json:"end_at"`
	Priority             int    `json:"priority"`
	AutoWalletFallback   bool   `json:"auto_wallet_fallback"`
	BindChannelGroup     string `json:"bind_channel_group"`
	ModelWhitelistCache  string `json:"model_whitelist_cache"`
	RedemptionID         *int64 `json:"redemption_id"`
	CouponID             *int64 `json:"coupon_id"`
	RedeemOption         string `json:"redeem_option"`
	Metadata             string `json:"metadata"`
	CreatedAt            int64  `json:"created_at"`
	UpdatedAt            int64  `json:"updated_at"`

	// 扩展字段（非数据库字段）
	PlanName             string `json:"plan_name"`
	RemainingDays        int64  `json:"remaining_days"`
	IsExpiringSoon       bool   `json:"is_expiring_soon"`
	UsageSummary         []SubscriptionUsagePeriodSummary `json:"usage_summary"`       // 当前窗口使用量摘要
	UsageHistory         []SubscriptionUsageHistoryItem   `json:"usage_history,omitempty"` // 使用量历史记录
}

// SubscriptionUsageHistoryItem 使用量历史记录项
type SubscriptionUsageHistoryItem struct {
	ID          int64   `json:"id"`
	Period      string  `json:"period"`
	UsedQuota   int64   `json:"used_quota"`
	LimitQuota  int64   `json:"limit_quota"`
	UsageRate   float64 `json:"usage_rate"`
	WindowStart int64   `json:"window_start"`
	WindowEnd   int64   `json:"window_end"`
	IsOverLimit bool    `json:"is_over_limit"`
	CreatedAt   int64   `json:"created_at"`
}

// SubscriptionListRequest 订阅列表查询请求
type SubscriptionListRequest struct {
	UserID   *int64 `form:"user_id" binding:"omitempty,min=1"`
	PlanID   *int64 `form:"plan_id" binding:"omitempty,min=1"`
	Status   string `form:"status" binding:"omitempty,oneof=pending active expired cancelled"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// SubscriptionCancelRequest 取消订阅请求
type SubscriptionCancelRequest struct {
	SubscriptionID int64  `json:"subscription_id" binding:"required,min=1"`
	Reason         string `json:"reason" binding:"omitempty,max=500"`
}

// SubscriptionRenewRequest 续费订阅请求
type SubscriptionRenewRequest struct {
	SubscriptionID int64   `json:"subscription_id" binding:"required,min=1"`
	CouponCode     *string `json:"coupon_code" binding:"omitempty,max=64"`
	PaymentChannel string  `json:"payment_channel" binding:"required,oneof=wallet stripe alipay wechat paypal"`
}

// SubscriptionUpdateRequest 更新订阅请求
type SubscriptionUpdateRequest struct {
	AutoWalletFallback *bool   `json:"auto_wallet_fallback" binding:"omitempty"`
	BindChannelGroup   *string `json:"bind_channel_group" binding:"omitempty,max=64"`
	Priority           *int    `json:"priority" binding:"omitempty,min=0"`
}

// UserBillResponse 用户账单响应
type UserBillResponse struct {
	ID                 int64  `json:"id"`
	UserID             int64  `json:"user_id"`
	BillingPeriod      string `json:"billing_period"`
	PeriodStart        int64  `json:"period_start"`
	PeriodEnd          int64  `json:"period_end"`
	TotalUsage         int64  `json:"total_usage"`
	TotalAmount        int64  `json:"total_amount"`
	Currency           string `json:"currency"`
	Status             string `json:"status"`
	PaidAt             *int64 `json:"paid_at"`
	PaymentChannel     string `json:"payment_channel"`
	RefundType         string `json:"refund_type"`         // 手工退款等标记
	ConversionMetadata string `json:"conversion_metadata"` // 兑换折算/不足一天金额等审计信息
	Metadata           string `json:"metadata"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

// UserBillListRequest 账单列表查询请求
type UserBillListRequest struct {
	UserID        *int64 `form:"user_id" binding:"omitempty,min=1"`
	Status        string `form:"status" binding:"omitempty,oneof=pending paid overdue cancelled"`
	BillingPeriod string `form:"billing_period" binding:"omitempty"`
	StartTime     *int64 `form:"start_time" binding:"omitempty,min=0"`
	EndTime       *int64 `form:"end_time" binding:"omitempty,min=0"`
	Page          int    `form:"page" binding:"omitempty,min=1"`
	PageSize      int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// UserBillPayRequest 支付账单请求
type UserBillPayRequest struct {
	BillID         int64  `json:"bill_id" binding:"required,min=1"`
	PaymentChannel string `json:"payment_channel" binding:"required,oneof=wallet stripe alipay wechat paypal"`
	PaymentToken   string `json:"payment_token" binding:"omitempty"`
}

// RedemptionSubscriptionRequest 兑换订阅请求
type RedemptionSubscriptionRequest struct {
	RedemptionKey string `json:"redemption_key" binding:"required"`
	PlanID        int64  `json:"plan_id" binding:"required,min=1"`
	RedeemOption  string `json:"redeem_option" binding:"required,oneof=stack coexist convert"`
	CouponCode    *string `json:"coupon_code" binding:"omitempty,max=64"`
}

// RedemptionSubscriptionResponse 兑换订阅响应
type RedemptionSubscriptionResponse struct {
	SubscriptionID int64  `json:"subscription_id"`
	RedemptionID   int64  `json:"redemption_id"`
	Status         string `json:"status"`
	Message        string `json:"message"`
}

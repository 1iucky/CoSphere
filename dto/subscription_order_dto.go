package dto

// SubscriptionOrderCreateRequest 创建订单请求
type SubscriptionOrderCreateRequest struct {
	PlanID         int64   `json:"plan_id" binding:"required,min=1"`
	CouponCode     *string `json:"coupon_code" binding:"omitempty,max=64"`
	PaymentChannel string  `json:"payment_channel" binding:"required,oneof=wallet alipay wechat stripe paypal"`
	RedeemOption   string  `json:"redeem_option" binding:"omitempty,oneof=stack coexist convert"`
	Metadata       string  `json:"metadata" binding:"omitempty"`
}

// SubscriptionOrderResponse 订单响应
type SubscriptionOrderResponse struct {
	ID              int64  `json:"id"`
	UserID          int64  `json:"user_id"`
	PlanID          int64  `json:"plan_id"`
	PlanSnapshot    string `json:"plan_snapshot"`
	CouponID        *int64 `json:"coupon_id"`
	CouponSnapshot  string `json:"coupon_snapshot"`
	PaymentChannel  string `json:"payment_channel"`
	PriceCents      int64  `json:"price_cents"`
	DiscountCents   int64  `json:"discount_cents"`
	FinalPriceCents int64  `json:"final_price_cents"`
	RedemptionID    *int64 `json:"redemption_id"`
	BillID          *int64 `json:"bill_id"`
	Status          string `json:"status"`
	Metadata        string `json:"metadata"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// SubscriptionOrderListRequest 订单列表查询请求
type SubscriptionOrderListRequest struct {
	UserID         *int64 `form:"user_id" binding:"omitempty,min=1"`
	PlanID         *int64 `form:"plan_id" binding:"omitempty,min=1"`
	Status         string `form:"status" binding:"omitempty,oneof=pending paid cancelled expired failed"`
	PaymentChannel string `form:"payment_channel" binding:"omitempty"`
	StartTime      *int64 `form:"start_time" binding:"omitempty,min=0"`
	EndTime        *int64 `form:"end_time" binding:"omitempty,min=0"`
	Page           int    `form:"page" binding:"omitempty,min=1"`
	PageSize       int    `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// SubscriptionOrderPayRequest 支付订单请求
type SubscriptionOrderPayRequest struct {
	OrderID        int64  `json:"order_id" binding:"required,min=1"`
	PaymentChannel string `json:"payment_channel" binding:"required,oneof=wallet"`
	PaymentToken   string `json:"payment_token" binding:"omitempty"`
}

// SubscriptionOrderCancelRequest 取消订单请求
type SubscriptionOrderCancelRequest struct {
	OrderID int64  `json:"order_id" binding:"required,min=1"`
	Reason  string `json:"reason" binding:"omitempty,max=500"`
}

// SubscriptionOrderPreviewResponse 订单预览响应（下单前计算价格）
type SubscriptionOrderPreviewResponse struct {
	PlanID          int64  `json:"plan_id"`
	PlanName        string `json:"plan_name"`
	PriceCents      int64  `json:"price_cents"`
	DiscountCents   int64  `json:"discount_cents"`
	FinalPriceCents int64  `json:"final_price_cents"`
	Currency        string `json:"currency"`
	CouponApplied   bool   `json:"coupon_applied"`
	CouponCode      string `json:"coupon_code"`
}

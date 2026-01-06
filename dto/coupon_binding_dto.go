package dto

// CouponCreateRequest 创建优惠券请求
type CouponCreateRequest struct {
	// 领取码（必填）：格式 COUPON + YYYYMMDD + 6位随机大写字母数字，如 COUPON20250109AB12CD
	Code              string  `json:"code" binding:"required,min=1,max=64"`
	Name              string  `json:"name" binding:"required,min=1,max=128"`
	Description       string  `json:"description" binding:"omitempty"`
	// 优惠券类型: discount(折扣), full_reduction(满减), instant_reduction(立减)
	Type              string  `json:"type" binding:"required,oneof=discount full_reduction instant_reduction"`
	// 适用范围: wallet(余额充值), subscription(订阅), wallet_subscription(充值+订阅均可)
	Scope             string  `json:"scope" binding:"required,oneof=wallet subscription wallet_subscription"`
	// 折扣值: 百分比(1-100)或金额(分)
	DiscountValue     int64   `json:"discount_value" binding:"required,min=1"`
	// 满减阈值(分): 订单金额需达到此阈值才能使用满减券
	ThresholdAmount   int64   `json:"threshold_amount" binding:"omitempty,min=0"`
	// 币种: 当前固定 CNY(人民币)
	Currency          string  `json:"currency" binding:"required,oneof=CNY"`
	// 适用套餐 IDs (JSON数组，如 [1,2,3]，空数组表示适用所有套餐)
	ApplicablePlanIDs []int64 `json:"applicable_plan_ids" binding:"omitempty"`
	// 总发行量（必填）：>= 1
	TotalCount        int64   `json:"total_count" binding:"required,min=1"`
	// 每用户限领数量（必填）：>= 1
	PerUserLimit      int64   `json:"per_user_limit" binding:"required,min=1"`
	ValidFrom         int64   `json:"valid_from" binding:"required,min=0"`
	ValidTo           int64   `json:"valid_to" binding:"required,gtfield=ValidFrom"`
	Status            string  `json:"status" binding:"omitempty,oneof=active inactive expired"`
	BindUserID        *int64  `json:"bind_user_id" binding:"omitempty,min=1"`

	// 兼容旧字段（可选，会被忽略，由 Type 字段自动推导）
	DiscountType      string  `json:"discount_type" binding:"omitempty,oneof=percentage fixed"`
}

// CouponResponse 优惠券响应
type CouponResponse struct {
	ID                int64   `json:"id"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	// 优惠券类型
	Type              string  `json:"type"`
	// 适用范围
	Scope             string  `json:"scope"`
	// 折扣值
	DiscountValue     int64   `json:"discount_value"`
	// 满减阈值
	ThresholdAmount   int64   `json:"threshold_amount"`
	// 币种
	Currency          string  `json:"currency"`
	// 适用套餐 IDs (JSON 数组，空数组表示适用所有套餐)
	ApplicablePlanIDs []int64 `json:"applicable_plan_ids"`
	TotalCount        int64   `json:"total_count"`
	UsedCount         int64   `json:"used_count"`
	PerUserLimit      int64   `json:"per_user_limit"`
	ValidFrom         int64   `json:"valid_from"`
	ValidTo           int64   `json:"valid_to"`
	Status            string  `json:"status"`
	BindUserID        *int64  `json:"bind_user_id"`
	CreatedBy         int64   `json:"created_by"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`

	// 计算字段
	IsAvailable       bool    `json:"is_available"`
	RemainingCount    int64   `json:"remaining_count"`

	// 兼容旧字段（已废弃，仅用于向后兼容）
	DiscountType      string  `json:"discount_type,omitempty"`
}

// CouponListRequest 优惠券列表查询请求
type CouponListRequest struct {
	// 状态筛选: active(激活), inactive(未激活), expired(已过期)
	Status       string `form:"status" binding:"omitempty,oneof=active inactive expired"`
	// 适用范围筛选
	Scope        string `form:"scope" binding:"omitempty,oneof=wallet subscription wallet_subscription"`
	// 优惠券类型筛选
	Type         string `form:"type" binding:"omitempty,oneof=discount full_reduction instant_reduction"`
	// 关键词搜索（支持 code, name）
	Keyword      string `form:"keyword" binding:"omitempty"`
	BindUserID   *int64 `form:"bind_user_id" binding:"omitempty,min=1"`
	CreatedBy    *int64 `form:"created_by" binding:"omitempty,min=1"`
	Page         int    `form:"page" binding:"omitempty,min=1"`
	PageSize     int    `form:"page_size" binding:"omitempty,min=1,max=100"`

	// 兼容旧字段
	DiscountType string `form:"discount_type" binding:"omitempty,oneof=percentage fixed"`
}

// CouponBindRequest 绑定优惠券请求（兑换码绑定）
type CouponBindRequest struct {
	RedemptionID   int64  `json:"redemption_id" binding:"required,min=1"` // 兑换码 ID
	BoundUserID    *int64 `json:"bound_user_id" binding:"omitempty,min=1"` // 可选：绑定专属用户 ID
}

// CouponValidateRequest 验证优惠券请求
type CouponValidateRequest struct {
	CouponCode string `json:"coupon_code" binding:"required,min=1,max=64"`
	PlanID     *int64 `json:"plan_id" binding:"omitempty,min=1"`
	UserID     int64  `json:"user_id" binding:"required,min=1"`
}

// CouponValidateResponse 验证优惠券响应
type CouponValidateResponse struct {
	IsValid       bool   `json:"is_valid"`
	CouponID      int64  `json:"coupon_id"`
	CouponCode    string `json:"coupon_code"`
	DiscountType  string `json:"discount_type"`
	DiscountValue int64  `json:"discount_value"`
	ErrorMessage  string `json:"error_message"`
}

// CouponUpdateRequest 更新优惠券请求
// 注意：这是部分更新（PATCH 语义），只更新提供的字段
// code 不可修改，total_count 只能增加不能减少
type CouponUpdateRequest struct {
	Name              *string  `json:"name" binding:"omitempty,min=1,max=128"`
	Description       *string  `json:"description" binding:"omitempty"`
	// 优惠券类型
	Type              *string  `json:"type" binding:"omitempty,oneof=discount full_reduction instant_reduction"`
	// 适用范围
	Scope             *string  `json:"scope" binding:"omitempty,oneof=wallet subscription wallet_subscription"`
	// 折扣值
	DiscountValue     *int64   `json:"discount_value" binding:"omitempty,min=1"`
	// 满减阈值
	ThresholdAmount   *int64   `json:"threshold_amount" binding:"omitempty,min=0"`
	// 币种（当前固定 CNY）
	Currency          *string  `json:"currency" binding:"omitempty,oneof=CNY"`
	// 适用套餐 IDs (JSON数组，如 [1,2,3]，空数组表示适用所有套餐)
	ApplicablePlanIDs *[]int64 `json:"applicable_plan_ids" binding:"omitempty"`
	Status            *string  `json:"status" binding:"omitempty,oneof=active inactive expired"`
	// 总库存（>= 1，只能增加不能减少）
	TotalCount        *int64   `json:"total_count" binding:"omitempty,min=1"`
	// 每用户限制（>= 1）
	PerUserLimit      *int64   `json:"per_user_limit" binding:"omitempty,min=1"`
	ValidFrom         *int64   `json:"valid_from" binding:"omitempty,min=0"`
	ValidTo           *int64   `json:"valid_to" binding:"omitempty,min=0"`
	// 绑定用户 ID（专属用户）
	BindUserID        *int64   `json:"bind_user_id" binding:"omitempty,min=1"`
}

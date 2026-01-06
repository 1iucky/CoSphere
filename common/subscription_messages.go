package common

// 订阅错误消息
const (
	MsgSubscriptionNotFound            = "订阅不存在"
	MsgSubscriptionExpired             = "订阅已过期"
	MsgSubscriptionNotActive           = "订阅未激活"
	MsgSubscriptionLimitReached        = "订阅额度已达上限"
	MsgSubscriptionQuotaExhausted      = "订阅额度已用完"
	MsgSubscriptionConflict            = "订阅冲突，同一时间只能有一个生效的订阅"
	MsgSubscriptionCancelled           = "订阅已取消"
	MsgSubscriptionAutoRenewalFail     = "订阅自动续费失败"
	MsgSubscriptionOperationFailed     = "订阅操作失败"
	MsgSubscriptionPriorityConflict    = "订阅优先级冲突"
	MsgSubscriptionInsufficientQuota   = "订阅额度不足"
	MsgSubscriptionPlanExpired         = "订阅套餐已过期"
	MsgSubscriptionPlanInactive        = "订阅套餐未激活"
	MsgSubscriptionMaxLimitReached     = "用户订阅数已达系统上限"
)

// 套餐错误消息
const (
	MsgPlanNotFound        = "套餐不存在"
	MsgPlanNotAvailable    = "套餐不可用"
	MsgPlanNotPublished    = "套餐未发布"
	MsgPlanInvalidStatus   = "套餐状态无效"
	MsgPlanLimitExceeded   = "套餐限制已超出"
	MsgPlanPeriodDuplicate = "套餐周期重复"
	MsgPlanTimeConflict    = "套餐时间冲突"
	MsgPlanSKUDuplicate    = "套餐 SKU 已存在"
	MsgPlanPriceInvalid    = "套餐价格无效"
	MsgPlanOperationFailed = "套餐操作失败"
)

// 优惠券错误消息
const (
	MsgCouponNotFound           = "优惠券不存在"
	MsgCouponExpired            = "优惠券已过期"
	MsgCouponExhausted          = "优惠券已用完"
	MsgCouponInvalidStatus      = "优惠券状态无效"
	MsgCouponAlreadyUsed        = "优惠券已使用"
	MsgCouponBindingLocked      = "优惠券已绑定，无法修改"
	MsgCouponBindingNotFound    = "优惠券绑定关系不存在"
	MsgCouponPlanMismatch       = "优惠券不适用于该套餐"
	MsgCouponUserMismatch       = "优惠券不适用于该用户"
	MsgCouponUsageLimitHit      = "优惠券使用次数已达上限"
	MsgCouponNotApplicable      = "优惠券不适用"
	MsgCouponReserved           = "优惠券已被预留"
	MsgCouponCodeDuplicate      = "优惠券代码已存在"
	MsgCouponOperationFailed    = "优惠券操作失败"
	MsgCouponScopeMismatch      = "优惠券作用域不匹配"
	MsgCouponUserLimitReached   = "用户优惠券使用次数已达上限"
	MsgCouponInsufficientQuota  = "优惠券数量不足"
)

// 订单错误消息
const (
	MsgOrderNotFound        = "订单不存在"
	MsgOrderInvalidStatus   = "订单状态无效"
	MsgOrderAlreadyPaid     = "订单已支付"
	MsgOrderExpired         = "订单已过期"
	MsgOrderPaymentFailed   = "订单支付失败"
	MsgOrderCancelled       = "订单已取消"
	MsgOrderCreationFailed  = "订单创建失败"
	MsgOrderInvalidAmount   = "订单金额无效"
	MsgOrderAlreadyRefunded = "订单已退款"
)

// 使用量错误消息
const (
	MsgUsageRecordFailed    = "使用量记录失败"
	MsgUsageQuotaExceeded   = "使用量已超额"
	MsgUsageInvalidType     = "使用量类型无效"
)

// 账单错误消息
const (
	MsgBillNotFound         = "账单不存在"
	MsgBillGenerationFailed = "账单生成失败"
	MsgBillAlreadyPaid      = "账单已支付"
	MsgBillOperationFailed  = "账单操作失败"
)

// 兑换相关错误消息（扩展）
const (
	MsgRedemptionConflict       = "兑换冲突，已存在有效订阅"
	MsgRedemptionInvalidType    = "兑换类型无效"
	MsgRedemptionExpired        = "兑换码已过期"
	MsgRedemptionUsed           = "兑换码已使用"
	MsgRedemptionInvalid        = "兑换码无效"
	MsgRedemptionNotFound       = "兑换码不存在"
	MsgRedemptionUserMismatch   = "兑换码不属于该用户"
	MsgRedemptionOperationFailed = "兑换操作失败"
)

// 验证和请求错误消息
const (
	MsgInvalidRequestParams   = "请求参数无效"
	MsgInsufficientBalance    = "账户余额不足"
)

// 订阅操作成功消息
const (
	MsgSubscriptionCreated       = "订阅创建成功"
	MsgSubscriptionUpdated       = "订阅更新成功"
	MsgSubscriptionCancelSuccess = "订阅取消成功"
	MsgSubscriptionRenewed       = "订阅续费成功"
)

// 套餐操作成功消息
const (
	MsgPlanCreated = "套餐创建成功"
	MsgPlanUpdated = "套餐更新成功"
	MsgPlanDeleted = "套餐删除成功"
)

// 优惠券操作成功消息
const (
	MsgCouponCreated = "优惠券创建成功"
	MsgCouponUpdated = "优惠券更新成功"
	MsgCouponDeleted = "优惠券删除成功"
	MsgCouponApplied = "优惠券应用成功"
)

// 订单操作成功消息
const (
	MsgOrderCreated      = "订单创建成功"
	MsgOrderPaidSuccess  = "订单支付成功"
	MsgOrderCancelSuccess = "订单取消成功"
)

// 使用量操作成功消息
const (
	MsgUsageRecorded = "使用量记录成功"
)

// 账单操作成功消息
const (
	MsgBillGenerated = "账单生成成功"
	MsgBillPaid      = "账单支付成功"
)

package common

import (
	//"os"
	//"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
)

var StartTime = time.Now().Unix() // unit: second
var Version = "v0.0.0"            // this hard coding will be replaced automatically when building, no need to manually change
var SystemName = "New API"
var Footer = ""
var Logo = ""
var TopUpLink = ""

// var ChatLink = ""
// var ChatLink2 = ""
var QuotaPerUnit = 500 * 1000.0 // $0.002 / 1K tokens
// 保留旧变量以兼容历史逻辑，实际展示由 general_setting.quota_display_type 控制
var DisplayInCurrencyEnabled = true
var DisplayTokenStatEnabled = true
var DrawingEnabled = true
var TaskEnabled = true
var DataExportEnabled = true
var DataExportInterval = 5         // unit: minute
var DataExportDefaultTime = "hour" // unit: minute
var DefaultCollapseSidebar = false // default value of collapse sidebar

// Any options with "Secret", "Token" in its key won't be return by GetOptions

var SessionSecret = uuid.New().String()
var CryptoSecret = uuid.New().String()

var OptionMap map[string]string
var OptionMapRWMutex sync.RWMutex

var ItemsPerPage = 10
var MaxRecentItems = 100

var PasswordLoginEnabled = true
var PasswordRegisterEnabled = true
var EmailVerificationEnabled = false
var GitHubOAuthEnabled = false
var LinuxDOOAuthEnabled = false
var WeChatAuthEnabled = false
var TelegramOAuthEnabled = false
var TurnstileCheckEnabled = false
var RegisterEnabled = true

var EmailDomainRestrictionEnabled = false // 是否启用邮箱域名限制
var EmailAliasRestrictionEnabled = false  // 是否启用邮箱别名限制
var EmailDomainWhitelist = []string{
	"gmail.com",
	"163.com",
	"126.com",
	"qq.com",
	"outlook.com",
	"hotmail.com",
	"icloud.com",
	"yahoo.com",
	"foxmail.com",
}
var EmailLoginAuthServerList = []string{
	"smtp.sendcloud.net",
	"smtp.azurecomm.net",
}

var DebugEnabled bool
var MemoryCacheEnabled bool

var LogConsumeEnabled = true

var SMTPServer = ""
var SMTPPort = 587
var SMTPSSLEnabled = false
var SMTPAccount = ""
var SMTPFrom = ""
var SMTPToken = ""

var GitHubClientId = ""
var GitHubClientSecret = ""
var LinuxDOClientId = ""
var LinuxDOClientSecret = ""
var LinuxDOMinimumTrustLevel = 0

var WeChatServerAddress = ""
var WeChatServerToken = ""
var WeChatAccountQRCodeImageURL = ""

var TurnstileSiteKey = ""
var TurnstileSecretKey = ""

var TelegramBotToken = ""
var TelegramBotName = ""

var QuotaForNewUser = 0
var QuotaForInviter = 0
var QuotaForInvitee = 0
var ChannelDisableThreshold = 5.0
var AutomaticDisableChannelEnabled = false
var AutomaticEnableChannelEnabled = false
var QuotaRemindThreshold = 1000
var PreConsumedQuota = 500

var RetryTimes = 0

//var RootUserEmail = ""

var IsMasterNode bool

var requestInterval int
var RequestInterval time.Duration

var SyncFrequency int // unit is second

var BatchUpdateEnabled = false
var BatchUpdateInterval int

var RelayTimeout int // unit is second

var GeminiSafetySetting string

// https://docs.cohere.com/docs/safety-modes Type; NONE/CONTEXTUAL/STRICT
var CohereSafetySetting string

const (
	RequestIdKey = "X-Oneapi-Request-Id"
)

const (
	RoleGuestUser  = 0
	RoleCommonUser = 1
	RoleAdminUser  = 10
	RoleRootUser   = 100
)

func IsValidateRole(role int) bool {
	return role == RoleGuestUser || role == RoleCommonUser || role == RoleAdminUser || role == RoleRootUser
}

var (
	FileUploadPermission    = RoleGuestUser
	FileDownloadPermission  = RoleGuestUser
	ImageUploadPermission   = RoleGuestUser
	ImageDownloadPermission = RoleGuestUser
)

// All duration's unit is seconds
// Shouldn't larger then RateLimitKeyExpirationDuration
var (
	GlobalApiRateLimitEnable   bool
	GlobalApiRateLimitNum      int
	GlobalApiRateLimitDuration int64

	GlobalWebRateLimitEnable   bool
	GlobalWebRateLimitNum      int
	GlobalWebRateLimitDuration int64

	CriticalRateLimitEnable   bool
	CriticalRateLimitNum            = 20
	CriticalRateLimitDuration int64 = 20 * 60

	UploadRateLimitNum            = 10
	UploadRateLimitDuration int64 = 60

	DownloadRateLimitNum            = 10
	DownloadRateLimitDuration int64 = 60
)

var RateLimitKeyExpirationDuration = 20 * time.Minute

const (
	UserStatusEnabled  = 1 // don't use 0, 0 is the default value!
	UserStatusDisabled = 2 // also don't use 0
)

const (
	TokenStatusEnabled   = 1 // don't use 0, 0 is the default value!
	TokenStatusDisabled  = 2 // also don't use 0
	TokenStatusExpired   = 3
	TokenStatusExhausted = 4
)

const (
	RedemptionCodeStatusEnabled  = 1 // don't use 0, 0 is the default value!
	RedemptionCodeStatusDisabled = 2 // also don't use 0
	RedemptionCodeStatusUsed     = 3 // also don't use 0
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // don't use 0, 0 is the default value!
	ChannelStatusManuallyDisabled = 2 // also don't use 0
	ChannelStatusAutoDisabled     = 3
)

const (
	ModelStatusDisabled = 0 // 禁用
	ModelStatusEnabled  = 1 // 启用（默认值）
)

const (
	TopUpStatusPending = "pending"
	TopUpStatusSuccess = "success"
	TopUpStatusExpired = "expired"
)

// ============================================================================
// 订阅系统常量定义
// ============================================================================

// 订阅状态
const (
	SubscriptionStatusPending   = "pending"   // 待激活
	SubscriptionStatusActive    = "active"    // 激活中
	SubscriptionStatusExpired   = "expired"   // 已过期
	SubscriptionStatusCancelled = "cancelled" // 已取消
)

// 套餐状态
const (
	PlanStatusDraft    = "draft"    // 草稿
	PlanStatusActive   = "active"   // 上架
	PlanStatusArchived = "archived" // 归档（下架）
)

// 计费周期类型
const (
	BillingCycleMonthly    = "monthly"     // 按月（1个月）
	BillingCycleYearly     = "yearly"      // 按年（12个月）
	BillingCycleCustom     = "custom"      // 自定义周期
	BillingCycleFiveHours  = "five_hours"  // 5小时
	BillingCycleDay        = "day"         // 按天
	BillingCycleWeek       = "week"        // 按周
	BillingCycleMonth      = "month"       // 按月（通用）
)

// 限额周期类型（滚动窗口）
const (
	PeriodFiveHours = "five_hours" // 5小时
	PeriodDay       = "day"        // 天
	PeriodWeek      = "week"       // 周
	PeriodMonth     = "month"      // 月

	// 别名（用于 model 层）
	LimitPeriodFiveHours = PeriodFiveHours
	LimitPeriodDay       = PeriodDay
	LimitPeriodWeek      = PeriodWeek
	LimitPeriodMonth     = PeriodMonth
)

// 限额单位
const (
	LimitUnitQuota    = "quota"    // 按额度
	LimitUnitTokens   = "tokens"   // 按 token 数
	LimitUnitRequests = "requests" // 按请求数
)

// 窗口策略
const (
	WindowStrategyRolling = "rolling" // 滚动窗口（从订阅开始时间起算）
	WindowStrategyFixed   = "fixed"   // 固定窗口（自然日/周/月边界）
	WindowStrategyNatural = "fixed"   // 自然窗口（别名，等同于 fixed）
)

// 兑换选项
const (
	RedeemOptionStack   = "stack"   // 叠加（延长时间）
	RedeemOptionCoexist = "coexist" // 共存（新订阅独立生效）
	RedeemOptionConvert = "convert" // 转换（余额转额度）
	RedeemOptionReplace = "replace" // 替换（新订阅替换旧订阅）
	RedeemOptionExtend  = "extend"  // 延期（延长现有订阅时间）
)

// 优惠券状态
const (
	CouponStatusActive   = "active"   // 激活
	CouponStatusInactive = "inactive" // 停用
	CouponStatusExpired  = "expired"  // 已过期
	CouponStatusReserved = "reserved" // 已预留（绑定但未使用，可解绑）
	CouponStatusUsed     = "used"     // 已使用（不可解绑，不可再次使用）
)

// 优惠券作用域
const (
	CouponScopeQuota              = "quota"        // 额度券
	CouponScopePlan               = "plan"         // 套餐券
	CouponScopeSubscription       = "subscription" // 订阅券
	CouponScopeWallet             = "wallet"        // 余额充值
	CouponScopeWalletSubscription = "wallet_subscription" // 充值+订阅均可
)

// 优惠券类型
const (
	CouponTypeDiscount         = "discount"          // 折扣券（百分比）
	CouponTypeFullReduction    = "full_reduction"    // 满减券
	CouponTypeInstantReduction = "instant_reduction" // 立减券
)

// 优惠券绑定状态
const (
	CouponBindingStatusReserved = "reserved" // 预留状态（已绑定但未使用，可解绑）
	CouponBindingStatusLocked   = "locked"   // 已锁定（已兑换使用，不可解绑）
)

// 用户优惠券状态
const (
	UserCouponStatusAvailable = "available" // 可用
	UserCouponStatusLocked    = "locked"    // 已锁定
	UserCouponStatusUsed      = "used"      // 已使用
	UserCouponStatusExpired   = "expired"   // 已过期
	UserCouponStatusInvalid   = "invalid"   // 已作废
)

// 优惠券折扣类型
const (
	DiscountTypePercentage = "percentage" // 百分比折扣（如 20 表示 8折）
	DiscountTypeFixed      = "fixed"      // 固定金额折扣（单位：分）
)

// 订单状态
const (
	OrderStatusPending   = "pending"   // 待支付
	OrderStatusPaid      = "paid"      // 已支付
	OrderStatusCancelled = "cancelled" // 已取消
	OrderStatusExpired   = "expired"   // 已过期
	OrderStatusFailed    = "failed"    // 支付失败
	OrderStatusRefunded  = "refunded"  // 已退款
)

// 支付渠道
const (
	PaymentChannelWallet     = "wallet"     // 余额支付
	PaymentChannelStripe     = "stripe"     // Stripe
	PaymentChannelAlipay     = "alipay"     // 支付宝
	PaymentChannelWechat     = "wechat"     // 微信支付
	PaymentChannelPaypal     = "paypal"     // PayPal
	PaymentChannelRedemption = "redemption" // 兑换码支付
	PaymentChannelFree       = "free"       // 免费（0元订单）
)

// 账单状态
const (
	BillStatusPending   = "pending"   // 待支付
	BillStatusPaid      = "paid"      // 已支付
	BillStatusOverdue   = "overdue"   // 逾期
	BillStatusCancelled = "cancelled" // 已取消
)

// 账单类型
const (
	BillTypeSubscription      = "subscription"       // 订阅购买
	BillTypeSubscriptionRenew = "subscription_renew" // 订阅续费
	BillTypeRefund            = "refund"             // 退款
	BillTypeRecharge          = "recharge"           // 充值
	BillTypeConsume           = "consume"            // 消费
	BillTypeAdjustment        = "adjustment"         // 调整
	BillTypeCouponDiscount    = "coupon_discount"    // 优惠券折扣
)

// 账单来源类型
const (
	BillSourceTypeSubscriptionOrder = "subscription_order" // 订阅订单
	BillSourceTypeRedemption        = "redemption"         // 兑换码
	BillSourceTypeCoupon            = "coupon"             // 优惠券
	BillSourceTypeCouponUsage       = "coupon_usage"       // 优惠券核销
	BillSourceTypeAdmin             = "admin"              // 管理员操作
	BillSourceTypeSystem            = "system"             // 系统操作
)

// 兑换类型（扩展 redemptions.type 字段）
const (
	RedemptionTypeQuota        = "quota"        // 额度兑换（原有逻辑）
	RedemptionTypeSubscription = "subscription" // 订阅兑换（新增）
)

// 货币单位
const (
	CurrencyUSD = "USD" // 美元
	CurrencyCNY = "CNY" // 人民币
	CurrencyEUR = "EUR" // 欧元
)

// 订阅系统配置项默认值（将在 2.1.4 中使用）
const (
	DefaultSubscriptionAutoWalletFallback = false   // 默认不自动兜底
	DefaultSubscriptionExpiryNoticeDays   = 7       // 默认到期前7天提醒
	DefaultSubscriptionMaxPerUser         = 10      // 默认每用户最多10个订阅
	DefaultSubscriptionQuotaLowThreshold  = 0.2     // 默认额度低于20%提醒
	DefaultSubscriptionV2Enabled          = false   // 默认订阅系统关闭
	DefaultSubscriptionGrayscaleThreshold = 0       // 默认灰度阈值0%（关闭灰度）
	DefaultSubscriptionGrayscaleMode      = "off"   // 默认灰度模式：off/percentage/user_id
)

// Token 订阅偏好设置（对应 tokens.subscription_preferred 字段）
// 语义：当用户持有令牌且有有效订阅时，是否优先使用订阅额度进行扣费
// 注意：DDL 中 tokens.subscription_preferred 默认值为 FALSE（即默认不优先使用订阅）
const (
	TokenSubscriptionPreferredEnabled  = true  // 启用订阅优先扣费
	TokenSubscriptionPreferredDisabled = false // 不优先使用订阅，仅使用钱包余额（默认）
)

// 订阅系统配置键（options 表键名）
const (
	OptionKeySubscriptionAutoWalletDefault  = "SUBSCRIPTION_AUTO_WALLET_DEFAULT"  // 自动兜底默认值
	OptionKeySubscriptionExpiryNoticeDays   = "SUBSCRIPTION_EXPIRY_NOTICE_DAYS"   // 到期提醒天数
	OptionKeySubscriptionMaxPerUser         = "SUBSCRIPTION_MAX_PER_USER"         // 每用户最大订阅数
	OptionKeySubscriptionQuotaLowThreshold  = "SUBSCRIPTION_QUOTA_LOW_THRESHOLD"  // 额度低阈值
	OptionKeySubscriptionV2Enabled          = "SUBSCRIPTION_V2_ENABLED"           // 订阅系统启用开关
	OptionKeySubscriptionGrayscaleThreshold = "SUBSCRIPTION_GRAYSCALE_THRESHOLD"  // 灰度发布阈值（0-100）
	OptionKeySubscriptionGrayscaleMode      = "SUBSCRIPTION_GRAYSCALE_MODE"       // 灰度模式：off/percentage/user_id
)

// 灰度模式常量
const (
	GrayscaleModeOff        = "off"        // 关闭灰度（完全由 V2Enabled 控制）
	GrayscaleModePercentage = "percentage" // 按百分比随机灰度
	GrayscaleModeUserID     = "user_id"    // 按用户ID灰度（user_id % 100 < threshold）
)

// 订阅优先级（数字越小优先级越高）
const (
	SubscriptionPriorityHighest = 1  // 最高优先级
	SubscriptionPriorityHigh    = 10
	SubscriptionPriorityNormal  = 50
	SubscriptionPriorityLow     = 90
	SubscriptionPriorityLowest  = 99 // 最低优先级
)

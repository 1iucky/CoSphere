package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

const (
	PaymentMethodStripe = "stripe"
)

var stripeAdaptor = &StripeAdaptor{}

type StripePayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type StripeAdaptor struct {
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if req.Amount < getStripeMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getStripeMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getStripePayMoney(float64(req.Amount), group)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if req.PaymentMethod != PaymentMethodStripe {
		c.JSON(200, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if req.Amount < getStripeMinTopup() {
		c.JSON(200, gin.H{"message": fmt.Sprintf("充值数量不能小于 %d", getStripeMinTopup()), "data": 10})
		return
	}
	if req.Amount > 10000 {
		c.JSON(200, gin.H{"message": "充值数量不能大于 10000", "data": 10})
		return
	}

	id := c.GetInt("id")
	user, _ := model.GetUserById(id, false)
	chargedMoney := GetChargedAmount(float64(req.Amount), *user)

	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	payLink, err := genStripeLink(referenceId, user.StripeCustomer, user.Email, req.Amount)
	if err != nil {
		log.Println("获取Stripe Checkout支付链接失败", err)
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	topUp := &model.TopUp{
		UserId:        id,
		Amount:        req.Amount,
		Money:         chargedMoney,
		TradeNo:       referenceId,
		PaymentMethod: PaymentMethodStripe,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("解析Stripe Webhook参数失败: %v\n", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	endpointSecret := setting.StripeWebhookSecret
	event, err := webhook.ConstructEventWithOptions(payload, signature, endpointSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		log.Printf("Stripe Webhook验签失败: %v\n", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		sessionCompleted(event)
	case stripe.EventTypeCheckoutSessionExpired:
		sessionExpired(event)
	default:
		log.Printf("不支持的Stripe Webhook事件类型: %s\n", event.Type)
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(event stripe.Event) {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "complete" != status {
		log.Println("错误的Stripe Checkout完成状态:", status, ",", referenceId)
		return
	}

	// 获取支付金额用于验证
	amountTotal, _ := strconv.ParseInt(event.GetObjectValue("amount_total"), 10, 64)
	currency := strings.ToUpper(event.GetObjectValue("currency"))

	// 尝试查找订阅订单（通过 trade_no）
	subOrder, err := model.GetSubscriptionOrderByTradeNo(referenceId)
	if err == nil && subOrder != nil {
		// 这是订阅订单支付回调
		handleSubscriptionStripeCompleted(subOrder, referenceId, amountTotal, currency)
		return
	}

	// 充值订单处理（原有逻辑）
	err = model.Recharge(referenceId, customerId)
	if err != nil {
		log.Println(err.Error(), referenceId)
		return
	}

	log.Printf("收到款项：%s, %.2f(%s)", referenceId, float64(amountTotal)/100, currency)
}

// handleSubscriptionStripeCompleted 处理订阅订单的 Stripe 支付完成回调
func handleSubscriptionStripeCompleted(order *model.SubscriptionOrder, referenceId string, amountTotal int64, currency string) {
	// 幂等性检查：订单已支付，直接返回
	if order.Status == common.OrderStatusPaid {
		log.Printf("Stripe 订阅订单已支付，跳过处理: orderId=%d, tradeNo=%s", order.Id, referenceId)
		return
	}

	// 验证订单状态
	if order.Status != common.OrderStatusPending {
		log.Printf("Stripe 订阅订单状态不正确: orderId=%d, status=%s", order.Id, order.Status)
		return
	}

	// 验证币种
	planSnapshot, _ := order.GetPlanSnapshotData()
	if planSnapshot != nil && strings.ToUpper(planSnapshot.Currency) != currency {
		log.Printf("Stripe 订阅订单币种不匹配，拒绝处理: orderId=%d, expected=%s, actual=%s",
			order.Id, planSnapshot.Currency, currency)
		return
	}

	// 验证金额（Stripe 返回的 amount_total 是最小货币单位，即分/cents）
	// 订阅订单禁用 Promotion Codes，但 Stripe 可能添加自动税/手续费
	// 因此只拒绝支付不足的情况，允许支付金额 >= 订单金额（税费只增不减）
	if amountTotal < order.FinalPriceCents {
		log.Printf("Stripe 订阅订单金额不足，拒绝处理: orderId=%d, expected=%d cents, actual=%d cents, tradeNo=%s",
			order.Id, order.FinalPriceCents, amountTotal, referenceId)
		return
	}

	// 金额含税费时，记录实际支付金额到 Metadata 以保证账实一致
	if amountTotal > order.FinalPriceCents {
		log.Printf("Stripe 订阅订单金额含税费: orderId=%d, expected=%d cents, actual=%d cents (差额=%d cents), tradeNo=%s",
			order.Id, order.FinalPriceCents, amountTotal, amountTotal-order.FinalPriceCents, referenceId)

		// 更新订单 Metadata 记录实际支付金额
		metadata := make(map[string]interface{})
		if order.Metadata != nil && *order.Metadata != "" {
			_ = json.Unmarshal([]byte(*order.Metadata), &metadata)
		}
		metadata["actual_paid_cents"] = amountTotal
		metadata["tax_fee_cents"] = amountTotal - order.FinalPriceCents
		metadata["currency"] = currency
		metadataBytes, _ := json.Marshal(metadata)
		metadataStr := string(metadataBytes)
		order.Metadata = &metadataStr
		if err := model.UpdateSubscriptionOrder(order); err != nil {
			log.Printf("Stripe 订阅订单更新 Metadata 失败: orderId=%d, err=%v", order.Id, err)
			// 继续处理，不阻止入账
		}
	}

	// 调用订阅订单服务完成支付处理
	svc := service.GetSubscriptionOrderService()
	result, err := svc.ProcessPaymentInternal(order.UserId, order.Id, common.PaymentChannelStripe, referenceId)
	if err != nil {
		log.Printf("Stripe 订阅订单支付处理失败: orderId=%d, err=%v", order.Id, err)
		return
	}

	if result.Success {
		log.Printf("Stripe 订阅订单支付成功: orderId=%d, userId=%d, tradeNo=%s, amount=%.2f %s",
			order.Id, order.UserId, referenceId, float64(amountTotal)/100, currency)
	} else {
		log.Printf("Stripe 订阅订单支付失败: orderId=%d, errMsg=%s", order.Id, result.ErrorMessage)
	}
}

func sessionExpired(event stripe.Event) {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		log.Println("错误的Stripe Checkout过期状态:", status, ",", referenceId)
		return
	}

	if len(referenceId) == 0 {
		log.Println("未提供支付单号")
		return
	}

	topUp := model.GetTopUpByTradeNo(referenceId)
	if topUp == nil {
		log.Println("充值订单不存在", referenceId)
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		log.Println("充值订单状态错误", referenceId)
	}

	topUp.Status = common.TopUpStatusExpired
	err := topUp.Update()
	if err != nil {
		log.Println("过期充值订单失败", referenceId, ", err:", err.Error())
		return
	}

	log.Println("充值订单已过期", referenceId)
}

func genStripeLink(referenceId string, customerId string, email string, amount int64) (string, error) {
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return "", fmt.Errorf("无效的Stripe API密钥")
	}

	stripe.Key = setting.StripeApiSecret

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(system_setting.ServerAddress + "/console/log"),
		CancelURL:         stripe.String(system_setting.ServerAddress + "/console/topup"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(setting.StripePriceId),
				Quantity: stripe.Int64(amount),
			},
		},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(setting.StripePromotionCodesEnabled),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := session.New(params)
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

// genStripeSubscriptionLink 生成订阅订单的 Stripe Checkout 链接
// 使用动态价格（PriceData.UnitAmount）而非 PriceId * Quantity
// amountCents 是订单金额（美分），直接作为 UnitAmount
// 注意：订阅订单禁用 Promotion Codes，确保金额与订单完全匹配
func genStripeSubscriptionLink(referenceId string, customerId string, email string, amountCents int64, productName string) (string, error) {
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return "", fmt.Errorf("无效的Stripe API密钥")
	}

	stripe.Key = setting.StripeApiSecret

	// 使用动态价格，避免 PriceId * Quantity 的金额倍增问题
	// 订阅订单禁用 Promotion Codes，防止用户使用优惠后金额不匹配
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(system_setting.ServerAddress + "/console/subscription"),
		CancelURL:         stripe.String(system_setting.ServerAddress + "/console/subscription"),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency:   stripe.String("usd"),
					UnitAmount: stripe.Int64(amountCents), // 金额已是美分，直接使用
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(productName),
					},
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(false), // 订阅订单禁用优惠码，确保金额一致
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := session.New(params)
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

func GetChargedAmount(count float64, user model.User) float64 {
	topUpGroupRatio := common.GetTopupGroupRatio(user.Group)
	if topUpGroupRatio == 0 {
		topUpGroupRatio = 1
	}

	return count * topUpGroupRatio
}

func getStripePayMoney(amount float64, group string) float64 {
	originalAmount := amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = amount / common.QuotaPerUnit
	}
	// Using float64 for monetary calculations is acceptable here due to the small amounts involved
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(originalAmount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	payMoney := amount * setting.StripeUnitPrice * topupGroupRatio * discount
	return payMoney
}

func getStripeMinTopup() int64 {
	minTopup := setting.StripeMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minTopup = minTopup * int(common.QuotaPerUnit)
	}
	return int64(minTopup)
}

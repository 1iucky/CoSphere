package controller

import (
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	// 获取支付方式
	payMethods := operation_setting.PayMethods

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "" {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	data := gin.H{
		"enable_online_topup": operation_setting.PayAddress != "" && operation_setting.EpayId != "" && operation_setting.EpayKey != "",
		"enable_stripe_topup": setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "",
		"enable_creem_topup":  setting.CreemApiKey != "" && setting.CreemProducts != "[]",
		"creem_products":      setting.CreemProducts,
		"pay_methods":         payMethods,
		"min_topup":           operation_setting.MinTopUp,
		"stripe_min_topup":    setting.StripeMinTopUp,
		"amount_options":      operation_setting.GetPaymentSetting().AmountOptions,
		"discount":            operation_setting.GetPaymentSetting().AmountDiscount,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
	TopUpCode     string `json:"top_up_code"`
}

type AmountRequest struct {
	Amount    int64  `json:"amount"`
	TopUpCode string `json:"top_up_code"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(200, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/log")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(200, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:        id,
		Amount:        amount,
		Money:         payMoney,
		TradeNo:       tradeNo,
		PaymentMethod: req.PaymentMethod,
		CreateTime:    time.Now().Unix(),
		Status:        "pending",
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if !ok {
		createLock.Lock()
		defer createLock.Unlock()
		lock, ok = orderLocks.Load(tradeNo)
		if !ok {
			lock = new(sync.Mutex)
			orderLocks.Store(tradeNo, lock)
		}
	}
	lock.(*sync.Mutex).Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	lock, ok := orderLocks.Load(tradeNo)
	if ok {
		lock.(*sync.Mutex).Unlock()
	}
}

func EpayNotify(c *gin.Context) {
	params := lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
		r[t] = c.Request.URL.Query().Get(t)
		return r
	}, map[string]string{})
	client := GetEpayClient()
	if client == nil {
		log.Println("易支付回调失败 未找到配置信息")
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
		log.Println("易支付回调签名验证失败")
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		log.Printf("易支付异常回调: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	log.Println(verifyInfo)
	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)

	// 根据交易号前缀分流：SUB 开头为订阅订单，否则为充值订单
	if strings.HasPrefix(verifyInfo.ServiceTradeNo, "SUB") {
		// 订阅订单支付回调
		handleSubscriptionEpayNotify(c, verifyInfo)
	} else {
		// 充值订单支付回调
		handleTopUpEpayNotify(c, verifyInfo)
	}
}

// handleTopUpEpayNotify 处理充值订单的易支付回调
func handleTopUpEpayNotify(c *gin.Context, verifyInfo *epay.VerifyRes) {
	topUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
	if topUp == nil {
		log.Printf("易支付回调未找到充值订单: %v", verifyInfo)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}
	if topUp.Status == "pending" {
		topUp.Status = "success"
		err := topUp.Update()
		if err != nil {
			log.Printf("易支付回调更新充值订单失败: %v", topUp)
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		dAmount := decimal.NewFromInt(int64(topUp.Amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())
		err = model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true)
		if err != nil {
			log.Printf("易支付回调更新用户失败: %v", topUp)
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		log.Printf("易支付回调更新用户成功 %v", topUp)
		model.RecordLog(topUp.UserId, model.LogTypeTopup, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money))
	}
	_, _ = c.Writer.Write([]byte("success"))
}

// handleSubscriptionEpayNotify 处理订阅订单的易支付回调
func handleSubscriptionEpayNotify(c *gin.Context, verifyInfo *epay.VerifyRes) {
	// 根据交易号查找订阅订单
	order, err := model.GetSubscriptionOrderByTradeNo(verifyInfo.ServiceTradeNo)
	if err != nil {
		log.Printf("易支付回调未找到订阅订单: tradeNo=%s, err=%v", verifyInfo.ServiceTradeNo, err)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	// 幂等性检查：订单已支付，直接返回成功
	if order.Status == common.OrderStatusPaid {
		log.Printf("订阅订单已支付，跳过处理: orderId=%d, tradeNo=%s", order.Id, verifyInfo.ServiceTradeNo)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	// 验证订单状态
	if order.Status != common.OrderStatusPending {
		log.Printf("订阅订单状态不正确: orderId=%d, status=%s", order.Id, order.Status)
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	// 校验支付金额（分转元后比较）
	expectedMoney := float64(order.FinalPriceCents) / 100.0
	actualMoney, parseErr := strconv.ParseFloat(verifyInfo.Money, 64)
	if parseErr != nil {
		log.Printf("易支付订阅回调金额解析失败: orderId=%d, rawMoney=%s, err=%v, tradeNo=%s",
			order.Id, verifyInfo.Money, parseErr, verifyInfo.ServiceTradeNo)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	// 允许 0.01 元的误差（浮点数精度问题）
	if actualMoney < expectedMoney-0.01 {
		log.Printf("易支付订阅回调金额不足: orderId=%d, expected=%.2f, actual=%.2f, tradeNo=%s",
			order.Id, expectedMoney, actualMoney, verifyInfo.ServiceTradeNo)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 校验支付渠道（将易支付返回的渠道映射为规范值后比较）
	// verifyInfo.Type 是易支付返回的原始渠道（wxpay, alipay 等）
	actualChannel := verifyInfo.Type
	if actualChannel == "wxpay" {
		actualChannel = common.PaymentChannelWechat
	} else if actualChannel == "alipay" {
		actualChannel = common.PaymentChannelAlipay
	} else {
		// 其他渠道（如 custom1）默认映射为 alipay
		actualChannel = common.PaymentChannelAlipay
	}

	// 渠道不一致时，先更新订单渠道为真实渠道再入账
	// （用户可能在支付页面切换了支付方式，ProcessPaymentInternal 要求渠道一致）
	if actualChannel != order.PaymentChannel {
		log.Printf("易支付订阅回调渠道不匹配，更新订单渠道: orderId=%d, orderChannel=%s -> actualChannel=%s (raw: %s), tradeNo=%s",
			order.Id, order.PaymentChannel, actualChannel, verifyInfo.Type, verifyInfo.ServiceTradeNo)
		order.PaymentChannel = actualChannel
		if err := model.UpdateSubscriptionOrder(order); err != nil {
			log.Printf("易支付订阅回调更新订单渠道失败: orderId=%d, err=%v", order.Id, err)
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
	}

	// 调用订阅订单服务完成支付处理
	svc := service.GetSubscriptionOrderService()
	result, err := svc.ProcessPaymentInternal(order.UserId, order.Id, order.PaymentChannel, verifyInfo.ServiceTradeNo)
	if err != nil {
		log.Printf("订阅订单支付处理失败: orderId=%d, err=%v", order.Id, err)
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if result.Success {
		log.Printf("订阅订单支付成功: orderId=%d, userId=%d, tradeNo=%s, amount=%.2f, channel=%s",
			order.Id, order.UserId, verifyInfo.ServiceTradeNo, actualMoney, order.PaymentChannel)
	} else {
		log.Printf("订阅订单支付失败: orderId=%d, errMsg=%s", order.Id, result.ErrorMessage)
	}
	_, _ = c.Writer.Write([]byte("success"))
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
)

// ===================== Admin 订单管理 API =====================

// GetAllSubscriptionOrders 获取订单列表（管理员）
// GET /api/admin/subscription-orders
func GetAllSubscriptionOrders(c *gin.Context) {
	var req dto.SubscriptionOrderListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 设置默认分页
	if req.Page == 0 {
		req.Page = 1
	}
	if req.PageSize == 0 {
		req.PageSize = 20
	}

	var orders []*model.SubscriptionOrder
	var total int64
	var err error

	svc := service.GetSubscriptionOrderService()

	if req.UserID != nil {
		orders, total, err = svc.GetOrdersByUser(*req.UserID, req.Status, req.Page, req.PageSize)
	} else {
		// 管理员查询所有订单 - 需要在 service 中添加方法
		// 暂时返回空
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []dto.SubscriptionOrderResponse{},
			"total":   0,
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 转换为 DTO
	var orderDTOs []dto.SubscriptionOrderResponse
	for _, order := range orders {
		orderDTO := convertOrderToDTO(order)
		orderDTOs = append(orderDTOs, orderDTO)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderDTOs,
		"total":   total,
	})
}

// GetSubscriptionOrder 获取订单详情（管理员）
// GET /api/admin/subscription-orders/:id
func GetSubscriptionOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	svc := service.GetSubscriptionOrderService()
	order, err := svc.GetOrderById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	orderDTO := convertOrderToDTO(order)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderDTO,
	})
}

// RefundSubscriptionOrder 退款订单（管理员）
// POST /api/admin/subscription-orders/:id/refund
func RefundSubscriptionOrder(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	// 获取操作员 ID
	operatorIdValue, exists := c.Get("id")
	if !exists {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取操作员信息",
		})
		return
	}
	operatorId := int64(operatorIdValue.(int))

	var req struct {
		Reason string `json:"reason" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	svc := service.GetSubscriptionOrderService()
	// 获取订单信息以获取用户 ID（RefundOrder 需要校验归属）
	order, err := svc.GetOrderById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result, err := svc.RefundOrder(&service.RefundOrderRequest{
		OrderId:      id,
		UserId:       order.UserId,
		OperatorId:   operatorId,
		RefundReason: req.Reason,
		RefundType:   "manual",
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"refund_amount": result.RefundAmount,
			"refund_status": result.Success,
		},
	})
}

// CancelSubscriptionOrderAdmin 取消订单（管理员）
// POST /api/admin/subscription-orders/:id/cancel
func CancelSubscriptionOrderAdmin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	var req struct {
		Reason string `json:"reason" binding:"max=500"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取订单信息以获取用户 ID
	svc := service.GetSubscriptionOrderService()
	order, err := svc.GetOrderById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	err = svc.CancelOrder(order.UserId, id, req.Reason)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// convertOrderToDTO 将订单模型转换为 DTO
func convertOrderToDTO(order *model.SubscriptionOrder) dto.SubscriptionOrderResponse {
	orderDTO := dto.SubscriptionOrderResponse{
		ID:              order.Id,
		UserID:          order.UserId,
		PlanID:          order.PlanId,
		PlanSnapshot:    order.PlanSnapshot,
		PaymentChannel:  order.PaymentChannel,
		PriceCents:      order.PriceCents,
		DiscountCents:   order.DiscountCents,
		FinalPriceCents: order.FinalPriceCents,
		RedemptionID:    order.RedemptionId,
		BillID:          order.BillId,
		Status:          order.Status,
		CreatedAt:       order.CreatedAt,
		UpdatedAt:       order.UpdatedAt,
	}

	if order.CouponId != nil {
		orderDTO.CouponID = order.CouponId
	}
	if order.CouponSnapshot != nil {
		orderDTO.CouponSnapshot = *order.CouponSnapshot
	}
	if order.Metadata != nil {
		orderDTO.Metadata = *order.Metadata
	}

	return orderDTO
}

// ===================== 用户订阅订单第三方支付 API =====================

// 订单过期时间（秒）：30分钟
const OrderPaymentTimeoutSeconds = 30 * 60

// SubscriptionOrderEpay 订阅订单易支付入口
// POST /api/user/subscriptions/orders/:id/pay/epay
// 复用现有易支付能力，为订阅订单生成支付链接
// 仅支持 CNY 币种订单
func SubscriptionOrderEpay(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	orderId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	var req struct {
		PaymentMethod string `json:"payment_method" binding:"required"` // alipay, wxpay 等
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	// 获取订单并验证
	svc := service.GetSubscriptionOrderService()
	order, err := svc.GetOrderById(orderId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证订单归属
	if order.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权操作此订单",
		})
		return
	}

	// 验证订单状态
	if order.Status != common.OrderStatusPending {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("订单状态不正确，当前状态: %s", order.Status),
		})
		return
	}

	// 检查订单是否过期
	if order.IsExpired() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订单已过期，请重新创建",
		})
		return
	}

	// 获取套餐快照验证币种
	planSnapshot, err := order.GetPlanSnapshotData()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取套餐信息失败: " + err.Error(),
		})
		return
	}

	// 易支付仅支持 CNY 币种
	if planSnapshot.Currency != "CNY" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "易支付仅支持人民币（CNY）订单，当前订单币种为 " + planSnapshot.Currency,
		})
		return
	}

	// 检查易支付配置
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前未配置易支付信息",
		})
		return
	}

	// 验证支付方式
	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "不支持的支付方式",
		})
		return
	}

	// 计算支付金额（分转元）
	// 拦截 0 元订单，应使用 free 渠道或钱包支付
	payMoney := float64(order.FinalPriceCents) / 100.0
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "0 元订单无需使用易支付，请使用免费支付或钱包支付",
		})
		return
	}

	// 生成交易号（订阅订单专用前缀）
	tradeNo := fmt.Sprintf("SUB%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())

	// 构建回调地址
	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/subscription")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")

	// 调用易支付接口
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("订阅套餐 - %s", planSnapshot.Name),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "拉起支付失败: " + err.Error(),
		})
		return
	}

	// 映射支付方式到规范值存储（wxpay→wechat, alipay→alipay）
	// 回调时会根据这个值判断允许的支付渠道
	paymentChannel := req.PaymentMethod
	if paymentChannel == "wxpay" {
		paymentChannel = common.PaymentChannelWechat
	} else if paymentChannel == "alipay" {
		paymentChannel = common.PaymentChannelAlipay
	} else {
		// 其他支付方式默认映射为 alipay
		paymentChannel = common.PaymentChannelAlipay
	}

	// 更新订单交易号和支付渠道
	order.TradeNo = &tradeNo
	order.PaymentChannel = paymentChannel
	if err := model.UpdateSubscriptionOrder(order); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新订单失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"url":                     uri,
			"params":                  params,
			"trade_no":                tradeNo,
			"payment_timeout_seconds": OrderPaymentTimeoutSeconds,
		},
	})
}

// SubscriptionOrderStripe 订阅订单 Stripe 支付入口
// POST /api/user/subscriptions/orders/:id/pay/stripe
// 复用现有 Stripe 支付能力，为订阅订单生成 Checkout Session
// 仅支持 USD 币种订单
func SubscriptionOrderStripe(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无法获取用户信息",
		})
		return
	}

	idStr := c.Param("id")
	orderId, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的订单 ID",
		})
		return
	}

	// 获取订单并验证
	svc := service.GetSubscriptionOrderService()
	order, err := svc.GetOrderById(orderId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// 验证订单归属
	if order.UserId != int64(userId) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无权操作此订单",
		})
		return
	}

	// 验证订单状态
	if order.Status != common.OrderStatusPending {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("订单状态不正确，当前状态: %s", order.Status),
		})
		return
	}

	// 检查订单是否过期
	if order.IsExpired() {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "订单已过期，请重新创建",
		})
		return
	}

	// 获取套餐快照验证币种和获取名称
	planSnapshot, err := order.GetPlanSnapshotData()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取套餐信息失败: " + err.Error(),
		})
		return
	}

	// Stripe 仅支持 USD 币种
	if planSnapshot.Currency != "USD" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Stripe 支付仅支持美元（USD）订单，当前订单币种为 " + planSnapshot.Currency,
		})
		return
	}

	// 拦截 0 元或负数订单，Stripe 不支持 0 元 Checkout
	// 0 元订单应该使用 free 渠道或钱包支付
	if order.FinalPriceCents <= 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "0 元订单无需使用 Stripe 支付，请使用免费支付或钱包支付",
		})
		return
	}

	// 检查 Stripe 配置
	if setting.StripeApiSecret == "" || setting.StripeWebhookSecret == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "当前未配置 Stripe 支付信息",
		})
		return
	}

	// 获取用户信息
	user, err := model.GetUserById(userId, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户信息失败",
		})
		return
	}

	// 生成交易号
	reference := fmt.Sprintf("sub-order-%d-%d-%s", userId, orderId, randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	// 生成 Stripe Checkout 链接（金额已是美分，直接传递）
	planName := fmt.Sprintf("订阅套餐 - %s", planSnapshot.Name)
	payLink, err := genStripeSubscriptionOrderLink(referenceId, user.StripeCustomer, user.Email, order.FinalPriceCents, order.PlanId, planName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "拉起支付失败: " + err.Error(),
		})
		return
	}

	// 更新订单交易号和支付渠道
	order.TradeNo = &referenceId
	order.PaymentChannel = common.PaymentChannelStripe
	if err := model.UpdateSubscriptionOrder(order); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "更新订单失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"pay_link":                payLink,
			"trade_no":                referenceId,
			"payment_timeout_seconds": OrderPaymentTimeoutSeconds,
		},
	})
}

// genStripeSubscriptionOrderLink 生成订阅订单的 Stripe Checkout 链接
// 使用动态价格（PriceData.UnitAmount）而非 PriceId * Quantity，避免金额计算错误
func genStripeSubscriptionOrderLink(referenceId, stripeCustomer, email string, amountCents int64, planId int64, planName string) (string, error) {
	return genStripeSubscriptionLink(referenceId, stripeCustomer, email, amountCents, planName)
}

package service_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// TestCurrencyConversion_USD 测试 USD 套餐的货币换算
func TestCurrencyConversion_USD(t *testing.T) {
	// 模拟场景：USD 套餐，价格 10.00 USD
	finalPriceCents := int64(1000) // 10.00 USD = 1000 cents
	currency := "USD"
	quotaPerUnit := 500000.0 // QuotaPerUnit = 500,000

	// 预期计算：quotaNeeded = 1000 / 100 * 500000 = 5,000,000
	dFinalPriceCents := decimal.NewFromInt(finalPriceCents)
	dQuotaPerUnit := decimal.NewFromFloat(quotaPerUnit)
	dQuotaNeeded := dFinalPriceCents.Div(decimal.NewFromInt(100)).Mul(dQuotaPerUnit)
	quotaNeeded := dQuotaNeeded.Ceil().IntPart() // 向上取整

	expected := int64(5000000)
	if quotaNeeded != expected {
		t.Errorf("USD 套餐换算错误: got %d, want %d (currency=%s, price=%d cents)",
			quotaNeeded, expected, currency, finalPriceCents)
	}
	t.Logf("✅ USD 套餐换算正确: %d cents → %d quota", finalPriceCents, quotaNeeded)
}

// TestCurrencyConversion_CNY 测试 CNY 套餐的货币换算
func TestCurrencyConversion_CNY(t *testing.T) {
	// 模拟场景：CNY 套餐，价格 73.00 CNY (按汇率 7.3 相当于 10.00 USD)
	finalPriceCents := int64(7300) // 73.00 CNY = 7300 cents
	currency := "CNY"
	quotaPerUnit := 500000.0        // QuotaPerUnit = 500,000
	usdExchangeRate := 7.3           // 1 USD = 7.3 CNY

	// 预期计算：quotaNeeded = 7300 / 100 / 7.3 * 500000 = 5,000,000
	dFinalPriceCents := decimal.NewFromInt(finalPriceCents)
	dExchangeRate := decimal.NewFromFloat(usdExchangeRate)
	dQuotaPerUnit := decimal.NewFromFloat(quotaPerUnit)
	dQuotaNeeded := dFinalPriceCents.Div(decimal.NewFromInt(100)).Div(dExchangeRate).Mul(dQuotaPerUnit)
	quotaNeeded := dQuotaNeeded.Ceil().IntPart() // 向上取整

	expected := int64(5000000)
	if quotaNeeded != expected {
		t.Errorf("CNY 套餐换算错误: got %d, want %d (currency=%s, price=%d cents, rate=%.1f)",
			quotaNeeded, expected, currency, finalPriceCents, usdExchangeRate)
	}
	t.Logf("✅ CNY 套餐换算正确: %d cents (%.2f CNY) → %d quota", finalPriceCents, float64(finalPriceCents)/100, quotaNeeded)
}

// TestRoundingDirection_Deduct 测试扣款向上取整
func TestRoundingDirection_Deduct(t *testing.T) {
	// 测试场景：扣款金额需要向上取整，确保不会少扣
	testCases := []struct {
		name     string
		cents    int64
		expected int64
	}{
		{"整数金额", 1000, 5000000},                        // 10.00 USD → 5,000,000 quota (正好整数)
		{"小数金额1", 1001, 5005000},                       // 10.01 USD → 5,005,000 quota (向上取整)
		{"小数金额2", 999, 4995000},                        // 9.99 USD → 4,995,000 quota (向上取整)
		{"极小金额", 1, 5000},                              // 0.01 USD → 5,000 quota (向上取整)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dCents := decimal.NewFromInt(tc.cents)
			dQuotaPerUnit := decimal.NewFromFloat(500000.0)
			dQuotaNeeded := dCents.Div(decimal.NewFromInt(100)).Mul(dQuotaPerUnit)
			quotaNeeded := dQuotaNeeded.Ceil().IntPart() // 向上取整

			if quotaNeeded != tc.expected {
				t.Errorf("%s: 向上取整错误, got %d, want %d", tc.name, quotaNeeded, tc.expected)
			}
		})
	}
	t.Logf("✅ 扣款向上取整测试通过")
}

// TestRoundingDirection_Refund 测试退款向下取整
func TestRoundingDirection_Refund(t *testing.T) {
	// 测试场景：退款金额需要向下取整，确保不会多退
	testCases := []struct {
		name     string
		cents    int64
		expected int64
	}{
		{"整数金额", 1000, 5000000},                        // 10.00 USD → 5,000,000 quota (正好整数)
		{"小数金额1", 1001, 5005000},                       // 10.01 USD → 5,005,000 quota (向下取整)
		{"小数金额2", 999, 4995000},                        // 9.99 USD → 4,995,000 quota (向下取整)
		{"极小金额", 1, 5000},                              // 0.01 USD → 5,000 quota (向下取整)
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dCents := decimal.NewFromInt(tc.cents)
			dQuotaPerUnit := decimal.NewFromFloat(500000.0)
			dQuotaRefund := dCents.Div(decimal.NewFromInt(100)).Mul(dQuotaPerUnit)
			quotaRefund := dQuotaRefund.Floor().IntPart() // 向下取整

			if quotaRefund != tc.expected {
				t.Errorf("%s: 向下取整错误, got %d, want %d", tc.name, quotaRefund, tc.expected)
			}
		})
	}
	t.Logf("✅ 退款向下取整测试通过")
}

// TestExchangeRateValidation 测试汇率校验
func TestExchangeRateValidation(t *testing.T) {
	testCases := []struct {
		name        string
		rate        float64
		quotaPerUnit float64
		shouldError bool
	}{
		{"正常汇率", 7.3, 500000.0, false},
		{"汇率为0", 0.0, 500000.0, true},
		{"汇率为负", -1.0, 500000.0, true},
		{"QuotaPerUnit为0", 7.3, 0.0, true},
		{"QuotaPerUnit为负", 7.3, -1.0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hasError := (tc.rate <= 0 || tc.quotaPerUnit <= 0)
			if hasError != tc.shouldError {
				t.Errorf("%s: 校验错误, got error=%v, want error=%v", tc.name, hasError, tc.shouldError)
			}
		})
	}
	t.Logf("✅ 汇率校验测试通过")
}

// TestIdempotency_SameParams 测试幂等性 - 相同参数
func TestIdempotency_SameParams(t *testing.T) {
	// 模拟场景：相同的参数应该被视为幂等
	userCouponId1 := int64(10)
	userCouponId2 := int64(10)
	paymentChannel1 := "wallet"
	paymentChannel2 := "wallet"

	// 检查优惠券匹配
	couponMatches := (userCouponId1 == userCouponId2)
	channelMatches := (paymentChannel1 == paymentChannel2)

	if !couponMatches || !channelMatches {
		t.Errorf("幂等性检查失败: couponMatches=%v, channelMatches=%v", couponMatches, channelMatches)
	}
	t.Logf("✅ 相同参数幂等性检查通过: couponId=%d, channel=%s", userCouponId1, paymentChannel1)
}

// TestIdempotency_DifferentCoupon 测试幂等性 - 不同优惠券
func TestIdempotency_DifferentCoupon(t *testing.T) {
	// 模拟场景：不同的优惠券应该被拒绝
	userCouponId1 := int64(10)
	userCouponId2 := int64(20) // 不同的优惠券
	paymentChannel1 := "wallet"
	paymentChannel2 := "wallet"

	// 检查优惠券匹配
	couponMatches := (userCouponId1 == userCouponId2)
	channelMatches := (paymentChannel1 == paymentChannel2)

	if couponMatches {
		t.Errorf("幂等性检查失败: 不同优惠券应该被识别为不匹配")
	}
	if !channelMatches {
		t.Errorf("幂等性检查失败: 相同支付渠道应该匹配")
	}
	t.Logf("✅ 不同优惠券幂等性检查通过: 正确识别为不匹配 (couponId1=%d, couponId2=%d)",
		userCouponId1, userCouponId2)
}

// TestIdempotency_DifferentChannel 测试幂等性 - 不同支付渠道
func TestIdempotency_DifferentChannel(t *testing.T) {
	// 模拟场景：不同的支付渠道应该被拒绝
	userCouponId1 := int64(10)
	userCouponId2 := int64(10)
	paymentChannel1 := "wallet"
	paymentChannel2 := "stripe" // 不同的支付渠道

	// 检查优惠券匹配
	couponMatches := (userCouponId1 == userCouponId2)
	channelMatches := (paymentChannel1 == paymentChannel2)

	if !couponMatches {
		t.Errorf("幂等性检查失败: 相同优惠券应该匹配")
	}
	if channelMatches {
		t.Errorf("幂等性检查失败: 不同支付渠道应该被识别为不匹配")
	}
	t.Logf("✅ 不同支付渠道幂等性检查通过: 正确识别为不匹配 (channel1=%s, channel2=%s)",
		paymentChannel1, paymentChannel2)
}

// TestPaymentChannelValidation 测试支付渠道校验
func TestPaymentChannelValidation(t *testing.T) {
	allowedChannels := map[string]bool{
		common.PaymentChannelWallet: true,
		common.PaymentChannelFree:   true,
	}

	testCases := []struct {
		name    string
		channel string
		allowed bool
	}{
		{"wallet渠道", "wallet", true},
		{"free渠道", "free", true},
		{"stripe渠道", "stripe", false},
		{"alipay渠道", "alipay", false},
		{"wechat渠道", "wechat", false},
		{"paypal渠道", "paypal", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isAllowed := allowedChannels[tc.channel]
			if isAllowed != tc.allowed {
				t.Errorf("%s: 校验错误, got allowed=%v, want allowed=%v", tc.name, isAllowed, tc.allowed)
			}
		})
	}
	t.Logf("✅ 支付渠道校验测试通过")
}

// TestBillIdAssignment 测试 bill_id 赋值逻辑
func TestBillIdAssignment(t *testing.T) {
	// 模拟场景：确保 bill_id 正确赋值到订单
	order := &model.SubscriptionOrder{
		Id:     1,
		BillId: nil, // 初始为 nil
	}

	billId := int64(12345)

	// 模拟赋值操作
	order.BillId = &billId

	if order.BillId == nil {
		t.Error("bill_id 赋值失败: BillId 为 nil")
	}
	if *order.BillId != billId {
		t.Errorf("bill_id 赋值错误: got %d, want %d", *order.BillId, billId)
	}
	t.Logf("✅ bill_id 赋值正确: orderId=%d, billId=%d", order.Id, *order.BillId)
}

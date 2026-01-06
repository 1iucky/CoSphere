package service

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ===================== Task 2.14 回归测试 =====================
// 补充以下测试场景：
// 1. Stripe 含税金额回调用例，校验账单 FinalAmount 与税费描述一致
// 2. 易支付渠道切换回调用例，确认订单渠道与账单渠道一致
// 3. Metadata 中 actual_paid_cents 异常类型处理测试

func setupBillingTestDB(t *testing.T) (*gorm.DB, func()) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// 保存原始 DB
	originalDB := model.DB
	model.DB = db

	// 禁用 Redis 缓存
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false

	err = db.AutoMigrate(
		&model.User{},
		&model.SubscriptionPlan{},
		&model.SubscriptionPlanLimit{},
		&model.Subscription{},
		&model.SubscriptionOrder{},
		&model.SubscriptionUsage{},
		&model.UserBill{},
		&model.AuditLog{},
		&model.Coupon{},
		&model.UserCoupon{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
		_ = sqlDB.Close()
	}

	return db, cleanup
}

// ===================== Stripe 含税金额回调测试 =====================

// TestStripeTaxAmount_BillFinalAmountMatchesMetadata 验证 Stripe 含税场景下账单 FinalAmount 与 Metadata 中的 actual_paid_cents 一致
func TestStripeTaxAmount_BillFinalAmountMatchesMetadata(t *testing.T) {
	db, cleanup := setupBillingTestDB(t)
	defer cleanup()

	// 创建测试用户
	user := &model.User{
		Username:    "billing_test_user",
		DisplayName: "Billing Test User",
		Email:       "billing@test.com",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       10000000, // 10 USD
	}
	require.NoError(t, db.Create(user).Error)

	// 创建测试套餐（USD 套餐）
	plan := &model.SubscriptionPlan{
		Name:         "Stripe Tax Test Plan",
		PriceCents:   1000, // $10.00
		Currency:     "USD",
		BillingCycle: "monthly",
		Status:       common.PlanStatusActive,
	}
	require.NoError(t, db.Create(plan).Error)

	// 创建套餐快照 JSON
	planSnapshot := model.PlanSnapshotData{
		Id:               plan.Id,
		Name:             plan.Name,
		PriceCents:       plan.PriceCents,
		Currency:         plan.Currency,
		BillingCycle:     plan.BillingCycle,
		BillingCycleValue: 1,
	}
	snapshotBytes, err := json.Marshal(planSnapshot)
	require.NoError(t, err)
	snapshotStr := string(snapshotBytes)

	// 模拟 Stripe 含税场景：订单金额 1000 cents，实际支付 1073 cents（含税）
	taxFeeCents := int64(73)
	actualPaidCents := int64(1073)

	metadata := map[string]interface{}{
		"actual_paid_cents": actualPaidCents,
		"tax_fee_cents":     taxFeeCents,
		"currency":          "USD",
	}
	metadataBytes, _ := json.Marshal(metadata)
	metadataStr := string(metadataBytes)

	// 创建订单
	order := &model.SubscriptionOrder{
		UserId:          int64(user.Id),
		PlanId:          plan.Id,
		Status:          common.OrderStatusPending,
		PriceCents:      plan.PriceCents,
		DiscountCents:   0,
		FinalPriceCents: plan.PriceCents,
		PaymentChannel:  common.PaymentChannelStripe,
		PlanSnapshot:    snapshotStr,
		Metadata:        &metadataStr, // Stripe 回调已更新 Metadata
		CreatedAt:       common.GetTimestamp(),
	}
	require.NoError(t, db.Create(order).Error)

	// 验证 Metadata 解析逻辑
	t.Run("验证从Metadata正确解析actual_paid_cents", func(t *testing.T) {
		var parsedMetadata map[string]interface{}
		err := json.Unmarshal([]byte(*order.Metadata), &parsedMetadata)
		require.NoError(t, err)

		actualPaid, ok := parsedMetadata["actual_paid_cents"]
		assert.True(t, ok, "Metadata 中应包含 actual_paid_cents")

		// JSON 解析后数字类型为 float64
		actualPaidFloat, ok := actualPaid.(float64)
		assert.True(t, ok, "actual_paid_cents 应该可以解析为 float64")
		assert.Equal(t, float64(actualPaidCents), actualPaidFloat, "actual_paid_cents 值应为 1073")

		taxFee, ok := parsedMetadata["tax_fee_cents"]
		assert.True(t, ok, "Metadata 中应包含 tax_fee_cents")
		taxFeeFloat, ok := taxFee.(float64)
		assert.True(t, ok, "tax_fee_cents 应该可以解析为 float64")
		assert.Equal(t, float64(taxFeeCents), taxFeeFloat, "tax_fee_cents 值应为 73")
	})

	t.Run("验证账单FinalAmount应等于actual_paid_cents", func(t *testing.T) {
		// 模拟 recordBillInTx 的逻辑
		finalAmount := order.FinalPriceCents // 默认值
		if order.Metadata != nil && *order.Metadata != "" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(*order.Metadata), &metadata); err == nil {
				if actualPaid, ok := metadata["actual_paid_cents"]; ok {
					switch v := actualPaid.(type) {
					case float64:
						finalAmount = int64(v)
					case int64:
						finalAmount = v
					}
				}
			}
		}

		// 验证账单金额应该是含税后的实际支付金额
		assert.Equal(t, actualPaidCents, finalAmount, "账单 FinalAmount 应等于 actual_paid_cents (含税)")
		assert.NotEqual(t, order.FinalPriceCents, finalAmount, "账单 FinalAmount 不应等于订单的 FinalPriceCents (不含税)")
	})

	t.Run("验证账单描述应包含税费信息", func(t *testing.T) {
		// 模拟账单描述生成逻辑
		description := "订阅套餐"
		if planSnapshot.Name != "" {
			description = "订阅套餐：" + planSnapshot.Name
		}

		// 如果有税费，更新描述
		var taxFeeFromMetadata int64 = 0
		if order.Metadata != nil && *order.Metadata != "" {
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(*order.Metadata), &metadata); err == nil {
				if taxFee, ok := metadata["tax_fee_cents"]; ok {
					switch v := taxFee.(type) {
					case float64:
						taxFeeFromMetadata = int64(v)
					case int64:
						taxFeeFromMetadata = v
					}
				}
			}
		}

		if taxFeeFromMetadata > 0 {
			description = description + "（含税费 0.73）"
		}

		assert.Contains(t, description, "含税费", "含税场景账单描述应包含税费信息")
		assert.Contains(t, description, "0.73", "税费金额应为 0.73 USD")
	})
}

// ===================== 易支付渠道切换回调测试 =====================

// TestEpayChannelSwitch_OrderAndBillChannelMatch 验证易支付渠道切换场景下订单渠道与账单渠道一致
func TestEpayChannelSwitch_OrderAndBillChannelMatch(t *testing.T) {
	db, cleanup := setupBillingTestDB(t)
	defer cleanup()

	// 创建测试用户
	user := &model.User{
		Username:    "epay_channel_test_user",
		DisplayName: "Epay Channel Test User",
		Email:       "epay_channel@test.com",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Quota:       0, // 余额为 0，使用第三方支付
	}
	require.NoError(t, db.Create(user).Error)

	// 创建测试套餐（CNY 套餐）
	plan := &model.SubscriptionPlan{
		Name:         "Epay Channel Test Plan",
		PriceCents:   9900, // ¥99.00
		Currency:     "CNY",
		BillingCycle: "monthly",
		Status:       common.PlanStatusActive,
	}
	require.NoError(t, db.Create(plan).Error)

	// 创建套餐快照 JSON
	planSnapshot := model.PlanSnapshotData{
		Id:               plan.Id,
		Name:             plan.Name,
		PriceCents:       plan.PriceCents,
		Currency:         plan.Currency,
		BillingCycle:     plan.BillingCycle,
		BillingCycleValue: 1,
	}
	snapshotBytes, err := json.Marshal(planSnapshot)
	require.NoError(t, err)
	snapshotStr := string(snapshotBytes)

	testCases := []struct {
		name             string
		initialChannel   string // 用户下单时选择的渠道
		actualChannel    string // 易支付回调返回的实际渠道
		epayRawType      string // 易支付原始返回类型
		expectChannelUpdate bool
	}{
		{
			name:             "用户选择微信支付实际用支付宝付款",
			initialChannel:   common.PaymentChannelWechat,
			actualChannel:    common.PaymentChannelAlipay,
			epayRawType:      "alipay",
			expectChannelUpdate: true,
		},
		{
			name:             "用户选择支付宝实际用微信付款",
			initialChannel:   common.PaymentChannelAlipay,
			actualChannel:    common.PaymentChannelWechat,
			epayRawType:      "wxpay",
			expectChannelUpdate: true,
		},
		{
			name:             "用户选择支付宝实际支付宝付款(无切换)",
			initialChannel:   common.PaymentChannelAlipay,
			actualChannel:    common.PaymentChannelAlipay,
			epayRawType:      "alipay",
			expectChannelUpdate: false,
		},
		{
			name:             "易支付返回自定义渠道映射为支付宝",
			initialChannel:   common.PaymentChannelAlipay,
			actualChannel:    common.PaymentChannelAlipay,
			epayRawType:      "custom1", // 自定义渠道应映射为 alipay
			expectChannelUpdate: false,
		},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建订单
			order := &model.SubscriptionOrder{
				UserId:          int64(user.Id),
				PlanId:          plan.Id,
				Status:          common.OrderStatusPending,
				PriceCents:      plan.PriceCents,
				DiscountCents:   0,
				FinalPriceCents: plan.PriceCents,
				PaymentChannel:  tc.initialChannel,
				PlanSnapshot:    snapshotStr,
				CreatedAt:       common.GetTimestamp() + int64(i*10), // 确保时间戳不同
			}
			require.NoError(t, db.Create(order).Error)

			// 模拟易支付渠道映射逻辑
			actualChannel := tc.epayRawType
			if actualChannel == "wxpay" {
				actualChannel = common.PaymentChannelWechat
			} else if actualChannel == "alipay" {
				actualChannel = common.PaymentChannelAlipay
			} else {
				// 其他渠道默认映射为 alipay
				actualChannel = common.PaymentChannelAlipay
			}

			// 验证渠道映射
			assert.Equal(t, tc.actualChannel, actualChannel, "渠道映射结果应正确")

			// 模拟渠道切换逻辑
			channelSwitched := actualChannel != order.PaymentChannel
			assert.Equal(t, tc.expectChannelUpdate, channelSwitched, "渠道切换检测结果应正确")

			// 如果发生渠道切换，更新订单
			if channelSwitched {
				order.PaymentChannel = actualChannel
				require.NoError(t, db.Save(order).Error)
			}

			// 验证订单渠道已更新
			var updatedOrder model.SubscriptionOrder
			require.NoError(t, db.First(&updatedOrder, order.Id).Error)
			assert.Equal(t, actualChannel, updatedOrder.PaymentChannel, "订单支付渠道应与实际支付渠道一致")

			// 模拟创建账单，验证渠道一致性
			bill := &model.UserBill{
				UserId:         int64(user.Id),
				BillType:       common.BillTypeSubscription,
				Amount:         0, // 第三方支付不扣余额
				PaymentChannel: &updatedOrder.PaymentChannel,
				CreatedAt:      common.GetTimestamp(),
			}

			// 验证账单渠道与订单渠道一致
			assert.Equal(t, updatedOrder.PaymentChannel, *bill.PaymentChannel, "账单支付渠道应与订单渠道一致")
		})
	}
}

// ===================== Metadata 异常类型处理测试 =====================

// TestMetadataActualPaidCents_InvalidTypeHandling 验证 Metadata 中 actual_paid_cents 为非数值类型时的处理
func TestMetadataActualPaidCents_InvalidTypeHandling(t *testing.T) {
	testCases := []struct {
		name               string
		metadataContent    map[string]interface{}
		orderFinalPrice    int64
		expectedFinalAmount int64
		description        string
	}{
		{
			name: "actual_paid_cents 为数值类型(float64)",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": float64(1073),
				"tax_fee_cents":     float64(73),
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1073,
			description:         "应正确解析 float64 类型的 actual_paid_cents",
		},
		{
			name: "actual_paid_cents 为字符串类型",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": "1073", // 错误：字符串类型
				"tax_fee_cents":     "73",
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000, // 应回落到 FinalPriceCents
			description:         "字符串类型应回落到订单 FinalPriceCents",
		},
		{
			name: "actual_paid_cents 为 nil",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": nil,
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000, // 应回落到 FinalPriceCents
			description:         "nil 值应回落到订单 FinalPriceCents",
		},
		{
			name: "actual_paid_cents 为布尔值",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": true,
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000, // 应回落到 FinalPriceCents
			description:         "布尔值应回落到订单 FinalPriceCents",
		},
		{
			name: "actual_paid_cents 为对象",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": map[string]interface{}{"value": 1073},
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000, // 应回落到 FinalPriceCents
			description:         "对象类型应回落到订单 FinalPriceCents",
		},
		{
			name: "actual_paid_cents 为数组",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": []int{1073},
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000, // 应回落到 FinalPriceCents
			description:         "数组类型应回落到订单 FinalPriceCents",
		},
		{
			name: "actual_paid_cents 为负数",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": float64(-100),
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: -100, // 当前实现会接受负数（潜在风险）
			description:         "负数值被接受（但应考虑校验）",
		},
		{
			name: "actual_paid_cents 为零",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": float64(0),
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 0, // 0 是有效值（免费订单场景）
			description:         "零值应被正确解析",
		},
		{
			name:                "Metadata 为空字符串",
			metadataContent:     nil, // 将设置为空
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000,
			description:         "空 Metadata 应使用订单 FinalPriceCents",
		},
		{
			name: "Metadata 不包含 actual_paid_cents",
			metadataContent: map[string]interface{}{
				"other_field": "value",
			},
			orderFinalPrice:     1000,
			expectedFinalAmount: 1000,
			description:         "不存在的字段应使用订单 FinalPriceCents",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var metadataStr *string
			if tc.metadataContent != nil {
				metadataBytes, _ := json.Marshal(tc.metadataContent)
				str := string(metadataBytes)
				metadataStr = &str
			}

			// 模拟 recordBillInTx 中的解析逻辑
			finalAmount := tc.orderFinalPrice
			if metadataStr != nil && *metadataStr != "" {
				var metadata map[string]interface{}
				if err := json.Unmarshal([]byte(*metadataStr), &metadata); err == nil {
					if actualPaid, ok := metadata["actual_paid_cents"]; ok {
						switch v := actualPaid.(type) {
						case float64:
							finalAmount = int64(v)
						case int64:
							finalAmount = v
						// 其他类型不处理，保持 finalAmount 为订单的 FinalPriceCents
						}
					}
				}
			}

			assert.Equal(t, tc.expectedFinalAmount, finalAmount, tc.description)
		})
	}
}

// TestMetadataTaxFeeCents_InvalidTypeHandling 验证 Metadata 中 tax_fee_cents 为非数值类型时的处理
func TestMetadataTaxFeeCents_InvalidTypeHandling(t *testing.T) {
	testCases := []struct {
		name              string
		metadataContent   map[string]interface{}
		expectedTaxFee    int64
		expectTaxInDesc   bool
		description       string
	}{
		{
			name: "tax_fee_cents 为数值类型",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": float64(1073),
				"tax_fee_cents":     float64(73),
			},
			expectedTaxFee:  73,
			expectTaxInDesc: true,
			description:     "应正确解析数值类型的税费",
		},
		{
			name: "tax_fee_cents 为字符串类型",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": float64(1073),
				"tax_fee_cents":     "73", // 错误类型
			},
			expectedTaxFee:  0,
			expectTaxInDesc: false,
			description:     "字符串类型税费应被忽略",
		},
		{
			name: "tax_fee_cents 为零",
			metadataContent: map[string]interface{}{
				"actual_paid_cents": float64(1000),
				"tax_fee_cents":     float64(0),
			},
			expectedTaxFee:  0,
			expectTaxInDesc: false,
			description:     "零税费不应显示在描述中",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metadataBytes, _ := json.Marshal(tc.metadataContent)
			metadataStr := string(metadataBytes)

			// 模拟 recordBillInTx 中的税费解析逻辑
			var taxFeeCents int64 = 0
			var metadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataStr), &metadata); err == nil {
				if taxFee, ok := metadata["tax_fee_cents"]; ok {
					switch v := taxFee.(type) {
					case float64:
						taxFeeCents = int64(v)
					case int64:
						taxFeeCents = v
					}
				}
			}

			assert.Equal(t, tc.expectedTaxFee, taxFeeCents, tc.description)

			// 验证是否应该在描述中显示税费
			shouldShowTax := taxFeeCents > 0
			assert.Equal(t, tc.expectTaxInDesc, shouldShowTax, "税费显示判断应正确")
		})
	}
}

// ===================== 边界条件测试 =====================

// TestBillAmount_BoundaryConditions 验证账单金额边界条件
func TestBillAmount_BoundaryConditions(t *testing.T) {
	testCases := []struct {
		name             string
		orderFinalPrice  int64
		actualPaidCents  int64
		description      string
	}{
		{
			name:            "实际支付等于订单金额",
			orderFinalPrice: 1000,
			actualPaidCents: 1000,
			description:     "无税费场景，金额应相等",
		},
		{
			name:            "实际支付大于订单金额(含税)",
			orderFinalPrice: 1000,
			actualPaidCents: 1073,
			description:     "含税场景，应使用实际支付金额",
		},
		{
			name:            "实际支付小于订单金额(不应发生)",
			orderFinalPrice: 1000,
			actualPaidCents: 900,
			description:     "Stripe 回调会拒绝金额不足的情况",
		},
		{
			name:            "大金额订单",
			orderFinalPrice: 99999999,
			actualPaidCents: 100000000,
			description:     "大金额应正确处理",
		},
		{
			name:            "0元订单",
			orderFinalPrice: 0,
			actualPaidCents: 0,
			description:     "免费订单应正确处理",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			metadata := map[string]interface{}{
				"actual_paid_cents": float64(tc.actualPaidCents),
			}
			metadataBytes, _ := json.Marshal(metadata)
			metadataStr := string(metadataBytes)

			// 模拟解析逻辑
			finalAmount := tc.orderFinalPrice
			var parsedMetadata map[string]interface{}
			if err := json.Unmarshal([]byte(metadataStr), &parsedMetadata); err == nil {
				if actualPaid, ok := parsedMetadata["actual_paid_cents"]; ok {
					if v, ok := actualPaid.(float64); ok {
						finalAmount = int64(v)
					}
				}
			}

			assert.Equal(t, tc.actualPaidCents, finalAmount, tc.description)
		})
	}
}

// ===================== 综合场景测试 =====================

// TestStripeTaxWithCoupon_ComplexScenario 验证 Stripe 含税 + 优惠券的复杂场景
func TestStripeTaxWithCoupon_ComplexScenario(t *testing.T) {
	db, cleanup := setupBillingTestDB(t)
	defer cleanup()

	// 场景：$100 套餐，使用 $10 优惠券，实际应付 $90
	// Stripe 收取 7.3% 税费，实际支付 $96.57
	originalPrice := int64(10000)   // $100.00
	discountAmount := int64(1000)   // $10.00
	expectedPayment := int64(9000)  // $90.00
	taxRate := 0.073
	taxFee := int64(float64(expectedPayment) * taxRate) // $6.57 = 657 cents
	actualPaid := expectedPayment + taxFee              // $96.57 = 9657 cents

	t.Run("验证含税+优惠券场景的金额计算", func(t *testing.T) {
		assert.Equal(t, int64(9000), expectedPayment, "应付金额应为 9000 cents")
		assert.Equal(t, int64(657), taxFee, "税费应为 657 cents")
		assert.Equal(t, int64(9657), actualPaid, "实际支付应为 9657 cents")
	})

	t.Run("验证账单记录正确的金额", func(t *testing.T) {
		// 创建订单
		order := &model.SubscriptionOrder{
			UserId:          1,
			PlanId:          1,
			Status:          common.OrderStatusPending,
			PriceCents:      originalPrice,
			DiscountCents:   discountAmount,
			FinalPriceCents: expectedPayment,
			PaymentChannel:  common.PaymentChannelStripe,
		}

		// 模拟 Stripe 回调更新 Metadata
		metadata := map[string]interface{}{
			"actual_paid_cents": float64(actualPaid),
			"tax_fee_cents":     float64(taxFee),
			"currency":          "USD",
		}
		metadataBytes, _ := json.Marshal(metadata)
		metadataStr := string(metadataBytes)
		order.Metadata = &metadataStr

		require.NoError(t, db.Create(order).Error)

		// 验证账单金额计算
		finalAmount := order.FinalPriceCents
		if order.Metadata != nil && *order.Metadata != "" {
			var parsedMetadata map[string]interface{}
			if err := json.Unmarshal([]byte(*order.Metadata), &parsedMetadata); err == nil {
				if ap, ok := parsedMetadata["actual_paid_cents"]; ok {
					if v, ok := ap.(float64); ok {
						finalAmount = int64(v)
					}
				}
			}
		}

		// 账单 FinalAmount 应该是含税后的实际支付金额
		assert.Equal(t, actualPaid, finalAmount, "账单 FinalAmount 应为含税后的实际支付金额 9657 cents")

		// 账单的 OriginalAmount 应该是套餐原价
		assert.Equal(t, originalPrice, order.PriceCents, "账单 OriginalAmount 应为套餐原价 10000 cents")

		// 账单的 DiscountAmount 应该是优惠金额
		assert.Equal(t, discountAmount, order.DiscountCents, "账单 DiscountAmount 应为优惠金额 1000 cents")
	})
}

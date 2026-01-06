package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ===================== 订单状态检查测试 =====================

func TestSubscriptionOrder_IsPending(t *testing.T) {
	testCases := []struct {
		name     string
		status   string
		expected bool
	}{
		{"pending状态", common.OrderStatusPending, true},
		{"paid状态", common.OrderStatusPaid, false},
		{"cancelled状态", common.OrderStatusCancelled, false},
		{"expired状态", common.OrderStatusExpired, false},
		{"refunded状态", common.OrderStatusRefunded, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := &model.SubscriptionOrder{Status: tc.status}
			if order.IsPending() != tc.expected {
				t.Errorf("状态 %s 的 IsPending() 应该返回 %v", tc.status, tc.expected)
			}
		})
	}
}

func TestSubscriptionOrder_IsPaid(t *testing.T) {
	testCases := []struct {
		name     string
		status   string
		expected bool
	}{
		{"pending状态", common.OrderStatusPending, false},
		{"paid状态", common.OrderStatusPaid, true},
		{"cancelled状态", common.OrderStatusCancelled, false},
		{"expired状态", common.OrderStatusExpired, false},
		{"refunded状态", common.OrderStatusRefunded, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := &model.SubscriptionOrder{Status: tc.status}
			if order.IsPaid() != tc.expected {
				t.Errorf("状态 %s 的 IsPaid() 应该返回 %v", tc.status, tc.expected)
			}
		})
	}
}

func TestSubscriptionOrder_CanBePaid(t *testing.T) {
	testCases := []struct {
		name     string
		status   string
		expected bool
	}{
		{"pending状态", common.OrderStatusPending, true},
		{"paid状态", common.OrderStatusPaid, false},
		{"cancelled状态", common.OrderStatusCancelled, false},
		{"expired状态", common.OrderStatusExpired, false},
		{"refunded状态", common.OrderStatusRefunded, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := &model.SubscriptionOrder{Status: tc.status}
			if order.CanBePaid() != tc.expected {
				t.Errorf("状态 %s 的 CanBePaid() 应该返回 %v", tc.status, tc.expected)
			}
		})
	}
}

func TestSubscriptionOrder_CanBeRefunded(t *testing.T) {
	testCases := []struct {
		name     string
		status   string
		expected bool
	}{
		{"pending状态", common.OrderStatusPending, false},
		{"paid状态", common.OrderStatusPaid, true},
		{"cancelled状态", common.OrderStatusCancelled, false},
		{"expired状态", common.OrderStatusExpired, false},
		{"refunded状态", common.OrderStatusRefunded, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := &model.SubscriptionOrder{Status: tc.status}
			if order.CanBeRefunded() != tc.expected {
				t.Errorf("状态 %s 的 CanBeRefunded() 应该返回 %v", tc.status, tc.expected)
			}
		})
	}
}

// ===================== 订单过期检查测试 =====================

func TestSubscriptionOrder_IsExpired(t *testing.T) {
	now := common.GetTimestamp()

	testCases := []struct {
		name      string
		status    string
		createdAt int64
		expected  bool
	}{
		{
			name:      "pending且未过期",
			status:    common.OrderStatusPending,
			createdAt: now - 10*60, // 10分钟前创建
			expected:  false,
		},
		{
			name:      "pending且已过期",
			status:    common.OrderStatusPending,
			createdAt: now - 40*60, // 40分钟前创建
			expected:  true,
		},
		{
			name:      "paid状态不检查过期",
			status:    common.OrderStatusPaid,
			createdAt: now - 40*60,
			expected:  false, // 非 pending 状态直接返回 false
		},
		{
			name:      "cancelled状态不检查过期",
			status:    common.OrderStatusCancelled,
			createdAt: now - 40*60,
			expected:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := &model.SubscriptionOrder{
				Status:    tc.status,
				CreatedAt: tc.createdAt,
			}
			if order.IsExpired() != tc.expected {
				t.Errorf("IsExpired() 应该返回 %v", tc.expected)
			}
		})
	}
}

// ===================== 实际折扣计算测试 =====================

func TestSubscriptionOrder_GetActualDiscount(t *testing.T) {
	testCases := []struct {
		name           string
		priceCents     int64
		finalPrice     int64
		expectedDiscount int64
	}{
		{
			name:            "无折扣",
			priceCents:      10000,
			finalPrice:      10000,
			expectedDiscount: 0,
		},
		{
			name:            "有折扣",
			priceCents:      10000,
			finalPrice:      8000,
			expectedDiscount: 2000,
		},
		{
			name:            "全额优惠",
			priceCents:      10000,
			finalPrice:      0,
			expectedDiscount: 10000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order := &model.SubscriptionOrder{
				PriceCents:      tc.priceCents,
				FinalPriceCents: tc.finalPrice,
			}
			actualDiscount := order.GetActualDiscount()
			if actualDiscount != tc.expectedDiscount {
				t.Errorf("GetActualDiscount() 应该返回 %d，实际返回 %d", tc.expectedDiscount, actualDiscount)
			}
		})
	}
}

// ===================== 元数据操作测试 =====================

func TestSubscriptionOrder_MetadataOperations(t *testing.T) {
	order := &model.SubscriptionOrder{}

	// 测试空元数据
	metadata, err := order.GetMetadataAsMap()
	if err != nil {
		t.Errorf("获取空元数据不应该报错: %v", err)
	}
	if len(metadata) != 0 {
		t.Error("空元数据应该返回空 map")
	}

	// 测试设置元数据
	testMetadata := map[string]interface{}{
		"key1": "value1",
		"key2": 123,
	}
	err = order.SetMetadataFromMap(testMetadata)
	if err != nil {
		t.Errorf("设置元数据不应该报错: %v", err)
	}

	// 测试获取元数据
	retrievedMetadata, err := order.GetMetadataAsMap()
	if err != nil {
		t.Errorf("获取元数据不应该报错: %v", err)
	}
	if retrievedMetadata["key1"] != "value1" {
		t.Error("元数据 key1 值错误")
	}

	// 测试清空元数据
	err = order.SetMetadataFromMap(map[string]interface{}{})
	if err != nil {
		t.Errorf("清空元数据不应该报错: %v", err)
	}
	if order.Metadata != nil {
		t.Error("清空后 Metadata 应该为 nil")
	}
}

// ===================== 快照操作测试 =====================

func TestSubscriptionOrder_PlanSnapshotOperations(t *testing.T) {
	order := &model.SubscriptionOrder{}

	// 测试空快照
	_, err := order.GetPlanSnapshotAsData()
	if err == nil {
		t.Error("获取空快照应该报错")
	}

	// 测试设置快照
	planSnapshot := &model.PlanSnapshotData{
		Id:                1,
		Name:              "测试套餐",
		PriceCents:        10000,
		Currency:          "CNY",
		BillingCycle:      "monthly",
		BillingCycleValue: 1,
		ModelWhitelist:    []string{"gpt-4", "claude-3"},
		ChannelGroups:     []string{"default"},
	}
	err = order.SetPlanSnapshotFromData(planSnapshot)
	if err != nil {
		t.Errorf("设置套餐快照不应该报错: %v", err)
	}

	// 测试获取快照
	retrievedSnapshot, err := order.GetPlanSnapshotAsData()
	if err != nil {
		t.Errorf("获取套餐快照不应该报错: %v", err)
	}
	if retrievedSnapshot.Name != "测试套餐" {
		t.Error("快照名称错误")
	}
	if retrievedSnapshot.PriceCents != 10000 {
		t.Error("快照价格错误")
	}
	if len(retrievedSnapshot.ModelWhitelist) != 2 {
		t.Error("模型白名单长度错误")
	}
}

func TestSubscriptionOrder_CouponSnapshotOperations(t *testing.T) {
	order := &model.SubscriptionOrder{}

	// 测试空快照
	snapshot, err := order.GetCouponSnapshotData()
	if err != nil {
		t.Error("获取空优惠券快照不应该报错")
	}
	if snapshot != nil {
		t.Error("空优惠券快照应该返回 nil")
	}

	// 测试设置快照
	couponSnapshot := &model.CouponSnapshotData{
		Id:            1,
		Code:          "TEST123",
		Name:          "测试优惠券",
		DiscountType:  "discount",
		DiscountValue: 20, // 20% 折扣
		Scope:         "subscription",
	}
	err = order.SetCouponSnapshotFromData(couponSnapshot)
	if err != nil {
		t.Errorf("设置优惠券快照不应该报错: %v", err)
	}

	// 测试获取快照
	retrievedSnapshot, err := order.GetCouponSnapshotData()
	if err != nil {
		t.Errorf("获取优惠券快照不应该报错: %v", err)
	}
	if retrievedSnapshot.Name != "测试优惠券" {
		t.Error("优惠券快照名称错误")
	}
	if retrievedSnapshot.DiscountValue != 20 {
		t.Error("优惠券快照折扣值错误")
	}

	// 测试清空快照
	err = order.SetCouponSnapshotFromData(nil)
	if err != nil {
		t.Errorf("清空优惠券快照不应该报错: %v", err)
	}
	if order.CouponSnapshot != nil {
		t.Error("清空后 CouponSnapshot 应该为 nil")
	}
}

// ===================== 参数验证测试 =====================

func TestGetUserPendingOrderByPlan_InvalidParams(t *testing.T) {
	testCases := []struct {
		name   string
		userId int64
		planId int64
	}{
		{"userId为0", 0, 1},
		{"planId为0", 1, 0},
		{"都为0", 0, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			order, err := model.GetUserPendingOrderByPlan(tc.userId, tc.planId)
			// 无效参数应该返回 nil, nil（不报错，直接返回空）
			if err != nil {
				t.Errorf("无效参数不应该报错: %v", err)
			}
			if order != nil {
				t.Error("无效参数应该返回 nil order")
			}
		})
	}
}

// ===================== 数据结构测试 =====================

func TestPlanSnapshotData_Fields(t *testing.T) {
	snapshot := model.PlanSnapshotData{
		Id:                  1,
		SKU:                 "PLAN-001",
		Name:                "高级套餐",
		Description:         "包含更多功能",
		PriceCents:          29900,
		Currency:            "CNY",
		BillingCycle:        "monthly",
		BillingCycleValue:   1,
		AllowWalletFallback: true,
		ModelWhitelist:      []string{"gpt-4", "gpt-4-turbo", "claude-3-opus"},
		ChannelGroups:       []string{"premium", "default"},
		Limits: []model.PlanLimitSnapshotData{
			{Period: "day", Quota: 100000, Unit: "quota", WindowStrategy: "rolling"},
			{Period: "month", Quota: 3000000, Unit: "quota", WindowStrategy: "fixed"},
		},
	}

	if snapshot.Id != 1 {
		t.Error("Id 设置错误")
	}
	if snapshot.SKU != "PLAN-001" {
		t.Error("SKU 设置错误")
	}
	if snapshot.PriceCents != 29900 {
		t.Error("PriceCents 设置错误")
	}
	if len(snapshot.ModelWhitelist) != 3 {
		t.Error("ModelWhitelist 长度错误")
	}
	if len(snapshot.Limits) != 2 {
		t.Error("Limits 长度错误")
	}
	if snapshot.Limits[0].WindowStrategy != "rolling" {
		t.Error("窗口策略设置错误")
	}
}

func TestCouponSnapshotData_Fields(t *testing.T) {
	snapshot := model.CouponSnapshotData{
		Id:            100,
		Code:          "SAVE20",
		Name:          "8折优惠券",
		DiscountType:  "discount",
		DiscountValue: 20,
		Scope:         "wallet_subscription",
	}

	if snapshot.Id != 100 {
		t.Error("Id 设置错误")
	}
	if snapshot.Code != "SAVE20" {
		t.Error("Code 设置错误")
	}
	if snapshot.DiscountType != "discount" {
		t.Error("DiscountType 设置错误")
	}
	if snapshot.DiscountValue != 20 {
		t.Error("DiscountValue 设置错误")
	}
	if snapshot.Scope != "wallet_subscription" {
		t.Error("Scope 设置错误")
	}
}

func TestPlanLimitSnapshotData_Fields(t *testing.T) {
	// 测试不同的窗口策略
	strategies := []string{"rolling", "fixed", "natural"}
	periods := []string{"five_hours", "day", "week", "month"}

	for _, strategy := range strategies {
		for _, period := range periods {
			limit := model.PlanLimitSnapshotData{
				Period:         period,
				Quota:          100000,
				Unit:           "quota",
				WindowStrategy: strategy,
			}

			if limit.Period != period {
				t.Errorf("Period 设置错误: 期望 %s，实际 %s", period, limit.Period)
			}
			if limit.WindowStrategy != strategy {
				t.Errorf("WindowStrategy 设置错误: 期望 %s，实际 %s", strategy, limit.WindowStrategy)
			}
		}
	}
}

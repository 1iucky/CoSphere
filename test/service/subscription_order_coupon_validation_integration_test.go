package service_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSubscriptionOrderTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

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

	model.DB = db

	if err := db.AutoMigrate(
		&model.User{},
		&model.SubscriptionPlan{},
		&model.SubscriptionPlanLimit{},
		&model.Coupon{},
		&model.UserCoupon{},
		&model.SubscriptionOrder{},
		&model.Subscription{},
		&model.SubscriptionUsage{},
		&model.UserBill{},
		&model.AuditLog{},
	); err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, cleanup
}

func createTestUserForOrders(t *testing.T, db *gorm.DB, userId int, quota int) *model.User {
	t.Helper()

	user := &model.User{
		Id:          userId,
		Username:    "order-test-user",
		Password:    "password123",
		DisplayName: "Order Test User",
		Email:       "order-test-user@example.com",
		AffCode:     "AFF-ORDER-TEST",
		Quota:       quota,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

func createTestPlanForOrders(t *testing.T, db *gorm.DB, name string, priceCents int64, currency string) *model.SubscriptionPlan {
	t.Helper()

	plan := &model.SubscriptionPlan{
		Name:              name,
		PriceCents:        priceCents,
		Currency:          currency,
		BillingCycle:      common.BillingCycleMonthly,
		BillingCycleValue: 1,
		Status:            common.PlanStatusActive,
		CreatedAt:         common.GetTimestamp(),
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("failed to create test plan: %v", err)
	}
	return plan
}

func createTestCouponForOrders(t *testing.T, db *gorm.DB, code string, currency string, applicablePlanIdsJSON *string) *model.Coupon {
	t.Helper()

	now := common.GetTimestamp()
	coupon := &model.Coupon{
		Code:          code,
		Name:          "订单测试券",
		Type:          common.CouponTypeDiscount,
		Scope:         common.CouponScopeSubscription,
		DiscountValue: 20,
		TotalCount:    100,
		PerUserLimit:  5,
		ValidFrom:     now - 3600,
		ValidTo:       now + 3600*24,
		Status:        common.CouponStatusActive,
		CreatedBy:     1,
		Currency:      currency,
	}
	if applicablePlanIdsJSON != nil {
		coupon.ApplicablePlanIds = applicablePlanIdsJSON
	}
	if err := db.Create(coupon).Error; err != nil {
		t.Fatalf("failed to create test coupon: %v", err)
	}
	return coupon
}

func createTestUserCouponForOrders(t *testing.T, db *gorm.DB, userId int64, couponId int64) *model.UserCoupon {
	t.Helper()

	now := common.GetTimestamp()
	userCoupon := &model.UserCoupon{
		CouponId:  couponId,
		UserId:    userId,
		Code:      "UC-" + common.GetUUID(),
		Status:    common.UserCouponStatusAvailable,
		ClaimedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(userCoupon).Error; err != nil {
		t.Fatalf("failed to create test user coupon: %v", err)
	}
	return userCoupon
}

func TestCreateOrder_CouponCurrencyMismatchRejected(t *testing.T) {
	_, cleanup := setupSubscriptionOrderTestDB(t)
	defer cleanup()

	user := createTestUserForOrders(t, model.DB, 1, 10_000_000)
	plan := createTestPlanForOrders(t, model.DB, "USD Plan", 1000, "USD")
	coupon := createTestCouponForOrders(t, model.DB, "ORDERCOUPON001", "CNY", nil)
	userCoupon := createTestUserCouponForOrders(t, model.DB, int64(user.Id), coupon.Id)

	svc := service.GetSubscriptionOrderService()
	_, err := svc.CreateOrder(int64(user.Id), plan.Id, &userCoupon.Id, common.PaymentChannelWallet)
	if err == nil {
		t.Fatalf("expected error for coupon currency mismatch")
	}
	if !strings.Contains(err.Error(), "优惠券币种与订单不匹配") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateOrder_CouponPlanMismatchRejected(t *testing.T) {
	_, cleanup := setupSubscriptionOrderTestDB(t)
	defer cleanup()

	user := createTestUserForOrders(t, model.DB, 1, 10_000_000)
	plan := createTestPlanForOrders(t, model.DB, "CNY Plan", 1000, "CNY")
	applicable := "[999]" // 不包含 plan.Id
	coupon := createTestCouponForOrders(t, model.DB, "ORDERCOUPON002", "CNY", &applicable)
	userCoupon := createTestUserCouponForOrders(t, model.DB, int64(user.Id), coupon.Id)

	svc := service.GetSubscriptionOrderService()
	_, err := svc.CreateOrder(int64(user.Id), plan.Id, &userCoupon.Id, common.PaymentChannelWallet)
	if err == nil {
		t.Fatalf("expected error for coupon plan mismatch")
	}
	if !strings.Contains(err.Error(), common.MsgCouponPlanMismatch) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateOrder_CouponApplicableSuccess(t *testing.T) {
	_, cleanup := setupSubscriptionOrderTestDB(t)
	defer cleanup()

	user := createTestUserForOrders(t, model.DB, 1, 10_000_000)
	plan := createTestPlanForOrders(t, model.DB, "CNY Plan", 1000, "CNY")
	applicable := "[" + strconv.FormatInt(plan.Id, 10) + "]"
	coupon := createTestCouponForOrders(t, model.DB, "ORDERCOUPON003", "CNY", &applicable)
	userCoupon := createTestUserCouponForOrders(t, model.DB, int64(user.Id), coupon.Id)

	svc := service.GetSubscriptionOrderService()
	result, err := svc.CreateOrder(int64(user.Id), plan.Id, &userCoupon.Id, common.PaymentChannelWallet)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
	if !result.CouponApplied {
		t.Fatalf("expected CouponApplied=true")
	}
	if result.DiscountAmount != 200 {
		t.Fatalf("expected DiscountAmount=200, got %d", result.DiscountAmount)
	}
	if result.FinalAmount != 800 {
		t.Fatalf("expected FinalAmount=800, got %d", result.FinalAmount)
	}
	if result.Order.CouponId == nil || *result.Order.CouponId != coupon.Id {
		t.Fatalf("expected order.coupon_id=%d, got %v", coupon.Id, result.Order.CouponId)
	}
	if result.Order.CouponSnapshot == nil || *result.Order.CouponSnapshot == "" {
		t.Fatalf("expected coupon_snapshot to be set")
	}

	snapshot, err := result.Order.GetCouponSnapshotData()
	if err != nil {
		t.Fatalf("GetCouponSnapshotData failed: %v", err)
	}
	if snapshot == nil {
		t.Fatalf("expected non-nil coupon snapshot")
	}
	if snapshot.Currency != "CNY" {
		t.Fatalf("expected snapshot.currency=CNY, got %q", snapshot.Currency)
	}
}

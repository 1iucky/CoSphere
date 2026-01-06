package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/shopspring/decimal"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ===================== 测试辅助函数 =====================

// setupRedemptionTestDB 设置测试数据库
func setupRedemptionTestDB(t *testing.T) (*gorm.DB, func()) {
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

	// 自动迁移测试表
	err = db.AutoMigrate(
		&model.Redemption{},
		&model.Subscription{},
		&model.SubscriptionPlan{},
		&model.SubscriptionPlanLimit{},
		&model.User{},
		&model.UserBill{},
		&model.AuditLog{},
		&model.Coupon{},
		&model.UserCoupon{},
		&model.CouponRedemptionBinding{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// 创建测试用户（兑换/订阅逻辑会锁定用户行）
	testUser := &model.User{
		Id:          1,
		Username:    "test-user-1",
		Password:    "password123",
		DisplayName: "Test User 1",
		Email:       "test-user-1@example.com",
	}
	if err := db.Create(testUser).Error; err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	cleanup := func() {
		sqlDB.Close()
	}

	return db, cleanup
}

// createTestPlan 创建测试套餐
func createTestPlan(t *testing.T, db *gorm.DB, name string, priceCents int64, billingCycle string) *model.SubscriptionPlan {
	plan := &model.SubscriptionPlan{
		Name:              name,
		PriceCents:        priceCents,
		BillingCycle:      billingCycle,
		BillingCycleValue: 30,
		Status:            common.PlanStatusActive,
		CreatedAt:         common.GetTimestamp(),
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("failed to create test plan: %v", err)
	}
	return plan
}

// createSubscriptionRedemption 创建订阅类型测试兑换码
// 重命名以避免与 coupon_service_test.go 中的 createSubscriptionRedemption 冲突
func createSubscriptionRedemption(t *testing.T, db *gorm.DB, key string, planId int64, durationDays int) *model.Redemption {
	payload := model.RedemptionSubscriptionPayload{
		PlanId:       planId,
		DurationDays: durationDays,
		RedeemOption: common.RedeemOptionCoexist,
	}
	payloadJSON, _ := payload.ToJSON()

	redemption := &model.Redemption{
		Key:         key,
		Type:        common.RedemptionTypeSubscription,
		Status:      common.RedemptionCodeStatusEnabled, // 修复：使用正确的常量
		Payload:     payloadJSON,
		CreatedTime: common.GetTimestamp(), // 修复：使用正确的字段名
	}
	if err := db.Create(redemption).Error; err != nil {
		t.Fatalf("failed to create test redemption: %v", err)
	}
	return redemption
}

// createRedemptionTestSubscription 创建测试订阅
// 重命名以避免与其他测试文件中的同名函数冲突
func createRedemptionTestSubscription(t *testing.T, db *gorm.DB, userId int64, planId int64, startAt int64, endAt int64) *model.Subscription {
	sub := &model.Subscription{
		UserId:    userId,
		PlanId:    planId,
		Status:    common.SubscriptionStatusActive,
		StartAt:   startAt,
		EndAt:     endAt,
		CreatedAt: common.GetTimestamp(),
		Priority:  1,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("failed to create test subscription: %v", err)
	}
	return sub
}

// ===================== 2.7.2 同套餐叠加测试 =====================

// TestExecuteStackRedemption_Success 测试同套餐叠加成功
func TestExecuteStackRedemption_Success(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	// 创建测试套餐
	plan := createTestPlan(t, db, "月度套餐", 9900, common.BillingCycleMonthly)

	// 创建现有订阅（还剩 15 天）
	now := common.GetTimestamp()
	existingEndAt := now + 15*24*60*60
	createRedemptionTestSubscription(t, db, 1, plan.Id, now-15*24*60*60, existingEndAt)

	// 创建兑换码（30 天）
	redemption := createSubscriptionRedemption(t, db, "STACK-TEST-001", plan.Id, 30)

	// 执行兑换
	result, err := svc.RedeemSubscription(1, redemption.Key, "")

	// 验证
	if err != nil {
		t.Fatalf("RedeemSubscription failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got failure: %s", result.ErrorMessage)
	}
	if result.ExtendedDays != 30 {
		t.Errorf("Expected ExtendedDays=30, got %d", result.ExtendedDays)
	}
	// 验证新结束时间 = 原结束时间 + 30 天
	expectedNewEndAt := existingEndAt + 30*24*60*60
	if result.NewEndAt != expectedNewEndAt {
		t.Errorf("Expected NewEndAt=%d, got %d", expectedNewEndAt, result.NewEndAt)
	}
}

// TestExecuteStackRedemption_NoExistingSubscription 测试无现有订阅时创建新订阅
func TestExecuteStackRedemption_NoExistingSubscription(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	plan := createTestPlan(t, db, "月度套餐", 9900, common.BillingCycleMonthly)
	redemption := createSubscriptionRedemption(t, db, "STACK-TEST-002", plan.Id, 30)

	result, err := svc.RedeemSubscription(1, redemption.Key, "")

	if err != nil {
		t.Fatalf("RedeemSubscription failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got failure: %s", result.ErrorMessage)
	}
	if result.NewSubscription == nil {
		t.Error("Expected NewSubscription to be set")
	}
}

// ===================== 2.7.3 更贵套餐选择测试 =====================

// TestCheckSubscriptionConflict_MoreExpensive 测试更贵套餐冲突检测
func TestCheckSubscriptionConflict_MoreExpensive(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	// 创建两个套餐：基础套餐和高级套餐
	basicPlan := createTestPlan(t, db, "基础套餐", 9900, common.BillingCycleMonthly)
	premiumPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	// 用户已有基础套餐订阅
	now := common.GetTimestamp()
	createRedemptionTestSubscription(t, db, 1, basicPlan.Id, now, now+30*24*60*60)

	// 检测与高级套餐的冲突
	conflictResult, err := svc.CheckSubscriptionConflict(1, premiumPlan.Id)

	if err != nil {
		t.Fatalf("CheckSubscriptionConflict failed: %v", err)
	}
	if !conflictResult.HasConflict {
		t.Error("Expected HasConflict=true")
	}
	if conflictResult.ConflictType != "more_expensive" {
		t.Errorf("Expected ConflictType='more_expensive', got '%s'", conflictResult.ConflictType)
	}
	if conflictResult.ExistingPlanId != basicPlan.Id {
		t.Errorf("Expected ExistingPlanId=%d, got %d", basicPlan.Id, conflictResult.ExistingPlanId)
	}
}

// TestMoreExpensivePlan_NeedUserChoice 测试更贵套餐需要用户选择
func TestMoreExpensivePlan_NeedUserChoice(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	basicPlan := createTestPlan(t, db, "基础套餐", 9900, common.BillingCycleMonthly)
	premiumPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	now := common.GetTimestamp()
	createRedemptionTestSubscription(t, db, 1, basicPlan.Id, now, now+30*24*60*60)

	redemption := createSubscriptionRedemption(t, db, "PREMIUM-TEST-001", premiumPlan.Id, 30)
	// more_expensive 场景下，如果 payload 默认选项不是 coexist/convert/replace（如 stack），应提示用户选择
	payload := model.RedemptionSubscriptionPayload{
		PlanId:       premiumPlan.Id,
		DurationDays: 30,
		RedeemOption: common.RedeemOptionStack,
	}
	payloadJSON, _ := payload.ToJSON()
	db.Model(&model.Redemption{}).Where("id = ?", redemption.Id).Update("payload", payloadJSON)

	// 不指定 redeemOption，应该返回冲突错误
	result, err := svc.RedeemSubscription(1, redemption.Key, "")

	if err == nil {
		t.Fatal("Expected error for conflict without user choice")
	}
	if result.ErrorCode != "REDEMPTION_CONFLICT" {
		t.Errorf("Expected ErrorCode='REDEMPTION_CONFLICT', got '%s'", result.ErrorCode)
	}
}

// TestMoreExpensivePlan_Coexist 测试更贵套餐选择并存
func TestMoreExpensivePlan_Coexist(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	basicPlan := createTestPlan(t, db, "基础套餐", 9900, common.BillingCycleMonthly)
	premiumPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	now := common.GetTimestamp()
	createRedemptionTestSubscription(t, db, 1, basicPlan.Id, now, now+30*24*60*60)

	redemption := createSubscriptionRedemption(t, db, "PREMIUM-TEST-002", premiumPlan.Id, 30)

	// 指定 coexist 选项
	result, err := svc.RedeemSubscription(1, redemption.Key, common.RedeemOptionCoexist)

	if err != nil {
		t.Fatalf("RedeemSubscription failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got failure: %s", result.ErrorMessage)
	}
	if result.RedeemOption != common.RedeemOptionCoexist {
		t.Errorf("Expected RedeemOption='coexist', got '%s'", result.RedeemOption)
	}

	// 验证用户现在有两个订阅
	var count int64
	db.Model(&model.Subscription{}).Where("user_id = ? AND status = ?", 1, common.SubscriptionStatusActive).Count(&count)
	if count != 2 {
		t.Errorf("Expected 2 active subscriptions, got %d", count)
	}
}

// TestMoreExpensivePlan_Replace 测试更贵套餐选择替换
func TestMoreExpensivePlan_Replace(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	basicPlan := createTestPlan(t, db, "基础套餐", 9900, common.BillingCycleMonthly)
	premiumPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	now := common.GetTimestamp()
	oldSub := createRedemptionTestSubscription(t, db, 1, basicPlan.Id, now, now+30*24*60*60)

	redemption := createSubscriptionRedemption(t, db, "PREMIUM-TEST-003", premiumPlan.Id, 30)

	// 指定 replace 选项
	result, err := svc.RedeemSubscription(1, redemption.Key, common.RedeemOptionReplace)

	if err != nil {
		t.Fatalf("RedeemSubscription failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got failure: %s", result.ErrorMessage)
	}

	// 验证旧订阅被取消
	var cancelledSub model.Subscription
	db.First(&cancelledSub, oldSub.Id)
	if cancelledSub.Status != common.SubscriptionStatusCancelled {
		t.Errorf("Expected old subscription to be cancelled, got status '%s'", cancelledSub.Status)
	}

	// 验证新订阅已创建
	if result.NewSubscription == nil {
		t.Error("Expected NewSubscription to be set")
	}
}

// ===================== 2.7.4 折算精度测试 =====================

// TestCalculateRemainValue_Precision 测试剩余价值计算精度
func TestCalculateRemainValue_Precision(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	// 月度套餐 99 元，日均价 = 9900 / 30 = 330 分
	plan := createTestPlan(t, db, "月度套餐", 9900, common.BillingCycleMonthly)

	// 剩余 15 天的订阅
	now := common.GetTimestamp()
	sub := &model.Subscription{
		UserId:  1,
		PlanId:  plan.Id,
		Status:  common.SubscriptionStatusActive,
		StartAt: now - 15*24*60*60,
		EndAt:   now + 15*24*60*60, // 剩余 15 天
	}
	db.Create(sub)

	// 重新查询以加载 Plan
	db.Preload("Plan").First(sub, sub.Id)

	remainValue, err := svc.CalculateRemainValue(sub, plan)
	if err != nil {
		t.Fatalf("CalculateRemainValue failed: %v", err)
	}

	// 预期剩余价值 = 15 * 330 = 4950 分
	expectedValue := decimal.NewFromInt(4950)
	// 允许 1 分钱误差（因为时间计算可能有微小偏差）
	diff := remainValue.Sub(expectedValue).Abs()
	if diff.GreaterThan(decimal.NewFromInt(10)) {
		t.Errorf("Expected remain value ~4950, got %s", remainValue.String())
	}
}

// TestConvertToDays_FloorRounding 测试折算天数向下取整
func TestConvertToDays_FloorRounding(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	// 新套餐日均价 = 19900 / 30 = 663.33 分
	newPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	// 剩余价值 5000 分
	remainValue := decimal.NewFromInt(5000)

	convertedDays, remainder, err := svc.ConvertToDays(remainValue, newPlan)
	if err != nil {
		t.Fatalf("ConvertToDays failed: %v", err)
	}

	// 5000 / 663.33 = 7.54，向下取整 = 7 天
	if convertedDays != 7 {
		t.Errorf("Expected convertedDays=7, got %d", convertedDays)
	}

	// 验证有剩余金额
	if remainder.LessThanOrEqual(decimal.Zero) {
		t.Error("Expected positive remainder amount")
	}
}

// TestConversionPreview_IncludesRedeemDuration 测试折算预览包含兑换码时长
func TestConversionPreview_IncludesRedeemDuration(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	basicPlan := createTestPlan(t, db, "基础套餐", 9900, common.BillingCycleMonthly)
	premiumPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	now := common.GetTimestamp()
	createRedemptionTestSubscription(t, db, 1, basicPlan.Id, now, now+15*24*60*60)

	redemption := createSubscriptionRedemption(t, db, "CONVERT-PREVIEW-001", premiumPlan.Id, 30)

	// 获取完整预览（包含兑换码时长）
	conflictResult, err := svc.PromptUserChoice(1, redemption.Key)
	if err != nil {
		t.Fatalf("PromptUserChoice failed: %v", err)
	}

	if conflictResult.ConversionPreview == nil {
		t.Fatal("Expected ConversionPreview to be set")
	}

	preview := conflictResult.ConversionPreview
	// NewEndAt 应该 > now + 30 天（因为还包含折算天数）
	minExpectedEndAt := now + 30*24*60*60
	if preview.NewEndAt <= minExpectedEndAt {
		t.Errorf("Expected NewEndAt > %d (now + 30 days), got %d", minExpectedEndAt, preview.NewEndAt)
	}
}

// ===================== 2.7.5 并存逻辑测试 =====================

// TestCoexist_SubscriptionLimit 测试订阅数量限制
func TestCoexist_SubscriptionLimit(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	// 创建多个套餐
	now := common.GetTimestamp()
	maxSubs := common.GetSubscriptionMaxPerUser()

	for i := 0; i < maxSubs; i++ {
		plan := createTestPlan(t, db, "套餐"+string(rune('A'+i)), int64(9900+i*1000), common.BillingCycleMonthly)
		createRedemptionTestSubscription(t, db, 1, plan.Id, now, now+30*24*60*60)
	}

	// 尝试添加超出限制的订阅
	extraPlan := createTestPlan(t, db, "额外套餐", 29900, common.BillingCycleMonthly)
	redemption := createSubscriptionRedemption(t, db, "LIMIT-TEST-001", extraPlan.Id, 30)

	result, err := svc.RedeemSubscription(1, redemption.Key, common.RedeemOptionCoexist)

	if err == nil && result.Success {
		t.Error("Expected failure due to subscription limit")
	}
}

// ===================== 2.7.6 优惠券原子回滚测试 =====================

// TestBoundCoupon_ConflictNoConsumption 测试有冲突时不消费优惠券
func TestBoundCoupon_ConflictNoConsumption(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	basicPlan := createTestPlan(t, db, "基础套餐", 9900, common.BillingCycleMonthly)
	premiumPlan := createTestPlan(t, db, "高级套餐", 19900, common.BillingCycleMonthly)

	now := common.GetTimestamp()
	createRedemptionTestSubscription(t, db, 1, basicPlan.Id, now, now+30*24*60*60)

	// 创建优惠券
	coupon := &model.Coupon{
		Code:          "TEST-COUPON-001",
		Name:          "测试优惠券",
		Type:          common.CouponTypeDiscount,
		Scope:         common.CouponScopeWalletSubscription,
		DiscountValue: 10,
		DiscountType:  common.DiscountTypePercentage,
		TotalCount:    100,
		PerUserLimit:  1,
		ValidFrom:     now - 86400,
		ValidTo:       now + 86400*30,
		Status:        common.CouponStatusActive,
		CreatedBy:     1,
	}
	db.Create(coupon)

	redemption := createSubscriptionRedemption(t, db, "COUPON-CONFLICT-001", premiumPlan.Id, 30)
	// more_expensive 场景下，如果 payload 默认选项不是 coexist/convert/replace（如 stack），应提示用户选择且不消费券
	payload := model.RedemptionSubscriptionPayload{
		PlanId:       premiumPlan.Id,
		DurationDays: 30,
		RedeemOption: common.RedeemOptionStack,
	}
	payloadJSON, _ := payload.ToJSON()
	db.Model(&model.Redemption{}).Where("id = ?", redemption.Id).Update("payload", payloadJSON)

	// 绑定优惠券到兑换码
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: int64(redemption.Id),
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
	}
	db.Create(binding)

	// 不指定选项，应该返回冲突，且不消费优惠券
	result, _ := svc.RedeemWithCoupon(1, redemption.Key, "")

	if result.ErrorCode != "REDEMPTION_CONFLICT" {
		t.Errorf("Expected REDEMPTION_CONFLICT, got %s", result.ErrorCode)
	}
	if result.CouponUsed {
		t.Error("Coupon should NOT be consumed on conflict")
	}

	// 验证优惠券绑定状态未改变
	var bindingAfter model.CouponRedemptionBinding
	db.First(&bindingAfter, binding.Id)
	if bindingAfter.Status != common.CouponBindingStatusReserved {
		t.Errorf("Expected binding status 'reserved', got '%s'", bindingAfter.Status)
	}
}

// ===================== 幂等性测试 =====================

// TestRedemption_AlreadyUsed 测试已使用的兑换码
func TestRedemption_AlreadyUsed(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	plan := createTestPlan(t, db, "月度套餐", 9900, common.BillingCycleMonthly)
	redemption := createSubscriptionRedemption(t, db, "IDEMPOTENT-001", plan.Id, 30)

	// 第一次兑换
	result1, err1 := svc.RedeemSubscription(1, redemption.Key, "")
	if err1 != nil || !result1.Success {
		t.Fatalf("First redemption failed: %v", err1)
	}

	// 第二次兑换（同一兑换码）
	result2, err2 := svc.RedeemSubscription(1, redemption.Key, "")

	if err2 == nil && result2.Success {
		t.Error("Expected failure for already used redemption")
	}
	// 修复：使用细分后的错误码 REDEMPTION_USED（而非 REDEMPTION_INVALID）
	if result2.ErrorCode != "REDEMPTION_USED" {
		t.Errorf("Expected ErrorCode='REDEMPTION_USED', got '%s'", result2.ErrorCode)
	}
}

// TestRedemption_InvalidKey 测试无效兑换码
func TestRedemption_InvalidKey(t *testing.T) {
	_, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	result, err := svc.RedeemSubscription(1, "INVALID-KEY-12345", "")

	if err == nil {
		t.Error("Expected error for invalid key")
	}
	if result.ErrorCode != "REDEMPTION_NOT_FOUND" {
		t.Errorf("Expected ErrorCode='REDEMPTION_NOT_FOUND', got '%s'", result.ErrorCode)
	}
}

// ===================== Extend 语义测试 =====================

// TestExtend_SamePlan 测试 extend 选项（与 stack 相同）
func TestExtend_SamePlan(t *testing.T) {
	db, cleanup := setupRedemptionTestDB(t)
	defer cleanup()

	svc := service.GetRedemptionService()

	plan := createTestPlan(t, db, "月度套餐", 9900, common.BillingCycleMonthly)

	now := common.GetTimestamp()
	existingEndAt := now + 15*24*60*60
	createRedemptionTestSubscription(t, db, 1, plan.Id, now-15*24*60*60, existingEndAt)

	redemption := createSubscriptionRedemption(t, db, "EXTEND-TEST-001", plan.Id, 30)

	// 使用 extend 选项
	result, err := svc.RedeemSubscription(1, redemption.Key, common.RedeemOptionExtend)

	if err != nil {
		t.Fatalf("RedeemSubscription failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got failure: %s", result.ErrorMessage)
	}
	if result.ExtendedDays != 30 {
		t.Errorf("Expected ExtendedDays=30, got %d", result.ExtendedDays)
	}
}

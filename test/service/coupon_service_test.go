package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ===================== 测试辅助函数 =====================

// setupCouponTestDB 设置测试数据库
func setupCouponTestDB(t *testing.T) (*gorm.DB, func()) {
	// 创建内存 SQLite 数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// SQLite in-memory 数据库是按连接隔离的，需限制连接池避免出现 "no such table" 的偶发错误
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// 设置全局数据库实例
	model.DB = db

	// 自动迁移测试表
	err = db.AutoMigrate(
		&model.Coupon{},
		&model.UserCoupon{},
		&model.AuditLog{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	// 返回清理函数
	cleanup := func() {
		sqlDB.Close()
	}

	return db, cleanup
}

// createTestCoupon 创建测试优惠券
func createTestCoupon(t *testing.T, db *gorm.DB, code string, totalCount int64, perUserLimit int64, validFrom int64, validTo int64) *model.Coupon {
	now := common.GetTimestamp()
	if validFrom == 0 {
		validFrom = now - 86400 // 默认昨天开始
	}
	if validTo == 0 {
		validTo = now + 86400*30 // 默认30天后过期
	}

	coupon := &model.Coupon{
		Code:          code,
		Name:          "测试优惠券",
		Type:          common.CouponTypeDiscount,
		Scope:         common.CouponScopeWalletSubscription,
		DiscountValue: 20, // 20% 折扣
		DiscountType:  common.DiscountTypePercentage,
		TotalCount:    totalCount,
		UsedCount:     0,
		PerUserLimit:  perUserLimit,
		ValidFrom:     validFrom,
		ValidTo:       validTo,
		Status:        common.CouponStatusActive,
		CreatedBy:     1,
		Currency:      "CNY",
		Version:       0,
	}

	err := db.Create(coupon).Error
	if err != nil {
		t.Fatalf("failed to create test coupon: %v", err)
	}

	return coupon
}

// ===================== 优惠券领取服务测试 =====================

// TestClaimCoupon_Success 测试成功领取优惠券
func TestClaimCoupon_Success(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(1001)
	claimCode := "COUPON20251215ABC123"

	// 创建测试优惠券
	createTestCoupon(t, db, claimCode, 100, 1, 0, 0)

	// 领取优惠券
	userCoupon, err := svc.ClaimCoupon(userId, claimCode)
	if err != nil {
		t.Fatalf("ClaimCoupon failed: %v", err)
	}

	// 验证结果
	if userCoupon == nil {
		t.Fatal("expected user coupon, got nil")
	}
	if userCoupon.UserId != userId {
		t.Errorf("expected userId %d, got %d", userId, userCoupon.UserId)
	}
	if userCoupon.Status != common.UserCouponStatusAvailable {
		t.Errorf("expected status %s, got %s", common.UserCouponStatusAvailable, userCoupon.Status)
	}
	if userCoupon.Code == "" {
		t.Error("expected user coupon code, got empty string")
	}

	// 验证优惠券库存已扣减
	var coupon model.Coupon
	db.Where("code = ?", claimCode).First(&coupon)
	if coupon.UsedCount != 1 {
		t.Errorf("expected used_count = 1, got %d", coupon.UsedCount)
	}
	if coupon.Version != 1 {
		t.Errorf("expected version = 1 (optimistic lock incremented), got %d", coupon.Version)
	}
}

// TestClaimCoupon_InsufficientStock 测试库存不足
func TestClaimCoupon_InsufficientStock(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	claimCode := "COUPON20251215XYZ789"

	// 创建优惠券，然后更新使库存耗尽（TotalCount=1, UsedCount=1）
	// 注意：GORM 在 TotalCount=0 时会使用默认值 1，所以需要用这种方式模拟库存耗尽
	coupon := createTestCoupon(t, db, claimCode, 1, 1, 0, 0)
	db.Model(coupon).Update("used_count", 1) // 更新为已用完

	// 尝试领取
	_, err := svc.ClaimCoupon(1001, claimCode)
	if err == nil {
		t.Fatal("expected error for insufficient stock, got nil")
	}

	expectedMsg := common.MsgCouponExhausted
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestClaimCoupon_DuplicateClaim 测试重复领取
func TestClaimCoupon_DuplicateClaim(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(1002)
	claimCode := "COUPON20251215DUP456"

	// 创建测试优惠券
	createTestCoupon(t, db, claimCode, 100, 1, 0, 0)

	// 第一次领取（成功）
	_, err := svc.ClaimCoupon(userId, claimCode)
	if err != nil {
		t.Fatalf("first claim failed: %v", err)
	}

	// 第二次领取（应该失败）
	_, err = svc.ClaimCoupon(userId, claimCode)
	if err == nil {
		t.Fatal("expected error for duplicate claim, got nil")
	}

	expectedMsg := "您已领取过该优惠券"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestClaimCoupon_Expired 测试已过期优惠券
func TestClaimCoupon_Expired(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	claimCode := "COUPON20251215EXP789"
	now := common.GetTimestamp()

	// 创建已过期的优惠券（valid_to 在昨天）
	createTestCoupon(t, db, claimCode, 100, 1, now-86400*30, now-86400)

	// 尝试领取
	_, err := svc.ClaimCoupon(1003, claimCode)
	if err == nil {
		t.Fatal("expected error for expired coupon, got nil")
	}

	expectedMsg := common.MsgCouponExpired
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestClaimCoupon_NotYetValid 测试尚未生效优惠券
func TestClaimCoupon_NotYetValid(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	claimCode := "COUPON20251215FUTURE"
	now := common.GetTimestamp()

	// 创建明天才生效的优惠券
	createTestCoupon(t, db, claimCode, 100, 1, now+86400, now+86400*30)

	// 尝试领取
	_, err := svc.ClaimCoupon(1004, claimCode)
	if err == nil {
		t.Fatal("expected error for not yet valid coupon, got nil")
	}

	expectedMsg := "优惠券尚未生效"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestClaimCoupon_InvalidStatus 测试无效状态优惠券
func TestClaimCoupon_InvalidStatus(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	claimCode := "COUPON20251215INACTIVE"

	// 创建已停用的优惠券
	coupon := createTestCoupon(t, db, claimCode, 100, 1, 0, 0)
	db.Model(coupon).Update("status", common.CouponStatusInactive)

	// 尝试领取
	_, err := svc.ClaimCoupon(1005, claimCode)
	if err == nil {
		t.Fatal("expected error for inactive coupon, got nil")
	}

	expectedMsg := common.MsgCouponInvalidStatus
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestClaimCoupon_ConcurrentClaim 测试并发领取（乐观锁机制）
// 注意：SQLite 内存数据库不支持真正的并发测试，此测试用于验证基本的并发防护逻辑
// 真正的并发压力测试应在集成测试中使用 PostgreSQL/MySQL 进行
func TestClaimCoupon_ConcurrentClaim(t *testing.T) {
	t.Skip("Skipping concurrent test - SQLite in-memory doesn't support true concurrency. Use integration tests with PostgreSQL for proper concurrent testing.")
}

// ===================== 优惠券校验方法测试 =====================

// TestValidateCouponClaimable_Valid 测试有效优惠券校验
func TestValidateCouponClaimable_Valid(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	claimCode := "COUPON20251215VALID"

	// 创建有效优惠券
	createTestCoupon(t, db, claimCode, 100, 1, 0, 0)

	// 校验
	result := svc.ValidateCouponClaimable(claimCode)

	// 验证结果
	if !result.Valid {
		t.Errorf("expected Valid = true, got false. Error: %s", result.ErrorMessage)
	}
	if result.RemainingCount != 100 {
		t.Errorf("expected RemainingCount = 100, got %d", result.RemainingCount)
	}
}

// TestValidateCouponClaimable_NotFound 测试不存在的优惠券
func TestValidateCouponClaimable_NotFound(t *testing.T) {
	_, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 校验不存在的优惠券
	result := svc.ValidateCouponClaimable("NONEXISTENT")

	// 验证结果
	if result.Valid {
		t.Error("expected Valid = false for non-existent coupon")
	}
	if result.ErrorCode != "COUPON_NOT_FOUND" {
		t.Errorf("expected ErrorCode = COUPON_NOT_FOUND, got %s", result.ErrorCode)
	}
}

// ===================== 用户领取限制检查测试 =====================

// TestCheckUserClaimLimit_CanClaim 测试可以领取
func TestCheckUserClaimLimit_CanClaim(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(3001)
	claimCode := "COUPON20251215LIMIT"

	// 创建优惠券
	createTestCoupon(t, db, claimCode, 100, 1, 0, 0)

	// 检查
	result := svc.CheckUserClaimLimit(userId, claimCode)

	// 验证结果
	if !result.CanClaim {
		t.Errorf("expected CanClaim = true, got false. Error: %s", result.ErrorMessage)
	}
	if result.AlreadyClaimedCount != 0 {
		t.Errorf("expected AlreadyClaimedCount = 0, got %d", result.AlreadyClaimedCount)
	}
	if result.PerUserLimit != 1 {
		t.Errorf("expected PerUserLimit = 1, got %d", result.PerUserLimit)
	}
}

// TestCheckUserClaimLimit_AlreadyClaimed 测试已领取
func TestCheckUserClaimLimit_AlreadyClaimed(t *testing.T) {
	db, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(3002)
	claimCode := "COUPON20251215CLAIMED"

	// 创建优惠券并领取
	coupon := createTestCoupon(t, db, claimCode, 100, 1, 0, 0)
	_, _ = model.ClaimCoupon(userId, claimCode)

	// 再次检查
	result := svc.CheckUserClaimLimit(userId, claimCode)

	// 验证结果
	if result.CanClaim {
		t.Error("expected CanClaim = false for already claimed coupon")
	}
	if result.ErrorCode != "COUPON_ALREADY_CLAIMED" {
		t.Errorf("expected ErrorCode = COUPON_ALREADY_CLAIMED, got %s", result.ErrorCode)
	}
	if !result.UserAlreadyClaimed {
		t.Error("expected UserAlreadyClaimed = true")
	}

	// 验证优惠券库存（应该已扣减）
	db.First(coupon, coupon.Id)
	if coupon.UsedCount != 1 {
		t.Errorf("expected UsedCount = 1, got %d", coupon.UsedCount)
	}
}

// ===================== 边界条件测试 =====================

// TestClaimCoupon_EmptyClaimCode 测试空领取码
func TestClaimCoupon_EmptyClaimCode(t *testing.T) {
	_, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()

	_, err := svc.ClaimCoupon(1001, "")
	if err == nil {
		t.Fatal("expected error for empty claim code")
	}

	expectedMsg := "领取码不能为空"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestClaimCoupon_ZeroUserId 测试零用户ID
func TestClaimCoupon_ZeroUserId(t *testing.T) {
	_, cleanup := setupCouponTestDB(t)
	defer cleanup()

	svc := service.GetCouponService()

	_, err := svc.ClaimCoupon(0, "COUPON123")
	if err == nil {
		t.Fatal("expected error for zero user ID")
	}

	expectedMsg := "用户 ID 不能为空"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// ===================== 2.6.2 优惠券核销测试 =====================

// setupCouponTestDBWithAll 设置测试数据库（包含所有相关表）
func setupCouponTestDBWithAll(t *testing.T) (*gorm.DB, func()) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to connect database: %v", err)
	}

	// SQLite in-memory 数据库是按连接隔离的，需限制连接池避免出现 "no such table" 的偶发错误
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	model.DB = db

	// 自动迁移所有相关测试表
	err = db.AutoMigrate(
		&model.Coupon{},
		&model.UserCoupon{},
		&model.AuditLog{},
		&model.UserBill{},
		&model.CouponRedemptionBinding{},
		&model.Redemption{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		sqlDB.Close()
	}

	return db, cleanup
}

// createTestCouponWithType 创建指定类型的测试优惠券
func createTestCouponWithType(t *testing.T, db *gorm.DB, code string, couponType string, discountValue int64, thresholdAmount int64) *model.Coupon {
	now := common.GetTimestamp()
	coupon := &model.Coupon{
		Code:            code,
		Name:            "测试优惠券-" + couponType,
		Type:            couponType,
		Scope:           common.CouponScopeWalletSubscription,
		DiscountValue:   discountValue,
		ThresholdAmount: thresholdAmount,
		DiscountType:    common.DiscountTypePercentage,
		TotalCount:      100,
		UsedCount:       0,
		PerUserLimit:    5,
		ValidFrom:       now - 86400,
		ValidTo:         now + 86400*30,
		Status:          common.CouponStatusActive,
		CreatedBy:       1,
		Currency:        "CNY",
		Version:         0,
	}

	err := db.Create(coupon).Error
	if err != nil {
		t.Fatalf("failed to create test coupon: %v", err)
	}

	return coupon
}

// createTestUserCoupon 创建测试用户优惠券
func createTestUserCoupon(t *testing.T, db *gorm.DB, userId int64, couponId int64, status string) *model.UserCoupon {
	now := common.GetTimestamp()
	userCoupon := &model.UserCoupon{
		CouponId:  couponId,
		UserId:    userId,
		Code:      "UC-" + common.GetUUID(),
		Status:    status,
		ClaimedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := db.Create(userCoupon).Error
	if err != nil {
		t.Fatalf("failed to create test user coupon: %v", err)
	}

	return userCoupon
}

// TestCalculateDiscount_Discount 测试折扣券计算
func TestCalculateDiscount_Discount(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 创建 20% 折扣券
	coupon := createTestCouponWithType(t, db, "DISCOUNT20", common.CouponTypeDiscount, 20, 0)

	// 计算 1000 分的订单
	discount, err := svc.CalculateDiscount(coupon.Id, 1000)
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}

	// 1000 * 20 / 100 = 200
	if discount != 200 {
		t.Errorf("expected discount = 200, got %d", discount)
	}
}

// TestCalculateDiscount_FullReduction 测试满减券计算
func TestCalculateDiscount_FullReduction(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 创建满 500 减 100 的券
	coupon := createTestCouponWithType(t, db, "FULL500-100", common.CouponTypeFullReduction, 100, 500)

	// 订单金额满足阈值
	discount, err := svc.CalculateDiscount(coupon.Id, 600)
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}
	if discount != 100 {
		t.Errorf("expected discount = 100, got %d", discount)
	}

	// 订单金额不满足阈值
	_, err = svc.CalculateDiscount(coupon.Id, 400)
	if err == nil {
		t.Error("expected error for order amount below threshold")
	}
}

// TestCalculateDiscount_InstantReduction 测试立减券计算
func TestCalculateDiscount_InstantReduction(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 创建立减 50 的券
	coupon := createTestCouponWithType(t, db, "INSTANT50", common.CouponTypeInstantReduction, 50, 0)

	// 正常订单
	discount, err := svc.CalculateDiscount(coupon.Id, 100)
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}
	if discount != 50 {
		t.Errorf("expected discount = 50, got %d", discount)
	}

	// 订单金额小于优惠金额（不能超过订单金额）
	discount, err = svc.CalculateDiscount(coupon.Id, 30)
	if err != nil {
		t.Fatalf("CalculateDiscount failed: %v", err)
	}
	if discount != 30 {
		t.Errorf("expected discount = 30 (capped to order amount), got %d", discount)
	}
}

// TestCheckCouponThreshold 测试阈值检查
func TestCheckCouponThreshold(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 创建满减券
	coupon := createTestCouponWithType(t, db, "THRESHOLD100", common.CouponTypeFullReduction, 20, 100)

	// 满足阈值
	met, _, err := svc.CheckCouponThreshold(coupon.Id, 150)
	if err != nil {
		t.Fatalf("CheckCouponThreshold failed: %v", err)
	}
	if !met {
		t.Error("expected threshold met = true")
	}

	// 不满足阈值
	met, code, err := svc.CheckCouponThreshold(coupon.Id, 50)
	if err == nil {
		t.Error("expected error for order below threshold")
	}
	if met {
		t.Error("expected threshold met = false")
	}
	if code != "COUPON_THRESHOLD_NOT_MET" {
		t.Errorf("expected error code = COUPON_THRESHOLD_NOT_MET, got %s", code)
	}
}

// ===================== 2.6.3 绑定/解绑测试 =====================

// TestValidateBoundUser_NoBinding 测试无绑定记录
func TestValidateBoundUser_NoBinding(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 无绑定记录应该允许使用
	allowed, _, err := svc.ValidateBoundUser(999, 1001)
	if err != nil {
		t.Fatalf("ValidateBoundUser failed: %v", err)
	}
	if !allowed {
		t.Error("expected allowed = true for no binding")
	}
}

// ===================== 2.6.4 列表与预览测试 =====================

// TestGetUserCoupons_Success 测试获取用户优惠券列表
func TestGetUserCoupons_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4001)

	// 创建两个不同的优惠券模板和用户优惠券（唯一索引要求每个 user+coupon 组合唯一）
	coupon1 := createTestCouponWithType(t, db, "LIST001", common.CouponTypeDiscount, 10, 0)
	coupon2 := createTestCouponWithType(t, db, "LIST002", common.CouponTypeDiscount, 20, 0)
	createTestUserCoupon(t, db, userId, coupon1.Id, common.UserCouponStatusAvailable)
	createTestUserCoupon(t, db, userId, coupon2.Id, common.UserCouponStatusUsed)

	// 获取所有优惠券
	result, err := svc.GetUserCoupons(userId, "", 1, 10)
	if err != nil {
		t.Fatalf("GetUserCoupons failed: %v", err)
	}

	if result.Total != 2 {
		t.Errorf("expected total = 2, got %d", result.Total)
	}

	// 获取可用优惠券
	result, err = svc.GetUserCoupons(userId, common.UserCouponStatusAvailable, 1, 10)
	if err != nil {
		t.Fatalf("GetUserCoupons failed: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected total = 1 for available, got %d", result.Total)
	}
}

// TestGetUserCoupons_Pagination 测试分页
func TestGetUserCoupons_Pagination(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4002)

	// 创建 5 个不同的优惠券和用户优惠券（唯一索引要求每个 user+coupon 组合唯一）
	for i := 0; i < 5; i++ {
		coupon := createTestCouponWithType(t, db, fmt.Sprintf("PAGE%03d", i), common.CouponTypeDiscount, int64(10+i), 0)
		createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)
	}

	// 第一页，每页 2 个
	result, err := svc.GetUserCoupons(userId, "", 1, 2)
	if err != nil {
		t.Fatalf("GetUserCoupons failed: %v", err)
	}

	if len(result.Coupons) != 2 {
		t.Errorf("expected 2 coupons on page 1, got %d", len(result.Coupons))
	}
	if result.Total != 5 {
		t.Errorf("expected total = 5, got %d", result.Total)
	}

	// 第二页
	result, err = svc.GetUserCoupons(userId, "", 2, 2)
	if err != nil {
		t.Fatalf("GetUserCoupons failed: %v", err)
	}

	if len(result.Coupons) != 2 {
		t.Errorf("expected 2 coupons on page 2, got %d", len(result.Coupons))
	}
}

// TestPreviewCouponUsage_Success 测试预览优惠券使用
func TestPreviewCouponUsage_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4003)

	// 创建 20% 折扣券
	coupon := createTestCouponWithType(t, db, "PREVIEW001", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	// 预览使用效果
	result, err := svc.PreviewCouponUsage(userId, userCoupon.Id, 1000, "subscription")
	if err != nil {
		t.Fatalf("PreviewCouponUsage failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected Valid = true, got false. Error: %s", result.ErrorMessage)
	}
	if result.DiscountAmount != 200 {
		t.Errorf("expected DiscountAmount = 200, got %d", result.DiscountAmount)
	}
	if result.FinalAmount != 800 {
		t.Errorf("expected FinalAmount = 800, got %d", result.FinalAmount)
	}
}

// TestPreviewCouponUsage_InvalidUser 测试非归属用户预览
func TestPreviewCouponUsage_InvalidUser(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	ownerId := int64(4004)
	otherId := int64(4005)

	coupon := createTestCouponWithType(t, db, "PREVIEW002", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, ownerId, coupon.Id, common.UserCouponStatusAvailable)

	// 非归属用户预览
	result, _ := svc.PreviewCouponUsage(otherId, userCoupon.Id, 1000, "subscription")

	if result.Valid {
		t.Error("expected Valid = false for non-owner user")
	}
	if result.ErrorCode != "COUPON_USER_MISMATCH" {
		t.Errorf("expected ErrorCode = COUPON_USER_MISMATCH, got %s", result.ErrorCode)
	}
}

// TestPreviewCouponUsage_UsedCoupon 测试已使用优惠券预览
func TestPreviewCouponUsage_UsedCoupon(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4006)

	coupon := createTestCouponWithType(t, db, "PREVIEW003", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusUsed)

	result, _ := svc.PreviewCouponUsage(userId, userCoupon.Id, 1000, "subscription")

	if result.Valid {
		t.Error("expected Valid = false for used coupon")
	}
	if result.ErrorCode != "COUPON_STATUS_INVALID" {
		t.Errorf("expected ErrorCode = COUPON_STATUS_INVALID, got %s", result.ErrorCode)
	}
}

func TestPreviewCouponUsageWithPlan_PlanMismatch(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4010)
	planId := int64(123)

	coupon := createTestCouponWithType(t, db, "PREVIEWPLAN001", common.CouponTypeDiscount, 20, 0)
	// 设置为仅适用于其他套餐
	if err := db.Model(&model.Coupon{}).Where("id = ?", coupon.Id).
		Update("applicable_plan_ids", "[999]").Error; err != nil {
		t.Fatalf("failed to update applicable_plan_ids: %v", err)
	}
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	result, err := svc.PreviewCouponUsageWithPlan(userId, userCoupon.Id, 1000, "subscription", &planId, "CNY")
	if err != nil {
		t.Fatalf("PreviewCouponUsageWithPlan failed: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected Valid=false for plan mismatch")
	}
	if result.ErrorCode != "COUPON_NOT_APPLICABLE" {
		t.Fatalf("expected ErrorCode=COUPON_NOT_APPLICABLE, got %s", result.ErrorCode)
	}
	if result.ErrorMessage != common.MsgCouponPlanMismatch {
		t.Fatalf("expected ErrorMessage=%q, got %q", common.MsgCouponPlanMismatch, result.ErrorMessage)
	}
}

func TestPreviewCouponUsageWithPlan_CurrencyMismatch(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4011)
	planId := int64(123)

	coupon := createTestCouponWithType(t, db, "PREVIEWPLAN002", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	result, err := svc.PreviewCouponUsageWithPlan(userId, userCoupon.Id, 1000, "subscription", &planId, "USD")
	if err != nil {
		t.Fatalf("PreviewCouponUsageWithPlan failed: %v", err)
	}
	if result.Valid {
		t.Fatalf("expected Valid=false for currency mismatch")
	}
	if result.ErrorCode != "COUPON_CURRENCY_MISMATCH" {
		t.Fatalf("expected ErrorCode=COUPON_CURRENCY_MISMATCH, got %s", result.ErrorCode)
	}
	if result.ErrorMessage != "优惠券币种与订单不匹配" {
		t.Fatalf("unexpected ErrorMessage: %q", result.ErrorMessage)
	}
}

func TestPreviewCouponUsageWithPlan_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(4012)
	planId := int64(123)

	coupon := createTestCouponWithType(t, db, "PREVIEWPLAN003", common.CouponTypeDiscount, 20, 0)
	// 设置为仅适用于当前 planId
	if err := db.Model(&model.Coupon{}).Where("id = ?", coupon.Id).
		Update("applicable_plan_ids", "[123]").Error; err != nil {
		t.Fatalf("failed to update applicable_plan_ids: %v", err)
	}
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	result, err := svc.PreviewCouponUsageWithPlan(userId, userCoupon.Id, 1000, "subscription", &planId, "CNY")
	if err != nil {
		t.Fatalf("PreviewCouponUsageWithPlan failed: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected Valid=true, got false. Error: %s", result.ErrorMessage)
	}
	if result.DiscountAmount != 200 {
		t.Fatalf("expected DiscountAmount=200, got %d", result.DiscountAmount)
	}
	if result.FinalAmount != 800 {
		t.Fatalf("expected FinalAmount=800, got %d", result.FinalAmount)
	}
}

// ===================== 2.6.5 退款处理测试 =====================

// TestProcessRefundWithCoupon_Success 测试退款处理
func TestProcessRefundWithCoupon_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(5001)
	orderId := int64(1001)

	// 先创建原账单（退款需要更新原账单状态）
	couponId := int64(1)
	userCouponId := int64(1)
	sourceType := common.BillSourceTypeSubscriptionOrder
	description := "订阅购买"
	originalBill := &model.UserBill{
		UserId:         userId,
		BillType:       common.BillTypeSubscription,
		Amount:         -800,
		BalanceBefore:  1000,
		BalanceAfter:   200,
		SourceType:     &sourceType,
		SourceId:       &orderId,
		CouponId:       &couponId,
		UserCouponId:   &userCouponId,
		DiscountAmount: 200,
		OriginalAmount: 1000,
		FinalAmount:    800,
		Description:    &description,
		CreatedAt:      common.GetTimestamp(),
	}
	if err := db.Create(originalBill).Error; err != nil {
		t.Fatalf("failed to create original bill: %v", err)
	}

	req := &service.RefundWithCouponRequest{
		OrderId:        orderId,
		UserId:         userId,
		CouponId:       couponId,
		UserCouponId:   userCouponId,
		OriginalAmount: 1000,
		DiscountAmount: 200,
		FinalAmount:    800,
		RefundType:     "manual",
		RefundReason:   "用户申请退款",
		OperatorId:     1,
	}

	result, err := svc.ProcessRefundWithCoupon(req)
	if err != nil {
		t.Fatalf("ProcessRefundWithCoupon failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected Success = true, got false. Error: %s", result.ErrorMessage)
	}
	if result.RefundAmount != 800 {
		t.Errorf("expected RefundAmount = 800 (final amount), got %d", result.RefundAmount)
	}
	if result.CouponRestored {
		t.Error("expected CouponRestored = false (coupon should never be restored)")
	}
	if result.BillId == 0 {
		t.Error("expected BillId > 0")
	}

	// 验证原账单的 refund_type 已更新
	var updatedBill model.UserBill
	db.First(&updatedBill, originalBill.Id)
	if updatedBill.RefundType == nil || *updatedBill.RefundType != "manual" {
		t.Errorf("expected refund_type = 'manual', got %v", updatedBill.RefundType)
	}
}

// TestProcessRefundWithCoupon_InvalidParams 测试无效参数
func TestProcessRefundWithCoupon_InvalidParams(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 缺少订单 ID
	req := &service.RefundWithCouponRequest{
		UserId:      5002,
		FinalAmount: 800,
		RefundType:  "manual",
	}

	result, err := svc.ProcessRefundWithCoupon(req)
	if err == nil {
		t.Error("expected error for missing order ID")
	}
	if result.Success {
		t.Error("expected Success = false")
	}

	// 无效退款类型
	req = &service.RefundWithCouponRequest{
		OrderId:     1002,
		UserId:      5002,
		FinalAmount: 800,
		RefundType:  "invalid",
	}

	result, err = svc.ProcessRefundWithCoupon(req)
	if err == nil {
		t.Error("expected error for invalid refund type")
	}
}

// TestProcessRefundWithCoupon_NegativeAmount 测试负数退款金额
func TestProcessRefundWithCoupon_NegativeAmount(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	req := &service.RefundWithCouponRequest{
		OrderId:     1003,
		UserId:      5003,
		FinalAmount: -100,
		RefundType:  "manual",
	}

	result, err := svc.ProcessRefundWithCoupon(req)
	if err == nil {
		t.Error("expected error for negative refund amount")
	}
	if result.Success {
		t.Error("expected Success = false")
	}
}

// ===================== Service 层参数校验测试 =====================

// TestUseCoupon_InvalidParams 测试核销优惠券参数校验
func TestUseCoupon_InvalidParams(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 用户 ID 为空
	_, err := svc.UseCoupon(0, 1, 1, 1000, "subscription")
	if err == nil {
		t.Error("expected error for zero user ID")
	}

	// 用户优惠券 ID 为空
	_, err = svc.UseCoupon(1, 0, 1, 1000, "subscription")
	if err == nil {
		t.Error("expected error for zero user coupon ID")
	}

	// 订单 ID 为空
	_, err = svc.UseCoupon(1, 1, 0, 1000, "subscription")
	if err == nil {
		t.Error("expected error for zero order ID")
	}

	// 订单金额为 0
	_, err = svc.UseCoupon(1, 1, 1, 0, "subscription")
	if err == nil {
		t.Error("expected error for zero order amount")
	}

	// 场景为空
	_, err = svc.UseCoupon(1, 1, 1, 1000, "")
	if err == nil {
		t.Error("expected error for empty scene")
	}
}

// TestGetCouponByCode_Success 测试根据领取码获取优惠券
func TestGetCouponByCode_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 创建优惠券
	createTestCouponWithType(t, db, "GETCODE001", common.CouponTypeDiscount, 15, 0)

	// 获取
	coupon, err := svc.GetCouponByCode("GETCODE001")
	if err != nil {
		t.Fatalf("GetCouponByCode failed: %v", err)
	}

	if coupon.Code != "GETCODE001" {
		t.Errorf("expected code = GETCODE001, got %s", coupon.Code)
	}
}

// TestGetCouponByCode_NotFound 测试优惠券不存在
func TestGetCouponByCode_NotFound(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	_, err := svc.GetCouponByCode("NONEXISTENT")
	if err == nil {
		t.Error("expected error for non-existent coupon")
	}
}

// TestGetCouponByCode_EmptyCode 测试空优惠码
func TestGetCouponByCode_EmptyCode(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	_, err := svc.GetCouponByCode("")
	if err == nil {
		t.Error("expected error for empty code")
	}
}

// ===================== 2.6.2 UseCoupon 核销测试（补充） =====================

// TestUseCoupon_Success 测试成功核销优惠券
func TestUseCoupon_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(6001)
	orderId := int64(10001)
	orderAmount := int64(1000) // 1000 分

	// 创建 20% 折扣券
	coupon := createTestCouponWithType(t, db, "USECOUPON001", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	// 核销优惠券
	result, err := svc.UseCoupon(userId, userCoupon.Id, orderId, orderAmount, "subscription")
	if err != nil {
		t.Fatalf("UseCoupon failed: %v", err)
	}

	// 验证核销结果
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	// 1000 * 20 / 100 = 200
	if result.DiscountAmount != 200 {
		t.Errorf("expected DiscountAmount = 200, got %d", result.DiscountAmount)
	}
	if result.FinalAmount != 800 {
		t.Errorf("expected FinalAmount = 800, got %d", result.FinalAmount)
	}

	// 验证用户优惠券状态已更新
	var updatedUserCoupon model.UserCoupon
	db.First(&updatedUserCoupon, userCoupon.Id)
	if updatedUserCoupon.Status != common.UserCouponStatusUsed {
		t.Errorf("expected status = %s, got %s", common.UserCouponStatusUsed, updatedUserCoupon.Status)
	}
	if updatedUserCoupon.OrderId == nil || *updatedUserCoupon.OrderId != orderId {
		t.Errorf("expected order_id = %d, got %v", orderId, updatedUserCoupon.OrderId)
	}
	if updatedUserCoupon.DiscountAmount != 200 {
		t.Errorf("expected discount_amount = 200, got %d", updatedUserCoupon.DiscountAmount)
	}

	// 验证账单已创建
	var bill model.UserBill
	err = db.Where("user_id = ? AND bill_type = ?", userId, common.BillTypeCouponDiscount).First(&bill).Error
	if err != nil {
		t.Fatalf("failed to find bill: %v", err)
	}
	if bill.DiscountAmount != 200 {
		t.Errorf("expected bill discount_amount = 200, got %d", bill.DiscountAmount)
	}
	if bill.OriginalAmount != 1000 {
		t.Errorf("expected bill original_amount = 1000, got %d", bill.OriginalAmount)
	}
	if bill.FinalAmount != 800 {
		t.Errorf("expected bill final_amount = 800, got %d", bill.FinalAmount)
	}
}

// TestUseCoupon_Idempotent 测试核销幂等性（同订单重复核销应返回相同结果）
func TestUseCoupon_Idempotent(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(6002)
	orderId := int64(10002)
	orderAmount := int64(1000)

	// 创建 20% 折扣券
	coupon := createTestCouponWithType(t, db, "USECOUPON002", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	// 第一次核销
	result1, err := svc.UseCoupon(userId, userCoupon.Id, orderId, orderAmount, "subscription")
	if err != nil {
		t.Fatalf("first UseCoupon failed: %v", err)
	}

	// 第二次核销（相同订单号，应该幂等返回）
	result2, err := svc.UseCoupon(userId, userCoupon.Id, orderId, orderAmount, "subscription")
	if err != nil {
		t.Fatalf("second UseCoupon failed (should be idempotent): %v", err)
	}

	// 验证两次结果一致
	if result1.DiscountAmount != result2.DiscountAmount {
		t.Errorf("idempotent check: DiscountAmount mismatch: %d vs %d", result1.DiscountAmount, result2.DiscountAmount)
	}
	if result1.FinalAmount != result2.FinalAmount {
		t.Errorf("idempotent check: FinalAmount mismatch: %d vs %d", result1.FinalAmount, result2.FinalAmount)
	}

	// 验证账单只创建了一次（不应重复）
	var billCount int64
	db.Model(&model.UserBill{}).Where("user_id = ? AND bill_type = ?", userId, common.BillTypeCouponDiscount).Count(&billCount)
	if billCount != 1 {
		t.Errorf("expected 1 bill (idempotent), got %d", billCount)
	}
}

// TestUseCoupon_DifferentOrder_Fail 测试已使用优惠券不能用于不同订单
func TestUseCoupon_DifferentOrder_Fail(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(6003)
	orderId1 := int64(10003)
	orderId2 := int64(10004)
	orderAmount := int64(1000)

	// 创建折扣券
	coupon := createTestCouponWithType(t, db, "USECOUPON003", common.CouponTypeDiscount, 20, 0)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	// 第一次核销
	_, err := svc.UseCoupon(userId, userCoupon.Id, orderId1, orderAmount, "subscription")
	if err != nil {
		t.Fatalf("first UseCoupon failed: %v", err)
	}

	// 第二次核销（不同订单号，应该失败）
	_, err = svc.UseCoupon(userId, userCoupon.Id, orderId2, orderAmount, "subscription")
	if err == nil {
		t.Fatal("expected error for used coupon with different order")
	}

	expectedMsg := "优惠券已使用"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestUseCoupon_ScopeMismatch 测试作用域不匹配
func TestUseCoupon_ScopeMismatch(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(6004)

	// 创建仅适用于 wallet 的优惠券
	now := common.GetTimestamp()
	coupon := &model.Coupon{
		Code:          "SCOPE001",
		Name:          "钱包专用券",
		Type:          common.CouponTypeDiscount,
		Scope:         common.CouponScopeWallet, // 仅钱包
		DiscountValue: 20,
		TotalCount:    100,
		PerUserLimit:  5,
		ValidFrom:     now - 86400,
		ValidTo:       now + 86400*30,
		Status:        common.CouponStatusActive,
		CreatedBy:     1,
		Currency:      "CNY",
	}
	db.Create(coupon)
	userCoupon := createTestUserCoupon(t, db, userId, coupon.Id, common.UserCouponStatusAvailable)

	// 尝试在 subscription 场景使用
	_, err := svc.UseCoupon(userId, userCoupon.Id, 10005, 1000, "subscription")
	if err == nil {
		t.Fatal("expected error for scope mismatch")
	}

	expectedMsg := "优惠券不适用于当前场景"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// ===================== 2.6.3 绑定/解绑测试（补充） =====================

// createTestRedemption 创建测试兑换码
func createTestRedemption(t *testing.T, db *gorm.DB, name string, status int) *model.Redemption {
	now := common.GetTimestamp()
	redemption := &model.Redemption{
		Name:        name,
		Key:         "KEY-" + common.GetUUID()[:8],
		Status:      status,
		Quota:       1000,
		CreatedTime: now,
	}
	err := db.Create(redemption).Error
	if err != nil {
		t.Fatalf("failed to create test redemption: %v", err)
	}
	return redemption
}

// TestBindCouponToRedemption_Success 测试绑定优惠券到兑换码
func TestBindCouponToRedemption_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	operatorId := int64(1)

	// 创建优惠券和兑换码
	coupon := createTestCouponWithType(t, db, "BIND001", common.CouponTypeDiscount, 20, 0)
	redemption := createTestRedemption(t, db, "绑定测试兑换码", common.RedemptionCodeStatusEnabled)

	// 绑定
	result, err := svc.BindCouponToRedemption(coupon.Id, int64(redemption.Id), nil, operatorId)
	if err != nil {
		t.Fatalf("BindCouponToRedemption failed: %v", err)
	}

	// 验证结果
	if !result.Success {
		t.Errorf("expected Success = true, got false. Error: %s", result.ErrorMessage)
	}
	if result.BindingId == 0 {
		t.Error("expected BindingId > 0")
	}

	// 验证绑定记录已创建
	var binding model.CouponRedemptionBinding
	err = db.Where("redemption_id = ?", int64(redemption.Id)).First(&binding).Error
	if err != nil {
		t.Fatalf("failed to find binding: %v", err)
	}
	if binding.CouponId != coupon.Id {
		t.Errorf("expected coupon_id = %d, got %d", coupon.Id, binding.CouponId)
	}
	if binding.Status != common.CouponBindingStatusReserved {
		t.Errorf("expected status = %s, got %s", common.CouponBindingStatusReserved, binding.Status)
	}

	// 验证���计日志已创建
	var auditLog model.AuditLog
	err = db.Where("action = ? AND object_type = ?", "coupon_bind", "coupon_redemption_binding").First(&auditLog).Error
	if err != nil {
		t.Fatalf("failed to find audit log: %v", err)
	}
}

// TestBindCouponToRedemption_WithExclusiveUser 测试绑定带专属用户的优惠券
func TestBindCouponToRedemption_WithExclusiveUser(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	operatorId := int64(1)
	exclusiveUserId := int64(7001)

	// 创建优惠券和兑换码
	coupon := createTestCouponWithType(t, db, "BIND002", common.CouponTypeDiscount, 20, 0)
	redemption := createTestRedemption(t, db, "专属用户绑定测试", common.RedemptionCodeStatusEnabled)

	// 绑定（带专属用户）
	result, err := svc.BindCouponToRedemption(coupon.Id, int64(redemption.Id), &exclusiveUserId, operatorId)
	if err != nil {
		t.Fatalf("BindCouponToRedemption failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected Success = true, got false. Error: %s", result.ErrorMessage)
	}

	// 验证专属用户已设置
	var binding model.CouponRedemptionBinding
	db.Where("redemption_id = ?", int64(redemption.Id)).First(&binding)
	if binding.UserId == nil || *binding.UserId != exclusiveUserId {
		t.Errorf("expected user_id = %d, got %v", exclusiveUserId, binding.UserId)
	}
}

// TestBindCouponToRedemption_DuplicateBinding 测试重复绑定
func TestBindCouponToRedemption_DuplicateBinding(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	operatorId := int64(1)

	// 创建优惠券和兑换码
	coupon1 := createTestCouponWithType(t, db, "BIND003", common.CouponTypeDiscount, 20, 0)
	coupon2 := createTestCouponWithType(t, db, "BIND004", common.CouponTypeDiscount, 30, 0)
	redemption := createTestRedemption(t, db, "重复绑定测试", common.RedemptionCodeStatusEnabled)

	// 第一次绑定
	_, err := svc.BindCouponToRedemption(coupon1.Id, int64(redemption.Id), nil, operatorId)
	if err != nil {
		t.Fatalf("first bind failed: %v", err)
	}

	// 第二次绑定（同一兑换码，应该失败）
	result, err := svc.BindCouponToRedemption(coupon2.Id, int64(redemption.Id), nil, operatorId)
	if err == nil {
		t.Fatal("expected error for duplicate binding")
	}
	if result.Success {
		t.Error("expected Success = false for duplicate binding")
	}
}

// TestUnbindCoupon_Success 测试解绑优惠券
func TestUnbindCoupon_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	operatorId := int64(1)

	// 创建优惠券、兑换码并绑定
	coupon := createTestCouponWithType(t, db, "UNBIND001", common.CouponTypeDiscount, 20, 0)
	redemption := createTestRedemption(t, db, "解绑测试兑换码", common.RedemptionCodeStatusEnabled)

	// 先绑定
	_, err := svc.BindCouponToRedemption(coupon.Id, int64(redemption.Id), nil, operatorId)
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	// 解绑
	err = svc.UnbindCoupon(int64(redemption.Id), operatorId)
	if err != nil {
		t.Fatalf("UnbindCoupon failed: %v", err)
	}

	// 验证绑定记录已删除
	var binding model.CouponRedemptionBinding
	err = db.Where("redemption_id = ?", int64(redemption.Id)).First(&binding).Error
	if err == nil {
		t.Error("expected binding to be deleted")
	}

	// 验证审计日志已创建
	var auditLog model.AuditLog
	err = db.Where("action = ?", "coupon_unbind").First(&auditLog).Error
	if err != nil {
		t.Fatalf("failed to find unbind audit log: %v", err)
	}
}

// TestUnbindCoupon_LockedBinding 测试解绑已锁定的绑定
func TestUnbindCoupon_LockedBinding(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	operatorId := int64(1)

	// 创建优惠券、兑换码并绑定
	coupon := createTestCouponWithType(t, db, "UNBIND002", common.CouponTypeDiscount, 20, 0)
	redemption := createTestRedemption(t, db, "已锁定解绑测试", common.RedemptionCodeStatusEnabled)

	// 绑定
	_, err := svc.BindCouponToRedemption(coupon.Id, int64(redemption.Id), nil, operatorId)
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	// 手动将状态改为 locked
	db.Model(&model.CouponRedemptionBinding{}).
		Where("redemption_id = ?", int64(redemption.Id)).
		Update("status", common.CouponBindingStatusLocked)

	// 尝试解绑（应该失败）
	err = svc.UnbindCoupon(int64(redemption.Id), operatorId)
	if err == nil {
		t.Fatal("expected error for unbinding locked binding")
	}

	expectedMsg := "无法解绑：绑定已锁定"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestUnbindCoupon_NotFound 测试解绑不存在的绑定
func TestUnbindCoupon_NotFound(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	err := svc.UnbindCoupon(99999, 1)
	if err == nil {
		t.Fatal("expected error for non-existent binding")
	}
}

// ===================== UseBoundCoupon 测试 =====================

// TestUseBoundCoupon_Success 测试使用绑定的优惠券
func TestUseBoundCoupon_Success(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(8001)
	orderId := int64(20001)
	orderAmount := int64(1000)
	operatorId := int64(1)

	// 创建优惠券、兑换码并绑定
	coupon := createTestCouponWithType(t, db, "USEBOUND001", common.CouponTypeDiscount, 20, 0)
	redemption := createTestRedemption(t, db, "使用绑定测试", common.RedemptionCodeStatusEnabled)

	_, err := svc.BindCouponToRedemption(coupon.Id, int64(redemption.Id), nil, operatorId)
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	// 使用绑定的优惠券
	result, err := svc.UseBoundCoupon(userId, int64(redemption.Id), orderId, orderAmount, "subscription")
	if err != nil {
		t.Fatalf("UseBoundCoupon failed: %v", err)
	}

	// 验证结果
	if !result.HasBinding {
		t.Error("expected HasBinding = true")
	}
	if !result.CouponUsed {
		t.Error("expected CouponUsed = true")
	}
	// 1000 * 20 / 100 = 200
	if result.DiscountAmount != 200 {
		t.Errorf("expected DiscountAmount = 200, got %d", result.DiscountAmount)
	}
	if result.FinalAmount != 800 {
		t.Errorf("expected FinalAmount = 800, got %d", result.FinalAmount)
	}

	// 验证绑定状态已更新为 locked
	var binding model.CouponRedemptionBinding
	db.Where("redemption_id = ?", int64(redemption.Id)).First(&binding)
	if binding.Status != common.CouponBindingStatusLocked {
		t.Errorf("expected binding status = %s, got %s", common.CouponBindingStatusLocked, binding.Status)
	}

	// 验证用户优惠券已创建并使用
	var userCoupon model.UserCoupon
	db.Where("user_id = ? AND coupon_id = ?", userId, coupon.Id).First(&userCoupon)
	if userCoupon.Status != common.UserCouponStatusUsed {
		t.Errorf("expected user coupon status = %s, got %s", common.UserCouponStatusUsed, userCoupon.Status)
	}
}

// TestUseBoundCoupon_NoBinding 测试无绑定时使用
func TestUseBoundCoupon_NoBinding(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	userId := int64(8002)
	orderId := int64(20002)
	orderAmount := int64(1000)

	// 使用不存在的兑换码
	result, err := svc.UseBoundCoupon(userId, 99999, orderId, orderAmount, "subscription")
	if err != nil {
		t.Fatalf("UseBoundCoupon failed: %v", err)
	}

	// 无绑定应该正常返回
	if result.HasBinding {
		t.Error("expected HasBinding = false")
	}
	if result.CouponUsed {
		t.Error("expected CouponUsed = false")
	}
	if result.FinalAmount != orderAmount {
		t.Errorf("expected FinalAmount = %d (unchanged), got %d", orderAmount, result.FinalAmount)
	}
}

// TestUseBoundCoupon_ExclusiveUserMismatch 测试专属用户不匹配
func TestUseBoundCoupon_ExclusiveUserMismatch(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()
	exclusiveUserId := int64(8003)
	otherUserId := int64(8004)
	orderId := int64(20003)
	orderAmount := int64(1000)
	operatorId := int64(1)

	// 创建优惠券、兑换码并绑定（带专属用户）
	coupon := createTestCouponWithType(t, db, "USEBOUND002", common.CouponTypeDiscount, 20, 0)
	redemption := createTestRedemption(t, db, "专属用户测试", common.RedemptionCodeStatusEnabled)

	_, err := svc.BindCouponToRedemption(coupon.Id, int64(redemption.Id), &exclusiveUserId, operatorId)
	if err != nil {
		t.Fatalf("bind failed: %v", err)
	}

	// 其他用户尝试使用
	result, err := svc.UseBoundCoupon(otherUserId, int64(redemption.Id), orderId, orderAmount, "subscription")
	if err == nil {
		t.Fatal("expected error for exclusive user mismatch")
	}

	if result.CouponUsed {
		t.Error("expected CouponUsed = false for user mismatch")
	}

	expectedMsg := "该优惠券仅限指定用户使用"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestUseBoundCoupon_ZeroRedemptionId 测试零兑换码 ID
func TestUseBoundCoupon_ZeroRedemptionId(t *testing.T) {
	_, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// redemptionId = 0 应该直接返回无绑定
	result, err := svc.UseBoundCoupon(8005, 0, 20004, 1000, "subscription")
	if err != nil {
		t.Fatalf("UseBoundCoupon failed: %v", err)
	}

	if result.HasBinding {
		t.Error("expected HasBinding = false for zero redemption ID")
	}
}

// ===================== 修复 2.16：绑定券禁止普通流程测试 =====================

// TestBoundCoupon_ClaimRejected 测试绑定到兑换码的优惠券不能通过普通流程领取
func TestBoundCoupon_ClaimRejected(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-CLAIM-TEST", 10, 1, 0, 0)

	// 2. 创建绑定记录（绑定到兑换码）
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99001, // 假设的兑换码 ID
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 3. 尝试通过普通流程领取，应被拒绝
	_, err = svc.ClaimCoupon(1001, coupon.Code)
	if err == nil {
		t.Fatal("expected error when claiming bound coupon, got nil")
	}

	expectedMsg := "该优惠券仅限随兑换码一起使用"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestBoundCoupon_ValidateClaimableRejected 测试绑定到兑换码的优惠券校验时被拒绝
func TestBoundCoupon_ValidateClaimableRejected(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-VALIDATE-TEST", 10, 1, 0, 0)

	// 2. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99002,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 3. 校验时应返回绑定券错误码
	result := svc.ValidateCouponClaimable(coupon.Code)
	if result.Valid {
		t.Fatal("expected Valid = false for bound coupon")
	}

	if result.ErrorCode != "COUPON_BOUND_TO_REDEMPTION" {
		t.Errorf("expected error code 'COUPON_BOUND_TO_REDEMPTION', got '%s'", result.ErrorCode)
	}
}

// TestBoundCoupon_PreviewRejected 测试绑定到兑换码的优惠券预览时被拒绝
func TestBoundCoupon_PreviewRejected(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-PREVIEW-TEST", 10, 1, 0, 0)

	// 2. 创建用户优惠券（模拟已领取）
	userCoupon := createTestUserCoupon(t, db, 1002, coupon.Id, common.UserCouponStatusAvailable)

	// 3. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99003,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 4. 预览时应返回绑定券错误码
	result, err := svc.PreviewCouponUsage(1002, userCoupon.Id, 1000, "subscription")
	if err != nil {
		t.Fatalf("PreviewCouponUsage returned error: %v", err)
	}

	if result.Valid {
		t.Fatal("expected Valid = false for bound coupon preview")
	}

	if result.ErrorCode != "COUPON_BOUND_TO_REDEMPTION" {
		t.Errorf("expected error code 'COUPON_BOUND_TO_REDEMPTION', got '%s'", result.ErrorCode)
	}
}

// TestBoundCoupon_UseRejected 测试绑定到兑换码的优惠券核销时被拒绝（非 locked 状态）
func TestBoundCoupon_UseRejected(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-USE-TEST", 10, 1, 0, 0)

	// 2. 创建用户优惠券（状态为 available，不是 locked）
	userCoupon := createTestUserCoupon(t, db, 1003, coupon.Id, common.UserCouponStatusAvailable)

	// 3. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99004,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 4. 核销时应被拒绝
	err = db.Transaction(func(tx *gorm.DB) error {
		_, usageErr := model.UseCouponWithPlanTx(tx, 1003, userCoupon.Id, 30001, 1000, "subscription", nil)
		return usageErr
	})

	if err == nil {
		t.Fatal("expected error when using bound coupon, got nil")
	}

	expectedMsg := "该优惠券仅限随兑换码一起使用"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestBoundCoupon_LockedStatusAllowsUse 测试已锁定状态的绑定券可以核销
func TestBoundCoupon_LockedStatusAllowsUse(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-LOCKED-TEST", 10, 1, 0, 0)

	// 2. 创建用户优惠券（状态为 locked，表示已被兑换流程锁定）
	userCoupon := createTestUserCoupon(t, db, 1004, coupon.Id, common.UserCouponStatusLocked)

	// 3. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99005,
		Status:       common.CouponBindingStatusLocked,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 4. 已锁定的绑定券应该可以核销（兑换流程中）
	err = db.Transaction(func(tx *gorm.DB) error {
		result, usageErr := model.UseCouponWithPlanTx(tx, 1004, userCoupon.Id, 30002, 1000, "subscription", nil)
		if usageErr != nil {
			return usageErr
		}
		// 验证核销成功
		if result.DiscountAmount <= 0 {
			return fmt.Errorf("expected discount > 0, got %d", result.DiscountAmount)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("expected locked bound coupon to be usable, got error: %v", err)
	}
}

// TestBoundCoupon_UseCouponRejected 测试绑定券通过 model.UseCoupon 核销时被拒绝
func TestBoundCoupon_UseCouponRejected(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-USECOUPON-TEST", 10, 1, 0, 0)

	// 2. 创建用户优惠券（状态为 available）
	userCoupon := createTestUserCoupon(t, db, 1010, coupon.Id, common.UserCouponStatusAvailable)

	// 3. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99010,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 4. 通过 UseCoupon 核销时应被拒绝
	_, err = model.UseCoupon(1010, userCoupon.Id, 40001, 1000, "wallet")

	if err == nil {
		t.Fatal("expected error when using bound coupon via UseCoupon, got nil")
	}

	expectedMsg := "该优惠券仅限随兑换码一起使用"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestBoundCoupon_UseCouponWithTxRejected 测试绑定券通过 model.UseCouponWithTx 核销时被拒绝
func TestBoundCoupon_UseCouponWithTxRejected(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-USECOUPONTX-TEST", 10, 1, 0, 0)

	// 2. 创建用户优惠券（状态为 available）
	userCoupon := createTestUserCoupon(t, db, 1011, coupon.Id, common.UserCouponStatusAvailable)

	// 3. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99011,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 4. 通过 UseCouponWithTx 核销时应被拒绝
	err = db.Transaction(func(tx *gorm.DB) error {
		_, usageErr := model.UseCouponWithTx(tx, 1011, userCoupon.Id, 40002, 1000, "wallet")
		return usageErr
	})

	if err == nil {
		t.Fatal("expected error when using bound coupon via UseCouponWithTx, got nil")
	}

	expectedMsg := "该优惠券仅限随兑换码一起使用"
	if err.Error() != expectedMsg {
		t.Errorf("expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestBoundCoupon_UseCouponWithTxLockedAllowed 测试锁定状态的绑定券可以通过 UseCouponWithTx 核销
func TestBoundCoupon_UseCouponWithTxLockedAllowed(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	// 1. 创建测试优惠券
	coupon := createTestCoupon(t, db, "BOUND-USECOUPONTX-LOCKED", 10, 1, 0, 0)

	// 2. 创建用户优惠券（状态为 locked，模拟兑换码流程锁定）
	userCoupon := createTestUserCoupon(t, db, 1012, coupon.Id, common.UserCouponStatusLocked)

	// 3. 创建绑定记录
	now := common.GetTimestamp()
	binding := &model.CouponRedemptionBinding{
		CouponId:     coupon.Id,
		RedemptionId: 99012,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	err := db.Create(binding).Error
	if err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	// 4. 锁定状态的绑定券应该可以核销
	err = db.Transaction(func(tx *gorm.DB) error {
		_, usageErr := model.UseCouponWithTx(tx, 1012, userCoupon.Id, 40003, 1000, "wallet")
		return usageErr
	})

	if err != nil {
		t.Fatalf("expected locked bound coupon to be usable via UseCouponWithTx, got error: %v", err)
	}
}

// TestUnboundCoupon_ClaimAllowed 测试未绑定的优惠券可以正常领取（回归测试）
func TestUnboundCoupon_ClaimAllowed(t *testing.T) {
	db, cleanup := setupCouponTestDBWithAll(t)
	defer cleanup()

	svc := service.GetCouponService()

	// 1. 创建测试优惠券（不创建绑定记录）
	coupon := createTestCoupon(t, db, "UNBOUND-CLAIM-TEST", 10, 1, 0, 0)

	// 2. 应该可以正常领取
	userCoupon, err := svc.ClaimCoupon(2001, coupon.Code)
	if err != nil {
		t.Fatalf("expected unbound coupon to be claimable, got error: %v", err)
	}

	if userCoupon == nil {
		t.Fatal("expected userCoupon to be non-nil")
	}

	if userCoupon.CouponId != coupon.Id {
		t.Errorf("expected coupon ID = %d, got %d", coupon.Id, userCoupon.CouponId)
	}
}

package service

import (
	"errors"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupPriorityTestDB 设置优先级测试用的 SQLite 数据库
func setupPriorityTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	prevDB := model.DB
	prevSQLitePath := common.SQLitePath
	prevUsingSQLite := common.UsingSQLite
	prevUsingMySQL := common.UsingMySQL
	prevUsingPostgreSQL := common.UsingPostgreSQL
	prevRedisEnabled := common.RedisEnabled

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "subscription_priority_test.db")
	common.SQLitePath = dbPath + "?_busy_timeout=30000"

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false // 测试时禁用 Redis

	db, err := gorm.Open(sqlite.Open(common.SQLitePath), &gorm.Config{PrepareStmt: true})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql DB failed: %v", err)
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(20)
	sqlDB.SetConnMaxLifetime(time.Minute)

	// 创建必要的表
	if err := db.AutoMigrate(&model.Subscription{}, &model.SubscriptionPlan{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	model.DB = db

	cleanup := func() {
		sqlDB.Close()
		model.DB = prevDB
		common.SQLitePath = prevSQLitePath
		common.UsingSQLite = prevUsingSQLite
		common.UsingMySQL = prevUsingMySQL
		common.UsingPostgreSQL = prevUsingPostgreSQL
		common.RedisEnabled = prevRedisEnabled
	}

	return db, cleanup
}

// createTestSubscription 创建测试订阅
func createTestSubscription(t *testing.T, db *gorm.DB, userId int64, priority int, endAt int64, createdAt int64) *model.Subscription {
	t.Helper()
	sub := &model.Subscription{
		UserId:             userId,
		PlanId:             1,
		Status:             common.SubscriptionStatusActive,
		StartAt:            common.GetTimestamp() - 3600,
		EndAt:              endAt,
		Priority:           priority,
		AutoWalletFallback: false,
		RedeemOption:       common.RedeemOptionStack,
		CreatedAt:          createdAt,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create subscription failed: %v", err)
	}
	return sub
}

// isPriorityConflict 检查是否是优先级冲突错误
func isPriorityConflict(err error) bool {
	var apiErr *types.NewAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.GetErrorCode() == types.ErrorCodeSubscriptionPriorityConflict && apiErr.StatusCode == http.StatusConflict
}

// ===================== 动态插入测试 =====================

// TestDynamicInsertion_InsertAtBeginning 测试插入到队列开头
func TestDynamicInsertion_InsertAtBeginning(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建两个已有订阅：end_at 较晚
	sub1 := createTestSubscription(t, db, userId, 1, now+7*86400, now-200) // 7天后到期
	sub2 := createTestSubscription(t, db, userId, 2, now+14*86400, now-100) // 14天后到期

	// 新订阅的 end_at 比现有所有订阅都早，应插入到开头
	newSub := &model.Subscription{
		UserId:    userId,
		PlanId:    1,
		Status:    common.SubscriptionStatusActive,
		StartAt:   now,
		EndAt:     now + 1*86400, // 1天后到期（最早）
		Priority:  0, // 待初始化
		CreatedAt: now,
	}
	if err := db.Create(newSub).Error; err != nil {
		t.Fatalf("create new subscription failed: %v", err)
	}

	// 初始化优先级
	err := svc.InitializeSubscriptionPriority(newSub)
	if err != nil {
		t.Fatalf("InitializeSubscriptionPriority failed: %v", err)
	}

	// 验证新订阅的优先级为 1
	if newSub.Priority != 1 {
		t.Errorf("expected newSub.Priority = 1, got %d", newSub.Priority)
	}

	// 验证新订阅的 priority 已写入数据库（reload 验证）
	var reloadedNewSub model.Subscription
	if err := db.First(&reloadedNewSub, newSub.Id).Error; err != nil {
		t.Fatalf("reload new subscription failed: %v", err)
	}
	if reloadedNewSub.Priority != 1 {
		t.Errorf("reloaded newSub.Priority should be 1, got %d", reloadedNewSub.Priority)
	}

	// 验证其他订阅的优先级被正确推移
	var updatedSub1, updatedSub2 model.Subscription
	db.First(&updatedSub1, sub1.Id)
	db.First(&updatedSub2, sub2.Id)

	if updatedSub1.Priority != 2 {
		t.Errorf("expected sub1.Priority = 2, got %d", updatedSub1.Priority)
	}
	if updatedSub2.Priority != 3 {
		t.Errorf("expected sub2.Priority = 3, got %d", updatedSub2.Priority)
	}
}

// TestDynamicInsertion_InsertInMiddle 测试插入到队列中间
func TestDynamicInsertion_InsertInMiddle(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建三个已有订阅
	sub1 := createTestSubscription(t, db, userId, 1, now+1*86400, now-300)  // 1天后到期
	sub2 := createTestSubscription(t, db, userId, 2, now+7*86400, now-200)  // 7天后到期
	sub3 := createTestSubscription(t, db, userId, 3, now+14*86400, now-100) // 14天后到期

	// 新订阅 end_at 在 sub1 和 sub2 之间，应插入到 sub1 之后、sub2 之前
	newSub := &model.Subscription{
		UserId:    userId,
		PlanId:    1,
		Status:    common.SubscriptionStatusActive,
		StartAt:   now,
		EndAt:     now + 3*86400, // 3天后到期
		Priority:  0,
		CreatedAt: now,
	}
	if err := db.Create(newSub).Error; err != nil {
		t.Fatalf("create new subscription failed: %v", err)
	}

	err := svc.InitializeSubscriptionPriority(newSub)
	if err != nil {
		t.Fatalf("InitializeSubscriptionPriority failed: %v", err)
	}

	// 验证优先级
	if newSub.Priority != 2 {
		t.Errorf("expected newSub.Priority = 2, got %d", newSub.Priority)
	}

	var updatedSub1, updatedSub2, updatedSub3 model.Subscription
	db.First(&updatedSub1, sub1.Id)
	db.First(&updatedSub2, sub2.Id)
	db.First(&updatedSub3, sub3.Id)

	if updatedSub1.Priority != 1 {
		t.Errorf("expected sub1.Priority = 1, got %d", updatedSub1.Priority)
	}
	if updatedSub2.Priority != 3 {
		t.Errorf("expected sub2.Priority = 3, got %d", updatedSub2.Priority)
	}
	if updatedSub3.Priority != 4 {
		t.Errorf("expected sub3.Priority = 4, got %d", updatedSub3.Priority)
	}
}

// TestDynamicInsertion_InsertAtEnd 测试插入到队列末尾
func TestDynamicInsertion_InsertAtEnd(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建两个已有订阅
	sub1 := createTestSubscription(t, db, userId, 1, now+1*86400, now-200) // 1天后到期
	sub2 := createTestSubscription(t, db, userId, 2, now+7*86400, now-100) // 7天后到期

	// 新订阅 end_at 最晚，应插入到末尾
	newSub := &model.Subscription{
		UserId:    userId,
		PlanId:    1,
		Status:    common.SubscriptionStatusActive,
		StartAt:   now,
		EndAt:     now + 30*86400, // 30天后到期（最晚）
		Priority:  0,
		CreatedAt: now,
	}
	if err := db.Create(newSub).Error; err != nil {
		t.Fatalf("create new subscription failed: %v", err)
	}

	err := svc.InitializeSubscriptionPriority(newSub)
	if err != nil {
		t.Fatalf("InitializeSubscriptionPriority failed: %v", err)
	}

	// 验证新订阅优先级为 3（末尾）
	if newSub.Priority != 3 {
		t.Errorf("expected newSub.Priority = 3, got %d", newSub.Priority)
	}

	// 原有订阅优先级不变
	var updatedSub1, updatedSub2 model.Subscription
	db.First(&updatedSub1, sub1.Id)
	db.First(&updatedSub2, sub2.Id)

	if updatedSub1.Priority != 1 {
		t.Errorf("expected sub1.Priority = 1, got %d", updatedSub1.Priority)
	}
	if updatedSub2.Priority != 2 {
		t.Errorf("expected sub2.Priority = 2, got %d", updatedSub2.Priority)
	}
}

// TestDynamicInsertion_SameEndAtByCreatedAt 测试相同 end_at 时按 created_at 排序
func TestDynamicInsertion_SameEndAtByCreatedAt(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()
	sameEndAt := now + 7*86400

	// 创建两个 end_at 相同的订阅
	sub1 := createTestSubscription(t, db, userId, 1, sameEndAt, now-200) // 创建时间较早
	sub2 := createTestSubscription(t, db, userId, 2, sameEndAt, now-100) // 创建时间较晚

	// 新订阅 end_at 相同，created_at 在 sub1 和 sub2 之间
	newSub := &model.Subscription{
		UserId:    userId,
		PlanId:    1,
		Status:    common.SubscriptionStatusActive,
		StartAt:   now,
		EndAt:     sameEndAt,
		Priority:  0,
		CreatedAt: now - 150, // 在 sub1 和 sub2 之间
	}
	if err := db.Create(newSub).Error; err != nil {
		t.Fatalf("create new subscription failed: %v", err)
	}

	err := svc.InitializeSubscriptionPriority(newSub)
	if err != nil {
		t.Fatalf("InitializeSubscriptionPriority failed: %v", err)
	}

	// 验证新订阅插入到 sub1 之后、sub2 之前
	if newSub.Priority != 2 {
		t.Errorf("expected newSub.Priority = 2, got %d", newSub.Priority)
	}

	var updatedSub1, updatedSub2 model.Subscription
	db.First(&updatedSub1, sub1.Id)
	db.First(&updatedSub2, sub2.Id)

	if updatedSub1.Priority != 1 {
		t.Errorf("expected sub1.Priority = 1, got %d", updatedSub1.Priority)
	}
	if updatedSub2.Priority != 3 {
		t.Errorf("expected sub2.Priority = 3, got %d", updatedSub2.Priority)
	}
}

// ===================== 排序测试 =====================

// TestSorting_PriorityFirst 测试排序：优先级优先
func TestSorting_PriorityFirst(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建订阅：end_at 相同但 priority 不同
	createTestSubscription(t, db, userId, 3, now+7*86400, now-100)
	createTestSubscription(t, db, userId, 1, now+7*86400, now-200)
	createTestSubscription(t, db, userId, 2, now+7*86400, now-150)

	sorted, err := svc.GetSortedSubscriptions(userId)
	if err != nil {
		t.Fatalf("GetSortedSubscriptions failed: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(sorted))
	}

	// 验证按 priority 升序排列
	if sorted[0].Priority != 1 || sorted[1].Priority != 2 || sorted[2].Priority != 3 {
		t.Errorf("expected priorities [1,2,3], got [%d,%d,%d]", sorted[0].Priority, sorted[1].Priority, sorted[2].Priority)
	}
}

// TestSorting_EndAtSecond 测试排序：priority 相同时按 end_at 排序
func TestSorting_EndAtSecond(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建订阅：priority 相同但 end_at 不同
	sub1 := createTestSubscription(t, db, userId, 1, now+14*86400, now-100) // end_at 最晚
	sub2 := createTestSubscription(t, db, userId, 1, now+1*86400, now-200)  // end_at 最早
	sub3 := createTestSubscription(t, db, userId, 1, now+7*86400, now-150)  // end_at 中间

	sorted, err := svc.GetSortedSubscriptions(userId)
	if err != nil {
		t.Fatalf("GetSortedSubscriptions failed: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(sorted))
	}

	// 验证按 end_at 升序排列（priority 相同时）
	if sorted[0].Id != sub2.Id || sorted[1].Id != sub3.Id || sorted[2].Id != sub1.Id {
		t.Errorf("expected order [sub2,sub3,sub1], got [%d,%d,%d]", sorted[0].Id, sorted[1].Id, sorted[2].Id)
	}
}

// TestSorting_CreatedAtThird 测试排序：priority 和 end_at 相同时按 created_at 排序
func TestSorting_CreatedAtThird(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()
	sameEndAt := now + 7*86400

	// 创建订阅：priority 和 end_at 都相同，但 created_at 不同
	sub1 := createTestSubscription(t, db, userId, 1, sameEndAt, now-100) // created_at 最新
	sub2 := createTestSubscription(t, db, userId, 1, sameEndAt, now-300) // created_at 最早
	sub3 := createTestSubscription(t, db, userId, 1, sameEndAt, now-200) // created_at 中间

	sorted, err := svc.GetSortedSubscriptions(userId)
	if err != nil {
		t.Fatalf("GetSortedSubscriptions failed: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(sorted))
	}

	// 验证按 created_at 升序排列
	if sorted[0].Id != sub2.Id || sorted[1].Id != sub3.Id || sorted[2].Id != sub1.Id {
		t.Errorf("expected order [sub2,sub3,sub1] by created_at, got [%d,%d,%d]", sorted[0].Id, sorted[1].Id, sorted[2].Id)
	}
}

// ===================== 冲突检测测试 =====================

// TestPriorityConflict_UpdateSingleSubscription 测试单个订阅更新时的冲突检测
func TestPriorityConflict_UpdateSingleSubscription(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建两个订阅
	_ = createTestSubscription(t, db, userId, 1, now+7*86400, now-200)  // sub1 作为冲突目标
	sub2 := createTestSubscription(t, db, userId, 2, now+14*86400, now-100)

	// 尝试将 sub2 的优先级改为 1（与 sub1 冲突）
	err := svc.UpdateUserPriority(userId, sub2.Id, 1)
	if err == nil {
		t.Fatal("expected priority conflict error, got nil")
	}

	if !isPriorityConflict(err) {
		t.Errorf("expected 409 conflict error, got: %v", err)
	}

	// 验证数据库中 sub2 的优先级未改变
	var unchanged model.Subscription
	db.First(&unchanged, sub2.Id)
	if unchanged.Priority != 2 {
		t.Errorf("sub2.Priority should remain 2, got %d", unchanged.Priority)
	}
}

// TestPriorityConflict_BatchUpdate 测试批量更新时的冲突检测
func TestPriorityConflict_BatchUpdate(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建三个订阅
	sub1 := createTestSubscription(t, db, userId, 1, now+7*86400, now-300)
	sub2 := createTestSubscription(t, db, userId, 2, now+14*86400, now-200)
	sub3 := createTestSubscription(t, db, userId, 3, now+21*86400, now-100)

	// 批量更新：sub1 -> 10, sub2 -> 20，但 sub3 保持 3
	// 这应该成功（无冲突）
	err := svc.BatchUpdatePriorities(userId, map[int64]int{
		sub1.Id: 10,
		sub2.Id: 20,
	})
	if err != nil {
		t.Fatalf("BatchUpdatePriorities should succeed: %v", err)
	}

	// 验证更新成功
	var updated1, updated2 model.Subscription
	db.First(&updated1, sub1.Id)
	db.First(&updated2, sub2.Id)
	if updated1.Priority != 10 || updated2.Priority != 20 {
		t.Errorf("expected priorities [10,20], got [%d,%d]", updated1.Priority, updated2.Priority)
	}

	// 尝试批量更新：sub1 -> 3（与未更新的 sub3 冲突）
	err = svc.BatchUpdatePriorities(userId, map[int64]int{
		sub1.Id: sub3.Priority, // 冲突：sub3 的优先级也是 3
	})
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	if !isPriorityConflict(err) {
		t.Errorf("expected 409 conflict error, got: %v", err)
	}
}

// TestPriorityConflict_BatchUpdateInternalDuplicate 测试批量更新内部重复
func TestPriorityConflict_BatchUpdateInternalDuplicate(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	sub1 := createTestSubscription(t, db, userId, 1, now+7*86400, now-200)
	sub2 := createTestSubscription(t, db, userId, 2, now+14*86400, now-100)

	// 批量更新：sub1 和 sub2 都设为优先级 5（内部冲突）
	err := svc.BatchUpdatePriorities(userId, map[int64]int{
		sub1.Id: 5,
		sub2.Id: 5, // 重复
	})
	if err == nil {
		t.Fatal("expected internal conflict error, got nil")
	}
	if !isPriorityConflict(err) {
		t.Errorf("expected 409 conflict error, got: %v", err)
	}
}

// ===================== 重排序完整性测试 =====================

// TestReorderSubscriptions_IncompleteList 测试重排序列表不完整
func TestReorderSubscriptions_IncompleteList(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建三个订阅
	sub1 := createTestSubscription(t, db, userId, 1, now+7*86400, now-300)
	sub2 := createTestSubscription(t, db, userId, 2, now+14*86400, now-200)
	_ = createTestSubscription(t, db, userId, 3, now+21*86400, now-100) // sub3 不在列表中

	// 只传入两个 ID，缺少 sub3
	err := svc.ReorderSubscriptions(userId, []int64{sub2.Id, sub1.Id})
	if err == nil {
		t.Fatal("expected incomplete list error, got nil")
	}
	if !isPriorityConflict(err) {
		t.Errorf("expected 409 conflict error for incomplete list, got: %v", err)
	}
}

// TestReorderSubscriptions_DuplicateIds 测试重排序列表有重复ID
func TestReorderSubscriptions_DuplicateIds(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	sub1 := createTestSubscription(t, db, userId, 1, now+7*86400, now-200)
	_ = createTestSubscription(t, db, userId, 2, now+14*86400, now-100) // 第二个订阅未在列表中

	// 传入重复的 ID
	err := svc.ReorderSubscriptions(userId, []int64{sub1.Id, sub1.Id})
	if err == nil {
		t.Fatal("expected duplicate id error, got nil")
	}
	if !isPriorityConflict(err) {
		t.Errorf("expected 409 conflict error for duplicate ids, got: %v", err)
	}
}

// TestReorderSubscriptions_Success 测试重排序成功场景
func TestReorderSubscriptions_Success(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	sub1 := createTestSubscription(t, db, userId, 1, now+7*86400, now-300)
	sub2 := createTestSubscription(t, db, userId, 2, now+14*86400, now-200)
	sub3 := createTestSubscription(t, db, userId, 3, now+21*86400, now-100)

	// 重排序：sub3 -> sub1 -> sub2
	err := svc.ReorderSubscriptions(userId, []int64{sub3.Id, sub1.Id, sub2.Id})
	if err != nil {
		t.Fatalf("ReorderSubscriptions failed: %v", err)
	}

	// 验证新的优先级
	var updated1, updated2, updated3 model.Subscription
	db.First(&updated1, sub1.Id)
	db.First(&updated2, sub2.Id)
	db.First(&updated3, sub3.Id)

	if updated3.Priority != 1 {
		t.Errorf("expected sub3.Priority = 1, got %d", updated3.Priority)
	}
	if updated1.Priority != 2 {
		t.Errorf("expected sub1.Priority = 2, got %d", updated1.Priority)
	}
	if updated2.Priority != 3 {
		t.Errorf("expected sub2.Priority = 3, got %d", updated2.Priority)
	}
}

// ===================== 规范化测试 =====================

// TestNormalizePriorities 测试优先级规范化
func TestNormalizePriorities(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建优先级不连续的订阅
	createTestSubscription(t, db, userId, 5, now+7*86400, now-300)
	createTestSubscription(t, db, userId, 10, now+14*86400, now-200)
	createTestSubscription(t, db, userId, 100, now+21*86400, now-100)

	// 规范化
	err := svc.NormalizePriorities(userId)
	if err != nil {
		t.Fatalf("NormalizePriorities failed: %v", err)
	}

	// 验证优先级变为连续的 1, 2, 3
	sorted, err := svc.GetSortedSubscriptions(userId)
	if err != nil {
		t.Fatalf("GetSortedSubscriptions failed: %v", err)
	}

	for i, sub := range sorted {
		expectedPriority := i + 1
		if sub.Priority != expectedPriority {
			t.Errorf("expected priority %d at index %d, got %d", expectedPriority, i, sub.Priority)
		}
	}
}

// ===================== 边界条件测试 =====================

// TestEmptySubscriptions 测试空订阅列表
func TestEmptySubscriptions(t *testing.T) {
	_, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)

	// 获取空列表
	sorted, err := svc.GetSortedSubscriptions(userId)
	if err != nil {
		t.Fatalf("GetSortedSubscriptions failed: %v", err)
	}
	if len(sorted) != 0 {
		t.Errorf("expected empty list, got %d items", len(sorted))
	}

	// 获取最高优先级（无订阅）
	highest, err := svc.GetHighestPrioritySubscription(userId)
	if err != nil {
		t.Fatalf("GetHighestPrioritySubscription failed: %v", err)
	}
	if highest != nil {
		t.Error("expected nil for empty subscriptions")
	}

	// 规范化空列表
	err = svc.NormalizePriorities(userId)
	if err != nil {
		t.Fatalf("NormalizePriorities on empty list failed: %v", err)
	}

	// 重排序空列表
	err = svc.ReorderSubscriptions(userId, []int64{})
	if err != nil {
		t.Fatalf("ReorderSubscriptions on empty list failed: %v", err)
	}
}

// TestDynamicInsertion_WithManuallyAdjustedPriority 测试手动调整 priority 后的动态插入
// 验证：动态插入会在当前 priority 序列中按 end_at 找位置，而不是重新按 end_at 全局排序
func TestDynamicInsertion_WithManuallyAdjustedPriority(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 场景：用户手动调整了优先级，让晚到期的订阅优先
	subA := createTestSubscription(t, db, userId, 1, now+30*86400, now-300) // 30天后到期，priority=1（用户手动设为最高）
	subB := createTestSubscription(t, db, userId, 2, now+7*86400, now-200)  // 7天后到期，priority=2

	// 新订阅 end_at 在 A 和 B 之间（14天后）
	newSub := &model.Subscription{
		UserId:    userId,
		PlanId:    1,
		Status:    common.SubscriptionStatusActive,
		StartAt:   now,
		EndAt:     now + 14*86400, // 14天后到期
		Priority:  0,
		CreatedAt: now,
	}
	if err := db.Create(newSub).Error; err != nil {
		t.Fatalf("create new subscription failed: %v", err)
	}

	// 初始化优先级
	err := svc.InitializeSubscriptionPriority(newSub)
	if err != nil {
		t.Fatalf("InitializeSubscriptionPriority failed: %v", err)
	}

	// 预期行为：
	// 1. sortSubscriptions 按 priority 排序：[A(p=1, 30天), B(p=2, 7天)]
	// 2. 在排序后列表中找第一个 end_at > 14 的，即 A(30天)
	// 3. 新订阅插入到 A 前面，newSub.priority = 1
	// 4. A 和 B 的 priority 依次 +1
	// 结果：newSub(14天, p=1), A(30天, p=2), B(7天, p=3)

	if newSub.Priority != 1 {
		t.Errorf("expected newSub.Priority = 1, got %d", newSub.Priority)
	}

	var updatedA, updatedB model.Subscription
	db.First(&updatedA, subA.Id)
	db.First(&updatedB, subB.Id)

	if updatedA.Priority != 2 {
		t.Errorf("expected A.Priority = 2, got %d", updatedA.Priority)
	}
	if updatedB.Priority != 3 {
		t.Errorf("expected B.Priority = 3, got %d", updatedB.Priority)
	}

	// 验证排序后的顺序（按 priority ASC）
	sorted, err := svc.GetSortedSubscriptions(userId)
	if err != nil {
		t.Fatalf("GetSortedSubscriptions failed: %v", err)
	}

	if len(sorted) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(sorted))
	}

	// 顺序应为：newSub(p=1, 14天), A(p=2, 30天), B(p=3, 7天)
	if sorted[0].Id != newSub.Id || sorted[1].Id != subA.Id || sorted[2].Id != subB.Id {
		t.Errorf("unexpected order after insertion: [%d,%d,%d], expected [%d,%d,%d]",
			sorted[0].Id, sorted[1].Id, sorted[2].Id, newSub.Id, subA.Id, subB.Id)
	}

	// 注意：这会改变用户的手动调整（A 从 p=1 变成 p=2）
	// 这是预期行为：新订阅按 end_at 在当前 priority 序列中找位置
	// 如果用户想保持 A 的最高优先级，需要在插入后重新手动调整
}

// TestFirstSubscription 测试用户的第一个订阅
func TestFirstSubscription(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 用户的第一个订阅
	newSub := &model.Subscription{
		UserId:    userId,
		PlanId:    1,
		Status:    common.SubscriptionStatusActive,
		StartAt:   now,
		EndAt:     now + 7*86400,
		Priority:  0,
		CreatedAt: now,
	}
	if err := db.Create(newSub).Error; err != nil {
		t.Fatalf("create subscription failed: %v", err)
	}

	err := svc.InitializeSubscriptionPriority(newSub)
	if err != nil {
		t.Fatalf("InitializeSubscriptionPriority failed: %v", err)
	}

	// 第一个订阅的优先级应为 1
	if newSub.Priority != 1 {
		t.Errorf("expected first subscription priority = 1, got %d", newSub.Priority)
	}
}

// TestGetHighestPrioritySubscription 测试获取最高优先级订阅
func TestGetHighestPrioritySubscription(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建订阅，优先级不同
	createTestSubscription(t, db, userId, 3, now+21*86400, now-100)
	sub2 := createTestSubscription(t, db, userId, 1, now+7*86400, now-200) // 最高优先级
	createTestSubscription(t, db, userId, 2, now+14*86400, now-150)

	highest, err := svc.GetHighestPrioritySubscription(userId)
	if err != nil {
		t.Fatalf("GetHighestPrioritySubscription failed: %v", err)
	}

	if highest == nil {
		t.Fatal("expected non-nil highest priority subscription")
	}

	if highest.Id != sub2.Id {
		t.Errorf("expected subscription %d, got %d", sub2.Id, highest.Id)
	}
}

// TestCalculateInsertPosition 测试计算插入位置
func TestCalculateInsertPosition(t *testing.T) {
	db, cleanup := setupPriorityTestDB(t)
	defer cleanup()

	svc := service.GetSubscriptionPriorityService()
	userId := int64(1)
	now := common.GetTimestamp()

	// 创建订阅
	createTestSubscription(t, db, userId, 1, now+7*86400, now-200)  // 7天后
	createTestSubscription(t, db, userId, 2, now+14*86400, now-100) // 14天后

	// 计算不同 end_at 的插入位置
	// 注意：返回值现在是 InsertPositionResult，包含 Position（逻辑位置）和 Priority（实际优先级值）
	testCases := []struct {
		name             string
		endAt            int64
		createdAt        int64
		expectedPosition int
		expectedPriority int
	}{
		{"插入到开头", now + 1*86400, now, 1, 1},    // 1天后 - 位置1，优先级1
		{"插入到中间", now + 10*86400, now, 2, 2},   // 10天后 - 位置2，优先级2（前一个的优先级+1）
		{"插入到末尾", now + 30*86400, now, 3, 3},   // 30天后 - 位置3，优先级3（前一个的优先级+1）
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.CalculateInsertPosition(userId, tc.endAt, tc.createdAt)
			if err != nil {
				t.Fatalf("CalculateInsertPosition failed: %v", err)
			}
			if result.Position != tc.expectedPosition {
				t.Errorf("expected position %d, got %d", tc.expectedPosition, result.Position)
			}
			if result.Priority != tc.expectedPriority {
				t.Errorf("expected priority %d, got %d", tc.expectedPriority, result.Priority)
			}
		})
	}
}

package model_test

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupUserSettingCacheTest 设置测试环境，包含 SQLite 数据库和 miniredis
func setupUserSettingCacheTest(t *testing.T) (*gorm.DB, *miniredis.Miniredis, func()) {
	// 创建 SQLite 内存数据库
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err, "failed to connect database")

	sqlDB, err := db.DB()
	require.NoError(t, err, "failed to get sql db")
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// 创建 miniredis 实例
	mr, err := miniredis.Run()
	require.NoError(t, err, "failed to start miniredis")

	// 保存原始状态以便恢复
	origDB := model.DB
	origRDB := common.RDB
	origRedisEnabled := common.RedisEnabled

	// 设置测试环境
	model.DB = db
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	// 迁移数据库表
	err = db.AutoMigrate(&model.User{})
	require.NoError(t, err, "failed to migrate database")

	cleanup := func() {
		_ = sqlDB.Close()
		mr.Close()
		model.DB = origDB
		common.RDB = origRDB
		common.RedisEnabled = origRedisEnabled
	}

	return db, mr, cleanup
}

// createTestUserWithCache 创建测试用户并初始化缓存
func createTestUserWithCache(t *testing.T, db *gorm.DB, userId int) *model.User {
	user := &model.User{
		Username:           "cachetest_user",
		Password:           "password",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		AutoWalletFallback: false,
		Setting:            `{"auto_wallet_fallback":false}`,
	}
	// 手动设置 ID
	user.Id = userId
	err := db.Create(user).Error
	require.NoError(t, err, "failed to create user")
	return user
}

// initUserCache 初始化用户缓存
func initUserCache(t *testing.T, mr *miniredis.Miniredis, userId int, setting string) {
	cacheKey := "user:" + itoa(userId)

	// 设置缓存 hash 字段（miniredis.HSet 无返回值）
	mr.HSet(cacheKey, "Id", itoa(userId))
	mr.HSet(cacheKey, "Setting", setting)
	mr.HSet(cacheKey, "Username", "cachetest_user")
	mr.HSet(cacheKey, "Status", itoa(common.UserStatusEnabled))

	// 设置过期时间，确保 RedisHSetField 能成功更新
	mr.SetTTL(cacheKey, time.Minute)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	result := ""
	for i > 0 {
		result = string(rune('0'+i%10)) + result
		i /= 10
	}
	return result
}

// TestUpdateAutoWalletFallback_CacheRefresh 测试非事务版本的缓存刷新
func TestUpdateAutoWalletFallback_CacheRefresh(t *testing.T) {
	db, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	// 创建测试用户
	userId := 100
	createTestUserWithCache(t, db, userId)

	// 初始化缓存（模拟已经存在的缓存）
	initUserCache(t, mr, userId, `{"auto_wallet_fallback":false}`)

	// 验证初始缓存存在
	cacheKey := "user:" + itoa(userId)
	initialSetting := mr.HGet(cacheKey, "Setting")
	assert.Contains(t, initialSetting, `"auto_wallet_fallback":false`)

	// 调用 UpdateAutoWalletFallback 更新设置
	err := model.UpdateAutoWalletFallback(userId, true)
	require.NoError(t, err)

	// 验证缓存已更新
	updatedSetting := mr.HGet(cacheKey, "Setting")
	assert.Contains(t, updatedSetting, `"auto_wallet_fallback":true`, "cache should be updated with new setting")
	assert.Contains(t, updatedSetting, `"auto_wallet_fallback_explicit":true`, "explicit flag should be set")
}

// TestUpdateAutoWalletFallbackWithTx_OnCommitCallback 测试事务版本的 onCommit 回调
func TestUpdateAutoWalletFallbackWithTx_OnCommitCallback(t *testing.T) {
	db, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	// 创建测试用户
	userId := 101
	createTestUserWithCache(t, db, userId)

	// 初始化缓存
	initUserCache(t, mr, userId, `{"auto_wallet_fallback":false}`)

	cacheKey := "user:" + itoa(userId)

	// 开始事务
	tx := db.Begin()
	require.NoError(t, tx.Error)

	// 调用 UpdateAutoWalletFallbackWithTx
	onCommit, err := model.UpdateAutoWalletFallbackWithTx(tx, userId, true)
	require.NoError(t, err)
	require.NotNil(t, onCommit, "onCommit callback should not be nil")

	// 在事务提交前，缓存应该还是旧值
	settingBeforeCommit := mr.HGet(cacheKey, "Setting")
	assert.Contains(t, settingBeforeCommit, `"auto_wallet_fallback":false`, "cache should not be updated before commit")

	// 提交事务
	err = tx.Commit().Error
	require.NoError(t, err)

	// 调用 onCommit 回调刷新缓存
	onCommit()

	// 验证缓存已更新
	settingAfterCommit := mr.HGet(cacheKey, "Setting")
	assert.Contains(t, settingAfterCommit, `"auto_wallet_fallback":true`, "cache should be updated after onCommit")
	assert.Contains(t, settingAfterCommit, `"auto_wallet_fallback_explicit":true`, "explicit flag should be set")
}

// TestUpdateAutoWalletFallbackWithTx_RollbackNoCacheUpdate 测试事务回滚时不刷新缓存
func TestUpdateAutoWalletFallbackWithTx_RollbackNoCacheUpdate(t *testing.T) {
	db, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	// 创建测试用户
	userId := 102
	createTestUserWithCache(t, db, userId)

	// 初始化缓存
	initUserCache(t, mr, userId, `{"auto_wallet_fallback":false}`)

	cacheKey := "user:" + itoa(userId)

	// 开始事务
	tx := db.Begin()
	require.NoError(t, tx.Error)

	// 调用 UpdateAutoWalletFallbackWithTx
	onCommit, err := model.UpdateAutoWalletFallbackWithTx(tx, userId, true)
	require.NoError(t, err)
	require.NotNil(t, onCommit)

	// 回滚事务（模拟事务失败场景）
	err = tx.Rollback().Error
	require.NoError(t, err)

	// 不调用 onCommit，因为事务回滚了

	// 验证缓存保持不变
	settingAfterRollback := mr.HGet(cacheKey, "Setting")
	assert.Contains(t, settingAfterRollback, `"auto_wallet_fallback":false`, "cache should not be updated after rollback")
}

// TestResetAutoWalletFallback_CacheRefresh 测试重置设置时的缓存刷新
func TestResetAutoWalletFallback_CacheRefresh(t *testing.T) {
	db, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	// 创建测试用户，设置为显式启用
	userId := 103
	user := &model.User{
		Username:           "cachetest_user",
		Password:           "password",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		AutoWalletFallback: true,
		Setting:            `{"auto_wallet_fallback":true,"auto_wallet_fallback_explicit":true}`,
	}
	user.Id = userId
	err := db.Create(user).Error
	require.NoError(t, err)

	// 初始化缓存
	initUserCache(t, mr, userId, `{"auto_wallet_fallback":true,"auto_wallet_fallback_explicit":true}`)

	cacheKey := "user:" + itoa(userId)

	// 调用 ResetAutoWalletFallback
	err = model.ResetAutoWalletFallback(userId)
	require.NoError(t, err)

	// 验证缓存已更新，explicit 标记应被清除
	updatedSetting := mr.HGet(cacheKey, "Setting")
	// 重置后，设置应该继承系统默认值，explicit 应该为 nil（不存在于 JSON 中）
	assert.NotContains(t, updatedSetting, `"auto_wallet_fallback_explicit":true`, "explicit flag should be cleared after reset")
}

// TestCacheRefreshWithRedisDisabled 测试 Redis 禁用时的行为
func TestCacheRefreshWithRedisDisabled(t *testing.T) {
	// 创建 SQLite 内存数据库（不启用 Redis）
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()

	// 保存原始状态
	origDB := model.DB
	origRDB := common.RDB
	origRedisEnabled := common.RedisEnabled

	// 设置测试环境（禁用 Redis）
	model.DB = db
	common.RedisEnabled = false
	common.RDB = nil

	defer func() {
		model.DB = origDB
		common.RDB = origRDB
		common.RedisEnabled = origRedisEnabled
	}()

	err = db.AutoMigrate(&model.User{})
	require.NoError(t, err)

	// 创建测试用户
	userId := 104
	user := &model.User{
		Username:           "cachetest_user",
		Password:           "password",
		Role:               common.RoleCommonUser,
		Status:             common.UserStatusEnabled,
		AutoWalletFallback: false,
	}
	user.Id = userId
	err = db.Create(user).Error
	require.NoError(t, err)

	// 调用 UpdateAutoWalletFallback，应该正常工作（缓存刷新被跳过）
	err = model.UpdateAutoWalletFallback(userId, true)
	assert.NoError(t, err, "should work even when Redis is disabled")

	// 验证数据库已更新
	var updatedUser model.User
	err = db.First(&updatedUser, userId).Error
	require.NoError(t, err)
	assert.True(t, updatedUser.AutoWalletFallback)
}

// TestUpdateAutoWalletFallbackWithTx_OnCommitCanBeCalledSafely 测试 onCommit 可以安全调用
func TestUpdateAutoWalletFallbackWithTx_OnCommitCanBeCalledSafely(t *testing.T) {
	db, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	userId := 105
	createTestUserWithCache(t, db, userId)
	initUserCache(t, mr, userId, `{}`)

	tx := db.Begin()
	require.NoError(t, tx.Error)

	onCommit, err := model.UpdateAutoWalletFallbackWithTx(tx, userId, true)
	require.NoError(t, err)

	err = tx.Commit().Error
	require.NoError(t, err)

	// 多次调用 onCommit 应该是安全的
	assert.NotPanics(t, func() {
		onCommit()
		onCommit() // 第二次调用也应该安全
	})
}

// TestUpdateAutoWalletFallbackWithTx_WithDBTransaction 测试与 DB.Transaction 配合使用
func TestUpdateAutoWalletFallbackWithTx_WithDBTransaction(t *testing.T) {
	db, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	userId := 106
	createTestUserWithCache(t, db, userId)
	initUserCache(t, mr, userId, `{"auto_wallet_fallback":false}`)

	cacheKey := "user:" + itoa(userId)

	var onCommit func()

	// 使用 DB.Transaction 模式
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		onCommit, err = model.UpdateAutoWalletFallbackWithTx(tx, userId, true)
		return err
	})
	require.NoError(t, err)

	// 事务成功提交后调用 onCommit
	if onCommit != nil {
		onCommit()
	}

	// 验证缓存已更新
	updatedSetting := mr.HGet(cacheKey, "Setting")
	assert.Contains(t, updatedSetting, `"auto_wallet_fallback":true`)
}

// TestCacheKeyFormat 验证缓存键格式
func TestCacheKeyFormat(t *testing.T) {
	_, mr, cleanup := setupUserSettingCacheTest(t)
	defer cleanup()

	// 验证缓存键格式
	ctx := context.Background()
	testKey := "user:123"
	err := common.RDB.HSet(ctx, testKey, "test", "value").Err()
	require.NoError(t, err)

	// 设置 TTL 以确保 key 存在
	mr.SetTTL(testKey, time.Minute)

	// 验证可以获取
	val := mr.HGet(testKey, "test")
	assert.Equal(t, "value", val)
}

package controller_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUserConfigTestDB(t *testing.T) (*gorm.DB, func()) {
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
	common.RedisEnabled = false
	common.RDB = nil

	err = db.AutoMigrate(
		&model.User{},
		&model.SubscriptionPlan{},
		&model.SubscriptionPlanLimit{},
		&model.Subscription{},
		&model.SubscriptionUsage{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, cleanup
}

func setupUserConfigRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test-session", store))

	// 用户订阅配置路由
	userSubConfig := r.Group("/api/user/subscriptions")
	userSubConfig.Use(middleware.UserAuth())
	{
		userSubConfig.PUT("/:id/auto-wallet", controller.UpdateSubscriptionAutoWallet)
		userSubConfig.PUT("/priorities", controller.BatchUpdateSubscriptionPriorities)
	}

	// 用户设置路由
	userSettings := r.Group("/api/user/settings")
	userSettings.Use(middleware.UserAuth())
	{
		userSettings.GET("/auto-wallet-fallback", controller.GetUserAutoWalletFallback)
		userSettings.PUT("/auto-wallet-fallback", controller.UpdateUserAutoWalletFallback)
	}

	return r
}

func createTestUser(t *testing.T, db *gorm.DB) *model.User {
	token := "test-user-token-" + strconv.Itoa(common.RoleCommonUser)
	user := &model.User{
		Username: "testuser",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	user.SetAccessToken(token)
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func createTestSubscriptionPlan(t *testing.T, db *gorm.DB) *model.SubscriptionPlan {
	sku := "test-sku-001"
	plan := &model.SubscriptionPlan{
		Name:         "Test Plan",
		SKU:          &sku,
		PriceCents:   1000,
		Currency:     "CNY",
		BillingCycle: "monthly",
		Status:       common.PlanStatusActive,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("failed to create plan: %v", err)
	}
	return plan
}

func createTestSubscription(t *testing.T, db *gorm.DB, userId int64, planId int64, priority int) *model.Subscription {
	now := common.GetTimestamp()
	sub := &model.Subscription{
		UserId:             userId,
		PlanId:             planId,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now,
		EndAt:              now + 30*24*60*60,
		Priority:           priority,
		AutoWalletFallback: false,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	return sub
}

func addUserHeaders(req *http.Request, user *model.User) {
	req.Header.Set("Authorization", user.GetAccessToken())
	req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	req.Header.Set("Content-Type", "application/json")
}

// TestUpdateSubscriptionAutoWallet 测试切换订阅自动兜底开关
func TestUpdateSubscriptionAutoWallet(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)
	plan := createTestSubscriptionPlan(t, db)
	sub := createTestSubscription(t, db, int64(user.Id), plan.Id, 1)

	r := setupUserConfigRouter()

	tests := []struct {
		name           string
		subId          int64
		userId         int
		enabled        bool
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "enable auto wallet fallback",
			subId:          sub.Id,
			userId:         user.Id,
			enabled:        true,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "disable auto wallet fallback",
			subId:          sub.Id,
			userId:         user.Id,
			enabled:        false,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "subscription not found",
			subId:          99999,
			userId:         user.Id,
			enabled:        true,
			expectedStatus: http.StatusOK,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"enabled": tt.enabled,
			})
			req, _ := http.NewRequest("PUT", "/api/user/subscriptions/"+strconv.FormatInt(tt.subId, 10)+"/auto-wallet", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			addUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])

			if tt.expectSuccess {
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, tt.enabled, data["auto_wallet_fallback"])
			}
		})
	}
}

// TestBatchUpdateSubscriptionPriorities 测试批量更新订阅优先级
func TestBatchUpdateSubscriptionPriorities(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)
	plan := createTestSubscriptionPlan(t, db)
	sub1 := createTestSubscription(t, db, int64(user.Id), plan.Id, 1)
	sub2 := createTestSubscription(t, db, int64(user.Id), plan.Id, 2)
	sub3 := createTestSubscription(t, db, int64(user.Id), plan.Id, 3)

	r := setupUserConfigRouter()

	tests := []struct {
		name            string
		subscriptionIds []int64
		expectedStatus  int
		expectSuccess   bool
	}{
		{
			name:            "reorder priorities",
			subscriptionIds: []int64{sub3.Id, sub1.Id, sub2.Id},
			expectedStatus:  http.StatusOK,
			expectSuccess:   true,
		},
		{
			name:            "empty subscription ids",
			subscriptionIds: []int64{},
			expectedStatus:  http.StatusOK,
			expectSuccess:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"subscription_ids": tt.subscriptionIds,
			})
			req, _ := http.NewRequest("PUT", "/api/user/subscriptions/priorities", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			addUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])
		})
	}
}

// TestAutoWalletMissingEnabled 测试 auto-wallet 接口缺少 enabled 参数
func TestAutoWalletMissingEnabled(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)
	plan := createTestSubscriptionPlan(t, db)
	sub := createTestSubscription(t, db, int64(user.Id), plan.Id, 1)

	r := setupUserConfigRouter()

	tests := []struct {
		name          string
		body          map[string]interface{}
		expectSuccess bool
		expectMessage string
	}{
		{
			name:          "empty object should fail",
			body:          map[string]interface{}{},
			expectSuccess: false,
			expectMessage: "enabled 参数为必填项",
		},
		{
			name:          "null enabled should fail",
			body:          map[string]interface{}{"enabled": nil},
			expectSuccess: false,
			expectMessage: "enabled 参数为必填项",
		},
		{
			name:          "other fields only should fail",
			body:          map[string]interface{}{"foo": "bar"},
			expectSuccess: false,
			expectMessage: "enabled 参数为必填项",
		},
		{
			name:          "explicit false should succeed",
			body:          map[string]interface{}{"enabled": false},
			expectSuccess: true,
			expectMessage: "",
		},
		{
			name:          "explicit true should succeed",
			body:          map[string]interface{}{"enabled": true},
			expectSuccess: true,
			expectMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req, _ := http.NewRequest("PUT", "/api/user/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/auto-wallet", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			addUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])

			if !tt.expectSuccess && tt.expectMessage != "" {
				assert.Contains(t, resp["message"], tt.expectMessage)
			}
		})
	}
}

// TestAutoWalletIdempotent 测试 auto-wallet 接口幂等性（同值更新应成功）
func TestAutoWalletIdempotent(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)
	plan := createTestSubscriptionPlan(t, db)
	sub := createTestSubscription(t, db, int64(user.Id), plan.Id, 1)

	r := setupUserConfigRouter()

	// 第一次设置为 true
	body, _ := json.Marshal(map[string]interface{}{"enabled": true})
	req, _ := http.NewRequest("PUT", "/api/user/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/auto-wallet", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	addUserHeaders(req, user)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp["success"].(bool), "第一次设置应成功")

	// 第二次设置相同的值（幂等请求）
	body, _ = json.Marshal(map[string]interface{}{"enabled": true})
	req, _ = http.NewRequest("PUT", "/api/user/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/auto-wallet", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	addUserHeaders(req, user)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp["success"].(bool), "幂等请求（同值更新）应成功")
}

// TestPrioritiesWithDuplicateIds 测试优先级排序含重复 ID
func TestPrioritiesWithDuplicateIds(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)
	plan := createTestSubscriptionPlan(t, db)
	sub1 := createTestSubscription(t, db, int64(user.Id), plan.Id, 1)
	sub2 := createTestSubscription(t, db, int64(user.Id), plan.Id, 2)

	r := setupUserConfigRouter()

	// 包含重复 ID
	body, _ := json.Marshal(map[string]interface{}{
		"subscription_ids": []int64{sub1.Id, sub2.Id, sub1.Id},
	})
	req, _ := http.NewRequest("PUT", "/api/user/subscriptions/priorities", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	addUserHeaders(req, user)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp["success"].(bool), "含重复 ID 应该失败")
	assert.Contains(t, resp["message"], "重复", "应该提示 ID 重复")
}

// TestPrioritiesWithOthersSubscription 测试优先级排序含他人订阅 ID
func TestPrioritiesWithOthersSubscription(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user1 := createTestUser(t, db)
	// 创建另一个用户
	token := "test-user2-token-priorities"
	user2 := &model.User{
		Username: "testuser2_priorities",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	user2.SetAccessToken(token)
	db.Create(user2)

	plan := createTestSubscriptionPlan(t, db)
	// 创建 user1 的订阅
	sub1 := createTestSubscription(t, db, int64(user1.Id), plan.Id, 1)
	// 创建 user2 的订阅
	sub2 := createTestSubscription(t, db, int64(user2.Id), plan.Id, 1)

	r := setupUserConfigRouter()

	// user1 尝试在优先级列表中包含 user2 的订阅
	body, _ := json.Marshal(map[string]interface{}{
		"subscription_ids": []int64{sub1.Id, sub2.Id},
	})
	req, _ := http.NewRequest("PUT", "/api/user/subscriptions/priorities", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	addUserHeaders(req, user1)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp["success"].(bool), "包含他人订阅 ID 应该失败")
}

// TestGetUserAutoWalletFallback 测试获取用户级别兜底配置
func TestGetUserAutoWalletFallback(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)

	r := setupUserConfigRouter()

	req, _ := http.NewRequest("GET", "/api/user/settings/auto-wallet-fallback", nil)
	addUserHeaders(req, user)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.Contains(t, data, "user_setting")
	assert.Contains(t, data, "system_default")
	assert.Contains(t, data, "effective")
	assert.Contains(t, data, "is_explicit")
	assert.Contains(t, data, "using_system_default")

	// 新用户应该使用系统默认（未显式设置）
	assert.False(t, data["is_explicit"].(bool), "新用户应该未显式设置")
	assert.True(t, data["using_system_default"].(bool), "新用户应该使用系统默认")
}

// TestUpdateUserAutoWalletFallback 测试更新用户级别兜底配置
func TestUpdateUserAutoWalletFallback(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)

	r := setupUserConfigRouter()

	t.Run("enable user auto wallet fallback", func(t *testing.T) {
		enabled := true
		body, _ := json.Marshal(map[string]interface{}{
			"enabled": enabled,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))

		data := resp["data"].(map[string]interface{})
		assert.Equal(t, enabled, data["auto_wallet_fallback"])
		assert.True(t, data["is_explicit"].(bool), "设置后应该是显式设置")
		assert.False(t, data["using_system_default"].(bool), "设置后不应该使用系统默认")

		// 验证数据库更新（包括 setting JSON）
		var updatedUser model.User
		db.First(&updatedUser, user.Id)
		assert.Equal(t, enabled, updatedUser.AutoWalletFallback)
		assert.NotEmpty(t, updatedUser.Setting, "Setting JSON 应该被更新")

		// 验证 explicit 标记被设置
		var settings dto.UserSetting
		json.Unmarshal([]byte(updatedUser.Setting), &settings)
		assert.NotNil(t, settings.AutoWalletFallbackExplicit, "explicit 标记应该被设置")
		assert.True(t, *settings.AutoWalletFallbackExplicit)
	})

	t.Run("disable user auto wallet fallback", func(t *testing.T) {
		enabled := false
		body, _ := json.Marshal(map[string]interface{}{
			"enabled": enabled,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))

		data := resp["data"].(map[string]interface{})
		assert.Equal(t, enabled, data["auto_wallet_fallback"])
		assert.True(t, data["is_explicit"].(bool))

		// 验证数据库更新
		var updatedUser model.User
		db.First(&updatedUser, user.Id)
		assert.Equal(t, enabled, updatedUser.AutoWalletFallback)
	})

	t.Run("reset to inherit system default", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"reset": true,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.True(t, resp["success"].(bool))

		data := resp["data"].(map[string]interface{})
		assert.False(t, data["is_explicit"].(bool), "重置后不应该是显式设置")
		assert.True(t, data["using_system_default"].(bool), "重置后应该使用系统默认")

		// 验证数据库更新（explicit 标记被清除）
		var updatedUser model.User
		db.First(&updatedUser, user.Id)

		var settings dto.UserSetting
		json.Unmarshal([]byte(updatedUser.Setting), &settings)
		assert.Nil(t, settings.AutoWalletFallbackExplicit, "explicit 标记应该被清除")

		// 验证 Setting JSON 中的 auto_wallet_fallback 等于系统默认值（修复 Low 问题）
		systemDefault := common.GetAutoWalletFallbackDefault()
		assert.Equal(t, systemDefault, settings.AutoWalletFallback, "Setting JSON 中的 auto_wallet_fallback 应等于系统默认值")
		assert.Equal(t, systemDefault, updatedUser.AutoWalletFallback, "DB 字段 auto_wallet_fallback 应等于系统默认值")
	})

	t.Run("empty body should fail", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.False(t, resp["success"].(bool), "空请求体应该失败")
		assert.Contains(t, resp["message"], "请提供 enabled 或 reset 参数")
	})

	t.Run("enabled and reset cannot be both provided", func(t *testing.T) {
		enabled := true
		body, _ := json.Marshal(map[string]interface{}{
			"enabled": enabled,
			"reset":   true,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.False(t, resp["success"].(bool))
		assert.Contains(t, resp["message"], "enabled 和 reset 参数不能同时提供")
	})
}

// TestUnauthorizedAccess 测试未授权访问
func TestUnauthorizedAccess(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	r := setupUserConfigRouter()

	// 测试未授权访问用户设置 - 不带 token 应该返回 401
	req, _ := http.NewRequest("GET", "/api/user/settings/auto-wallet-fallback", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 认证失败返回 401
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	_ = db
}

// TestSettingJSONConsistency 测试 Setting JSON 与 DB 字段的一致性
// 验证缓存刷新后读取的数据与 DB 一致
func TestSettingJSONConsistency(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user := createTestUser(t, db)
	r := setupUserConfigRouter()

	t.Run("enable then verify setting JSON", func(t *testing.T) {
		enabled := true
		body, _ := json.Marshal(map[string]interface{}{
			"enabled": enabled,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 验证 DB 和 Setting JSON 一致
		var updatedUser model.User
		db.First(&updatedUser, user.Id)

		var settings dto.UserSetting
		err := json.Unmarshal([]byte(updatedUser.Setting), &settings)
		assert.NoError(t, err)

		// DB 字段与 Setting JSON 一致
		assert.Equal(t, enabled, updatedUser.AutoWalletFallback, "DB 字段应为 true")
		assert.Equal(t, enabled, settings.AutoWalletFallback, "Setting JSON 中的值应为 true")
		assert.NotNil(t, settings.AutoWalletFallbackExplicit, "explicit 标记应存在")
		assert.True(t, *settings.AutoWalletFallbackExplicit, "explicit 标记应为 true")
	})

	t.Run("disable then verify setting JSON", func(t *testing.T) {
		enabled := false
		body, _ := json.Marshal(map[string]interface{}{
			"enabled": enabled,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 验证 DB 和 Setting JSON 一致
		var updatedUser model.User
		db.First(&updatedUser, user.Id)

		var settings dto.UserSetting
		err := json.Unmarshal([]byte(updatedUser.Setting), &settings)
		assert.NoError(t, err)

		// DB 字段与 Setting JSON 一致
		assert.Equal(t, enabled, updatedUser.AutoWalletFallback, "DB 字段应为 false")
		assert.Equal(t, enabled, settings.AutoWalletFallback, "Setting JSON 中的值应为 false")
		assert.NotNil(t, settings.AutoWalletFallbackExplicit, "explicit 标记应存在")
		assert.True(t, *settings.AutoWalletFallbackExplicit, "explicit 标记应为 true")
	})

	t.Run("reset then verify setting JSON equals system default", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"reset": true,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)

		// 验证 DB 和 Setting JSON 一致，且等于系统默认值
		var updatedUser model.User
		db.First(&updatedUser, user.Id)

		var settings dto.UserSetting
		err := json.Unmarshal([]byte(updatedUser.Setting), &settings)
		assert.NoError(t, err)

		systemDefault := common.GetAutoWalletFallbackDefault()

		// DB 字段与 Setting JSON 一致，且等于系统默认值
		assert.Equal(t, systemDefault, updatedUser.AutoWalletFallback, "DB 字段应等于系统默认值")
		assert.Equal(t, systemDefault, settings.AutoWalletFallback, "Setting JSON 中的值应等于系统默认值")
		assert.Nil(t, settings.AutoWalletFallbackExplicit, "explicit 标记应被清除")
	})

	t.Run("GetEffectiveAutoWalletFallback returns correct values", func(t *testing.T) {
		systemDefault := common.GetAutoWalletFallbackDefault()

		// 先重置，验证继承系统默认
		body, _ := json.Marshal(map[string]interface{}{
			"reset": true,
		})
		req, _ := http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var updatedUser model.User
		db.First(&updatedUser, user.Id)

		effective, usingDefault := updatedUser.GetEffectiveAutoWalletFallback(systemDefault)
		assert.Equal(t, systemDefault, effective, "有效值应等于系统默认值")
		assert.True(t, usingDefault, "应该使用系统默认")

		// 然后显式设置，验证不再使用系统默认
		enabled := !systemDefault // 故意设置为与系统默认相反的值
		body, _ = json.Marshal(map[string]interface{}{
			"enabled": enabled,
		})
		req, _ = http.NewRequest("PUT", "/api/user/settings/auto-wallet-fallback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		addUserHeaders(req, user)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		db.First(&updatedUser, user.Id)
		effective, usingDefault = updatedUser.GetEffectiveAutoWalletFallback(systemDefault)
		assert.Equal(t, enabled, effective, "有效值应等于用户设置")
		assert.False(t, usingDefault, "不应该使用系统默认")
	})
}

// TestAccessOthersSubscription 测试访问他人订阅
func TestAccessOthersSubscription(t *testing.T) {
	db, cleanup := setupUserConfigTestDB(t)
	defer cleanup()

	user1 := createTestUser(t, db)
	// 创建另一个用户
	token := "test-user2-token"
	user2 := &model.User{
		Username: "testuser2",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	user2.SetAccessToken(token)
	db.Create(user2)

	plan := createTestSubscriptionPlan(t, db)
	// 创建属于 user2 的订阅
	sub := createTestSubscription(t, db, int64(user2.Id), plan.Id, 1)

	r := setupUserConfigRouter()

	// user1 尝试修改 user2 的订阅
	body, _ := json.Marshal(map[string]interface{}{
		"enabled": true,
	})
	req, _ := http.NewRequest("PUT", "/api/user/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/auto-wallet", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	addUserHeaders(req, user1)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.False(t, resp["success"].(bool))
	// 使用新的单字段更新函数后，错误消息变为 "订阅不存在或无权修改"
	assert.Contains(t, resp["message"], "无权修改")
}

package controller_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
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

func setupUserCouponTestDB(t *testing.T) (*gorm.DB, func()) {
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
		&model.Coupon{},
		&model.UserCoupon{},
		&model.SubscriptionPlan{},
		&model.AuditLog{},
		&model.CouponRedemptionBinding{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, cleanup
}

func setupUserCouponRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test-session", store))

	// 用户优惠券路由
	userCouponRoute := r.Group("/api/user/coupons")
	userCouponRoute.Use(middleware.UserAuth())
	{
		userCouponRoute.POST("/claim", controller.ClaimCoupon)
		userCouponRoute.GET("/", controller.GetUserCoupons)
		userCouponRoute.GET("/available", controller.GetAvailableCoupons)
		userCouponRoute.GET("/:id", controller.GetUserCouponDetail)
	}

	// 用户支付优惠券预览路由
	userPaymentRoute := r.Group("/api/user/payment")
	userPaymentRoute.Use(middleware.UserAuth())
	{
		userPaymentRoute.POST("/coupon/preview", controller.PreviewCouponUsage)
	}

	return r
}

func createCouponTestUser(t *testing.T, db *gorm.DB) *model.User {
	token := "test-coupon-user-token-" + strconv.Itoa(common.RoleCommonUser)
	user := &model.User{
		Username: "coupontestuser",
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

func createTestCoupon(t *testing.T, db *gorm.DB, name string, totalCount int) *model.Coupon {
	now := time.Now().Unix()
	desc := "Test coupon " + name
	coupon := &model.Coupon{
		Code:            "TEST-COUPON-" + name,
		Name:            name,
		Description:     &desc,
		Type:            common.CouponTypeFullReduction,
		Scope:           common.CouponScopeSubscription,
		DiscountValue:   1000, // 10元
		ThresholdAmount: 5000, // 满50元可用
		Currency:        "CNY",
		Status:          common.CouponStatusActive,
		TotalCount:      int64(totalCount),
		UsedCount:       0,
		PerUserLimit:    1,
		ValidFrom:       now - 3600,
		ValidTo:         now + 86400*30, // 30天后过期
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(coupon).Error; err != nil {
		t.Fatalf("failed to create coupon: %v", err)
	}
	return coupon
}

func createTestUserCoupon(t *testing.T, db *gorm.DB, userId int64, coupon *model.Coupon) *model.UserCoupon {
	now := time.Now().Unix()
	userCoupon := &model.UserCoupon{
		CouponId:  coupon.Id,
		UserId:    userId,
		Code:      "UC-" + common.GetUUID(),
		Status:    common.UserCouponStatusAvailable,
		ClaimedAt: now,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(userCoupon).Error; err != nil {
		t.Fatalf("failed to create user coupon: %v", err)
	}
	return userCoupon
}

func addCouponUserHeaders(req *http.Request, user *model.User) {
	req.Header.Set("Authorization", user.GetAccessToken())
	req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	req.Header.Set("Content-Type", "application/json")
}

// TestClaimCoupon 测试领取优惠券
func TestClaimCoupon(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)
	coupon := createTestCoupon(t, db, "ClaimTest", 100)

	r := setupUserCouponRouter()

	tests := []struct {
		name           string
		claimCode      string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "successful claim",
			claimCode:      coupon.Code,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "invalid claim code",
			claimCode:      "INVALID-CODE",
			expectedStatus: http.StatusOK,
			expectSuccess:  false,
		},
		{
			name:           "empty claim code",
			claimCode:      "",
			expectedStatus: http.StatusOK,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"claim_code": tt.claimCode,
			})
			req, _ := http.NewRequest("POST", "/api/user/coupons/claim", bytes.NewBuffer(body))
			addCouponUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])

			if tt.expectSuccess {
				data := resp["data"].(map[string]interface{})
				assert.NotNil(t, data["id"])
				assert.Equal(t, common.UserCouponStatusAvailable, data["status"])
			}
		})
	}
}

// TestClaimCouponDuplicate 测试重复领取优惠券
func TestClaimCouponDuplicate(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)
	coupon := createTestCoupon(t, db, "DuplicateTest", 100)

	r := setupUserCouponRouter()

	// 第一次领取
	body, _ := json.Marshal(map[string]interface{}{
		"claim_code": coupon.Code,
	})
	req, _ := http.NewRequest("POST", "/api/user/coupons/claim", bytes.NewBuffer(body))
	addCouponUserHeaders(req, user)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.True(t, resp["success"].(bool), "first claim should succeed")

	// 第二次领取（应该失败）
	req2, _ := http.NewRequest("POST", "/api/user/coupons/claim", bytes.NewBuffer(body))
	addCouponUserHeaders(req2, user)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	assert.False(t, resp2["success"].(bool), "duplicate claim should fail")
}

// TestGetUserCoupons 测试获取用户优惠券列表
func TestGetUserCoupons(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)
	coupon1 := createTestCoupon(t, db, "List1", 100)
	coupon2 := createTestCoupon(t, db, "List2", 100)
	createTestUserCoupon(t, db, int64(user.Id), coupon1)
	createTestUserCoupon(t, db, int64(user.Id), coupon2)

	r := setupUserCouponRouter()

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		expectSuccess  bool
		expectedCount  int
	}{
		{
			name:           "get all coupons",
			query:          "",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			expectedCount:  2,
		},
		{
			name:           "filter by status available",
			query:          "?status=available",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			expectedCount:  2,
		},
		{
			name:           "filter by status used",
			query:          "?status=used",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			expectedCount:  0,
		},
		{
			name:           "with pagination",
			query:          "?page=1&page_size=1",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			expectedCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/user/coupons/"+tt.query, nil)
			addCouponUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])

			if tt.expectSuccess {
				if resp["data"] != nil {
					data := resp["data"].([]interface{})
					assert.Equal(t, tt.expectedCount, len(data))
				} else {
					assert.Equal(t, tt.expectedCount, 0)
				}
			}
		})
	}
}

// TestGetUserCouponDetail 测试获取用户优惠券详情
func TestGetUserCouponDetail(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)
	coupon := createTestCoupon(t, db, "Detail", 100)
	userCoupon := createTestUserCoupon(t, db, int64(user.Id), coupon)

	r := setupUserCouponRouter()

	tests := []struct {
		name           string
		couponId       int64
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "valid coupon id",
			couponId:       userCoupon.Id,
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "invalid coupon id",
			couponId:       99999,
			expectedStatus: http.StatusOK,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/api/user/coupons/"+strconv.FormatInt(tt.couponId, 10), nil)
			addCouponUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])

			if tt.expectSuccess {
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, float64(userCoupon.Id), data["id"])
				assert.Equal(t, coupon.Name, data["coupon_name"])
			}
		})
	}
}

// TestGetUserCouponDetailOtherUser 测试访问其他用户的优惠券
func TestGetUserCouponDetailOtherUser(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user1 := createCouponTestUser(t, db)

	// 创建另一个用户
	token := "test-user2-coupon-token"
	user2 := &model.User{
		Username: "coupontestuser2",
		Password: "password",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
	}
	user2.SetAccessToken(token)
	db.Create(user2)

	coupon := createTestCoupon(t, db, "OtherUser", 100)
	userCoupon := createTestUserCoupon(t, db, int64(user2.Id), coupon)

	r := setupUserCouponRouter()

	// user1 尝试访问 user2 的优惠券
	req, _ := http.NewRequest("GET", "/api/user/coupons/"+strconv.FormatInt(userCoupon.Id, 10), nil)
	addCouponUserHeaders(req, user1)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.False(t, resp["success"].(bool))
	assert.Contains(t, resp["message"], "无权访问")
}

// TestGetAvailableCoupons 测试获取可领取的优惠券列表
func TestGetAvailableCoupons(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)
	createTestCoupon(t, db, "Available1", 100)
	createTestCoupon(t, db, "Available2", 100)

	// 创建一个已被用户领取的优惠券
	coupon3 := createTestCoupon(t, db, "AlreadyClaimed", 100)
	createTestUserCoupon(t, db, int64(user.Id), coupon3)

	r := setupUserCouponRouter()

	req, _ := http.NewRequest("GET", "/api/user/coupons/available", nil)
	addCouponUserHeaders(req, user)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].([]interface{})
	// 应该只返回2个未领取的优惠券（排除已领取的 coupon3）
	assert.Equal(t, 2, len(data))
}

// TestPreviewCouponUsage 测试优惠券使用预览
func TestPreviewCouponUsage(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)
	coupon := createTestCoupon(t, db, "Preview", 100)
	userCoupon := createTestUserCoupon(t, db, int64(user.Id), coupon)

	r := setupUserCouponRouter()

	tests := []struct {
		name           string
		userCouponId   int64
		orderAmount    int64
		scene          string
		expectedStatus int
		expectSuccess  bool
		expectValid    bool
	}{
		{
			name:           "valid preview subscription",
			userCouponId:   userCoupon.Id,
			orderAmount:    10000, // 100元，满足50元门槛
			scene:          "subscription",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			expectValid:    true,
		},
		{
			name:           "below threshold",
			userCouponId:   userCoupon.Id,
			orderAmount:    3000, // 30元，不满足50元门槛
			scene:          "subscription",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
			expectValid:    false,
		},
		{
			name:           "invalid user coupon id",
			userCouponId:   99999,
			orderAmount:    10000,
			scene:          "subscription",
			expectedStatus: http.StatusOK,
			expectSuccess:  true, // API 返回 success: true, 但 valid: false
			expectValid:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{
				"user_coupon_id": tt.userCouponId,
				"order_amount":   tt.orderAmount,
				"scene":          tt.scene,
			})
			req, _ := http.NewRequest("POST", "/api/user/payment/coupon/preview", bytes.NewBuffer(body))
			addCouponUserHeaders(req, user)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var resp map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectSuccess, resp["success"])

			if tt.expectSuccess {
				data := resp["data"].(map[string]interface{})
				assert.Equal(t, tt.expectValid, data["valid"])

				if tt.expectValid {
					// 验证折扣计算
					assert.Equal(t, float64(tt.orderAmount), data["original_amount"])
					assert.True(t, data["discount_amount"].(float64) > 0)
					assert.True(t, data["final_amount"].(float64) < float64(tt.orderAmount))
				}
			}
		})
	}
}

// TestUnauthorizedAccessCoupon 测试未授权访问
func TestUnauthorizedAccessCoupon(t *testing.T) {
	_, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	r := setupUserCouponRouter()

	// 测试未授权访问用户优惠券列表
	req, _ := http.NewRequest("GET", "/api/user/coupons/", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 认证失败返回 401
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// TestGetAvailableCoupons_FiltersBoundCoupons 测试获取可领取优惠券时过滤绑定券
func TestGetAvailableCoupons_FiltersBoundCoupons(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)

	// 创建普通优惠券（未绑定）
	normalCoupon := createTestCoupon(t, db, "NormalCoupon", 100)

	// 创建绑定到兑换码的优惠券
	boundCoupon := createTestCoupon(t, db, "BoundCoupon", 100)
	now := time.Now().Unix()
	binding := &model.CouponRedemptionBinding{
		CouponId:     boundCoupon.Id,
		RedemptionId: 88001,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(binding).Error; err != nil {
		t.Fatalf("failed to create binding: %v", err)
	}

	r := setupUserCouponRouter()

	req, _ := http.NewRequest("GET", "/api/user/coupons/available", nil)
	addCouponUserHeaders(req, user)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].([]interface{})
	// 应该只返回1个未绑定的优惠券（排除绑定的 boundCoupon）
	assert.Equal(t, 1, len(data))

	// 验证返回的是普通优惠券
	returnedCoupon := data[0].(map[string]interface{})
	assert.Equal(t, float64(normalCoupon.Id), returnedCoupon["id"])
}

// TestGetAvailableCoupons_AllBoundCouponsFiltered 测试所有优惠券都绑定时返回空列表
func TestGetAvailableCoupons_AllBoundCouponsFiltered(t *testing.T) {
	db, cleanup := setupUserCouponTestDB(t)
	defer cleanup()

	user := createCouponTestUser(t, db)

	// 创建两个都绑定到兑换码的优惠券
	boundCoupon1 := createTestCoupon(t, db, "BoundCoupon1", 100)
	boundCoupon2 := createTestCoupon(t, db, "BoundCoupon2", 100)

	now := time.Now().Unix()
	binding1 := &model.CouponRedemptionBinding{
		CouponId:     boundCoupon1.Id,
		RedemptionId: 88002,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	binding2 := &model.CouponRedemptionBinding{
		CouponId:     boundCoupon2.Id,
		RedemptionId: 88003,
		Status:       common.CouponBindingStatusReserved,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := db.Create(binding1).Error; err != nil {
		t.Fatalf("failed to create binding1: %v", err)
	}
	if err := db.Create(binding2).Error; err != nil {
		t.Fatalf("failed to create binding2: %v", err)
	}

	r := setupUserCouponRouter()

	req, _ := http.NewRequest("GET", "/api/user/coupons/available", nil)
	addCouponUserHeaders(req, user)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.True(t, resp["success"].(bool))

	// 所有优惠券都绑定了，应该返回空列表
	if resp["data"] != nil {
		data := resp["data"].([]interface{})
		assert.Equal(t, 0, len(data))
	}
}

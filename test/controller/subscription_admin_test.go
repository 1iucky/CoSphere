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
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSubscriptionAdminTestDB(t *testing.T) (*gorm.DB, func()) {
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
		&model.SubscriptionOrder{},
		&model.SubscriptionUsage{},
		&model.UserBill{},
		&model.AuditLog{},
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, cleanup
}

func setupSubscriptionAdminRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test-session", store))

	admin := r.Group("/api/admin/subscriptions")
	admin.Use(middleware.AdminAuth())
	{
		admin.GET("/", controller.GetAllSubscriptions)
		admin.GET("/:id", controller.GetSubscriptionAdmin)
		admin.POST("/:id/cancel", controller.CancelSubscriptionAdmin)
		admin.POST("/:id/refund", controller.RefundSubscriptionAdmin)
		admin.POST("/:id/activate", controller.ActivateSubscriptionAdmin)
		admin.POST("/:id/expire", controller.ExpireSubscriptionAdmin)
	}

	return r
}

func createSubscriptionAdminUser(t *testing.T, db *gorm.DB, role int) *model.User {
	nano := strconv.FormatInt(time.Now().UnixNano(), 10)
	token := "test-token-sub-admin-" + strconv.Itoa(role) + "-" + nano
	user := &model.User{
		Username: "admin-sub-" + nano,
		Password: "password",
		Role:     role,
		Status:   common.UserStatusEnabled,
		Quota:    100000, // 100 元额度
		AffCode:  "aff-" + nano, // 唯一的 AffCode
	}
	user.SetAccessToken(token)
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func createTestPlan(t *testing.T, db *gorm.DB, name string) *model.SubscriptionPlan {
	plan := &model.SubscriptionPlan{
		Name:          name,
		Status:        common.PlanStatusActive,
		BillingCycle:  common.BillingCycleMonthly,
		PriceCents:    9900, // 99 元
		Currency:      common.CurrencyCNY,
		CreatedAt:     common.GetTimestamp(),
		UpdatedAt:     common.GetTimestamp(),
	}
	err := db.Create(plan).Error
	require.NoError(t, err)
	return plan
}

func createTestSubscriptionAdmin(t *testing.T, db *gorm.DB, userId int64, planId int64, status string) *model.Subscription {
	now := common.GetTimestamp()
	sub := &model.Subscription{
		UserId:    userId,
		PlanId:    planId,
		Status:    status,
		StartAt:   now,
		EndAt:     now + 30*24*3600, // 30 天后
		Priority:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	err := db.Create(sub).Error
	require.NoError(t, err)
	return sub
}

func addSubscriptionAdminHeaders(req *http.Request, user *model.User) {
	req.Header.Set("Authorization", user.GetAccessToken())
	req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	req.Header.Set("Content-Type", "application/json")
}

func decodeSubscriptionAdminResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	return response
}

// ===================== 测试 GetAllSubscriptions =====================

func TestGetAllSubscriptions_FullList(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user1 := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	user2 := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")

	// 创建多个订阅
	createTestSubscriptionAdmin(t, db, int64(user1.Id), plan.Id, common.SubscriptionStatusActive)
	createTestSubscriptionAdmin(t, db, int64(user2.Id), plan.Id, common.SubscriptionStatusActive)
	createTestSubscriptionAdmin(t, db, int64(user1.Id), plan.Id, common.SubscriptionStatusCancelled)

	router := setupSubscriptionAdminRouter()

	// 无筛选条件，应返回全部
	req, _ := http.NewRequest("GET", "/api/admin/subscriptions/", nil)
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.True(t, resp["success"].(bool))
	assert.Equal(t, float64(3), resp["total"].(float64))
}

func TestGetAllSubscriptions_FilterByStatus(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")

	// 创建不同状态的订阅
	createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusActive)
	createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusActive)
	createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusCancelled)

	router := setupSubscriptionAdminRouter()

	// 筛选活跃状态
	req, _ := http.NewRequest("GET", "/api/admin/subscriptions/?status=active", nil)
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.True(t, resp["success"].(bool))
	assert.Equal(t, float64(2), resp["total"].(float64))
}

func TestGetAllSubscriptions_CombinedFilter(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user1 := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	user2 := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")

	// 创建多个订阅
	createTestSubscriptionAdmin(t, db, int64(user1.Id), plan.Id, common.SubscriptionStatusActive)
	createTestSubscriptionAdmin(t, db, int64(user2.Id), plan.Id, common.SubscriptionStatusActive)
	createTestSubscriptionAdmin(t, db, int64(user1.Id), plan.Id, common.SubscriptionStatusCancelled)

	router := setupSubscriptionAdminRouter()

	// 组合筛选：user_id + status
	req, _ := http.NewRequest("GET", "/api/admin/subscriptions/?user_id="+strconv.Itoa(user1.Id)+"&status=active", nil)
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.True(t, resp["success"].(bool))
	assert.Equal(t, float64(1), resp["total"].(float64))
}

// ===================== 测试 CancelSubscriptionAdmin =====================

func TestCancelSubscriptionAdmin_Success(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")
	sub := createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusActive)

	router := setupSubscriptionAdminRouter()

	// 取消订阅
	cancelReq := map[string]string{
		"reason": "用户申请退款",
	}
	body, _ := json.Marshal(cancelReq)
	req, _ := http.NewRequest("POST", "/api/admin/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/cancel", bytes.NewReader(body))
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.True(t, resp["success"].(bool))

	// 验证数据库状态
	var updatedSub model.Subscription
	err := db.First(&updatedSub, sub.Id).Error
	require.NoError(t, err)
	assert.Equal(t, common.SubscriptionStatusCancelled, updatedSub.Status)
	// 验证 end_at 已设置为当前时间（允许 1 秒误差）
	now := common.GetTimestamp()
	assert.InDelta(t, now, updatedSub.EndAt, 2)

	// 验证取消原因存入 metadata
	metadata, err := updatedSub.GetMetadataAsMap()
	require.NoError(t, err)
	assert.Equal(t, "用户申请退款", metadata["cancel_reason"])

	// 验证审计日志
	var auditLog model.AuditLog
	err = db.Where("object_type = ? AND object_id = ? AND action = ?", "subscription", sub.Id, "cancel").First(&auditLog).Error
	require.NoError(t, err)
	assert.Equal(t, int64(admin.Id), *auditLog.OperatorId)
}

func TestCancelSubscriptionAdmin_AlreadyCancelled(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")
	sub := createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusCancelled)

	router := setupSubscriptionAdminRouter()

	req, _ := http.NewRequest("POST", "/api/admin/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/cancel", nil)
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.False(t, resp["success"].(bool))
	// 现在的错误消息是"只能取消活跃或待激活状态的订阅"
	assert.Contains(t, resp["message"].(string), "只能取消")
}

// ===================== 测试 RefundSubscriptionAdmin =====================

func TestRefundSubscriptionAdmin_Success(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	originalQuota := user.Quota
	plan := createTestPlan(t, db, "Test Plan")
	sub := createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusCancelled)

	router := setupSubscriptionAdminRouter()

	// 退款请求（现在仅记录流水，不增加余额）
	refundReq := map[string]interface{}{
		"amount":      5000, // 50 元
		"reason":      "用户申请部分退款",
		"refund_type": "manual", // 使用新的退款类型
	}
	body, _ := json.Marshal(refundReq)
	req, _ := http.NewRequest("POST", "/api/admin/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/refund", bytes.NewReader(body))
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.True(t, resp["success"].(bool))
	// 验证返回消息提示实际退款走线下处理
	assert.Contains(t, resp["message"].(string), "线下")

	// 验证用户余额不变（仅记录流水，不增加余额）
	var updatedUser model.User
	err := db.First(&updatedUser, user.Id).Error
	require.NoError(t, err)
	assert.Equal(t, originalQuota, updatedUser.Quota) // 余额应保持不变

	// 验证账单记录
	var bill model.UserBill
	err = db.Where("user_id = ? AND bill_type = ?", user.Id, common.BillTypeRefund).First(&bill).Error
	require.NoError(t, err)
	// Amount 设为 0 避免污染统计（退款金额通过 FinalAmount 和 metadata 记录）
	assert.Equal(t, int64(0), bill.Amount)
	assert.Equal(t, "manual", *bill.RefundType)
	// 验证余额前后一致（不实际增加余额）
	assert.Equal(t, bill.BalanceBefore, bill.BalanceAfter)
	// 验证 FinalAmount 记录了本次退款金额
	assert.Equal(t, int64(5000), bill.FinalAmount)
	// 验证 metadata 包含退款信息
	assert.NotNil(t, bill.Metadata)
	assert.Contains(t, *bill.Metadata, "refund_amount")

	// 验证审计日志
	var auditLog model.AuditLog
	err = db.Where("object_type = ? AND object_id = ? AND action = ?", "subscription", sub.Id, "refund").First(&auditLog).Error
	require.NoError(t, err)
	assert.Equal(t, int64(admin.Id), *auditLog.OperatorId)
}

func TestRefundSubscriptionAdmin_MissingAmount(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")
	sub := createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusCancelled)

	router := setupSubscriptionAdminRouter()

	// 缺少必填字段
	refundReq := map[string]interface{}{
		"reason": "退款原因",
	}
	body, _ := json.Marshal(refundReq)
	req, _ := http.NewRequest("POST", "/api/admin/subscriptions/"+strconv.FormatInt(sub.Id, 10)+"/refund", bytes.NewReader(body))
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.False(t, resp["success"].(bool))
	assert.Contains(t, resp["message"].(string), "参数错误")
}

func TestRefundSubscriptionAdmin_InvalidSubscription(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	router := setupSubscriptionAdminRouter()

	refundReq := map[string]interface{}{
		"amount": 5000,
		"reason": "退款原因",
	}
	body, _ := json.Marshal(refundReq)
	req, _ := http.NewRequest("POST", "/api/admin/subscriptions/99999/refund", bytes.NewReader(body))
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.False(t, resp["success"].(bool))
}

// ===================== 测试 GetSubscriptionAdmin 使用量摘要 =====================

func TestGetSubscriptionAdmin_WithUsageSummary(t *testing.T) {
	db, cleanup := setupSubscriptionAdminTestDB(t)
	defer cleanup()

	admin := createSubscriptionAdminUser(t, db, common.RoleAdminUser)
	user := createSubscriptionAdminUser(t, db, common.RoleCommonUser)
	plan := createTestPlan(t, db, "Test Plan")
	sub := createTestSubscriptionAdmin(t, db, int64(user.Id), plan.Id, common.SubscriptionStatusActive)

	// 创建使用量记录
	now := common.GetTimestamp()
	usage := &model.SubscriptionUsage{
		SubscriptionId: sub.Id,
		Period:         common.LimitPeriodDay,
		WindowStart:    now,
		WindowEnd:      now + 24*3600,
		UsedQuota:      5000,
		LimitQuota:     10000,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	err := db.Create(usage).Error
	require.NoError(t, err)

	router := setupSubscriptionAdminRouter()

	req, _ := http.NewRequest("GET", "/api/admin/subscriptions/"+strconv.FormatInt(sub.Id, 10), nil)
	addSubscriptionAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeSubscriptionAdminResponse(t, w)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.NotNil(t, data["usage_summary"])
	usageSummary := data["usage_summary"].([]interface{})
	assert.Len(t, usageSummary, 1)

	firstUsage := usageSummary[0].(map[string]interface{})
	assert.Equal(t, common.LimitPeriodDay, firstUsage["period"])
	assert.Equal(t, float64(5000), firstUsage["used_quota"])
	assert.Equal(t, float64(10000), firstUsage["limit_quota"])
	assert.Equal(t, float64(50), firstUsage["usage_rate"])
}

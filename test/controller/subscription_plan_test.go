package controller_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func setupSubscriptionPlanTestDB(t *testing.T) (*gorm.DB, func()) {
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
	)
	if err != nil {
		t.Fatalf("failed to migrate database: %v", err)
	}

	cleanup := func() {
		_ = sqlDB.Close()
	}

	return db, cleanup
}

func setupAdminRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	store := cookie.NewStore([]byte("test-secret"))
	r.Use(sessions.Sessions("test-session", store))

	admin := r.Group("/api/admin/subscription-plans")
	admin.Use(middleware.AdminAuth())
	{
		admin.GET("/", controller.GetAllSubscriptionPlans)
		admin.GET("/:id", controller.GetSubscriptionPlan)
		admin.POST("/", controller.CreateSubscriptionPlan)
		admin.PUT("/:id", controller.UpdateSubscriptionPlan)
		admin.POST("/:id/publish", controller.PublishSubscriptionPlan)
		admin.POST("/:id/unpublish", controller.UnpublishSubscriptionPlan)
		admin.DELETE("/:id", controller.DeleteSubscriptionPlan)
	}

	return r
}

func createAdminUser(t *testing.T, db *gorm.DB, role int) *model.User {
	token := "test-token-" + strconv.Itoa(role)
	user := &model.User{
		Username: "admin-" + strconv.Itoa(role),
		Password: "password",
		Role:     role,
		Status:   common.UserStatusEnabled,
	}
	user.SetAccessToken(token)
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	return user
}

func addAdminHeaders(req *http.Request, user *model.User) {
	req.Header.Set("Authorization", user.GetAccessToken())
	req.Header.Set("New-Api-User", strconv.Itoa(user.Id))
	req.Header.Set("Content-Type", "application/json")
}

func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	return response
}

func TestAdminAuthRequiredForSubscriptionPlans(t *testing.T) {
	_, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	router := setupAdminRouter()

	req, _ := http.NewRequest("GET", "/api/admin/subscription-plans/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAdminAuthRejectsNonAdmin(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	user := createAdminUser(t, db, common.RoleCommonUser)
	router := setupAdminRouter()

	req, _ := http.NewRequest("GET", "/api/admin/subscription-plans/", nil)
	addAdminHeaders(req, user)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	assert.False(t, resp["success"].(bool))
}

func TestCreateSubscriptionPlan(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	admin := createAdminUser(t, db, common.RoleAdminUser)
	router := setupAdminRouter()

	reqBody := dto.SubscriptionPlanRequest{
		Name:         "测试套餐",
		PriceCents:   10000,
		Currency:     "USD",
		BillingCycle: "monthly",
		Limits: []dto.SubscriptionPlanLimitRequest{
			{
				Period: "day",
				Quota:  100000,
				Unit:   "quota",
			},
		},
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", "/api/admin/subscription-plans/", bytes.NewBuffer(body))
	addAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	success, ok := resp["success"].(bool)
	if !assert.True(t, ok, "missing success flag: %v", resp) {
		return
	}
	if !assert.Truef(t, success, "create failed: %v", resp["message"]) {
		return
	}

	data := resp["data"].(map[string]interface{})
	planID := int64(data["id"].(float64))

	plan, err := model.GetSubscriptionPlanById(planID)
	assert.NoError(t, err)
	assert.Equal(t, reqBody.Name, plan.Name)
	assert.Len(t, plan.Limits, 1)
}

func TestUpdateSubscriptionPlan(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	admin := createAdminUser(t, db, common.RoleAdminUser)
	router := setupAdminRouter()

	plan := &model.SubscriptionPlan{
		Name:              "旧套餐",
		PriceCents:        10000,
		Currency:          "USD",
		BillingCycle:      common.BillingCycleMonthly,
		BillingCycleValue: 30,
		Status:            common.PlanStatusDraft,
		Limits: []model.SubscriptionPlanLimit{
			{
				Period: "day",
				Quota:  1000,
				Unit:   "quota",
			},
		},
	}
	err := model.CreateSubscriptionPlan(plan)
	assert.NoError(t, err)

	updateReq := dto.SubscriptionPlanRequest{
		Name:              "新套餐",
		PriceCents:        20000,
		Currency:          "USD",
		BillingCycle:      "monthly",
		BillingCycleValue: 30,
		Status:            common.PlanStatusActive,
		Limits: []dto.SubscriptionPlanLimitRequest{
			{
				Period: "day",
				Quota:  2000,
				Unit:   "quota",
			},
			{
				Period: "week",
				Quota:  8000,
				Unit:   "quota",
			},
		},
	}

	body, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", "/api/admin/subscription-plans/"+strconv.FormatInt(plan.Id, 10), bytes.NewBuffer(body))
	addAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	success, ok := resp["success"].(bool)
	if !assert.True(t, ok, "missing success flag: %v", resp) {
		return
	}
	if !assert.Truef(t, success, "update failed: %v", resp["message"]) {
		return
	}

	updated, err := model.GetSubscriptionPlanById(plan.Id)
	assert.NoError(t, err)
	assert.Equal(t, updateReq.Name, updated.Name)
	assert.Equal(t, updateReq.PriceCents, updated.PriceCents)
	assert.Equal(t, updateReq.Status, updated.Status)
	assert.Len(t, updated.Limits, 2)
}

func TestUpdateSubscriptionPlan_MissingRequiredFields(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	admin := createAdminUser(t, db, common.RoleAdminUser)
	router := setupAdminRouter()

	plan := &model.SubscriptionPlan{
		Name:              "旧套餐",
		PriceCents:        10000,
		Currency:          "USD",
		BillingCycle:      common.BillingCycleMonthly,
		BillingCycleValue: 30,
		Status:            common.PlanStatusDraft,
		Limits: []model.SubscriptionPlanLimit{
			{
				Period: "day",
				Quota:  1000,
				Unit:   "quota",
			},
		},
	}
	err := model.CreateSubscriptionPlan(plan)
	assert.NoError(t, err)

	updateReq := dto.SubscriptionPlanRequest{
		Name:       "缺少周期",
		PriceCents: 20000,
	}

	body, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("PUT", "/api/admin/subscription-plans/"+strconv.FormatInt(plan.Id, 10), bytes.NewBuffer(body))
	addAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	assert.False(t, resp["success"].(bool))
}

func TestDeleteSubscriptionPlan_SoftDelete(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	admin := createAdminUser(t, db, common.RoleAdminUser)
	router := setupAdminRouter()

	plan := &model.SubscriptionPlan{
		Name:              "待删除套餐",
		PriceCents:        10000,
		Currency:          "USD",
		BillingCycle:      common.BillingCycleMonthly,
		BillingCycleValue: 30,
		Status:            common.PlanStatusDraft,
		Limits: []model.SubscriptionPlanLimit{
			{
				Period: "day",
				Quota:  1000,
				Unit:   "quota",
			},
		},
	}
	err := model.CreateSubscriptionPlan(plan)
	assert.NoError(t, err)

	req, _ := http.NewRequest("DELETE", "/api/admin/subscription-plans/"+strconv.FormatInt(plan.Id, 10), nil)
	addAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	success, ok := resp["success"].(bool)
	if !assert.True(t, ok, "missing success flag: %v", resp) {
		return
	}
	if !assert.Truef(t, success, "list failed: %v", resp["message"]) {
		return
	}

	deleted, err := model.GetSubscriptionPlanById(plan.Id)
	assert.NoError(t, err)
	assert.Equal(t, common.PlanStatusArchived, deleted.Status)
}

func TestUnpublishSubscriptionPlan(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	admin := createAdminUser(t, db, common.RoleAdminUser)
	router := setupAdminRouter()

	plan := &model.SubscriptionPlan{
		Name:              "已上架套餐",
		PriceCents:        10000,
		Currency:          "USD",
		BillingCycle:      common.BillingCycleMonthly,
		BillingCycleValue: 30,
		Status:            common.PlanStatusActive,
		Limits: []model.SubscriptionPlanLimit{
			{
				Period: "day",
				Quota:  1000,
				Unit:   "quota",
			},
		},
	}
	err := model.CreateSubscriptionPlan(plan)
	assert.NoError(t, err)

	req, _ := http.NewRequest("POST", "/api/admin/subscription-plans/"+strconv.FormatInt(plan.Id, 10)+"/unpublish", nil)
	addAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	assert.True(t, resp["success"].(bool))

	unpublished, err := model.GetSubscriptionPlanById(plan.Id)
	assert.NoError(t, err)
	assert.Equal(t, common.PlanStatusArchived, unpublished.Status)
}

func TestGetAllSubscriptionPlans_ReturnsDisabledLimits(t *testing.T) {
	db, cleanup := setupSubscriptionPlanTestDB(t)
	defer cleanup()

	admin := createAdminUser(t, db, common.RoleAdminUser)
	router := setupAdminRouter()

	plan := &model.SubscriptionPlan{
		Name:              "限额套餐",
		PriceCents:        10000,
		Currency:          "USD",
		BillingCycle:      common.BillingCycleMonthly,
		BillingCycleValue: 30,
		Status:            common.PlanStatusActive,
		Limits: []model.SubscriptionPlanLimit{
			{
				Period:  "day",
				Quota:   1000,
				Unit:    "quota",
				Enabled: true,
			},
			{
				Period:  "week",
				Quota:   5000,
				Unit:    "quota",
				Enabled: false,
			},
		},
	}
	if assert.Len(t, plan.Limits, 2) {
		assert.Falsef(t, plan.Limits[1].Enabled, "plan.Limits[1].Enabled should be false")
	}
	err := model.CreateSubscriptionPlan(plan)
	assert.NoError(t, err)

	limits, err := model.GetPlanLimitsByPlanId(plan.Id)
	assert.NoError(t, err)
	if assert.Len(t, limits, 2) {
		var weekEnabled *bool
		for _, limit := range limits {
			if limit.Period == "week" {
				enabled := limit.Enabled
				weekEnabled = &enabled
				break
			}
		}
		if assert.NotNil(t, weekEnabled) {
			assert.Falsef(t, *weekEnabled, "db week limit should be disabled")
		}
	}
	var rawWeekEnabled int
	err = db.Raw("SELECT enabled FROM subscription_plan_limits WHERE plan_id = ? AND period = ?", plan.Id, "week").
		Scan(&rawWeekEnabled).Error
	assert.NoError(t, err)
	assert.Equal(t, 0, rawWeekEnabled)

	detailReq, _ := http.NewRequest("GET", fmt.Sprintf("/api/admin/subscription-plans/%d", plan.Id), nil)
	addAdminHeaders(detailReq, admin)
	detailW := httptest.NewRecorder()
	router.ServeHTTP(detailW, detailReq)
	assert.Equal(t, http.StatusOK, detailW.Code)
	detailResp := decodeResponse(t, detailW)
	detailData := detailResp["data"].(map[string]interface{})
	detailLimits := detailData["limits"].([]interface{})
	var detailHasDisabled bool
	for _, limit := range detailLimits {
		limitMap := limit.(map[string]interface{})
		if enabled, ok := limitMap["enabled"].(bool); ok && !enabled {
			detailHasDisabled = true
			break
		}
	}
	assert.True(t, detailHasDisabled)

	req, _ := http.NewRequest("GET", "/api/admin/subscription-plans/", nil)
	addAdminHeaders(req, admin)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	resp := decodeResponse(t, w)
	assert.True(t, resp["success"].(bool))

	data := resp["data"].([]interface{})
	assert.Len(t, data, 1)
	planMap := data[0].(map[string]interface{})
	listLimits := planMap["limits"].([]interface{})
	assert.Len(t, listLimits, 2)
	hasWeek := false
	for _, limit := range listLimits {
		limitMap := limit.(map[string]interface{})
		if period, ok := limitMap["period"].(string); ok && period == "week" {
			hasWeek = true
			break
		}
	}
	if !assert.True(t, hasWeek) {
		t.Fatalf("expected week limit in list, got: %+v, raw: %s", listLimits, w.Body.String())
	}
}

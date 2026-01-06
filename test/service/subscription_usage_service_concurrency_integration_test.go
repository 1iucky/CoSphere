package service

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTestSQLiteDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()

	// Save and restore global state to avoid affecting other tests.
	prevDB := model.DB
	prevSQLitePath := common.SQLitePath
	prevUsingSQLite := common.UsingSQLite
	prevUsingMySQL := common.UsingMySQL
	prevUsingPostgreSQL := common.UsingPostgreSQL

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "subscription_usage_test.db")
	common.SQLitePath = dbPath + "?_busy_timeout=30000"

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

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

	// Minimal schema for SubscriptionUsageService.
	if err := db.AutoMigrate(&model.Subscription{}, &model.SubscriptionUsage{}); err != nil {
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
	}

	return db, cleanup
}

func isUsageQuotaExceeded(err error) bool {
	var apiErr *types.NewAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.GetErrorCode() == types.ErrorCodeUsageQuotaExceeded && apiErr.StatusCode == http.StatusTooManyRequests
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "busy") ||
		strings.Contains(msg, "locked")
}

// TestConcurrentTryPreConsume1000_NoOverConsume 2.4.9 并发测试（数据库原子性）
// 目标：1000 并发下，used_quota 不会超过 limit_quota，并且超额请求返回 429 + usage_quota_exceeded。
func TestConcurrentTryPreConsume1000_NoOverConsume(t *testing.T) {
	db, cleanup := setupTestSQLiteDB(t)
	defer cleanup()

	svc := service.GetSubscriptionUsageService()

	subscriptionId := int64(999)
	concurrency := 1000
	amount := int64(1)
	limitQuota := int64(100)
	period := common.LimitPeriodDay

	now := common.GetTimestamp()

	// Create a subscription row to match production behavior (window creation may lock this row).
	sub := &model.Subscription{
		Id:                 subscriptionId,
		UserId:             1,
		PlanId:             1,
		Status:             common.SubscriptionStatusActive,
		StartAt:            now - 3600,
		EndAt:              now + 24*3600,
		Priority:           1,
		AutoWalletFallback: false,
		RedeemOption:       common.RedeemOptionStack,
	}
	if err := db.Create(sub).Error; err != nil {
		t.Fatalf("create subscription failed: %v", err)
	}

	// Pre-create one active window so the test focuses on atomic UPDATE (no window-creation races).
	windowStart := common.GetTimestamp()
	windowEnd := model.CalculateRollingWindowEnd(period, windowStart)
	initialUsage := &model.SubscriptionUsage{
		SubscriptionId: subscriptionId,
		Period:         period,
		WindowStart:    windowStart,
		WindowEnd:      windowEnd,
		UsedQuota:      0,
		LimitQuota:     limitQuota,
	}
	if err := db.Create(initialUsage).Error; err != nil {
		t.Fatalf("create initial usage window failed: %v", err)
	}

	periods := map[string]int64{period: limitQuota}

	start := make(chan struct{})
	var ready sync.WaitGroup
	var wg sync.WaitGroup

	var successCount int64
	var quotaExceededCount int64
	var otherErrCount int64

	errCh := make(chan error, concurrency)

	ready.Add(concurrency)
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			ready.Done()
			<-start

			var lastErr error
			for attempt := 0; attempt < 10; attempt++ {
				_, err := svc.TryPreConsume(subscriptionId, amount, periods, common.WindowStrategyRolling)
				if err == nil {
					atomic.AddInt64(&successCount, 1)
					return
				}
				if isUsageQuotaExceeded(err) {
					atomic.AddInt64(&quotaExceededCount, 1)
					return
				}
				if isSQLiteBusy(err) {
					lastErr = err
					time.Sleep(time.Duration(attempt+1) * 5 * time.Millisecond)
					continue
				}
				lastErr = err
				break
			}

			atomic.AddInt64(&otherErrCount, 1)
			if lastErr != nil {
				errCh <- lastErr
			}
		}()
	}

	ready.Wait()
	close(start)
	wg.Wait()
	close(errCh)

	if atomic.LoadInt64(&otherErrCount) > 0 {
		// Log a few sample errors for debugging.
		samples := 0
		for err := range errCh {
			t.Logf("unexpected error: %v", err)
			samples++
			if samples >= 5 {
				break
			}
		}
		t.Fatalf("unexpected errors occurred: %d", atomic.LoadInt64(&otherErrCount))
	}

	finalSuccess := atomic.LoadInt64(&successCount)
	finalQuotaExceeded := atomic.LoadInt64(&quotaExceededCount)

	// Verify DB state: used_quota == successCount * amount and never exceeds limit_quota.
	var got model.SubscriptionUsage
	if err := db.First(&got, "subscription_id = ? AND period = ? AND window_start = ?", subscriptionId, period, windowStart).Error; err != nil {
		t.Fatalf("load usage window failed: %v", err)
	}

	expectedUsed := finalSuccess * amount
	if got.UsedQuota != expectedUsed {
		t.Fatalf("used_quota mismatch: expected %d, got %d (success=%d, exceeded=%d)", expectedUsed, got.UsedQuota, finalSuccess, finalQuotaExceeded)
	}
	if got.UsedQuota > got.LimitQuota {
		t.Fatalf("used_quota exceeded limit: used=%d, limit=%d", got.UsedQuota, got.LimitQuota)
	}

	if expectedMaxSuccess := limitQuota / amount; finalSuccess != expectedMaxSuccess {
		t.Fatalf("success count mismatch: expected %d, got %d (exceeded=%d)", expectedMaxSuccess, finalSuccess, finalQuotaExceeded)
	}
	if finalQuotaExceeded != int64(concurrency)-finalSuccess {
		t.Fatalf("quota exceeded count mismatch: expected %d, got %d", int64(concurrency)-finalSuccess, finalQuotaExceeded)
	}

	t.Logf("concurrency test ok: success=%d exceeded=%d used=%d limit=%d", finalSuccess, finalQuotaExceeded, got.UsedQuota, got.LimitQuota)
}

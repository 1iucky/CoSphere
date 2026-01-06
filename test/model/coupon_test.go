package model_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func TestCouponIsValid_WithStock(t *testing.T) {
	now := common.GetTimestamp()
	coupon := &model.Coupon{
		Status:     common.CouponStatusActive,
		ValidFrom:  now - 10,
		ValidTo:    now + 10,
		TotalCount: 1000,
		UsedCount:  500,
	}

	if !coupon.IsValid() {
		t.Fatalf("expected coupon.IsValid() to be true when stock available, got false")
	}
}

func TestCouponIsValid_OutOfStock(t *testing.T) {
	now := common.GetTimestamp()
	coupon := &model.Coupon{
		Status:     common.CouponStatusActive,
		ValidFrom:  now - 10,
		ValidTo:    now + 10,
		TotalCount: 1000,
		UsedCount:  1000,
	}

	if coupon.IsValid() {
		t.Fatalf("expected coupon.IsValid() to be false when out of stock, got true")
	}
}

package httpapi

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	limiter := NewLoginLimiter(2, time.Minute)
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	limiter.RecordFailure("key", now)
	if limiter.Blocked("key", now) {
		t.Fatal("should not block before threshold")
	}
	limiter.RecordFailure("key", now)
	if !limiter.Blocked("key", now) {
		t.Fatal("should block at threshold")
	}
	if limiter.Blocked("key", now.Add(2*time.Minute)) {
		t.Fatal("should reset after window")
	}
}

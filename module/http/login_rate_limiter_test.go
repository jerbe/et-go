package http

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jerbe/et-go/db"
)

func TestLoginRateLimiterRejectsAfterLimit(t *testing.T) {
	limiter, err := NewLoginRateLimiterComponent(2, 0)
	if err != nil {
		t.Fatalf("NewLoginRateLimiterComponent error = %v", err)
	}
	if !limiter.Allow("127.0.0.1|user") {
		t.Fatal("first request should be allowed")
	}
	if !limiter.Allow("127.0.0.1|user") {
		t.Fatal("second request should be allowed")
	}
	if limiter.Allow("127.0.0.1|user") {
		t.Fatal("third request should be rejected")
	}
	if !limiter.Allow("127.0.0.1|other") {
		t.Fatal("different username should use an independent key")
	}
}

func TestLoginRateLimitKeyNormalizesRemoteAndUsername(t *testing.T) {
	if got := loginRateLimitKey("127.0.0.1:1234", " User "); got != "127.0.0.1|user" {
		t.Fatalf("loginRateLimitKey = %q, want %q", got, "127.0.0.1|user")
	}
}

func TestLoginRateLimiterContextCancellation(t *testing.T) {
	limiter, err := NewLoginRateLimiterComponent(1, time.Minute)
	if err != nil {
		t.Fatalf("NewLoginRateLimiterComponent error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := limiter.AllowContext(ctx, "127.0.0.1|user"); err == nil {
		t.Fatal("AllowContext should return canceled context error")
	}
}

func TestMongoLoginRateLimiterRequiresExplicitStore(t *testing.T) {
	if limiter, err := NewMongoLoginRateLimiterComponent(nil, 1, time.Minute); limiter != nil ||
		!errors.Is(err, ErrLoginRateLimiterInvalid) {
		t.Fatalf("NewMongoLoginRateLimiterComponent result = limiter %#v err %v", limiter, err)
	}
	if store, err := NewMongoLoginRateLimitStore(nil); store != nil ||
		!errors.Is(err, ErrLoginRateLimiterInvalid) {
		t.Fatalf("NewMongoLoginRateLimitStore result = store %#v err %v", store, err)
	}
}

func TestDBManagerLoginRateLimiterFailsWithoutDatabase(t *testing.T) {
	limiter, err := NewDBManagerLoginRateLimiterComponent(&db.DBManagerComponent{}, 1, 1, time.Minute)
	if err != nil {
		t.Fatalf("NewDBManagerLoginRateLimiterComponent error = %v", err)
	}
	if _, err := limiter.AllowContext(context.Background(), "127.0.0.1|user"); err == nil {
		t.Fatal("AllowContext should fail without configured database")
	}
}

package ratelimit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eden9th/bedrock/ratelimit"
)

func TestLimiterStartsWithBurstTokens(t *testing.T) {
	l := ratelimit.New(10, 2)

	if !l.Allow() || !l.Allow() {
		t.Fatal("expected initial burst tokens to be available")
	}
	if l.Allow() {
		t.Fatal("expected third token to be unavailable before refill")
	}
}

func TestWaitNExceedsBurst(t *testing.T) {
	l := ratelimit.New(10, 2)

	err := l.WaitN(context.Background(), 3)
	if !errors.Is(err, ratelimit.ErrExceedsBurst) {
		t.Fatalf("expected ErrExceedsBurst, got %v", err)
	}
}

func TestNewKeyLimiterWithZeroTTL(t *testing.T) {
	kl := ratelimit.NewKeyLimiter(0, 0, 0)
	defer kl.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := kl.Wait(ctx, "k"); err != nil {
		t.Fatalf("expected zero-value config to be normalized, got %v", err)
	}
}

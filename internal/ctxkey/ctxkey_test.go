package ctxkey_test

import (
	"context"
	"testing"

	"github.com/eden9th/bedrock/internal/ctxkey"
)

func TestTraceID_RoundTrip(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.TraceID, "abc-123")
	got, ok := ctx.Value(ctxkey.TraceID).(string)
	if !ok {
		t.Fatal("expected string value from context, got none")
	}
	if got != "abc-123" {
		t.Fatalf("expected %q, got %q", "abc-123", got)
	}
}

func TestTraceID_TypeIsolation(t *testing.T) {
	// 确保使用不同 key 类型存入的值不会互相干扰
	type otherKey struct{}
	ctx := context.WithValue(context.Background(), otherKey{}, "other-value")

	// ctxkey.TraceID 取不到 otherKey 存入的值
	got := ctx.Value(ctxkey.TraceID)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestTraceID_NilContext(t *testing.T) {
	// context.WithValue 要求 ctx 非 nil，这里验证正常使用场景
	ctx := context.Background()
	got := ctx.Value(ctxkey.TraceID)
	if got != nil {
		t.Fatalf("empty context should return nil, got %v", got)
	}
}

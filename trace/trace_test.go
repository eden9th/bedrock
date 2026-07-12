package trace_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eden9th/bedrock/bm"
	"github.com/eden9th/bedrock/trace"
)

func TestNewTraceID_Unique(t *testing.T) {
	a := trace.NewTraceID()
	b := trace.NewTraceID()
	if a == "" {
		t.Fatal("NewTraceID returned empty string")
	}
	if a == b {
		t.Fatalf("NewTraceID returned duplicate values: %q", a)
	}
}

func TestWithTraceID_GetTraceID_RoundTrip(t *testing.T) {
	ctx := trace.WithTraceID(context.Background(), "test-trace-id")
	got := trace.GetTraceID(ctx)
	if got != "test-trace-id" {
		t.Fatalf("expected %q, got %q", "test-trace-id", got)
	}
}

func TestGetTraceID_EmptyContext(t *testing.T) {
	got := trace.GetTraceID(context.Background())
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestMiddleware_GeneratesTraceID(t *testing.T) {
	e := bm.New()
	e.Use(trace.Middleware())

	var capturedTraceID string
	e.GET("/ping", func(c *bm.Context) {
		capturedTraceID = trace.GetTraceID(c.Request.Context())
		c.String(200, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if capturedTraceID == "" {
		t.Fatal("expected trace_id to be injected into context, got empty string")
	}
	// 响应 header 应包含相同的 trace_id
	respTraceID := w.Header().Get(trace.HeaderName)
	if respTraceID != capturedTraceID {
		t.Fatalf("response header trace_id %q != context trace_id %q", respTraceID, capturedTraceID)
	}
}

func TestMiddleware_ReuseExistingTraceID(t *testing.T) {
	e := bm.New()
	e.Use(trace.Middleware())

	const incomingID = "upstream-trace-id"
	var capturedTraceID string
	e.GET("/ping", func(c *bm.Context) {
		capturedTraceID = trace.GetTraceID(c.Request.Context())
		c.String(200, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.Header.Set(trace.HeaderName, incomingID)
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	if capturedTraceID != incomingID {
		t.Fatalf("expected reused trace_id %q, got %q", incomingID, capturedTraceID)
	}
	if w.Header().Get(trace.HeaderName) != incomingID {
		t.Fatalf("response header should echo incoming trace_id %q", incomingID)
	}
}

func TestInjectRequest_WithTraceID(t *testing.T) {
	ctx := trace.WithTraceID(context.Background(), "inject-me")
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	trace.InjectRequest(ctx, req)

	got := req.Header.Get(trace.HeaderName)
	if got != "inject-me" {
		t.Fatalf("expected header %q, got %q", "inject-me", got)
	}
}

func TestInjectRequest_NoTraceID(t *testing.T) {
	ctx := context.Background()
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	trace.InjectRequest(ctx, req)

	got := req.Header.Get(trace.HeaderName)
	if got != "" {
		t.Fatalf("expected no header when trace_id absent, got %q", got)
	}
}

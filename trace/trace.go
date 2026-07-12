// Package trace 提供轻量级请求追踪能力。
// 每个请求生成唯一 trace_id，存入 context，通过 HTTP header X-Trace-Id 传播。
package trace

import (
	"context"
	"net/http"

	"github.com/eden9th/bedrock/bm"
	"github.com/eden9th/bedrock/internal/ctxkey"

	"github.com/google/uuid"
)

const HeaderName = "X-Trace-Id"

// NewTraceID 生成一个新的 trace_id（UUID v4）
func NewTraceID() string {
	return uuid.New().String()
}

// WithTraceID 将 trace_id 注入 context，供 log 包自动提取
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, ctxkey.TraceID, traceID)
}

// GetTraceID 从 context 中取出 trace_id，不存在返回空字符串
func GetTraceID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxkey.TraceID).(string); ok {
		return id
	}
	return ""
}

// Middleware bm 中间件：从请求 header 读取或生成 trace_id，注入 context，并写入响应 header
func Middleware() bm.HandlerFunc {
	return func(c *bm.Context) {
		traceID := c.GetHeader(HeaderName)
		if traceID == "" {
			traceID = NewTraceID()
		}
		ctx := WithTraceID(c.Request.Context(), traceID)
		c.Request = c.Request.WithContext(ctx)
		c.Header(HeaderName, traceID)
		c.Next()
	}
}

// InjectRequest 向外发出的 HTTP 请求注入 trace_id header
func InjectRequest(ctx context.Context, req *http.Request) {
	if traceID := GetTraceID(ctx); traceID != "" {
		req.Header.Set(HeaderName, traceID)
	}
}

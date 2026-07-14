package bm

import (
	"context"
	"net/http"
	"time"
)

// Timeout 返回一个请求超时中间件。
// handler 链执行超过 d 时间后，context 被取消，返回 HTTP 504 Gateway Timeout。
//
// 使用：
//
//	e.Use(bm.Timeout(30 * time.Second))              // 全局 30s 超时
//	api := e.Group("/api", bm.Timeout(10 * time.Second)) // 分组 10s 超时
//
// handler 应通过 c.Request.Context() 感知超时：
//
//	select {
//	case <-c.Request.Context().Done():
//	    return // 超时
//	default:
//	    // 正常处理
//	}
func Timeout(d time.Duration) HandlerFunc {
	return func(c *Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()

		// 替换 request 的 context，让后续 handler 感知超时
		c.Request = c.Request.WithContext(ctx)

		done := make(chan struct{})
		go func() {
			c.Next()
			close(done)
		}()

		select {
		case <-done:
			return
		case <-ctx.Done():
			// 超时：设置 504 状态码（仅当尚未写入响应头）
			if rw, ok := c.Writer.(*responseWriter); ok && !rw.wroteHeader {
				c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				c.Writer.WriteHeader(http.StatusGatewayTimeout)
				c.Writer.Write([]byte(`{"detail":"Request Timeout"}`))
			}
			c.Abort()
			return
		}
	}
}

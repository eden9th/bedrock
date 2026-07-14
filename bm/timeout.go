package bm

import (
	"bytes"
	"context"
	"net/http"
	"sync"
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

		origWriter := c.Writer
		tw := newTimeoutWriter()
		c.Writer = tw

		done := make(chan struct{})
		panicCh := make(chan any, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			c.Next()
			close(done)
		}()

		select {
		case <-done:
			c.Writer = origWriter
			tw.flushTo(origWriter)
			return
		case r := <-panicCh:
			c.Writer = origWriter
			panic(r)
		case <-ctx.Done():
			c.Writer = origWriter
			tw.markTimedOut()
			origWriter.Header().Set("Content-Type", "application/json; charset=utf-8")
			origWriter.WriteHeader(http.StatusGatewayTimeout)
			_, _ = origWriter.Write([]byte(`{"detail":"Request Timeout"}`))
			c.Abort()
			return
		}
	}
}

type timeoutWriter struct {
	mu          sync.Mutex
	header      http.Header
	body        bytes.Buffer
	status      int
	wroteHeader bool
	timedOut    bool
}

func newTimeoutWriter() *timeoutWriter {
	return &timeoutWriter{header: make(http.Header), status: http.StatusOK}
}

func (w *timeoutWriter) Header() http.Header {
	return w.header
}

func (w *timeoutWriter) WriteHeader(status int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timedOut || w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
}

func (w *timeoutWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timedOut {
		return len(p), nil
	}
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.body.Write(p)
}

func (w *timeoutWriter) markTimedOut() {
	w.mu.Lock()
	w.timedOut = true
	w.mu.Unlock()
}

func (w *timeoutWriter) flushTo(dst http.ResponseWriter) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timedOut {
		return
	}
	for k, values := range w.header {
		for _, v := range values {
			dst.Header().Add(k, v)
		}
	}
	if w.wroteHeader {
		dst.WriteHeader(w.status)
	}
	if w.body.Len() > 0 {
		_, _ = dst.Write(w.body.Bytes())
	}
}

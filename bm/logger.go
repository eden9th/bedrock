package bm

import (
	"encoding/json"
	"os"
	"time"
)

// Logger 返回一个 access log 中间件，以 JSON 格式输出每个请求。
// 输出到 stderr，格式为：
//
//	{"time":"...","method":"GET","path":"/api/users/:id","status":200,"duration_ms":1.2,"bytes":123}
//
// 使用：
//
//	e.Use(bm.Logger())
func Logger() HandlerFunc {
	return func(c *Context) {
		start := time.Now()
		c.Next()
		duration := float64(time.Since(start).Microseconds()) / 1000.0 // μs → ms

		method := c.Request.Method
		path := c.Pattern()
		if path == "" {
			path = c.Request.URL.Path
		}
		status := c.WriterStatus()
		bytes := c.BytesWritten()

		_ = json.NewEncoder(os.Stderr).Encode(accessLog{
			Time:       start.Format("2006/01/02-15:04:05.000"),
			Method:     method,
			Path:       path,
			Status:     status,
			DurationMS: duration,
			Bytes:      bytes,
		})
	}
}

type accessLog struct {
	Time       string  `json:"time"`
	Method     string  `json:"method"`
	Path       string  `json:"path"`
	Status     int     `json:"status"`
	DurationMS float64 `json:"duration_ms"`
	Bytes      int     `json:"bytes"`
}

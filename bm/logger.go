package bm

import (
	"fmt"
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

		fmt.Fprintf(os.Stderr,
			`{"time":"%s","method":"%s","path":"%s","status":%d,"duration_ms":%.2f,"bytes":%d}`+"\n",
			start.Format("2006/01/02-15:04:05.000"),
			method, path, status, duration, bytes,
		)
	}
}

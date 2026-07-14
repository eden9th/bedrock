package bm

import (
	"fmt"
	"net/http/httputil"
	"os"
	"runtime"
)

// Recovery 返回一个 panic recovery 中间件。
// 当 handler 链中的任意 handler panic 时，捕获 panic 并打印堆栈和请求信息，
// 返回 HTTP 500 Internal Server Error，防止进程崩溃。
//
// panic 日志包含：
//   - panic 值和完整堆栈
//   - HTTP 请求 dump（method / path / headers，不含 body）
//
// 使用：
//
//	e.Use(bm.Recovery())                    // 所有路由生效
//	api := e.Group("/api", bm.Recovery())    // 仅分组生效
func Recovery() HandlerFunc {
	return func(c *Context) {
		defer func() {
			if r := recover(); r != nil {
				// 打印堆栈和请求 dump 到 stderr
				buf := make([]byte, 64<<10) // 64KB 足够完整堆栈
				n := runtime.Stack(buf, false)

				var reqDump []byte
				if c.Request != nil {
					reqDump, _ = httputil.DumpRequest(c.Request, false)
				}

				fmt.Fprintf(os.Stderr, "[bm] panic recovered: %v\n\nRequestDump:\n%s\n\nStack:\n%s\n",
					r, reqDump, buf[:n])

				// 如果还未写入响应头，返回 500
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}

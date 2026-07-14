// Package bm 提供轻量级 HTTP 框架
// 核心概念：
//   - Context：封装 http.ResponseWriter + *http.Request，提供 JSON/Abort/Param 等快捷方法
//   - Engine：路由注册 + 中间件链，支持路径参数（:name）和通配符（*filepath）
package bm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// responseWriter 包装 http.ResponseWriter，追踪写入的状态码和字节数。
// 供中间件（如监控）在 handler 链结束后获取响应元数据。
type responseWriter struct {
	nethttp.ResponseWriter
	status       int
	bytesWritten int
	wroteHeader  bool
}

func (rw *responseWriter) WriteHeader(status int) {
	if rw.wroteHeader {
		return
	}
	rw.status = status
	rw.wroteHeader = true
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(nethttp.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

// unwrap 返回原始的 ResponseWriter（类型断言用）
func (rw *responseWriter) Unwrap() nethttp.ResponseWriter {
	return rw.ResponseWriter
}

func (rw *responseWriter) Flush() {
	f, ok := rw.ResponseWriter.(nethttp.Flusher)
	if !ok {
		return
	}
	if !rw.wroteHeader {
		rw.WriteHeader(nethttp.StatusOK)
	}
	f.Flush()
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(nethttp.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("bm: underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

func (rw *responseWriter) Push(target string, opts *nethttp.PushOptions) error {
	p, ok := rw.ResponseWriter.(nethttp.Pusher)
	if !ok {
		return nethttp.ErrNotSupported
	}
	return p.Push(target, opts)
}

func (rw *responseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(nethttp.StatusOK)
	}
	if rf, ok := rw.ResponseWriter.(io.ReaderFrom); ok {
		n, err := rf.ReadFrom(r)
		rw.bytesWritten += int(n)
		return n, err
	}
	var written int64
	buf := make([]byte, 32*1024)
	for {
		nr, er := r.Read(buf)
		if nr > 0 {
			nw, ew := rw.Write(buf[:nr])
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nw != nr {
				return written, io.ErrShortWrite
			}
		}
		if er == io.EOF {
			return written, nil
		}
		if er != nil {
			return written, er
		}
	}
}

// HandlerFunc 是 bm handler 函数签名，与 blademaster 一致
type HandlerFunc func(c *Context)

// Context 封装单次请求的上下文，贯穿 handler 链
type Context struct {
	Writer  nethttp.ResponseWriter
	Request *nethttp.Request

	// params 是从 URL 路径中提取的命名参数，如 /api/tokens/:token
	params map[string]string

	// pattern 是匹配到的路由注册时的原始 pattern，如 /api/tokens/:token
	// 由 Engine.ServeHTTP 在匹配成功后写入，供中间件获取路由模板
	pattern string

	// values 存储通过 Set/Get 在 handler 链中传递的任意值
	values map[string]any

	// aborted 表示中间件链已被终止
	aborted bool

	// index / handlers 用于中间件链执行
	index    int
	handlers []HandlerFunc
}

// Param 取路径参数，如路由 /tokens/:token，Param("token") 返回对应值
func (c *Context) Param(key string) string {
	return c.params[key]
}

// Pattern 返回匹配到的路由注册时的原始 pattern，如 /api/users/:id。
// 未匹配到路由时（如 404）返回空字符串。供监控中间件等场景获取路由模板。
func (c *Context) Pattern() string {
	return c.pattern
}

// WriterStatus 返回实际写入的 HTTP 状态码。
// 在 handler 链执行期间调用时，返回当前已写入的状态码（由首个 WriteHeader 调用确定）。
// handler 链执行完毕后调用时，返回最终状态码（默认 200）。
func (c *Context) WriterStatus() int {
	if rw, ok := c.Writer.(*responseWriter); ok && rw.wroteHeader {
		return rw.status
	}
	return nethttp.StatusOK
}

// BytesWritten 返回已写入的响应字节数。
func (c *Context) BytesWritten() int {
	if rw, ok := c.Writer.(*responseWriter); ok {
		return rw.bytesWritten
	}
	return 0
}

// Query 取 URL query 参数，等价于 c.Request.URL.Query().Get(key)
func (c *Context) Query(key string) string {
	return c.Request.URL.Query().Get(key)
}

// FormValue 取表单或 query 参数
func (c *Context) FormValue(key string) string {
	return c.Request.FormValue(key)
}

// GetHeader 取请求 header
func (c *Context) GetHeader(key string) string {
	return c.Request.Header.Get(key)
}

// Header 设置响应 header
func (c *Context) Header(key, value string) {
	c.Writer.Header().Set(key, value)
}

// response 是标准响应信封，对齐 blademaster 格式：
//
//	{"code": 0, "message": "ok", "data": ...}
//	{"code": -1, "message": "some error", "data": nil}
type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

// JSON 输出标准响应信封：
//   - err == nil → {"code":0,"message":"ok","data":data}
//   - err != nil → {"code":-1,"message":"<err>","data":nil}，HTTP 200
//
// handler 层一般使用 httputil.JSON，c.JSON 供需要信封格式的场景显式调用。
func (c *Context) JSON(data any, err error) {
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(nethttp.StatusOK)
	if err != nil {
		json.NewEncoder(c.Writer).Encode(response{Code: -1, Message: err.Error(), Data: nil})
		return
	}
	json.NewEncoder(c.Writer).Encode(response{Code: 0, Message: "ok", Data: data})
}

// String 输出纯文本
func (c *Context) String(status int, body string) {
	c.Writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	c.Writer.WriteHeader(status)
	c.Writer.Write([]byte(body))
}

// Redirect 302 跳转
func (c *Context) Redirect(url string) {
	nethttp.Redirect(c.Writer, c.Request, url, nethttp.StatusFound)
}

// AbortWithStatus 终止中间件链，写入状态码，4xx/5xx 自动附带 JSON body
func (c *Context) AbortWithStatus(status int) {
	c.aborted = true
	if status >= 400 {
		c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		c.Writer.WriteHeader(status)
		msg := nethttp.StatusText(status)
		json.NewEncoder(c.Writer).Encode(map[string]string{"detail": msg})
		return
	}
	c.Writer.WriteHeader(status)
}

// Abort 终止中间件链（不写状态码，由调用方自行写）
func (c *Context) Abort() {
	c.aborted = true
}

// IsAborted 返回是否已终止
func (c *Context) IsAborted() bool {
	return c.aborted
}

// Next 显式调用下一个 handler（用于中间件）
func (c *Context) Next() {
	c.index++
	for c.index < len(c.handlers) {
		if c.aborted {
			return
		}
		c.handlers[c.index](c)
		c.index++
	}
}

// Set 在 handler 链中存储任意值，供后续 handler 通过 Get 取出。
// 使用内部 map 存储，避免每次调用 request.WithContext 产生 GC 压力。
func (c *Context) Set(key string, val any) {
	if c.values == nil {
		c.values = make(map[string]any)
	}
	c.values[key] = val
}

// Get 取出通过 Set 存入的值，key 不存在时返回 nil, false。
func (c *Context) Get(key string) (any, bool) {
	if c.values == nil {
		return nil, false
	}
	val, ok := c.values[key]
	return val, ok
}

// ── Engine ────────────────────────────────────────────────────────────────

// route 是一条已注册路由
type route struct {
	method   string
	pattern  string   // 原始 pattern，如 /api/tokens/:token
	segments []string // 按 / 分割后的 segment 列表
	handlers []HandlerFunc
}

// Engine 是路由引擎，持有全局中间件和路由表
type Engine struct {
	mu         sync.RWMutex
	routes     []*route
	middleware []HandlerFunc

	// inflight 追踪正在处理中的请求，用于优雅关闭时等待请求完成
	inflight sync.WaitGroup
}

// New 创建空 Engine
func New() *Engine {
	return &Engine{}
}

// Use 注册全局中间件
func (e *Engine) Use(h ...HandlerFunc) {
	e.middleware = append(e.middleware, h...)
}

// GET 注册 GET 路由
func (e *Engine) GET(pattern string, handlers ...HandlerFunc) {
	e.add("GET", pattern, handlers...)
}

// POST 注册 POST 路由
func (e *Engine) POST(pattern string, handlers ...HandlerFunc) {
	e.add("POST", pattern, handlers...)
}

// DELETE 注册 DELETE 路由
func (e *Engine) DELETE(pattern string, handlers ...HandlerFunc) {
	e.add("DELETE", pattern, handlers...)
}

// PUT 注册 PUT 路由
func (e *Engine) PUT(pattern string, handlers ...HandlerFunc) {
	e.add("PUT", pattern, handlers...)
}

// PATCH 注册 PATCH 路由
func (e *Engine) PATCH(pattern string, handlers ...HandlerFunc) {
	e.add("PATCH", pattern, handlers...)
}

// OPTIONS 注册 OPTIONS 路由
func (e *Engine) OPTIONS(pattern string, handlers ...HandlerFunc) {
	e.add("OPTIONS", pattern, handlers...)
}

// HEAD 注册 HEAD 路由
func (e *Engine) HEAD(pattern string, handlers ...HandlerFunc) {
	e.add("HEAD", pattern, handlers...)
}

func (e *Engine) add(method, pattern string, handlers ...HandlerFunc) {
	segs := splitPath(pattern)
	r := &route{
		method:   method,
		pattern:  pattern,
		segments: segs,
		handlers: handlers,
	}
	e.mu.Lock()
	e.routes = append(e.routes, r)
	e.mu.Unlock()
}

// Group 返回一个路由组，复用公共前缀
func (e *Engine) Group(prefix string, middleware ...HandlerFunc) *RouterGroup {
	return &RouterGroup{engine: e, prefix: prefix, middleware: middleware}
}

// ServeHTTP 实现 http.Handler
func (e *Engine) ServeHTTP(w nethttp.ResponseWriter, r *nethttp.Request) {
	e.mu.RLock()
	routes := e.routes
	e.mu.RUnlock()

	// 包装 ResponseWriter 以追踪状态码和字节数
	rw := &responseWriter{ResponseWriter: w, status: nethttp.StatusOK}

	path := r.URL.Path
	method := r.Method

	methodMismatch := false
	var mismatchParams map[string]string
	var mismatchPattern string
	for _, route := range routes {
		params, ok := matchRoute(route.segments, path)
		if !ok {
			continue
		}
		if route.method != method {
			methodMismatch = true
			if mismatchPattern == "" {
				mismatchParams = params
				mismatchPattern = route.pattern
			}
			continue
		}
		// 构建 handler 链：全局中间件 + 路由 handlers
		chain := make([]HandlerFunc, 0, len(e.middleware)+len(route.handlers))
		chain = append(chain, e.middleware...)
		chain = append(chain, route.handlers...)

		c := &Context{
			Writer:   rw,
			Request:  r,
			params:   params,
			pattern:  route.pattern,
			index:    -1,
			handlers: chain,
		}

		// 追踪在途请求，用于优雅关闭时等待完成
		e.inflight.Add(1)
		defer e.inflight.Done()
		c.Next()
		return
	}

	if methodMismatch {
		if method == nethttp.MethodOptions {
			chain := make([]HandlerFunc, 0, len(e.middleware)+1)
			chain = append(chain, e.middleware...)
			chain = append(chain, func(c *Context) {
				c.Writer.WriteHeader(nethttp.StatusNoContent)
			})

			c := &Context{
				Writer:   rw,
				Request:  r,
				params:   mismatchParams,
				pattern:  mismatchPattern,
				index:    -1,
				handlers: chain,
			}
			e.inflight.Add(1)
			defer e.inflight.Done()
			c.Next()
			return
		}
		rw.Header().Set("Content-Type", "application/json; charset=utf-8")
		rw.WriteHeader(nethttp.StatusMethodNotAllowed)
		json.NewEncoder(rw).Encode(map[string]string{"detail": nethttp.StatusText(nethttp.StatusMethodNotAllowed)})
		return
	}
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(nethttp.StatusNotFound)
	json.NewEncoder(rw).Encode(map[string]string{"detail": nethttp.StatusText(nethttp.StatusNotFound)})
}

// NewServer 创建绑定到 addr 的 http.Server
func (e *Engine) NewServer(addr string) *nethttp.Server {
	return &nethttp.Server{Addr: addr, Handler: e}
}

// Start 在 addr 上监听并 serve（阻塞）。
// 不会处理系统信号——如需优雅关闭，使用 StartWithShutdown。
func (e *Engine) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := e.NewServer(addr)
	return srv.Serve(ln)
}

// Shutdown 优雅关闭服务器。
// 1. 调用 srv.Shutdown(ctx) 停止接受新连接
// 2. 等待所有在途请求处理完毕（由 inflight WaitGroup 追踪）
// ctx 用于设置关闭超时。
func (e *Engine) Shutdown(ctx context.Context) error {
	// 等待在途请求完成，或 ctx 超时
	done := make(chan struct{})
	go func() {
		e.inflight.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StartWithShutdown 启动服务并处理系统信号，实现优雅关闭。
//
// 流程：
//  1. 监听 addr 并启动 HTTP 服务（goroutine）
//  2. 等待 SIGTERM / SIGINT 信号
//  3. 收到信号后，创建 30s 超时 context，调用 Shutdown
//  4. 返回 Shutdown 的结果
//
// 这是生产推荐的启动方式。
func (e *Engine) StartWithShutdown(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	srv := e.NewServer(addr)

	// 启动 HTTP 服务
	go func() {
		if err := srv.Serve(ln); err != nil && err != nethttp.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "[bm] serve error: %v\n", err)
		}
	}()

	// 等待信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	fmt.Fprintf(os.Stderr, "[bm] received signal %v, shutting down...\n", sig)

	// 先关闭 HTTP 服务器（停止接受新连接）
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(os.Stderr, "[bm] HTTP server shutdown error: %v\n", err)
	}

	// 再等待在途请求完成
	if err := e.Shutdown(shutdownCtx); err != nil {
		return err
	}

	return nil
}

// ── RouterGroup ───────────────────────────────────────────────────────────

// RouterGroup 提供带公共前缀和中间件的路由分组
type RouterGroup struct {
	engine     *Engine
	prefix     string
	middleware []HandlerFunc
}

func (g *RouterGroup) GET(path string, handlers ...HandlerFunc) {
	g.engine.add("GET", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) POST(path string, handlers ...HandlerFunc) {
	g.engine.add("POST", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) DELETE(path string, handlers ...HandlerFunc) {
	g.engine.add("DELETE", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) PUT(path string, handlers ...HandlerFunc) {
	g.engine.add("PUT", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) PATCH(path string, handlers ...HandlerFunc) {
	g.engine.add("PATCH", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) OPTIONS(path string, handlers ...HandlerFunc) {
	g.engine.add("OPTIONS", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) HEAD(path string, handlers ...HandlerFunc) {
	g.engine.add("HEAD", g.prefix+path, g.wrap(handlers...)...)
}

func (g *RouterGroup) Group(prefix string, middleware ...HandlerFunc) *RouterGroup {
	return &RouterGroup{
		engine:     g.engine,
		prefix:     g.prefix + prefix,
		middleware: append(g.middleware, middleware...),
	}
}

func (g *RouterGroup) wrap(handlers ...HandlerFunc) []HandlerFunc {
	if len(g.middleware) == 0 {
		return handlers
	}
	chain := make([]HandlerFunc, 0, len(g.middleware)+len(handlers))
	chain = append(chain, g.middleware...)
	chain = append(chain, handlers...)
	return chain
}

// ── 路由匹配 ──────────────────────────────────────────────────────────────

func splitPath(path string) []string {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return []string{}
	}
	return strings.Split(path, "/")
}

// matchRoute 尝试把请求路径匹配到路由 segments，返回参数 map 和是否成功
func matchRoute(segments []string, path string) (map[string]string, bool) {
	reqSegs := splitPath(path)

	// 处理通配符段（*filepath 必须是最后一段）
	if len(segments) > 0 && strings.HasPrefix(segments[len(segments)-1], "*") {
		if len(reqSegs) < len(segments)-1 {
			return nil, false
		}
		params := make(map[string]string)
		for i, seg := range segments[:len(segments)-1] {
			if strings.HasPrefix(seg, ":") {
				params[seg[1:]] = reqSegs[i]
			} else if seg != reqSegs[i] {
				return nil, false
			}
		}
		key := segments[len(segments)-1][1:]
		params[key] = "/" + strings.Join(reqSegs[len(segments)-1:], "/")
		return params, true
	}

	if len(segments) != len(reqSegs) {
		return nil, false
	}

	params := make(map[string]string)
	for i, seg := range segments {
		if strings.HasPrefix(seg, ":") {
			params[seg[1:]] = reqSegs[i]
		} else if seg != reqSegs[i] {
			return nil, false
		}
	}
	return params, true
}

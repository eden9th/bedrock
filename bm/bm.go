// Package bm 提供轻量级 HTTP 框架
// 核心概念：
//   - Context：封装 http.ResponseWriter + *http.Request，提供 JSON/Abort/Param 等快捷方法
//   - Engine：路由注册 + 中间件链，支持路径参数（:name）和通配符（*filepath）
package bm

import (
	"context"
	"encoding/json"
	"net"
	nethttp "net/http"
	"strings"
	"sync"
)

// HandlerFunc 是 bm handler 函数签名，与 blademaster 一致
type HandlerFunc func(c *Context)

// Context 封装单次请求的上下文，贯穿 handler 链
type Context struct {
	Writer  nethttp.ResponseWriter
	Request *nethttp.Request

	// params 是从 URL 路径中提取的命名参数，如 /api/tokens/:token
	params map[string]string

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

// Set / Get 在 context 里存取任意值（通过 request context）
func (c *Context) Set(key string, val any) {
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), contextKey(key), val))
}

func (c *Context) Get(key string) (any, bool) {
	val := c.Request.Context().Value(contextKey(key))
	return val, val != nil
}

type contextKey string

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

	path := r.URL.Path
	method := r.Method

	methodMismatch := false
	for _, route := range routes {
		params, ok := matchRoute(route.segments, path)
		if !ok {
			continue
		}
		if route.method != method {
			methodMismatch = true
			continue
		}
		// 构建 handler 链：全局中间件 + 路由 handlers
		chain := make([]HandlerFunc, 0, len(e.middleware)+len(route.handlers))
		chain = append(chain, e.middleware...)
		chain = append(chain, route.handlers...)

		c := &Context{
			Writer:   w,
			Request:  r,
			params:   params,
			index:    -1,
			handlers: chain,
		}
		c.Next()
		return
	}

	if methodMismatch {
		nethttp.Error(w, "Method Not Allowed", nethttp.StatusMethodNotAllowed)
		return
	}
	nethttp.NotFound(w, r)
}

// NewServer 创建绑定到 addr 的 http.Server
func (e *Engine) NewServer(addr string) *nethttp.Server {
	return &nethttp.Server{Addr: addr, Handler: e}
}

// Start 在 addr 上监听并 serve（阻塞）
func (e *Engine) Start(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := e.NewServer(addr)
	return srv.Serve(ln)
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

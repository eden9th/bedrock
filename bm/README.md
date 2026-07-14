# bm — 轻量级 HTTP 框架

> 对标 blademaster，提供 Context、Engine、RouterGroup 和中间件链。支持路径参数（`:id`）、通配符（`*filepath`）、路由分组。

## 设计哲学

**足够用，不冗余。** bm 的目标是 500 行代码提供 80% 的 HTTP 路由需求。不追求 ORM 集成、模板渲染、参数绑定等重框架特性——这些由专用库或应用层负责。

核心关注点：
1. **路由匹配**：精确匹配 + 路径参数 + 通配符，O(n) 线性扫描（路由量少时优于 Radix Tree 开销）
2. **中间件链**：全局 + 分组 + 路由级，洋葱模型执行
3. **兼容性**：实现 `http.Handler`，可嵌入任何 Go HTTP 生态

## 核心概念

### 路由匹配

```
GET  /api/users/:id        → /api/users/123 → params: {id: "123"}
GET  /static/*filepath     → /static/js/app.js → params: {filepath: "/js/app.js"}
POST /api/tokens           → /api/tokens → params: {}
```

- `:name` — 单段路径参数
- `*name` — 通配符（必须为最后一段），匹配剩余全部路径
- 路由按注册顺序线性扫描，先注册先匹配

### 中间件链

```
Incoming Request
      ↓
global middleware 1 → global middleware 2 → route handler 3
      ↓                    ↓                     ↓
  c.Next() →          c.Next() →           business logic
      ↓                    ↓                     ↓
  (after Next)       (after Next)          (returns)
      ↓
Response
```

### Context

`bm.Context` 贯穿整个请求生命周期，提供：
- 路径参数读取：`Param("id")`
- Query/Form/Header 读取
- 响应输出：`JSON()`, `String()`, `Redirect()`
- 中间件控制：`Next()`, `Abort()`, `AbortWithStatus()`
- Handler 链传值：`Set()` / `Get()`
- 路由模板：`Pattern()`（v0.2.0）
- 响应追踪：`WriterStatus()`, `BytesWritten()`（v0.2.0）
- 内置中间件：`Recovery()` / `CORS()`（v0.3.0）/ `Logger()` / `Timeout()`（v0.5.0）
- pprof 端点：`RegisterPprof()`（v0.4.0）

## 快速开始

### 基础路由

```go
import "github.com/eden9th/bedrock/bm"

e := bm.New()
e.GET("/ping", func(c *bm.Context) {
    c.String(200, "pong")
})
e.GET("/api/users/:id", func(c *bm.Context) {
    id := c.Param("id")
    c.JSON(map[string]string{"id": id}, nil)
})
e.Start(":8080")
```

### 中间件

```go
// 全局中间件
e.Use(loggingMiddleware, authMiddleware)

// 分组中间件
api := e.Group("/api", rateLimitMiddleware)
api.GET("/users", listUsers)

// 路由级中间件
e.GET("/admin", adminAuthMiddleware, adminHandler)
```

### 响应格式

```go
// 标准 JSON 信封
c.JSON(data, nil)  // → {"code":0,"message":"ok","data":{...}}
c.JSON(nil, err)   // → {"code":-1,"message":"<err>","data":null}

// 纯文本
c.String(200, "hello")

// 错误终止
c.AbortWithStatus(401) // → {"detail":"Unauthorized"}
```

### 内置中间件（v0.3.0+）

```go
// panic recovery — 捕获 handler panic、dump 请求信息，返回 500 而非进程崩溃
e.Use(bm.Recovery())

// CORS 跨域 — 自动处理 OPTIONS 预检请求
e.Use(bm.CORS(bm.DefaultCORS()))      // 允许 localhost 开发
e.Use(bm.CORS(bm.CORSAllowAll()))      // 允许所有源（内部服务）

// access log — JSON 格式记录每个请求（v0.5.0+）
e.Use(bm.Logger())

// 请求超时 — 超时取消 context，返回 504（v0.5.0+）
e.Use(bm.Timeout(30 * time.Second))

// pprof — 注册标准 /debug/pprof 端点（v0.4.0+）
bm.RegisterPprof(e)
```

### 优雅关闭（v0.3.0+）

```go
// 生产推荐：自动处理 SIGTERM/SIGINT，drain 在途请求
e.StartWithShutdown(":8080")

// 或手动控制
go e.Start(":8080")
// ... 收到信号后：
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
e.Shutdown(ctx)
```

## API 参考

### Engine

```go
func New() *Engine
func (e *Engine) Use(h ...HandlerFunc)
func (e *Engine) GET/POST/DELETE/PUT/PATCH/OPTIONS/HEAD(pattern string, h ...HandlerFunc)
func (e *Engine) Group(prefix string, middleware ...HandlerFunc) *RouterGroup
func (e *Engine) Start(addr string) error
func (e *Engine) StartWithShutdown(addr string) error  // v0.3.0+
func (e *Engine) Shutdown(ctx context.Context) error    // v0.3.0+
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request)
```

### Context

```go
// 输入
func (c *Context) Param(key string) string
func (c *Context) Query(key string) string
func (c *Context) FormValue(key string) string
func (c *Context) GetHeader(key string) string

// 输出
func (c *Context) JSON(data any, err error)
func (c *Context) String(status int, body string)
func (c *Context) Redirect(url string)
func (c *Context) Header(key, value string)

// 控制
func (c *Context) Next()
func (c *Context) Abort()
func (c *Context) AbortWithStatus(status int)
func (c *Context) IsAborted() bool

// 传值
func (c *Context) Set(key string, val any)
func (c *Context) Get(key string) (any, bool)

// 响应追踪
func (c *Context) Pattern() string       // v0.2.0+
func (c *Context) WriterStatus() int     // v0.2.0+
func (c *Context) BytesWritten() int     // v0.2.0+
```

### 内置中间件

```go
func Recovery() HandlerFunc              // v0.3.0+ panic recovery
func CORS(cfg CORSConfig) HandlerFunc    // v0.3.0+ 跨域处理
func DefaultCORS() CORSConfig            // v0.3.0+ localhost 开发配置
func CORSAllowAll() CORSConfig           // v0.3.0+ 全允许配置
func Logger() HandlerFunc                // v0.5.0+ access log (JSON to stderr)
func Timeout(d time.Duration) HandlerFunc // v0.5.0+ 请求超时中间件
func RegisterPprof(e *Engine)            // v0.4.0+ /debug/pprof 端点注册
```

## 常见问题

### Q: 路由冲突怎么处理？

先注册先匹配。例如：

```go
e.GET("/api/users/me", handler1)
e.GET("/api/users/:id", handler2)
```

`/api/users/me` 会匹配 handler1（先注册），而 `/api/users/123` 匹配 handler2（`:id` 段）。

### Q: 性能怎么样？

路由匹配是 O(n) 线性扫描——对于 < 100 条路由的场景完全足够。如果需要数千条路由，建议引入 Radix Tree 路由器或换用更重的框架。bm 的定位就是中小型服务的路由层。

### Q: 如何获取客户端 IP？

```go
ip := c.Request.RemoteAddr
// 如果有反向代理，从 header 中取
ip = c.GetHeader("X-Forwarded-For")
// 或使用标准库
ip, _, _ = net.SplitHostPort(c.Request.RemoteAddr)
```

### Q: 支持 WebSocket 吗？

bm 不内置 WebSocket 支持。但 `c.Writer` 和 `c.Request` 是标准库类型，可以直接传给 gorilla/websocket 或 nhooyr.io/websocket 的 upgrade 函数。

## 依赖

纯标准库。

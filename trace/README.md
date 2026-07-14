# trace — 请求追踪

> 每个请求生成唯一 `trace_id`（UUID v4），存入 `context.Context`，通过 HTTP header `X-Trace-Id` 自动传播。日志包自动关联 `trace_id`，无需手动传递。

## 设计哲学

**一句话接入。** `e.Use(trace.Middleware())` 即可为所有请求注入分布式追踪能力。

**零配置。** 不需要采样率、不需要 collector endpoint、不需要 agent——trace_id 只做标识和传播，不做 span 收集。如需完整链路追踪，在应用层引入 OpenTelemetry。

## 核心概念

### trace_id 生命周期

```
客户端请求（带 X-Trace-Id header 或空）
       ↓
trace.Middleware()：读取或生成 trace_id
       ↓
注入 context.Context
       ↓
业务 handler：log.Info(ctx, ...)   → 自动带 trace_id
向下游请求：trace.InjectRequest()  → 自动带 X-Trace-Id header
       ↓
响应 header 中返回 X-Trace-Id
```

### 与 log 包的协作

trace 和 log 通过 `internal/ctxkey.TraceID` 共享 context key。只要在请求链中使用了 `trace.Middleware()`，后续 `log.Info(ctx, ...)` 自动带上 `trace_id` 字段，无需额外代码。

## 快速开始

```go
import "github.com/eden9th/bedrock/trace"

e := bm.New()
e.Use(trace.Middleware())  // 一行接入

// 后续所有 log 调用自动带上 trace_id
e.GET("/api/users/:id", func(c *bm.Context) {
    log.Info(c.Request.Context(), "fetching user, id=%s", c.Param("id"))
    // → {"trace_id":"uuid-here",...,"msg":"fetching user, id=123"}
})
```

### 向下游传播

```go
req, _ := http.NewRequest("GET", "https://api.downstream/data", nil)
trace.InjectRequest(ctx, req)
http.DefaultClient.Do(req) // 下游收到 X-Trace-Id header
```

### 手动控制

```go
// 生成
id := trace.NewTraceID()

// 注入 context
ctx = trace.WithTraceID(ctx, id)

// 读取
traceID := trace.GetTraceID(ctx)
```

## API 参考

```go
func Middleware() bm.HandlerFunc                         // bm 中间件
func NewTraceID() string                                // 生成 UUID v4
func WithTraceID(ctx context.Context, id string) context.Context // 注入 context
func GetTraceID(ctx context.Context) string              // 从 context 提取
func InjectRequest(ctx context.Context, req *http.Request)       // 注入 HTTP header

const HeaderName = "X-Trace-Id"
```

## 常见问题

### Q: trace_id 会跨服务传播吗？

会。客户端在请求中携带 `X-Trace-Id` header → 服务端 `trace.Middleware()` 读取 → 如果存在则沿用，不存在则生成新的。向下游请求时 `InjectRequest` 自动注入。这样同一个 trace_id 可以穿透整个调用链。

### Q: 性能开销大吗？

UUID v4 生成 + context 注入 + header 操作，单次请求 < 1μs。不做 span 收集，零外部上报。

### Q: 和 OpenTelemetry / Jaeger 的关系？

trace 包只做 trace_id 生成和传播，不做 span 收集。如果需要完整的分布式追踪（span tree、采样、上报），引入 OpenTelemetry SDK。trace 包的 `X-Trace-Id` header 与 OpenTelemetry 的 `traceparent` 是互补关系，可以共存。

### Q: 如何按 trace_id 查询日志？

```bash
# jq 过滤
jq 'select(.trace_id == "uuid-here")' app.log

# grep 简单过滤
grep '"trace_id":"uuid-here"' app.log
```

## 依赖

- `github.com/google/uuid` — UUID v4 生成
- `bedrock/bm` — HTTP 中间件框架

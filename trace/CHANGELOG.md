# CHANGELOG — trace

---

## [0.1.0] — 2026-07-12

### Added

- Middleware() — bm 中间件，自动读取/生成/传播 trace_id
- NewTraceID() — 生成 UUID v4
- WithTraceID(ctx, id) — 将 trace_id 注入 context
- GetTraceID(ctx) — 从 context 提取 trace_id
- InjectRequest(ctx, req) — 向外发请求时注入 X-Trace-Id header
- HeaderName 常量 = "X-Trace-Id"
- 与 log 包通过 internal/ctxkey 共享 context key，日志自动关联 trace_id
- trace_test.go 功能测试

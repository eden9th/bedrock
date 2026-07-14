# CHANGELOG — bm

---

## [0.4.0] — 2026-07-14

### Added

- RegisterPprof(e) — 内置 /debug/pprof 路由组注册

### Changed

- Recovery() — panic 日志增加 HTTP 请求 dump（method/path/headers）

---

## [0.3.0] — 2026-07-14

### Added

- Recovery() — panic recovery 中间件，捕获 panic 并返回 500
- CORS(cfg) — 跨域资源共享中间件，处理 OPTIONS 预检请求
- DefaultCORS() / CORSAllowAll() — CORS 预设配置
- Engine.Shutdown(ctx) — 优雅关闭，等待在途请求完成
- Engine.StartWithShutdown(addr) — 启动 + SIGTERM/SIGINT 信号处理 + 优雅关闭
- Engine.inflight sync.WaitGroup — 在途请求追踪

---

## [0.2.0] — 2026-07-14

### Added

- Context.Pattern() — 返回匹配的路由 template pattern
- Context.WriterStatus() — 返回实际写入的 HTTP 状态码
- Context.BytesWritten() — 返回实际写入的响应字节数
- 内部 responseWriter 包装器 — 透明追踪 status 和 bytes，实现 `http.ResponseWriter`

### Changed

- ServeHTTP 中创建 Context 时传入 route.pattern
- ServeHTTP 使用 responseWriter 包装原始 ResponseWriter

---

## [0.1.0] — 2026-07-12

### Added

- Engine（路由引擎）：Use / GET / POST / DELETE / PUT / Group / Start / ServeHTTP
- RouterGroup（路由分组）：GET / POST / DELETE / PUT / Group
- Context（请求上下文）：Param / Query / FormValue / GetHeader / JSON / String / Redirect / Abort / AbortWithStatus / Next / Set / Get
- 路径参数匹配（`:name`）
- 通配符匹配（`*filepath`）
- 全局 + 分组 + 路由级中间件链
- bm_test.go 完整功能测试

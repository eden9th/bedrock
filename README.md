# bedrock

Go 基础工具库，供个人项目共享使用。提取自 demo/invest/hearth 框架的公共 `util` 层，统一维护，各项目通过 go module 引用。

## 安装

```bash
go get github.com/eden9th/bedrock@latest
```

## 包列表

### `bm` — 轻量级 HTTP 框架

对标 bilibili blademaster，提供 `Context`、`Engine`、`RouterGroup`，支持路径参数（`:id`）和通配符（`*filepath`），以及全局/分组中间件链。

```go
e := bm.New()
e.Use(trace.Middleware())
e.GET("/ping", func(c *bm.Context) { c.String(200, "pong") })
e.Start(":8080")
```

### `conf` — TOML 配置 + 热更新

从指定目录加载 TOML 配置文件，基于 `fsnotify` 监听文件变更，触发 `Setter` 回调实现热更新。

```go
conf.Init("configs/")
var cfg AppConfig
conf.Get("application.toml").UnmarshalTOML(&cfg)
```

### `duration` — TOML 可反序列化的 Duration

在 TOML 配置里直接写 `"1h30m"`，自动解析为 `time.Duration`。

```toml
timeout = "30s"
```

```go
type Config struct {
    Timeout duration.Duration `toml:"timeout"`
}
// cfg.Timeout.Duration() → time.Duration
```

### `httputil` — Handler 工具函数

在 `bm.Context` 上封装常用响应和请求体解析，输出裸 JSON（不带信封）。

```go
httputil.JSON(c, data)                        // 200 + JSON
httputil.JSONError(c, http.StatusBadRequest, "invalid id")  // 错误响应 + Abort
var req CreateReq
if !httputil.Bind(c, &req) { return }         // 解析失败自动 400
```

### `log` — 结构化 JSON 日志

输出 JSON 行，字段包含 `time / level / trace_id / caller / msg`，默认写 stderr，可选同时写文件（lumberjack 按大小轮转）。

```go
log.Init(&log.Config{
    FilePath:   "logs/app.2026-07-12.log",  // 空字符串仅输出到 stderr
    MaxSizeMB:  200,
    MaxBackups: 30,
    MaxAgeDays: 30,
})

log.Info(ctx, "server started on %s", addr)
log.Error(ctx, "query failed, id=%d err=%+v", id, err)
```

### `trace` — 请求追踪

每个请求生成唯一 `trace_id`，存入 `context.Context`，通过 HTTP header `X-Trace-Id` 传播，日志自动附带。

```go
e.Use(trace.Middleware())  // 注册为 bm 中间件

// 向下游传播
trace.InjectRequest(ctx, req)
```

## 项目结构

```
bedrock/
├── bm/          轻量级 HTTP 框架
├── conf/        TOML 配置加载 + 热更新
├── duration/    可序列化 Duration 类型
├── httputil/    Handler 工具函数
├── log/         结构化 JSON 日志
└── trace/       请求追踪
```

## 版本

遵循语义化版本（semver）。breaking change 升 minor 或 major，向后兼容的修复只升 patch。

## 依赖

| 包 | 用途 |
|---|---|
| `github.com/BurntSushi/toml` | TOML 解析 |
| `github.com/fsnotify/fsnotify` | 配置文件热更新 |
| `github.com/google/uuid` | trace_id 生成 |
| `gopkg.in/natefinch/lumberjack.v2` | 日志文件轮转 |

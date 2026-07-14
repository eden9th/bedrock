# bedrock

> Go 基础工具库，为个人项目提供共享的基础组件。提取自 demo / invest / hearth 框架的公共层，统一维护，各项目通过 go module 引用。

## 设计原则

| 原则 | 含义 |
|------|------|
| **小即美** | 每个包聚焦一件事，源码控制在 500 行以内（核心逻辑） |
| **零/低外部依赖** | 尽最大可能使用标准库。外部依赖必须有充分理由 |
| **显式优于隐式** | 接口简洁，行为可预测，魔法的部分在文档中写清楚 |
| **开箱即用** | `Init(...)` 然后用，不需要理解内部实现 |
| **可组合** | 每个包独立可用，不强绑定其他 bedrock 包 |

## 包概览

| 包 | 定位 | 核心能力 | 外部依赖 | 文档 |
|---|---|---|---|---|
| **[bm](bm/)** | HTTP 框架 | 路由、Recovery、CORS、Logger、Timeout、pprof、优雅关闭 | 0 | [README](bm/README.md) |
| **[circuitbreaker](circuitbreaker/)** | 熔断器 | 三态机（closed/open/half-open）、滑动窗口、自动恢复 | 0 | — |
| **[conf](conf/)** | 配置管理 | TOML 加载、fsnotify 热更新、Setter 回调、环境变量覆盖、Validator 校验 | 2 (toml, fsnotify) | [README](conf/README.md) |
| **[cron](cron/)** | 定时任务 | Job 接口、Scheduler（wrap/pause/resume）、RunTask 手动触发、lifecycle.Service | 1 (robfig/cron) | — |
| **[db](db/)** | 数据库助手 | 结构体扫描、事务 helper、连接池配置、连接池指标自动注册 | 1 (monitor) | [README](db/README.md) |
| **[duration](duration/)** | 配置 duration | TOML → time.Duration | 0 | [README](duration/README.md) |
| **[errors](errors/)** | 结构化错误 | AppError、CodeError/InvalidArgument/NotFound/Forbidden/Internal、HTTPStatus()、Wrap/Is/As | 0 | — |
| **[feature](feature/)** | 功能开关 | 布尔开关、百分比灰度、TOML/Map 双后端 | 1 (conf) | — |
| **[httputil](httputil/)** | Handler 工具 | JSON 响应、请求体解析（含大小限制）、结构化校验、BindValidate[T]（validator） | 1 (validator) | [README](httputil/README.md) |
| **[lifecycle](lifecycle/)** | 生命周期 | 有序启停、信号驱动优雅关闭、Service 接口 | 0 | — |
| **[log](log/)** | 结构化日志 | JSON 行输出、trace_id 关联、类型化 KV 字段、日志采样 | 1 (lumberjack) | [README](log/README.md) |
| **[migrate](migrate/)** | 数据库迁移 | 按序号 SQL 文件执行、版本记录、PostgreSQL/MySQL/SQLite 占位符自动适配 | 1 (database/sql) | — |
| **[monitor](monitor/)** | 监控原语 | Counter/Gauge/Histogram、DefaultLabels、慢请求检测 | 0 | [README](monitor/README.md) |
| **[pager](pager/)** | 内存分页 | 越界安全边界计算（尾页触底规范） | 0 | [README](pager/README.md) |
| **[ratelimit](ratelimit/)** | 令牌桶限流 | Allow/Wait、KeyLimiter 多键分桶、TTL 清理、context 感知 | 0 | [README](ratelimit/README.md) |
| **[retry](retry/)** | 指数退避重试 | 退避 + 抖动、最大次数、context 感知 | 0 | [README](retry/README.md) |
| **[trace](trace/)** | 请求追踪 | trace_id 生成/传播、Span（操作级耗时+属性）、log 自动关联 | 1 (uuid) | [README](trace/README.md) |
| **[xslices](xslices/)** | 泛型切片工具 | Map/Filter/Reduce/RemoveDuplicate/Splits/IfIntersect/Reverse | 0 | [README](xslices/README.md) |
| **[xstrings](xstrings/)** | 字符串工具 | Join/Split/GenSQLPlaceholder/IsEmpty/IsBlank/Truncate/PadLeft/PadRight | 0 | [README](xstrings/README.md) |

## 安装

```bash
go get github.com/eden9th/bedrock@latest
```

## 五分钟启动

```go
package main

import (
    "github.com/eden9th/bedrock/bm"
    "github.com/eden9th/bedrock/conf"
    "github.com/eden9th/bedrock/httputil"
    "github.com/eden9th/bedrock/log"
    "github.com/eden9th/bedrock/monitor"
    "github.com/eden9th/bedrock/trace"
)

func main() {
    // 1. 配置（支持环境变量覆盖）
    conf.Init("configs/", conf.WithEnvPrefix("APP_"))

    // 2. 日志（支持采样）
    log.Init(&log.Config{
        FilePath: "logs/app.2026-07-14.log",
        Level:    log.LevelInfo,
        Sampling: &log.SamplingConfig{Window: time.Second, MaxCount: 10},
    })

    // 3. HTTP 框架 + 基础中间件
    e := bm.New()
    e.Use(bm.Recovery())           // panic 保护 + 请求 dump
    e.Use(bm.Logger())             // access log（JSON 格式）
    e.Use(bm.Timeout(30*time.Second)) // 请求超时保护
    e.Use(trace.Middleware())       // 请求追踪

    // 4. 监控（可选）
    r := monitor.NewRegistry()
    hm := monitor.NewHTTPMetrics(monitor.DefaultHTTPBuckets)
    hm.Register(r)
    e.Use(hm.Middleware())
    e.GET("/metrics", monitor.Handler(r))

    // 5. 健康检查（可选）
    hr := monitor.NewHealthRegistry()
    e.GET("/health", monitor.HealthHandler(hr))

    // 6. 业务路由
    e.GET("/api/ping", func(c *bm.Context) {
        httputil.JSON(c, map[string]string{"status": "ok"})
    })

    log.Info(nil, "server starting on :8080")

    // 7. 启动（生产推荐：支持 SIGTERM 优雅关闭）
    if err := e.StartWithShutdown(":8080"); err != nil {
        log.Error(nil, "shutdown: %v", err)
    }
}
```

## 架构关系

```
                         ┌─────────┐
                         │  conf   │ ← TOML 配置 + 热更新 + Env 覆盖 + Validator
                         └────┬────┘
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
    ┌────▼───┐          ┌────▼──────┐       ┌─────▼──────┐
    │  log   │          │    bm     │       │  monitor   │
    │ JSON   │          │ HTTP 框架 │       │ 监控原语   │
    │ 日志   │          │ +Recovery │       │ +Health    │
    │+Sampling│         │ +CORS     │       │ +Middleware│
    └───┬────┘          │ +Logger   │       └─────┬──────┘
        │               │ +Timeout  │             │
        │               │ +Shutdown │             │
        │               └─────┬─────┘             │
        │                     │                   │
        └──────────┬──────────┤                   │
                   │          │                   │
              ┌────▼────┐     │              ┌────▼─────┐
              │  trace  │     │              │ ratelimit │
              │ 追踪    │     │              │ 令牌桶    │
              │ +Span   │     │              │+KeyLimiter│
              └─────────┘     │              └──────────┘
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
    ┌────▼──────┐       ┌────▼──────┐       ┌─────▼──────┐
    │ httputil  │       │   retry   │       │     db     │
    │ JSON/Bind │       │ 退避重试  │       │ 查询助手   │
    │+Validate  │       │           │       │+PoolConfig │
    └───────────┘       └───────────┘       └────────────┘

    ┌───────────┐  ┌──────────────┐  ┌──────────────┐
    │ duration  │  │circuitbreaker│  │  lifecycle   │
    │ TOML时间  │  │ 三态熔断器   │  │ 有序启停     │
    └───────────┘  └──────────────┘  └──────────────┘

    ┌───────────┐  ┌──────────────┐  ┌──────────────┐
    │  migrate  │  │   feature    │  │  xslices +   │
    │ SQL 迁移  │  │  功能开关    │  │  xstrings    │
    └───────────┘  └──────────────┘  └──────────────┘
```

**关键协作路径**：

1. `trace` → `log`：共享 trace_id，日志自动关联
2. `monitor` → `bm`：HTTP 中间件（Pattern + WriterStatus + BytesWritten）
3. `db` → `monitor`：HealthChecker 接口（连接池健康检查直接接入）
4. `conf` → `duration`：TOML duration 字段自动解析
5. `retry` + `circuitbreaker`：退避重试 + 熔断保护组合使用
6. `lifecycle` → `bm` + `db`：统一管理所有组件的启停

## 版本策略

遵循语义化版本（[SemVer](https://semver.org/lang/zh-CN/)）。

- **Major (X.0.0)**：拆除/重命名公开 API、移除包、行为大幅变更
- **Minor (0.X.0)**：新增包、新增公开 API、非破坏性功能扩展
- **Patch (0.0.X)**：Bug 修复、性能优化、文档更新

当前版本：**0.5.0**

## 变更记录

见 [CHANGELOG.md](CHANGELOG.md)。

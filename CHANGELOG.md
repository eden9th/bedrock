# CHANGELOG — bedrock

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [0.6.0] — 2026-07-14

### Added

- **errors** — 全新包：结构化业务错误（AppError），内置 CodeError / InvalidArgument / NotFound / Forbidden / Internal，HTTPStatus() 自动映射状态码，Wrap/Is/As 兼容标准库，零外部依赖
- **cron** — 全新包：定时任务调度器，封装 robfig/cron/v3；Job 接口 + FuncJob；wrap() 统一注入 panic recover / PAUSED 开关 / 耗时+错误日志；Pause/Resume/RunTask/Entries；实现 lifecycle.Service 可直接接入 lifecycle.Manager
- **httputil**: BindValidate[T]() — 基于 go-playground/validator/v10 的泛型 Bind+校验，失败自动返回 400/422 AppError

### Fixed

- **migrate**: executeMigration INSERT 占位符适配 PostgreSQL（`$1/$2`），其他驱动（MySQL/SQLite）继续使用 `?`，通过 driver 类型名反射自动检测

### Dependencies

- 新增 `github.com/go-playground/validator/v10 v10.22.1`
- 新增 `github.com/robfig/cron/v3 v3.0.1`

---

## [0.5.0] — 2026-07-14

### Added

- **circuitbreaker** — 全新包：三态熔断器（closed/open/half-open），滑动窗口计数，零外部依赖
- **lifecycle** — 全新包：Service 接口 + Manager，有序启停，信号驱动优雅关闭
- **migrate** — 全新包：按序号 SQL 文件执行迁移，自动版本记录
- **feature** — 全新包：功能开关（布尔/百分比灰度），TOML/Map 双后端
- **bm**: OPTIONS/PATCH/HEAD 方法注册（Engine + RouterGroup 完整 REST 动词）
- **bm**: Logger() — access log 中间件（JSON 格式输出到 stderr）
- **bm**: Timeout(d) — 请求超时中间件（context 取消 + 504 响应）
- **bm**: ServeHTTP 404/405 统一返回 JSON（`{"detail":"..."}`)，不再输出纯文本
- **conf**: WithEnvPrefix("APP_") — 环境变量覆盖 TOML 配置值
- **conf**: Validator 接口 — Watch 回调前校验配置有效性，失败跳过 Set
- **db**: PoolConfig + ConfigurePool() — 连接池参数一键配置
- **httputil**: BindWithLimit() — 可指定最大请求体大小（默认 1MB），防 OOM
- **httputil**: BindAndValidate() + FieldError — 结构化校验 + 422 响应
- **log**: SamplingConfig — 日志采样（相同消息窗口内最大 N 次），防日志风暴
- **ratelimit**: KeyLimiter — 按 key 分桶限流，内置 TTL 自动清理
- **trace**: Span — 操作级追踪（StartSpan/End/SetAttribute/AddEvent），日志关联
- **xslices**: Reduce() — 泛型归约函数
- **xstrings**: IsEmpty/IsBlank/Truncate/PadLeft/PadRight — 字符串常用工具

### Changed

- **conf**: Init 签名改为可变参数 `Init(dir string, opts ...Option)`，旧调用方式兼容

---

## [0.4.0] — 2026-07-14

### Added

- **xslices** — 全新包：泛型切片工具（Map/Filter/RemoveDuplicate/Splits/IfIntersect/Reverse）
- **xstrings** — 全新包：泛型字符串-切片互转（Join/Split/GenSQLPlaceholder）
- **pager** — 全新包：内存分页边界计算（越界安全返回）
- **bm**: RegisterPprof(e) — 内置 /debug/pprof 路由组
- **monitor**: Counter/Gauge/Histogram 支持 DefaultLabels 预设 label 值
- **monitor**: HTTPMetrics.SlowThreshold — 慢请求自动 WARN 到 stderr
- **db**: HealthChecker.RegisterPoolMetrics / CollectPoolMetrics — 连接池指标自动注册
- **log**: Field 类型化 KV 字段（KVString/KVInt64/KVFloat64/KVBool/KVDuration）
- **log**: Infov/Warnv/Errorv — 结构化 KV 日志函数

### Changed

- **bm**: Recovery() panic 日志增加 HTTP 请求 dump（method/path/headers，对标 blademaster）
- **bm**: Engine.ServeHTTP 使用 64KB 堆栈缓冲区（原来 4KB）

### Fixed

- **xstrings**: 使用自定义 `Integer` 接口替代已移除的 `golang.org/x/exp/constraints`

---

## [0.3.0] — 2026-07-14

### Added

- **bm**: Recovery() panic recovery 中间件
- **bm**: CORS(cfg) 跨域中间件，含 DefaultCORS / CORSAllowAll 预设
- **bm**: Engine.Shutdown(ctx) 优雅关闭 + Engine.StartWithShutdown(addr) 信号处理
- **retry** — 全新包：指数退避 + 随机抖动 + context 感知重试
- **ratelimit** — 全新包：令牌桶限流器，Allow + Wait
- **db** — 全新包：结构体扫描查询助手 + 事务 helper + 连接池健康检查
- **conf**: SetterFunc 类型（func → Setter 适配器）

### Changed

- **bm**: Engine 内部新增 inflight WaitGroup 追踪在途请求
- **conf**: README 重写，修正 API 文档（OnChange→Watch, File→Value）

### Fixed

- **monitor**: GC Counter 数值错误修复（NumGC 累计绝对值 → delta 增量）
- **monitor**: sortedLabelKeys O(n²) 冒泡排序 → sort.Strings

---

## [0.2.0] — 2026-07-14

### Added

- **monitor** — 进程内监控原语（全新包）
  - Counter / Gauge / Histogram 三种指标类型，支持 label 维度
  - Registry 注册中心，提供 Snapshot 快照导出
  - HTTP 暴露端点：Prometheus text format + JSON format
  - HTTP 请求指标中间件（`HTTPMetrics.Middleware()`）
  - 系统探针：Go 运行时指标（RuntimeStats）+ 磁盘使用量（DiskStats）
  - 健康检查模式：HealthChecker 接口 + 并发执行 HealthRegistry
  - 零外部依赖（核心），纯标准库

### Changed

- **bm** — HTTP 框架
  - Context 新增 `Pattern()`：返回匹配的路由 template pattern
  - Context 新增 `WriterStatus()`：返回实际写入的 HTTP 状态码
  - Context 新增 `BytesWritten()`：返回实际写入的响应字节数
  - 内部新增 `responseWriter` 包装器：透明追踪 status 和 bytes
  - ServeHTTP 使用 `responseWriter` 包装原始 `ResponseWriter`

### Documentation

- 新增所有组件的 README.md + CHANGELOG.md
  - bm, conf, duration, httputil, log, monitor, trace
- 重写项目级 README.md：宏观架构介绍 + 五分钟启动
- 新增项目级 CHANGELOG.md（本文件）

---

## [0.1.0] — 2026-07-12

### Added

- **bm** — 轻量级 HTTP 框架
  - Engine + RouterGroup + Context
  - 路径参数（`:id`）+ 通配符（`*filepath`）
  - 全局/分组/路由级中间件链
  - 标准 JSON 信封响应

- **conf** — TOML 配置 + 热更新
  - 目录级 TOML 文件加载
  - fsnotify 文件变更监听
  - Setter 回调热更新

- **duration** — TOML 可反序列化的 Duration 类型
  - `"1h30m"` 自动解析为 `time.Duration`

- **httputil** — Handler 工具函数
  - 裸 JSON 输出
  - JSON 错误响应
  - 请求体 Bind

- **log** — 结构化 JSON 日志
  - JSON 行格式
  - trace_id 自动关联
  - stderr + 文件双输出（lumberjack 分片）

- **trace** — 请求追踪
  - uuid v4 trace_id
  - X-Trace-Id header 传播
  - 与 log 包 context key 共享

- 项目初始化，go module: `github.com/eden9th/bedrock`

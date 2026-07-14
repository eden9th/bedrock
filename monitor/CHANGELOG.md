# CHANGELOG — monitor

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

---

## [0.2.0] — 2026-07-14

### Added

- Counter/Gauge/Histogram 支持 DefaultLabels 预设 label 值
- HTTPMetrics.SlowThreshold — 慢请求自动 WARN 到 stderr

---

## [0.1.1] — 2026-07-14

### Fixed

- **GC Counter 数值错误修复**：`RuntimeStats.counterGCCount` 之前直接使用 `runtime.ReadMemStats.NumGC`（累计绝对值）作为 `Counter.Add` 的增量，导致值随采集次数平方增长。修复方案：维护 `lastNumGC` 字段，每次 Collect 计算 delta 再 Add。
- `sortedLabelKeys` 将 O(n²) 冒泡排序替换为 `sort.Strings`，代码更 idiomatic

---

## [0.1.0] — 2026-07-14

### Added

- **metric.go**: Counter / Gauge / Histogram 三种指标类型，支持 label 维度
  - Counter：只增不减，uint64 存储，溢出保护
  - Gauge：可增可减，float64 存储
  - Histogram：分桶分布，预定义 bucket 边界，自动追加 +Inf bucket
  - 全部并发安全（sync.RWMutex）
  - label 编码使用 `\x00` 分隔符，避免与用户数据碰撞
  - Snapshot 输出稳定排序

- **registry.go**: 指标注册中心
  - Register / MustRegister / Unregister
  - Snapshot() 全量快照导出
  - 全局默认 Registry（RegisterGlobal / DefaultRegistry）

- **handler.go**: HTTP 暴露端点
  - Prometheus text exposition format（默认）
  - JSON format（`?format=json`）
  - bm HandlerFunc 和标准库 http.HandlerFunc 双版本

- **system.go**: 系统探针
  - RuntimeStats：Go 运行时指标（heap/stack/GC/goroutines），纯 stdlib
  - DiskStats：磁盘使用量，基于 syscall.Statfs，Unix 可用
  - 明确标注 OS 级 CPU/内存指标的缺失和替代方案

- **middleware.go**: HTTP 请求指标中间件
  - 自动记录请求计数、延迟分布、并发量
  - 使用 bm route pattern 避免 path label 高基数
  - 可配置 bucket、状态码分组、响应大小记录
  - 提供 DefaultHTTPBuckets 和 DefaultResponseSizeBuckets

- **health.go**: 健康检查模式
  - HealthChecker 接口：Name() + Check(ctx)
  - HealthRegistry：并发执行所有检查，独立超时
  - HealthHandler / HealthHandlerFunc：HTTP 暴露

### Changed (bedrock/bm)

- Context 新增 `Pattern()` 方法：返回匹配的路由 template
- Context 新增 `WriterStatus()` 方法：返回实际写入的 HTTP 状态码
- Context 新增 `BytesWritten()` 方法：返回实际写入的响应字节数
- 内部新增 `responseWriter` 包装器：追踪 status + bytes

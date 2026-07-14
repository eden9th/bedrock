# monitor — 进程内监控原语

> 提供 Counter / Gauge / Histogram 指标类型、注册中心、HTTP 暴露端点和健康检查模式。轻量、零外部依赖（核心）、可组合。

## 设计哲学

**提供原语，而非应用。** monitor 不是"监控子系统"，而是构建监控能力的乐高积木。具体项目的业务指标、定时调度、数据持久化、Dashboard 展示由消费方自行组装。

| 本包提供 | 本包不提供（由消费方决定） |
|----------|---------------------------|
| Counter / Gauge / Histogram 类型 | PostgreSQL / 内存存储方案 |
| 注册中心 + HTTP 暴露 | 定时调度器（`time.Ticker` 就够） |
| Go 运行时 + 磁盘探针 | Dashboard HTML |
| 健康检查接口 + 并发执行 | 具体服务的健康检查实现 |
| HTTP 请求指标中间件 | 业务级指标（SSE 连接、用户活跃度等） |

**对标关系**：定位在 `expvar` 和 `prometheus/client_golang` 之间——比 expvar 结构化（支持 label 维度 + Histogram），比 prometheus client 轻量（0 外部依赖，~35KB 源码）。

## 核心概念

### 三种指标类型

```
Counter   → 只增不减（请求总数、错误计数）
Gauge     → 可增可减（CPU 使用率、当前连接数）
Histogram → 分桶分布（请求延迟、响应大小）
```

每种指标都附带：
- **name**：唯一标识，如 `http_requests_total`
- **help**：人类可读的描述文本
- **labels**：可选的维度列表，如 `["method", "status"]`

### 数据流

```
业务代码 → Counter.Inc() / Gauge.Set() / Histogram.Observe()
                  ↓
           Registry（内存中）
                  ↓
        HTTP GET /metrics → Prometheus text 或 JSON
                  ↓
         外部 scraper / Grafana / 自定义消费方
```

指标永远只存内存中。持久化、聚合、告警等由外部消费方负责。

## 快速开始

### 最小示例

```go
import "github.com/eden9th/bedrock/monitor"

// 1. 创建指标
reqCount := monitor.NewCounter(
    "http_requests_total",
    "Total number of HTTP requests",
    []string{"method", "status"},
)
reqLatency := monitor.NewHistogram(
    "http_request_duration_ms",
    "Request latency in milliseconds",
    []float64{1, 5, 10, 50, 100, 500, 1000},
    []string{"method"},
)

// 2. 注册到 Registry
r := monitor.NewRegistry()
r.MustRegister(reqCount, reqLatency)

// 3. 在 handler 中使用
func myHandler(c *bm.Context) {
    start := time.Now()
    // ... 业务逻辑 ...
    reqCount.Inc("GET", "200")
    reqLatency.Observe(float64(time.Since(start).Milliseconds()), "GET")
}

// 4. 暴露 HTTP 端点
e.GET("/metrics", monitor.Handler(r))
```

### 使用 HTTP 中间件（推荐）

```go
r := monitor.NewRegistry()
hm := monitor.NewHTTPMetrics(monitor.DefaultHTTPBuckets)
hm.Register(r)
e.Use(hm.Middleware())          // 自动记录每个请求
e.GET("/metrics", monitor.Handler(r))
```

中间件自动生成 3 个指标：
| 指标 | 类型 | 说明 |
|------|------|------|
| `http_requests_total{method,path,status}` | Counter | 请求总数 |
| `http_request_duration_ms{method,path,status}` | Histogram | 请求延迟分布 |
| `http_requests_in_flight` | Gauge | 当前并发请求数 |

可选启用 `RecordResponseSize` 额外记录响应大小分布。

### 健康检查

```go
// 实现 HealthChecker 接口
type PostgresChecker struct{ db *sql.DB }
func (c *PostgresChecker) Name() string          { return "postgres" }
func (c *PostgresChecker) Check(ctx context.Context) error { return c.db.PingContext(ctx) }

// 注册并暴露
hr := monitor.NewHealthRegistry()
hr.Register(&PostgresChecker{db})
e.GET("/health", monitor.HealthHandler(hr))
// 健康 → 200 {"healthy":true,"checks":{...}}
// 不健康 → 503 {"healthy":false,"checks":{...}}
```

`CheckAll()` 并发执行所有检查，每个检查独立 2s 超时。

### 系统指标

```go
// Go 运行时指标（heap, stack, GC, goroutines）
rs := monitor.NewRuntimeStats()
rs.Register(r)

// 磁盘使用量
ds := monitor.NewDiskStats("/")
ds.Register(r)

// 定时采集
go func() {
    ticker := time.NewTicker(10 * time.Second)
    for range ticker.C {
        rs.Collect()
        ds.Collect()
    }
}()
```

**注意**：OS 级 CPU 使用率和物理内存使用率不在本包范围内。需要这些指标时，在消费方引入 `github.com/shirou/gopsutil/v3`：

```go
import "github.com/shirou/gopsutil/v3/cpu"
cpuPercent, _ := cpu.Percent(0, false)
cpuGauge.Set(cpuPercent[0])
```

## API 参考

### 指标类型

```go
// Counter
func NewCounter(name, help string, labels []string) *Counter
func (c *Counter) Inc(values ...string)
func (c *Counter) Add(delta float64, values ...string)
func (c *Counter) Value(values ...string) uint64

// Gauge
func NewGauge(name, help string, labels []string) *Gauge
func (g *Gauge) Set(val float64, values ...string)
func (g *Gauge) Add(delta float64, values ...string)
func (g *Gauge) Value(values ...string) float64

// Histogram
func NewHistogram(name, help string, buckets []float64, labels []string) *Histogram
func (h *Histogram) Observe(val float64, values ...string)
```

所有方法并发安全。label values 数量不匹配直接 panic。

### Registry

```go
func NewRegistry() *Registry
func (r *Registry) Register(metrics ...Metric) error    // 重复 name 返回 error
func (r *Registry) MustRegister(metrics ...Metric)       // 重复 name 时 panic
func (r *Registry) Unregister(name string)
func (r *Registry) Snapshot() []MetricSnapshot           // 全部指标当前快照
```

### HTTP 暴露

```go
func Handler(reg *Registry) bm.HandlerFunc              // bm 框架
func HandlerFunc(reg *Registry) http.HandlerFunc         // 标准库
```

输出格式：默认 Prometheus text（`?format=prometheus`），可选 JSON（`?format=json`）。

### 系统探针

```go
func NewRuntimeStats() *RuntimeStats
func (rs *RuntimeStats) Register(r *Registry)
func (rs *RuntimeStats) Collect()

func NewDiskStats(path string) *DiskStats
func (ds *DiskStats) Register(r *Registry)
func (ds *DiskStats) Collect()
```

### 健康检查

```go
type HealthChecker interface { Name() string; Check(ctx context.Context) error }
func NewHealthRegistry() *HealthRegistry
func (hr *HealthRegistry) Register(c HealthChecker) error
func (hr *HealthRegistry) CheckAll(ctx context.Context) HealthResult
func HealthHandler(hr *HealthRegistry) bm.HandlerFunc
func HealthHandlerFunc(hr *HealthRegistry) http.HandlerFunc
```

## 常见问题

### Q: label 基数爆炸怎么办？

label 的每个唯一组合都会在内存中创建独立的计数/值存储。如果 label 值空间很大（如把 `user_id` 作为 label），会导致内存泄漏。

**规则：label 只能取有限枚举值。** 方法（GET/POST）、状态码（2xx/5xx）、路由 pattern 都是安全的。user_id、request_id、timestamp 是危险的。

如果确实需要按用户维度统计，在应用层做预聚合，只暴露聚合后的指标。

### Q: 重启后指标丢失？

是的。指标只存内存，重启归零。这符合 Prometheus 模型（指标是运行态数据，不是业务数据）。如果需要长期存储，用 Prometheus server 或 Grafana Agent scrape `/metrics` 端点。

### Q: Prometheus 格式兼容性？

输出的 text format 遵循 [Prometheus Exposition Format](https://prometheus.io/docs/instrumenting/exposition_formats/) 0.0.4 规范。Counter/Gauge/Histogram 的 HELP、TYPE、数据行完全兼容，可直接被 Prometheus server、Grafana Agent、VictoriaMetrics 等 scrape。

### Q: 不想要 Prometheus 格式？

添加 `?format=json` query param，或直接调用 `registry.Snapshot()` 在代码中获取结构化数据。

### Q: Histogram 的默认 bucket 适合我的场景吗？

默认 bucket（1~10000ms）适合大多数 Web 服务的延迟分布。如果服务延迟特征不同（如数据库查询通常 10~500ms，AI API 调用可能 1000~30000ms），创建 Histogram 时指定自定义 bucket。

### Q: 为什么不用 prometheus/client_golang？

prometheus client 功能更全（Summary、带 exemplar、PushGateway 等），但依赖 protobuf + ~40 个子包。对于小团队、单服务的场景，本包足够且更轻量。如果需要企业级可观测性，直接引入 prometheus client 即可。

## 依赖

- 核心包（metric / registry / handler / health）：纯标准库
- middleware 子模块：依赖 `bedrock/bm`
- 系统探针（system.go）：`runtime` + `syscall`（标准库）

无任何第三方外部依赖。

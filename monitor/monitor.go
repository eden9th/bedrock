// Package monitor 提供进程内监控原语，作为 bedrock 基础组件。
//
// # 设计原则
//
// 本包提供的是可组合的监控原语，而非完整的监控应用。具体项目的业务指标、
// 定时调度、数据持久化、Dashboard 展示等由消费方自行组装。
//
// # 核心组件
//
//   - 指标类型（metric.go）：Counter / Gauge / Histogram，带 label 维度
//   - 注册中心（registry.go）：管理指标的生命周期，提供快照导出
//   - HTTP 暴露（handler.go）：Prometheus text 格式 + JSON 格式
//   - 系统探针（system.go）：CPU / 内存 / 磁盘 / Goroutine 等系统指标
//   - 健康检查（health.go）：通用的 HealthChecker 接口 + 并发执行
//
// # 快速开始
//
//	// 1. 创建指标
//	reqCount := monitor.NewCounter("http_requests_total", "help", []string{"method", "status"})
//	reqLatency := monitor.NewHistogram("http_request_duration_ms", "help",
//	    []float64{1, 5, 10, 50, 100, 500, 1000}, []string{"method"})
//
//	// 2. 注册
//	r := monitor.NewRegistry()
//	r.MustRegister(reqCount, reqLatency)
//
//	// 3. 在 handler 中使用
//	reqCount.Inc("GET", "200")
//	reqLatency.Observe(12.5, "GET")
//
//	// 4. 暴露 HTTP 端点
//	e := bm.New()
//	e.GET("/metrics", monitor.Handler(r))
//
//	// 5. 健康检查
//	hr := monitor.NewHealthRegistry()
//	hr.Register(myPostgresChecker)
//	e.GET("/health", monitor.HealthHandler(hr))
//
// # 与 invest/monitoring_technical_spec.md 的关系
//
// 本包从 invest 监控方案中提取了可作为基础组件的能力：
//
//	invest 方案                                → bedrock/monitor
//	───────────────────────────────────────────    ──────────────────
//	Counter / Gauge / Histogram 类型              → metric.go
//	存储接口 RecordCounter/Gauge/Histogram       → 废弃（指标只存内存，导出由消费方决定）
//	调度器 Scheduler                             → 废弃（time.Ticker 就够）
//	Dashboard HTML                                → 不纳入（展示层）
//	PostgreSQL / 内存存储                         → 不纳入（应用层决策）
//	DeepSeek / Tushare / Ollama 等具体服务检查    → 不纳入（业务逻辑）
//	SSE / 用户活跃度等业务指标                    → 不纳入（业务逻辑）
//	SystemStatusProvider 接口                    → HealthChecker 接口（通用化）
//	系统级指标采集                                → SystemCollector + RuntimeStats（通用化）
//	指标命名规范 {domain}.{entity}.{metric}.{unit} → 保留，消费方自行遵守
//
// # 依赖
//
//	核心（monitor/metric.go, registry.go, handler.go, health.go）:
//	  - 标准库 only
//	系统指标（monitor/system.go）:
//	  - github.com/shirou/gopsutil/v3 (cpu, mem, disk)
//
// 如果不需要系统指标，可以不 import gopsutil（Go 编译器会剔除未使用的依赖）。
package monitor

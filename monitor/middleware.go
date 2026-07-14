package monitor

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/eden9th/bedrock/bm"
)

// HTTPMetrics 持有 HTTP 中间件相关的指标。
// 通过 NewHTTPMetrics 创建，Register 注册到 Registry。
//
// 使用示例：
//
//	r := monitor.NewRegistry()
//	hm := monitor.NewHTTPMetrics(monitor.DefaultHTTPBuckets)
//	hm.Register(r)
//	e.Use(hm.Middleware())
//	e.GET("/metrics", monitor.Handler(r))
type HTTPMetrics struct {
	// 指标定义
	RequestCount    *Counter
	RequestDuration *Histogram
	InFlightCount   *Gauge
	ResponseSize    *Histogram

	// 可选的 path 归一化函数
	// 当 bm 路由返回空 pattern（如 404）时，使用此函数做降级处理。
	// 为 nil 时使用原始 URL path。
	UnknownPathHandler func(c *bm.Context) string

	// 是否记录请求/响应大小（默认 false，减少 Histogram 内存开销）
	RecordResponseSize bool

	// 哪些 HTTP 状态码范围记录为独立 label value。
	// 默认: status codes are grouped as "1xx","2xx","3xx","4xx","5xx"
	GroupStatus bool

	// SlowThreshold 慢请求阈值。请求耗时超过此值时输出 WARN 到 stderr。
	// 0 表示不启用慢请求检测。默认 0（关闭）。
	// 推荐值：生产环境 500ms，内部服务 200ms。
	SlowThreshold time.Duration
}

// DefaultHTTPBuckets 是 HTTP 请求延迟的推荐 bucket 边界（毫秒）。
// 覆盖从 1ms 到 10s 的范围，适合大多数 Web 服务。
var DefaultHTTPBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// DefaultResponseSizeBuckets 是响应大小的推荐 bucket 边界（字节）。
var DefaultResponseSizeBuckets = []float64{100, 1024, 4096, 16384, 65536, 262144, 1048576}

// NewHTTPMetrics 创建 HTTP 指标集。
// durationBuckets 为延迟 bucket 边界（毫秒），传 nil 使用 DefaultHTTPBuckets。
func NewHTTPMetrics(durationBuckets []float64) *HTTPMetrics {
	if len(durationBuckets) == 0 {
		durationBuckets = DefaultHTTPBuckets
	}
	return &HTTPMetrics{
		RequestCount: NewCounter(
			"http_requests_total",
			"Total number of HTTP requests",
			[]string{"method", "path", "status"},
		),
		RequestDuration: NewHistogram(
			"http_request_duration_ms",
			"HTTP request duration in milliseconds",
			durationBuckets,
			[]string{"method", "path", "status"},
		),
		InFlightCount: NewGauge(
			"http_requests_in_flight",
			"Current number of HTTP requests being handled",
			nil,
		),
		ResponseSize: NewHistogram(
			"http_response_size_bytes",
			"HTTP response size in bytes",
			DefaultResponseSizeBuckets,
			[]string{"method", "path", "status"},
		),
		GroupStatus: true,
	}
}

// Register 将 HTTP 指标注册到指定 Registry。
func (hm *HTTPMetrics) Register(r *Registry) {
	r.MustRegister(hm.RequestCount, hm.RequestDuration, hm.InFlightCount)
	if hm.RecordResponseSize {
		r.MustRegister(hm.ResponseSize)
	}
}

// Middleware 返回一个 bm 中间件，自动记录每个请求的计数、延迟和并发量。
//
// 指标 label 说明：
//   - method: HTTP 方法（GET/POST/...）
//   - path:   路由 pattern（如 /api/users/:id），404 时降级为原始 URL path
//   - status: HTTP 状态码。GroupStatus=true 时按范围分组（2xx/4xx/...），
//     GroupStatus=false 时使用具体状态码（200/404/...）
//
// 若设置了 SlowThreshold，请求耗时超过阈值时自动输出 WARN 到 stderr。
func (hm *HTTPMetrics) Middleware() bm.HandlerFunc {
	return func(c *bm.Context) {
		hm.InFlightCount.Add(1)
		defer hm.InFlightCount.Add(-1)

		start := time.Now()

		c.Next()

		duration := float64(time.Since(start).Microseconds()) / 1000.0 // μs → ms

		method := c.Request.Method
		path := hm.resolvePath(c)
		status := hm.resolveStatus(c)

		hm.RequestCount.Inc(method, path, status)
		hm.RequestDuration.Observe(duration, method, path, status)

		// 慢请求检测：超过阈值时输出 WARN 到 stderr
		if hm.SlowThreshold > 0 && time.Since(start) > hm.SlowThreshold {
			fmt.Fprintf(os.Stderr, "[monitor] slow request: %s %s → %s, duration=%.1fms\n",
				method, path, status, duration)
		}

		if hm.RecordResponseSize {
			size := hm.estimateResponseSize(c)
			hm.ResponseSize.Observe(float64(size), method, path, status)
		}
	}
}

// resolvePath 获取用于 metric label 的路径值。
// 优先使用 bm 路由 pattern（如 /api/users/:id），避免高基数。
func (hm *HTTPMetrics) resolvePath(c *bm.Context) string {
	if pattern := c.Pattern(); pattern != "" {
		return pattern
	}
	if hm.UnknownPathHandler != nil {
		return hm.UnknownPathHandler(c)
	}
	return c.Request.URL.Path
}

// resolveStatus 将 HTTP 状态码转为指标 label 值。
func (hm *HTTPMetrics) resolveStatus(c *bm.Context) string {
	status := c.WriterStatus()
	if hm.GroupStatus {
		return statusGroup(status)
	}
	return strconv.Itoa(status)
}

// estimateResponseSize 返回实际写入的响应字节数。
func (hm *HTTPMetrics) estimateResponseSize(c *bm.Context) int {
	return c.BytesWritten()
}

// statusGroup 将状态码按百位分组。
func statusGroup(code int) string {
	switch {
	case code < 200:
		return "1xx"
	case code < 300:
		return "2xx"
	case code < 400:
		return "3xx"
	case code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

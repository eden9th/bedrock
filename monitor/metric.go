// Package monitor 提供进程内监控原语：指标定义、注册、暴露和系统探针。
//
// 核心概念（参考 Prometheus 指标模型，但精简为进程内直接可用）：
//
//   - Counter：只增不减的累计值（如请求总数），重启归零
//   - Gauge：可增可减的瞬时值（如 CPU 使用率、当前连接数）
//   - Histogram：观测值的分桶分布（如请求延迟），预定义 bucket 边界
//
// 每个指标都附带 name（唯一标识）、help（描述）和可选的 label 维度。
// label 值数量必须与声明时的 label key 数量一致，不匹配会触发 panic（编程错误，应尽早暴露）。
//
// 线程安全：所有指标类型的方法都是并发安全的。
package monitor

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// ─── label key 编码 ──────────────────────────────────────────────────────────

// labelKey 把 label values 编码为内部 map key，用 \x00 分隔避免与用户数据碰撞
func labelKey(vals []string) string {
	return strings.Join(vals, "\x00")
}

// validateLabels 检查 labels 声明和传值的一致性
func validateLabels(declared []string, values []string) {
	if len(declared) != len(values) {
		panic(fmt.Sprintf("monitor: label count mismatch: declared %d labels %v, got %d values %v",
			len(declared), declared, len(values), values))
	}
}

// ─── Metric 接口 ─────────────────────────────────────────────────────────────

// Metric 是所有指标类型必须实现的接口。
// 用户通常不需要直接实现此接口；使用 NewCounter/NewGauge/NewHistogram 即可。
type Metric interface {
	// Name 返回创建时指定的指标名称
	Name() string
	// Help 返回创建时指定的帮助文本
	Help() string
	// Type 返回指标类型字符串: "counter" / "gauge" / "histogram"
	Type() string
	// Labels 返回声明时的 label key 列表
	Labels() []string
	// Snapshot 返回当前所有 label 组合下的值快照，用于序列化导出
	Snapshot() []MetricSample
}

// MetricSample 是单个 label 组合下的指标快照。
// 对于 Counter / Gauge，Value 字段有效。
// 对于 Histogram，Values 字段包含 bucket 累计值，末尾追加 +Inf bucket（总数）。
type MetricSample struct {
	Labels   map[string]string // label key → value，无 label 时为空 map
	Value    float64           // Counter / Gauge 的值
	Values   []float64         // Histogram 的各 bucket 累计计数
	Sum      float64           // Histogram 的观测值总和
	Count    uint64            // Histogram 的观测次数
	Buckets  []float64         // Histogram 的 bucket 边界（拷贝，方便消费方）
}

// ─── Counter ─────────────────────────────────────────────────────────────────

// Counter 是只增不减的累计指标。常用于请求计数、错误计数等。
// 零值不可用，必须通过 NewCounter 创建。
type Counter struct {
	name   string
	help   string
	labels []string

	// DefaultLabels 预设的 label 值。设置后 Inc/Add 只需传非默认的 label 值。
	// 例：DefaultLabels{"service":"api"}, labels=["method"]，
	// 调用 c.Inc("GET") 自动补全为 ("api","GET")。
	DefaultLabels map[string]string

	mu     sync.RWMutex
	values map[string]uint64 // labelKey → value
}

// NewCounter 创建一个 Counter 指标。
//
//	name: 唯一标识，如 "http_requests_total"
//	help: 描述文本
//	labels: 维度名列表，如 []string{"method", "status"}，无维度传 nil 或空切片
func NewCounter(name, help string, labels []string) *Counter {
	return &Counter{
		name:   name,
		help:   help,
		labels: copyLabels(labels),
		values: make(map[string]uint64),
	}
}

func (c *Counter) Name() string              { return c.name }
func (c *Counter) Help() string              { return c.help }
func (c *Counter) Type() string              { return "counter" }
func (c *Counter) Labels() []string          { return c.labels }

// Inc 将指定 label 组合的 Counter 值 +1。
// values 必须与创建时的 labels 一一对应，数量不匹配会 panic。
func (c *Counter) Inc(values ...string) { c.Add(1, c.prependDefaults(values)...) }

// Add 将指定 label 组合的 Counter 值 +delta（delta 必须 >= 0）。
func (c *Counter) Add(delta float64, values ...string) {
	values = c.prependDefaults(values)
	if delta < 0 {
		return // Counter 不下降
	}
	validateLabels(c.labels, values)
	key := labelKey(values)

	c.mu.Lock()
	// uint64 加 float64，先转再累加，溢出按 math.MaxUint64 截断
	v := c.values[key]
	added := uint64(delta)
	if math.MaxUint64-v < added {
		c.values[key] = math.MaxUint64
	} else {
		c.values[key] = v + added
	}
	c.mu.Unlock()
}

// Value 返回指定 label 组合的当前值，不存在则返回 0。
func (c *Counter) Value(values ...string) uint64 {
	values = c.prependDefaults(values)
	validateLabels(c.labels, values)
	key := labelKey(values)

	c.mu.RLock()
	v := c.values[key]
	c.mu.RUnlock()
	return v
}

// prependDefaults 将 DefaultLabels 的值按 key 顺序插到 values 前面。
func (c *Counter) prependDefaults(values []string) []string {
	if len(c.DefaultLabels) == 0 {
		return values
	}
	n := len(c.DefaultLabels)
	all := make([]string, 0, n+len(values))
	for _, k := range c.Labels()[:n] {
		all = append(all, c.DefaultLabels[k])
	}
	all = append(all, values...)
	return all
}

// Snapshot 返回 Counter 的所有 label 组合的快照。
func (c *Counter) Snapshot() []MetricSample {
	c.mu.RLock()
	defer c.mu.RUnlock()

	samples := make([]MetricSample, 0, len(c.values))
	for key, val := range c.values {
		samples = append(samples, MetricSample{
			Labels: splitLabels(c.labels, key),
			Value:  float64(val),
		})
	}
	sortSamples(samples)
	return samples
}

// ─── Gauge ───────────────────────────────────────────────────────────────────

// Gauge 是可增可减的瞬时指标。常用于 CPU 使用率、内存用量、当前连接数等。
// 零值不可用，必须通过 NewGauge 创建。
type Gauge struct {
	name   string
	help   string
	labels []string

	// DefaultLabels 预设的 label 值。设置后 Set/Add 只需传非默认的 label 值。
	DefaultLabels map[string]string

	mu     sync.RWMutex
	values map[string]float64 // labelKey → value
}

// NewGauge 创建一个 Gauge 指标。
func NewGauge(name, help string, labels []string) *Gauge {
	return &Gauge{
		name:   name,
		help:   help,
		labels: copyLabels(labels),
		values: make(map[string]float64),
	}
}

func (g *Gauge) Name() string              { return g.name }
func (g *Gauge) Help() string              { return g.help }
func (g *Gauge) Type() string              { return "gauge" }
func (g *Gauge) Labels() []string          { return g.labels }

// Set 将指定 label 组合的 Gauge 值设为 val。
func (g *Gauge) Set(val float64, values ...string) {
	values = g.prependDefaults(values)
	validateLabels(g.labels, values)
	key := labelKey(values)

	g.mu.Lock()
	g.values[key] = val
	g.mu.Unlock()
}

// Add 将指定 label 组合的 Gauge 值 +delta（delta 可为负）。
func (g *Gauge) Add(delta float64, values ...string) {
	values = g.prependDefaults(values)
	validateLabels(g.labels, values)
	key := labelKey(values)

	g.mu.Lock()
	g.values[key] += delta
	g.mu.Unlock()
}

// Value 返回指定 label 组合的当前值，不存在则返回 0。
func (g *Gauge) Value(values ...string) float64 {
	values = g.prependDefaults(values)
	validateLabels(g.labels, values)
	key := labelKey(values)

	g.mu.RLock()
	v := g.values[key]
	g.mu.RUnlock()
	return v
}

// prependDefaults 将 DefaultLabels 的值按 key 顺序插到 values 前面。
func (g *Gauge) prependDefaults(values []string) []string {
	if len(g.DefaultLabels) == 0 {
		return values
	}
	n := len(g.DefaultLabels)
	all := make([]string, 0, n+len(values))
	for _, k := range g.Labels()[:n] {
		all = append(all, g.DefaultLabels[k])
	}
	all = append(all, values...)
	return all
}

// Snapshot 返回 Gauge 的所有 label 组合的快照。
func (g *Gauge) Snapshot() []MetricSample {
	g.mu.RLock()
	defer g.mu.RUnlock()

	samples := make([]MetricSample, 0, len(g.values))
	for key, val := range g.values {
		samples = append(samples, MetricSample{
			Labels: splitLabels(g.labels, key),
			Value:  val,
		})
	}
	sortSamples(samples)
	return samples
}

// ─── Histogram ───────────────────────────────────────────────────────────────

// Histogram 是观测值的分桶分布。常用于请求延迟、响应大小等。
// bucket 边界必须在创建时指定，按升序排列。
// 默认追加 +Inf bucket 用于统计总次数，无需用户显式添加。
type Histogram struct {
	name    string
	help    string
	labels  []string
	buckets []float64 // 不含 +Inf

	// DefaultLabels 预设的 label 值。设置后 Observe 只需传非默认的 label 值。
	DefaultLabels map[string]string

	mu     sync.RWMutex
	values map[string]*histogramData // labelKey → data
}

// histogramData 存储单个 label 组合的直方图数据。
type histogramData struct {
	count   uint64
	sum     float64
	buckets []uint64 // 与 Histogram.buckets 一一对应，累计计数
}

// NewHistogram 创建一个 Histogram 指标。
//
//	buckets 必须按升序排列，如 []float64{1, 5, 10, 50, 100, 500, 1000}
func NewHistogram(name, help string, buckets []float64, labels []string) *Histogram {
	if len(buckets) == 0 {
		buckets = defaultBuckets
	}
	// 拷贝并校验升序
	sorted := make([]float64, len(buckets))
	copy(sorted, buckets)
	sort.Float64s(sorted)

	return &Histogram{
		name:    name,
		help:    help,
		labels:  copyLabels(labels),
		buckets: sorted,
		values:  make(map[string]*histogramData),
	}
}

// 默认 bucket: 覆盖从 1ms 到 10s 的常见延迟范围
var defaultBuckets = []float64{
	1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000,
}

func (h *Histogram) Name() string     { return h.name }
func (h *Histogram) Help() string     { return h.help }
func (h *Histogram) Type() string     { return "histogram" }
func (h *Histogram) Labels() []string { return h.labels }

// Observe 记录一次观测值。
func (h *Histogram) Observe(val float64, values ...string) {
	values = h.prependDefaults(values)
	validateLabels(h.labels, values)
	key := labelKey(values)

	h.mu.Lock()
	h.observeLocked(key, val)
	h.mu.Unlock()
}

// observeLocked 在持有锁时记录观测值
func (h *Histogram) observeLocked(key string, val float64) {
	hd, ok := h.values[key]
	if !ok {
		hd = &histogramData{buckets: make([]uint64, len(h.buckets))}
		h.values[key] = hd
	}
	hd.count++
	hd.sum += val
	// 找到第一个 >= val 的 bucket 位置
	for i, upper := range h.buckets {
		if val <= upper {
			hd.buckets[i]++
		}
	}
}

// prependDefaults 将 DefaultLabels 的值按 key 顺序插到 values 前面。
func (h *Histogram) prependDefaults(values []string) []string {
	if len(h.DefaultLabels) == 0 {
		return values
	}
	n := len(h.DefaultLabels)
	all := make([]string, 0, n+len(values))
	for _, k := range h.Labels()[:n] {
		all = append(all, h.DefaultLabels[k])
	}
	all = append(all, values...)
	return all
}

// Snapshot 返回 Histogram 的所有 label 组合的快照。
// 对于 Histogram，每个 MetricSample 的 Values 包含各 bucket 累计计数 + 末尾追加 +Inf（总次数）。
func (h *Histogram) Snapshot() []MetricSample {
	h.mu.RLock()
	defer h.mu.RUnlock()

	samples := make([]MetricSample, 0, len(h.values))
	bucketCopy := make([]float64, len(h.buckets))
	copy(bucketCopy, h.buckets)

	for key, hd := range h.values {
		// Values: bucket 累计值 + 末尾追加 total count
		vals := make([]float64, len(hd.buckets)+1)
		for i := range hd.buckets {
			vals[i] = float64(hd.buckets[i])
		}
		vals[len(hd.buckets)] = float64(hd.count)

		samples = append(samples, MetricSample{
			Labels:  splitLabels(h.labels, key),
			Values:  vals,
			Sum:     hd.sum,
			Count:   hd.count,
			Buckets: bucketCopy,
		})
	}
	sortSamples(samples)
	return samples
}

// ─── 内部工具函数 ────────────────────────────────────────────────────────────

func copyLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, len(labels))
	copy(out, labels)
	return out
}

// splitLabels 将 labelKey 解析回 map[string]string
func splitLabels(keys []string, encoded string) map[string]string {
	if len(keys) == 0 {
		return map[string]string{}
	}
	vals := strings.Split(encoded, "\x00")
	m := make(map[string]string, len(keys))
	for i, k := range keys {
		m[k] = vals[i]
	}
	return m
}

// sortSamples 按 label 序列化 key 排序，保证输出稳定
func sortSamples(samples []MetricSample) {
	sort.Slice(samples, func(i, j int) bool {
		return labelMapKey(samples[i].Labels) < labelMapKey(samples[j].Labels)
	})
}

func labelMapKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		parts = append(parts, k, labels[k])
	}
	return strings.Join(parts, "\x00")
}

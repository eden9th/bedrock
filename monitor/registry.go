package monitor

import (
	"fmt"
	"sort"
	"sync"
)

// Registry 是指标的注册中心。
// 所有需要暴露的指标必须注册到 Registry，然后通过 Handler 导出。
//
// 使用示例:
//
//	r := monitor.NewRegistry()
//	c := monitor.NewCounter("requests_total", "help", []string{"method"})
//	r.MustRegister(c)
//	// ...
//	e.GET("/metrics", monitor.Handler(r))
type Registry struct {
	mu      sync.RWMutex
	metrics map[string]Metric // name → Metric
}

// NewRegistry 创建一个空的 Registry。
func NewRegistry() *Registry {
	return &Registry{
		metrics: make(map[string]Metric),
	}
}

// Register 注册一个或多个指标。name 冲突时返回错误。
func (r *Registry) Register(metrics ...Metric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, m := range metrics {
		if _, ok := r.metrics[m.Name()]; ok {
			return fmt.Errorf("monitor: metric %q already registered", m.Name())
		}
		r.metrics[m.Name()] = m
	}
	return nil
}

// MustRegister 注册指标，name 冲突时 panic（适用于 init 阶段注册）。
func (r *Registry) MustRegister(metrics ...Metric) {
	if err := r.Register(metrics...); err != nil {
		panic(err)
	}
}

// Unregister 移除指定名称的指标。不存在时静默忽略。
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	delete(r.metrics, name)
	r.mu.Unlock()
}

// Get 按名称查找已注册的指标，未注册返回 nil。
func (r *Registry) Get(name string) Metric {
	r.mu.RLock()
	m := r.metrics[name]
	r.mu.RUnlock()
	return m
}

// Metrics 返回所有已注册指标的列表（按名称排序）。
func (r *Registry) Metrics() []Metric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Metric, 0, len(r.metrics))
	for _, m := range r.metrics {
		list = append(list, m)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name() < list[j].Name()
	})
	return list
}

// Snapshot 返回所有已注册指标的当前快照。
// 返回按指标名排序，每个指标的 sample 按 label 排序。
func (r *Registry) Snapshot() []MetricSnapshot {
	metrics := r.Metrics()

	snapshots := make([]MetricSnapshot, 0, len(metrics))
	for _, m := range metrics {
		snapshots = append(snapshots, MetricSnapshot{
			Name:    m.Name(),
			Help:    m.Help(),
			Type:    m.Type(),
			Samples: m.Snapshot(),
		})
	}
	return snapshots
}

// MetricSnapshot 是单个指标的完整快照，包含元数据和所有 label 组合的当前值。
type MetricSnapshot struct {
	Name    string         // 指标名称
	Help    string         // 帮助文本
	Type    string         // counter / gauge / histogram
	Samples []MetricSample // 各 label 组合的值
}

// ─── 全局默认 Registry ───────────────────────────────────────────────────────

var defaultRegistry = NewRegistry()

// DefaultRegistry 返回全局默认 Registry。
// 适用于简单场景：注册到全局，直接使用 Handler(nil) 时自动使用此 Registry。
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// RegisterGlobal 注册指标到全局默认 Registry。
func RegisterGlobal(metrics ...Metric) {
	defaultRegistry.MustRegister(metrics...)
}

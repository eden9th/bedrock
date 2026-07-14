package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/eden9th/bedrock/bm"
)

// ─── 格式常量 ────────────────────────────────────────────────────────────────

const (
	// 通过 query param ?format= 选择输出格式
	FormatPrometheus = "prometheus" // Prometheus text 格式（默认）
	FormatJSON       = "json"       // JSON 格式
)

// Handler 返回一个 bm.HandlerFunc，以 HTTP 形式暴露 Registry 中的指标。
//
//	reg 为 nil 时使用全局默认 Registry。
//
// 路由示例:
//
//	e.GET("/metrics", monitor.Handler(nil))       // 默认 Registry
//	e.GET("/metrics", monitor.Handler(myRegistry)) // 自定义 Registry
//
// 输出格式:
//
//	?format=prometheus (默认) → Prometheus text exposition format
//	?format=json       → JSON: {"metrics": [{"name":"...","type":"...","samples":[...]}]}
func Handler(reg *Registry) bm.HandlerFunc {
	if reg == nil {
		reg = defaultRegistry
	}
	return func(c *bm.Context) {
		format := c.Query("format")
		c.Header("Content-Type", contentType(format))
		c.Writer.WriteHeader(http.StatusOK)

		snapshots := reg.Snapshot()
		switch format {
		case FormatJSON:
			_ = json.NewEncoder(c.Writer).Encode(map[string]any{
				"metrics": snapshots,
			})
		default:
			_, _ = c.Writer.Write(formatPrometheus(snapshots))
		}
	}
}

// HandlerFunc 返回一个标准库 http.HandlerFunc（不依赖 bm 框架）。
// 适用于非 bm 的 HTTP 场景。
func HandlerFunc(reg *Registry) http.HandlerFunc {
	if reg == nil {
		reg = defaultRegistry
	}
	return func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")
		w.Header().Set("Content-Type", contentType(format))
		w.WriteHeader(http.StatusOK)

		snapshots := reg.Snapshot()
		switch format {
		case FormatJSON:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metrics": snapshots,
			})
		default:
			_, _ = w.Write(formatPrometheus(snapshots))
		}
	}
}

func contentType(format string) string {
	switch format {
	case FormatJSON:
		return "application/json; charset=utf-8"
	default:
		return "text/plain; version=0.0.4; charset=utf-8"
	}
}

// ─── Prometheus text format 输出 ─────────────────────────────────────────────

// formatPrometheus 按 Prometheus exposition format 序列化指标快照。
// 参考: https://prometheus.io/docs/instrumenting/exposition_formats/
func formatPrometheus(snapshots []MetricSnapshot) []byte {
	var b strings.Builder

	for _, m := range snapshots {
		// HELP line
		fmt.Fprintf(&b, "# HELP %s %s\n", m.Name, escapeHelp(m.Help))
		// TYPE line
		fmt.Fprintf(&b, "# TYPE %s %s\n", m.Name, m.Type)

		for _, s := range m.Samples {
			switch m.Type {
			case "counter", "gauge":
				// name{labels} value
				b.WriteString(m.Name)
				b.WriteString(formatLabelPairs(s.Labels))
				// Counter/Gauge 值始终为整数（uint64 累积），直接输出
				fmt.Fprintf(&b, " %v\n", formatValue(s.Value))

			case "histogram":
				// bucket 累计: name_bucket{labels,le="upper"} count
				for i, upper := range s.Buckets {
					b.WriteString(m.Name + "_bucket")
					// 合并已有 labels 和 le label
					pairs := makeLabelPairsWith(s.Labels, "le", formatValue(upper))
					b.WriteString(pairs)
					fmt.Fprintf(&b, " %d\n", uint64(s.Values[i]))
				}
				// +Inf bucket
				b.WriteString(m.Name + "_bucket")
				pairs := makeLabelPairsWith(s.Labels, "le", "+Inf")
				b.WriteString(pairs)
				fmt.Fprintf(&b, " %d\n", s.Count)

				// sum: name_sum{labels} sum
				b.WriteString(m.Name + "_sum")
				b.WriteString(formatLabelPairs(s.Labels))
				fmt.Fprintf(&b, " %v\n", formatValue(s.Sum))

				// count: name_count{labels} count
				b.WriteString(m.Name + "_count")
				b.WriteString(formatLabelPairs(s.Labels))
				fmt.Fprintf(&b, " %d\n", s.Count)
			}
		}
	}

	return []byte(b.String())
}

// formatValue 把 float64 格式化为最短字符串。整数不保留小数点。
func formatValue(v float64) string {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%g", v)
}

func escapeHelp(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// formatLabelPairs 把 label map 格式化为 Prometheus {key="value",...} 形式
func formatLabelPairs(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	first := true
	keys := sortedLabelKeys(labels)
	for _, k := range keys {
		if !first {
			b.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&b, "%s=%q", k, labels[k])
	}
	b.WriteByte('}')
	return b.String()
}

// makeLabelPairsWith 同 formatLabelPairs，但额外追加一个 key=value
func makeLabelPairsWith(labels map[string]string, extraK, extraV string) string {
	// 如果 labels 为空且只有一个 extra，直接格式化为 {key="value"}
	if len(labels) == 0 {
		return fmt.Sprintf("{%s=%q}", extraK, extraV)
	}

	var b strings.Builder
	b.WriteByte('{')
	first := true
	keys := sortedLabelKeys(labels)
	for _, k := range keys {
		if !first {
			b.WriteByte(',')
		}
		first = false
		fmt.Fprintf(&b, "%s=%q", k, labels[k])
	}
	if !first {
		b.WriteByte(',')
	}
	fmt.Fprintf(&b, "%s=%q", extraK, extraV)
	b.WriteByte('}')
	return b.String()
}

func sortedLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

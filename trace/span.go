package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Span 表示一个操作追踪区间。纯内存实现，日志关联。
// 与 trace_id 关联，提供操作级别的耗时和属性记录。
//
// 使用示例：
//
//	span := trace.StartSpan(ctx, "db_query")
//	defer span.End()
//	span.SetAttribute("db.table", "users")
//	span.AddEvent("cache_miss")
//
// 结束时自动输出 JSON 日志到 stderr。
type Span struct {
	Name       string
	TraceID    string
	startTime  time.Time
	attributes map[string]string
	events     []SpanEvent
	mu         sync.Mutex
	ended      bool
}

// SpanEvent 是 Span 中记录的事件。
type SpanEvent struct {
	Name      string
	Timestamp time.Time
}

// StartSpan 从 context 中提取 trace_id，创建新的 Span 并开始计时。
// ctx 中没有 trace_id 时，trace_id 为空。
func StartSpan(ctx context.Context, name string) *Span {
	return &Span{
		Name:       name,
		TraceID:    GetTraceID(ctx),
		startTime:  time.Now(),
		attributes: make(map[string]string),
	}
}

// SetAttribute 设置 Span 属性（key-value）。
func (s *Span) SetAttribute(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attributes[key] = value
}

// AddEvent 记录一个时间点事件（如 cache_hit、retry_attempt）。
func (s *Span) AddEvent(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, SpanEvent{Name: name, Timestamp: time.Now()})
}

// End 结束 Span，输出 JSON 日志到 stderr。
// 多次调用只生效一次。
func (s *Span) End() {
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	s.ended = true
	s.mu.Unlock()

	duration := time.Since(s.startTime)

	eventNames := make([]string, len(s.events))
	for i, e := range s.events {
		eventNames[i] = e.Name
	}

	output, _ := json.Marshal(map[string]any{
		"time":        s.startTime.Format("2006/01/02-15:04:05.000"),
		"span":        s.Name,
		"trace_id":    s.TraceID,
		"duration_ms": float64(duration.Microseconds()) / 1000.0,
		"attrs":       s.attributes,
		"events":      eventNames,
	})
	fmt.Fprintln(os.Stderr, string(output))
}

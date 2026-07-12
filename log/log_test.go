// 白盒测试，直接访问包内变量验证行为
package log

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/eden9th/bedrock/internal/ctxkey"
)

// captureLog 临时替换 writers 为 buffer，执行 fn 后还原，返回捕获的输出
func captureLog(fn func()) string {
	mu.Lock()
	orig := writers
	buf := &bytes.Buffer{}
	writers = []io.Writer{buf}
	mu.Unlock()

	fn()

	mu.Lock()
	writers = orig
	mu.Unlock()
	return buf.String()
}

// setLevel 临时设置 minLevel，返回 restore 函数
func setLevel(l Level) func() {
	mu.Lock()
	orig := minLevel
	minLevel = l
	mu.Unlock()
	return func() {
		mu.Lock()
		minLevel = orig
		mu.Unlock()
	}
}

func TestLevelDebug_ExplicitlySettable(t *testing.T) {
	// 回归测试 P1：LevelDebug 必须可以被显式设置（原来零值 bug 导致无法设置）
	restore := setLevel(LevelDebug)
	defer restore()

	out := captureLog(func() {
		Debug(context.Background(), "debug message")
	})
	if out == "" {
		t.Fatal("LevelDebug set explicitly but Debug log was not output")
	}
	if !strings.Contains(out, "DEBUG") {
		t.Fatalf("expected DEBUG level in output, got: %s", out)
	}
}

func TestLevelUnset_DoesNotChangeMinLevel(t *testing.T) {
	// LevelUnset (零值) 传入 Init 不应修改 minLevel
	restore := setLevel(LevelWarn)
	defer restore()

	Init(&Config{Level: LevelUnset})

	mu.RLock()
	got := minLevel
	mu.RUnlock()

	if got != LevelWarn {
		t.Fatalf("LevelUnset should not change minLevel, expected LevelWarn(%d), got %d", LevelWarn, got)
	}
}

func TestTraceID_AppearsInLog(t *testing.T) {
	// 回归测试 P0：trace_id 必须出现在日志输出中
	restore := setLevel(LevelInfo)
	defer restore()

	ctx := context.WithValue(context.Background(), ctxkey.TraceID, "trace-abc-123")

	out := captureLog(func() {
		Info(ctx, "test message")
	})

	var e entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &e); err != nil {
		t.Fatalf("failed to parse log JSON: %v\noutput: %s", err, out)
	}
	if e.TraceID != "trace-abc-123" {
		t.Fatalf("expected trace_id %q in log, got %q", "trace-abc-123", e.TraceID)
	}
}

func TestTraceID_OmittedWhenEmpty(t *testing.T) {
	restore := setLevel(LevelInfo)
	defer restore()

	out := captureLog(func() {
		Info(context.Background(), "no trace")
	})

	// omitempty：trace_id 为空时不应出现在 JSON
	if strings.Contains(out, "trace_id") {
		t.Fatalf("trace_id field should be omitted when empty, got: %s", out)
	}
}

func TestLevelFiltering(t *testing.T) {
	restore := setLevel(LevelError)
	defer restore()

	out := captureLog(func() {
		Debug(context.Background(), "debug msg")
		Info(context.Background(), "info msg")
		Warn(context.Background(), "warn msg")
		Error(context.Background(), "error msg")
	})

	if strings.Contains(out, "debug msg") {
		t.Fatal("DEBUG should be filtered when minLevel=ERROR")
	}
	if strings.Contains(out, "info msg") {
		t.Fatal("INFO should be filtered when minLevel=ERROR")
	}
	if strings.Contains(out, "warn msg") {
		t.Fatal("WARN should be filtered when minLevel=ERROR")
	}
	if !strings.Contains(out, "error msg") {
		t.Fatal("ERROR should be output when minLevel=ERROR")
	}
}

func TestCallerField(t *testing.T) {
	restore := setLevel(LevelInfo)
	defer restore()

	out := captureLog(func() {
		Info(context.Background(), "caller test")
	})

	var e entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &e); err != nil {
		t.Fatalf("failed to parse log JSON: %v", err)
	}
	// caller 应包含文件名和行号，格式如 log/log_test.go:99
	if e.Caller == "" {
		t.Fatal("caller field should not be empty")
	}
	if !strings.Contains(e.Caller, ":") {
		t.Fatalf("caller field should contain line number, got: %q", e.Caller)
	}
}

func TestLogFields_Structure(t *testing.T) {
	restore := setLevel(LevelInfo)
	defer restore()

	out := captureLog(func() {
		Info(context.Background(), "structured %s", "test")
	})

	var e entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &e); err != nil {
		t.Fatalf("failed to parse log JSON: %v\noutput: %s", err, out)
	}
	if e.Time == "" {
		t.Fatal("time field should not be empty")
	}
	if e.Level != "INFO" {
		t.Fatalf("expected level INFO, got %q", e.Level)
	}
	if e.Msg != "structured test" {
		t.Fatalf("expected msg %q, got %q", "structured test", e.Msg)
	}
}

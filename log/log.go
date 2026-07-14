// Package log 提供结构化日志能力：
//   - 接受 context.Context，自动从中提取 trace_id 附加到每条日志
//   - JSON 格式输出，便于日志检索和解析
//   - 同时写 stderr 和文件（文件按大小分片，基于 lumberjack）
//   - 文件名应含日期，如 app.2026-07-12.log，lumberjack 按大小切分时归档文件也带时间戳
//   - 无外部上报、无采样率配置
package log

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/eden9th/bedrock/internal/ctxkey"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Level 日志级别
type Level int

const (
	// LevelUnset 是零值，表示未显式设置级别，Init 遇到此值时不修改默认级别
	LevelUnset Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

var levelStr = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

// SamplingConfig 日志采样配置，用于高 QPS 场景防止日志风暴。
// 相同消息在窗口内最多输出指定次数。
type SamplingConfig struct {
	// Window 采样窗口。默认 1s。
	Window time.Duration
	// MaxCount 窗口内最多输出次数。默认 10。
	MaxCount int
}

// Config 日志配置
type Config struct {
	// FilePath 日志文件路径，空字符串表示只输出到 stderr
	// 文件名应含日期，如 app.2026-07-12.log，lumberjack 按大小切分时归档文件也带时间戳
	FilePath string
	// MaxSizeMB 单个日志文件最大大小（MB），默认 200MB
	MaxSizeMB int
	// MaxBackups 保留最近多少个归档文件，默认 30
	MaxBackups int
	// MaxAgeDays 保留最近多少天的日志，默认 30
	MaxAgeDays int
	// Level 最低输出级别，默认 LevelInfo
	Level Level
	// Sampling 日志采样配置。nil 表示不启用采样。
	Sampling *SamplingConfig
}

// entry 一条日志记录
type entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	TraceID string         `json:"trace_id,omitempty"`
	Caller  string         `json:"caller"`
	Msg     string         `json:"msg"`
	Extra   map[string]any `json:"extra,omitempty"` // Infov/Warnv/Errorv 的类型化 KV 字段
}

var (
	mu       sync.RWMutex
	writers  []io.Writer
	minLevel = LevelInfo

	// 采样状态
	samplingCfg  *SamplingConfig
	samplingMu   sync.Mutex
	samplingBuckets = make(map[string]*sampleBucket) // msg key → bucket
)

type sampleBucket struct {
	count  int
	resetAt time.Time
}

// Init 初始化日志，cfg 为 nil 时只输出到 stderr
func Init(cfg *Config) {
	mu.Lock()
	defer mu.Unlock()

	ws := []io.Writer{os.Stderr}

	if cfg != nil {
		if cfg.Level != LevelUnset {
			minLevel = cfg.Level
		}
		// 采样配置
		if cfg.Sampling != nil {
			samplingMu.Lock()
			samplingCfg = cfg.Sampling
			if samplingCfg.Window <= 0 {
				samplingCfg.Window = time.Second
			}
			if samplingCfg.MaxCount <= 0 {
				samplingCfg.MaxCount = 10
			}
			samplingMu.Unlock()
		}
		if cfg.FilePath != "" {
			maxSize := 200
			if cfg.MaxSizeMB > 0 {
				maxSize = cfg.MaxSizeMB
			}
			maxBackups := 30
			if cfg.MaxBackups > 0 {
				maxBackups = cfg.MaxBackups
			}
			maxAge := 30
			if cfg.MaxAgeDays > 0 {
				maxAge = cfg.MaxAgeDays
			}
			ws = append(ws, &lumberjack.Logger{
				Filename:   cfg.FilePath,
				MaxSize:    maxSize,
				MaxBackups: maxBackups,
				MaxAge:     maxAge,
				Compress:   true,
				LocalTime:  true,
			})
		}
	}

	writers = ws
}

// Close 关闭日志（lumberjack 不需要显式关闭，保留接口兼容）
func Close() {}

// sampleCheck 检查当前消息是否超过采样限制。返回 true 表示应跳过输出。
func sampleCheck(msg string) bool {
	samplingMu.Lock()
	defer samplingMu.Unlock()

	if samplingCfg == nil {
		return false
	}

	now := time.Now()
	b, ok := samplingBuckets[msg]
	if !ok || now.After(b.resetAt) {
		samplingBuckets[msg] = &sampleBucket{count: 1, resetAt: now.Add(samplingCfg.Window)}
		return false
	}

	if b.count < samplingCfg.MaxCount {
		b.count++
		return false
	}
	return true
}

// writev 输出带类型化 KV 字段的日志（供 Infov/Warnv/Errorv 使用）
func writev(ctx any, level Level, msg string, fields []Field) {
	mu.RLock()
	lvl := minLevel
	ws := writers
	mu.RUnlock()

	if level < lvl {
		return
	}
	if sampleCheck(msg) {
		return
	}

	// 提取 trace_id：使用与 write() 相同的 ctxkey.TraceID
	traceID := ""
	if c, ok := ctx.(interface{ Value(any) any }); ok {
		if id, ok2 := c.Value(ctxkey.TraceID).(string); ok2 {
			traceID = id
		}
	}

	caller := ""
	if _, file, line, ok := runtime.Caller(2); ok {
		short := file
		slashes := 0
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				slashes++
				if slashes == 2 {
					short = file[i+1:]
					break
				}
			}
		}
		caller = fmt.Sprintf("%s:%d", short, line)
	}

	extra := make(map[string]any, len(fields))
	for _, f := range fields {
		extra[f.Key] = f.Value
	}

	e := entry{
		Time:    time.Now().Format("2006/01/02-15:04:05.000"),
		Level:   levelStr[level],
		TraceID: traceID,
		Caller:  caller,
		Msg:     msg,
		Extra:   extra,
	}

	raw, _ := json.Marshal(e)
	raw = append(raw, '\n')

	for _, w := range ws {
		_, _ = w.Write(raw)
	}
}

func write(ctx context.Context, level Level, format string, args ...any) {
	mu.RLock()
	lvl := minLevel
	ws := writers
	mu.RUnlock()

	if level < lvl {
		return
	}

	msg := fmt.Sprintf(format, args...)
	if sampleCheck(msg) {
		return
	}

	// 提取 trace_id，使用与 trace 包共享的 ctxkey.TraceID 确保能正确读到注入的值
	traceID := ""
	if ctx != nil {
		if id, ok := ctx.Value(ctxkey.TraceID).(string); ok {
			traceID = id
		}
	}

	// 调用者信息，跳过 log 包本身的两层，保留最后两段路径
	caller := ""
	if _, file, line, ok := runtime.Caller(2); ok {
		short := file
		slashes := 0
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				slashes++
				if slashes == 2 {
					short = file[i+1:]
					break
				}
			}
		}
		caller = fmt.Sprintf("%s:%d", short, line)
	}

	e := entry{
		Time:    time.Now().Format("2006/01/02-15:04:05.000"),
		Level:   levelStr[level],
		TraceID: traceID,
		Caller:  caller,
		Msg:     msg,
	}

	raw, _ := json.Marshal(e)
	raw = append(raw, '\n')

	for _, w := range ws {
		_, _ = w.Write(raw)
	}
}

// Debug 调试日志（默认不输出，需 Config.Level = LevelDebug）
func Debug(ctx context.Context, format string, args ...any) {
	write(ctx, LevelDebug, format, args...)
}

// Info 普通日志
func Info(ctx context.Context, format string, args ...any) {
	write(ctx, LevelInfo, format, args...)
}

// Warn 警告日志
func Warn(ctx context.Context, format string, args ...any) {
	write(ctx, LevelWarn, format, args...)
}

// Error 错误日志
func Error(ctx context.Context, format string, args ...any) {
	write(ctx, LevelError, format, args...)
}

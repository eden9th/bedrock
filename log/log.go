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

	"gopkg.in/natefinch/lumberjack.v2"
)

// contextKey 用于从 context 取 trace_id，与 trace 包保持一致
type contextKey struct{}

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
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
}

// entry 一条日志记录
type entry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	TraceID string `json:"trace_id,omitempty"`
	Caller  string `json:"caller"`
	Msg     string `json:"msg"`
}

var (
	mu       sync.RWMutex
	writers  []io.Writer
	minLevel = LevelInfo
)

// Init 初始化日志，cfg 为 nil 时只输出到 stderr
func Init(cfg *Config) {
	mu.Lock()
	defer mu.Unlock()

	ws := []io.Writer{os.Stderr}

	if cfg != nil {
		if cfg.Level != 0 {
			minLevel = cfg.Level
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

func write(ctx context.Context, level Level, format string, args ...any) {
	mu.RLock()
	lvl := minLevel
	ws := writers
	mu.RUnlock()

	if level < lvl {
		return
	}

	msg := fmt.Sprintf(format, args...)

	// 提取 trace_id
	traceID := ""
	if ctx != nil {
		if id, ok := ctx.Value(contextKey{}).(string); ok {
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

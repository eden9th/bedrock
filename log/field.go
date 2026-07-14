package log

import "time"

// Field 是类型化的日志键值对。与 log.Info(ctx, "msg") 的 printf 风格互补，
// 提供结构化更强的方式写入 JSON 日志——数值保留为 number 而非字符串。
//
// 使用：
//
//	log.Infov(ctx, "user action", log.KVInt64("uid", 123), log.KVString("action", "login"))
//	// → {"msg":"user action","uid":123,"action":"login"}
type Field struct {
	Key       string
	Value     any
	Int64Val  int64
	StringVal string
	Type      fieldType
}

type fieldType int

const (
	fieldString   fieldType = iota // JSON string
	fieldInt64                     // JSON number (int64)
	fieldFloat64                   // JSON number (float64)
	fieldBool                      // JSON bool
	fieldDuration                  // JSON number (nanoseconds)
)

// ─── KV 构造器 ──────────────────────────────────────────────────────────────

// KVString 创建一个字符串类型的日志字段。
func KVString(key, value string) Field {
	return Field{Key: key, Type: fieldString, StringVal: value, Value: value}
}

// KVInt64 创建一个 int64 类型的日志字段（JSON number）。
func KVInt64(key string, value int64) Field {
	return Field{Key: key, Type: fieldInt64, Int64Val: value, Value: value}
}

// KVInt 创建一个 int 类型的日志字段（JSON number）。
func KVInt(key string, value int) Field {
	return Field{Key: key, Type: fieldInt64, Int64Val: int64(value), Value: value}
}

// KVFloat64 创建一个 float64 类型的日志字段（JSON number）。
func KVFloat64(key string, value float64) Field {
	return Field{Key: key, Type: fieldFloat64, Value: value}
}

// KVBool 创建一个 bool 类型的日志字段。
func KVBool(key string, value bool) Field {
	return Field{Key: key, Type: fieldBool, StringVal: fmtBool(value), Value: value}
}

// KVDuration 创建一个 duration 类型的日志字段（纳秒数）。
func KVDuration(key string, value time.Duration) Field {
	return Field{Key: key, Type: fieldDuration, Int64Val: int64(value), Value: value.Nanoseconds()}
}

// ─── 类型化日志函数 ─────────────────────────────────────────────────────────

// Infov 输出 INFO 级别日志，附带类型化 KV 字段。
func Infov(ctx any, msg string, fields ...Field) {
	// 构建完整的格式化消息
	writev(ctx, LevelInfo, msg, fields)
}

// Warnv 输出 WARN 级别日志，附带类型化 KV 字段。
func Warnv(ctx any, msg string, fields ...Field) {
	writev(ctx, LevelWarn, msg, fields)
}

// Errorv 输出 ERROR 级别日志，附带类型化 KV 字段。
func Errorv(ctx any, msg string, fields ...Field) {
	writev(ctx, LevelError, msg, fields)
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

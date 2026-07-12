// Package ctxkey 定义 bedrock 内部跨包共享的 context key。
// 仅供 bedrock 内部包使用（internal 约束），外部无法引用。
package ctxkey

// traceIDKey 是 trace_id context key 的类型，不导出防止外部构造。
type traceIDKey struct{}

// TraceID 是 trace 包写入、log 包读取 trace_id 时共同使用的 key 值。
// 使用同一个变量（而非各自定义私有类型）确保 context.Value 能正确取到值。
var TraceID traceIDKey

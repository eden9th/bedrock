# log — 结构化 JSON 日志

> 输出 JSON 行格式日志，字段包含 `time / level / trace_id / caller / msg`。自动从 `context.Context` 提取 `trace_id`。同时写 stderr 和文件（文件按大小分片，基于 lumberjack）。

## 设计哲学

**一行一条，机器可读。** 所有日志以 JSON 行输出，便于 `jq` 过滤、日志平台采集和全文检索。

**trace_id 自动关联。** 日志和 `bedrock/trace` 共享 context key，无需手动传递 trace_id——只要请求经过了 `trace.Middleware()`，后续所有日志自动带上 `trace_id`。

## 核心概念

### 日志格式

```json
{"time":"2026/07/14-15:04:05.000","level":"INFO","trace_id":"uuid","caller":"handler/users.go:42","msg":"user login succeeded, uid=123"}
```

| 字段 | 说明 |
|------|------|
| `time` | 时间戳，格式 `YYYY/MM/DD-HH:MM:SS.mmm` |
| `level` | DEBUG / INFO / WARN / ERROR |
| `trace_id` | 请求追踪 ID（从 context 自动提取） |
| `caller` | 调用位置（文件:行号，保留最后两段路径） |
| `msg` | 日志消息 |

### 双输出

- **stderr**：始终输出，用于开发调试和容器环境
- **文件**：可选，基于 lumberjack 按大小分片（200MB 一个文件）

### 日志级别

```
DEBUG < INFO < WARN < ERROR
```

`Init` 设置最低输出级别。低于该级别的日志静默丢弃。

## 快速开始

```go
import "github.com/eden9th/bedrock/log"

// 初始化
log.Init(&log.Config{
    FilePath:   "logs/app.2026-07-14.log",
    MaxSizeMB:  200,  // 单文件最大 200MB
    MaxBackups: 30,   // 保留 30 个归档
    MaxAgeDays: 30,   // 保留 30 天
    Level:      log.LevelInfo,
})

// 使用（传入 context 以自动附加 trace_id）
log.Info(ctx, "server started on %s", addr)
log.Error(ctx, "query failed, id=%d err=%+v", id, err)
log.Debug(ctx, "cache hit, key=%s", key)  // 默认不输出，需 LevelDebug

// 应用退出前
defer log.Close()
```

## API 参考

```go
// 初始化
func Init(cfg *Config)    // cfg 为 nil 时仅输出 stderr
func Close()

// 日志输出
func Debug(ctx context.Context, format string, args ...any)
func Info(ctx context.Context, format string, args ...any)
func Warn(ctx context.Context, format string, args ...any)
func Error(ctx context.Context, format string, args ...any)

// 配置
type Config struct {
    FilePath   string // 文件路径，含日期（如 "app.2026-07-14.log"），空则仅 stderr
    MaxSizeMB  int    // 单文件最大 MB 数，默认 200
    MaxBackups int    // 保留归档文件数，默认 30
    MaxAgeDays int    // 保留天数，默认 30
    Level      Level  // 最低输出级别，默认 LevelInfo
}

type Level int
const (
    LevelUnset Level = iota // 零值，不修改默认级别
    LevelDebug
    LevelInfo
    LevelWarn
    LevelError
)
```

## 常见问题

### Q: context 没有 trace_id 怎么办？

`trace_id` 字段会输出空字符串 `""`。不影响其他字段。

### Q: 日志文件什么时候轮转？

lumberjack 在文件大小超过 `MaxSizeMB` 时自动轮转。轮转后的文件命名为 `app.2026-07-14.log-2026-07-14T15-04-05.000` 格式并压缩。

### Q: 高性能场景会阻塞吗？

每条日志都会持 `sync.RWMutex` 的读锁，写入 stderr + 文件。对于高频日志场景（> 10000 条/秒），考虑使用缓冲 channel + 异步写入。当前设计面向中等吞吐量。

### Q: 为什么日志中文件名只保留最后两段路径？

减少噪音。`/Users/chaos/Documents/work/go/src/myapp/handler/users.go:42` 变成 `handler/users.go:42`。在大多数情况下这足够定位代码位置。

## 依赖

- `gopkg.in/natefinch/lumberjack.v2` — 日志文件轮转与压缩

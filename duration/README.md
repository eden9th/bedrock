# duration — TOML 可反序列化的 Duration

> 在 TOML 配置中直接写 `"1h30m"`，通过 `UnmarshalTOML` 自动解析为 `time.Duration` 类型的配置字段。

## 设计哲学

**让配置更自然。** Go 标准配置字段写 `"30s"` 需要手动 `time.ParseDuration`，或者写成毫秒数 `30000` 难以阅读。`duration.Duration` 实现了 `toml.Unmarshaler`，配置中直接写 `"30s"` 即可。

## 核心概念

```toml
# config.toml
timeout = "30s"       # → 30 * time.Second
interval = "1h30m"   # → 1h30m0s
retry_delay = "500ms"
```

```go
type Config struct {
    Timeout    duration.Duration `toml:"timeout"`
    Interval   duration.Duration `toml:"interval"`
    RetryDelay duration.Duration `toml:"retry_delay"`
}

var cfg Config
conf.Get("config.toml").UnmarshalTOML(&cfg)

cfg.Timeout.Duration()  // → time.Duration(30_000_000_000)
```

## 快速开始

```go
import "github.com/eden9th/bedrock/duration"

type AppConfig struct {
    Timeout duration.Duration `toml:"timeout"`
}

var cfg AppConfig
conf.Get("app.toml").UnmarshalTOML(&cfg)

// 使用
ctx, cancel := context.WithTimeout(ctx, cfg.Timeout.Duration())
```

## API 参考

```go
type Duration struct { ... }
func (d *Duration) Duration() time.Duration  // 返回解析后的 time.Duration
func (d *Duration) UnmarshalTOML(v any) error  // 实现 toml.Unmarshaler
```

## 常见问题

### Q: 支持哪些格式？

任何 `time.ParseDuration` 支持的格式：`"300ms"`, `"1.5h"`, `"2h45m"`, `"1h30m10s"` 等。不支持纯数字（如 `30`）。

### Q: 零值怎么表示？

配置中不写该字段或写 `"0s"`，`Duration()` 返回 `0`。

### Q: 可以负数吗？

不支持。`time.ParseDuration` 允许负值，但配置场景中负 duration 没有意义。

## 依赖

纯标准库。

# conf — TOML 配置 + 热更新

> 从指定目录加载 TOML 文件，基于 `fsnotify` 监听文件变更，触发回调实现热更新。零外部配置中心依赖。

## 设计哲学

**文件即配置。** 配置保存在本地 TOML 文件，不依赖 etcd / consul / 配置中心。适合单机部署或小规模服务。

**热更新通过 Setter 接口。** 调用方实现 `Setter` 接口来决定配置变更时的行为。使用 `conf.SetterFunc`（类似 `http.HandlerFunc`）可以把普通函数转换为 `Setter`。

## 核心概念

### 配置加载

```
configs/
├── app.toml         → conf.Get("app.toml")
├── database.toml    → conf.Get("database.toml")
└── monitoring.toml  → conf.Get("monitoring.toml")
```

`conf.Init(dir)` 加载目录下所有 `.toml` 文件，文件名作为 key。返回 `*Value`，通过 `UnmarshalTOML` 反序列化。

### 热更新

1. `fsnotify` 检测到文件 Write 或 Create 事件
2. 重新读取文件内容
3. 调用该文件已注册的所有 `Setter.Set(rawText)`
4. Setter 内部 panic 或返回 error 均被捕获，打印到 stderr，不影响其他 Setter

```go
conf.Watch("app.toml", conf.SetterFunc(func(raw string) error {
    var cfg AppConfig
    if _, err := toml.Decode(raw, &cfg); err != nil {
        return err // 校验失败，拒绝更新
    }
    atomic.StorePointer(&globalCfg, unsafe.Pointer(&cfg))
    return nil
}))
```

### 类型

`Value` 封装单个 TOML 文件的路径，提供反序列化方法：

```go
type Value struct { ... }
func (v *Value) UnmarshalTOML(out any) error
```

`Setter` 是热更新回调接口（对齐 blademaster paladin 的 Setter 接口）：

```go
type Setter interface {
    Set(text string) error
}

// SetterFunc 将 func(string) error 转为 Setter（类似 http.HandlerFunc）
type SetterFunc func(text string) error
func (f SetterFunc) Set(text string) error { return f(text) }
```

## 快速开始

```go
import "github.com/eden9th/bedrock/conf"

func main() {
    // 初始化配置目录（Init 内部启动 fsnotify 监听）
    if err := conf.Init("configs/"); err != nil {
        panic(err)
    }
    defer conf.Close()

    // 读取配置
    var cfg AppConfig
    if err := conf.Get("app.toml").UnmarshalTOML(&cfg); err != nil {
        panic(err)
    }

    // 注册热更新回调
    conf.Watch("app.toml", conf.SetterFunc(func(raw string) error {
        var newCfg AppConfig
        if _, err := toml.Decode(raw, &newCfg); err != nil {
            return err // 配置非法，保持旧配置
        }
        updateRuntimeConfig(newCfg)
        return nil
    }))

    // 阻塞运行...
}
```

## API 参考

```go
// 生命周期
func Init(dir string) error                     // 初始化，启动 fsnotify 监听
func Close()                                     // 关闭监听

// 配置获取
func Get(filename string) *Value                 // 获取配置 Value（可用 UnmarshalTOML 反序列化）

// 热更新
func Watch(filename string, s Setter) error      // 注册热更新回调

// 类型
type Setter interface { Set(text string) error }
type SetterFunc func(text string) error
func (f SetterFunc) Set(text string) error

type Value struct { ... }
func (v *Value) UnmarshalTOML(out any) error
```

## 常见问题

### Q: `Init(dir)` 出错返回什么？

`fsnotify.NewWatcher()` 失败或目录监听失败时会返回 error。调用方应检查并处理。

### Q: 文件不存在时 `conf.Get()` 返回什么？

返回一个 `*Value` 指向不存在的文件路径。调用 `UnmarshalTOML()` 时会返回 TOML decode 错误。

### Q: Watch 回调在哪个 goroutine 执行？

在 fsnotify 的事件处理 goroutine 中同步执行。回调应尽快返回；耗时操作在回调内部启动新 goroutine。

### Q: 如何做配置校验？

在 `Setter.Set()` 中做校验。返回 error 即可拒绝更新（日志会打印到 stderr）：

```go
conf.Watch("app.toml", conf.SetterFunc(func(raw string) error {
    var cfg AppConfig
    if _, err := toml.Decode(raw, &cfg); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }
    if cfg.Port <= 0 {
        return fmt.Errorf("invalid port: %d", cfg.Port)
    }
    updateGlobalConfig(cfg)
    return nil
}))
```

### Q: Setter panic 了会怎样？

被 `safeSet` 捕获，打印 panic 信息和堆栈到 stderr，不影响其他 Setter 和程序运行。

## 依赖

- `github.com/BurntSushi/toml` — TOML 解析
- `github.com/fsnotify/fsnotify` — 文件变更监听

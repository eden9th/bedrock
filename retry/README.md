# retry — 指数退避重试

> 通用重试机制：指数退避 + 随机抖动 + 最大尝试次数限制。context 感知，零依赖。

## 设计哲学

**解决 thundering herd。** 多个客户端同时遇到失败时，如果都用固定间隔重试，会在同一时刻同时发起请求，压垮下游。加入随机抖动后，重试时间分散，保护下游恢复。

**简单 > 可配置。** 默认 3 次重试、100ms 起始、±25% 抖动覆盖 90% 场景。需要定制时通过函数式 Option 修改。

## 退避算法

```
delay = min(MaxDelay, BaseDelay × 2^attempt) ± JitterFactor×delay

示例（BaseDelay=100ms, MaxDelay=30s, JitterFactor=0.25）:
  attempt 0（首次失败后）: 100ms ± 25ms   ≈  75~125ms
  attempt 1:                 200ms ± 50ms   ≈ 150~250ms
  attempt 2:                 400ms ± 100ms  ≈ 300~500ms
  ...
```

## 快速开始

```go
import "github.com/eden9th/bedrock/retry"

// 最小用法：默认 3 次，100ms 起始退避
err := retry.Do(ctx, func(ctx context.Context) error {
    resp, err := http.Get("https://api.example.com/data")
    if err != nil {
        return err // 网络错误 → 自动重试
    }
    defer resp.Body.Close()
    if resp.StatusCode == http.StatusTooManyRequests {
        return retry.ErrRetryable // 明确标记可重试
    }
    if resp.StatusCode >= 500 {
        return fmt.Errorf("server error: %d", resp.StatusCode)
    }
    return nil
})

// 自定义配置
err := retry.Do(ctx, fn,
    retry.MaxAttempts(5),
    retry.BaseDelay(500*time.Millisecond),
    retry.MaxDelay(10*time.Second),
    retry.JitterFactor(0.5), // ±50% 抖动
)
```

## API 参考

```go
// 核心函数
func Do(ctx context.Context, fn func(ctx context.Context) error, opts ...Option) error

// 配置选项
func MaxAttempts(n int) Option
func BaseDelay(d time.Duration) Option
func MaxDelay(d time.Duration) Option
func JitterFactor(f float64) Option

// 默认值
func DefaultConfig() Config

// 标记错误
var ErrRetryable error  // 返回此错误必然触发重试
```

## 常见问题

### Q: 如何区分哪些错误应该重试？

在 fn 内部判断：
- 网络错误（timeout、connection refused）→ 重试
- HTTP 429 / 5xx → 重试
- HTTP 4xx（非 429）→ 不重试（客户端错误，重试无意义）
- 业务错误（如"用户名已存在"）→ 不重试

### Q: ctx 超时了会怎样？

`Do` 立即返回 `ctx.Err()`（`context.DeadlineExceeded` 或 `context.Canceled`），不会等到 MaxAttempts 用完。

### Q: 抖动因子应该设多大？

- ±25%（默认）：适合大多数场景
- ±50%：高并发重试场景（缓解 thundering herd）
- 0%：固定间隔重试（不推荐，仅在测试时用）

## 依赖

纯标准库。

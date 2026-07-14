# ratelimit — 令牌桶限流

> 基于令牌桶算法的并发安全限流器。零依赖，纯标准库。

## 设计哲学

**两个核心场景，一个实现。**

| 场景 | 模式 | 方法 |
|------|------|------|
| 服务端保护 | 非阻塞拒绝 | `Allow()` → true/false，false 返回 429 |
| 客户端限速 | 阻塞等待 | `Wait(ctx)` → 等令牌或 ctx 取消 |

## 算法

令牌桶（Token Bucket）：以固定速率往桶中放令牌（最多 `burst` 个），请求时消耗令牌。

```
rate = 100/s, burst = 200

时间 →
令牌数: 0 ──→ 50 ──→ 100 ──→ 150 ──→ 200（上限）
                    ↓ 请求消耗 80
                  20 ──→ 70 ──→ 120 ...
```

## 快速开始

```go
import "github.com/eden9th/bedrock/ratelimit"

// 服务端：每秒 100 请求，burst 200
limiter := ratelimit.New(100, 200)
e.Use(func(c *bm.Context) {
    if !limiter.Allow() {
        c.AbortWithStatus(429)
        return
    }
    c.Next()
})

// 客户端：遵守外部 API 的 10 req/s 限制
limiter := ratelimit.New(10, 10)
for _, item := range items {
    if err := limiter.Wait(ctx); err != nil {
        return err // ctx 取消
    }
    callExternalAPI(item)
}
```

## API 参考

```go
func New(rate, burst float64) *Limiter

func (l *Limiter) Allow() bool                        // 非阻塞获取 1 个令牌
func (l *Limiter) AllowN(n float64) bool               // 非阻塞获取 n 个令牌
func (l *Limiter) Wait(ctx context.Context) error      // 阻塞等待 1 个令牌
func (l *Limiter) WaitN(ctx context.Context, n float64) error // 阻塞等待 n 个令牌
func (l *Limiter) Tokens() float64                     // 当前可用令牌数
func (l *Limiter) Rate() float64                       // 令牌生成速率
```

## 常见问题

### Q: rate 和 burst 怎么设？

- **服务端保护**：rate = 正常 QPS 的 2x，burst = rate 的 1.5~2x。例如正常 100 QPS → New(200, 350)
- **客户端限速**：rate = 外部 API 的限制，burst = rate。例如限制 10/s → New(10, 10)

### Q: Wait 会精确等待吗？

使用 `time.After` 实现。对于小于 1ms 的等待时间自动退化为 1ms 等待以避免 busy-loop。高精度限流场景可考虑 `golang.org/x/time/rate`。

### Q: 多个 goroutine 共享一个 Limiter 安全吗？

是。所有方法持有 `sync.Mutex`。令牌补充逻辑在锁内执行。

## 依赖

纯标准库。

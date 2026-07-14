// Package retry 提供带指数退避和随机抖动的通用重试机制。
//
// # 核心设计
//
// 退避策略：backoff = min(MaxDelay, BaseDelay * 2^attempt)
// 抖动范围：backoff ± JitterFactor * backoff（默认 25%）
// 用于防止 thundering herd 问题——多个客户端同时重试时不会在同一个时间点发起请求。
//
// # 使用示例
//
//	// 默认配置：最多 3 次，100ms 起始，30s 上限
//	err := retry.Do(ctx, func(ctx context.Context) error {
//	    resp, err := http.Get("https://api.example.com/data")
//	    if err != nil { return err }         // 网络错误 → 重试
//	    if resp.StatusCode == 429 {           // 限流 → 重试
//	        return retry.ErrRetryable
//	    }
//	    return nil                             // 成功
//	})
//
//	// 自定义配置
//	err := retry.Do(ctx, fn,
//	    retry.MaxAttempts(5),
//	    retry.BaseDelay(500*time.Millisecond),
//	    retry.MaxDelay(10*time.Second),
//	)
//
// # 重试判定
//
//   - 返回 nil → 成功，停止重试
//   - 返回 error → 重试（记录日志后继续）
//   - context 取消 → 立即停止，返回 ctx.Err()
//   - 达到 MaxAttempts → 返回最后一次 error
package retry

import (
	"context"
	"math/rand"
	"time"
)

// ErrRetryable 是一个标记错误。函数返回此错误时必然触发重试。
// 消费方可使用 errors.Is(err, ErrRetryable) 判断是否需要重试。
var ErrRetryable = &retryableError{}

type retryableError struct{}

func (e *retryableError) Error() string { return "retryable error" }

// ─── Config ──────────────────────────────────────────────────────────────────

// Config 控制重试行为。零值不可用，使用 DefaultConfig 或 WithXxx 选项。
type Config struct {
	MaxAttempts  int           // 最大尝试次数（含首次），默认 3
	BaseDelay    time.Duration // 首次重试延迟，默认 100ms
	MaxDelay     time.Duration // 最大退避上限，默认 30s
	JitterFactor float64       // 抖动因子 [0, 1]，默认 0.25（±25%）
}

// DefaultConfig 返回推荐默认配置：最多 3 次，100ms → 400ms → 1600ms 退避。
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		BaseDelay:    100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		JitterFactor: 0.25,
	}
}

// Option 是函数式配置项
type Option func(*Config)

// MaxAttempts 设置最大尝试次数（含首次调用）
func MaxAttempts(n int) Option { return func(c *Config) { c.MaxAttempts = n } }

// BaseDelay 设置首次重试的基础延迟
func BaseDelay(d time.Duration) Option { return func(c *Config) { c.BaseDelay = d } }

// MaxDelay 设置退避延迟的上限
func MaxDelay(d time.Duration) Option { return func(c *Config) { c.MaxDelay = d } }

// JitterFactor 设置随机抖动比例 [0, 1]
func JitterFactor(f float64) Option { return func(c *Config) { c.JitterFactor = f } }

// ─── Do ──────────────────────────────────────────────────────────────────────

// Do 执行带重试的函数 fn。
//
// 每次尝试后检查：如果 fn 返回 nil，立即返回成功。
// 如果 fn 返回 error，按退避策略等待后重试，直到达到 MaxAttempts
// 或 context 被取消。
//
// 返回最后一次 fn 的错误（或 context.Canceled / context.DeadlineExceeded）。
func Do(ctx context.Context, fn func(ctx context.Context) error, opts ...Option) error {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}
		lastErr = err

		// 最后一次尝试后不再等待
		if attempt >= cfg.MaxAttempts-1 {
			break
		}

		// 计算退避延迟
		delay := backoff(cfg, attempt)

		// 等待：context 取消或延迟到期
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return lastErr
}

// backoff 计算第 attempt 次重试的退避延迟。
// attempt 从 0 开始（0 = 首次失败后的重试延迟）。
func backoff(cfg Config, attempt int) time.Duration {
	// 指数退避: baseDelay * 2^attempt
	exp := time.Duration(1 << attempt) // 2^attempt
	delay := cfg.BaseDelay * exp
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// 添加随机抖动: [-jitterFactor, +jitterFactor]
	if cfg.JitterFactor > 0 {
		jitterRange := float64(delay) * cfg.JitterFactor
		jitter := time.Duration(rand.Float64()*2*jitterRange - jitterRange)
		delay += jitter
	}

	if delay < 0 {
		delay = 0
	}
	return delay
}

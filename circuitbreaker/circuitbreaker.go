// Package circuitbreaker 提供三态熔断器（closed → open → half-open）。
//
// # 核心设计
//
// 熔断器保护下游服务：当失败率超过阈值时自动"断开"（open），
// 直接快速失败而不调用下游。一段时间后进入半开（half-open）状态，
// 放行少量请求探测下游是否恢复。探测成功则闭合（closed），失败则重新断开。
//
// # 三态转换
//
//	          ┌──────────┐
//	          │  closed  │ ← 正常状态，统计成功/失败
//	          └────┬─────┘
//	               │ 失败率 >= FailureThreshold && 请求数 >= MinRequests
//	               ▼
//	          ┌──────────┐
//	          │   open   │ ← 拒绝所有请求，快速失败（返回 ErrOpen）
//	          └────┬─────┘
//	               │ 经过 OpenDuration 后
//	               ▼
//	          ┌──────────┐
//	          │half-open │ ← 放行探测请求
//	          └────┬─────┘
//	   探测成功     │     探测失败
//	  ┌────────────┼────────────┐
//	  ▼            │            ▼
//	closed          │          open（重新断开）
//
// # 使用示例
//
//	cb := circuitbreaker.New("redis",
//	    circuitbreaker.FailureThreshold(0.5),
//	    circuitbreaker.OpenDuration(30*time.Second),
//	)
//
//	err := cb.Do(ctx, func(ctx context.Context) error {
//	    return redis.Ping(ctx).Err()
//	})
//	if errors.Is(err, circuitbreaker.ErrOpen) {
//	    // 熔断器断开，使用降级逻辑
//	}
//
// # 线程安全
//
// 所有方法并发安全。
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrOpen 表示熔断器处于断开状态，请求被拒绝。
var ErrOpen = errors.New("circuitbreaker: circuit is open")

// State 表示熔断器当前状态。
type State int

const (
	StateClosed   State = iota // 闭合：正常执行
	StateOpen                  // 断开：快速失败
	StateHalfOpen              // 半开：探测恢复
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config 熔断器配置。零值不可用，使用 DefaultConfig 或 New。
type Config struct {
	// FailureThreshold 失败率阈值 [0, 1]。达到此比例时从 closed → open。
	// 默认 0.5（50%）。
	FailureThreshold float64

	// OpenDuration 熔断器保持 open 状态的时间，之后进入 half-open。
	// 默认 30s。
	OpenDuration time.Duration

	// HalfOpenMaxRequests half-open 状态下允许放行的最大探测请求数。
	// 默认 1。
	HalfOpenMaxRequests int

	// MinRequests 统计窗口内的最少请求数。少于此时不触发熔断。
	// 默认 10。防止低流量下偶然失败触发熔断。
	MinRequests int

	// WindowDuration 滑动窗口的时间长度。窗口外的数据被丢弃。
	// 默认 60s。
	WindowDuration time.Duration
}

// DefaultConfig 返回推荐默认配置。
func DefaultConfig() Config {
	return Config{
		FailureThreshold:    0.5,
		OpenDuration:        30 * time.Second,
		HalfOpenMaxRequests: 1,
		MinRequests:         10,
		WindowDuration:      60 * time.Second,
	}
}

// Option 函数式配置项。
type Option func(*Config)

// FailureThreshold 设置失败率阈值。
func FailureThreshold(v float64) Option { return func(c *Config) { c.FailureThreshold = v } }

// OpenDuration 设置断开持续时间。
func OpenDuration(d time.Duration) Option { return func(c *Config) { c.OpenDuration = d } }

// HalfOpenMaxRequests 设置半开状态最大探测请求数。
func HalfOpenMaxRequests(n int) Option { return func(c *Config) { c.HalfOpenMaxRequests = n } }

// MinRequests 设置统计最少请求数。
func MinRequests(n int) Option { return func(c *Config) { c.MinRequests = n } }

// WindowDuration 设置滑动窗口时长。
func WindowDuration(d time.Duration) Option { return func(c *Config) { c.WindowDuration = d } }

// ─── CircuitBreaker ─────────────────────────────────────────────────────────

// CircuitBreaker 是三态熔断器。并发安全。
type CircuitBreaker struct {
	name   string
	config Config

	mu    sync.Mutex
	state State

	// 滑动窗口中的请求记录（成功/失败时间戳）
	successes []time.Time
	failures  []time.Time

	// open 状态的过期时间
	openUntil time.Time

	// half-open 状态已放行的请求计数
	halfOpenCount int32
}

// New 创建熔断器。name 用于日志/指标标识。
func New(name string, opts ...Option) *CircuitBreaker {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	return &CircuitBreaker{
		name:   name,
		config: cfg,
		state:  StateClosed,
	}
}

// Name 返回熔断器名称。
func (cb *CircuitBreaker) Name() string { return cb.name }

// State 返回当前状态。
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transition()
	return cb.state
}

// Do 执行 fn。熔断器断开时直接返回 ErrOpen。
// fn 返回 nil 视为成功，返回 error 视为失败。
func (cb *CircuitBreaker) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	cb.mu.Lock()
	cb.transition()

	switch cb.state {
	case StateOpen:
		cb.mu.Unlock()
		return ErrOpen

	case StateHalfOpen:
		// 半开状态：限制放行数量
		if atomic.AddInt32(&cb.halfOpenCount, 1) > int32(cb.config.HalfOpenMaxRequests) {
			atomic.AddInt32(&cb.halfOpenCount, -1)
			cb.mu.Unlock()
			return ErrOpen
		}
		cb.mu.Unlock()

		err := fn(ctx)

		cb.mu.Lock()
		if err == nil {
			// 探测成功 → 闭合
			cb.toClosed()
		} else {
			// 探测失败 → 重新断开
			cb.toOpen()
		}
		atomic.StoreInt32(&cb.halfOpenCount, 0)
		cb.mu.Unlock()
		return err

	default: // StateClosed
		cb.mu.Unlock()

		err := fn(ctx)

		cb.mu.Lock()
		now := time.Now()
		if err == nil {
			cb.successes = append(cb.successes, now)
		} else {
			cb.failures = append(cb.failures, now)
		}
		cb.pruneWindow(now)
		cb.checkThreshold(now)
		cb.mu.Unlock()
		return err
	}
}

// ─── 内部状态转换 ────────────────────────────────────────────────────────────

// transition 检查并执行状态转换（需持有锁）。
func (cb *CircuitBreaker) transition() {
	switch cb.state {
	case StateOpen:
		if time.Now().After(cb.openUntil) {
			cb.state = StateHalfOpen
			atomic.StoreInt32(&cb.halfOpenCount, 0)
		}
	}
}

// toClosed 将熔断器闭合（需持有锁）。
func (cb *CircuitBreaker) toClosed() {
	cb.state = StateClosed
	cb.successes = nil
	cb.failures = nil
}

// toOpen 将熔断器断开（需持有锁）。
func (cb *CircuitBreaker) toOpen() {
	cb.state = StateOpen
	cb.openUntil = time.Now().Add(cb.config.OpenDuration)
	cb.successes = nil
	cb.failures = nil
}

// pruneWindow 清理窗口外的旧记录（需持有锁）。
func (cb *CircuitBreaker) pruneWindow(now time.Time) {
	cutoff := now.Add(-cb.config.WindowDuration)

	cb.successes = pruneOld(cb.successes, cutoff)
	cb.failures = pruneOld(cb.failures, cutoff)
}

func pruneOld(times []time.Time, cutoff time.Time) []time.Time {
	for i, t := range times {
		if t.After(cutoff) {
			return times[i:]
		}
	}
	return nil
}

// checkThreshold 检查统计是否达到熔断阈值（需持有锁）。
func (cb *CircuitBreaker) checkThreshold(now time.Time) {
	total := len(cb.successes) + len(cb.failures)
	if total < cb.config.MinRequests {
		return
	}

	failRate := float64(len(cb.failures)) / float64(total)
	if failRate >= cb.config.FailureThreshold {
		cb.toOpen()
	}
}

// Stats 返回当前统计信息（用于监控）。
type Stats struct {
	Name         string  `json:"name"`
	State        string  `json:"state"`
	TotalCount   int     `json:"total_count"`
	SuccessCount int     `json:"success_count"`
	FailureCount int     `json:"failure_count"`
	FailRate     float64 `json:"fail_rate"`
}

// Stats 返回当前熔断器统计快照。
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transition()
	now := time.Now()
	cb.pruneWindow(now)

	total := len(cb.successes) + len(cb.failures)
	var failRate float64
	if total > 0 {
		failRate = float64(len(cb.failures)) / float64(total)
	}

	return Stats{
		Name:         cb.name,
		State:        cb.state.String(),
		TotalCount:   total,
		SuccessCount: len(cb.successes),
		FailureCount: len(cb.failures),
		FailRate:     failRate,
	}
}

// Reset 手动重置熔断器到闭合状态。用于测试或运维干预。
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.toClosed()
}

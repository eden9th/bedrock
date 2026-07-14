// Package ratelimit 提供基于令牌桶算法的并发安全限流器。
//
// # 核心设计
//
// 令牌桶（Token Bucket）算法：以固定速率生成令牌到桶中（最多 burst 个）。
// 每次请求消耗一个令牌。令牌不足时拒绝（Allow 返回 false）或等待（Wait 阻塞）。
//
// 两个核心场景：
//   - 服务端保护：限流 API 调用量，防止单一客户端打爆服务
//   - 客户端限速：遵守外部 API 的 rate limit（如"每秒最多 10 次"）
//
// # 使用示例
//
//	// 服务端：每秒 100 个请求，允许 burst 到 200
//	limiter := ratelimit.New(100, 200)
//	if !limiter.Allow() {
//	    http.Error(w, "rate limit exceeded", 429)
//	    return
//	}
//
//	// 客户端：遵守外部 API 的限速，阻塞等待
//	limiter := ratelimit.New(10, 10) // 10 req/s
//	for _, item := range items {
//	    if err := limiter.Wait(ctx); err != nil {
//	        return err // context 取消
//	    }
//	    callExternalAPI(item)
//	}
//
// # 线程安全
//
// 所有方法并发安全。
package ratelimit

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// ErrExceedsBurst 表示一次请求的令牌数超过桶容量，等待也无法满足。
var ErrExceedsBurst = errors.New("ratelimit: requested tokens exceed burst")

// Limiter 是令牌桶限流器。并发安全。
// 零值不可用，使用 New 创建。
type Limiter struct {
	mu sync.Mutex

	rate   float64   // 令牌生成速率（个/秒）
	burst  float64   // 桶容量上限
	tokens float64   // 当前令牌数
	last   time.Time // 上次补充令牌的时间
}

// New 创建令牌桶限流器。
//
//	rate:  每秒生成的令牌数
//	burst: 桶的最大容量（允许的瞬时并发峰值）
//
// burst 应 >= 1。rate 和 burst 都会被取 math.Ceil 向上取整。
func New(rate, burst float64) *Limiter {
	rate = math.Ceil(rate)
	if rate <= 0 {
		rate = 1
	}
	burst = math.Ceil(burst)
	if burst <= 0 {
		burst = 1
	}
	return &Limiter{
		rate:   rate,
		burst:  burst,
		tokens: burst,
		last:   time.Now(),
	}
}

// Allow 非阻塞地尝试获取一个令牌。返回 true 表示获取成功。
// 适合服务端请求拦截场景。
func (l *Limiter) Allow() bool {
	return l.AllowN(1)
}

// AllowN 非阻塞地尝试获取 n 个令牌。
func (l *Limiter) AllowN(n float64) bool {
	if n <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.refill()
	if l.tokens >= n {
		l.tokens -= n
		return true
	}
	return false
}

// Wait 阻塞等待直到获取一个令牌或 context 取消。
// 适合客户端限速场景。
func (l *Limiter) Wait(ctx context.Context) error {
	return l.WaitN(ctx, 1)
}

// WaitN 阻塞等待直到获取 n 个令牌或 context 取消。
func (l *Limiter) WaitN(ctx context.Context, n float64) error {
	if n <= 0 {
		return nil
	}
	if n > l.burst {
		return ErrExceedsBurst
	}
	for {
		if l.AllowN(n) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 计算需要等待的时间
		l.mu.Lock()
		l.refill()
		need := n - l.tokens
		waitDuration := time.Duration(need / l.rate * float64(time.Second))
		l.mu.Unlock()

		// 最小等待 1ms 避免 busy-loop
		if waitDuration < time.Millisecond {
			waitDuration = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
		}
	}
}

// Tokens 返回当前可用的令牌数（近似值，非原子快照）。
func (l *Limiter) Tokens() float64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refill()
	return l.tokens
}

// Rate 返回令牌生成速率。
func (l *Limiter) Rate() float64 {
	return l.rate
}

// refill 根据时间流逝补充令牌（需持有锁）。
func (l *Limiter) refill() {
	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.tokens += elapsed * l.rate
	if l.tokens > l.burst {
		l.tokens = l.burst
	}
	l.last = now
}

// ─── KeyLimiter ─────────────────────────────────────────────────────────────

// KeyLimiter 提供按 key（如 IP、用户ID）分桶的限流器。
// 内置 TTL 清理，避免 key 无限增长导致内存泄漏。
//
// 使用示例：
//
//	kl := ratelimit.NewKeyLimiter(10, 10, 5*time.Minute)
//	if kl.Allow("192.168.1.1") { ... }
//	if err := kl.Wait(ctx, "user_123"); err != nil { ... }
type KeyLimiter struct {
	rate  float64
	burst float64
	ttl   time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
	done    chan struct{} // 关闭后停止后台清理 goroutine
	closed  bool
}

// bucket 是 KeyLimiter 内部的带过期时间的令牌桶。
type bucket struct {
	limiter  *Limiter
	lastUsed time.Time
}

// NewKeyLimiter 创建按 key 分桶的限流器。
//
//	rate:  每秒生成的令牌数（每个 key 独立）
//	burst: 每个 key 的桶容量
//	ttl:   key 未被使用后的过期时间。过期后自动从内存中清理。
func NewKeyLimiter(rate, burst float64, ttl time.Duration) *KeyLimiter {
	rate = math.Ceil(rate)
	if rate <= 0 {
		rate = 1
	}
	burst = math.Ceil(burst)
	if burst <= 0 {
		burst = 1
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	kl := &KeyLimiter{
		rate:    rate,
		burst:   burst,
		ttl:     ttl,
		buckets: make(map[string]*bucket),
		done:    make(chan struct{}),
	}

	go kl.cleanup()
	return kl
}

// Allow 非阻塞地尝试获取指定 key 的一个令牌。
func (kl *KeyLimiter) Allow(key string) bool {
	return kl.getLimiter(key).Allow()
}

// Wait 阻塞等待直到获取指定 key 的一个令牌或 context 取消。
func (kl *KeyLimiter) Wait(ctx context.Context, key string) error {
	return kl.getLimiter(key).Wait(ctx)
}

// Tokens 返回指定 key 的当前可用令牌数。
func (kl *KeyLimiter) Tokens(key string) float64 {
	return kl.getLimiter(key).Tokens()
}

// getLimiter 获取或创建指定 key 的令牌桶。
func (kl *KeyLimiter) getLimiter(key string) *Limiter {
	kl.mu.Lock()
	defer kl.mu.Unlock()

	b, ok := kl.buckets[key]
	if ok {
		b.lastUsed = time.Now()
		return b.limiter
	}

	l := New(kl.rate, kl.burst)
	kl.buckets[key] = &bucket{limiter: l, lastUsed: time.Now()}
	return l
}

// cleanup 定期清理过期的 key。作为后台 goroutine 运行，Close() 时停止。
func (kl *KeyLimiter) cleanup() {
	ticker := time.NewTicker(kl.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-kl.done:
			return
		case <-ticker.C:
			kl.mu.Lock()
			now := time.Now()
			for key, b := range kl.buckets {
				if now.Sub(b.lastUsed) > kl.ttl {
					delete(kl.buckets, key)
				}
			}
			kl.mu.Unlock()
		}
	}
}

// Close 停止后台清理 goroutine。多次调用安全。
func (kl *KeyLimiter) Close() {
	kl.mu.Lock()
	defer kl.mu.Unlock()
	if !kl.closed {
		kl.closed = true
		close(kl.done)
	}
}

// Count 返回当前活跃 key 数量（用于监控）。
func (kl *KeyLimiter) Count() int {
	kl.mu.Lock()
	defer kl.mu.Unlock()
	return len(kl.buckets)
}

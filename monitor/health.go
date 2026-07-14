package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/eden9th/bedrock/bm"
)

// ─── HealthChecker ───────────────────────────────────────────────────────────

// HealthChecker 是健康检查的抽象接口。
// 每个实现负责检查一个外部依赖或内部组件的健康状态。
//
// 实现约定：
//   - 检查应设置合理的超时（通过 ctx 传入）
//   - 返回 error 表示不健康，nil 表示健康
//   - 尽量轻量（如 ping），避免耗时操作
//   - 必须是并发安全的（Check 可能被并发调用）
type HealthChecker interface {
	// Name 返回此检查的唯一名称，如 "postgres"、"redis"
	Name() string
	// Check 执行健康检查，返回 nil 表示健康
	Check(ctx context.Context) error
}

// ─── HealthRegistry ──────────────────────────────────────────────────────────

// HealthRegistry 管理一组 HealthChecker，支持并发执行和结果聚合。
//
// 使用示例：
//
//	hr := monitor.NewHealthRegistry()
//	hr.Register(&PostgresChecker{db})
//	hr.Register(&RedisChecker{client})
//
//	// 作为 bm handler
//	e.GET("/health", monitor.HealthHandler(hr))
//
//	// 代码中检查
//	result := hr.CheckAll(context.Background())
//	fmt.Println(result.Healthy) // true/false
type HealthRegistry struct {
	mu      sync.RWMutex
	checkers map[string]HealthChecker
}

// NewHealthRegistry 创建空的 HealthRegistry。
func NewHealthRegistry() *HealthRegistry {
	return &HealthRegistry{
		checkers: make(map[string]HealthChecker),
	}
}

// Register 注册一个健康检查器。name 冲突时返回错误。
func (hr *HealthRegistry) Register(c HealthChecker) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if _, ok := hr.checkers[c.Name()]; ok {
		return fmt.Errorf("monitor: health checker %q already registered", c.Name())
	}
	hr.checkers[c.Name()] = c
	return nil
}

// Unregister 移除指定名称的健康检查器。
func (hr *HealthRegistry) Unregister(name string) {
	hr.mu.Lock()
	delete(hr.checkers, name)
	hr.mu.Unlock()
}

// CheckAll 并发执行所有已注册的健康检查，返回聚合结果。
// 每个检查使用独立的 ctx 超时控制（从父 ctx 派生，默认超时 2s）。
// 只要有一个不健康，Healthy 就是 false。
func (hr *HealthRegistry) CheckAll(ctx context.Context) HealthResult {
	hr.mu.RLock()
	names := make([]string, 0, len(hr.checkers))
	for name := range hr.checkers {
		names = append(names, name)
	}
	hr.mu.RUnlock()

	if len(names) == 0 {
		return HealthResult{Healthy: true, Checks: map[string]HealthCheckResult{}}
	}

	type checkResult struct {
		name   string
		err    error
		elapsed time.Duration
	}

	ch := make(chan checkResult, len(names))
	var wg sync.WaitGroup

	for _, name := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			hr.mu.RLock()
			checker, ok := hr.checkers[name]
			hr.mu.RUnlock()

			if !ok {
				ch <- checkResult{name: name, err: fmt.Errorf("checker removed")}
				return
			}

			checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			start := time.Now()
			err := checker.Check(checkCtx)
			ch <- checkResult{name: name, err: err, elapsed: time.Since(start)}
		}(name)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	result := HealthResult{Checks: make(map[string]HealthCheckResult, len(names))}
	allHealthy := true

	for r := range ch {
		healthy := r.err == nil
		if !healthy {
			allHealthy = false
		}
		msg := "ok"
		if r.err != nil {
			msg = r.err.Error()
		}
		result.Checks[r.name] = HealthCheckResult{
			Healthy: healthy,
			Message: msg,
			Elapsed: r.elapsed.String(),
		}
	}

	result.Healthy = allHealthy
	return result
}

// ─── 结果类型 ────────────────────────────────────────────────────────────────

// HealthResult 是 CheckAll 的聚合返回结果。
type HealthResult struct {
	Healthy bool                        // 全部健康为 true
	Checks  map[string]HealthCheckResult // name → 各检查结果
}

// HealthCheckResult 是单个检查的结果。
type HealthCheckResult struct {
	Healthy bool   `json:"healthy"`
	Message string `json:"message"`
	Elapsed string `json:"elapsed"`
}

// ─── HTTP Handler ────────────────────────────────────────────────────────────

// HealthHandler 返回一个 bm.HandlerFunc，用于暴露健康检查端点。
//
// 响应示例：
//
//	HTTP 200: {"healthy":true,"checks":{"postgres":{"healthy":true,"message":"ok","elapsed":"1.2ms"}}}
//	HTTP 503: {"healthy":false,"checks":{"redis":{"healthy":false,"message":"dial timeout","elapsed":"2.0s"}}}
func HealthHandler(hr *HealthRegistry) bm.HandlerFunc {
	return func(c *bm.Context) {
		result := hr.CheckAll(c.Request.Context())

		c.Header("Content-Type", "application/json; charset=utf-8")
		status := http.StatusOK
		if !result.Healthy {
			status = http.StatusServiceUnavailable
		}
		c.Writer.WriteHeader(status)
		_ = json.NewEncoder(c.Writer).Encode(result)
	}
}

// HealthHandlerFunc 返回标准库 http.HandlerFunc（不依赖 bm）。
func HealthHandlerFunc(hr *HealthRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := hr.CheckAll(r.Context())

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		status := http.StatusOK
		if !result.Healthy {
			status = http.StatusServiceUnavailable
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(result)
	}
}

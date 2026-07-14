# CHANGELOG — retry

---

## [0.1.0] — 2026-07-14

### Added

- Do(ctx, fn, opts...) — 带指数退避和随机抖动的重试执行
- DefaultConfig() — 推荐默认配置（3 次，100ms 起始，±25% 抖动）
- 函数式 Option：MaxAttempts / BaseDelay / MaxDelay / JitterFactor
- ErrRetryable 标记错误
- context 感知：ctx 取消立即终止

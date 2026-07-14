# CHANGELOG — ratelimit

---

## [0.1.0] — 2026-07-14

### Added

- New(rate, burst) — 创建令牌桶限流器
- Allow() / AllowN(n) — 非阻塞获取令牌
- Wait(ctx) / WaitN(ctx, n) — 阻塞等待令牌，context 感知
- Tokens() — 当前可用令牌数
- Rate() — 令牌生成速率
- 并发安全（sync.Mutex）

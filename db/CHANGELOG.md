# CHANGELOG — db

---

## [0.2.0] — 2026-07-14

### Added

- HealthChecker.RegisterPoolMetrics(r) — 开箱即用注册 5 个连接池 Gauge 指标
- HealthChecker.CollectPoolMetrics() — 采集并更新连接池指标

---

## [0.1.0] — 2026-07-14

### Added

- Query(ctx, db, *[]T, sql, args...) — 多行结构体扫描
- QueryRow(ctx, db, *T, sql, args...) — 单行结构体扫描
- Exec(ctx, db, sql, args...) — 写操作执行
- Transact(ctx, db, fn) — 事务 helper（auto rollback）
- HealthChecker — 实现 monitor.HealthChecker 接口
- StatsSnapshot() — 连接池统计快照
- 列名映射：db tag 优先，自动 CamelCase → snake_case 转换
- Queryable / Execable 接口抽象（支持 *sql.DB / *sql.Tx / *sql.Conn）

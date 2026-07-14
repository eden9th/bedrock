# db — 轻量数据库查询助手

> 消除 `rows.Scan(&a, &b, &c, ...)` 的样板代码。自动将 SQL 列名映射到 Go struct 字段，零外部 ORM 依赖。

## 设计哲学

**不是 ORM，只是 Scan 的更优雅写法。** 不生成 SQL、不做 migration、不做关系映射。唯一做的事：把 `SELECT` 的行自动映射到 Go struct。

**关键取舍：**
- ✅ 自动列名映射（snake_case column → CamelCase field / db tag）
- ✅ 事务 helper（auto rollback on error）
- ✅ 连接池健康检查（对接 monitor.HealthChecker）
- ❌ 不生成 SQL
- ❌ 不做 migration
- ❌ 不做关联查询

## 快速开始

```go
import "github.com/eden9th/bedrock/db"

type User struct {
    ID        int64  `db:"id"`
    Name      string `db:"name"`
    CreatedAt string `db:"created_at"` // db tag 优先
}

// 查询多行
var users []User
if err := db.Query(ctx, pool, &users,
    "SELECT id, name, created_at FROM users WHERE status = ?", "active"); err != nil {
    return err
}

// 查询单行
var user User
err := db.QueryRow(ctx, pool, &user,
    "SELECT id, name, created_at FROM users WHERE id = ?", uid)
if errors.Is(err, sql.ErrNoRows) {
    // 未找到
}

// 事务
err := db.Transact(ctx, pool, func(tx *sql.Tx) error {
    _, err := tx.ExecContext(ctx, "UPDATE users SET name = ? WHERE id = ?", "new", uid)
    return err
})

// 健康检查（对接 monitor 包）
hc := db.NewHealthChecker(pool, "postgres")
hr.Register(hc) // hc 实现 monitor.HealthChecker
```

## API 参考

```go
// 查询
func Query(ctx, db, dest, query, args...) error        // dest: *[]T
func QueryRow(ctx, db, dest, query, args...) error      // dest: *T

// 写入
func Exec(ctx, db, query, args...) (sql.Result, error)

// 事务
func Transact(ctx, db *sql.DB, fn func(tx *sql.Tx) error) error

// 健康检查
func NewHealthChecker(db *sql.DB, name string) *HealthChecker
func (h *HealthChecker) Name() string
func (h *HealthChecker) Check(ctx) error
func (h *HealthChecker) StatsSnapshot() StatsMetrics
```

## 列名映射规则

1. **db tag 优先**：`db:"column_name"` → column_name
2. **自动 snake_case**：`UserID` → `user_id`，`CreatedAt` → `created_at`
3. **无匹配的列被丢弃**：SQL 返回了但 struct 中没有的列，静默忽略
4. **无匹配的字段为零值**：struct 有但 SQL 没返回的字段，保持零值

## 常见问题

### Q: 和 sqlx 的区别？

sqlx 功能更全（命名参数、In() 展开、结构体扫描）。db 包更轻量（~200 行 vs sqlx ~3000 行），只覆盖最常见的 Query + QueryRow + Exec + Transact。如果后续需要命名参数等功能，直接升级到 sqlx 即可——API 设计上保持了兼容。

### Q: 如何处理 NULL 字段？

使用 `sql.NullString` / `sql.NullInt64` 等类型：

```go
type User struct {
    ID    int64
    Name  string
    Bio   sql.NullString // 可为 NULL
}
```

### Q: 支持哪些数据库？

任何实现了 `database/sql` 驱动的数据库（PostgreSQL、MySQL、SQLite 等）。`HealthChecker` 使用 `PingContext`，`StatsSnapshot` 使用 `sql.DB.Stats()`——两者都是标准库接口。

## 依赖

- `database/sql`（标准库）

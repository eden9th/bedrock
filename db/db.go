// Package db 提供基于 database/sql 的轻量查询助手。
//
// # 核心设计
//
// 不是 ORM。只解决一个痛点：消除 rows.Scan(&a, &b, &c, ...) 的样板代码。
// 自动将 SQL 列名映射到 Go struct 字段（snake_case → CamelCase，或使用 db tag）。
//
// # 使用示例
//
//	type User struct {
//	    ID   int64  `db:"id"`
//	    Name string `db:"name"`
//	}
//
//	// 查询多行
//	var users []User
//	if err := db.Query(ctx, pool, &users,
//	    "SELECT id, name FROM users WHERE status = ?", "active"); err != nil {
//	    return err
//	}
//
//	// 查询单行
//	var user User
//	if err := db.QueryRow(ctx, pool, &user,
//	    "SELECT id, name FROM users WHERE id = ?", uid); err != nil {
//	    return err
//	}
//
//	// 执行写操作
//	result, err := db.Exec(ctx, pool,
//	    "INSERT INTO users (name) VALUES (?)", "alice")
//
//	// 事务
//	err := db.Transact(ctx, pool, func(tx *sql.Tx) error {
//	    _, err := tx.ExecContext(ctx, "UPDATE ...")
//	    return err
//	})
//
//	// 健康检查（实现 monitor.HealthChecker）
//	hc := db.NewHealthChecker(pool, "postgres")
//	hr.Register(hc)
//
// # 列名映射
//
//   - 优先使用 struct tag `db:"column_name"`
//   - 无 tag 时自动将字段名转为 snake_case：UserID → user_id, CreatedAt → created_at
package db

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/eden9th/bedrock/monitor"
)

// ─── Column Mapping ──────────────────────────────────────────────────────────

// columnMap 建立 SQL 列名 → struct 字段索引的映射。
// columns 来自 rows.Columns()，elemType 是目标切片的元素类型。
func columnMap(columns []string, elemType reflect.Type) ([]int, error) {
	// 构建字段索引：tag > snake_case(fieldName)
	tagIndex := make(map[string]int)
	snakeIndex := make(map[string]int)

	for i := 0; i < elemType.NumField(); i++ {
		f := elemType.Field(i)
		if !f.IsExported() {
			continue
		}
		if tag := f.Tag.Get("db"); tag != "" {
			tagIndex[tag] = i
		}
		snakeIndex[toSnakeCase(f.Name)] = i
	}

	indices := make([]int, len(columns))
	for i, col := range columns {
		if idx, ok := tagIndex[col]; ok {
			indices[i] = idx
		} else if idx, ok := snakeIndex[col]; ok {
			indices[i] = idx
		} else {
			indices[i] = -1 // 列在 struct 中无对应字段
		}
	}
	return indices, nil
}

// scanRow 将一行数据扫描到 struct 中。
func scanRow(rows *sql.Rows, indices []int, elemType reflect.Type) (reflect.Value, error) {
	val := reflect.New(elemType).Elem()

	// 构建扫描目标（仅扫描有映射的列）
	cols, _ := rows.Columns()
	targets := make([]any, len(cols))
	for i, idx := range indices {
		if idx >= 0 {
			targets[i] = val.Field(idx).Addr().Interface()
		} else {
			var discard any
			targets[i] = &discard
		}
	}

	if err := rows.Scan(targets...); err != nil {
		return reflect.Value{}, err
	}
	return val, nil
}

// ─── Query ───────────────────────────────────────────────────────────────────

// Query 执行查询并将所有行扫描到 dest（*[]T）中。
// dest 必须是 *[]T 类型，其中 T 是一个 struct。
func Query(ctx context.Context, db Queryable, dest any, query string, args ...any) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("db: dest must be *[]T, got %T", dest)
	}
	sliceVal := destVal.Elem()
	elemType := sliceVal.Type().Elem()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("db: query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("db: columns: %w", err)
	}

	indices, err := columnMap(columns, elemType)
	if err != nil {
		return err
	}

	for rows.Next() {
		val, err := scanRow(rows, indices, elemType)
		if err != nil {
			return fmt.Errorf("db: scan: %w", err)
		}
		sliceVal = reflect.Append(sliceVal, val)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("db: rows: %w", err)
	}

	destVal.Elem().Set(sliceVal)
	return nil
}

// ─── QueryRow ────────────────────────────────────────────────────────────────

// QueryRow 执行查询并将单行扫描到 dest（*T）中。
// 没有数据时返回 sql.ErrNoRows。
func QueryRow(ctx context.Context, db Queryable, dest any, query string, args ...any) error {
	destVal := reflect.ValueOf(dest)
	if destVal.Kind() != reflect.Ptr || destVal.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("db: dest must be *T (struct), got %T", dest)
	}
	elemType := destVal.Elem().Type()

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("db: query row: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return fmt.Errorf("db: rows: %w", err)
		}
		return sql.ErrNoRows
	}

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("db: columns: %w", err)
	}

	indices, err := columnMap(columns, elemType)
	if err != nil {
		return err
	}

	val, err := scanRow(rows, indices, elemType)
	if err != nil {
		return fmt.Errorf("db: scan: %w", err)
	}

	destVal.Elem().Set(val)
	return rows.Err()
}

// ─── Exec ────────────────────────────────────────────────────────────────────

// Exec 执行写操作（INSERT / UPDATE / DELETE）。
func Exec(ctx context.Context, db Execable, query string, args ...any) (sql.Result, error) {
	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("db: exec: %w", err)
	}
	return result, nil
}

// ─── Transact ────────────────────────────────────────────────────────────────

// Transact 在事务中执行 fn。fn 返回 error 时自动回滚，否则提交。
func Transact(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin tx: %w", err)
	}

	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

// ─── Abstractions ────────────────────────────────────────────────────────────

// Queryable 抽象可执行查询的类型（*sql.DB / *sql.Tx / *sql.Conn）。
type Queryable interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Execable 抽象可执行写操作的类型。
type Execable interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ─── Health Checker ──────────────────────────────────────────────────────────

// HealthChecker 实现 monitor.HealthChecker 接口，用于数据库连接池健康检查。
// name 可选，为空时默认使用 "database"。
type HealthChecker struct {
	db   *sql.DB
	name string
	pm   *poolMetrics // 可选，由 RegisterPoolMetrics 设置
}

// NewHealthChecker 创建数据库健康检查器。
func NewHealthChecker(db *sql.DB, name string) *HealthChecker {
	if name == "" {
		name = "database"
	}
	return &HealthChecker{db: db, name: name}
}

// Name 返回检查器名称。
func (h *HealthChecker) Name() string { return h.name }

// Check 执行 Ping 检查数据库连通性。
func (h *HealthChecker) Check(ctx context.Context) error {
	return h.db.PingContext(ctx)
}

// Stats 返回连接池统计信息，用于监控指标。
func (h *HealthChecker) Stats() sql.DBStats {
	return h.db.Stats()
}

// StatsMetrics 将连接池统计写入 Gauge 指标（需配合 monitor 包使用）。
type StatsMetrics struct {
	MaxOpenConnections int // 最大连接数
	OpenConnections    int // 当前打开连接数
	InUse              int // 正在使用中的连接数
	Idle               int // 空闲连接数
	WaitCount          int // 等待连接的请求数
	WaitDuration       time.Duration // 总等待时间
}

// poolMetrics 持有自动注册的连接池指标。
type poolMetrics struct {
	maxOpen *monitor.Gauge
	open    *monitor.Gauge
	inUse   *monitor.Gauge
	idle    *monitor.Gauge
	wait    *monitor.Gauge
}

// RegisterPoolMetrics 将连接池指标注册到 Registry，开箱即用。
// 注册的指标（均为 Gauge，带 label name=HealthChecker.name）：
//
//	db_pool_max_open_connections
//	db_pool_open_connections
//	db_pool_in_use
//	db_pool_idle
//	db_pool_wait_count
//
// 调用方需定时调用 CollectPoolMetrics() 更新指标值。
func (h *HealthChecker) RegisterPoolMetrics(r *monitor.Registry) *HealthChecker {
	labelName := h.name
	pm := &poolMetrics{
		maxOpen: monitor.NewGauge("db_pool_max_open_connections", "Maximum number of open connections", []string{"name"}),
		open:    monitor.NewGauge("db_pool_open_connections", "Number of open connections", []string{"name"}),
		inUse:   monitor.NewGauge("db_pool_in_use", "Number of in-use connections", []string{"name"}),
		idle:    monitor.NewGauge("db_pool_idle", "Number of idle connections", []string{"name"}),
		wait:    monitor.NewGauge("db_pool_wait_count", "Total number of connections waited for", []string{"name"}),
	}
	// 将 poolMetrics 存储为 HealthChecker 的私有状态
	h.pm = pm
	// 预设 label 值
	pm.maxOpen.DefaultLabels = map[string]string{"name": labelName}
	pm.open.DefaultLabels = map[string]string{"name": labelName}
	pm.inUse.DefaultLabels = map[string]string{"name": labelName}
	pm.idle.DefaultLabels = map[string]string{"name": labelName}
	pm.wait.DefaultLabels = map[string]string{"name": labelName}

	r.MustRegister(pm.maxOpen, pm.open, pm.inUse, pm.idle, pm.wait)
	return h
}

// CollectPoolMetrics 采集连接池统计并更新已注册的指标。
// 需先调用 RegisterPoolMetrics。未注册时静默忽略。
func (h *HealthChecker) CollectPoolMetrics() {
	if h.pm == nil {
		return
	}
	s := h.db.Stats()
	h.pm.maxOpen.Set(float64(s.MaxOpenConnections))
	h.pm.open.Set(float64(s.OpenConnections))
	h.pm.inUse.Set(float64(s.InUse))
	h.pm.idle.Set(float64(s.Idle))
	h.pm.wait.Set(float64(s.WaitCount))
}

// PoolConfig 是连接池配置参数，与 database/sql 的 SetXxx 方法一一对应。
// 零值表示不设置（使用 Go 默认值）。
type PoolConfig struct {
	// MaxOpenConns 最大打开连接数。0 表示不限制（Go 默认）。
	MaxOpenConns int
	// MaxIdleConns 最大空闲连接数。0 表示不设置（Go 默认 2）。
	MaxIdleConns int
	// ConnMaxLifetime 连接最大存活时间。0 表示不设置（永不过期）。
	ConnMaxLifetime int // 秒
	// ConnMaxIdleTime 连接最大空闲时间。0 表示不设置（永不过期）。
	ConnMaxIdleTime int // 秒
}

// ConfigurePool 将连接池配置应用到 *sql.DB。
// 零值字段不修改（保留 Go 默认值）。
//
// 使用示例（配合 conf TOML）：
//
//	type AppConfig struct {
//	    DB db.PoolConfig `toml:"db"`
//	}
//	db.ConfigurePool(pool, cfg.DB)
func ConfigurePool(db *sql.DB, cfg PoolConfig) {
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)
	}
	if cfg.ConnMaxIdleTime > 0 {
		db.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTime) * time.Second)
	}
}

// StatsSnapshot 返回连接池的当前统计快照。
func (h *HealthChecker) StatsSnapshot() StatsMetrics {
	s := h.db.Stats()
	return StatsMetrics{
		MaxOpenConnections: s.MaxOpenConnections,
		OpenConnections:    s.OpenConnections,
		InUse:              s.InUse,
		Idle:               s.Idle,
		WaitCount:          int(s.WaitCount),
		WaitDuration:       s.WaitDuration,
	}
}

// ─── Internal: snake_case converter ──────────────────────────────────────────

// toSnakeCase 将 CamelCase 转为 snake_case。
// UserID → user_id, CreatedAt → created_at, ID → id。
func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				// 前一个字符是小写，插入下划线
				prev := rune(s[i-1])
				if unicode.IsLower(prev) {
					b.WriteByte('_')
				} else if i+1 < len(s) && unicode.IsLower(rune(s[i+1])) {
					// 连续大写后出现小写：HTTPServer → http_server
					if i > 0 {
						b.WriteByte('_')
					}
				}
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

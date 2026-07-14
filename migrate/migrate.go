// Package migrate 提供基于 SQL 文件的轻量级数据库迁移。
//
// # 核心设计
//
// 不是 DSL 迁移工具。只解决一个痛点：按序号执行 SQL 文件并记录版本。
// 迁移文件放在 migrates/ 目录下，按文件名排序执行：
//
//	migrates/
//	  001_create_users.sql
//	  002_add_email_column.sql
//	  003_create_indexes.sql
//
// 执行过的迁移记录在 _schema_migrations 表中，不会重复执行。
//
// # 使用示例
//
//	db, _ := sql.Open("postgres", "...")
//	migrate.Up(db, "migrates/")
//
// # 与成熟工具的关系
//
// 如需更复杂的功能（回滚、校验和、DSL），推荐使用 golang-migrate。
// 本包适合简单场景：不超过 50 个迁移文件，无需回滚。
package migrate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const migrationTable = "_schema_migrations"

// Up 执行所有未执行的迁移文件。
// dir 是包含 .sql 文件的目录路径。
//
// 流程：
//  1. 创建 _schema_migrations 表（如不存在）
//  2. 列出 dir 下所有 .sql 文件，按文件名排序
//  3. 跳过已执行的迁移
//  4. 在事务中执行每个迁移文件，并记录版本
func Up(ctx context.Context, db *sql.DB, dir string) error {
	// 1. 确保迁移记录表存在
	if err := ensureTable(ctx, db); err != nil {
		return fmt.Errorf("migrate: ensure table: %w", err)
	}

	// 2. 列出迁移文件
	files, err := listSQLFiles(dir)
	if err != nil {
		return fmt.Errorf("migrate: list files: %w", err)
	}

	// 3. 获取已执行的迁移
	executed, err := getExecuted(ctx, db)
	if err != nil {
		return fmt.Errorf("migrate: get executed: %w", err)
	}

	// 4. 按序执行未执行的迁移
	for _, f := range files {
		version := versionFromFile(f)
		if executed[version] {
			fmt.Fprintf(os.Stderr, "[migrate] skip %s (already executed)\n", version)
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return fmt.Errorf("migrate: read %s: %w", f, err)
		}

		fmt.Fprintf(os.Stderr, "[migrate] executing %s...\n", version)

		if err := executeMigration(ctx, db, version, string(content)); err != nil {
			return fmt.Errorf("migrate: execute %s: %w", version, err)
		}

		fmt.Fprintf(os.Stderr, "[migrate] %s done\n", version)
	}

	return nil
}

// ensureTable 创建迁移记录表（如不存在）。
func ensureTable(ctx context.Context, db *sql.DB) error {
	sql := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		version TEXT PRIMARY KEY,
		executed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`, migrationTable)
	_, err := db.ExecContext(ctx, sql)
	return err
}

// listSQLFiles 列出目录下所有 .sql 文件，按文件名排序。
func listSQLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	return files, nil
}

// getExecuted 获取已执行的迁移版本集合。
func getExecuted(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT version FROM %s", migrationTable))
	if err != nil {
		// 表不存在时不算错误
		return map[string]bool{}, nil
	}
	defer rows.Close()

	executed := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		executed[v] = true
	}
	return executed, rows.Err()
}

// versionFromFile 从文件名提取版本号（去除 .sql 后缀）。
func versionFromFile(name string) string {
	return strings.TrimSuffix(name, ".sql")
}

// executeMigration 在事务中执行迁移并记录版本。
func executeMigration(ctx context.Context, db *sql.DB, version, sqlContent string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 执行迁移 SQL（可能包含多条语句，由 driver 处理）
	if _, err := tx.ExecContext(ctx, sqlContent); err != nil {
		return err
	}

	// 记录版本，根据驱动类型选择占位符
	// PostgreSQL 使用 $1/$2，MySQL/SQLite 使用 ?/?
	now := time.Now().Format("2006-01-02 15:04:05")
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (version, executed_at) VALUES (%s, %s)",
		migrationTable,
		placeholder(db, 1),
		placeholder(db, 2),
	)
	if _, err := tx.ExecContext(ctx, insertSQL, version, now); err != nil {
		return err
	}

	return tx.Commit()
}

// placeholder 根据数据库驱动类型返回对应的占位符。
// PostgreSQL 返回 $n（如 $1、$2），其他驱动返回 ?。
func placeholder(db *sql.DB, n int) string {
	if isPostgres(db.Driver()) {
		return fmt.Sprintf("$%d", n)
	}
	return "?"
}

// isPostgres 通过反射检查驱动类型名称，判断是否为 PostgreSQL 驱动。
// 支持 lib/pq、pgx 等主流 PostgreSQL 驱动。
func isPostgres(d driver.Driver) bool {
	t := reflect.TypeOf(d)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := strings.ToLower(t.PkgPath() + "." + t.Name())
	return strings.Contains(name, "postgres") ||
		strings.Contains(name, "pgx") ||
		strings.Contains(name, "pq")
}

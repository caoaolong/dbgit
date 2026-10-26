package dbprobe

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// Databases 列出实例下的用户库（过滤系统库）。
func Databases(ctx context.Context, host string, port int, user, password string) ([]string, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.User = user
	cfg.Passwd = password
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("创建连接失败: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, fmt.Errorf("查询库列表失败: %w", err)
	}
	defer rows.Close()

	system := map[string]bool{
		"information_schema": true,
		"mysql":              true,
		"performance_schema": true,
		"sys":                true,
	}
	out := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !system[name] {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// EnsureUser 在后端实例上创建代理账号（不存在则建，有则重置密码并重授），供 ProxySQL 回连使用。
func EnsureUser(ctx context.Context, host string, port int, adminUser, adminPass, user, pass string) error {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.User = adminUser
	cfg.Passwd = adminPass
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second
	cfg.AllowCleartextPasswords = true

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return fmt.Errorf("连接后端失败: %w", err)
	}
	defer db.Close()

	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	stmts := []string{
		fmt.Sprintf("CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'", esc(user), esc(pass)),
		fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'", esc(user), esc(pass)),
		fmt.Sprintf("GRANT ALL PRIVILEGES ON *.* TO '%s'@'%%' WITH GRANT OPTION", esc(user)),
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("创建代理账号失败: %w", err)
		}
	}
	return nil
}

// Version 探测 MySQL 协议实例的版本号（SELECT VERSION()）。
// ctx 建议带超时；连不上直接返回错误，调用方决定是否阻塞流程。
func Version(ctx context.Context, host string, port int, user, password string) (string, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.User = user
	cfg.Passwd = password
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 5 * time.Second
	cfg.WriteTimeout = 5 * time.Second

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return "", fmt.Errorf("创建连接失败: %w", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return "", fmt.Errorf("查询版本失败: %w", err)
	}
	return version, nil
}

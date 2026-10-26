package dbprobe

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// validIdent 校验库名可安全拼入 DDL（与后端的库名规则一致：字母/数字/下划线/$，不含反引号）。
func validIdent(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, r := range name {
		if r == '`' || r == ';' || r == '\'' || r == '"' || r == ' ' {
			return false
		}
	}
	return true
}

func openAdmin(ctx context.Context, host string, port int, user, password string) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.User = user
	cfg.Passwd = password
	cfg.Timeout = 5 * time.Second
	cfg.ReadTimeout = 10 * time.Second
	cfg.WriteTimeout = 10 * time.Second

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("创建连接失败: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接实例失败: %w", err)
	}
	return db, nil
}

// DatabaseExists 查询目标实例上是否存在指定库。
func DatabaseExists(ctx context.Context, host string, port int, user, password, dbName string) (bool, error) {
	if !validIdent(dbName) {
		return false, fmt.Errorf("库名非法: %s", dbName)
	}
	db, err := openAdmin(ctx, host, port, user, password)
	if err != nil {
		return false, err
	}
	defer db.Close()
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`, dbName).Scan(&n); err != nil {
		return false, fmt.Errorf("查询库是否存在失败: %w", err)
	}
	return n > 0, nil
}

// DBCharset 查询源库的默认字符集（用于新建目标库时保持一致；查不到则返回空串，调用方回退默认）。
func DBCharset(ctx context.Context, host string, port int, user, password, dbName string) string {
	if !validIdent(dbName) {
		return ""
	}
	db, err := openAdmin(ctx, host, port, user, password)
	if err != nil {
		return ""
	}
	defer db.Close()
	var cs sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT DEFAULT_CHARACTER_SET_NAME FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`, dbName).Scan(&cs); err != nil {
		return ""
	}
	return strings.TrimSpace(cs.String)
}

// DropDatabase 删除目标实例上的指定库（调用前应已确认存在且用户勾选覆盖）。
func DropDatabase(ctx context.Context, host string, port int, user, password, dbName string) error {
	if !validIdent(dbName) {
		return fmt.Errorf("库名非法: %s", dbName)
	}
	db, err := openAdmin(ctx, host, port, user, password)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, fmt.Sprintf("DROP DATABASE `%s`", dbName)); err != nil {
		return fmt.Errorf("删除数据库失败: %w", err)
	}
	return nil
}

// CreateDatabase 在目标实例上新建库（调用前应已确认不存在且用户勾选新建；charset 为空则用实例默认）。
func CreateDatabase(ctx context.Context, host string, port int, user, password, dbName, charset string) error {
	if !validIdent(dbName) {
		return fmt.Errorf("库名非法: %s", dbName)
	}
	db, err := openAdmin(ctx, host, port, user, password)
	if err != nil {
		return err
	}
	defer db.Close()
	stmt := fmt.Sprintf("CREATE DATABASE `%s`", dbName)
	if strings.TrimSpace(charset) != "" {
		stmt += fmt.Sprintf(" CHARACTER SET %s", strings.TrimSpace(charset))
	}
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("新建数据库失败: %w", err)
	}
	return nil
}

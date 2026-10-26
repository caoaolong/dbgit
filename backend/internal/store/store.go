package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB SQLite 封装。
type DB struct {
	sql *sql.DB
}

// Instance 数据库实例记录。密码明文存放（本地内网工具定位），列表接口默认不返回。
// Version 为连接探测到的版本号（SELECT VERSION()），探测失败时为空。
type Instance struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	DBType    string `json:"db_type"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password,omitempty"`
	Version   string `json:"version"`
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// Open 打开（不存在则创建）SQLite 文件并建表。
func Open(path string) (*DB, error) {
	if path == "" {
		path = "data/dbgit.db"
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("创建数据目录失败: %w", err)
		}
	}
	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	db := &DB{sql: sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Close 关闭连接。
func (d *DB) Close() error {
	return d.sql.Close()
}

func (d *DB) migrate() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS db_instances (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		db_type TEXT NOT NULL DEFAULT 'mysql',
		host TEXT NOT NULL,
		port INTEGER NOT NULL DEFAULT 3306,
		username TEXT NOT NULL DEFAULT '',
		password TEXT NOT NULL DEFAULT '',
		remark TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("建表失败: %w", err)
	}
	_, err = d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_db_instances_name ON db_instances(name)`)
	if err != nil {
		return fmt.Errorf("建索引失败: %w", err)
	}
	if err := d.migrateProjects(); err != nil {
		return fmt.Errorf("建项目表失败: %w", err)
	}
	if err := d.migrateProxy(); err != nil {
		return fmt.Errorf("建代理映射表失败: %w", err)
	}
	if err := d.migrateProxyNodes(); err != nil {
		return fmt.Errorf("建代理节点表失败: %w", err)
	}
	if err := d.migrateSQLChanges(); err != nil {
		return fmt.Errorf("建变更记录表失败: %w", err)
	}
	if err := d.migrateSyncConfig(); err != nil {
		return fmt.Errorf("建同步配置表失败: %w", err)
	}
	if err := d.migrateSyncMerges(); err != nil {
		return fmt.Errorf("建同步合并表失败: %w", err)
	}
	if err := d.migrateSyncClones(); err != nil {
		return fmt.Errorf("建同步克隆表失败: %w", err)
	}
	// 存量库补 version 列。
	if !d.hasColumn("db_instances", "version") {
		if _, err := d.sql.Exec(`ALTER TABLE db_instances ADD COLUMN version TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("补 version 列失败: %w", err)
		}
	}
	return nil
}

func (d *DB) hasColumn(table, column string) bool {
	rows, err := d.sql.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false
	}
	defer rows.Close()
	var cid int
	var name, ctype string
	var notnull, pk int
	var dflt sql.NullString
	for rows.Next() {
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == column {
			return true
		}
	}
	return false
}

func msToRFC3339(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func scanInstance(row interface {
	Scan(dest ...any) error
}, withPassword bool) (Instance, error) {
	var ins Instance
	var created, updated int64
	var password sql.NullString
	var remark, username, version, dbType sql.NullString
	err := row.Scan(
		&ins.ID, &ins.Name, &dbType, &ins.Host, &ins.Port,
		&username, &password, &version, &remark, &created, &updated,
	)
	if err != nil {
		return ins, err
	}
	ins.DBType = dbType.String
	ins.Username = username.String
	ins.Version = version.String
	ins.Remark = remark.String
	if withPassword {
		ins.Password = password.String
	}
	ins.CreatedAt = msToRFC3339(created)
	ins.UpdatedAt = msToRFC3339(updated)
	return ins, nil
}

// 实例查询统一列（含 version）。
const instanceColumns = `id, name, db_type, host, port, username, password, version, remark, created_at, updated_at`

// ListInstances 返回全部实例（不含密码），按 id 升序。
func (d *DB) ListInstances() ([]Instance, error) {
	rows, err := d.sql.Query(`SELECT ` + instanceColumns + ` FROM db_instances ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Instance, 0)
	for rows.Next() {
		ins, err := scanInstance(rows, false)
		if err != nil {
			return nil, err
		}
		out = append(out, ins)
	}
	return out, rows.Err()
}

// GetInstance 按 id 查询（含密码，供编辑回显）。
func (d *DB) GetInstance(id int64) (Instance, error) {
	row := d.sql.QueryRow(`SELECT `+instanceColumns+` FROM db_instances WHERE id = ?`, id)
	return scanInstance(row, true)
}

// CreateInstance 新增实例。
func (d *DB) CreateInstance(ins Instance) (Instance, error) {
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO db_instances
		(name, db_type, host, port, username, password, version, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ins.Name, ins.DBType, ins.Host, ins.Port, ins.Username, ins.Password, ins.Version, ins.Remark, now, now)
	if err != nil {
		return Instance{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Instance{}, err
	}
	return d.GetInstance(id)
}

// UpdateInstance 全量更新；password 为空字符串表示保持原密码不变。
// version 为空表示保持原版本（探测失败时调用方传空即可）。
func (d *DB) UpdateInstance(id int64, ins Instance) (Instance, error) {
	old, err := d.GetInstance(id)
	if err != nil {
		return Instance{}, err
	}
	password := old.Password
	if ins.Password != "" {
		password = ins.Password
	}
	version := old.Version
	if ins.Version != "" {
		version = ins.Version
	}
	_, err = d.sql.Exec(`UPDATE db_instances SET
		name = ?, db_type = ?, host = ?, port = ?, username = ?, password = ?, version = ?, remark = ?, updated_at = ?
		WHERE id = ?`,
		ins.Name, ins.DBType, ins.Host, ins.Port, ins.Username, password, version, ins.Remark,
		time.Now().UnixMilli(), id)
	if err != nil {
		return Instance{}, err
	}
	return d.GetInstance(id)
}

// DeleteInstance 删除实例，返回是否真实删除了一行。
// 注意：调用方需先确认无项目环境绑定（CountBindingsByInstance）。
func (d *DB) DeleteInstance(id int64) (bool, error) {
	res, err := d.sql.Exec(`DELETE FROM db_instances WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// IsUniqueViolation 判断是否为唯一键冲突（名称重复）。
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

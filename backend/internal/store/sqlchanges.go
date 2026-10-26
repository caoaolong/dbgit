package store

import (
	"database/sql"
	"strings"
	"time"
)

// SQLChange 某绑定库的一条变更语句记录（采集自 ProxySQL Advanced Events Logging）。
// EventTime / CollectedAt 对外为 RFC3339，库内以毫秒时间戳存储。
type SQLChange struct {
	ID           int64  `json:"id"`
	BindingID    int64  `json:"binding_id"`
	StmtType     string `json:"stmt_type"`
	SQLText      string `json:"sql_text"`
	Username     string `json:"username"`
	ClientAddr   string `json:"client_addr"`
	Schemaname   string `json:"schemaname"`
	HostgroupID  int    `json:"hostgroup_id"`
	DurationUs   int64  `json:"duration_us"`
	RowsAffected int64  `json:"rows_affected"`
	EventTime    string `json:"event_time"`
	CollectedAt  string `json:"collected_at"`
}

func (d *DB) migrateSQLChanges() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS sql_statements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		binding_id INTEGER NOT NULL,
		stmt_type TEXT NOT NULL DEFAULT '',
		sql_text TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT '',
		client_addr TEXT NOT NULL DEFAULT '',
		schemaname TEXT NOT NULL DEFAULT '',
		hostgroup_id INTEGER NOT NULL DEFAULT 0,
		duration_us INTEGER NOT NULL DEFAULT 0,
		rows_affected INTEGER NOT NULL DEFAULT 0,
		event_time INTEGER NOT NULL DEFAULT 0,
		collected_at INTEGER NOT NULL,
		dedup TEXT NOT NULL DEFAULT '',
		UNIQUE(dedup)
	)`)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_sql_statements_binding_time
		ON sql_statements(binding_id, event_time DESC)`)
	return err
}

const sqlChangeColumns = `id, binding_id, stmt_type, sql_text, username, client_addr,
	schemaname, hostgroup_id, duration_us, rows_affected, event_time, collected_at`

func scanSQLChange(row interface {
	Scan(dest ...any) error
}) (SQLChange, error) {
	var c SQLChange
	var eventTime, collectedAt int64
	err := row.Scan(
		&c.ID, &c.BindingID, &c.StmtType, &c.SQLText, &c.Username, &c.ClientAddr,
		&c.Schemaname, &c.HostgroupID, &c.DurationUs, &c.RowsAffected,
		&eventTime, &collectedAt,
	)
	if err != nil {
		return SQLChange{}, err
	}
	c.EventTime = msToRFC3339(eventTime)
	c.CollectedAt = msToRFC3339(collectedAt)
	return c, nil
}

// InsertSQLChanges 批量写入采集到的语句（按 dedup 去重），返回实际写入行数。
func (d *DB) InsertSQLChanges(rows []SQLChange, dedups []string, eventMs []int64) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO sql_statements
		(binding_id, stmt_type, sql_text, username, client_addr, schemaname,
		 hostgroup_id, duration_us, rows_affected, event_time, collected_at, dedup)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	now := time.Now().UnixMilli()
	var total int64
	for i, r := range rows {
		res, err := stmt.Exec(
			r.BindingID, r.StmtType, r.SQLText, r.Username, r.ClientAddr, r.Schemaname,
			r.HostgroupID, r.DurationUs, r.RowsAffected, eventMs[i], now, dedups[i],
		)
		if err != nil {
			return total, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += n
	}
	if err := tx.Commit(); err != nil {
		return total, err
	}
	return total, nil
}

// SQLChangeFilter 变更记录查询条件（时间均为毫秒时间戳，0 表示不限）。
type SQLChangeFilter struct {
	Since    int64
	Until    int64
	Keyword  string
	StmtType string
	Page     int
	PageSize int
}

func (f SQLChangeFilter) normalize() (page, pageSize int) {
	page, pageSize = f.Page, f.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

func sqlChangeWhere(bindingID int64, f SQLChangeFilter) (string, []any) {
	var sb strings.Builder
	args := make([]any, 0, 8)
	sb.WriteString(`WHERE binding_id = ?`)
	args = append(args, bindingID)
	if f.Since > 0 {
		sb.WriteString(` AND event_time >= ?`)
		args = append(args, f.Since)
	}
	if f.Until > 0 {
		sb.WriteString(` AND event_time <= ?`)
		args = append(args, f.Until)
	}
	if t := strings.TrimSpace(f.StmtType); t != "" {
		sb.WriteString(` AND stmt_type = ?`)
		args = append(args, strings.ToUpper(t))
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		sb.WriteString(` AND sql_text LIKE ? ESCAPE '\'`)
		args = append(args, "%"+escapeLike(kw)+"%")
	}
	return sb.String(), args
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// CountSQLChanges 按条件统计变更记录数。
func (d *DB) CountSQLChanges(bindingID int64, f SQLChangeFilter) (int, error) {
	where, args := sqlChangeWhere(bindingID, f)
	var n int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM sql_statements `+where, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// ListSQLChanges 按条件分页查询变更记录（按执行时间倒序）。
func (d *DB) ListSQLChanges(bindingID int64, f SQLChangeFilter) ([]SQLChange, error) {
	page, pageSize := f.normalize()
	where, args := sqlChangeWhere(bindingID, f)
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := d.sql.Query(`SELECT `+sqlChangeColumns+` FROM sql_statements `+where+
		` ORDER BY event_time DESC, id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SQLChange, 0, pageSize)
	for rows.Next() {
		c, err := scanSQLChange(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CountSQLStatements 统计变更记录总数。
func (d *DB) CountSQLStatements() (int64, error) {
	var n int64
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM sql_statements`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// MaxSQLChangeEventTime 已入库的最大事件时间（毫秒），空表返回 0。
func (d *DB) MaxSQLChangeEventTime() (int64, error) {
	var v sql.NullInt64
	if err := d.sql.QueryRow(`SELECT MAX(event_time) FROM sql_statements`).Scan(&v); err != nil {
		return 0, err
	}
	if !v.Valid {
		return 0, nil
	}
	return v.Int64, nil
}

// DeleteSQLChangesBefore 清理指定时间点之前的变更记录，返回删除行数。
func (d *DB) DeleteSQLChangesBefore(cutoffMs int64) (int64, error) {
	res, err := d.sql.Exec(`DELETE FROM sql_statements WHERE event_time < ?`, cutoffMs)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// deleteSQLChangesByBinding 删除某绑定下的全部变更记录（解绑级联清理用）。
func (d *DB) deleteSQLChangesByBindingTx(tx *sql.Tx, bindingID int64) error {
	_, err := tx.Exec(`DELETE FROM sql_statements WHERE binding_id = ?`, bindingID)
	return err
}

func (d *DB) deleteSQLChangesByEnvTx(tx *sql.Tx, envID int64) error {
	_, err := tx.Exec(`DELETE FROM sql_statements
		WHERE binding_id IN (SELECT id FROM project_env_dbs WHERE env_id = ?)`, envID)
	return err
}

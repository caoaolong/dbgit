package store

import (
	"database/sql"
	"time"
)

// SyncClone 一次克隆记录：把某绑定库拷贝到目标分支，为后续合并建立共同基线。
// 当前仅记录血缘与绑定关系（元数据层面），实际数据搬运由后续同步执行。
type SyncClone struct {
	ID              int64  `json:"id"`
	ProjectID       int64  `json:"project_id"`
	SourceBindingID int64  `json:"source_binding_id"`
	SourceDBName    string `json:"source_db_name"`
	SourceEnvID     int64  `json:"source_env_id"`
	SourceEnvName   string `json:"source_env_name"`
	TargetEnvID     int64  `json:"target_env_id"`
	TargetEnvName   string `json:"target_env_name"`
	TargetBindingID int64  `json:"target_binding_id"`
	TargetDBName    string `json:"target_db_name"`
	Comment         string `json:"comment"`
	StmtCount       int    `json:"stmt_count"`
	CreatedAt       string `json:"created_at"`
}

func (d *DB) migrateSyncClones() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS sync_clones (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		source_binding_id INTEGER NOT NULL,
		target_env_id INTEGER NOT NULL,
		target_binding_id INTEGER NOT NULL DEFAULT 0,
		comment TEXT NOT NULL DEFAULT '',
		stmt_count INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_sync_clones_project ON sync_clones(project_id, id)`)
	return err
}

// CreateSyncClone 创建一次克隆记录。
func (d *DB) CreateSyncClone(projectID, sourceBindingID, targetEnvID, targetBindingID int64, comment string, stmtCount int) (int64, error) {
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO sync_clones
		(project_id, source_binding_id, target_env_id, target_binding_id, comment, stmt_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, sourceBindingID, targetEnvID, targetBindingID, comment, stmtCount, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const syncCloneColumns = `c.id, c.project_id, c.source_binding_id, sb.db_name,
	se.id, se.name, c.target_env_id, te.name, c.target_binding_id, COALESCE(tb.db_name, ''),
	c.comment, c.stmt_count, c.created_at`

const syncCloneJoin = `FROM sync_clones c
	JOIN project_env_dbs sb ON sb.id = c.source_binding_id
	JOIN project_envs se ON se.id = sb.env_id
	JOIN project_envs te ON te.id = c.target_env_id
	LEFT JOIN project_env_dbs tb ON tb.id = c.target_binding_id`

func scanSyncClone(row interface {
	Scan(dest ...any) error
}) (SyncClone, error) {
	var m SyncClone
	var createdAt int64
	var targetDB sql.NullString
	err := row.Scan(
		&m.ID, &m.ProjectID, &m.SourceBindingID, &m.SourceDBName,
		&m.SourceEnvID, &m.SourceEnvName, &m.TargetEnvID, &m.TargetEnvName,
		&m.TargetBindingID, &targetDB, &m.Comment, &m.StmtCount, &createdAt,
	)
	if err != nil {
		return SyncClone{}, err
	}
	m.TargetDBName = targetDB.String
	m.CreatedAt = msToRFC3339(createdAt)
	return m, nil
}

// ListSyncClones 列出某项目的全部克隆记录（按创建时间升序，供分支树按序重放）。
func (d *DB) ListSyncClones(projectID int64) ([]SyncClone, error) {
	rows, err := d.sql.Query(`SELECT `+syncCloneColumns+` `+syncCloneJoin+` WHERE c.project_id = ? ORDER BY c.id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SyncClone, 0)
	for rows.Next() {
		m, err := scanSyncClone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// HasCloneLineage 是否存在从源绑定到目标绑定的克隆血缘。
func (d *DB) HasCloneLineage(sourceBindingID, targetBindingID int64) (bool, error) {
	var n int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM sync_clones WHERE source_binding_id = ? AND target_binding_id = ?`,
		sourceBindingID, targetBindingID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

package store

import (
	"database/sql"
	"time"
)

// SyncMerge 一次同步合并记录：把某绑定库的变更合并到目标分支。
type SyncMerge struct {
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
	// HasBase 是否有共同基线（克隆血缘或同名库）。
	HasBase   bool   `json:"has_base"`
	CreatedAt string `json:"created_at"`
}

func (d *DB) migrateSyncMerges() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS sync_merges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL,
		source_binding_id INTEGER NOT NULL,
		target_env_id INTEGER NOT NULL,
		target_binding_id INTEGER NOT NULL DEFAULT 0,
		comment TEXT NOT NULL DEFAULT '',
		stmt_count INTEGER NOT NULL DEFAULT 0,
		has_base INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}
	// 存量表补 has_base 列。
	if !d.hasColumn("sync_merges", "has_base") {
		if _, err := d.sql.Exec(`ALTER TABLE sync_merges ADD COLUMN has_base INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	_, err = d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_sync_merges_project
		ON sync_merges(project_id, id)`)
	return err
}

// ListEnvDBs 列出某分支下的全部绑定（供合并目标解析用）。
func (d *DB) ListEnvDBs(envID int64) ([]EnvDB, error) {
	return d.listEnvDBs(envID)
}

// CreateSyncMerge 创建一次同步合并记录（hasBase 表示合并时是否存在共同基线）。
func (d *DB) CreateSyncMerge(projectID, sourceBindingID, targetEnvID, targetBindingID int64, comment string, stmtCount int, hasBase bool) (int64, error) {
	now := time.Now().UnixMilli()
	base := 0
	if hasBase {
		base = 1
	}
	res, err := d.sql.Exec(`INSERT INTO sync_merges
		(project_id, source_binding_id, target_env_id, target_binding_id, comment, stmt_count, has_base, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, sourceBindingID, targetEnvID, targetBindingID, comment, stmtCount, base, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const syncMergeColumns = `m.id, m.project_id, m.source_binding_id, sb.db_name,
	se.id, se.name, m.target_env_id, te.name, m.target_binding_id,
	COALESCE(tb.db_name, ''), m.comment, m.stmt_count, COALESCE(m.has_base, 0), m.created_at`

const syncMergeJoin = `FROM sync_merges m
	JOIN project_env_dbs sb ON sb.id = m.source_binding_id
	JOIN project_envs se ON se.id = sb.env_id
	JOIN project_envs te ON te.id = m.target_env_id
	LEFT JOIN project_env_dbs tb ON tb.id = m.target_binding_id`

func scanSyncMerge(row interface {
	Scan(dest ...any) error
}) (SyncMerge, error) {
	var m SyncMerge
	var createdAt int64
	var hasBase int
	var targetDB sql.NullString
	err := row.Scan(
		&m.ID, &m.ProjectID, &m.SourceBindingID, &m.SourceDBName,
		&m.SourceEnvID, &m.SourceEnvName, &m.TargetEnvID, &m.TargetEnvName,
		&m.TargetBindingID, &targetDB, &m.Comment, &m.StmtCount, &hasBase, &createdAt,
	)
	if err != nil {
		return SyncMerge{}, err
	}
	m.TargetDBName = targetDB.String
	m.HasBase = hasBase != 0
	m.CreatedAt = msToRFC3339(createdAt)
	return m, nil
}

// ListSyncMerges 列出某项目的全部合并记录（按创建时间升序，供分支树按序重放）。
func (d *DB) ListSyncMerges(projectID int64) ([]SyncMerge, error) {
	rows, err := d.sql.Query(`SELECT `+syncMergeColumns+` `+syncMergeJoin+` WHERE m.project_id = ? ORDER BY m.id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SyncMerge, 0)
	for rows.Next() {
		m, err := scanSyncMerge(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

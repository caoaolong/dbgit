package store

import (
	"database/sql"
	"time"
)

// 项目同步配置：类 .gitignore 文本，描述同步时忽略哪些表/操作。
// SyncConfigRevision 是同步配置的一次版本快照（每次保存追加一条，供分支树回放）。
type SyncConfigRevision struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	RuleCount int    `json:"rule_count"`
	Comment   string `json:"comment"`
	CreatedAt string `json:"created_at"`
}

func (d *DB) migrateSyncConfig() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS project_sync_config (
		project_id INTEGER PRIMARY KEY REFERENCES projects(id),
		content TEXT NOT NULL DEFAULT '',
		updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`CREATE TABLE IF NOT EXISTS project_sync_config_revisions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id INTEGER NOT NULL REFERENCES projects(id),
		content TEXT NOT NULL DEFAULT '',
		rule_count INTEGER NOT NULL DEFAULT 0,
		comment TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}
	_, err = d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_sync_config_revisions_project
		ON project_sync_config_revisions(project_id, id)`)
	return err
}

// GetSyncConfig 读取项目同步配置；未配置过返回空字符串。
func (d *DB) GetSyncConfig(projectID int64) (string, error) {
	var content string
	err := d.sql.QueryRow(`SELECT content FROM project_sync_config WHERE project_id = ?`, projectID).Scan(&content)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	return content, nil
}

// UpsertSyncConfig 保存项目同步配置快照（不存在则创建）。
func (d *DB) UpsertSyncConfig(projectID int64, content string) error {
	_, err := d.sql.Exec(`INSERT INTO project_sync_config (project_id, content, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET content = excluded.content, updated_at = excluded.updated_at`,
		projectID, content, time.Now().UnixMilli())
	return err
}

// InsertSyncConfigRevision 追加一条配置版本快照，返回版本 id。
func (d *DB) InsertSyncConfigRevision(projectID int64, content string, ruleCount int, comment string) (int64, error) {
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO project_sync_config_revisions
		(project_id, content, rule_count, comment, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		projectID, content, ruleCount, comment, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListSyncConfigRevisions 列出某项目的配置版本（按创建时间升序，供分支树回放）。
func (d *DB) ListSyncConfigRevisions(projectID int64) ([]SyncConfigRevision, error) {
	rows, err := d.sql.Query(`SELECT id, project_id, rule_count, comment, created_at
		FROM project_sync_config_revisions WHERE project_id = ? ORDER BY id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SyncConfigRevision, 0)
	for rows.Next() {
		var r SyncConfigRevision
		var createdAt int64
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.RuleCount, &r.Comment, &createdAt); err != nil {
			return nil, err
		}
		r.CreatedAt = msToRFC3339(createdAt)
		out = append(out, r)
	}
	return out, rows.Err()
}

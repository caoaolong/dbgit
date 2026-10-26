package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Project 项目（一级）。
type Project struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ProjectEnv 项目分支（二级，如开发/测试/生产）。
type ProjectEnv struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Remark    string `json:"remark"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// EnvDB 分支绑定的数据库（三级，具体到某连接下的某个库）。
type EnvDB struct {
	ID           int64  `json:"id"`
	EnvID        int64  `json:"env_id"`
	InstanceID   int64  `json:"instance_id"`
	InstanceName string `json:"instance_name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DBType       string `json:"db_type"`
	Version      string `json:"version"`
	DBName       string `json:"db_name"`
	Remark       string `json:"remark"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// EnvNode 分支节点（含绑定库）。
type EnvNode struct {
	ProjectEnv
	DBs []EnvDB `json:"dbs"`
}

// ProjectNode 项目树节点。
type ProjectNode struct {
	Project
	Envs []EnvNode `json:"envs"`
}

func (d *DB) migrateProjects() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS project_envs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id),
			name TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(project_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS project_env_dbs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env_id INTEGER NOT NULL REFERENCES project_envs(id),
			instance_id INTEGER NOT NULL REFERENCES db_instances(id),
			db_name TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(env_id, instance_id, db_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_envs_project ON project_envs(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_envdbs_env ON project_env_dbs(env_id)`,
		`CREATE INDEX IF NOT EXISTS idx_envdbs_instance ON project_env_dbs(instance_id)`,
	}
	for _, s := range stmts {
		if _, err := d.sql.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func scanProject(row interface{ Scan(dest ...any) error }) (Project, error) {
	var p Project
	var created, updated int64
	var remark sql.NullString
	if err := row.Scan(&p.ID, &p.Name, &remark, &created, &updated); err != nil {
		return p, err
	}
	p.Remark = remark.String
	p.CreatedAt = msToRFC3339(created)
	p.UpdatedAt = msToRFC3339(updated)
	return p, nil
}

func scanEnv(row interface{ Scan(dest ...any) error }) (ProjectEnv, error) {
	var e ProjectEnv
	var created, updated int64
	var remark sql.NullString
	if err := row.Scan(&e.ID, &e.ProjectID, &e.Name, &remark, &created, &updated); err != nil {
		return e, err
	}
	e.Remark = remark.String
	e.CreatedAt = msToRFC3339(created)
	e.UpdatedAt = msToRFC3339(updated)
	return e, nil
}

func scanEnvDB(row interface{ Scan(dest ...any) error }) (EnvDB, error) {
	var b EnvDB
	var created, updated int64
	var remark, version, dbType sql.NullString
	err := row.Scan(
		&b.ID, &b.EnvID, &b.InstanceID, &b.DBName, &remark, &created, &updated,
		&b.InstanceName, &b.Host, &b.Port, &dbType, &version,
	)
	if err != nil {
		return b, err
	}
	b.Remark = remark.String
	b.DBType = dbType.String
	b.Version = version.String
	b.CreatedAt = msToRFC3339(created)
	b.UpdatedAt = msToRFC3339(updated)
	return b, nil
}

const envDBColumns = `b.id, b.env_id, b.instance_id, b.db_name, b.remark, b.created_at, b.updated_at,
	i.name, i.host, i.port, i.db_type, i.version`

const envDBJoin = `FROM project_env_dbs b JOIN db_instances i ON i.id = b.instance_id`

// GetProjectTree 返回完整项目树（项目→分支→绑定库）。
// 注意：先一次性读出项目再查子级，避免在 rows 未关闭时嵌套查询
// （SQLite 单连接下会死锁）。
func (d *DB) GetProjectTree() ([]ProjectNode, error) {
	projects, err := d.listProjects()
	if err != nil {
		return nil, err
	}
	out := make([]ProjectNode, 0, len(projects))
	for _, p := range projects {
		node := ProjectNode{Project: p, Envs: make([]EnvNode, 0)}
		envs, err := d.listEnvs(p.ID)
		if err != nil {
			return nil, err
		}
		for _, e := range envs {
			dbs, err := d.listEnvDBs(e.ID)
			if err != nil {
				return nil, err
			}
			node.Envs = append(node.Envs, EnvNode{ProjectEnv: e, DBs: dbs})
		}
		out = append(out, node)
	}
	return out, nil
}

func (d *DB) listProjects() ([]Project, error) {
	rows, err := d.sql.Query(`SELECT id, name, remark, created_at, updated_at FROM projects ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Project, 0)
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (d *DB) listEnvs(projectID int64) ([]ProjectEnv, error) {
	rows, err := d.sql.Query(`SELECT id, project_id, name, remark, created_at, updated_at
		FROM project_envs WHERE project_id = ? ORDER BY id ASC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProjectEnv, 0)
	for rows.Next() {
		e, err := scanEnv(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) listEnvDBs(envID int64) ([]EnvDB, error) {
	rows, err := d.sql.Query(`SELECT `+envDBColumns+` `+envDBJoin+` WHERE b.env_id = ? ORDER BY b.id ASC`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]EnvDB, 0)
	for rows.Next() {
		b, err := scanEnvDB(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CreateProject 新增项目。
func (d *DB) CreateProject(name, remark string) (Project, error) {
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO projects (name, remark, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		name, remark, now, now)
	if err != nil {
		return Project{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, err
	}
	return d.GetProject(id)
}

// GetProject 按 id 查询项目。
func (d *DB) GetProject(id int64) (Project, error) {
	return scanProject(d.sql.QueryRow(`SELECT id, name, remark, created_at, updated_at FROM projects WHERE id = ?`, id))
}

// UpdateProject 更新项目。
func (d *DB) UpdateProject(id int64, name, remark string) (Project, error) {
	if _, err := d.GetProject(id); err != nil {
		return Project{}, err
	}
	if _, err := d.sql.Exec(`UPDATE projects SET name = ?, remark = ?, updated_at = ? WHERE id = ?`,
		name, remark, time.Now().UnixMilli(), id); err != nil {
		return Project{}, err
	}
	return d.GetProject(id)
}

// DeleteProject 级联删除项目及其分支与绑定。
func (d *DB) DeleteProject(id int64) (bool, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sql_statements
		WHERE binding_id IN (SELECT b.id FROM project_env_dbs b
			JOIN project_envs e ON e.id = b.env_id WHERE e.project_id = ?)`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM project_env_dbs WHERE env_id IN (SELECT id FROM project_envs WHERE project_id = ?)`, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM project_envs WHERE project_id = ?`, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateEnv 新增分支。
func (d *DB) CreateEnv(projectID int64, name, remark string) (ProjectEnv, error) {
	if _, err := d.GetProject(projectID); err != nil {
		return ProjectEnv{}, err
	}
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO project_envs (project_id, name, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, projectID, name, remark, now, now)
	if err != nil {
		return ProjectEnv{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ProjectEnv{}, err
	}
	return d.GetEnv(id)
}

// GetEnv 按 id 查询分支。
func (d *DB) GetEnv(id int64) (ProjectEnv, error) {
	return scanEnv(d.sql.QueryRow(`SELECT id, project_id, name, remark, created_at, updated_at FROM project_envs WHERE id = ?`, id))
}

// CloneTarget 整分支克隆时单个源绑定的目标映射。
type CloneTarget struct {
	SourceBindingID int64  `json:"source_binding_id"`
	InstanceID      int64  `json:"instance_id"`
	DBName          string `json:"db_name"`
	// Dropped / Created 记录克隆前对真实库执行的 DDL 动作（由调用方完成），
	// 仅用于写入克隆标记文本，不参与事务。
	Dropped bool `json:"-"`
	Created bool `json:"-"`
}

// CloneEnv 完整克隆一个分支：新建同项目分支并复制其全部绑定
// （含血缘记录与基准标记提交），整体在一个事务内完成，原子提交。
// 返回含绑定库的新分支节点；来源分支无绑定时等价于普通创建。
// 目标映射默认沿用来源的连接与库名（不做真实 DDL），需逐库指定目标时用 CloneEnvMapped。
func (d *DB) CloneEnv(sourceEnvID int64, name, remark, comment string) (EnvNode, error) {
	return d.CloneEnvMapped(sourceEnvID, name, remark, comment, nil)
}

// CloneEnvMapped 按逐库目标映射完整克隆分支：targets 为空时等价于 CloneEnv；
// 非空时必须恰好覆盖来源分支的全部绑定（每个源绑定出现一次），目标库去重。
// 注意：真实库的 DROP / CREATE 由调用方在调用前完成（DDL 无法纳入 SQLite 事务），
// 此处只做元数据落库（建 env → 建绑定 → 写血缘 → 写 CLONE 标记），单事务原子提交。
func (d *DB) CloneEnvMapped(sourceEnvID int64, name, remark, comment string, targets []CloneTarget) (EnvNode, error) {
	var out EnvNode
	src, err := d.GetEnv(sourceEnvID)
	if err != nil {
		return out, err
	}
	srcDBs, err := d.listEnvDBs(sourceEnvID)
	if err != nil {
		return out, err
	}
	maps := make([]CloneTarget, 0, len(srcDBs))
	if len(targets) == 0 {
		for _, b := range srcDBs {
			maps = append(maps, CloneTarget{SourceBindingID: b.ID, InstanceID: b.InstanceID, DBName: b.DBName})
		}
	} else {
		bySource := make(map[int64]CloneTarget, len(targets))
		seenTarget := make(map[string]struct{}, len(targets))
		for _, t := range targets {
			if _, dup := bySource[t.SourceBindingID]; dup {
				return out, fmt.Errorf("来源库 %d 被映射了多次", t.SourceBindingID)
			}
			key := fmt.Sprintf("%d\x00%s", t.InstanceID, t.DBName)
			if _, dup := seenTarget[key]; dup {
				return out, fmt.Errorf("目标库 %s 重复", t.DBName)
			}
			seenTarget[key] = struct{}{}
			bySource[t.SourceBindingID] = t
		}
		for _, b := range srcDBs {
			t, ok := bySource[b.ID]
			if !ok {
				return out, fmt.Errorf("来源库 %s（id=%d）未指定克隆目标", b.DBName, b.ID)
			}
			maps = append(maps, t)
		}
	}
	// 先统计各源绑定的变更数（用作血缘快照）。
	counts := make([]int, len(srcDBs))
	for i, b := range srcDBs {
		n, err := d.CountSQLChanges(b.ID, SQLChangeFilter{})
		if err != nil {
			return out, err
		}
		counts[i] = n
	}
	// 目标实例必须存在（SQLite 单连接：事务开启后不能再并发查询，此处提前预加载）。
	instIDs := make(map[int64]struct{}, len(maps))
	for _, m := range maps {
		instIDs[m.InstanceID] = struct{}{}
	}
	for id := range instIDs {
		if _, err := d.GetInstance(id); err != nil {
			return out, err
		}
	}
	now := time.Now().UnixMilli()
	tx, err := d.sql.Begin()
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO project_envs (project_id, name, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, src.ProjectID, name, remark, now, now)
	if err != nil {
		return out, err
	}
	newEnvID, err := res.LastInsertId()
	if err != nil {
		return out, err
	}
	for i, b := range srcDBs {
		m := maps[i]
		rb, err := tx.Exec(`INSERT INTO project_env_dbs (env_id, instance_id, db_name, remark, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)`, newEnvID, m.InstanceID, m.DBName, b.Remark, now, now)
		if err != nil {
			return out, err
		}
		newBid, err := rb.LastInsertId()
		if err != nil {
			return out, err
		}
		if _, err := tx.Exec(`INSERT INTO sync_clones
			(project_id, source_binding_id, target_env_id, target_binding_id, comment, stmt_count, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			src.ProjectID, b.ID, newEnvID, newBid, comment, counts[i], now); err != nil {
			return out, err
		}
		markSQL := fmt.Sprintf("-- 克隆基线：%s（%s → %s）", b.DBName, src.Name, name)
		if m.DBName != b.DBName {
			markSQL += fmt.Sprintf("，目标库 %s", m.DBName)
		}
		switch {
		case m.Dropped:
			markSQL += "（已覆盖重建）"
		case m.Created:
			markSQL += "（已自动新建）"
		}
		if strings.TrimSpace(comment) != "" {
			markSQL += " " + strings.TrimSpace(comment)
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO sql_statements
			(binding_id, stmt_type, sql_text, username, client_addr, schemaname,
			 hostgroup_id, duration_us, rows_affected, event_time, collected_at, dedup)
			VALUES (?, 'CLONE', ?, 'dbgit', '', ?, 0, 0, 0, ?, ?, ?)`,
			newBid, markSQL, m.DBName, now, now, fmt.Sprintf("clone-marker-env-%d", newBid)); err != nil {
			return out, err
		}
	}
	if err := tx.Commit(); err != nil {
		return out, err
	}
	env, err := d.GetEnv(newEnvID)
	if err != nil {
		return out, err
	}
	dbs, err := d.listEnvDBs(newEnvID)
	if err != nil {
		return out, err
	}
	out = EnvNode{ProjectEnv: env, DBs: dbs}
	return out, nil
}

// UpdateEnv 更新分支。
func (d *DB) UpdateEnv(id int64, name, remark string) (ProjectEnv, error) {
	old, err := d.GetEnv(id)
	if err != nil {
		return ProjectEnv{}, err
	}
	if _, err := d.sql.Exec(`UPDATE project_envs SET name = ?, remark = ?, updated_at = ? WHERE id = ?`,
		name, remark, time.Now().UnixMilli(), id); err != nil {
		return ProjectEnv{}, err
	}
	_ = old
	return d.GetEnv(id)
}

// DeleteEnv 级联删除分支及其绑定。
func (d *DB) DeleteEnv(id int64) (bool, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := d.deleteSQLChangesByEnvTx(tx, id); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM project_env_dbs WHERE env_id = ?`, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM project_envs WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CreateEnvDB 绑定数据库（具体到某连接下的某个库）。
func (d *DB) CreateEnvDB(envID, instanceID int64, dbName, remark string) (EnvDB, error) {
	if _, err := d.GetEnv(envID); err != nil {
		return EnvDB{}, err
	}
	if _, err := d.GetInstance(instanceID); err != nil {
		return EnvDB{}, err
	}
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO project_env_dbs (env_id, instance_id, db_name, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, envID, instanceID, dbName, remark, now, now)
	if err != nil {
		return EnvDB{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return EnvDB{}, err
	}
	return d.GetEnvDB(id)
}

// GetEnvDB 按 id 查询绑定（含实例信息）。
func (d *DB) GetEnvDB(id int64) (EnvDB, error) {
	return scanEnvDB(d.sql.QueryRow(`SELECT `+envDBColumns+` `+envDBJoin+` WHERE b.id = ?`, id))
}

// UpdateEnvDB 更新绑定。
func (d *DB) UpdateEnvDB(id, instanceID int64, dbName, remark string) (EnvDB, error) {
	if _, err := d.GetEnvDB(id); err != nil {
		return EnvDB{}, err
	}
	if _, err := d.GetInstance(instanceID); err != nil {
		return EnvDB{}, err
	}
	if _, err := d.sql.Exec(`UPDATE project_env_dbs SET instance_id = ?, db_name = ?, remark = ?, updated_at = ? WHERE id = ?`,
		instanceID, dbName, remark, time.Now().UnixMilli(), id); err != nil {
		return EnvDB{}, err
	}
	return d.GetEnvDB(id)
}

// DeleteEnvDB 解绑（级联清理该绑定下的变更记录）。
func (d *DB) DeleteEnvDB(id int64) (bool, error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := d.deleteSQLChangesByBindingTx(tx, id); err != nil {
		return false, err
	}
	res, err := tx.Exec(`DELETE FROM project_env_dbs WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// CountBindingsByInstance 统计某连接被绑定的次数（删除实例前检查）。
func (d *DB) CountBindingsByInstance(instanceID int64) (int, error) {
	var n int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM project_env_dbs WHERE instance_id = ?`, instanceID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

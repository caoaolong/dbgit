package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// ProxyBackend 实例在某个 ProxySQL 节点上的代理映射。
// 约定：hostgroup = 1000 + instance_id（各节点独立，互不干扰）；代理账号 dbgit_<instance_id>，密码随机生成存库。
type ProxyBackend struct {
	InstanceID       int64  `json:"instance_id"`
	NodeID           int64  `json:"node_id"`
	HostgroupID      int    `json:"hostgroup_id"`
	FrontendUser     string `json:"frontend_user"`
	FrontendPassword string `json:"frontend_password"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// ProxyBackendView 代理卡片视图（实例 LEFT JOIN 映射）。
type ProxyBackendView struct {
	InstanceID   int64   `json:"instance_id"`
	InstanceName string  `json:"instance_name"`
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	DBType       string  `json:"db_type"`
	Version      string  `json:"version"`
	Remark       string  `json:"remark"`
	NodeID       int64   `json:"node_id"`
	HostgroupID  *int    `json:"hostgroup_id"`
	FrontendUser *string `json:"frontend_user"`
}

func (d *DB) migrateProxy() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS proxysql_backends (
		instance_id INTEGER NOT NULL REFERENCES db_instances(id),
		node_id INTEGER NOT NULL DEFAULT 0,
		hostgroup_id INTEGER NOT NULL,
		frontend_user TEXT NOT NULL,
		frontend_password TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (node_id, instance_id)
	)`)
	if err != nil {
		return err
	}
	// 存量单节点表：补 node_id 列。
	if !d.hasColumn("proxysql_backends", "node_id") {
		if _, err := d.sql.Exec(`ALTER TABLE proxysql_backends ADD COLUMN node_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// 存量单节点表：旧主键为 instance_id 单列（含 hostgroup_id/frontend_user 的单列 UNIQUE），
	// 会阻止多节点映射同一实例，需要重建为 (node_id, instance_id) 联合主键。
	if isLegacyProxyBackends(d) {
		if err := d.rebuildProxyBackends(); err != nil {
			return err
		}
	}
	_, err = d.sql.Exec(`CREATE INDEX IF NOT EXISTS idx_proxy_backends_node ON proxysql_backends(node_id)`)
	return err
}

// isLegacyProxyBackends 检测旧表结构（instance_id 为单列主键）。
func isLegacyProxyBackends(d *DB) bool {
	rows, err := d.sql.Query(`PRAGMA table_info(proxysql_backends)`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			continue
		}
		if name == "instance_id" && pk == 1 {
			return true
		}
	}
	return false
}

func (d *DB) rebuildProxyBackends() error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmts := []string{
		`CREATE TABLE proxysql_backends_new (
			instance_id INTEGER NOT NULL REFERENCES db_instances(id),
			node_id INTEGER NOT NULL DEFAULT 0,
			hostgroup_id INTEGER NOT NULL,
			frontend_user TEXT NOT NULL,
			frontend_password TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (node_id, instance_id)
		)`,
		`INSERT INTO proxysql_backends_new
			(instance_id, node_id, hostgroup_id, frontend_user, frontend_password, created_at, updated_at)
			SELECT instance_id, COALESCE(node_id, 0), hostgroup_id, frontend_user, frontend_password, created_at, updated_at
			FROM proxysql_backends`,
		`DROP TABLE proxysql_backends`,
		`ALTER TABLE proxysql_backends_new RENAME TO proxysql_backends`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("重建代理映射表失败: %w", err)
		}
	}
	return tx.Commit()
}

func genProxyPassword() string {
	var b [9]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "dbgit-proxy-123"
	}
	return "px-" + hex.EncodeToString(b[:])
}

// EnsureProxyBackend 获取某节点上的映射，不存在则创建（hostgroup 与账号按实例 id 确定性分配）。
func (d *DB) EnsureProxyBackend(nodeID, instanceID int64) (ProxyBackend, error) {
	var pb ProxyBackend
	var created, updated int64
	err := d.sql.QueryRow(`SELECT instance_id, node_id, hostgroup_id, frontend_user, frontend_password, created_at, updated_at
		FROM proxysql_backends WHERE node_id = ? AND instance_id = ?`, nodeID, instanceID).Scan(
		&pb.InstanceID, &pb.NodeID, &pb.HostgroupID, &pb.FrontendUser, &pb.FrontendPassword, &created, &updated)
	if err == nil {
		pb.CreatedAt = msToRFC3339(created)
		pb.UpdatedAt = msToRFC3339(updated)
		return pb, nil
	}
	if err != sql.ErrNoRows {
		return pb, err
	}
	now := time.Now().UnixMilli()
	pb = ProxyBackend{
		InstanceID:       instanceID,
		NodeID:           nodeID,
		HostgroupID:      1000 + int(instanceID),
		FrontendUser:     "dbgit_" + itoa(instanceID),
		FrontendPassword: genProxyPassword(),
	}
	if _, err := d.sql.Exec(`INSERT INTO proxysql_backends
		(instance_id, node_id, hostgroup_id, frontend_user, frontend_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		pb.InstanceID, pb.NodeID, pb.HostgroupID, pb.FrontendUser, pb.FrontendPassword, now, now); err != nil {
		return pb, err
	}
	pb.CreatedAt = msToRFC3339(now)
	pb.UpdatedAt = msToRFC3339(now)
	return pb, nil
}

// GetProxyBackend 按节点与实例取映射（无则 sql.ErrNoRows）。
func (d *DB) GetProxyBackend(nodeID, instanceID int64) (ProxyBackend, error) {
	var pb ProxyBackend
	var created, updated int64
	err := d.sql.QueryRow(`SELECT instance_id, node_id, hostgroup_id, frontend_user, frontend_password, created_at, updated_at
		FROM proxysql_backends WHERE node_id = ? AND instance_id = ?`, nodeID, instanceID).Scan(
		&pb.InstanceID, &pb.NodeID, &pb.HostgroupID, &pb.FrontendUser, &pb.FrontendPassword, &created, &updated)
	if err != nil {
		return pb, err
	}
	pb.CreatedAt = msToRFC3339(created)
	pb.UpdatedAt = msToRFC3339(updated)
	return pb, nil
}

// ListProxyBackendsByInstance 列出某实例在所有节点上的映射（删除实例时逐节点清理用）。
func (d *DB) ListProxyBackendsByInstance(instanceID int64) ([]ProxyBackend, error) {
	return d.listProxyBackends(`WHERE instance_id = ?`, instanceID)
}

// ListProxyBackendsByNode 列出某节点下的全部映射。
func (d *DB) ListProxyBackendsByNode(nodeID int64) ([]ProxyBackend, error) {
	return d.listProxyBackends(`WHERE node_id = ?`, nodeID)
}

func (d *DB) listProxyBackends(where string, args ...any) ([]ProxyBackend, error) {
	rows, err := d.sql.Query(`SELECT instance_id, node_id, hostgroup_id, frontend_user, frontend_password, created_at, updated_at
		FROM proxysql_backends `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProxyBackend, 0)
	for rows.Next() {
		var pb ProxyBackend
		var created, updated int64
		if err := rows.Scan(&pb.InstanceID, &pb.NodeID, &pb.HostgroupID, &pb.FrontendUser, &pb.FrontendPassword, &created, &updated); err != nil {
			return nil, err
		}
		pb.CreatedAt = msToRFC3339(created)
		pb.UpdatedAt = msToRFC3339(updated)
		out = append(out, pb)
	}
	return out, rows.Err()
}

// DeleteProxyBackend 删除某节点上的映射。
func (d *DB) DeleteProxyBackend(nodeID, instanceID int64) error {
	_, err := d.sql.Exec(`DELETE FROM proxysql_backends WHERE node_id = ? AND instance_id = ?`, nodeID, instanceID)
	return err
}

// DeleteProxyBackendsByNode 删除某节点下的全部映射（删除节点时用）。
func (d *DB) DeleteProxyBackendsByNode(nodeID int64) (int64, error) {
	res, err := d.sql.Exec(`DELETE FROM proxysql_backends WHERE node_id = ?`, nodeID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ListProxyBackendViews 某节点下所有实例与其代理映射（未同步的映射字段为 null）。
func (d *DB) ListProxyBackendViews(nodeID int64) ([]ProxyBackendView, error) {
	return d.listProxyBackendViews(`AND b.node_id = ?`, nodeID)
}

// ListAllProxyBackendViews 所有节点的映射视图（SQL 采集等全局场景用）。
func (d *DB) ListAllProxyBackendViews() ([]ProxyBackendView, error) {
	return d.listProxyBackendViews(``, 0)
}

func (d *DB) listProxyBackendViews(extra string, args ...any) ([]ProxyBackendView, error) {
	rows, err := d.sql.Query(`SELECT i.id, i.name, i.host, i.port, i.db_type, i.version, i.remark,
		b.node_id, b.hostgroup_id, b.frontend_user
		FROM db_instances i LEFT JOIN proxysql_backends b ON b.instance_id = i.id `+extra+`
		ORDER BY i.id ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProxyBackendView, 0)
	for rows.Next() {
		var v ProxyBackendView
		var version, remark, dbType sql.NullString
		var nodeID sql.NullInt64
		var hg sql.NullInt64
		var fu sql.NullString
		if err := rows.Scan(&v.InstanceID, &v.InstanceName, &v.Host, &v.Port,
			&dbType, &version, &remark, &nodeID, &hg, &fu); err != nil {
			return nil, err
		}
		v.DBType = dbType.String
		v.Version = version.String
		v.Remark = remark.String
		if nodeID.Valid {
			v.NodeID = nodeID.Int64
		}
		if hg.Valid {
			h := int(hg.Int64)
			v.HostgroupID = &h
		}
		if fu.Valid {
			u := fu.String
			v.FrontendUser = &u
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// InstanceNodeRef 实例的代理节点引用（实例卡片右上角展示用）。
type InstanceNodeRef struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// InstanceWithNodes 实例及其代理节点。
type InstanceWithNodes struct {
	Instance
	ProxyNodes []InstanceNodeRef `json:"proxy_nodes"`
}

// ListInstancesWithNodes 列出实例及其代理节点。
func (d *DB) ListInstancesWithNodes() ([]InstanceWithNodes, error) {
	instances, err := d.ListInstances()
	if err != nil {
		return nil, err
	}
	rows, err := d.sql.Query(`SELECT b.instance_id, n.id, n.name
		FROM proxysql_backends b JOIN proxy_nodes n ON n.id = b.node_id
		ORDER BY b.instance_id ASC, n.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byInstance := make(map[int64][]InstanceNodeRef)
	for rows.Next() {
		var iid, nid int64
		var name string
		if err := rows.Scan(&iid, &nid, &name); err != nil {
			return nil, err
		}
		byInstance[iid] = append(byInstance[iid], InstanceNodeRef{ID: nid, Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]InstanceWithNodes, 0, len(instances))
	for _, ins := range instances {
		nodes := byInstance[ins.ID]
		if nodes == nil {
			nodes = []InstanceNodeRef{}
		}
		out = append(out, InstanceWithNodes{Instance: ins, ProxyNodes: nodes})
	}
	return out, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

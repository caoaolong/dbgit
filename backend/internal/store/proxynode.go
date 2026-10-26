package store

import (
	"database/sql"
	"time"
)

// ProxyNode ProxySQL 代理节点（支持多实例，可分布在不同主机）。
// method: docker（本机容器，走 docker exec 管理）、package（本机系统包，走 TCP）、remote（远程主机，走 TCP）。
type ProxyNode struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Host          string `json:"host"`
	AdminPort     int    `json:"admin_port"`
	MySQLPort     int    `json:"mysql_port"`
	AdminUser     string `json:"admin_user"`
	AdminPassword string `json:"admin_password,omitempty"`
	Method        string `json:"method"`
	ContainerName string `json:"container_name"`
	// DockerHost 远端 docker 守护进程地址（如 ssh://ops@10.0.1.5），为空表示本机。
	DockerHost string `json:"docker_host"`
	Remark     string `json:"remark"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func (d *DB) migrateProxyNodes() error {
	_, err := d.sql.Exec(`CREATE TABLE IF NOT EXISTS proxy_nodes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		host TEXT NOT NULL DEFAULT '127.0.0.1',
		admin_port INTEGER NOT NULL DEFAULT 6032,
		mysql_port INTEGER NOT NULL DEFAULT 6033,
		admin_user TEXT NOT NULL DEFAULT 'admin',
		admin_password TEXT NOT NULL DEFAULT '',
		method TEXT NOT NULL DEFAULT 'remote',
		container_name TEXT NOT NULL DEFAULT '',
		docker_host TEXT NOT NULL DEFAULT '',
		remark TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	)`)
	if err != nil {
		return err
	}
	// 存量 proxysql_backends 补 node_id 列（老数据回填在 EnsureDefaultNode 中完成）。
	if !d.hasColumn("proxysql_backends", "node_id") {
		if _, err := d.sql.Exec(`ALTER TABLE proxysql_backends ADD COLUMN node_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	// 跨版本兼容：docker_host 列。
	if !d.hasColumn("proxy_nodes", "docker_host") {
		if _, err := d.sql.Exec(`ALTER TABLE proxy_nodes ADD COLUMN docker_host TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	return nil
}

const proxyNodeColumns = `id, name, host, admin_port, mysql_port, admin_user, admin_password,
	method, container_name, docker_host, remark, created_at, updated_at`

func scanProxyNode(row interface {
	Scan(dest ...any) error
}, withPassword bool) (ProxyNode, error) {
	var n ProxyNode
	var created, updated int64
	var password sql.NullString
	err := row.Scan(
		&n.ID, &n.Name, &n.Host, &n.AdminPort, &n.MySQLPort,
		&n.AdminUser, &password, &n.Method, &n.ContainerName, &n.DockerHost, &n.Remark,
		&created, &updated,
	)
	if err != nil {
		return ProxyNode{}, err
	}
	if withPassword {
		n.AdminPassword = password.String
	}
	n.CreatedAt = msToRFC3339(created)
	n.UpdatedAt = msToRFC3339(updated)
	return n, nil
}

// ListProxyNodes 列出全部代理节点。
func (d *DB) ListProxyNodes(withPassword bool) ([]ProxyNode, error) {
	rows, err := d.sql.Query(`SELECT ` + proxyNodeColumns + ` FROM proxy_nodes ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProxyNode, 0)
	for rows.Next() {
		n, err := scanProxyNode(rows, withPassword)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetProxyNode 按 id 查询节点。
func (d *DB) GetProxyNode(id int64, withPassword bool) (ProxyNode, error) {
	return scanProxyNode(d.sql.QueryRow(`SELECT `+proxyNodeColumns+` FROM proxy_nodes WHERE id = ?`, id), withPassword)
}

// CreateProxyNode 新增代理节点。
func (d *DB) CreateProxyNode(n ProxyNode) (ProxyNode, error) {
	now := time.Now().UnixMilli()
	res, err := d.sql.Exec(`INSERT INTO proxy_nodes
		(name, host, admin_port, mysql_port, admin_user, admin_password, method, container_name, docker_host, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		n.Name, n.Host, n.AdminPort, n.MySQLPort, n.AdminUser, n.AdminPassword,
		n.Method, n.ContainerName, n.DockerHost, n.Remark, now, now)
	if err != nil {
		return ProxyNode{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ProxyNode{}, err
	}
	return d.GetProxyNode(id, true)
}

// UpdateProxyNode 更新节点；password 为空表示保持原密码不变。
func (d *DB) UpdateProxyNode(id int64, n ProxyNode) (ProxyNode, error) {
	old, err := d.GetProxyNode(id, true)
	if err != nil {
		return ProxyNode{}, err
	}
	password := old.AdminPassword
	if n.AdminPassword != "" {
		password = n.AdminPassword
	}
	_, err = d.sql.Exec(`UPDATE proxy_nodes SET
		name = ?, host = ?, admin_port = ?, mysql_port = ?, admin_user = ?, admin_password = ?,
		method = ?, container_name = ?, docker_host = ?, remark = ?, updated_at = ? WHERE id = ?`,
		n.Name, n.Host, n.AdminPort, n.MySQLPort, n.AdminUser, password,
		n.Method, n.ContainerName, n.DockerHost, n.Remark, time.Now().UnixMilli(), id)
	if err != nil {
		return ProxyNode{}, err
	}
	return d.GetProxyNode(id, true)
}

// DeleteProxyNode 删除节点（调用方先清理其代理映射）。
func (d *DB) DeleteProxyNode(id int64) (bool, error) {
	res, err := d.sql.Exec(`DELETE FROM proxy_nodes WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// EnsureDefaultNode 无节点时创建一个默认本地节点，并把 node_id=0 的存量映射收编到它下面。
// 返回是否为新建。
func (d *DB) EnsureDefaultNode(fallback ProxyNode) (ProxyNode, bool, error) {
	nodes, err := d.ListProxyNodes(true)
	if err != nil {
		return ProxyNode{}, false, err
	}
	if len(nodes) > 0 {
		if _, err := d.sql.Exec(`UPDATE proxysql_backends SET node_id = ? WHERE node_id = 0`, nodes[0].ID); err != nil {
			return ProxyNode{}, false, err
		}
		return nodes[0], false, nil
	}
	now := time.Now().UnixMilli()
	if fallback.Name == "" {
		fallback.Name = "本地代理"
	}
	res, err := d.sql.Exec(`INSERT INTO proxy_nodes
		(name, host, admin_port, mysql_port, admin_user, admin_password, method, container_name, remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fallback.Name, fallback.Host, fallback.AdminPort, fallback.MySQLPort,
		fallback.AdminUser, fallback.AdminPassword, fallback.Method,
		fallback.ContainerName, fallback.Remark, now, now)
	if err != nil {
		return ProxyNode{}, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ProxyNode{}, false, err
	}
	if _, err := d.sql.Exec(`UPDATE proxysql_backends SET node_id = ? WHERE node_id = 0`, id); err != nil {
		return ProxyNode{}, false, err
	}
	n, err := d.GetProxyNode(id, true)
	return n, true, err
}

// CountBackendsByNode 统计某节点下的代理映射数（卡片只展示该个数）。
func (d *DB) CountBackendsByNode(nodeID int64) (int, error) {
	var n int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM proxysql_backends WHERE node_id = ?`, nodeID).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

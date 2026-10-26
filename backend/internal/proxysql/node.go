package proxysql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// NodeConfig 连接一个 ProxySQL 节点的参数。
// Container 非空表示 docker 容器，管理操作走 docker exec（本机或经 DockerHost 跳到远端）
// Container 为空表示本机系统包／二进制，走 TCP 直连 Admin 端口。
type NodeConfig struct {
	Host          string
	AdminPort     int
	MySQLPort     int
	AdminUser     string
	AdminPassword string
	Container     string
	// DockerHost 远端 docker 守护进程地址（如 ssh://ops@10.0.1.5），
	// 为空表示本机 docker。仅 Container 非空时有效。
	DockerHost string
}

func (n NodeConfig) addr() string {
	host := strings.TrimSpace(n.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := n.AdminPort
	if port <= 0 {
		port = DefaultAdminPort
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func (n NodeConfig) user() string {
	if strings.TrimSpace(n.AdminUser) == "" {
		return "admin"
	}
	return n.AdminUser
}

// ExecNodeSQL 在节点 Admin 上执行语句。
func ExecNodeSQL(ctx context.Context, n NodeConfig, stmts ...string) error {
	if n.Container != "" {
		return ExecAdmin(ctx, n.Container, n.user(), n.AdminPassword, n.DockerHost, stmts...)
	}
	db, err := openNodeAdmin(ctx, n)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, s := range stmts {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("proxysql admin 执行失败（%s）: %w", s, err)
		}
	}
	return nil
}

// QueryNodeValue 在节点 Admin 上查询单个标量值（如 SELECT VERSION()）。
func QueryNodeValue(ctx context.Context, n NodeConfig, query string) (string, error) {
	if n.Container != "" {
		out, err := QueryAdminRaw(ctx, n.Container, n.user(), n.AdminPassword, n.DockerHost, query, true)
		if err != nil {
			return "", err
		}
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSuffix(strings.TrimSpace(line), "\r")
			if line == "" || strings.HasPrefix(line, "WARNING:") {
				continue
			}
			if i := strings.Index(line, "\t"); i >= 0 {
				return strings.TrimSpace(line[:i]), nil
			}
			return line, nil
		}
		return "", nil
	}
	db, err := openNodeAdmin(ctx, n)
	if err != nil {
		return "", err
	}
	defer db.Close()
	var v any
	if err := db.QueryRowContext(ctx, query).Scan(&v); err != nil {
		return "", err
	}
	return cellString(v), nil
}

func openNodeAdmin(ctx context.Context, n NodeConfig) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/?timeout=5s&readTimeout=15s&writeTimeout=5s",
		url.QueryEscape(n.user()), url.QueryEscape(n.AdminPassword), n.addr())
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接 ProxySQL Admin 失败: %w", err)
	}
	return db, nil
}

func cellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return fmt.Sprintf("%d", t)
	case bool:
		if t {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", t)
	}
}

// NodeStatus 节点存活探测结果。
type NodeStatus struct {
	Running bool
	Version string
	Detail  string
}

// ProbeNodeStatus 探测节点存活并尽力获取版本。
// 经管理通道执行 SELECT VERSION()：成功即视为运行中（兼顾本机/远端容器与二进制部署）。
func ProbeNodeStatus(ctx context.Context, n NodeConfig) NodeStatus {
	var st NodeStatus
	v, err := QueryNodeValue(ctx, n, "SELECT VERSION()")
	if err != nil {
		st.Detail = "Admin 不可达: " + firstLine(err.Error())
		return st
	}
	st.Running = true
	if v = strings.TrimSpace(v); v != "" {
		st.Version = v
	}
	return st
}

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	if len(s) > 200 {
		return strings.TrimSpace(s[:200])
	}
	return strings.TrimSpace(s)
}

// SyncInstanceToNode 将一个后端实例推送为某节点的后端 + 前端账号（幂等）。
// 后端地址换算仅适用于本机容器；远端节点原样下发（需保证该节点可达后端）。
func SyncInstanceToNode(ctx context.Context, node NodeConfig, host string, port, hostgroup int, frontUser, frontPass string) error {
	targetHost, targetPort := strings.TrimSpace(host), port
	if node.Container != "" && node.DockerHost == "" {
		targetHost, targetPort = resolveBackendTarget(ctx, node.Container, host, port)
	}
	return ExecNodeSQL(ctx, node,
		fmt.Sprintf("DELETE FROM mysql_servers WHERE hostgroup_id = %d", hostgroup),
		fmt.Sprintf("INSERT INTO mysql_servers (hostgroup_id, hostname, port) VALUES (%d, '%s', %d)",
			hostgroup, escSQL(targetHost), targetPort),
		fmt.Sprintf("DELETE FROM mysql_users WHERE username = '%s'", escSQL(frontUser)),
		fmt.Sprintf(`INSERT INTO mysql_users (username, password, default_hostgroup, transaction_persistent) VALUES ('%s', '%s', %d, 1)`,
			escSQL(frontUser), escSQL(frontPass), hostgroup),
		"LOAD MYSQL SERVERS TO RUNTIME",
		"LOAD MYSQL USERS TO RUNTIME",
		"SAVE MYSQL SERVERS TO DISK",
		"SAVE MYSQL USERS TO DISK",
	)
}

// RemoveBackendFromNode 删除某节点上的后端与前端账号（幂等）。
func RemoveBackendFromNode(ctx context.Context, node NodeConfig, hostgroup int, frontUser string) error {
	return ExecNodeSQL(ctx, node,
		fmt.Sprintf("DELETE FROM mysql_servers WHERE hostgroup_id = %d", hostgroup),
		fmt.Sprintf("DELETE FROM mysql_users WHERE username = '%s'", escSQL(frontUser)),
		"LOAD MYSQL SERVERS TO RUNTIME",
		"LOAD MYSQL USERS TO RUNTIME",
		"SAVE MYSQL SERVERS TO DISK",
		"SAVE MYSQL USERS TO DISK",
	)
}

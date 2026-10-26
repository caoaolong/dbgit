package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/proxysql"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// ProxyNodes 代理节点（多 ProxySQL 实例，可分布在不同主机）。
type ProxyNodes struct {
	db *store.DB
	// 全局配置里的 Admin 账号：自动创建默认节点时回退使用。
	adminUser string
	adminPass string
}

// NewProxyNodes 创建 handler。
func NewProxyNodes(db *store.DB, adminUser, adminPass string) *ProxyNodes {
	return &ProxyNodes{db: db, adminUser: adminUser, adminPass: adminPass}
}

// nodeConfig 由节点记录构造连接参数。
func nodeConfig(n store.ProxyNode) proxysql.NodeConfig {
	return proxysql.NodeConfig{
		Host:          n.Host,
		AdminPort:     n.AdminPort,
		MySQLPort:     n.MySQLPort,
		AdminUser:     n.AdminUser,
		AdminPassword: n.AdminPassword,
		Container:     n.ContainerName,
		DockerHost:    n.DockerHost,
	}
}

// DefaultNode 无节点时创建默认本地节点（优先复用安装记录；密码回退用全局配置），并收编存量映射。
func DefaultNode(db *store.DB, adminUser, adminPass string) (store.ProxyNode, error) {
	fallback := store.ProxyNode{
		Name: "本地代理", Host: "127.0.0.1",
		AdminPort: 6032, MySQLPort: 6033,
		AdminUser: "admin", Method: "package",
	}
	if adminUser != "" {
		fallback.AdminUser = adminUser
	}
	fallback.AdminPassword = adminPass
	if s, ok := proxysql.LoadInstallState(); ok {
		if s.ContainerName != "" {
			fallback.Method = "docker"
			fallback.ContainerName = s.ContainerName
		} else if string(s.Method) == "package" {
			fallback.Method = "package"
		}
		if s.AdminPort > 0 {
			fallback.AdminPort = s.AdminPort
		}
		if s.MySQLPort > 0 {
			fallback.MySQLPort = s.MySQLPort
		}
	}
	n, _, err := db.EnsureDefaultNode(fallback)
	return n, err
}

// NodeCard 节点卡片：只展示数据库连接个数，外加基本信息与存活状态。
type NodeCard struct {
	store.ProxyNode
	Running      bool   `json:"running"`
	Version      string `json:"version,omitempty"`
	Detail       string `json:"detail,omitempty"`
	BackendCount int    `json:"backend_count"`
}

func (h *ProxyNodes) fillCard(ctx context.Context, n store.ProxyNode) NodeCard {
	card := NodeCard{ProxyNode: n}
	card.AdminPassword = "" // 列表不回显密码
	st := proxysql.ProbeNodeStatus(ctx, nodeConfig(n))
	card.Running = st.Running
	card.Version = st.Version
	card.Detail = st.Detail
	if c, err := h.db.CountBackendsByNode(n.ID); err == nil {
		card.BackendCount = c
	}
	return card
}

// ListNodes GET /api/v1/proxysql/nodes
func (h *ProxyNodes) ListNodes(c *gin.Context) {
	nodes, err := h.db.ListProxyNodes(true)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if len(nodes) == 0 {
		if n, err := DefaultNode(h.db, h.adminUser, h.adminPass); err == nil {
			nodes = []store.ProxyNode{n}
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	cards := make([]NodeCard, len(nodes))
	var wg sync.WaitGroup
	for i, n := range nodes {
		wg.Add(1)
		go func(i int, n store.ProxyNode) {
			defer wg.Done()
			cards[i] = h.fillCard(ctx, n)
		}(i, n)
	}
	wg.Wait()
	response.OK(c, cards)
}

// GetNode GET /api/v1/proxysql/nodes/:id（含存活状态与连接数）
func (h *ProxyNodes) GetNode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	n, err := h.db.GetProxyNode(id, true)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "代理节点不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	defer cancel()
	response.OK(c, h.fillCard(ctx, n))
}

type nodeForm struct {
	Name          string `json:"name"`
	Host          string `json:"host"`
	AdminPort     int    `json:"admin_port"`
	MySQLPort     int    `json:"mysql_port"`
	AdminUser     string `json:"admin_user"`
	AdminPassword string `json:"admin_password"`
	ContainerName string `json:"container_name"`
	DockerHost    string `json:"docker_host"`
	Remark        string `json:"remark"`
}

func (f *nodeForm) normalize() {
	f.Name = strings.TrimSpace(f.Name)
	f.Host = strings.TrimSpace(f.Host)
	f.AdminUser = strings.TrimSpace(f.AdminUser)
	f.ContainerName = strings.TrimSpace(f.ContainerName)
	f.DockerHost = strings.TrimSpace(f.DockerHost)
	f.Remark = strings.TrimSpace(f.Remark)
}

func (f *nodeForm) validate() error {
	if f.Name == "" {
		return errors.New("节点名称不能为空")
	}
	if len(f.Name) > 64 {
		return errors.New("节点名称过长（最多 64 字符）")
	}
	if f.Host == "" {
		return errors.New("主机地址不能为空")
	}
	if f.AdminPort < 1 || f.AdminPort > 65535 {
		return errors.New("管理端口非法（1-65535）")
	}
	if f.MySQLPort < 1 || f.MySQLPort > 65535 {
		return errors.New("MySQL 端口非法（1-65535）")
	}
	if len(f.Remark) > 256 {
		return errors.New("备注过长（最多 256 字符）")
	}
	// 远端容器必须同时给出 docker 守护进程地址（Docker-over-SSH，如 ssh://ops@10.0.1.5）。
	if f.DockerHost != "" {
		if !strings.HasPrefix(f.DockerHost, "ssh://") {
			return errors.New("远端 docker 地址须为 ssh://user@host[:port] 格式")
		}
		if f.ContainerName == "" {
			return errors.New("远端节点必须填写容器名")
		}
		if isLocalHost(f.Host) {
			return errors.New("远端节点的代理主机不能是本机地址")
		}
	}
	return nil
}

func (h *ProxyNodes) toStore(f nodeForm) store.ProxyNode {
	user := f.AdminUser
	if user == "" {
		user = "admin"
	}
	host := f.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return store.ProxyNode{
		Name: f.Name, Host: host,
		AdminPort: f.AdminPort, MySQLPort: f.MySQLPort,
		AdminUser: user, AdminPassword: f.AdminPassword,
		Method: methodFor(f), ContainerName: f.ContainerName, DockerHost: f.DockerHost,
		Remark: f.Remark,
	}
}

func methodFor(f nodeForm) string {
	if f.ContainerName != "" {
		return "docker"
	}
	return "package"
}

func isLocalHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1", "":
		return true
	default:
		return false
	}
}

// probeOrFail 探测节点 Admin 是否可达，不可达则返回用户可读错误。
func probeOrFail(ctx context.Context, n store.ProxyNode) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	st := proxysql.ProbeNodeStatus(ctx, nodeConfig(n))
	if !st.Running {
		msg := "节点 Admin 端口不可达，请检查主机、端口与防火墙"
		if st.Detail != "" {
			msg += "（" + st.Detail + "）"
		}
		return errors.New(msg)
	}
	return nil
}

// CreateNode POST /api/v1/proxysql/nodes（要求节点当前可达）
func (h *ProxyNodes) CreateNode(c *gin.Context) {
	var f nodeForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := f.validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	n := h.toStore(f)
	if err := probeOrFail(c.Request.Context(), n); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	created, err := h.db.CreateProxyNode(n)
	if err != nil {
		if store.IsUniqueViolation(err) {
			response.Abort(c, http.StatusBadRequest, 400, "节点名称已存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "创建失败: "+err.Error())
		return
	}
	created.AdminPassword = ""
	response.OK(c, created)
}

// UpdateNode PUT /api/v1/proxysql/nodes/:id（密码为空表示保持不变；要求节点当前可达）
func (h *ProxyNodes) UpdateNode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f nodeForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := f.validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	n := h.toStore(f)
	if err := probeOrFail(c.Request.Context(), n); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	updated, err := h.db.UpdateProxyNode(id, n)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "代理节点不存在")
		case store.IsUniqueViolation(err):
			response.Abort(c, http.StatusBadRequest, 400, "节点名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "更新失败: "+err.Error())
		}
		return
	}
	updated.AdminPassword = ""
	response.OK(c, updated)
}

// DeleteNode DELETE /api/v1/proxysql/nodes/:id（尽力清理该节点上的代理配置，再删映射与节点）
func (h *ProxyNodes) DeleteNode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	n, err := h.db.GetProxyNode(id, true)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "代理节点不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if pbs, err := h.db.ListProxyBackendsByNode(n.ID); err == nil {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		cfg := nodeConfig(n)
		for _, pb := range pbs {
			if err := proxysql.RemoveBackendFromNode(ctx, cfg, pb.HostgroupID, pb.FrontendUser); err != nil {
				slog.Warn("删除节点时清理代理配置失败", "node", n.Name, "instance", pb.InstanceID, "err", err)
			}
		}
	}
	if _, err := h.db.DeleteProxyBackendsByNode(n.ID); err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "清理映射失败: "+err.Error())
		return
	}
	deleted, err := h.db.DeleteProxyNode(n.ID)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "删除失败: "+err.Error())
		return
	}
	if !deleted {
		response.Abort(c, http.StatusNotFound, 404, "代理节点不存在")
		return
	}
	response.OK(c, gin.H{"id": id})
}

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dbgit/dbgit/internal/dbprobe"
	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/proxysql"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// ProxyBackends ProxySQL 代理的后端连接。
type ProxyBackends struct {
	db *store.DB
	// 全局配置里的 Admin 账号：默认节点回退使用。
	adminUser string
	adminPass string
}

// NewProxyBackends 创建 handler。
func NewProxyBackends(db *store.DB, adminUser, adminPass string) *ProxyBackends {
	return &ProxyBackends{db: db, adminUser: adminUser, adminPass: adminPass}
}

// BackendCard 代理卡片。
type BackendCard struct {
	InstanceID       int64   `json:"instance_id"`
	InstanceName     string  `json:"instance_name"`
	Host             string  `json:"host"`
	Port             int     `json:"port"`
	Version          string  `json:"version"`
	NodeID           int64   `json:"node_id"`
	HostgroupID      *int    `json:"hostgroup_id"`
	FrontendUser     *string `json:"frontend_user"`
	FrontendPassword string  `json:"frontend_password,omitempty"`
	MySQLHost        string  `json:"mysql_host"`
	MySQLPort        int     `json:"mysql_port"`
	// status: online | error | unsynced
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// resolveNode 解析 ?node_id=，缺省为默认节点。
func (h *ProxyBackends) resolveNode(c *gin.Context) (store.ProxyNode, bool) {
	var nodeID int64
	if v := c.Query("node_id"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			nodeID = n
		}
	}
	if nodeID > 0 {
		n, err := h.db.GetProxyNode(nodeID, true)
		if err != nil {
			if isNotFound(err) {
				response.Abort(c, http.StatusNotFound, 404, "代理节点不存在")
			} else {
				response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
			}
			return store.ProxyNode{}, false
		}
		return n, true
	}
	n, err := DefaultNode(h.db, h.adminUser, h.adminPass)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return store.ProxyNode{}, false
	}
	return n, true
}

// List GET /api/v1/proxysql/backends?node_id=（逐个经该节点前端探测连通性）
func (h *ProxyBackends) List(c *gin.Context) {
	node, ok := h.resolveNode(c)
	if !ok {
		return
	}
	views, err := h.db.ListProxyBackendViews(node.ID)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	cards := make([]BackendCard, 0, len(views))
	for _, v := range views {
		card := BackendCard{
			InstanceID:   v.InstanceID,
			InstanceName: v.InstanceName,
			Host:         v.Host,
			Port:         v.Port,
			Version:      v.Version,
			NodeID:       node.ID,
			HostgroupID:  v.HostgroupID,
			FrontendUser: v.FrontendUser,
			MySQLHost:    node.Host,
			MySQLPort:    node.MySQLPort,
			Status:       "unsynced",
			Message:      "尚未同步到该 ProxySQL",
		}
		if v.HostgroupID != nil && v.FrontendUser != nil {
			pb, err := h.db.EnsureProxyBackend(node.ID, v.InstanceID)
			if err == nil {
				card.FrontendPassword = pb.FrontendPassword
				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				ver, err := dbprobe.Version(ctx, node.Host, node.MySQLPort, *v.FrontendUser, pb.FrontendPassword)
				cancel()
				if err != nil {
					card.Status = "error"
					card.Message = "代理连接失败: " + err.Error()
				} else {
					card.Status = "online"
					card.Message = "经 ProxySQL 探测版本: " + ver
				}
			} else {
				card.Status = "error"
				card.Message = "读取映射失败: " + err.Error()
			}
		}
		cards = append(cards, card)
	}
	response.OK(c, cards)
}

// Resync POST /api/v1/proxysql/resync（全量重推代理配置；传 node_id 则只推该节点）
func (h *ProxyBackends) Resync(c *gin.Context) {
	var f struct {
		NodeID int64 `json:"node_id"`
	}
	_ = c.ShouldBindJSON(&f)
	var nodes []store.ProxyNode
	if f.NodeID > 0 {
		n, err := h.db.GetProxyNode(f.NodeID, true)
		if err != nil {
			if isNotFound(err) {
				response.Abort(c, http.StatusNotFound, 404, "代理节点不存在")
			} else {
				response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
			}
			return
		}
		nodes = []store.ProxyNode{n}
	} else {
		var err error
		nodes, err = h.db.ListProxyNodes(true)
		if err != nil {
			response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
			return
		}
		if len(nodes) == 0 {
			if n, err := DefaultNode(h.db, h.adminUser, h.adminPass); err == nil {
				nodes = []store.ProxyNode{n}
			}
		}
	}
	instances, err := h.db.ListInstances()
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	ok, failed := 0, make([]string, 0)
	for _, n := range nodes {
		for _, ins := range instances {
			if err := SyncProxyForInstance(h.db, n, ins.ID); err != nil {
				failed = append(failed, fmt.Sprintf("%s/%s: %v", n.Name, ins.Name, err))
				continue
			}
			ok++
		}
	}
	if len(failed) > 0 {
		response.Fail(c, 1, fmt.Sprintf("部分同步失败（成功 %d）: %v", ok, failed))
		return
	}
	response.OK(c, gin.H{"synced": ok})
}

// SyncProxyForInstance 同步单个实例到指定节点（创建映射 + 推送配置）。
// ProxySQL 不可用时返回错误，由调用方决定是否阻塞主流程。
func SyncProxyForInstance(db *store.DB, node store.ProxyNode, instanceID int64) error {
	ins, err := db.GetInstance(instanceID)
	if err != nil {
		return fmt.Errorf("实例不存在: %w", err)
	}
	pb, err := db.EnsureProxyBackend(node.ID, instanceID)
	if err != nil {
		return fmt.Errorf("创建映射失败: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// 先在后端实例上建好代理账号，再推送 ProxySQL 配置。
	if err := dbprobe.EnsureUser(ctx, ins.Host, ins.Port, ins.Username, ins.Password, pb.FrontendUser, pb.FrontendPassword); err != nil {
		return fmt.Errorf("创建后端代理账号失败: %w", err)
	}
	if err := proxysql.SyncInstanceToNode(ctx, nodeConfig(node),
		ins.Host, ins.Port, pb.HostgroupID, pb.FrontendUser, pb.FrontendPassword); err != nil {
		return err
	}
	return nil
}

// SyncInstanceEverywhere 同步实例到所有节点（逐个尽力而为）。
func SyncInstanceEverywhere(db *store.DB, instanceID int64) (int, []string) {
	nodes, err := db.ListProxyNodes(true)
	if err != nil {
		return 0, []string{"查询节点失败: " + err.Error()}
	}
	ok, failed := 0, make([]string, 0)
	for _, n := range nodes {
		if err := SyncProxyForInstance(db, n, instanceID); err != nil {
			failed = append(failed, n.Name+": "+err.Error())
			continue
		}
		ok++
	}
	return ok, failed
}

// RemoveProxyEverywhere 删除实例在所有节点上的代理配置与映射（尽力而为）。
func RemoveProxyEverywhere(db *store.DB, instanceID int64) {
	pbs, err := db.ListProxyBackendsByInstance(instanceID)
	if err != nil {
		slog.Warn("清理代理映射失败：查询失败", "instance", instanceID, "err", err)
		return
	}
	for _, pb := range pbs {
		node, err := db.GetProxyNode(pb.NodeID, true)
		if err != nil {
			// 节点已删，只清理映射行。
			if derr := db.DeleteProxyBackend(pb.NodeID, pb.InstanceID); derr != nil {
				slog.Warn("清理代理映射失败", "instance", pb.InstanceID, "err", derr)
			}
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		rerr := proxysql.RemoveBackendFromNode(ctx, nodeConfig(node), pb.HostgroupID, pb.FrontendUser)
		cancel()
		if rerr != nil {
			slog.Warn("清理代理配置失败", "node", node.Name, "instance", pb.InstanceID, "err", rerr)
		}
		if derr := db.DeleteProxyBackend(pb.NodeID, pb.InstanceID); derr != nil {
			slog.Warn("清理代理映射失败", "instance", pb.InstanceID, "err", derr)
		}
	}
}

// SyncAllProxies 启动时全量同步所有节点（尽力而为，只记日志）。
func SyncAllProxies(db *store.DB) {
	nodes, err := db.ListProxyNodes(true)
	if err != nil {
		slog.Warn("代理全量同步：查询节点失败", "err", err)
		return
	}
	instances, err := db.ListInstances()
	if err != nil {
		slog.Warn("代理全量同步：查询实例失败", "err", err)
		return
	}
	for _, n := range nodes {
		for _, ins := range instances {
			if err := SyncProxyForInstance(db, n, ins.ID); err != nil {
				slog.Warn("代理全量同步失败", "node", n.Name, "instance", ins.Name, "err", err)
				continue
			}
			slog.Info("代理已同步", "node", n.Name, "instance", ins.Name)
		}
	}
}

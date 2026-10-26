package handler

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dbgit/dbgit/internal/dbprobe"
	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// Instances 数据库实例 CRUD。
type Instances struct {
	db *store.DB
}

// NewInstances 创建 handler。
func NewInstances(db *store.DB) *Instances {
	return &Instances{db: db}
}

var allowedDBTypes = map[string]bool{"mysql": true}

type instanceForm struct {
	Name     string `json:"name"`
	DBType   string `json:"db_type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Remark   string `json:"remark"`
}

func (f *instanceForm) normalize() {
	f.Name = strings.TrimSpace(f.Name)
	f.DBType = strings.TrimSpace(f.DBType)
	f.Host = strings.TrimSpace(f.Host)
	f.Username = strings.TrimSpace(f.Username)
	f.Remark = strings.TrimSpace(f.Remark)
	if f.DBType == "" {
		f.DBType = "mysql"
	}
	if f.Port == 0 {
		f.Port = 3306
	}
}

func (f *instanceForm) validate() error {
	switch {
	case f.Name == "":
		return errors.New("名称不能为空")
	case len(f.Name) > 64:
		return errors.New("名称过长（最多 64 字符）")
	case !allowedDBTypes[f.DBType]:
		return errors.New("类型仅支持 mysql")
	case f.Host == "":
		return errors.New("主机地址不能为空")
	case f.Port < 1 || f.Port > 65535:
		return errors.New("端口非法（1-65535）")
	case len(f.Remark) > 256:
		return errors.New("备注过长（最多 256 字符）")
	}
	return nil
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Abort(c, http.StatusBadRequest, 400, "非法 id")
		return 0, false
	}
	return id, true
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

// List GET /api/v1/instances（不含密码，含各实例的代理节点）
func (h *Instances) List(c *gin.Context) {
	list, err := h.db.ListInstancesWithNodes()
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// Get GET /api/v1/instances/:id（含密码，供编辑回显）
func (h *Instances) Get(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ins, err := h.db.GetInstance(id)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "实例不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	response.OK(c, ins)
}

// probeVersion 连接实例探测版本号；失败返回空字符串且只记日志，不阻塞保存。
func probeVersion(host string, port int, user, password string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	v, err := dbprobe.Version(ctx, host, port, user, password)
	if err != nil {
		slog.Warn("版本探测失败", "host", host, "port", port, "err", err)
		return ""
	}
	return v
}

// Create POST /api/v1/instances
func (h *Instances) Create(c *gin.Context) {
	var f instanceForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := f.validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	ins, err := h.db.CreateInstance(store.Instance{
		Name: f.Name, DBType: f.DBType, Host: f.Host, Port: f.Port,
		Username: f.Username, Password: f.Password, Remark: f.Remark,
		Version: probeVersion(f.Host, f.Port, f.Username, f.Password),
	})
	if err != nil {
		if store.IsUniqueViolation(err) {
			response.Abort(c, http.StatusBadRequest, 400, "名称已存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "创建失败: "+err.Error())
		return
	}
	// 自动配置 ProxySQL 代理（同步到所有节点，失败不阻塞创建，卡片上会显示未同步）
	if ok, failed := SyncInstanceEverywhere(h.db, ins.ID); len(failed) > 0 {
		slog.Warn("自动配置 ProxySQL 代理部分失败", "instance", ins.Name, "ok", ok, "failed", failed)
	}
	response.OK(c, ins)
}

// Update PUT /api/v1/instances/:id（password 为空表示保持原密码）
func (h *Instances) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f instanceForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := f.validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	// 用提交的连接信息重新探测；密码为空则沿用库中旧密码探测。
	password := f.Password
	if password == "" {
		if old, err := h.db.GetInstance(id); err == nil {
			password = old.Password
		}
	}
	ins, err := h.db.UpdateInstance(id, store.Instance{
		Name: f.Name, DBType: f.DBType, Host: f.Host, Port: f.Port,
		Username: f.Username, Password: f.Password, Remark: f.Remark,
		Version: probeVersion(f.Host, f.Port, f.Username, password),
	})
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "实例不存在")
		case store.IsUniqueViolation(err):
			response.Abort(c, http.StatusBadRequest, 400, "名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "更新失败: "+err.Error())
		}
		return
	}
	// 连接信息可能变更，重新推送各节点代理配置（失败不阻塞更新）
	if ok, failed := SyncInstanceEverywhere(h.db, ins.ID); len(failed) > 0 {
		slog.Warn("自动配置 ProxySQL 代理部分失败", "instance", ins.Name, "ok", ok, "failed", failed)
	}
	response.OK(c, ins)
}

// probeConn 探测连接参数（表单测试连接用，不落库）。
type probeConn struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// Probe POST /api/v1/instances/probe（不落库，仅连通性 + 版本探测）
func (h *Instances) Probe(c *gin.Context) {
	var p probeConn
	if err := c.ShouldBindJSON(&p); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	p.Host = strings.TrimSpace(p.Host)
	if p.Host == "" {
		response.Abort(c, http.StatusBadRequest, 400, "主机地址不能为空")
		return
	}
	if p.Port < 1 || p.Port > 65535 {
		response.Abort(c, http.StatusBadRequest, 400, "端口非法（1-65535）")
		return
	}
	v := probeVersion(p.Host, p.Port, strings.TrimSpace(p.Username), p.Password)
	if v == "" {
		response.Abort(c, http.StatusBadGateway, 502, "连接失败，请检查地址、端口与账号密码")
		return
	}
	response.OK(c, gin.H{"version": v})
}

// Databases GET /api/v1/instances/:id/databases（用库中账号列出用户库）
func (h *Instances) Databases(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ins, err := h.db.GetInstance(id)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "实例不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	dbs, err := dbprobe.Databases(ctx, ins.Host, ins.Port, ins.Username, ins.Password)
	if err != nil {
		response.Abort(c, http.StatusBadGateway, 502, "连接实例失败: "+err.Error())
		return
	}
	response.OK(c, dbs)
}

// Delete DELETE /api/v1/instances/:id（被项目环境绑定时拒绝删除）
func (h *Instances) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	n, err := h.db.CountBindingsByInstance(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "检查绑定失败: "+err.Error())
		return
	}
	if n > 0 {
		response.Abort(c, http.StatusBadRequest, 400, "该连接已被项目环境绑定，请先解绑")
		return
	}
	deleted, err := h.db.DeleteInstance(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "删除失败: "+err.Error())
		return
	}
	if !deleted {
		response.Abort(c, http.StatusNotFound, 404, "实例不存在")
		return
	}
	// 同步清理其在所有节点上的 ProxySQL 代理配置与映射
	RemoveProxyEverywhere(h.db, id)
	response.OK(c, gin.H{"id": id})
}

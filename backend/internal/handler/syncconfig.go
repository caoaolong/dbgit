package handler

import (
	"net/http"
	"strings"

	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/dbgit/dbgit/internal/syncignore"
	"github.com/gin-gonic/gin"
)

// SyncConfig 项目同步配置（类 .gitignore 忽略规则）。
type SyncConfig struct {
	db *store.DB
}

// NewSyncConfig 创建 handler。
func NewSyncConfig(db *store.DB) *SyncConfig {
	return &SyncConfig{db: db}
}

// GetSyncConfig GET /api/v1/projects/:id/sync-config
func (h *SyncConfig) GetSyncConfig(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if _, err := h.db.GetProject(id); err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "项目不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	content, err := h.db.GetSyncConfig(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"project_id": id, "content": content})
}

type syncConfigForm struct {
	Content string `json:"content"`
	Comment string `json:"comment"`
}

// UpdateSyncConfig PUT /api/v1/projects/:id/sync-config
// 保存快照的同时追加一条版本记录，分支树上会生成一条提交。
func (h *SyncConfig) UpdateSyncConfig(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f syncConfigForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	rules, err := syncignore.Parse(f.Content)
	if err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "规则非法: "+err.Error())
		return
	}
	comment := strings.TrimSpace(f.Comment)
	if len(comment) > 256 {
		response.Abort(c, http.StatusBadRequest, 400, "说明过长（最多 256 字符）")
		return
	}
	if _, err := h.db.GetProject(id); err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "项目不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if err := h.db.UpsertSyncConfig(id, f.Content); err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "保存失败: "+err.Error())
		return
	}
	revID, err := h.db.InsertSyncConfigRevision(id, f.Content, len(rules), comment)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "保存版本失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"project_id": id, "content": f.Content, "revision_id": revID})
}

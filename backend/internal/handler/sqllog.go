package handler

import (
	"net/http"
	"strconv"

	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/sqllog"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// SQLLog 变更记录（SQL 日志）接口。
type SQLLog struct {
	db        *store.DB
	collector *sqllog.Collector
}

// NewSQLLog 创建 handler。
func NewSQLLog(db *store.DB, c *sqllog.Collector) *SQLLog {
	return &SQLLog{db: db, collector: c}
}

// EventsLogStatus GET /api/v1/proxysql/eventslog/status（采集状态）。
func (h *SQLLog) EventsLogStatus(c *gin.Context) {
	response.OK(c, h.collector.Status())
}

// EventsLogCollect POST /api/v1/proxysql/eventslog/collect（手动触发一次采集）。
func (h *SQLLog) EventsLogCollect(c *gin.Context) {
	scanned, inserted, err := h.collector.CollectOnce(c.Request.Context())
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "采集失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"scanned": scanned, "inserted": inserted})
}

// BindingChanges GET /api/v1/bindings/:id/changes（某绑定库的变更记录）。
func (h *SQLLog) BindingChanges(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if _, err := h.db.GetEnvDB(id); err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "绑定不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	f := store.SQLChangeFilter{
		Keyword:  c.Query("q"),
		StmtType: c.Query("stmt_type"),
	}
	if v := c.Query("since"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.Since = n
		}
	}
	if v := c.Query("until"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			f.Until = n
		}
	}
	if v := c.Query("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Page = n
		}
	}
	if v := c.Query("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.PageSize = n
		}
	}
	list, err := h.db.ListSQLChanges(id, f)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	total, err := h.db.CountSQLChanges(id, f)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	page, pageSize := f.Page, f.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 20
	}
	response.OK(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

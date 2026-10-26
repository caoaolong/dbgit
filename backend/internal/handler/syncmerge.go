package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// SyncMerges 同步合并记录。
type SyncMerges struct {
	db *store.DB
}

// NewSyncMerges 创建 handler。
func NewSyncMerges(db *store.DB) *SyncMerges {
	return &SyncMerges{db: db}
}

type mergeForm struct {
	TargetEnvID int64  `json:"target_env_id"`
	Comment     string `json:"comment"`
}

// CreateMerge POST /api/v1/bindings/:id/merges
// 把某绑定库的变更合并到目标分支（仅记录合并意图与快照，实际数据同步后续实现）。
func (h *SyncMerges) CreateMerge(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f mergeForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.Comment = strings.TrimSpace(f.Comment)
	if f.TargetEnvID <= 0 {
		response.Abort(c, http.StatusBadRequest, 400, "请选择要合并到的分支")
		return
	}
	if f.Comment == "" {
		response.Abort(c, http.StatusBadRequest, 400, "请填写本次合并的说明")
		return
	}
	if len(f.Comment) > 256 {
		response.Abort(c, http.StatusBadRequest, 400, "合并说明过长（最多 256 字符）")
		return
	}
	src, err := h.db.GetEnvDB(id)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "绑定不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	srcEnv, err := h.db.GetEnv(src.EnvID)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	tgtEnv, err := h.db.GetEnv(f.TargetEnvID)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "目标分支不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if tgtEnv.ProjectID != srcEnv.ProjectID {
		response.Abort(c, http.StatusBadRequest, 400, "只能合并到同一项目下的分支")
		return
	}
	if tgtEnv.ID == srcEnv.ID {
		response.Abort(c, http.StatusBadRequest, 400, "目标分支不能与来源分支相同")
		return
	}
	// 在目标分支中按同名库解析合并目标绑定（没有则记 0）。
	var targetBindingID int64
	var targetDBName string
	if dbs, err := h.db.ListEnvDBs(tgtEnv.ID); err == nil {
		for _, b := range dbs {
			if strings.EqualFold(b.DBName, src.DBName) {
				targetBindingID = b.ID
				targetDBName = b.DBName
				break
			}
		}
	}
	stmtCount, err := h.db.CountSQLChanges(src.ID, store.SQLChangeFilter{})
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	// 判定共同基线：目标库与来源同名，或存在克隆血缘。
	hasBase := targetBindingID != 0 && strings.EqualFold(src.DBName, targetDBName)
	if !hasBase && targetBindingID != 0 {
		if ok, _ := h.db.HasCloneLineage(src.ID, targetBindingID); ok {
			hasBase = true
		}
	}
	mergeID, err := h.db.CreateSyncMerge(srcEnv.ProjectID, src.ID, tgtEnv.ID, targetBindingID, f.Comment, stmtCount, hasBase)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "创建合并记录失败: "+err.Error())
		return
	}
	// 在来源与目标分支各落一条 MERGE 提交：源分支记录发起，目标分支（若有绑定）记录接收。
	nowMs := time.Now().UnixMilli()
	markSQL := fmt.Sprintf("-- 合并：%s（%s → %s）", f.Comment, srcEnv.Name, tgtEnv.Name)
	if !hasBase {
		markSQL += " · 无共同基线"
	}
	markRows := []store.SQLChange{{
		BindingID:  src.ID,
		StmtType:   "MERGE",
		SQLText:    markSQL,
		Username:   "dbgit",
		Schemaname: src.DBName,
	}}
	markDedups := []string{fmt.Sprintf("merge-marker-%d-src", mergeID)}
	markTimes := []int64{nowMs}
	if targetBindingID != 0 {
		var targetDBName string
		if dbs, _ := h.db.ListEnvDBs(tgtEnv.ID); dbs != nil {
			for _, b := range dbs {
				if b.ID == targetBindingID {
					targetDBName = b.DBName
					break
				}
			}
		}
		markRows = append(markRows, store.SQLChange{
			BindingID:  targetBindingID,
			StmtType:   "MERGE",
			SQLText:    markSQL,
			Username:   "dbgit",
			Schemaname: targetDBName,
		})
		markDedups = append(markDedups, fmt.Sprintf("merge-marker-%d-dst", mergeID))
		markTimes = append(markTimes, nowMs)
	}
	if _, err := h.db.InsertSQLChanges(markRows, markDedups, markTimes); err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "写入合并标记失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{
		"id":                mergeID,
		"source_binding_id": src.ID,
		"target_env_id":     tgtEnv.ID,
		"target_binding_id": targetBindingID,
		"stmt_count":        stmtCount,
		"has_base":          hasBase,
	})
}

type cloneForm struct {
	TargetEnvID     int64  `json:"target_env_id"`
	TargetBindingID int64  `json:"target_binding_id"`
	Comment         string `json:"comment"`
}

// CreateClone POST /api/v1/bindings/:id/clone
// 把来源库完整克隆到目标分支的某个库上（覆盖式，不产生新库；目标分支为空时才自动创建同名库）：
// 记录克隆血缘并在目标库上生成一条 CLONE 提交，为后续合并建立共同基线。
// 注：当前仅记录血缘与标记，实际数据搬运由后续同步执行完成。
func (h *SyncMerges) CreateClone(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f cloneForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.Comment = strings.TrimSpace(f.Comment)
	if f.TargetEnvID <= 0 {
		response.Abort(c, http.StatusBadRequest, 400, "请选择要克隆到的分支")
		return
	}
	if len(f.Comment) > 256 {
		response.Abort(c, http.StatusBadRequest, 400, "说明过长（最多 256 字符）")
		return
	}
	src, err := h.db.GetEnvDB(id)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "绑定不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	srcEnv, err := h.db.GetEnv(src.EnvID)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	tgtEnv, err := h.db.GetEnv(f.TargetEnvID)
	if err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "目标分支不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if tgtEnv.ProjectID != srcEnv.ProjectID {
		response.Abort(c, http.StatusBadRequest, 400, "只能克隆到同一项目下的分支")
		return
	}
	if tgtEnv.ID == srcEnv.ID {
		response.Abort(c, http.StatusBadRequest, 400, "目标分支不能与来源分支相同")
		return
	}
	tgtDBs, err := h.db.ListEnvDBs(tgtEnv.ID)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	// 解析目标库：显式指定 > 同名库 > 唯一库；空分支则自动创建同名库。
	var target *store.EnvDB
	created := false
	if f.TargetBindingID > 0 {
		for i := range tgtDBs {
			if tgtDBs[i].ID == f.TargetBindingID {
				target = &tgtDBs[i]
				break
			}
		}
		if target == nil {
			response.Abort(c, http.StatusBadRequest, 400, "目标库不属于目标分支")
			return
		}
		if target.ID == src.ID {
			response.Abort(c, http.StatusBadRequest, 400, "目标库不能与来源库相同")
			return
		}
	} else {
		for i := range tgtDBs {
			if strings.EqualFold(tgtDBs[i].DBName, src.DBName) {
				target = &tgtDBs[i]
				break
			}
		}
		if target == nil && len(tgtDBs) == 1 {
			target = &tgtDBs[0]
		}
		if target == nil && len(tgtDBs) == 0 {
			remark := f.Comment
			if remark == "" {
				remark = "从 " + srcEnv.Name + " 克隆"
			}
			t, err := h.db.CreateEnvDB(tgtEnv.ID, src.InstanceID, src.DBName, remark)
			if err != nil {
				if store.IsUniqueViolation(err) {
					response.Abort(c, http.StatusBadRequest, 400, "目标分支已存在同名库")
					return
				}
				response.Abort(c, http.StatusInternalServerError, 500, "创建目标绑定失败: "+err.Error())
				return
			}
			target = &t
			created = true
		}
		if target == nil {
			response.Abort(c, http.StatusBadRequest, 400, "请明确选择要覆盖的目标库")
			return
		}
	}
	stmtCount, err := h.db.CountSQLChanges(src.ID, store.SQLChangeFilter{})
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	cloneID, err := h.db.CreateSyncClone(srcEnv.ProjectID, src.ID, tgtEnv.ID, target.ID, f.Comment, stmtCount)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "创建克隆记录失败: "+err.Error())
		return
	}
	// 在目标分支落一条 CLONE 提交记录：标记本次克隆建立的共同基线。
	nowMs := time.Now().UnixMilli()
	markSQL := fmt.Sprintf("-- 克隆基线：%s（%s → %s）", src.DBName, srcEnv.Name, tgtEnv.Name)
	if strings.TrimSpace(f.Comment) != "" {
		markSQL += " " + strings.TrimSpace(f.Comment)
	}
	if _, err := h.db.InsertSQLChanges(
		[]store.SQLChange{{
			BindingID:  target.ID,
			StmtType:   "CLONE",
			SQLText:    markSQL,
			Username:   "dbgit",
			Schemaname: target.DBName,
		}},
		[]string{fmt.Sprintf("clone-marker-%d", cloneID)},
		[]int64{nowMs},
	); err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "写入克隆标记失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{
		"id":                cloneID,
		"source_binding_id": src.ID,
		"target_env_id":     tgtEnv.ID,
		"target_binding_id": target.ID,
		"stmt_count":        stmtCount,
		"created":           created,
	})
}

// GraphDB 分支树上的数据库节点（含近期语句）。
type GraphDB struct {
	ID           int64       `json:"id"`
	DBName       string      `json:"db_name"`
	InstanceName string      `json:"instance_name"`
	Statements   []GraphStmt `json:"statements"`
}

// GraphStmt 分支树上的提交节点（一条 SQL）。
type GraphStmt struct {
	ID        int64  `json:"id"`
	StmtType  string `json:"stmt_type"`
	SQLText   string `json:"sql_text"`
	Username  string `json:"username"`
	EventTime string `json:"event_time"`
}

// GraphEnv 分支树上的分支（一个分支）。
type GraphEnv struct {
	ID   int64     `json:"id"`
	Name string    `json:"name"`
	DBs  []GraphDB `json:"dbs"`
}

// SyncGraph GET /api/v1/projects/:id/sync-graph?per_binding=20
func (h *SyncMerges) SyncGraph(c *gin.Context) {
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
	perBinding := 20
	if v := c.Query("per_binding"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			perBinding = n
		}
	}
	if perBinding <= 0 || perBinding > 100 {
		perBinding = 20
	}
	tree, err := h.db.GetProjectTree()
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	var envs []GraphEnv
	for _, p := range tree {
		if p.ID != id {
			continue
		}
		for _, e := range p.Envs {
			ge := GraphEnv{ID: e.ID, Name: e.Name, DBs: []GraphDB{}}
			for _, b := range e.DBs {
				gd := GraphDB{ID: b.ID, DBName: b.DBName, InstanceName: b.InstanceName, Statements: []GraphStmt{}}
				stmts, err := h.db.ListSQLChanges(b.ID, store.SQLChangeFilter{Page: 1, PageSize: perBinding})
				if err != nil {
					response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
					return
				}
				for _, s := range stmts {
					gd.Statements = append(gd.Statements, GraphStmt{
						ID: s.ID, StmtType: s.StmtType, SQLText: s.SQLText,
						Username: s.Username, EventTime: s.EventTime,
					})
				}
				ge.DBs = append(ge.DBs, gd)
			}
			envs = append(envs, ge)
		}
	}
	merges, err := h.db.ListSyncMerges(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	clones, err := h.db.ListSyncClones(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	revs, err := h.db.ListSyncConfigRevisions(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if envs == nil {
		envs = []GraphEnv{}
	}
	if merges == nil {
		merges = []store.SyncMerge{}
	}
	if clones == nil {
		clones = []store.SyncClone{}
	}
	if revs == nil {
		revs = []store.SyncConfigRevision{}
	}
	response.OK(c, gin.H{"envs": envs, "merges": merges, "clones": clones, "config_changes": revs})
}

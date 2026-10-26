package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/dbgit/dbgit/internal/dbprobe"
	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// Projects 项目 / 分支 / 库绑定。
type Projects struct {
	db *store.DB
}

// NewProjects 创建 handler。
func NewProjects(db *store.DB) *Projects {
	return &Projects{db: db}
}

var dbNameRe = regexp.MustCompile(`^[\p{L}\p{N}_$]+$`)

func checkName(name string, what string) error {
	name = strings.TrimSpace(name)
	switch {
	case name == "":
		return errors.New(what + "名称不能为空")
	case len(name) > 64:
		return errors.New(what + "名称过长（最多 64 字符）")
	}
	return nil
}

func checkDBName(dbName string) error {
	switch {
	case dbName == "":
		return errors.New("库名不能为空")
	case len(dbName) > 64:
		return errors.New("库名过长（最多 64 字符）")
	case !dbNameRe.MatchString(dbName):
		return errors.New("库名仅支持字母、数字、下划线与 $")
	}
	return nil
}

func checkRemark(remark string) error {
	if len(remark) > 256 {
		return errors.New("备注过长（最多 256 字符）")
	}
	return nil
}

func uniqueErr(err error) bool {
	return store.IsUniqueViolation(err)
}

// Tree GET /api/v1/projects/tree
func (h *Projects) Tree(c *gin.Context) {
	tree, err := h.db.GetProjectTree()
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	response.OK(c, tree)
}

type nameForm struct {
	Name   string `json:"name"`
	Remark string `json:"remark"`
}

func (f *nameForm) normalize() {
	f.Name = strings.TrimSpace(f.Name)
	f.Remark = strings.TrimSpace(f.Remark)
}

// CreateProject POST /api/v1/projects
func (h *Projects) CreateProject(c *gin.Context) {
	var f nameForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := checkName(f.Name, "项目"); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	p, err := h.db.CreateProject(f.Name, f.Remark)
	if err != nil {
		if uniqueErr(err) {
			response.Abort(c, http.StatusBadRequest, 400, "项目名称已存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "创建失败: "+err.Error())
		return
	}
	response.OK(c, p)
}

// UpdateProject PUT /api/v1/projects/:id
func (h *Projects) UpdateProject(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f nameForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := checkName(f.Name, "项目"); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	p, err := h.db.UpdateProject(id, f.Name, f.Remark)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "项目不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "项目名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "更新失败: "+err.Error())
		}
		return
	}
	response.OK(c, p)
}

// DeleteProject DELETE /api/v1/projects/:id（级联删除分支与绑定）
func (h *Projects) DeleteProject(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	deleted, err := h.db.DeleteProject(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "删除失败: "+err.Error())
		return
	}
	if !deleted {
		response.Abort(c, http.StatusNotFound, 404, "项目不存在")
		return
	}
	response.OK(c, gin.H{"id": id})
}

// CreateEnv POST /api/v1/projects/:id/envs
func (h *Projects) CreateEnv(c *gin.Context) {
	projectID, ok := parseID(c)
	if !ok {
		return
	}
	var f nameForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := checkName(f.Name, "分支"); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	e, err := h.db.CreateEnv(projectID, f.Name, f.Remark)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "项目不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "该项目下分支名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "创建失败: "+err.Error())
		}
		return
	}
	response.OK(c, e)
}

// CloneEnv POST /api/v1/envs/:id/clone
// 以指定分支为来源完整克隆出一个新分支：复制其全部绑定，
// 并在每个新库上落一条 CLONE 基准标记，建立共同基线。整体在一个事务内完成。
// targets 非空时逐库指定目标连接与库名：先对真实库执行 DROP / CREATE（勾选项控制），
// 再单事务落元数据；DDL 失败则不建分支（无元数据残留）。
func (h *Projects) CloneEnv(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f struct {
		Name    string `json:"name"`
		Remark  string `json:"remark"`
		Comment string `json:"comment"`
		Targets []struct {
			SourceBindingID int64  `json:"source_binding_id"`
			InstanceID      int64  `json:"instance_id"`
			DBName          string `json:"db_name"`
			Overwrite       bool   `json:"overwrite"`
			CreateIfMissing bool   `json:"create_if_missing"`
		} `json:"targets"`
	}
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.Name = strings.TrimSpace(f.Name)
	f.Remark = strings.TrimSpace(f.Remark)
	f.Comment = strings.TrimSpace(f.Comment)
	if err := checkName(f.Name, "分支"); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if len(f.Comment) > 256 {
		response.Abort(c, http.StatusBadRequest, 400, "说明过长（最多 256 字符）")
		return
	}
	// 无逐库目标时走默认映射（沿用来源连接与库名，不做真实 DDL）。
	if len(f.Targets) == 0 {
		env, err := h.db.CloneEnv(id, f.Name, f.Remark, f.Comment)
		if err != nil {
			switch {
			case isNotFound(err):
				response.Abort(c, http.StatusNotFound, 404, "来源分支不存在")
			case uniqueErr(err):
				response.Abort(c, http.StatusBadRequest, 400, "该项目下分支名称已存在")
			default:
				response.Abort(c, http.StatusInternalServerError, 500, "克隆失败: "+err.Error())
			}
			return
		}
		response.OK(c, gin.H{"env": env, "cloned": len(env.DBs)})
		return
	}
	if _, err := h.db.GetEnv(id); err != nil {
		if isNotFound(err) {
			response.Abort(c, http.StatusNotFound, 404, "来源分支不存在")
			return
		}
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	srcDBs, err := h.db.ListEnvDBs(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
		return
	}
	if len(srcDBs) == 0 {
		response.Abort(c, http.StatusBadRequest, 400, "来源分支暂无可克隆的库")
		return
	}
	srcByID := make(map[int64]store.EnvDB, len(srcDBs))
	for _, b := range srcDBs {
		srcByID[b.ID] = b
	}
	// 预校验：目标归属、库名、连接、覆盖完备性、目标去重（DDL 执行前全部拦下）。
	instances := make(map[int64]store.Instance)
	seenTarget := make(map[string]int64)
	maps := make([]store.CloneTarget, 0, len(f.Targets))
	for _, t := range f.Targets {
		src, ok := srcByID[t.SourceBindingID]
		if !ok {
			response.Abort(c, http.StatusBadRequest, 400, "来源库不是该分支的绑定")
			return
		}
		dbName := strings.TrimSpace(t.DBName)
		if err := checkDBName(dbName); err != nil {
			response.Abort(c, http.StatusBadRequest, 400, "目标库名非法（库 "+src.DBName+"）："+err.Error())
			return
		}
		ins, ok := instances[t.InstanceID]
		if !ok {
			ins, err = h.db.GetInstance(t.InstanceID)
			if err != nil {
				if isNotFound(err) {
					response.Abort(c, http.StatusNotFound, 404, "目标连接不存在（库 "+src.DBName+"）")
					return
				}
				response.Abort(c, http.StatusInternalServerError, 500, "查询失败: "+err.Error())
				return
			}
			instances[t.InstanceID] = ins
		}
		key := strings.ToLower(dbName) + "\x00" + strings.ToLower(ins.Host) + ":" + fmt.Sprintf("%d", ins.Port)
		if _, dup := seenTarget[key]; dup {
			response.Abort(c, http.StatusBadRequest, 400, "目标库 "+dbName+" 重复")
			return
		}
		seenTarget[key] = t.SourceBindingID
		maps = append(maps, store.CloneTarget{
			SourceBindingID: t.SourceBindingID,
			InstanceID:      t.InstanceID,
			DBName:          dbName,
		})
	}
	if len(maps) != len(srcDBs) {
		response.Abort(c, http.StatusBadRequest, 400, "需为来源分支的全部绑定库指定目标")
		return
	}
	// DDL 阶段：逐库按勾选项处理真实库（先全部检查再执行，避免中途失败已 DROP 难回滚——
	// DROP 本身不可逆，此处按源库顺序执行，失败即停并报错，元数据尚未写入）。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	overwriteBySrc := make(map[int64]bool, len(f.Targets))
	createBySrc := make(map[int64]bool, len(f.Targets))
	for _, t := range f.Targets {
		overwriteBySrc[t.SourceBindingID] = t.Overwrite
		createBySrc[t.SourceBindingID] = t.CreateIfMissing
	}
	for i := range maps {
		m := &maps[i]
		ins := instances[m.InstanceID]
		exists, err := dbprobe.DatabaseExists(ctx, ins.Host, ins.Port, ins.Username, ins.Password, m.DBName)
		if err != nil {
			response.Abort(c, http.StatusInternalServerError, 500, "连接目标实例「"+ins.Name+"」失败: "+err.Error())
			return
		}
		switch {
		case exists && !overwriteBySrc[m.SourceBindingID]:
			response.Abort(c, http.StatusBadRequest, 400, "目标库 "+m.DBName+" 在「"+ins.Name+"」上已存在（未勾选覆盖）")
			return
		case !exists && !createBySrc[m.SourceBindingID]:
			response.Abort(c, http.StatusBadRequest, 400, "目标库 "+m.DBName+" 在「"+ins.Name+"」上不存在（未勾选不存在时新建）")
			return
		case exists:
			if err := dbprobe.DropDatabase(ctx, ins.Host, ins.Port, ins.Username, ins.Password, m.DBName); err != nil {
				response.Abort(c, http.StatusInternalServerError, 500, "覆盖目标库 "+m.DBName+" 失败: "+err.Error())
				return
			}
			m.Dropped = true
		default:
			// 新建时尽量沿用源库字符集，避免实例默认字符集不一致导致乱码。
			charset := ""
			if srcIns, err := h.db.GetInstance(srcByID[m.SourceBindingID].InstanceID); err == nil {
				charset = dbprobe.DBCharset(ctx, srcIns.Host, srcIns.Port, srcIns.Username, srcIns.Password, srcByID[m.SourceBindingID].DBName)
			}
			if err := dbprobe.CreateDatabase(ctx, ins.Host, ins.Port, ins.Username, ins.Password, m.DBName, charset); err != nil {
				response.Abort(c, http.StatusInternalServerError, 500, "新建目标库 "+m.DBName+" 失败: "+err.Error())
				return
			}
			m.Created = true
		}
	}
	env, err := h.db.CloneEnvMapped(id, f.Name, f.Remark, f.Comment, maps)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "来源分支或目标连接不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "该项目下分支名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "克隆失败: "+err.Error())
		}
		return
	}
	response.OK(c, gin.H{"env": env, "cloned": len(env.DBs), "ddl": ddlSummary(maps)})
}

// ddlSummary 汇总逐库真实 DDL 动作，供前端展示（dropped/created 为库名列表）。
func ddlSummary(maps []store.CloneTarget) gin.H {
	dropped := make([]string, 0)
	created := make([]string, 0)
	for _, m := range maps {
		if m.Dropped {
			dropped = append(dropped, m.DBName)
		}
		if m.Created {
			created = append(created, m.DBName)
		}
	}
	return gin.H{"dropped": dropped, "created": created}
}

// UpdateEnv PUT /api/v1/envs/:id
func (h *Projects) UpdateEnv(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f nameForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := checkName(f.Name, "分支"); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	e, err := h.db.UpdateEnv(id, f.Name, f.Remark)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "分支不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "该项目下分支名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "更新失败: "+err.Error())
		}
		return
	}
	response.OK(c, e)
}

// DeleteEnv DELETE /api/v1/envs/:id（级联删除绑定）
func (h *Projects) DeleteEnv(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	deleted, err := h.db.DeleteEnv(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "删除失败: "+err.Error())
		return
	}
	if !deleted {
		response.Abort(c, http.StatusNotFound, 404, "分支不存在")
		return
	}
	response.OK(c, gin.H{"id": id})
}

type bindingForm struct {
	InstanceID int64  `json:"instance_id"`
	DBName     string `json:"db_name"`
	Remark     string `json:"remark"`
}

func (f *bindingForm) normalize() {
	f.DBName = strings.TrimSpace(f.DBName)
	f.Remark = strings.TrimSpace(f.Remark)
}

func (f *bindingForm) validate() error {
	if f.InstanceID <= 0 {
		return errors.New("请选择数据库连接")
	}
	return checkDBName(f.DBName)
}

// CreateBinding POST /api/v1/envs/:id/bindings
func (h *Projects) CreateBinding(c *gin.Context) {
	envID, ok := parseID(c)
	if !ok {
		return
	}
	var f bindingForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := f.validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	b, err := h.db.CreateEnvDB(envID, f.InstanceID, f.DBName, f.Remark)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "分支或数据库连接不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "该库已绑定，请勿重复添加")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "绑定失败: "+err.Error())
		}
		return
	}
	response.OK(c, b)
}

// UpdateBinding PUT /api/v1/bindings/:id
func (h *Projects) UpdateBinding(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var f bindingForm
	if err := c.ShouldBindJSON(&f); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	f.normalize()
	if err := f.validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	if err := checkRemark(f.Remark); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}
	b, err := h.db.UpdateEnvDB(id, f.InstanceID, f.DBName, f.Remark)
	if err != nil {
		switch {
		case isNotFound(err):
			response.Abort(c, http.StatusNotFound, 404, "绑定或数据库连接不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "该库已绑定，请勿重复添加")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "更新失败: "+err.Error())
		}
		return
	}
	response.OK(c, b)
}

// DeleteBinding DELETE /api/v1/bindings/:id
func (h *Projects) DeleteBinding(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	deleted, err := h.db.DeleteEnvDB(id)
	if err != nil {
		response.Abort(c, http.StatusInternalServerError, 500, "解绑失败: "+err.Error())
		return
	}
	if !deleted {
		response.Abort(c, http.StatusNotFound, 404, "绑定不存在")
		return
	}
	response.OK(c, gin.H{"id": id})
}

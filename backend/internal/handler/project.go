package handler

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// Projects 项目 / 环境 / 库绑定。
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

// DeleteProject DELETE /api/v1/projects/:id（级联删除环境与绑定）
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
	if err := checkName(f.Name, "环境"); err != nil {
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
			response.Abort(c, http.StatusBadRequest, 400, "该项目下环境名称已存在")
		default:
			response.Abort(c, http.StatusInternalServerError, 500, "创建失败: "+err.Error())
		}
		return
	}
	response.OK(c, e)
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
	if err := checkName(f.Name, "环境"); err != nil {
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
			response.Abort(c, http.StatusNotFound, 404, "环境不存在")
		case uniqueErr(err):
			response.Abort(c, http.StatusBadRequest, 400, "该项目下环境名称已存在")
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
		response.Abort(c, http.StatusNotFound, 404, "环境不存在")
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
			response.Abort(c, http.StatusNotFound, 404, "环境或数据库连接不存在")
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

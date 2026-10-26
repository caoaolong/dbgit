package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/dbgit/dbgit/internal/proxysql"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-gonic/gin"
)

// JobState 任务状态。
type JobState string

const (
	JobRunning JobState = "running"
	JobSuccess JobState = "success"
	JobFailed  JobState = "failed"
)

// InstallJob 一次安装任务。
type InstallJob struct {
	mu         sync.Mutex
	sb         strings.Builder
	ID         string     `json:"-"`
	Method     string     `json:"-"`
	State      JobState   `json:"-"`
	ErrMsg     string     `json:"-"`
	StartedAt  time.Time  `json:"-"`
	FinishedAt *time.Time `json:"-"`
}

// jobView 对外展示的任务快照（避免拷贝锁）。
type jobView struct {
	ID         string     `json:"id"`
	Method     string     `json:"method"`
	State      JobState   `json:"state"`
	Log        string     `json:"log"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// ProxySQL 安装相关 handler（任务存于内存，重启丢失）。
type ProxySQL struct {
	db      *store.DB
	mu      sync.Mutex
	jobs    map[string]*InstallJob
	counter atomic.Uint64
}

// NewProxySQL 创建 handler。
func NewProxySQL(db *store.DB) *ProxySQL {
	return &ProxySQL{db: db, jobs: make(map[string]*InstallJob)}
}

// Status GET /api/v1/proxysql/status
func (h *ProxySQL) Status(c *gin.Context) {
	response.OK(c, proxysql.Detect())
}

type installRequest struct {
	Method        string `json:"method"`
	Version       string `json:"version"`
	ContainerName string `json:"container_name"`
	AdminPort     int    `json:"admin_port"`
	MySQLPort     int    `json:"mysql_port"`
	AdminUser     string `json:"admin_user"`
	AdminPassword string `json:"admin_password"`
}

// Install POST /api/v1/proxysql/install（异步执行，立即返回 job id）。
func (h *ProxySQL) Install(c *gin.Context) {
	var req installRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, "请求参数非法: "+err.Error())
		return
	}
	opts := proxysql.Options{
		Method:        proxysql.InstallMethod(req.Method),
		Version:       strings.TrimSpace(req.Version),
		ContainerName: strings.TrimSpace(req.ContainerName),
		AdminPort:     req.AdminPort,
		MySQLPort:     req.MySQLPort,
		AdminUser:     strings.TrimSpace(req.AdminUser),
		AdminPassword: req.AdminPassword,
	}
	if err := opts.Validate(); err != nil {
		response.Abort(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	job := &InstallJob{
		ID:        fmt.Sprintf("job-%d", h.counter.Add(1)),
		Method:    string(opts.Method),
		State:     JobRunning,
		StartedAt: time.Now(),
	}
	h.mu.Lock()
	h.jobs[job.ID] = job
	h.mu.Unlock()

	go h.run(job, opts)

	response.OK(c, gin.H{"job_id": job.ID})
}

// Job GET /api/v1/proxysql/jobs/:id（轮询进度与日志）。
func (h *ProxySQL) Job(c *gin.Context) {
	h.mu.Lock()
	job, ok := h.jobs[c.Param("id")]
	h.mu.Unlock()
	if !ok {
		response.Abort(c, http.StatusNotFound, 404, "任务不存在")
		return
	}
	job.mu.Lock()
	view := jobView{
		ID:         job.ID,
		Method:     job.Method,
		State:      job.State,
		Log:        job.sb.String(),
		Error:      job.ErrMsg,
		StartedAt:  job.StartedAt,
		FinishedAt: job.FinishedAt,
	}
	job.mu.Unlock()
	response.OK(c, view)
}

func (h *ProxySQL) run(job *InstallJob, opts proxysql.Options) {
	logf := func(format string, args ...any) {
		job.mu.Lock()
		defer job.mu.Unlock()
		job.sb.WriteString(fmt.Sprintf(format, args...) + "\n")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	logf("开始安装 ProxySQL（方式: %s，本机执行）", opts.Method)
	if err := proxysql.Install(ctx, opts, logf); err != nil {
		now := time.Now()
		job.mu.Lock()
		job.State = JobFailed
		job.ErrMsg = err.Error()
		job.FinishedAt = &now
		job.mu.Unlock()
		logf("安装失败：%v", err)
		return
	}
	now := time.Now()
	if err := proxysql.SaveInstallState(opts); err != nil {
		logf("警告：安装记录保存失败（不影响使用）：%v", err)
	} else {
		h.registerLocalNode(opts)
	}
	job.mu.Lock()
	job.State = JobSuccess
	job.FinishedAt = &now
	job.mu.Unlock()
}

// registerLocalNode 安装成功后登记/更新本地代理节点（幂等：按容器名或本机端口匹配）。
func (h *ProxySQL) registerLocalNode(opts proxysql.Options) {
	user := strings.TrimSpace(opts.AdminUser)
	if user == "" {
		user = "admin"
	}
	pass := opts.AdminPassword
	if pass == "" {
		pass = "admin" // 与 ProxySQL 官方镜像默认一致
	}
	method, container := "package", ""
	if opts.Method == proxysql.MethodDocker {
		method, container = "docker", strings.TrimSpace(opts.ContainerName)
	}
	name := "本地代理" + suffixForContainer(container)
	nodes, err := h.db.ListProxyNodes(true)
	if err != nil {
		return
	}
	for _, n := range nodes {
		match := container != "" && n.ContainerName == container
		if !match && container == "" && n.Host == "127.0.0.1" && n.AdminPort == opts.AdminPort {
			match = true
		}
		if match {
			_, _ = h.db.UpdateProxyNode(n.ID, store.ProxyNode{
				Name: name, Host: "127.0.0.1",
				AdminPort: opts.AdminPort, MySQLPort: opts.MySQLPort,
				AdminUser: user, AdminPassword: pass,
				Method: method, ContainerName: container,
				Remark: n.Remark,
			})
			return
		}
	}
	_, _ = h.db.CreateProxyNode(store.ProxyNode{
		Name: name, Host: "127.0.0.1",
		AdminPort: opts.AdminPort, MySQLPort: opts.MySQLPort,
		AdminUser: user, AdminPassword: pass,
		Method: method, ContainerName: container,
	})
}

func suffixForContainer(container string) string {
	if container == "" {
		return ""
	}
	return "（" + container + "）"
}

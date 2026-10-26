package router

import (
	"time"

	"github.com/dbgit/dbgit/internal/config"
	"github.com/dbgit/dbgit/internal/handler"
	"github.com/dbgit/dbgit/internal/middleware"
	"github.com/dbgit/dbgit/internal/sqllog"
	"github.com/dbgit/dbgit/internal/store"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// New 注册路由与中间件。
func New(cfg *config.Config, db *store.DB, collector *sqllog.Collector) *gin.Engine {
	gin.SetMode(cfg.Server.Mode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger())
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	health := handler.NewHealth(cfg)
	px := handler.NewProxySQL(db)
	inst := handler.NewInstances(db)
	pb := handler.NewProxyBackends(db, cfg.ProxySQL.AdminUser, cfg.ProxySQL.AdminPassword)
	proj := handler.NewProjects(db)
	slog := handler.NewSQLLog(db, collector)
	scfg := handler.NewSyncConfig(db)
	smg := handler.NewSyncMerges(db)
	pnodes := handler.NewProxyNodes(db, cfg.ProxySQL.AdminUser, cfg.ProxySQL.AdminPassword)

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", health.Check)
		v1.GET("/proxysql/status", px.Status)
		v1.POST("/proxysql/install", px.Install)
		v1.GET("/proxysql/jobs/:id", px.Job)
		v1.GET("/proxysql/backends", pb.List)
		v1.POST("/proxysql/resync", pb.Resync)
		v1.GET("/proxysql/nodes", pnodes.ListNodes)
		v1.POST("/proxysql/nodes", pnodes.CreateNode)
		v1.GET("/proxysql/nodes/:id", pnodes.GetNode)
		v1.PUT("/proxysql/nodes/:id", pnodes.UpdateNode)
		v1.DELETE("/proxysql/nodes/:id", pnodes.DeleteNode)
		v1.GET("/instances", inst.List)
		v1.POST("/instances", inst.Create)
		v1.GET("/instances/:id", inst.Get)
		v1.PUT("/instances/:id", inst.Update)
		v1.POST("/instances/probe", inst.Probe)
		v1.GET("/instances/:id/databases", inst.Databases)
		v1.DELETE("/instances/:id", inst.Delete)
		v1.GET("/projects/tree", proj.Tree)
		v1.POST("/projects", proj.CreateProject)
		v1.PUT("/projects/:id", proj.UpdateProject)
		v1.DELETE("/projects/:id", proj.DeleteProject)
		v1.POST("/projects/:id/envs", proj.CreateEnv)
		v1.PUT("/envs/:id", proj.UpdateEnv)
		v1.DELETE("/envs/:id", proj.DeleteEnv)
		v1.POST("/envs/:id/bindings", proj.CreateBinding)
		v1.POST("/envs/:id/clone", proj.CloneEnv)
		v1.PUT("/bindings/:id", proj.UpdateBinding)
		v1.DELETE("/bindings/:id", proj.DeleteBinding)
		v1.GET("/bindings/:id/changes", slog.BindingChanges)
		v1.POST("/bindings/:id/merges", smg.CreateMerge)
		v1.POST("/bindings/:id/clone", smg.CreateClone)
		v1.GET("/projects/:id/sync-graph", smg.SyncGraph)
		v1.GET("/projects/:id/sync-config", scfg.GetSyncConfig)
		v1.PUT("/projects/:id/sync-config", scfg.UpdateSyncConfig)
		v1.GET("/proxysql/eventslog/status", slog.EventsLogStatus)
		v1.POST("/proxysql/eventslog/collect", slog.EventsLogCollect)
	}

	return r
}

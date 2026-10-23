package router

import (
	"time"

	"github.com/dbgit/dbgit/internal/config"
	"github.com/dbgit/dbgit/internal/handler"
	"github.com/dbgit/dbgit/internal/middleware"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// New 注册路由与中间件。
func New(cfg *config.Config) *gin.Engine {
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

	v1 := r.Group("/api/v1")
	{
		v1.GET("/health", health.Check)
	}

	return r
}

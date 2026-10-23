package handler

import (
	"github.com/dbgit/dbgit/internal/config"
	"github.com/dbgit/dbgit/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// Health 健康检查与基础信息。
type Health struct {
	cfg *config.Config
}

// NewHealth 创建健康检查 handler。
func NewHealth(cfg *config.Config) *Health {
	return &Health{cfg: cfg}
}

// Check GET /api/v1/health
func (h *Health) Check(c *gin.Context) {
	response.OK(c, gin.H{
		"status":  "up",
		"service": "dbgit",
		"mode":    h.cfg.Server.Mode,
	})
}

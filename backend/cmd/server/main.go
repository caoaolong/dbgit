package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/dbgit/dbgit/internal/config"
	"github.com/dbgit/dbgit/internal/handler"
	"github.com/dbgit/dbgit/internal/router"
	"github.com/dbgit/dbgit/internal/sqllog"
	"github.com/dbgit/dbgit/internal/store"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("加载配置失败", "err", err)
		os.Exit(1)
	}

	initLogger(cfg.Log.Level)

	db, err := store.Open(cfg.Database.Path)
	if err != nil {
		slog.Error("打开数据库失败", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// 启动时全量同步代理配置（尽力而为）
	handler.SyncAllProxies(db)

	// 启动 SQL 采集器（Advanced Events Logging 轮询，失败只记日志）。
	collector := sqllog.New(db, sqllog.Options{
		AdminAddr:     cfg.ProxySQL.AdminAddr,
		AdminUser:     cfg.ProxySQL.AdminUser,
		AdminPassword: cfg.ProxySQL.AdminPassword,
		Interval:      time.Duration(cfg.SQLLog.IntervalSeconds) * time.Second,
		BufferSize:    cfg.SQLLog.BufferSize,
		RetentionDays: cfg.SQLLog.RetentionDays,
	})
	go collector.Start(context.Background())

	r := router.New(cfg, db, collector)
	slog.Info("dbgit 服务启动", "addr", cfg.Server.Addr)
	if err := r.Run(cfg.Server.Addr); err != nil {
		slog.Error("服务退出", "err", err)
		os.Exit(1)
	}
}

func initLogger(level string) {
	var lv slog.Level
	switch level {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lv})))
}

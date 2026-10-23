package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/dbgit/dbgit/internal/config"
	"github.com/dbgit/dbgit/internal/router"
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

	r := router.New(cfg)
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

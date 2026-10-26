package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置。
type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	ProxySQL ProxySQLConfig `mapstructure:"proxysql"`
	SQLLog   SQLLogConfig   `mapstructure:"sqllog"`
	Log      LogConfig      `mapstructure:"log"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Addr string `mapstructure:"addr"`
	Mode string `mapstructure:"mode"`
}

// DatabaseConfig SQLite 配置。
type DatabaseConfig struct {
	Path string `mapstructure:"path"`
}

// ProxySQLConfig ProxySQL Admin 连接配置（占位，后续对接）。
type ProxySQLConfig struct {
	AdminAddr     string `mapstructure:"admin_addr"`
	AdminUser     string `mapstructure:"admin_user"`
	AdminPassword string `mapstructure:"admin_password"`
}

// SQLLogConfig 变更记录（SQL 日志）采集配置。
type SQLLogConfig struct {
	// IntervalSeconds 采集间隔（秒）。
	IntervalSeconds int `mapstructure:"interval_seconds"`
	// BufferSize ProxySQL eventslog 环形缓冲区大小（事件数）。
	BufferSize int `mapstructure:"buffer_size"`
	// RetentionDays 变更记录保留天数。
	RetentionDays int `mapstructure:"retention_days"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level string `mapstructure:"level"`
}

// Load 从文件加载配置；path 为空时默认 configs/config.yaml。
func Load(path string) (*Config, error) {
	if path == "" {
		path = "configs/config.yaml"
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("DBGIT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.addr", ":8080")
	v.SetDefault("server.mode", "debug")
	v.SetDefault("database.path", "data/dbgit.db")
	v.SetDefault("proxysql.admin_addr", "127.0.0.1:6032")
	v.SetDefault("proxysql.admin_user", "admin")
	v.SetDefault("sqllog.interval_seconds", 60)
	v.SetDefault("sqllog.buffer_size", 200000)
	v.SetDefault("sqllog.retention_days", 30)
	v.SetDefault("log.level", "info")
}

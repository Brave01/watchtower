package core

import "time"

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	LogMonitor LogMonitorConfig `yaml:"log_monitor"`
	Dashboard  DashboardConfig  `yaml:"dashboard"`
	StoreCfg   StoreConfig      `yaml:"store_cfg"`
	Auth       AuthConfig       `yaml:"auth"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type LogMonitorConfig struct {
	FeishuWebhook FeishuWebhookConfig `yaml:"feishu_webhook"`
}

type FeishuWebhookConfig struct {
	URL        string `yaml:"url"`
	MaxRetries int    `yaml:"max_retries"`
}

type DashboardConfig struct {
	ProbeInterval string `yaml:"probe_interval"`
}

type StoreConfig struct {
	Driver string `yaml:"driver"`
	Path   string `yaml:"path"`
}

type AuthConfig struct {
	AdminUser string `yaml:"admin_user"`
}

func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{Port: 8080},
		StoreCfg: StoreConfig{
			Driver: "sqlite",
			Path:   "./data/server.db",
		},
		Dashboard: DashboardConfig{
			ProbeInterval: "15s",
		},
		Auth: AuthConfig{
			AdminUser: "admin",
		},
	}
}

func ParseDuration(s string, defaultDuration time.Duration) time.Duration {
	if s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return defaultDuration
}

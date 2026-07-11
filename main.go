package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"watchtower/internal/auth"
	"watchtower/internal/dashboard"
	"watchtower/internal/handler"
	"watchtower/internal/logmonitor/dedup"
	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/store"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

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
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	FeishuWebhook FeishuWebhookConfig `yaml:"feishu_webhook"`
}

type ElasticsearchConfig struct {
	Address  string                 `yaml:"address"`
	Username string                 `yaml:"username"`
	Index    string                 `yaml:"index"`
	Interval int                    `yaml:"interval"`
	Size     int                    `yaml:"size"`
	Query    map[string]interface{} `yaml:"query"`
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

func main() {
	cfg := loadConfig()
	log.Printf("[Main] 启动 SCM 服务，端口 :%d", cfg.Server.Port)

	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatalf("[Main] 创建 data 目录失败: %v", err)
	}

	st, err := store.NewSQLiteStore(cfg.StoreCfg.Path)
	if err != nil {
		log.Fatalf("[Main] 数据库初始化失败: %v", err)
	}
	defer st.Close()

	// ========== 日志监控组件 ==========
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logParser := parser.NewParser()
	deduper := dedup.NewDeduplicator(5*time.Minute, 100000)
	wsHub := ws.New()

	// 过滤引擎 + 加载规则
	filterEngine := filter.NewEngine(logParser)
	rules, _ := st.ListAlertRules()
	for _, r := range rules {
		fr := &filter.AlertRule{
			ID:              r.ID,
			Name:            r.Name,
			Enabled:         r.Enabled,
			Keywords:        splitStrings(r.Keywords),
			ExcludeKeywords: splitStrings(r.ExcludeKeywords),
			Level:           r.Level,
			RegexPattern:    r.RegexPattern,
			Cooldown:        r.Cooldown,
			MessageTemplate: r.MessageTemplate,
			WebhookID:       r.WebhookID,
		}
		if err := filterEngine.AddRule(fr); err != nil {
			log.Printf("[Main] 加载规则失败 %s: %v", r.Name, err)
		}
	}
	log.Printf("[Main] 已加载 %d 条告警规则", len(rules))

	// ========== Webhook 客户端（支持多个） ==========
	whClients := map[int]*webhook.Client{}
	var defaultWhClient *webhook.Client
	dbWhCfgs, _ := st.ListWebhookConfigs()
	if len(dbWhCfgs) > 0 {
		for _, dbWhCfg := range dbWhCfgs {
			if dbWhCfg.URL == "" {
				continue
			}
			client := webhook.NewClient(&webhook.WebhookConfig{
				Platform:           dbWhCfg.Platform,
				URL:                dbWhCfg.URL,
				Secret:             dbWhCfg.Secret,
				Enabled:            dbWhCfg.Enabled,
				MaxRetries:         dbWhCfg.MaxRetries,
				MentionType:        dbWhCfg.MentionType,
				MentionUsers:       splitStrings(dbWhCfg.MentionUsers),
				RateLimit:          dbWhCfg.RateLimit,
				RateLimitPerSecond: dbWhCfg.RateLimitPerSecond,
				RingBufferSize:     dbWhCfg.RingBufferSize,
				Template:           dbWhCfg.Template,
			})
			whClients[dbWhCfg.ID] = client
			if defaultWhClient == nil {
				defaultWhClient = client
			}
		}
	}
	if defaultWhClient == nil {
		// 从 config.yaml 回退
		yamlCfg := &webhook.WebhookConfig{
			URL:        cfg.LogMonitor.FeishuWebhook.URL,
			MaxRetries: cfg.LogMonitor.FeishuWebhook.MaxRetries,
		}
		if yamlCfg.RateLimit == 0 {
			yamlCfg.RateLimit = 10
		}
		if yamlCfg.RateLimitPerSecond == 0 {
			yamlCfg.RateLimitPerSecond = 2
		}
		defaultWhClient = webhook.NewClient(yamlCfg)
		whClients[0] = defaultWhClient
	}

	// 为每个 Webhook 客户端设置限流溢出回调 + 重试循环
	for id, client := range whClients {
		clientID := id
		client.SetOverflowHandler(func(entry *webhook.LimitedAlertEntry) {
			limitedAt := entry.LimitedAt
			if limitedAt.IsZero() {
				limitedAt = time.Now()
			}
			st.SaveLimitedAlert(&store.LimitedAlert{
				RuleName:  entry.RuleName,
				Message:   entry.Message,
				Level:     entry.Level,
				Source:    entry.Source,
				Timestamp: entry.Timestamp,
				LimitedAt: limitedAt.Format(time.RFC3339),
				Summary:   entry.Summary,
			})
		})
		client.StartDrainLoop(ctx, func() []*webhook.LimitedAlertEntry {
			alerts, _ := st.LoadLimitedAlertsForRetry(100)
			if len(alerts) == 0 {
				return nil
			}
			entries := make([]*webhook.LimitedAlertEntry, len(alerts))
			for i, a := range alerts {
				entries[i] = &webhook.LimitedAlertEntry{
					RuleName:  a.RuleName,
					Message:   a.Message,
					Level:     a.Level,
					Source:    a.Source,
					Timestamp: a.Timestamp,
					LimitedAt: parseTime(a.LimitedAt),
					Summary:   a.Summary,
				}
			}
			return entries
		}, nil)
		log.Printf("[Main] Webhook #%d 已就绪 (%s)", clientID, client.GetConfig().Platform)
	}

	// ES 管道（从数据库配置动态启动）
	esPipeline := handler.NewESPipeline(logParser, deduper, filterEngine, wsHub, whClients, defaultWhClient)
	dbCfg, _ := st.GetESConfig()
	if dbCfg != nil && dbCfg.Enabled && dbCfg.Address != "" {
		// 从环境变量补充密码（如果 DB 中未保存）
		if dbCfg.Password == "" {
			dbCfg.Password = getEnv("ES_PASSWORD", "")
		}
		if err := esPipeline.Start(dbCfg); err != nil {
			log.Printf("[Main] ES 管道启动失败: %v", err)
		}
	} else {
		// 回退到 config.yaml 配置
		esPwd := getEnv("ES_PASSWORD", "")
		if cfg.LogMonitor.Elasticsearch.Address != "" && esPwd != "" {
			fallbackCfg := &store.ESConfig{
				Address:  cfg.LogMonitor.Elasticsearch.Address,
				Username: cfg.LogMonitor.Elasticsearch.Username,
				Password: esPwd,
				Index:    cfg.LogMonitor.Elasticsearch.Index,
				Interval: cfg.LogMonitor.Elasticsearch.Interval,
				Enabled:  true,
			}
			if cfg.LogMonitor.Elasticsearch.Query != nil {
				qb, _ := json.Marshal(cfg.LogMonitor.Elasticsearch.Query)
				fallbackCfg.Query = string(qb)
			}
			// 保存回 DB 以便页面管理
			st.SaveESConfig(fallbackCfg)
			if err := esPipeline.Start(fallbackCfg); err != nil {
				log.Printf("[Main] ES 管道启动失败: %v", err)
			}
		} else {
			log.Printf("[Main] ES 未配置或 ES_PASSWORD 未设置，日志轮询已跳过")
		}
	}

	// ========== 拨测调度器 ==========
	interval := 15 * time.Second
	if cfg.Dashboard.ProbeInterval != "" {
		if d, err := time.ParseDuration(cfg.Dashboard.ProbeInterval); err == nil {
			interval = d
		}
	}
	scheduler := dashboard.NewScheduler(st, interval)
	scheduler.Start()

	// ========== 认证 ==========
	secret := []byte(getEnv("AUTH_JWT_SECRET", "dev-secret"))
	adminUser := getEnv("ADMIN_USER", cfg.Auth.AdminUser)
	if adminUser == "" {
		adminUser = "admin"
	}
	defaultPwd := "admin"
	adminHash := getEnv("ADMIN_PASSWORD_HASH", "")
	if adminHash == "" {
		h, err := bcrypt.GenerateFromPassword([]byte(defaultPwd), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("[Main] bcrypt 哈希失败: %v", err)
		}
		adminHash = string(h)
		log.Printf("[Main] 未配置 ADMIN_PASSWORD_HASH，使用默认密码: %s", defaultPwd)
	}

	// ========== HTTP 路由 ==========
	mux := http.NewServeMux()
	handler.RegisterAuth(mux, secret, adminUser, adminHash)
	handler.RegisterLogMonitor(mux, &handler.LogMonitorDeps{
		Store:          st,
		WsHub:          wsHub,
		Filter:         filterEngine,
		Webhook:        defaultWhClient,
		WebhookClients: whClients,
		ESPipeline:     esPipeline,
	})
	handler.RegisterDashboard(mux, &handler.DashboardDeps{
		Store:     st,
		Scheduler: scheduler,
	})

	exemptPaths := []string{"/login", "/api/auth/*", "/common/*", "/api/health", "/assets/*"}
	mid := auth.Middleware(secret, exemptPaths)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: mid(mux)}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		log.Printf("[Main] 监听端口 %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Main] HTTP 服务错误: %v", err)
		}
	}()
	<-quit
	log.Println("[Main] 正在关闭...")
	cancel()
	scheduler.Stop()
}

func loadConfig() *Config {
	// 自动加载 .env 文件（如果存在），使 ES_PASSWORD 等环境变量生效
	loadEnvFile("configs/.env")

	cfg := &Config{}
	cfg.Server.Port = 8080
	cfg.StoreCfg.Driver = "sqlite"
	cfg.StoreCfg.Path = "./data/server.db"
	data, err := os.ReadFile("configs/config.yaml")
	if err != nil {
		log.Printf("[Main] 未找到 configs/config.yaml，使用默认配置: %v", err)
		return cfg
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Fatalf("[Main] 配置文件解析失败: %v", err)
	}
	return cfg
}

// loadEnvFile 读取 .env 文件并设置环境变量（不覆盖已存在的）
func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// 去掉引号
		val = strings.Trim(val, `"'`)
		// 仅当环境变量未设置时注入
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseTime(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t }

func splitStrings(s string) []string {
	if s == "" {
		return nil
	}
	// 尝试 JSON 数组格式（兼容前端 JSON.stringify 存储的关键词）
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "[") {
		var items []string
		if err := json.Unmarshal([]byte(trimmed), &items); err == nil {
			result := make([]string, 0, len(items))
			for _, item := range items {
				if item != "" {
					result = append(result, item)
				}
			}
			return result
		}
	}
	// 兼容旧格式：逗号分隔
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

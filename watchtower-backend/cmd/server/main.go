package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"watchtower/internal/app"
	v1app "watchtower/internal/app/v1"
	"watchtower/internal/app/v1/dashboard"
	"watchtower/internal/app/v1/logmonitor"
	"watchtower/internal/app/v1/user"
	"watchtower/internal/core"
	appcrypto "watchtower/internal/crypto"
	dashpkg "watchtower/internal/dashboard"
	"watchtower/internal/logmonitor/dedup"
	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/middleware"
	"watchtower/internal/model"
	"watchtower/internal/store"
	"watchtower/pkg/utils"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	cfg := core.LoadConfig()
	log.Printf("[Main] 启动 SCM 服务，端口 :%d", cfg.Server.Port)

	if err := os.MkdirAll("./data", 0755); err != nil {
		log.Fatalf("[Main] 创建 data 目录失败: %v", err)
	}

	st, err := store.NewSQLiteStore(cfg.StoreCfg.Path)
	if err != nil {
		log.Fatalf("[Main] 数据库初始化失败: %v", err)
	}
	defer st.Close()

	// 初始化 SSH 凭据加密
	if encKey := core.GetEnv("SSH_CRED_KEY", ""); encKey != "" {
		aead, err := appcrypto.NewAEAD([]byte(encKey))
		if err != nil {
			log.Printf("[Main] SSH 凭据加密初始化失败: %v", err)
		} else {
			st.SetAEAD(aead)
			log.Println("[Main] SSH 凭据 AES-256-GCM 加密已启用")
			migrateSSHCredentials(st)
		}
	}

	// 配置 SSH 主机密钥校验
	dashpkg.SetHostKeyCheck(cfg.SSH.HostKeyCheck)
	log.Printf("[Main] SSH 主机密钥校验: %v", cfg.SSH.HostKeyCheck)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ========== 初始化认证 ==========
	secret := []byte(core.GetEnv("AUTH_JWT_SECRET", "dev-secret"))
	adminUser := core.GetEnv("ADMIN_USER", cfg.Auth.AdminUser)
	if adminUser == "" {
		adminUser = "admin"
	}
	defaultPwd := "admin"
	adminHash := core.GetEnv("ADMIN_PASSWORD_HASH", "")
	if adminHash == "" {
		h, err := bcrypt.GenerateFromPassword([]byte(defaultPwd), bcrypt.DefaultCost)
		if err != nil {
			log.Fatalf("[Main] bcrypt 哈希失败: %v", err)
		}
		adminHash = string(h)
		log.Printf("[Main] 未配置 ADMIN_PASSWORD_HASH，使用默认密码: %s", defaultPwd)
	}
	existing, _ := st.GetUser(adminUser)
	if existing == nil {
		if err := st.SaveUser(&model.User{Username: adminUser, PasswordHash: adminHash}); err != nil {
			log.Printf("[Main] 写入管理员用户失败: %v", err)
		} else {
			log.Printf("[Main] 管理员用户 %s 已初始化", adminUser)
		}
	}

	// ========== 初始化 App 层 ==========
	appInstance := initApp(ctx, cfg, st, secret, adminUser)

	// ========== HTTP 路由 ==========
	mux := http.NewServeMux()

	// 静态文件路由（兼容旧版）
	registerStaticRoutes(mux)

	// 注册业务路由
	appInstance.RegisterRoutes(mux)

	// 中间件
	exemptPaths := []string{"/login", "/api/auth/*", "/common/*", "/api/health"}
	mid := middleware.Auth(secret, exemptPaths)
	var allowedOrigins []string
	if cfg.Server.CORSOrigins != "" {
		for _, o := range strings.Split(cfg.Server.CORSOrigins, ",") {
			o = strings.TrimSpace(o)
			if o != "" {
				allowedOrigins = append(allowedOrigins, o)
			}
		}
	}
	corsHandler := middleware.CORS(mid(mux), allowedOrigins...)

	// 启动 HTTP 服务
	port := cfg.Server.Port
	if envPort := core.GetEnv("PORT", ""); envPort != "" {
		port, _ = strconv.Atoi(envPort)
	}
	if port <= 0 {
		port = 3972
	}
	addr := fmt.Sprintf(":%d", port)
	srv := &http.Server{Addr: addr, Handler: corsHandler}

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
}

func initApp(ctx context.Context, cfg *core.Config, st *store.SQLiteStore, secret []byte, adminUser string) *app.App {
	// ========== 日志监控组件 ==========
	lmHandler := initLogMonitor(ctx, cfg, st)

	// ========== 拨测调度器 ==========
	interval := core.ParseDuration(cfg.Dashboard.ProbeInterval, 15*time.Second)
	sched := dashpkg.NewScheduler(st, interval)
	sched.Start()

	schedWrapper := &dashboard.SchedulerWrapper{
		Trigger:  sched.Trigger,
		ProbeAll: func() {},
		ProbeOne: sched.ProbeHost,
	}

	// ========== 创建业务模块 ==========
	authHandler := user.NewAuthHandler(secret, adminUser, st)
	dashHandler := dashboard.NewDashboardHandler(st)
	dashHandler.SetScheduler(schedWrapper)

	// ========== 创建 App ==========
	v1 := v1app.New(authHandler, dashHandler, lmHandler)
	return app.New(v1)
}

func initLogMonitor(ctx context.Context, cfg *core.Config, st *store.SQLiteStore) *logmonitor.LogMonitorHandler {
	// 日志解析器
	logParser := parser.NewParser()

	// WebSocket Hub
	wsHub := ws.New()

	// 从 DB 加载告警规则初始化过滤引擎
	filterRules := make([]*filter.AlertRule, 0)
	dbRules, err := st.ListAlertRules()
	if err != nil {
		log.Printf("[LogMonitor] 加载告警规则失败: %v", err)
	} else {
		for _, r := range dbRules {
			filterRules = append(filterRules, &filter.AlertRule{
				ID:              r.ID,
				Name:            r.Name,
				Enabled:         r.Enabled,
				Keywords:        utils.SplitStrings(r.Keywords),
				ExcludeKeywords: utils.SplitStrings(r.ExcludeKeywords),
				Level:           r.Level,
				RegexPattern:    r.RegexPattern,
				Cooldown:        r.Cooldown,
				MessageTemplate: r.MessageTemplate,
				WebhookID:       r.WebhookID,
			})
		}
	}
	filterEngine := filter.NewEngine(logParser)

	// 将 DB 中的规则加载到过滤引擎
	for _, r := range dbRules {
		filterEngine.AddRule(&filter.AlertRule{
			ID:              r.ID,
			Name:            r.Name,
			Enabled:         r.Enabled,
			Keywords:        utils.SplitStrings(r.Keywords),
			ExcludeKeywords: utils.SplitStrings(r.ExcludeKeywords),
			Level:           r.Level,
			RegexPattern:    r.RegexPattern,
			Cooldown:        r.Cooldown,
			MessageTemplate: r.MessageTemplate,
			WebhookID:       r.WebhookID,
		})
	}

	// Webhook 客户端
	webhookConfigs, _ := st.ListWebhookConfigs()
	webhookClients := make(map[int]*webhook.Client)
	defaultWebhook := webhook.NewClient(&webhook.WebhookConfig{
		Platform:   "feishu",
		URL:        cfg.LogMonitor.FeishuWebhook.URL,
		MaxRetries: cfg.LogMonitor.FeishuWebhook.MaxRetries,
		Enabled:    cfg.LogMonitor.FeishuWebhook.URL != "",
	})
	for _, wc := range webhookConfigs {
		client := webhook.NewClient(&webhook.WebhookConfig{
			Platform:           wc.Platform,
			URL:                wc.URL,
			Secret:             wc.Secret,
			Enabled:            wc.Enabled,
			MaxRetries:         wc.MaxRetries,
			MentionType:        wc.MentionType,
			MentionUsers:       utils.SplitComma(wc.MentionUsers),
			RateLimit:          wc.RateLimit,
			RateLimitPerSecond: wc.RateLimitPerSecond,
			RingBufferSize:     wc.RingBufferSize,
			Template:           wc.Template,
		})
		webhookClients[wc.ID] = client
	}

	// 缓冲区溢出回调（写入数据库持久化）
	overflowFn := func(entry *webhook.LimitedAlertEntry) {
		if err := st.SaveLimitedAlert(&model.LimitedAlert{
			RuleName:  entry.RuleName,
			Message:   entry.Message,
			Level:     entry.Level,
			Source:    entry.Source,
			Timestamp: entry.Timestamp,
			LimitedAt: entry.LimitedAt.Format("2006-01-02 15:04:05"),
			Summary:   entry.Summary,
		}); err != nil {
			log.Printf("[Webhook] 保存限流告警失败: %v", err)
		}
	}
	if defaultWebhook != nil {
		defaultWebhook.SetOverflowHandler(overflowFn)
	}
	for _, client := range webhookClients {
		client.SetOverflowHandler(overflowFn)
	}

	// ES 管道
	deduper := dedup.NewDeduplicator(5*time.Minute, 100000)
	esPipeline := logmonitor.NewESPipeline(deduper, logParser, filterEngine, wsHub, defaultWebhook, webhookClients)
	esConfig, _ := st.GetESConfig()
	if esConfig != nil && esConfig.Enabled && esConfig.Address != "" {
		esConfigWithPwd := *esConfig
		esConfigWithPwd.Password = esConfig.Password
		if err := esPipeline.Start(&esConfigWithPwd); err != nil {
			log.Printf("[ES] 自动启动 ES 管道失败: %v", err)
		}
	}

	return logmonitor.NewLogMonitorHandler(st, wsHub, filterEngine, defaultWebhook, webhookClients, esPipeline)
}

// migrateSSHCredentials 将数据库中现有明文 SSH 凭据加密存储。
func migrateSSHCredentials(st *store.SQLiteStore) {
	creds, err := st.ListSSHCredentials()
	if err != nil {
		log.Printf("[Main] 读取 SSH 凭据进行迁移时出错: %v", err)
		return
	}
	migrated := 0
	for _, c := range creds {
		// 重新保存将触发加密（AddSSHCredential 内部会检查 aead）
		if _, err := st.AddSSHCredential(&c); err != nil {
			log.Printf("[Main] SSH 凭据 %s 迁移失败: %v", c.ID, err)
		} else {
			migrated++
		}
	}
	if migrated > 0 {
		log.Printf("[Main] 已迁移 %d 条 SSH 凭据为加密存储", migrated)
	}
}

func registerStaticRoutes(mux *http.ServeMux) {
	// 登录页由前端提供，后端不再内嵌
}

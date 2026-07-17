package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"watchtower/internal/app"
	v1app "watchtower/internal/app/v1"
	"watchtower/internal/app/v1/dashboard"
	"watchtower/internal/app/v1/logmonitor"
	"watchtower/internal/app/v1/user"
	"watchtower/internal/core"
	dashpkg "watchtower/internal/dashboard"
	"watchtower/internal/middleware"
	"watchtower/internal/model"
	"watchtower/internal/store"

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
	corsHandler := middleware.CORS(mid(mux))

	// 启动 HTTP 服务
	addr := ":" + core.GetEnv("PORT", "")
	if addr == ":" {
		addr = ":3972"
	}
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
	return &logmonitor.LogMonitorHandler{}
}

func registerStaticRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "internal/webui/static/login.html")
	})
}

package handler

import (
	"io"
	"io/fs"
	"log"
	"net/http"

	"watchtower/internal/auth"
	"watchtower/internal/store"
	"watchtower/internal/webui"
)

func RegisterAuth(mux *http.ServeMux, secret []byte, adminUser string, s store.Store) {
	mux.HandleFunc("/api/auth/login", auth.LoginHandler(secret, adminUser, s))
	mux.HandleFunc("/api/auth/logout", auth.LogoutHandler())
	mux.HandleFunc("/api/auth/me", auth.MeHandler(secret))
	mux.HandleFunc("/api/auth/change-password", auth.ChangePasswordHandler(secret, s))
	// 共享静态文件：nav.js、theme.css、login.html 等（旧版页面/多服务共用）
	subFS, _ := fs.Sub(webui.StaticFS, "static")
	mux.Handle("/common/", http.StripPrefix("/common/", http.FileServer(http.FS(subFS))))
	// 登录页（旧版向后兼容，新前端不再依赖）
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		f, err := webui.StaticFS.Open("static/login.html")
		if err != nil {
			http.Error(w, "not found", 404)
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	log.Println("[Handler] auth routes registered")
}

package handler

import (
	"io"
	"io/fs"
	"log"
	"net/http"

	"watchtower/internal/auth"
	"watchtower/internal/webui"
)

func RegisterAuth(mux *http.ServeMux, secret []byte, adminUser, adminPwdHash string) {
	cfg := auth.Config{Secret: secret, AdminUser: adminUser, AdminPasswordHash: adminPwdHash}
	mux.HandleFunc("/api/auth/login", auth.LoginHandler(cfg))
	mux.HandleFunc("/api/auth/logout", auth.LogoutHandler())
	mux.HandleFunc("/api/auth/me", auth.MeHandler(secret))
	subFS, _ := fs.Sub(webui.StaticFS, "static")
	mux.Handle("/common/", http.StripPrefix("/common/", http.FileServer(http.FS(subFS))))
	// Vite SPA asset files (built to static/dist/assets/)
	distAssetsFS, _ := fs.Sub(webui.StaticFS, "static/dist/assets")
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(distAssetsFS))))
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
	// 根路径提供 Vite SPA
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		f, err := webui.StaticFS.Open("static/dist/index.html")
		if err != nil {
			// Fallback to old index.html
			f, err = webui.StaticFS.Open("static/index.html")
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			defer f.Close()
			data, _ := io.ReadAll(f)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		defer f.Close()
		data, _ := io.ReadAll(f)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	log.Println("[Handler] auth routes registered")
}

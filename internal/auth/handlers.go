package auth

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// Config 描述登录 handler 需要的账号信息，两个服务从各自的
// ADMIN_USER / ADMIN_PASSWORD_HASH / AUTH_JWT_SECRET 环境变量构造出一份相同的 Config。
type Config struct {
	Secret            []byte
	AdminUser         string
	AdminPasswordHash string // bcrypt hash
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginHandler 返回一个校验账号密码、签发 JWT 并写入共享 Cookie 的 handler。
func LoginHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Username == "" || req.Username != cfg.AdminUser {
			writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(cfg.AdminPasswordHash), []byte(req.Password)); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}

		token, err := Sign(cfg.Secret, req.Username, SessionTTL)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to sign session")
			return
		}
		SetSessionCookie(w, token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "username": req.Username})
	}
}

// LogoutHandler 清除共享会话 Cookie。
func LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ClearSessionCookie(w)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}

// MeHandler 返回当前会话是否已登录（供前端探测登录状态，不做 401/跳转，始终 200）。
// 注册路由时应将其路径加入中间件的 exemptPaths，因为它自己独立校验 Cookie。
func MeHandler(secret []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
			return
		}
		claims, err := Verify(secret, cookie.Value)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"authenticated": false})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"authenticated": true, "username": claims.Subject})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

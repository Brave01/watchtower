package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"watchtower/internal/store"

	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ---------- IP 登录频率限制 ----------

type ipAttempt struct {
	count       int
	first       time.Time
	lockedUntil time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]*ipAttempt
}

var limiter = &loginLimiter{
	attempts: make(map[string]*ipAttempt),
}

const (
	maxLoginAttempts = 5
	loginWindow      = 5 * time.Minute
	loginLockout     = 5 * time.Minute
)

func (l *loginLimiter) check(ip string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	a, exists := l.attempts[ip]

	// 已锁定
	if exists && now.Before(a.lockedUntil) {
		remaining := a.lockedUntil.Sub(now).Round(time.Second)
		return fmt.Errorf("登录尝试过于频繁，请在 %v 后再试", remaining)
	}

	// 窗口过期，重置
	if exists && now.Sub(a.first) > loginWindow {
		delete(l.attempts, ip)
		exists = false
	}

	if !exists {
		l.attempts[ip] = &ipAttempt{count: 1, first: now}
	} else {
		a.count++
		if a.count >= maxLoginAttempts {
			a.lockedUntil = now.Add(loginLockout)
			return fmt.Errorf("登录失败次数过多，已锁定 %v", loginLockout)
		}
	}

	return nil
}

func (l *loginLimiter) reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

func getRealIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	idx := strings.LastIndex(r.RemoteAddr, ":")
	if idx >= 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}

// -------------------------------------

// LoginHandler 从数据库读取用户密码 hash 校验登录。
func LoginHandler(secret []byte, adminUser string, s store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// IP 频率限制
		ip := getRealIP(r)
		if err := limiter.check(ip); err != nil {
			writeJSONError(w, http.StatusTooManyRequests, err.Error())
			return
		}

		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Username == "" || req.Username != adminUser {
			writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}

		// 从数据库查用户密码 hash
		user, err := s.GetUser(req.Username)
		if err != nil || user == nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}

		// 登录成功，清空该 IP 的失败记录
		limiter.reset(ip)

		token, err := Sign(secret, req.Username, SessionTTL)
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

// ChangePasswordHandler 修改密码，将新密码的 bcrypt hash 写入数据库。
func ChangePasswordHandler(secret []byte, s store.Store) http.HandlerFunc {
	var mu sync.Mutex
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// 校验当前登录身份
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		claims, err := Verify(secret, cookie.Value)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		var req changePasswordRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.NewPassword == "" {
			writeJSONError(w, http.StatusBadRequest, "new password cannot be empty")
			return
		}

		mu.Lock()
		defer mu.Unlock()

		// 从数据库读取当前密码 hash
		user, err := s.GetUser(claims.Subject)
		if err != nil || user == nil {
			writeJSONError(w, http.StatusUnauthorized, "user not found")
			return
		}
		// 校验旧密码
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
			writeJSONError(w, http.StatusUnauthorized, "old password is incorrect")
			return
		}

		// 生成新 hash
		newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to hash password")
			return
		}

		// 更新数据库
		if err := s.UpdatePassword(claims.Subject, string(newHash)); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to update password")
			return
		}

		// 重新签发 JWT（让当前会话保持有效）
		token, err := Sign(secret, claims.Subject, SessionTTL)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to sign session")
			return
		}
		SetSessionCookie(w, token)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "密码修改成功"})
	}
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CookieName 是两个服务共用的会话 Cookie 名称。
const CookieName = "scm_session"

// SessionTTL 是登录会话的有效期。
const SessionTTL = 24 * time.Hour

type contextKey string

const claimsContextKey contextKey = "scm_claims"

// ClaimsFromContext 取出中间件放入 context 的 Claims（未登录路由不会有）。
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsContextKey).(*Claims)
	return c, ok
}

// Middleware 用共享密钥校验请求携带的 scm_session Cookie。
// exemptPaths 中的路径（精确匹配或以 "*" 结尾的前缀匹配）无需登录即可访问。
// 校验失败时：/api/ 开头的请求返回 401 JSON；其余请求 302 跳转到 /login。
func Middleware(secret []byte, exemptPaths []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isExempt(r.URL.Path, exemptPaths) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(CookieName)
			if err != nil {
				denyUnauthenticated(w, r)
				return
			}
			claims, err := Verify(secret, cookie.Value)
			if err != nil {
				denyUnauthenticated(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func isExempt(path string, exemptPaths []string) bool {
	for _, p := range exemptPaths {
		if strings.HasSuffix(p, "*") {
			if strings.HasPrefix(path, strings.TrimSuffix(p, "*")) {
				return true
			}
			continue
		}
		if path == p {
			return true
		}
	}
	return false
}

func denyUnauthenticated(w http.ResponseWriter, r *http.Request) {
	// WebSocket 升级请求（如 /ws、/api/ssh/ws）无法跟随 302 重定向，一律按 API 语义返回 401。
	if strings.HasPrefix(r.URL.Path, "/api/") || r.Header.Get("Upgrade") != "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
		return
	}
	redirect := "/login?redirect=" + url.QueryEscape(r.URL.RequestURI())
	http.Redirect(w, r, redirect, http.StatusFound)
}

// SetSessionCookie 把签好的 JWT 写入响应的 Cookie。
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
}

// ClearSessionCookie 登出时清除 Cookie。
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

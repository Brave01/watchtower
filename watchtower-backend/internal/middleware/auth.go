package middleware

import (
	"context"
	"net/http"

	"watchtower/internal/auth"
)

type contextKey string

const (
	claimsKey contextKey = "claims"
)

func Auth(secret []byte, exemptPaths []string) func(http.Handler) http.Handler {
	return auth.Middleware(secret, exemptPaths)
}

func ClaimsFromContext(ctx context.Context) *auth.Claims {
	c, _ := auth.ClaimsFromContext(ctx)
	return c
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	auth.SetSessionCookie(w, token)
}

func ClearSessionCookie(w http.ResponseWriter) {
	auth.ClearSessionCookie(w)
}

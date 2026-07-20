package user

import (
	"net/http"

	"watchtower/internal/store"
)

type AuthHandler struct {
	store     store.Store
	secret    []byte
	adminUser string
}

func NewAuthHandler(secret []byte, adminUser string, s store.Store) *AuthHandler {
	return &AuthHandler{
		store:     s,
		secret:    secret,
		adminUser: adminUser,
	}
}

func (h *AuthHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/auth/login", h.Login)
	mux.HandleFunc("/api/auth/logout", h.Logout)
	mux.HandleFunc("/api/auth/me", h.Me)
	mux.HandleFunc("/api/auth/change-password", h.ChangePassword)
}

package user

import (
	"encoding/json"
	"net/http"

	"watchtower/internal/auth"
	"watchtower/pkg/utils"

	"golang.org/x/crypto/bcrypt"
)

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	httpHandler := auth.LoginHandler(h.secret, h.adminUser, h.store)
	httpHandler(w, r)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	auth.ClearSessionCookie(w)
	utils.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]bool{"authenticated": false})
		return
	}
	claims, err := auth.Verify(h.secret, cookie.Value)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]bool{"authenticated": false})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"authenticated": true, "username": claims.Subject})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	cookie, err := r.Cookie(auth.CookieName)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	claims, err := auth.Verify(h.secret, cookie.Value)
	if err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.NewPassword == "" {
		utils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "new password cannot be empty"})
		return
	}

	user, err := h.store.GetUser(claims.Subject)
	if err != nil || user == nil {
		utils.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		utils.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "old password is incorrect"})
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		utils.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
		return
	}
	if err := h.store.UpdatePassword(claims.Subject, string(newHash)); err != nil {
		utils.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update password"})
		return
	}

	token, err := auth.Sign(h.secret, claims.Subject, auth.SessionTTL)
	if err != nil {
		utils.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to sign session"})
		return
	}
	auth.SetSessionCookie(w, token)
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "message": "密码修改成功"})
}

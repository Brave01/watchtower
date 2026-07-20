package v1

import (
	"net/http"

	"watchtower/internal/app/v1/dashboard"
	"watchtower/internal/app/v1/logmonitor"
	"watchtower/internal/app/v1/user"
)

type V1App struct {
	Auth       *user.AuthHandler
	Dashboard  *dashboard.DashboardHandler
	LogMonitor *logmonitor.LogMonitorHandler
}

func New(auth *user.AuthHandler, dash *dashboard.DashboardHandler, lm *logmonitor.LogMonitorHandler) *V1App {
	return &V1App{
		Auth:       auth,
		Dashboard:  dash,
		LogMonitor: lm,
	}
}

func (v *V1App) RegisterAll(mux *http.ServeMux) {
	v.Auth.Register(mux)
	v.Dashboard.Register(mux)
	v.LogMonitor.Register(mux)
}

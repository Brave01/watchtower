package dashboard

import (
	"net/http"

	"watchtower/internal/store"
)

type DashboardHandler struct {
	store     store.Store
	scheduler *SchedulerWrapper
}

type SchedulerWrapper struct {
	Trigger  func()
	ProbeAll func()
	ProbeOne func(hostID string)
}

func NewDashboardHandler(s store.Store) *DashboardHandler {
	return &DashboardHandler{store: s}
}

func (h *DashboardHandler) SetScheduler(sched *SchedulerWrapper) {
	h.scheduler = sched
}

func (h *DashboardHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard", h.GetDashboard)
	mux.HandleFunc("/api/hosts", h.HandleHosts)
	mux.HandleFunc("/api/hosts/update", h.HandleHostUpdate)
	mux.HandleFunc("/api/hosts/batch", h.HandleBatchHosts)
	mux.HandleFunc("/api/hosts/export", h.HandleExport)
	mux.HandleFunc("/api/hosts/maintenance", h.HandleMaintenance)
	mux.HandleFunc("/api/hosts/collect", h.HandleCollect)
	mux.HandleFunc("/api/roles", h.HandleRoles)
	mux.HandleFunc("/api/roles/batch", h.HandleBatchRoles)
	mux.HandleFunc("/api/assign", h.HandleAssign)
	mux.HandleFunc("/api/assign/batch", h.HandleBatchAssign)
	mux.HandleFunc("/api/refresh", h.HandleRefresh)
	mux.HandleFunc("/api/ssh-credential", h.HandleSSHCredential)
	mux.HandleFunc("/api/ssh/ws", h.HandleSSHWebSocket)
	mux.HandleFunc("/api/hosts/batch-simple", h.HandleBatchSimple)
	mux.HandleFunc("/api/hosts/batch/status", h.HandleBatchStatus)
	mux.HandleFunc("/api/hosts/batch-delete", h.HandleBatchDelete)
}

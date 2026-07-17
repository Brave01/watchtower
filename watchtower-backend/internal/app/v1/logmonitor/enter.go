package logmonitor

import (
	"net/http"

	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/store"
)

type LogMonitorHandler struct {
	store          store.Store
	wsHub          *ws.Hub
	filter         *filter.Engine
	webhook        *webhook.Client
	webhookClients map[int]*webhook.Client
	esPipeline     ESPipeline
}

type ESPipeline interface {
	Status() string
	LastError() string
	IsRunning() bool
	Start(cfg interface{}) error
	Stop()
}

func NewLogMonitorHandler(s store.Store, wsHub *ws.Hub, filter *filter.Engine, wh *webhook.Client, whClients map[int]*webhook.Client, pipeline ESPipeline) *LogMonitorHandler {
	return &LogMonitorHandler{
		store:          s,
		wsHub:          wsHub,
		filter:         filter,
		webhook:        wh,
		webhookClients: whClients,
		esPipeline:     pipeline,
	}
}

func (h *LogMonitorHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/api/health", h.HandleHealth)
	mux.HandleFunc("/api/stats", h.HandleStats)
	mux.HandleFunc("/api/rules", h.HandleRules)
	mux.HandleFunc("/api/rules/update", h.HandleRuleUpdate)
	mux.HandleFunc("/api/rules/delete", h.HandleRuleDelete)
	mux.HandleFunc("/api/webhook/config", h.HandleWebhookConfig)
	mux.HandleFunc("/api/webhook/test", h.HandleWebhookTest)
	mux.HandleFunc("/api/webhook/limited-alerts", h.HandleLimitedAlerts)
	mux.HandleFunc("/api/webhook/limited-alerts/history", h.HandleLimitedHistory)
	mux.HandleFunc("/api/webhook/limited-alerts/clear", h.HandleLimitedClear)
	mux.HandleFunc("/api/webhook/limited-alerts/cleanup", h.HandleLimitedCleanup)
	mux.HandleFunc("/api/es/config", h.HandleESConfig)
	if h.wsHub != nil {
		mux.HandleFunc("/ws", h.wsHub.HandleWS)
	}
}

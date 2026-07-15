package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/store"
)

type LogMonitorDeps struct {
	WsHub          *ws.Hub
	Store          store.Store
	Filter         *filter.Engine
	Webhook        *webhook.Client         // 默认 Webhook 客户端（向后兼容）
	WebhookClients map[int]*webhook.Client // 多 Webhook 客户端，key 为 DB ID
	ESPipeline     *ESPipeline             // ES 日志轮询管道
}

func RegisterLogMonitor(mux *http.ServeMux, deps *LogMonitorDeps) {
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/stats", handleStats(deps))
	mux.HandleFunc("/api/rules", handleRules(deps))
	mux.HandleFunc("/api/rules/update", handleRuleUpdate(deps))
	mux.HandleFunc("/api/rules/delete", handleRuleDelete(deps))
	mux.HandleFunc("/api/webhook/config", handleWebhookConfig(deps))
	mux.HandleFunc("/api/webhook/test", handleWebhookTest(deps))
	mux.HandleFunc("/api/webhook/limited-alerts", handleLimitedAlerts(deps))
	mux.HandleFunc("/api/webhook/limited-alerts/history", handleLimitedHistory(deps))
	mux.HandleFunc("/api/webhook/limited-alerts/clear", handleLimitedClear(deps))
	mux.HandleFunc("/api/webhook/limited-alerts/cleanup", handleLimitedCleanup(deps))
	mux.HandleFunc("/api/es/config", handleESConfig(deps))
	if deps.WsHub != nil {
		mux.HandleFunc("/ws", deps.WsHub.HandleWS)
	}
	log.Println("[INFO] [Handler] log-monitor routes registered")
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleStats(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		stats := map[string]interface{}{"ws_clients": 0, "dedup_size": 0, "rule_count": 0, "uptime": time.Now().Format(time.RFC3339)}
		if deps.WsHub != nil {
			stats["ws_clients"] = deps.WsHub.ClientCount()
		}
		if deps.Filter != nil {
			stats["rule_count"] = len(deps.Filter.GetRules())
		}
		// 汇总所有 Webhook 客户端统计
		var totalSent, totalLimited int64
		if deps.Webhook != nil {
			if rl := deps.Webhook.GetRateLimitStats(); rl != nil {
				totalSent += rl.TotalSent
				totalLimited += rl.TotalLimited
				stats["rate_limit"] = rl
			}
		}
		if deps.WebhookClients != nil {
			allRl := []*webhook.RateLimitStats{}
			for _, client := range deps.WebhookClients {
				if rl := client.GetRateLimitStats(); rl != nil {
					totalSent += rl.TotalSent
					totalLimited += rl.TotalLimited
					allRl = append(allRl, rl)
				}
			}
			stats["webhook_stats"] = allRl
		}
		stats["total_sent"] = totalSent
		stats["total_limited"] = totalLimited
		json.NewEncoder(w).Encode(stats)
	}
}

func handleRules(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			rules, _ := deps.Store.ListAlertRules()
			json.NewEncoder(w).Encode(map[string]interface{}{"rules": rules})
			return
		}
		if r.Method == "POST" {
			var rule store.AlertRule
			if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			if rule.ID == "" {
				rule.ID = uuid.New().String()
			}
			deps.Store.SaveAlertRule(&rule)
			// 同步到内存中的过滤引擎
			if deps.Filter != nil {
				fr := &filter.AlertRule{
					ID:              rule.ID,
					Name:            rule.Name,
					Enabled:         rule.Enabled,
					Keywords:        splitJSONOrComma(rule.Keywords),
					ExcludeKeywords: splitJSONOrComma(rule.ExcludeKeywords),
					Level:           rule.Level,
					RegexPattern:    rule.RegexPattern,
					Cooldown:        rule.Cooldown,
					MessageTemplate: rule.MessageTemplate,
					WebhookID:       rule.WebhookID,
				}
				deps.Filter.AddRule(fr)
			}
			json.NewEncoder(w).Encode(map[string]string{"message": "ok"})
			return
		}
		http.Error(w, "method not allowed", 405)
	}
}

func handleRuleUpdate(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		var rule store.AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if rule.ID == "" {
			rule.ID = r.URL.Query().Get("id")
		}
		if rule.ID == "" {
			rule.ID = uuid.New().String()
		}
		// 如果是部分更新（仅 toggle 启禁用），先从 DB 读取已有记录合并
		if rule.Name == "" && rule.Keywords == "" {
			existing, _ := deps.Store.GetAlertRule(rule.ID)
			if existing != nil {
				if rule.Name == "" {
					rule.Name = existing.Name
				}
				if rule.Keywords == "" {
					rule.Keywords = existing.Keywords
				}
				if rule.Level == "" {
					rule.Level = existing.Level
				}
				if rule.Cooldown == 0 {
					rule.Cooldown = existing.Cooldown
				}
				if rule.WebhookID == 0 {
					rule.WebhookID = existing.WebhookID
				}
			}
		}
		deps.Store.SaveAlertRule(&rule)
		// 同步到内存中的过滤引擎
		if deps.Filter != nil {
			fr := &filter.AlertRule{
				ID:              rule.ID,
				Name:            rule.Name,
				Enabled:         rule.Enabled,
				Keywords:        splitJSONOrComma(rule.Keywords),
				ExcludeKeywords: splitJSONOrComma(rule.ExcludeKeywords),
				Level:           rule.Level,
				RegexPattern:    rule.RegexPattern,
				Cooldown:        rule.Cooldown,
				MessageTemplate: rule.MessageTemplate,
				WebhookID:       rule.WebhookID,
			}
			deps.Filter.UpdateRule(fr)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "ok", "id": rule.ID})
	}
}

func handleRuleDelete(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "id required"})
			return
		}
		deps.Store.DeleteAlertRule(id)
		// 从内存中的过滤引擎删除
		if deps.Filter != nil {
			deps.Filter.DeleteRule(id)
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "deleted"})
	}
}

func handleWebhookConfig(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			configs, _ := deps.Store.ListWebhookConfigs()
			if configs == nil {
				configs = []store.WebhookConfig{}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"webhooks": configs})
			return
		}
		if r.Method == "POST" {
			var cfg store.WebhookConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				http.Error(w, "bad request", 400)
				return
			}
			deps.Store.SaveWebhookConfig(&cfg)
			// 同步更新内存中的 Webhook 客户端
			if cfg.ID > 0 && deps.WebhookClients != nil {
				if client, ok := deps.WebhookClients[cfg.ID]; ok {
					client.UpdateConfig(&webhook.WebhookConfig{
						Platform:           cfg.Platform,
						URL:                cfg.URL,
						Secret:             cfg.Secret,
						Enabled:            cfg.Enabled,
						MaxRetries:         cfg.MaxRetries,
						MentionType:        cfg.MentionType,
						MentionUsers:       splitComma(cfg.MentionUsers),
						RateLimit:          cfg.RateLimit,
						RateLimitPerSecond: cfg.RateLimitPerSecond,
						RingBufferSize:     cfg.RingBufferSize,
						Template:           cfg.Template,
					})
				} else {
					// 新建客户端（页面新增的 webhook 不在启动时的 map 中）
					client := webhook.NewClient(&webhook.WebhookConfig{
						Platform:           cfg.Platform,
						URL:                cfg.URL,
						Secret:             cfg.Secret,
						Enabled:            cfg.Enabled,
						MaxRetries:         cfg.MaxRetries,
						MentionType:        cfg.MentionType,
						MentionUsers:       splitComma(cfg.MentionUsers),
						RateLimit:          cfg.RateLimit,
						RateLimitPerSecond: cfg.RateLimitPerSecond,
						RingBufferSize:     cfg.RingBufferSize,
						Template:           cfg.Template,
					})
					deps.WebhookClients[cfg.ID] = client
				}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"message": "ok", "id": cfg.ID})
			return
		}
		if r.Method == "DELETE" {
			idStr := r.URL.Query().Get("id")
			if idStr == "" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "id required"})
				return
			}
			var id int
			fmt.Sscanf(idStr, "%d", &id)
			deps.Store.DeleteWebhookConfig(id)
			// 从内存中移除客户端
			if deps.WebhookClients != nil {
				delete(deps.WebhookClients, id)
			}
			json.NewEncoder(w).Encode(map[string]string{"message": "deleted"})
			return
		}
		http.Error(w, "method not allowed", 405)
	}
}

func handleWebhookTest(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "method not allowed", 405)
			return
		}
		var req struct {
			WebhookID int    `json:"webhook_id"`
			RuleName  string `json:"rule_name"`
			Message   string `json:"message"`
			Level     string `json:"level"`
			Source    string `json:"source"`
			Template  string `json:"template"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		// 确定使用哪个 webhook 客户端
		var client *webhook.Client
		if req.WebhookID > 0 && deps.WebhookClients != nil {
			if c, ok := deps.WebhookClients[req.WebhookID]; ok {
				client = c
			}
		}
		if client == nil {
			client = deps.Webhook
		}
		if client == nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "Webhook 未配置"})
			return
		}
		ruleName := req.RuleName
		if ruleName == "" {
			ruleName = "测试告警"
		}
		message := req.Message
		if message == "" {
			message = "这是一条测试告警消息"
		}
		level := req.Level
		if level == "" {
			level = "INFO"
		}
		source := req.Source
		if source == "" {
			source = "webhook-test"
		}
		template := req.Template
		p := &parser.ParsedLog{Message: message, Level: level, Source: source, Timestamp: time.Now().Format(time.RFC3339)}
		if err := client.SendAlert(ruleName, p, template); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "测试消息已发送"})
	}
}

func handleLimitedAlerts(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alerts, _ := deps.Store.ListLimitedAlerts(200, 0)
		dbTotal, _ := deps.Store.CountLimitedAlerts()
		// deps.Webhook may be nil; if set, also get its in-memory buffer
		var bufAlerts []map[string]interface{}
		if deps.Webhook != nil {
			for _, e := range deps.Webhook.GetLimitedAlerts() {
				bufAlerts = append(bufAlerts, map[string]interface{}{
					"rule_name": e.RuleName, "message": e.Message,
					"level": e.Level, "source": e.Source,
					"timestamp": e.Timestamp, "limited_at": e.LimitedAt,
					"summary": e.Summary,
				})
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"alerts": bufAlerts, "db_alerts": alerts, "db_total": dbTotal})
	}
}

func handleLimitedHistory(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, pageSize := 1, 20
		if p := r.URL.Query().Get("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if ps := r.URL.Query().Get("page_size"); ps != "" {
			fmt.Sscanf(ps, "%d", &pageSize)
		}
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 20
		}
		offset := (page - 1) * pageSize
		records, _ := deps.Store.ListLimitedAlerts(pageSize, offset)
		total, _ := deps.Store.CountLimitedAlerts()
		totalPages := (total + pageSize - 1) / pageSize
		json.NewEncoder(w).Encode(map[string]interface{}{
			"records": records, "total": total,
			"page": page, "page_size": pageSize, "total_pages": totalPages,
		})
	}
}

func handleLimitedClear(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Webhook != nil {
			deps.Webhook.ClearLimitedAlerts()
		}
		deps.Store.ClearLimitedAlerts()
		json.NewEncoder(w).Encode(map[string]string{"message": "cleared"})
	}
}

func handleLimitedCleanup(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n, _ := deps.Store.DeleteOldLimitedAlerts(time.Now().Add(-24 * time.Hour))
		json.NewEncoder(w).Encode(map[string]interface{}{"message": "cleanup done", "deleted": n})
	}
}

// splitComma 将逗号分隔的字符串拆分为切片，过滤空值
func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// splitJSONOrComma 将字符串拆分为切片，支持 JSON 数组格式和逗号分隔两种格式
func splitJSONOrComma(s string) []string {
	if s == "" {
		return nil
	}
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "[") {
		var items []string
		if err := json.Unmarshal([]byte(trimmed), &items); err == nil {
			result := make([]string, 0, len(items))
			for _, item := range items {
				if item != "" {
					result = append(result, item)
				}
			}
			return result
		}
	}
	// 兼容旧格式：逗号分隔
	return splitComma(s)
}

func handleESConfig(deps *LogMonitorDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			cfg, _ := deps.Store.GetESConfig()
			status := "disconnected"
			if deps.ESPipeline != nil && deps.ESPipeline.IsRunning() {
				status = "connected"
			}
			resp := map[string]interface{}{
				"config": &store.ESConfig{
					Address:  "",
					Username: "",
					Password: "",
					Index:    "logs-*",
					Interval: 15,
					Size:     100,
					Query:    "",
					Enabled:  false,
				},
				"status": status,
			}
			if cfg != nil {
				// 不返回密码
				cfg.Password = ""
				resp["config"] = cfg
			}
			json.NewEncoder(w).Encode(resp)

		case http.MethodPost:
			var req struct {
				Address  string `json:"address"`
				Username string `json:"username"`
				Password string `json:"password"`
				Index    string `json:"index"`
				Interval int    `json:"interval"`
				Size     int    `json:"size"`
				Query    string `json:"query"`
				Enabled  bool   `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": "无效 JSON"})
				return
			}

			// 读取已有配置以保留密码（如果前端未传）
			if req.Password == "" {
				existing, err := deps.Store.GetESConfig()
				if err == nil && existing != nil && existing.Password != "" {
					req.Password = existing.Password
				}
			}

			cfg := &store.ESConfig{
				ID:       1,
				Address:  req.Address,
				Username: req.Username,
				Password: req.Password,
				Index:    req.Index,
				Interval: req.Interval,
				Size:     req.Size,
				Query:    req.Query,
				Enabled:  req.Enabled,
			}
			if err := deps.Store.SaveESConfig(cfg); err != nil {
				json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": err.Error()})
				return
			}

			// 重新启动 ES 管道
			if deps.ESPipeline != nil {
				cfg.Password = req.Password
				if err := deps.ESPipeline.Start(cfg); err != nil {
					log.Printf("[ES] 启动管道失败: %v", err)
					json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "配置已保存，但启动失败: " + err.Error()})
					return
				}
			}

			json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "配置已保存"})

		default:
			json.NewEncoder(w).Encode(map[string]interface{}{"error": "method not allowed"})
		}
	}
}

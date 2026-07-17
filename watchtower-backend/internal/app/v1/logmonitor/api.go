package logmonitor

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	"watchtower/internal/model"
	"watchtower/pkg/utils"

	"github.com/google/uuid"
)

func (h *LogMonitorHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	utils.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *LogMonitorHandler) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats := map[string]interface{}{
		"ws_clients": 0, "dedup_size": 0, "rule_count": 0,
		"uptime": time.Now().Format(time.RFC3339),
	}
	if h.wsHub != nil {
		stats["ws_clients"] = h.wsHub.ClientCount()
	}
	if h.filter != nil {
		stats["rule_count"] = len(h.filter.GetRules())
	}
	var totalSent, totalLimited int64
	var totalBufferUsed int
	if h.webhook != nil {
		if rl := h.webhook.GetRateLimitStats(); rl != nil {
			totalSent += rl.TotalSent
			totalLimited += rl.TotalLimited
			totalBufferUsed += rl.BufferUsed
			stats["rate_limit"] = rl
		}
	}
	if h.webhookClients != nil {
		allRl := []*webhook.RateLimitStats{}
		for _, client := range h.webhookClients {
			if rl := client.GetRateLimitStats(); rl != nil {
				totalSent += rl.TotalSent
				totalLimited += rl.TotalLimited
				totalBufferUsed += rl.BufferUsed
				allRl = append(allRl, rl)
			}
		}
		stats["webhook_stats"] = allRl
	}
	stats["total_sent"] = totalSent
	stats["total_limited"] = totalLimited
	stats["buffer_used"] = totalBufferUsed
	utils.WriteJSON(w, http.StatusOK, stats)
}

func (h *LogMonitorHandler) HandleRules(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		rules, _ := h.store.ListAlertRules()

		type ruleWithStats struct {
			model.AlertRule
			RateLimitStats *webhook.RateLimitStats `json:"rate_limit_stats,omitempty"`
		}
		result := make([]ruleWithStats, 0, len(rules))
		for _, rule := range rules {
			item := ruleWithStats{AlertRule: rule}
			whID := rule.WebhookID
			if whID > 0 && h.webhookClients != nil {
				if c, ok := h.webhookClients[whID]; ok {
					item.RateLimitStats = c.GetRateLimitStats()
				}
			}
			if item.RateLimitStats == nil && h.webhook != nil {
				item.RateLimitStats = h.webhook.GetRateLimitStats()
			}
			result = append(result, item)
		}
		utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"rules": result})
		return
	}
	if r.Method == "POST" {
		var rule model.AlertRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if rule.ID == "" {
			rule.ID = uuid.New().String()
		}
		h.store.SaveAlertRule(&rule)
		if h.filter != nil {
			fr := &filter.AlertRule{
				ID:              rule.ID,
				Name:            rule.Name,
				Enabled:         rule.Enabled,
				Keywords:        utils.SplitStrings(rule.Keywords),
				ExcludeKeywords: utils.SplitStrings(rule.ExcludeKeywords),
				Level:           rule.Level,
				RegexPattern:    rule.RegexPattern,
				Cooldown:        rule.Cooldown,
				MessageTemplate: rule.MessageTemplate,
				WebhookID:       rule.WebhookID,
			}
			h.filter.AddRule(fr)
		}
		utils.WriteJSON(w, http.StatusOK, map[string]string{"message": "ok"})
		return
	}
	http.Error(w, "method not allowed", 405)
}

func (h *LogMonitorHandler) HandleRuleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var rule model.AlertRule
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
	if rule.Name == "" && rule.Keywords == "" {
		existing, _ := h.store.GetAlertRule(rule.ID)
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
	h.store.SaveAlertRule(&rule)
	if h.filter != nil {
		fr := &filter.AlertRule{
			ID:              rule.ID,
			Name:            rule.Name,
			Enabled:         rule.Enabled,
			Keywords:        utils.SplitStrings(rule.Keywords),
			ExcludeKeywords: utils.SplitStrings(rule.ExcludeKeywords),
			Level:           rule.Level,
			RegexPattern:    rule.RegexPattern,
			Cooldown:        rule.Cooldown,
			MessageTemplate: rule.MessageTemplate,
			WebhookID:       rule.WebhookID,
		}
		h.filter.UpdateRule(fr)
	}
	utils.WriteJSON(w, http.StatusOK, map[string]interface{}{"message": "ok", "id": rule.ID})
}

func (h *LogMonitorHandler) HandleRuleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "id required"})
		return
	}
	h.store.DeleteAlertRule(id)
	if h.filter != nil {
		h.filter.DeleteRule(id)
	}
	json.NewEncoder(w).Encode(map[string]string{"message": "deleted"})
}

func (h *LogMonitorHandler) HandleWebhookConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		configs, _ := h.store.ListWebhookConfigs()
		if configs == nil {
			configs = []model.WebhookConfig{}
		}
		log.Printf("[Webhook] 查询配置: 共 %d 条", len(configs))
		json.NewEncoder(w).Encode(map[string]interface{}{"webhooks": configs})
		return
	}
	if r.Method == "POST" {
		var cfg model.WebhookConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			log.Printf("[Webhook] 保存失败: 请求体解析失败: %s", err)
			http.Error(w, "bad request", 400)
			return
		}
		h.store.SaveWebhookConfig(&cfg)
		if cfg.ID > 0 && h.webhookClients != nil {
			if client, ok := h.webhookClients[cfg.ID]; ok {
				client.UpdateConfig(&webhook.WebhookConfig{
					Platform:           cfg.Platform,
					URL:                cfg.URL,
					Secret:             cfg.Secret,
					Enabled:            cfg.Enabled,
					MaxRetries:         cfg.MaxRetries,
					MentionType:        cfg.MentionType,
					MentionUsers:       utils.SplitComma(cfg.MentionUsers),
					RateLimit:          cfg.RateLimit,
					RateLimitPerSecond: cfg.RateLimitPerSecond,
					RingBufferSize:     cfg.RingBufferSize,
					Template:           cfg.Template,
				})
				log.Printf("[Webhook] 配置已更新: id=%d, name=%s, url=%s, enabled=%v", cfg.ID, cfg.Name, cfg.URL, cfg.Enabled)
			} else {
				client := webhook.NewClient(&webhook.WebhookConfig{
					Platform:           cfg.Platform,
					URL:                cfg.URL,
					Secret:             cfg.Secret,
					Enabled:            cfg.Enabled,
					MaxRetries:         cfg.MaxRetries,
					MentionType:        cfg.MentionType,
					MentionUsers:       utils.SplitComma(cfg.MentionUsers),
					RateLimit:          cfg.RateLimit,
					RateLimitPerSecond: cfg.RateLimitPerSecond,
					RingBufferSize:     cfg.RingBufferSize,
					Template:           cfg.Template,
				})
				h.webhookClients[cfg.ID] = client
				log.Printf("[Webhook] 配置已创建: id=%d, name=%s, url=%s, enabled=%v", cfg.ID, cfg.Name, cfg.URL, cfg.Enabled)
			}
		} else {
			log.Printf("[Webhook] 配置已保存: id=%d, name=%s (未加载到内存，webhookClients=nil)", cfg.ID, cfg.Name)
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
		h.store.DeleteWebhookConfig(id)
		if h.webhookClients != nil {
			if _, ok := h.webhookClients[id]; ok {
				delete(h.webhookClients, id)
				log.Printf("[Webhook] 配置已删除: id=%d", id)
			}
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "deleted"})
		log.Printf("[Webhook] 配置已从 DB 删除: id=%d", id)
		return
	}
	http.Error(w, "method not allowed", 405)
}

func (h *LogMonitorHandler) HandleWebhookTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", 405)
		return
	}
	var req struct {
		WebhookID int    `json:"webhook_id"`
		URL       string `json:"url"`
		Platform  string `json:"platform"`
		Secret    string `json:"secret"`
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
	var client *webhook.Client
	// 测试请求：优先用传入的参数创建临时客户端，忽略 DB 中 enabled 状态
	if req.URL != "" {
		platform := req.Platform
		if platform == "" {
			platform = "feishu"
		}
		client = webhook.NewClient(&webhook.WebhookConfig{
			Platform: platform,
			URL:      req.URL,
			Secret:   req.Secret,
			Enabled:  true,
			Template: req.Template,
		})
	}
	if client == nil && req.WebhookID > 0 && h.webhookClients != nil {
		if c, ok := h.webhookClients[req.WebhookID]; ok {
			client = c
		}
	}
	if client == nil {
		client = h.webhook
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

func (h *LogMonitorHandler) HandleLimitedAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, _ := h.store.ListLimitedAlerts(200, 0)
	if alerts == nil {
		alerts = make([]model.LimitedAlert, 0)
	}
	dbTotal, _ := h.store.CountLimitedAlerts()
	bufAlerts := make([]map[string]interface{}, 0)
	if h.webhook != nil {
		for _, e := range h.webhook.GetLimitedAlerts() {
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

func (h *LogMonitorHandler) HandleLimitedHistory(w http.ResponseWriter, r *http.Request) {
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
	records, _ := h.store.ListLimitedAlerts(pageSize, offset)
	total, _ := h.store.CountLimitedAlerts()
	totalPages := (total + pageSize - 1) / pageSize
	json.NewEncoder(w).Encode(map[string]interface{}{
		"records": records, "total": total,
		"page": page, "page_size": pageSize, "total_pages": totalPages,
	})
}

func (h *LogMonitorHandler) HandleLimitedClear(w http.ResponseWriter, r *http.Request) {
	if h.webhook != nil {
		h.webhook.ClearLimitedAlerts()
	}
	h.store.ClearLimitedAlerts()
	json.NewEncoder(w).Encode(map[string]string{"message": "cleared"})
}

func (h *LogMonitorHandler) HandleLimitedCleanup(w http.ResponseWriter, r *http.Request) {
	n, _ := h.store.DeleteOldLimitedAlerts(time.Now().Add(-24 * time.Hour))
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "cleanup done", "deleted": n})
}

func (h *LogMonitorHandler) HandleESConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, _ := h.store.GetESConfig()
		status := "disconnected"
		lastError := ""
		if h.esPipeline != nil {
			status = h.esPipeline.Status()
			lastError = h.esPipeline.LastError()
		}
		resp := map[string]interface{}{
			"config": &model.ESConfig{
				Address:  "",
				Username: "",
				Password: "",
				Index:    "logs-*",
				Interval: 15,
				Size:     100,
				Query:    "",
				Enabled:  false,
			},
			"status":     status,
			"last_error": lastError,
		}
		if cfg != nil {
			hasPassword := cfg.Password != ""
			cfg.Password = ""
			resp["config"] = cfg
			resp["has_password"] = hasPassword
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
		if req.Password == "" {
			existing, err := h.store.GetESConfig()
			if err == nil && existing != nil && existing.Password != "" {
				req.Password = existing.Password
			}
		}
		cfg := &model.ESConfig{
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
		if err := h.store.SaveESConfig(cfg); err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "message": err.Error()})
			return
		}
		if h.esPipeline != nil {
			cfg.Password = req.Password
			if err := h.esPipeline.Start(cfg); err != nil {
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
	return utils.SplitComma(s)
}

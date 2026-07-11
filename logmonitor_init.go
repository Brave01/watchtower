package main

import (
	"context"
	"log"
	"time"

	"watchtower/internal/logmonitor/dedup"
	"watchtower/internal/logmonitor/es"
	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/store"
)

type LogMonitorComponents struct {
	Parser  *parser.Parser
	Deduper *dedup.Deduplicator
	WsHub   *ws.Hub
	Filter  *filter.Engine
	Webhook *webhook.Client
}

func InitLogMonitor(st store.Store, feishuCfg FeishuWebhookConfig) *LogMonitorComponents {
	logParser := parser.NewParser()
	deduper := dedup.NewDeduplicator(5*time.Minute, 100000)
	wsHub := ws.New()
	filterEngine := filter.NewEngine(logParser)
	rules, _ := st.ListAlertRules()
	for _, r := range rules {
		fr := &filter.AlertRule{
			ID:              r.ID,
			Name:            r.Name,
			Enabled:         r.Enabled,
			Keywords:        splitStrings(r.Keywords),
			ExcludeKeywords: splitStrings(r.ExcludeKeywords),
			Level:           r.Level,
			RegexPattern:    r.RegexPattern,
			Cooldown:        r.Cooldown,
			MessageTemplate: r.MessageTemplate,
		}
		if err := filterEngine.AddRule(fr); err != nil {
			log.Printf("[ERROR] [Init] 加载规则失败 %s: %v", r.Name, err)
		}
	}
	log.Printf("[INFO] [Init] 已加载 %d 条告警规则", len(rules))
	var whClient *webhook.Client
	dbWhCfgs, _ := st.ListWebhookConfigs()
	if len(dbWhCfgs) > 0 && dbWhCfgs[0].URL != "" {
		dbWhCfg := dbWhCfgs[0]
		whClient = webhook.NewClient(&webhook.WebhookConfig{
			URL:                dbWhCfg.URL,
			MaxRetries:         dbWhCfg.MaxRetries,
			MentionType:        dbWhCfg.MentionType,
			MentionUsers:       splitStrings(dbWhCfg.MentionUsers),
			RateLimit:          dbWhCfg.RateLimit,
			RateLimitPerSecond: dbWhCfg.RateLimitPerSecond,
			RingBufferSize:     dbWhCfg.RingBufferSize,
		})
	} else {
		yamlCfg := &webhook.WebhookConfig{URL: feishuCfg.URL, MaxRetries: feishuCfg.MaxRetries}
		if yamlCfg.RateLimit == 0 {
			yamlCfg.RateLimit = 20
		}
		if yamlCfg.RateLimitPerSecond == 0 {
			yamlCfg.RateLimitPerSecond = 5
		}
		whClient = webhook.NewClient(yamlCfg)
	}
	whClient.SetOverflowHandler(func(entry *webhook.LimitedAlertEntry) {
		limitedAt := entry.LimitedAt
		if limitedAt.IsZero() {
			limitedAt = time.Now()
		}
		st.SaveLimitedAlert(&store.LimitedAlert{
			RuleName:  entry.RuleName,
			Message:   entry.Message,
			Level:     entry.Level,
			Source:    entry.Source,
			Timestamp: entry.Timestamp,
			LimitedAt: limitedAt.Format(time.RFC3339),
			Summary:   entry.Summary,
		})
	})
	return &LogMonitorComponents{Parser: logParser, Deduper: deduper, WsHub: wsHub, Filter: filterEngine, Webhook: whClient}
}

func StartESPipeline(ctx context.Context, comps *LogMonitorComponents, st store.Store, esCfg ElasticsearchConfig, esPassword string) {
	if esCfg.Address == "" || esPassword == "" {
		log.Printf("[INFO] [ES] 未配置，跳过")
		return
	}
	size := esCfg.Size
	if size <= 0 {
		size = 100
	}
	esClient, err := es.NewClient(esCfg.Address, esCfg.Username, esPassword, esCfg.Index, esCfg.Interval, size, esCfg.Query)
	if err != nil {
		log.Printf("[ERROR] [ES] 初始化失败: %v", err)
		return
	}
	log.Printf("[INFO] [ES] %s -> %s (%ds)", esCfg.Address, esCfg.Index, esCfg.Interval)
	comps.Webhook.StartDrainLoop(ctx, func() []*webhook.LimitedAlertEntry {
		alerts, _ := st.LoadLimitedAlertsForRetry(100)
		if len(alerts) == 0 {
			return nil
		}
		entries := make([]*webhook.LimitedAlertEntry, len(alerts))
		for i, a := range alerts {
			entries[i] = &webhook.LimitedAlertEntry{RuleName: a.RuleName, Message: a.Message, Level: a.Level}
		}
		return entries
	}, nil)
	go esClient.Start(ctx, func(entries []es.LogEntry) {
		for _, entry := range entries {
			parsedLog, err := comps.Parser.Parse(entry.RawJSON)
			if err != nil {
				continue
			}
			if comps.Deduper.CheckAndMark(dedup.GenerateKey(entry.RawJSON)) {
				continue
			}
			results := comps.Filter.Filter(parsedLog)
			if len(results) == 0 {
				comps.WsHub.Broadcast(ws.LogEvent{Type: "raw_log", Data: parsedLog})
				continue
			}
			for _, result := range results {
				comps.WsHub.Broadcast(ws.LogEvent{Type: "log_match", Data: result})
				if result.IsAlert {
					rule := comps.Filter.GetRule(result.RuleID)
					template := ""
					if rule != nil {
						template = rule.MessageTemplate
					}
					comps.Webhook.SendAlert(result.RuleName, parsedLog, template)
					if rl := comps.Webhook.GetRateLimitStats(); rl != nil {
						log.Printf("[INFO] [ESPipeline] 告警处理: %s [令牌剩余: %.0f/%.0f]", result.RuleName, rl.RemainingMinute, rl.RemainingSecond)
					}
				}
			}
		}
	})
}

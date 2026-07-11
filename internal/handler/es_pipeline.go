package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"watchtower/internal/logmonitor/dedup"
	"watchtower/internal/logmonitor/es"
	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/store"
)

type ESPipeline struct {
	mu     sync.Mutex
	client *es.Client
	cancel context.CancelFunc

	Parser    *parser.Parser
	Deduper   *dedup.Deduplicator
	Filter    *filter.Engine
	WsHub     *ws.Hub
	Webhooks  map[int]*webhook.Client
	DefaultWH *webhook.Client
}

func NewESPipeline(p *parser.Parser, d *dedup.Deduplicator, f *filter.Engine, w *ws.Hub, whs map[int]*webhook.Client, defWh *webhook.Client) *ESPipeline {
	return &ESPipeline{
		Parser:    p,
		Deduper:   d,
		Filter:    f,
		WsHub:     w,
		Webhooks:  whs,
		DefaultWH: defWh,
	}
}

func (ep *ESPipeline) Start(cfg *store.ESConfig) error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.cancel != nil {
		ep.cancel()
		ep.cancel = nil
	}
	if ep.client != nil {
		ep.client.Stop()
		ep.client = nil
	}

	if cfg == nil || cfg.Address == "" || !cfg.Enabled {
		log.Printf("[INFO] [ESPipeline] ES 未配置或未启用，跳过启动")
		return nil
	}

	password := cfg.Password
	var queryMap map[string]interface{}
	if cfg.Query != "" {
		json.Unmarshal([]byte(cfg.Query), &queryMap)
	}
	if queryMap == nil {
		queryMap = map[string]interface{}{}
	}

	size := 100
	if cfg.Size > 0 {
		size = cfg.Size
	}

	client, err := es.NewClient(cfg.Address, cfg.Username, password, cfg.Index, cfg.Interval, size, queryMap)
	if err != nil {
		return err
	}
	ep.client = client

	ctx, cancel := context.WithCancel(context.Background())
	ep.cancel = cancel

	log.Printf("[INFO] [ESPipeline] ES: %s -> %s (间隔%ds)", cfg.Address, cfg.Index, cfg.Interval)
	go client.Start(ctx, func(entries []es.LogEntry) {
		for _, entry := range entries {
			parsedLog, err := ep.Parser.Parse(entry.RawJSON)
			if err != nil {
				continue
			}
			dedupKey := dedup.GenerateKey(entry.RawJSON)
			if ep.Deduper.CheckAndMark(dedupKey) {
				continue
			}
			results := ep.Filter.Filter(parsedLog)
			if len(results) == 0 {
				ep.WsHub.Broadcast(ws.LogEvent{Type: "raw_log", Data: parsedLog})
				continue
			}
			for _, result := range results {
				ep.WsHub.Broadcast(ws.LogEvent{Type: "log_match", Data: result})
				if result.IsAlert {
					rule := ep.Filter.GetRule(result.RuleID)
					if rule != nil && !rule.Enabled {
						continue
					}
					var targetClient *webhook.Client
					whID := 0
					if rule != nil {
						whID = rule.WebhookID
					}
					if whID > 0 {
						if c, ok := ep.Webhooks[whID]; ok {
							targetClient = c
						}
					}
					if targetClient == nil {
						targetClient = ep.DefaultWH
					}
					if targetClient == nil {
						log.Printf("[WARN] [ESPipeline] 无可用 Webhook 客户端，跳过告警")
						continue
					}
					template := ""
					if rule != nil {
						template = rule.MessageTemplate
					}
					err := targetClient.SendAlert(result.RuleName, parsedLog, template)
					nowStr := time.Now().Format("15:04:05")
					tokenInfo := ""
					if rl := targetClient.GetRateLimitStats(); rl != nil {
						tokenInfo = fmt.Sprintf(" [令牌剩余: %.0f/%.0f]", rl.RemainingMinute, rl.RemainingSecond)
					}
					if err != nil {
						log.Printf("[ERROR] [ESPipeline] 告警失败: %v%s", err, tokenInfo)
						ep.WsHub.Broadcast(ws.LogEvent{Type: "log_match", Data: map[string]interface{}{
							"rule_name": result.RuleName,
							"level":     "ERROR",
							"message":   fmt.Sprintf("告警失败: %s", err.Error()),
							"time":      nowStr,
							"source":    parsedLog.Source,
						}})
					} else {
						log.Printf("[INFO] [ESPipeline] 告警成功: %s%s", result.RuleName, tokenInfo)
						ep.WsHub.Broadcast(ws.LogEvent{Type: "log_match", Data: map[string]interface{}{
							"rule_name": result.RuleName,
							"level":     "INFO",
							"message":   "告警成功",
							"time":      nowStr,
							"source":    parsedLog.Source,
						}})
					}
				}
			}
		}
	})
	return nil
}

func (ep *ESPipeline) Stop() {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if ep.cancel != nil {
		ep.cancel()
		ep.cancel = nil
	}
	if ep.client != nil {
		ep.client.Stop()
		ep.client = nil
	}
}

func (ep *ESPipeline) IsRunning() bool {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	return ep.client != nil && ep.cancel != nil
}

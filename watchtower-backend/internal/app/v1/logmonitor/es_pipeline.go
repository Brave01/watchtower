package logmonitor

import (
	"context"
	"encoding/json"
	"log"
	"sync"

	"watchtower/internal/logmonitor/dedup"
	"watchtower/internal/logmonitor/es"
	"watchtower/internal/logmonitor/filter"
	"watchtower/internal/logmonitor/parser"
	"watchtower/internal/logmonitor/webhook"
	ws "watchtower/internal/logmonitor/ws"
	"watchtower/internal/model"
)

type esPipelineImpl struct {
	dedup      *dedup.Deduplicator
	parser     *parser.Parser
	filter     *filter.Engine
	wsHub      *ws.Hub
	defaultWH  *webhook.Client
	webhookWHs map[int]*webhook.Client
	esClient   *es.Client

	mu        sync.RWMutex
	running   bool
	cancel    context.CancelFunc
	lastError string
}

func NewESPipeline(d *dedup.Deduplicator, p *parser.Parser, f *filter.Engine, wsHub *ws.Hub, defaultWH *webhook.Client, webhookWHs map[int]*webhook.Client) ESPipeline {
	return &esPipelineImpl{
		dedup:      d,
		parser:     p,
		filter:     f,
		wsHub:      wsHub,
		defaultWH:  defaultWH,
		webhookWHs: webhookWHs,
	}
}

func (ep *esPipelineImpl) Start(cfg interface{}) error {
	esCfg, ok := cfg.(*model.ESConfig)
	if !ok {
		log.Printf("[ESPipeline] 启动失败: 配置类型错误 (cfg type=%T)", cfg)
		return nil
	}

	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.running {
		log.Printf("[ESPipeline] 正在重启管道: %s/%s", esCfg.Address, esCfg.Index)
		ep.stopLocked()
	}

	var queryMap map[string]interface{}
	if esCfg.Query != "" {
		if err := json.Unmarshal([]byte(esCfg.Query), &queryMap); err != nil {
			log.Printf("[ESPipeline] 查询语句解析失败: %s, error=%v", esCfg.Query, err)
		}
	}

	client, err := es.NewClient(esCfg.Address, esCfg.Username, esCfg.Password, esCfg.Index, esCfg.Interval, esCfg.Size, queryMap)
	if err != nil {
		ep.lastError = err.Error()
		log.Printf("[ESPipeline] 客户端创建失败: %s/%s, 原因=%s", esCfg.Address, esCfg.Index, err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	ep.esClient = client
	ep.cancel = cancel
	ep.running = true
	ep.lastError = ""

	go ep.runClient(ctx, client)
	log.Printf("[ESPipeline] 管道已启动: %s/%s (interval=%ds, size=%d)", esCfg.Address, esCfg.Index, esCfg.Interval, esCfg.Size)
	return nil
}

func (ep *esPipelineImpl) Stop() {
	ep.mu.Lock()
	defer ep.mu.Unlock()
	if ep.running {
		log.Printf("[ESPipeline] 正在停止管道")
		ep.stopLocked()
		log.Printf("[ESPipeline] 管道已停止")
	}
}

func (ep *esPipelineImpl) stopLocked() {
	if ep.cancel != nil {
		ep.cancel()
		ep.cancel = nil
	}
	ep.running = false
	ep.lastError = ""
}

func (ep *esPipelineImpl) Status() string {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	if ep.running {
		return "connected"
	}
	if ep.lastError != "" {
		return "error"
	}
	return "disconnected"
}

func (ep *esPipelineImpl) LastError() string {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.lastError
}

func (ep *esPipelineImpl) IsRunning() bool {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.running
}

func (ep *esPipelineImpl) runClient(ctx context.Context, client *es.Client) {
	log.Printf("[ESPipeline] 开始 ES 日志轮询")
	n := 0
	client.Start(ctx, func(entries []es.LogEntry) {
		n++
		count := len(entries)
		log.Printf("[ESPipeline] 轮询 #%d: 获取到 %d 条日志", n, count)
		ep.handleEntries(ctx, entries)
	})
	log.Printf("[ESPipeline] 日志轮询已退出")
}

func (ep *esPipelineImpl) handleEntries(ctx context.Context, entries []es.LogEntry) {
	matched := 0
	deduped := 0
	alerted := 0
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			log.Printf("[ESPipeline] 管道已取消，停止处理 (已处理: 总%d, 去重%d, 匹配%d, 告警%d)", len(entries), deduped, matched, alerted)
			return
		default:
		}

		// 去重
		dedupKey := dedup.GenerateKey(entry.RawJSON)
		if ep.dedup.CheckAndMark(dedupKey) {
			deduped++
			continue
		}

		// 解析
		parsed, err := ep.parser.Parse(entry.RawJSON)
		if err != nil {
			log.Printf("[ESPipeline] 日志解析失败: %s", err)
			continue
		}

		// 过滤
		results := ep.filter.Filter(parsed)
		if len(results) == 0 {
			// 未匹配规则，推送原始日志
			if ep.wsHub != nil {
				ep.wsHub.Broadcast(ws.LogEvent{
					Type: "raw_log",
					Data: parsed,
				})
			}
			continue
		}

		matched++

		// 匹配规则，推送告警日志
		for _, result := range results {
			if ep.wsHub != nil {
				ep.wsHub.Broadcast(ws.LogEvent{
					Type: "log_match",
					Data: result,
				})
			}

			// 发送 Webhook 告警
			if result.IsAlert {
				alerted++
				whID := 0
				if rule := ep.filter.GetRule(result.RuleID); rule != nil {
					whID = rule.WebhookID
				}
				whClient := ep.defaultWH
				if whID > 0 && ep.webhookWHs != nil {
					if c, ok := ep.webhookWHs[whID]; ok {
						whClient = c
					}
				}
				if whClient != nil {
					go func(client *webhook.Client, ruleName, ruleID string, p *parser.ParsedLog) {
						if err := client.SendAlert(ruleName, p, ""); err != nil {
							log.Printf("[ESPipeline] Webhook 发送失败: rule=%s(%s), 原因=%s", ruleName, ruleID, err)
						}
					}(whClient, result.RuleName, result.RuleID, parsed)
				} else {
					log.Printf("[ESPipeline] Webhook 告警跳过: rule=%s, 原因=未找到可用的 Webhook 客户端", result.RuleName)
				}
			}
		}
	}
	log.Printf("[ESPipeline] 处理完成: 共%d条 (去重%d, 未匹配%d, 匹配%d, 告警%d)",
		len(entries), deduped, len(entries)-matched-deduped, matched, alerted)
}

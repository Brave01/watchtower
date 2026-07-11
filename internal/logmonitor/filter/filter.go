package filter

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"watchtower/internal/logmonitor/parser"
	"strings"
	"sync"
	"time"
)

// AlertRule 告警规则
type AlertRule struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	Keywords        []string `json:"keywords"`
	ExcludeKeywords []string `json:"exclude_keywords"`
	Level           string   `json:"level"`
	RegexPattern    string   `json:"regex_pattern"`
	Cooldown        int      `json:"cooldown"` // 冷却时间（秒），相同指纹日志的冷却期
	MessageTemplate string   `json:"message_template"`
	WebhookID       int      `json:"webhook_id"` // 使用的 webhook ID，0 表示默认
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`

	// 运行时状态 - 按指纹追踪冷却期
	fingerprintLast     map[string]time.Time
	fingerprintCooldown int // 指纹级别冷却时间（秒），默认等于 Cooldown
	regex               *regexp.Regexp
	mu                  sync.RWMutex
}

// GenerateFingerprint 根据日志生成指纹，用于判断是否为"相同日志"
// 相同指纹 = 同一来源的同一错误 → 走冷却期聚合
// 不同指纹 = 新类型的错误 → 立即告警
func (r *AlertRule) GenerateFingerprint(log *parser.ParsedLog) string {
	var base string
	if log.K8s != nil && log.K8s.PodName != "" {
		// K8s 日志: k8s:{namespace}:{pod}:{container}:{message_md5_prefix}
		base = fmt.Sprintf("k8s:%s:%s:%s", log.K8s.Namespace, log.K8s.PodName, log.K8s.ContainerName)
	} else if log.Source != "" {
		// 非 K8s 日志（nginx/mqtt/kafka/script 等）: {source}:{message_md5_prefix}
		base = fmt.Sprintf("src:%s", log.Source)
	} else {
		// 兜底
		base = "unknown"
	}
	// 加上消息内容哈希作为具体指纹
	msgHash := fmt.Sprintf("%x", md5.Sum([]byte(log.Message)))[:16]
	return fmt.Sprintf("%s:%s", base, msgHash)
}

// FilteredLog 过滤后的日志
type FilteredLog struct {
	RuleID     string            `json:"rule_id"`
	RuleName   string            `json:"rule_name"`
	ParsedLog  *parser.ParsedLog `json:"parsed_log"`
	MatchedAt  time.Time         `json:"matched_at"`
	IsExcluded bool              `json:"is_excluded"`
	IsAlert    bool              `json:"is_alert"`
	IsCooldown bool              `json:"is_cooldown"` // 是否在冷却期内
}

// Engine 过滤引擎
type Engine struct {
	mu     sync.RWMutex
	rules  map[string]*AlertRule
	parser *parser.Parser
}

// NewEngine 创建过滤引擎
func NewEngine(pr *parser.Parser) *Engine {
	return &Engine{
		rules:  make(map[string]*AlertRule),
		parser: pr,
	}
}

// initRuntime 初始化运行时状态
func (r *AlertRule) initRuntime() {
	if r.fingerprintLast == nil {
		r.fingerprintLast = make(map[string]time.Time)
	}
	if r.fingerprintCooldown == 0 {
		r.fingerprintCooldown = r.Cooldown
		if r.fingerprintCooldown <= 0 {
			r.fingerprintCooldown = 300 // 默认 5 分钟
		}
	}
}

// AddRule 添加告警规则
func (e *Engine) AddRule(rule *AlertRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if rule.RegexPattern != "" {
		re, err := regexp.Compile(rule.RegexPattern)
		if err != nil {
			return err
		}
		rule.regex = re
	}

	rule.initRuntime()
	rule.CreatedAt = time.Now().Format(time.RFC3339)
	rule.UpdatedAt = rule.CreatedAt
	e.rules[rule.ID] = rule
	return nil
}

// UpdateRule 更新告警规则
func (e *Engine) UpdateRule(rule *AlertRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if rule.RegexPattern != "" {
		re, err := regexp.Compile(rule.RegexPattern)
		if err != nil {
			return err
		}
		rule.regex = re
	}

	rule.initRuntime()
	rule.UpdatedAt = time.Now().Format(time.RFC3339)
	if existing, ok := e.rules[rule.ID]; ok {
		rule.CreatedAt = existing.CreatedAt
	}
	e.rules[rule.ID] = rule
	return nil
}

// DeleteRule 删除告警规则
func (e *Engine) DeleteRule(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.rules, id)
}

// GetRule 获取告警规则
func (e *Engine) GetRule(id string) *AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.rules[id]
}

// GetRules 获取所有告警规则
func (e *Engine) GetRules() []*AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	rules := make([]*AlertRule, 0, len(e.rules))
	for _, rule := range e.rules {
		rules = append(rules, rule)
	}
	return rules
}

// Filter 对日志执行过滤，返回匹配的日志列表
func (e *Engine) Filter(parsedLog *parser.ParsedLog) []*FilteredLog {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var results []*FilteredLog

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}

		result := e.matchRule(rule, parsedLog)
		if result != nil {
			results = append(results, result)
		}
	}

	return results
}

func (e *Engine) matchRule(rule *AlertRule, log *parser.ParsedLog) *FilteredLog {
	// 检查排除关键词
	for _, keyword := range rule.ExcludeKeywords {
		if strings.Contains(log.Message, keyword) {
			return &FilteredLog{
				RuleID:     rule.ID,
				RuleName:   rule.Name,
				ParsedLog:  log,
				MatchedAt:  time.Now(),
				IsExcluded: true,
			}
		}
	}

	// 检查日志级别
	if rule.Level != "" && !strings.EqualFold(log.Level, rule.Level) {
		return nil
	}

	// 检查关键词
	matched := false
	for _, keyword := range rule.Keywords {
		if strings.Contains(log.Message, keyword) {
			matched = true
			break
		}
	}

	// 检查正则
	if !matched && rule.regex != nil {
		matched = rule.regex.MatchString(log.Message)
	}

	if !matched {
		return nil
	}

	// 生成日志指纹
	fingerprint := rule.GenerateFingerprint(log)

	// 按指纹检查冷却时间
	// 相同指纹 = 同一来源同一错误 → 走冷却期
	// 不同指纹 = 新错误类型 → 立即告警
	rule.mu.Lock()
	rule.initRuntime()
	lastTime, exists := rule.fingerprintLast[fingerprint]
	now := time.Now()
	canAlert := true
	isCooldown := false

	if exists {
		// 相同指纹出现过，检查冷却期
		if now.Sub(lastTime) < time.Duration(rule.fingerprintCooldown)*time.Second {
			canAlert = false
			isCooldown = true
		} else {
			// 冷却期已过，更新最后触发时间
			rule.fingerprintLast[fingerprint] = now
		}
	} else {
		// 新指纹，立即告警并记录
		rule.fingerprintLast[fingerprint] = now
	}
	rule.mu.Unlock()

	return &FilteredLog{
		RuleID:     rule.ID,
		RuleName:   rule.Name,
		ParsedLog:  log,
		MatchedAt:  time.Now(),
		IsAlert:    canAlert,
		IsCooldown: isCooldown,
	}
}

package filter

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"watchtower/internal/logmonitor/parser"
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
//
// 处理策略：
//   - 去掉消息头部的 ISO 时间戳
//   - 基于规则 ID + K8s 来源 + 主要异常类名/关键词生成指纹
//   - 忽略线程名、deptNum、JVM Lambda 地址等不稳定内容
func (r *AlertRule) GenerateFingerprint(log *parser.ParsedLog) string {
	var base string
	if log.K8s != nil && log.K8s.PodName != "" {
		base = fmt.Sprintf("k8s:%s:%s", log.K8s.Namespace, log.K8s.PodName)
	} else if log.Source != "" {
		base = fmt.Sprintf("src:%s", log.Source)
	} else {
		base = "unknown"
	}

	// 去掉消息头部的时间戳
	msg := stripTimestampPrefix(log.Message)
	// 提取主要异常特征：取第一行的前 150 字符，再从中提取稳定的异常标识
	sig := extractErrorSignature(msg)

	return fmt.Sprintf("%s:%s:%s", r.ID, base, sig)
}

// extractErrorSignature 从消息中提取稳定的异常特征。
// 忽略线程名、deptNum、Lambda 地址等不稳定内容。
// 策略：取消息第一行（不含堆栈），从中提取类名和异常关键词。
func extractErrorSignature(msg string) string {
	// 取第一行（换行符之前的部分）
	firstLine := msg
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		firstLine = msg[:idx]
	}

	// 如果第一行过长则截断
	if len(firstLine) > 150 {
		firstLine = firstLine[:150]
	}

	// 对第一行做稳定化处理：
	// 1. 去掉线程名: "[ fan-agc-task14]" 或 "[ fan-agc-task20]"
	// 2. 去掉 deptNum: "deptNum: 10001011" → "deptNum:"
	// 3. 去掉 Lambda 地址: "$$Lambda/0x00007f..." → "$$Lambda"
	// 4. 去掉 "@哈希值": "@58a48e9e" 或 "@28e3829a"
	sig := firstLine
	sig = stripThreadName(sig)
	sig = stripDeptNum(sig)
	sig = stripLambdaAddress(sig)

	// 取稳定化后的前 80 字符做 hash
	if len(sig) > 80 {
		sig = sig[:80]
	}
	return fmt.Sprintf("%x", md5.Sum([]byte(sig)))[:16]
}

var threadNameRe = regexp.MustCompile(`\[\s*[a-zA-Z0-9._-]+(?:-task\d+)?\]`)
var deptNumRe = regexp.MustCompile(`deptNum:\s*\d+`)
var lambdaRe = regexp.MustCompile(`\$\$Lambda(?:/\w+)?(?:@\w+)?`)

func stripThreadName(s string) string    { return threadNameRe.ReplaceAllString(s, "[]") }
func stripDeptNum(s string) string       { return deptNumRe.ReplaceAllString(s, "deptNum:") }
func stripLambdaAddress(s string) string { return lambdaRe.ReplaceAllString(s, "$$Lambda") }

// stripTimestampPrefix 去掉日志消息头部的 ISO 时间戳和日志级别前缀。
// 匹配格式: "2026-07-16T11:30:00.284+08:00 ERROR ..." 或 "2026-07-16 11:30:00.284 ERROR ..."
func stripTimestampPrefix(msg string) string {
	// 跳过 ISO 8601 时间戳: YYYY-MM-DDTHH:MM:SS.mmm±HH:MM 或 YYYY-MM-DD HH:MM:SS.mmm
	idx := 0
	for idx < len(msg) {
		ch := msg[idx]
		if ch == ' ' || ch == 'T' || ch == '-' || ch == ':' || ch == '.' || ch == '+' || ch == 'Z' || (ch >= '0' && ch <= '9') {
			idx++
			continue
		}
		break
	}
	if idx == 0 {
		return msg
	}
	// 跳过后面的空格
	for idx < len(msg) && msg[idx] == ' ' {
		idx++
	}
	if idx >= len(msg) {
		return msg
	}
	return msg[idx:]
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

	// 全局硬编码过滤：p6spy SQL 日志拦截器输出，非真正错误
	if strings.Contains(parsedLog.Message, "p6spy") {
		return nil
	}

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

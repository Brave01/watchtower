package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"watchtower/internal/logmonitor/parser"
)

// WebhookConfig Webhook 配置
type WebhookConfig struct {
	Platform           string   `json:"platform"` // feishu, dingtalk, wechat, custom
	URL                string   `json:"url"`
	Secret             string   `json:"secret"`  // 签名密钥
	Enabled            bool     `json:"enabled"` // 启用/停用
	MaxRetries         int      `json:"max_retries"`
	MentionType        string   `json:"mention_type"`          // "none", "all", "specific"
	MentionUsers       []string `json:"mention_users"`         // 当 MentionType 为 specific 时的用户列表
	RateLimit          int      `json:"rate_limit"`            // 每分钟最大发送条数，0 表示不限
	RateLimitPerSecond int      `json:"rate_limit_per_second"` // 每秒最大发送条数，0 表示不限
	RingBufferSize     int      `json:"ring_buffer_size"`      // 内存缓冲区大小，默认 10000
	Template           string   `json:"template"`              // 告警消息模板
}

// RateLimitStats 限流统计
type RateLimitStats struct {
	LimitPerMinute  int     `json:"limit_per_minute"`
	RemainingMinute float64 `json:"remaining_minute"`
	LimitPerSecond  int     `json:"limit_per_second"`
	RemainingSecond float64 `json:"remaining_second"`
	TotalSent       int64   `json:"total_sent"`
	TotalLimited    int64   `json:"total_limited"`
	BufferSize      int     `json:"buffer_size"`
	BufferUsed      int     `json:"buffer_used"`
}

// LimitedAlertEntry 被限流缓存的告警记录
type LimitedAlertEntry struct {
	RuleName  string    `json:"rule_name"`
	Message   string    `json:"message"`
	Level     string    `json:"level"`
	Source    string    `json:"source"`
	Timestamp string    `json:"timestamp"`
	LimitedAt time.Time `json:"limited_at"`
	Summary   string    `json:"summary"`
}

const defaultRingBufferSize = 10000

// Client Webhook 客户端
type Client struct {
	mu           sync.RWMutex
	webhookURL   string
	platform     string // feishu, dingtalk, wechat, custom
	enabled      bool
	maxRetries   int
	mentionType  string
	mentionUsers []string
	client       *http.Client

	// 令牌桶限流器 - 每分钟
	rateLimit  int // 每分钟最大条数
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time

	// 令牌桶限流器 - 每秒
	rateLimitPerSec  int // 每秒最大条数
	tokensPerSec     float64
	maxTokensPerSec  float64
	refillRatePerSec float64
	lastRefillPerSec time.Time

	totalSent    int64
	totalLimited int64

	// 被限流日志环形缓冲区（默认 10000 条）
	limitedRing     []*LimitedAlertEntry
	ringSize        int
	ringHead        int
	ringCount       int
	overflowHandler func(*LimitedAlertEntry) // 缓冲区满时溢出回调（写入数据库）
}

// NewClient 创建 Webhook 客户端
func NewClient(cfg *WebhookConfig) *Client {
	size := defaultRingBufferSize
	if cfg != nil && cfg.RingBufferSize > 0 {
		size = cfg.RingBufferSize
	}
	c := &Client{
		maxRetries: 3,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastRefill:  time.Now(),
		ringSize:    size,
		limitedRing: make([]*LimitedAlertEntry, size),
	}
	if cfg != nil {
		c.webhookURL = cfg.URL
		c.platform = cfg.Platform
		c.enabled = cfg.Enabled
		if cfg.MaxRetries > 0 {
			c.maxRetries = cfg.MaxRetries
		}
		c.mentionType = cfg.MentionType
		c.mentionUsers = cfg.MentionUsers
		c.setRateLimit(cfg.RateLimit)
		c.setRateLimitPerSec(cfg.RateLimitPerSecond)
	}
	return c
}

// UpdateConfig 更新配置
func (c *Client) UpdateConfig(cfg *WebhookConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.webhookURL = cfg.URL
	c.platform = cfg.Platform
	c.enabled = cfg.Enabled
	if cfg.MaxRetries > 0 {
		c.maxRetries = cfg.MaxRetries
	}
	c.mentionType = cfg.MentionType
	c.mentionUsers = cfg.MentionUsers
	c.setRateLimit(cfg.RateLimit)
	c.setRateLimitPerSec(cfg.RateLimitPerSecond)
	// 支持动态调整缓冲区大小
	if cfg.RingBufferSize > 0 && cfg.RingBufferSize != c.ringSize {
		c.resizeRing(cfg.RingBufferSize)
	}
}

// SetOverflowHandler 设置缓冲区溢出回调
func (c *Client) SetOverflowHandler(fn func(*LimitedAlertEntry)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.overflowHandler = fn
}

// GetConfig 获取配置
func (c *Client) GetConfig() *WebhookConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &WebhookConfig{
		Platform:           c.platform,
		URL:                c.webhookURL,
		Enabled:            c.enabled,
		MaxRetries:         c.maxRetries,
		MentionType:        c.mentionType,
		MentionUsers:       c.mentionUsers,
		RateLimit:          c.rateLimit,
		RateLimitPerSecond: c.rateLimitPerSec,
		RingBufferSize:     c.ringSize,
	}
}

// GetRateLimitStats 获取限流统计
func (c *Client) GetRateLimitStats() *RateLimitStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &RateLimitStats{
		LimitPerMinute:  c.rateLimit,
		RemainingMinute: c.tokens,
		LimitPerSecond:  c.rateLimitPerSec,
		RemainingSecond: c.tokensPerSec,
		TotalSent:       c.totalSent,
		TotalLimited:    c.totalLimited,
		BufferSize:      c.ringSize,
		BufferUsed:      c.ringCount,
	}
}

// setRateLimit 设置限流参数（调用方需持有锁）
func (c *Client) setRateLimit(limit int) {
	c.rateLimit = limit
	if limit > 0 {
		c.maxTokens = float64(limit)
		c.tokens = float64(limit)
		c.refillRate = float64(limit) / 60.0
	} else {
		c.maxTokens = 0
		c.tokens = 0
		c.refillRate = 0
	}
	c.lastRefill = time.Now()
}

// setRateLimitPerSec 设置每秒限流参数（调用方需持有锁）
func (c *Client) setRateLimitPerSec(limit int) {
	c.rateLimitPerSec = limit
	if limit > 0 {
		c.maxTokensPerSec = float64(limit)
		c.tokensPerSec = float64(limit)
		c.refillRatePerSec = float64(limit)
	} else {
		c.maxTokensPerSec = 0
		c.tokensPerSec = 0
		c.refillRatePerSec = 0
	}
	c.lastRefillPerSec = time.Now()
}

// allow 检查是否允许发送（调用方需持有写锁）
func (c *Client) allow() bool {
	now := time.Now()

	// 第1步：先补充令牌（基于时间流逝）
	if c.rateLimit > 0 {
		elapsed := now.Sub(c.lastRefill).Seconds()
		c.tokens = math.Min(c.maxTokens, c.tokens+elapsed*c.refillRate)
		c.lastRefill = now
	}
	if c.rateLimitPerSec > 0 {
		elapsed := now.Sub(c.lastRefillPerSec).Seconds()
		c.tokensPerSec = math.Min(c.maxTokensPerSec, c.tokensPerSec+elapsed*c.refillRatePerSec)
		c.lastRefillPerSec = now
	}

	// 第2步：检查双桶是否都有足够令牌
	minuteOK := c.rateLimit <= 0 || c.tokens >= 1
	secondOK := c.rateLimitPerSec <= 0 || c.tokensPerSec >= 1

	if minuteOK && secondOK {
		// 第3步：双桶都通过才消耗令牌
		if c.rateLimit > 0 {
			c.tokens--
		}
		if c.rateLimitPerSec > 0 {
			c.tokensPerSec--
		}
		return true
	}

	return false
}

// pushLimitedAlert 将限流日志加入环形缓冲区
func (c *Client) pushLimitedAlert(entry *LimitedAlertEntry) {
	if c.ringCount >= c.ringSize {
		// 缓冲区已满，溢出最旧的记录到回调（写入数据库）
		oldest := c.limitedRing[c.ringHead]
		if oldest != nil && c.overflowHandler != nil {
			c.overflowHandler(oldest)
		}
	}
	c.limitedRing[c.ringHead] = entry
	c.ringHead = (c.ringHead + 1) % c.ringSize
	if c.ringCount < c.ringSize {
		c.ringCount++
	}
}

// resizeRing 动态调整环形缓冲区大小（调用方需持有写锁）
func (c *Client) resizeRing(newSize int) {
	newRing := make([]*LimitedAlertEntry, newSize)
	// 复制现有数据
	count := 0
	if c.ringCount > 0 {
		start := (c.ringHead - c.ringCount + c.ringSize) % c.ringSize
		for i := 0; i < c.ringCount && count < newSize; i++ {
			idx := (start + i) % c.ringSize
			if c.limitedRing[idx] != nil {
				newRing[count] = c.limitedRing[idx]
				count++
			}
		}
	}
	c.limitedRing = newRing
	c.ringSize = newSize
	c.ringHead = count % newSize
	c.ringCount = count
}

// GetLimitedAlerts 获取被限流的日志列表（按时间倒序）
func (c *Client) GetLimitedAlerts() []*LimitedAlertEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.ringCount == 0 {
		return nil
	}

	result := make([]*LimitedAlertEntry, 0, c.ringCount)
	// 从最旧的开始遍历
	start := (c.ringHead - c.ringCount + c.ringSize) % c.ringSize
	for i := 0; i < c.ringCount; i++ {
		idx := (start + i) % c.ringSize
		if c.limitedRing[idx] != nil {
			result = append(result, c.limitedRing[idx])
		}
	}
	return result
}

// ClearLimitedAlerts 清空限流日志缓冲区
func (c *Client) ClearLimitedAlerts() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limitedRing = make([]*LimitedAlertEntry, c.ringSize)
	c.ringHead = 0
	c.ringCount = 0
}

// popOldestLocked 从环形缓冲区取出最旧的条目（调用方需持有写锁）
func (c *Client) popOldestLocked() *LimitedAlertEntry {
	if c.ringCount == 0 {
		return nil
	}
	oldestIdx := (c.ringHead - c.ringCount + c.ringSize) % c.ringSize
	entry := c.limitedRing[oldestIdx]
	c.limitedRing[oldestIdx] = nil
	c.ringCount--
	return entry
}

// StartDrainLoop 启动后台循环，被限流的日志按限流速率逐条重发
// onSent 回调在成功重发后被调用（用于统计等目的）
func (c *Client) StartDrainLoop(ctx context.Context, dbLoader func() []*LimitedAlertEntry, onSent func(ruleName string)) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.drainOne(dbLoader, onSent)
			}
		}
	}()
	log.Println("[限流重试] 后台重试循环已启动（每10秒检查一次）")
}

// drainOne 尝试重发一条被限流的日志
func (c *Client) drainOne(dbLoader func() []*LimitedAlertEntry, onSent func(ruleName string)) {
	c.mu.Lock()

	// 缓冲区为空时尝试从 DB 加载
	if c.ringCount == 0 && dbLoader != nil {
		entries := dbLoader()
		for _, e := range entries {
			c.pushLimitedAlert(e)
		}
	}

	// 检查限流是否允许发送
	if !c.allow() {
		c.mu.Unlock()
		return
	}

	// 弹出最旧的条目
	entry := c.popOldestLocked()
	url := c.webhookURL
	c.mu.Unlock()

	if entry == nil || url == "" {
		return
	}

	// 构建消息并发送（使用 entry 中保存的 template/summary）
	mockLog := &parser.ParsedLog{
		Message:   entry.Message,
		Level:     entry.Level,
		Source:    entry.Source,
		Timestamp: entry.Timestamp,
	}
	message := c.buildMessage(entry.RuleName, mockLog, entry.Summary)
	err := c.send(url, message)
	if err == nil {
		c.mu.Lock()
		c.totalSent++
		c.mu.Unlock()
		log.Printf("[限流重试] 重发成功: %s", entry.RuleName)
		if onSent != nil {
			onSent(entry.RuleName)
		}
	} else {
		log.Printf("[限流重试] 重发失败: %v，放回缓冲区", err)
		// 非限流原因的失败（网络等），放回缓冲区等待下次重试
		c.mu.Lock()
		c.pushLimitedAlert(entry)
		c.mu.Unlock()
	}
}

// FeishuMessage 飞书消息结构
type FeishuMessage struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Post struct {
			ZhCN struct {
				Title   string            `json:"title"`
				Content [][]FeishuPostTag `json:"content"`
			} `json:"zh_cn"`
		} `json:"post"`
	} `json:"content"`
}

// FeishuPostTag 飞书富文本标签
type FeishuPostTag struct {
	Tag    string `json:"tag"`
	Text   string `json:"text,omitempty"`
	Href   string `json:"href,omitempty"`
	UserID string `json:"user_id,omitempty"`
}

// SendAlert 发送告警
func (c *Client) SendAlert(ruleName string, parsedLog *parser.ParsedLog, template string) error {
	c.mu.Lock()
	url := c.webhookURL
	enabled := c.enabled
	if !enabled {
		c.mu.Unlock()
		log.Printf("[Webhook] 告警发送失败: rule=%s, 原因=Webhook 已停用（enabled=false）", ruleName)
		return fmt.Errorf("Webhook 已停用（enabled=false）")
	}
	limited := !c.allow()
	if limited {
		c.totalLimited++
		// 保存到限流缓冲区
		c.pushLimitedAlert(&LimitedAlertEntry{
			RuleName:  ruleName,
			Message:   parsedLog.Message,
			Level:     parsedLog.Level,
			Source:    parsedLog.Source,
			Timestamp: parsedLog.Timestamp,
			LimitedAt: time.Now(),
			Summary:   template,
		})
		log.Printf("[Webhook] 告警发送失败: rule=%s, 原因=限流(令牌用尽, rateLimit=%d/s, rateLimitSec=%d/s, 已限流%d次)",
			ruleName, c.rateLimit, c.rateLimitPerSec, c.totalLimited)
		c.mu.Unlock()
		return fmt.Errorf("限流: 每分钟最多 %d 条 / 每秒最多 %d 条，令牌已用尽", c.rateLimit, c.rateLimitPerSec)
	}
	c.mu.Unlock()

	if url == "" {
		log.Printf("[Webhook] 告警发送失败: rule=%s, 原因=Webhook URL 未配置", ruleName)
		return fmt.Errorf("飞书 Webhook URL 未配置")
	}

	message := c.buildMessage(ruleName, parsedLog, template)
	err := c.sendWithRetry(message, ruleName)
	if err == nil {
		c.mu.Lock()
		c.totalSent++
		c.mu.Unlock()
	}
	return err
}

// SendSimpleMessage 发送简单文本消息（用于心跳等轻量通知）
func (c *Client) SendSimpleMessage(text string) error {
	c.mu.Lock()
	url := c.webhookURL
	limited := !c.allow()
	if limited {
		c.totalLimited++
		c.mu.Unlock()
		return nil // 心跳被限流静默忽略
	}
	c.mu.Unlock()

	if url == "" {
		return fmt.Errorf("飞书 Webhook URL 未配置")
	}

	msg := &FeishuMessage{
		MsgType: "post",
	}
	msg.Content.Post.ZhCN.Title = "⏳ 告警进展"

	// 添加 @提及
	c.mu.RLock()
	mentionType := c.mentionType
	mentionUsers := make([]string, len(c.mentionUsers))
	copy(mentionUsers, c.mentionUsers)
	c.mu.RUnlock()

	contentLine := []FeishuPostTag{
		{Tag: "text", Text: text},
	}

	if mentionType == "all" {
		contentLine = append(contentLine, FeishuPostTag{Tag: "text", Text: "\n"})
		contentLine = append(contentLine, FeishuPostTag{Tag: "at", UserID: "all"})
	} else if mentionType == "specific" && len(mentionUsers) > 0 {
		contentLine = append(contentLine, FeishuPostTag{Tag: "text", Text: "\n"})
		for _, uid := range mentionUsers {
			uid = strings.TrimSpace(uid)
			if uid != "" {
				contentLine = append(contentLine, FeishuPostTag{Tag: "at", UserID: uid})
			}
		}
	}

	msg.Content.Post.ZhCN.Content = [][]FeishuPostTag{
		contentLine,
		{
			{Tag: "text", Text: fmt.Sprintf("---\n⏰ %s", time.Now().Format("2006-01-02 15:04:05"))},
		},
	}

	return c.sendWithRetry(msg, "heartbeat")
}

func (c *Client) buildMessage(ruleName string, parsedLog *parser.ParsedLog, template string) *FeishuMessage {
	// 替换模板变量
	content := template
	if content == "" {
		content = "🚨 告警: {rule_name}\n级别: {level}\n来源: {source}\n时间: {timestamp}\n消息: {message}"
	}
	content = strings.ReplaceAll(content, "{rule_name}", ruleName)
	content = strings.ReplaceAll(content, "{timestamp}", parsedLog.Timestamp)
	// 截断消息内容，避免超过飞书 30KB 限制
	msgText := parsedLog.Message
	if len(msgText) > 2000 {
		msgText = msgText[:2000] + "\n...(truncated)"
	}
	content = strings.ReplaceAll(content, "{message}", msgText)
	content = strings.ReplaceAll(content, "{level}", parsedLog.Level)
	content = strings.ReplaceAll(content, "{source}", parsedLog.Source)
	// 整体内容截断（防止模板 + 其他字段合计超限）
	if len(content) > 10000 {
		content = content[:10000] + "\n...(truncated)"
	}

	msg := &FeishuMessage{
		MsgType: "post",
	}
	msg.Content.Post.ZhCN.Title = fmt.Sprintf("🚨 告警: %s", ruleName)

	// 构建消息内容
	contentLine := []FeishuPostTag{
		{Tag: "text", Text: content},
	}

	// 添加 @提及
	c.mu.RLock()
	mentionType := c.mentionType
	mentionUsers := make([]string, len(c.mentionUsers))
	copy(mentionUsers, c.mentionUsers)
	c.mu.RUnlock()

	if mentionType == "all" {
		contentLine = append(contentLine, FeishuPostTag{Tag: "text", Text: "\n"})
		contentLine = append(contentLine, FeishuPostTag{Tag: "at", UserID: "all"})
	} else if mentionType == "specific" && len(mentionUsers) > 0 {
		contentLine = append(contentLine, FeishuPostTag{Tag: "text", Text: "\n"})
		for _, uid := range mentionUsers {
			uid = strings.TrimSpace(uid)
			if uid != "" {
				contentLine = append(contentLine, FeishuPostTag{Tag: "at", UserID: uid})
			}
		}
	}

	msg.Content.Post.ZhCN.Content = [][]FeishuPostTag{
		contentLine,
		{
			{Tag: "text", Text: fmt.Sprintf("\n---\n⏰ %s", time.Now().Format("2006-01-02 15:04:05"))},
		},
	}

	return msg
}

func (c *Client) sendWithRetry(msg *FeishuMessage, ruleName string) error {
	var lastErr error

	c.mu.RLock()
	maxRetries := c.maxRetries
	url := c.webhookURL
	c.mu.RUnlock()

	for i := 0; i <= maxRetries; i++ {
		if err := c.send(url, msg); err != nil {
			lastErr = err
			log.Printf("[Webhook] 告警发送失败: rule=%s, 原因=%s (第%d次重试)", ruleName, err, i+1)
			// 飞书频率限制错误（code=11232 或 9499）-> 等更长时间再重试
			if strings.Contains(err.Error(), "code=11232") || strings.Contains(err.Error(), "code=9499") ||
				strings.Contains(err.Error(), "frequency limited") ||
				strings.Contains(err.Error(), "too many request") {
				time.Sleep(15 * time.Second)
			} else {
				time.Sleep(time.Duration(1<<i) * time.Second)
			}
			continue
		}
		// 发送成功日志
		c.mu.RLock()
		log.Printf("[Webhook] 告警发送成功: rule=%s, 令牌剩余: minute=%.1f/s, second=%.1f/s",
			ruleName, c.tokens, c.tokensPerSec)
		c.mu.RUnlock()
		return nil
	}

	log.Printf("[Webhook] 告警发送最终失败: rule=%s, 原因=%s", ruleName, lastErr)
	return fmt.Errorf("发送飞书消息失败(已重试%d次): %v", maxRetries, lastErr)
}

func (c *Client) send(url string, msg *FeishuMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化消息失败: %w", err)
	}

	resp, err := c.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		// 尝试解析飞书 JSON 错误码
		var feishuErr struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if json.Unmarshal(body, &feishuErr) == nil && feishuErr.Code != 0 {
			return fmt.Errorf("HTTP %d: code=%d, msg=%s", resp.StatusCode, feishuErr.Code, feishuErr.Msg)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(body, &result); err == nil && result.Code != 0 {
		return fmt.Errorf("飞书返回错误: code=%d, msg=%s", result.Code, result.Msg)
	}

	return nil
}

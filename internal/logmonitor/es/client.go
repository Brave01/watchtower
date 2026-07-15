package es

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

// LogEntry ES 日志条目
type LogEntry struct {
	Index     string          `json:"_index"`
	ID        string          `json:"_id"`
	Score     float64         `json:"_score"`
	Timestamp time.Time       `json:"@timestamp"`
	Source    json.RawMessage `json:"_source"`
	RawJSON   string          `json:"-"`
}

// Client ES 客户端
type Client struct {
	es       *elasticsearch.Client
	address  string
	username string
	password string
	index    string
	interval int
	size     int
	query    map[string]interface{}

	mu      sync.RWMutex
	running bool
	stopCh  chan struct{}
}

// NewClient 创建 ES 客户端
func NewClient(address, username, password, index string, interval, size int, query map[string]interface{}) (*Client, error) {
	cfg := elasticsearch.Config{
		Addresses: []string{address},
		Username:  username,
		Password:  password,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
			ResponseHeaderTimeout: 10 * time.Second,
		},
	}

	esClient, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 ES 客户端失败: %w", err)
	}

	c := &Client{
		es:       esClient,
		address:  address,
		username: username,
		password: password,
		index:    index,
		interval: interval,
		size:     size,
		query:    query,
		stopCh:   make(chan struct{}),
	}

	return c, nil
}

// TestQuery 发送一条测试查询，验证 ES 是否可连接且索引可访问
func (c *Client) TestQuery(ctx context.Context) error {
	// 用 size=1 的匹配查询验证索引和连接
	queryBody := map[string]interface{}{
		"query": map[string]interface{}{
			"match_all": map[string]interface{}{},
		},
	}
	body, err := json.Marshal(queryBody)
	if err != nil {
		return fmt.Errorf("构建测试查询失败: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(c.index),
		c.es.Search.WithBody(bytes.NewReader(body)),
		c.es.Search.WithSize(1),
		c.es.Search.WithSort("@timestamp:desc"),
	)
	if err != nil {
		return fmt.Errorf("ES 连接失败: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("ES 查询返回错误: %s", res.String())
	}

	return nil
}

// Search 执行搜索
func (c *Client) Search(ctx context.Context) ([]LogEntry, error) {
	// 构建查询体，替换 {interval} 占位符
	queryBody := c.buildQueryBody()

	body, err := json.Marshal(queryBody)
	if err != nil {
		return nil, fmt.Errorf("序列化查询体失败: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(c.index),
		c.es.Search.WithBody(bytes.NewReader(body)),
		c.es.Search.WithSize(c.size),
		c.es.Search.WithSort("@timestamp:desc"),
	)
	if err != nil {
		return nil, fmt.Errorf("ES 搜索失败: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("ES 搜索返回错误: %s", res.String())
	}

	var result struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Index  string          `json:"_index"`
				ID     string          `json:"_id"`
				Score  float64         `json:"_score"`
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析 ES 响应失败: %w", err)
	}

	entries := make([]LogEntry, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		entry := LogEntry{
			Index:   hit.Index,
			ID:      hit.ID,
			Score:   hit.Score,
			Source:  hit.Source,
			RawJSON: string(hit.Source),
		}

		// 尝试解析 @timestamp
		var source struct {
			Timestamp string `json:"@timestamp"`
		}
		if err := json.Unmarshal(hit.Source, &source); err == nil && source.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, source.Timestamp); err == nil {
				entry.Timestamp = t
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func (c *Client) buildQueryBody() map[string]interface{} {
	body := deepCopyMap(c.query)
	setInterval(body, c.interval)
	return body
}

func setInterval(query map[string]interface{}, interval int) {
	intervalStr := fmt.Sprintf("%d", interval)

	// 遍历替换所有 gte 字段中的 {interval} 占位符
	walkAndReplace(query, func(v string) string {
		return strings.ReplaceAll(v, "{interval}", intervalStr)
	})
}

func walkAndReplace(data interface{}, replacer func(string) string) {
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			switch val2 := val.(type) {
			case string:
				v[k] = replacer(val2)
			case map[string]interface{}:
				walkAndReplace(val2, replacer)
			case []interface{}:
				for i, item := range val2 {
					if s, ok := item.(string); ok {
						val2[i] = replacer(s)
					} else {
						walkAndReplace(item, replacer)
					}
				}
			}
		}
	case []interface{}:
		for i, item := range v {
			if s, ok := item.(string); ok {
				v[i] = replacer(s)
			} else {
				walkAndReplace(item, replacer)
			}
		}
	}
}

func deepCopyMap(m map[string]interface{}) map[string]interface{} {
	data, _ := json.Marshal(m)
	var copy map[string]interface{}
	json.Unmarshal(data, &copy)
	return copy
}

// Start 开始定时查询
func (c *Client) Start(ctx context.Context, handler func([]LogEntry)) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	ticker := time.NewTicker(time.Duration(c.interval) * time.Second)
	defer ticker.Stop()

	// 立即执行一次
	c.executeQuery(ctx, handler)

	for {
		select {
		case <-ticker.C:
			c.executeQuery(ctx, handler)
		case <-c.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Client) executeQuery(ctx context.Context, handler func([]LogEntry)) {
	entries, err := c.Search(ctx)
	if err != nil {
		log.Printf("[ERROR] [ES] 查询失败: %v", err)
		return
	}

	if len(entries) > 0 {
		log.Printf("[INFO] [ES] 查询到 %d 条日志", len(entries))
		handler(entries)
	}
}

// Stop 停止定时查询
func (c *Client) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		close(c.stopCh)
		c.running = false
	}
}

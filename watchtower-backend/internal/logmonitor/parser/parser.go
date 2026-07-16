package parser

import (
	"encoding/json"
	"fmt"
	"strings"
)

// K8sFields Kubernetes 元信息
type K8sFields struct {
	Namespace     string `json:"namespace,omitempty"`
	PodName       string `json:"pod_name,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	ContainerImage string `json:"container_image,omitempty"`
	NodeName      string `json:"node_name,omitempty"`
	Host          string `json:"host,omitempty"`
}

// ParsedLog 解析后的日志结构
type ParsedLog struct {
	Raw       string                 `json:"raw"`
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Source    string                 `json:"source"`
	K8s       *K8sFields            `json:"k8s,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// Parser 日志解析器
type Parser struct {
	fieldMappings map[string]string
}

// NewParser 创建解析器
func NewParser() *Parser {
	return &Parser{
		fieldMappings: map[string]string{
			"log_level":  "level",
			"severity":   "level",
			"level":      "level",
			"stream":     "level",
			"message":    "message",
			"msg":        "message",
			"log":        "message",
			"content":    "message",
			"@timestamp": "timestamp",
			"timestamp":  "timestamp",
			"time":       "timestamp",
			"host":       "source",
			"hostname":   "source",
			"server":     "source",
			"service":    "source",
		},
	}
}

// Parse 解析 ES 日志条目
func (p *Parser) Parse(rawJSON string) (*ParsedLog, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %w", err)
	}

	log := &ParsedLog{
		Raw:    rawJSON,
		Fields: data,
	}

	// 提取字段
	for jsonField, logField := range p.fieldMappings {
		if val, ok := data[jsonField]; ok {
			strVal := fmt.Sprintf("%v", val)
			switch logField {
			case "level":
				if log.Level == "" {
					log.Level = strVal
				}
			case "message":
				if log.Message == "" {
					log.Message = strVal
				}
			case "timestamp":
				if log.Timestamp == "" {
					log.Timestamp = strVal
				}
			case "source":
				if log.Source == "" {
					log.Source = strVal
				}
			}
		}
	}

	// 如果 message 为空，尝试提取整个日志的 message 字段
	if log.Message == "" {
		if msg, ok := extractMessage(data); ok {
			log.Message = msg
		}
	}

	// 从日志消息内容中提取级别（k8s 日志格式: "2026-07-02 09:02:15.002  INFO ..."）
	if log.Level == "" || log.Level == "stdout" || log.Level == "stderr" {
		if level := extractLevelFromMessage(log.Message); level != "" {
			log.Level = level
		}
	}

	// 从 kubernetes 信息提取 source
	if log.Source == "" {
		if kube, ok := data["kubernetes"].(map[string]interface{}); ok {
			if ns, ok := kube["namespace_name"].(string); ok {
				log.Source = ns
			}
			if pod, ok := kube["pod_name"].(string); ok {
				if log.Source != "" {
					log.Source = log.Source + "/" + pod
				} else {
					log.Source = pod
				}
			}
		}
	}

	// 提取 Kubernetes 元信息
	if kube, ok := data["kubernetes"].(map[string]interface{}); ok {
		k8s := &K8sFields{}
		if v, ok := kube["namespace_name"].(string); ok {
			k8s.Namespace = v
		}
		if v, ok := kube["pod_name"].(string); ok {
			k8s.PodName = v
		}
		if v, ok := kube["container_name"].(string); ok {
			k8s.ContainerName = v
		}
		if v, ok := kube["container_image"].(string); ok {
			k8s.ContainerImage = v
		}
		if v, ok := kube["node_name"].(string); ok {
			k8s.NodeName = v
		}
		if v, ok := kube["host"].(string); ok {
			k8s.Host = v
		}
		if k8s.PodName != "" || k8s.ContainerName != "" {
			log.K8s = k8s
		}
	}

	return log, nil
}

// 常见日志级别
var levelPatterns = []struct {
	keyword string
	level   string
}{
	{"FATAL", "fatal"},
	{"ERROR", "error"},
	{"WARN", "warn"},
	{"INFO", "info"},
	{"DEBUG", "debug"},
	{"TRACE", "trace"},
}

// extractLevelFromMessage 从日志消息中提取级别
func extractLevelFromMessage(msg string) string {
	for _, p := range levelPatterns {
		if strings.Contains(msg, p.keyword) {
			return p.level
		}
	}
	return ""
}

// extractMessage 递归提取 message 字段
func extractMessage(data map[string]interface{}) (string, bool) {
	// 尝试常见字段
	for _, key := range []string{"message", "msg", "log", "content"} {
		if val, ok := data[key]; ok {
			if s, ok := val.(string); ok {
				return s, true
			}
		}
	}

	// 尝试从嵌套字段中提取
	for _, val := range data {
		if nested, ok := val.(map[string]interface{}); ok {
			if msg, found := extractMessage(nested); found {
				return msg, found
			}
		}
	}

	return "", false
}

// MatchKeywords 检查日志是否匹配关键词
func (p *Parser) MatchKeywords(log *ParsedLog, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(log.Message, keyword) {
			return true
		}
	}
	return false
}

// MatchRegex 检查日志是否匹配正则表达式
func (p *Parser) MatchRegex(log *ParsedLog, pattern string) bool {
	return strings.Contains(log.Message, pattern)
}

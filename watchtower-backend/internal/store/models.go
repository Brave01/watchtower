package store

import (
	"encoding/json"
	"time"
)

type AlertRule struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	Keywords        string `json:"keywords"`
	ExcludeKeywords string `json:"exclude_keywords"`
	Level           string `json:"level"`
	RegexPattern    string `json:"regex_pattern"`
	Cooldown        int    `json:"cooldown"`
	MessageTemplate string `json:"message_template"`
	WebhookID       int    `json:"webhook_id"` // 关联的 webhook ID，0 表示使用默认
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}
type Host struct {
	ID            string    `json:"id"`
	IP            string    `json:"ip"`
	Hostname      string    `json:"hostname"`
	Project       string    `json:"project"`
	CPU           string    `json:"cpu"`
	Memory        string    `json:"memory"`
	Disk          string    `json:"disk"`
	Status        int       `json:"status"`
	Maintenance   bool      `json:"maintenance"`
	LastCheckTime time.Time `json:"last_check_time"`
}

type Role struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Timeout int    `json:"timeout"`
}

type Assignment struct {
	HostID              string    `json:"host_id"`
	RoleID              string    `json:"role_id"`
	Role                *Role     `json:"role,omitempty"`
	Status              int       `json:"status"`
	StatusCode          int       `json:"status_code"`
	LastCheckTime       time.Time `json:"last_check_time"`
	ErrorMessage        string    `json:"error_message,omitempty"`
	OverridePort        *int      `json:"override_port,omitempty"`
	OverridePath        string    `json:"override_path,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
}

type SSHCredential struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Username   string `json:"username"`
	AuthMethod string `json:"auth_method"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
}
type User struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type DashboardData struct {
	Hosts []HostWithRoles `json:"hosts"`
}
type HostWithRoles struct {
	Host    Host         `json:"host"`
	Roles   []Assignment `json:"roles"`
	IsAlive bool         `json:"is_alive"`
}
type ProbeResult struct {
	Status     int
	StatusCode int
	Error      string
}

type ESConfig struct {
	ID       int    `json:"id"`
	Address  string `json:"address"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
	Index    string `json:"index"`
	Interval int    `json:"interval"`
	Size     int    `json:"size"`
	Query    string `json:"query"`
	Enabled  bool   `json:"enabled"`
}

type WebhookConfig struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Platform           string `json:"platform"` // feishu, dingtalk, wechat, custom
	URL                string `json:"url"`
	Secret             string `json:"secret"`  // webhook secret/signature
	Enabled            bool   `json:"enabled"` // 启用/停用
	MaxRetries         int    `json:"max_retries"`
	MentionType        string `json:"mention_type"`          // none, all, specific
	MentionUsers       string `json:"mention_users"`         // @指定人 ID，逗号分隔
	RateLimit          int    `json:"rate_limit"`            // 每分钟限流次数（0=不限制）
	RateLimitPerSecond int    `json:"rate_limit_per_second"` // 每秒限流次数（0=不限制）
	RingBufferSize     int    `json:"ring_buffer_size"`
	Template           string `json:"template"` // 告警消息模板
}

type LimitedAlert struct {
	ID        int    `json:"id"`
	RuleName  string `json:"rule_name"`
	Message   string `json:"message"`
	Level     string `json:"level"`
	Source    string `json:"source"`
	Timestamp string `json:"timestamp"`
	LimitedAt string `json:"limited_at"`
	Summary   string `json:"summary"`
}

const (
	HostStatusUnknown = 0
	HostStatusUp      = 1
	HostStatusDown    = 2
	HostStatusWarning = 3
	HostStatusMuted   = 4
)
const (
	ProbeTypeICMP = "ICMP"
	ProbeTypeTCP  = "TCP"
	ProbeTypeHTTP = "HTTP"
	ProbeTypeSSH  = "SSH"
)
const RoleIDICMP = "role-icmp"

func JoinStrings(items []string) string {
	data, _ := json.Marshal(items)
	return string(data)
}
func SplitStrings(s string) []string {
	var items []string
	if s == "" {
		return items
	}
	json.Unmarshal([]byte(s), &items)
	return items
}
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func IntToBool(i int) bool {
	return i != 0
}

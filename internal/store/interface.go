package store

import "time"

type Store interface {
	// 告警规则
	ListAlertRules() ([]AlertRule, error)
	GetAlertRule(id string) (*AlertRule, error)
	SaveAlertRule(r *AlertRule) error
	DeleteAlertRule(id string) error

	// Webhook 配置
	ListWebhookConfigs() ([]WebhookConfig, error)
	GetWebhookConfig(id int) (*WebhookConfig, error)
	SaveWebhookConfig(c *WebhookConfig) error
	DeleteWebhookConfig(id int) error

	// 限流日志
	SaveLimitedAlert(a *LimitedAlert) error
	ListLimitedAlerts(limit, offset int) ([]LimitedAlert, error)
	CountLimitedAlerts() (int, error)
	LoadLimitedAlertsForRetry(limit int) ([]LimitedAlert, error)
	ClearLimitedAlerts() error
	DeleteOldLimitedAlerts(before time.Time) (int64, error)

	// 主机
	ListHosts() ([]Host, error)
	GetHost(id string) (*Host, error)
	AddHost(h *Host) error
	UpdateHost(h *Host) error
	UpdateHostStatus(id string, status int, checkTime time.Time) error
	UpdateHostMaintenance(id string, maintenance bool) error
	DeleteHost(id string) error

	// 角色
	ListRoles() ([]Role, error)
	GetRole(id string) (*Role, error)
	AddRole(r *Role) error
	DeleteRole(id string) error

	// 指派
	ListAssignments() ([]Assignment, error)
	GetAssignment(hostID, roleID string) (*Assignment, error)
	AddAssignment(a *Assignment) error
	DeleteAssignment(hostID, roleID string) error
	UpdateAssignmentStatus(hostID, roleID string, status, statusCode int, errMsg string, checkTime time.Time) error
	UpdateAssignmentConsecutiveFailures(hostID, roleID string, failures int) error

	// SSH 凭据
	ListSSHCredentials() ([]SSHCredential, error)
	GetSSHCredential(id string) (*SSHCredential, error)
	AddSSHCredential(c *SSHCredential) (string, error)
	DeleteSSHCredential(id string) error

	// ES 配置
	GetESConfig() (*ESConfig, error)
	SaveESConfig(c *ESConfig) error

	// 用户
	GetUser(username string) (*User, error)

	Close() error
}

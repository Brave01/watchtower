package store

import (
	"time"
	"watchtower/internal/model"
)

type Store interface {
	// 告警规则
	ListAlertRules() ([]model.AlertRule, error)
	GetAlertRule(id string) (*model.AlertRule, error)
	SaveAlertRule(r *model.AlertRule) error
	DeleteAlertRule(id string) error

	// Webhook 配置
	ListWebhookConfigs() ([]model.WebhookConfig, error)
	GetWebhookConfig(id int) (*model.WebhookConfig, error)
	SaveWebhookConfig(c *model.WebhookConfig) error
	DeleteWebhookConfig(id int) error

	// 限流日志
	SaveLimitedAlert(a *model.LimitedAlert) error
	ListLimitedAlerts(limit, offset int) ([]model.LimitedAlert, error)
	CountLimitedAlerts() (int, error)
	LoadLimitedAlertsForRetry(limit int) ([]model.LimitedAlert, error)
	ClearLimitedAlerts() error
	DeleteOldLimitedAlerts(before time.Time) (int64, error)

	// 主机
	ListHosts() ([]model.Host, error)
	GetHost(id string) (*model.Host, error)
	AddHost(h *model.Host) error
	UpdateHost(h *model.Host) error
	UpdateHostStatus(id string, status int, checkTime time.Time) error
	UpdateHostMaintenance(id string, maintenance bool) error
	DeleteHost(id string) error

	// 角色
	ListRoles() ([]model.Role, error)
	GetRole(id string) (*model.Role, error)
	AddRole(r *model.Role) error
	UpdateRole(r *model.Role) error
	DeleteRole(id string) error

	// 指派
	ListAssignments() ([]model.Assignment, error)
	GetAssignment(hostID, roleID string) (*model.Assignment, error)
	AddAssignment(a *model.Assignment) error
	DeleteAssignment(hostID, roleID string) error
	UpdateAssignmentStatus(hostID, roleID string, status, statusCode int, errMsg string, checkTime time.Time) error
	UpdateAssignmentConsecutiveFailures(hostID, roleID string, failures int) error

	// SSH 凭据
	ListSSHCredentials() ([]model.SSHCredential, error)
	GetSSHCredential(id string) (*model.SSHCredential, error)
	AddSSHCredential(c *model.SSHCredential) (string, error)
	DeleteSSHCredential(id string) error

	// ES 配置
	GetESConfig() (*model.ESConfig, error)
	SaveESConfig(c *model.ESConfig) error

	// 用户
	GetUser(username string) (*model.User, error)
	SaveUser(u *model.User) error
	UpdatePassword(username, passwordHash string) error

	Close() error
}

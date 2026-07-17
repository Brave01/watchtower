package store

import (
	"database/sql"
	"fmt"
	"time"
	"watchtower/internal/model"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			keywords TEXT DEFAULT "",
			exclude_keywords TEXT DEFAULT "",
			level TEXT DEFAULT "",
			regex_pattern TEXT DEFAULT "",
			cooldown INTEGER DEFAULT 300,
			message_template TEXT DEFAULT "",
			webhook_id INTEGER DEFAULT 0,
			created_at TEXT DEFAULT "",
			updated_at TEXT DEFAULT ""
		)`,
		`CREATE TABLE IF NOT EXISTS webhook_config (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT DEFAULT "",
			platform TEXT DEFAULT "feishu",
			url TEXT DEFAULT "",
			secret TEXT DEFAULT "",
			enabled INTEGER DEFAULT 1,
			max_retries INTEGER DEFAULT 3,
			mention_type TEXT DEFAULT "none",
			mention_users TEXT DEFAULT "",
			rate_limit INTEGER DEFAULT 0,
			rate_limit_per_second INTEGER DEFAULT 0,
			ring_buffer_size INTEGER DEFAULT 10000,
			template TEXT DEFAULT ""
		)`,
		`CREATE TABLE IF NOT EXISTS limited_alert_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			rule_name TEXT NOT NULL,
			message TEXT NOT NULL,
			level TEXT DEFAULT "",
			source TEXT DEFAULT "",
			timestamp TEXT DEFAULT "",
			limited_at TEXT DEFAULT "",
			summary TEXT DEFAULT ""
		)`,
		`CREATE TABLE IF NOT EXISTS hosts (
			id TEXT PRIMARY KEY,
			ip TEXT NOT NULL,
			hostname TEXT NOT NULL,
			project TEXT DEFAULT "",
			cpu TEXT DEFAULT "",
			memory TEXT DEFAULT "",
			disk TEXT DEFAULT "",
			status INTEGER DEFAULT 0,
			maintenance INTEGER DEFAULT 0,
			ssh_credential_id TEXT DEFAULT "",
			last_check_time TEXT DEFAULT ""
		)`,
		`CREATE TABLE IF NOT EXISTS roles (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			port INTEGER DEFAULT 0,
			path TEXT DEFAULT "",
			timeout INTEGER DEFAULT 5
		)`,
		`CREATE TABLE IF NOT EXISTS assignments (
			host_id TEXT NOT NULL,
			role_id TEXT NOT NULL,
			status INTEGER DEFAULT 0,
			status_code INTEGER DEFAULT 0,
			last_check_time TEXT DEFAULT "",
			error_message TEXT DEFAULT "",
			override_port INTEGER,
			override_path TEXT DEFAULT "",
			consecutive_failures INTEGER DEFAULT 0,
			PRIMARY KEY (host_id, role_id)
		)`,
		`CREATE TABLE IF NOT EXISTS ssh_credentials (
			id TEXT PRIMARY KEY,
			label TEXT NOT NULL,
			username TEXT NOT NULL,
			auth_method TEXT DEFAULT "password",
			password TEXT DEFAULT "",
			private_key TEXT DEFAULT "",
			port INTEGER DEFAULT 22
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			username TEXT PRIMARY KEY,
			password_hash TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS es_config (
			id INTEGER PRIMARY KEY,
			address TEXT DEFAULT "",
			username TEXT DEFAULT "",
			password TEXT DEFAULT "",
			"index" TEXT DEFAULT "",
			interval INTEGER DEFAULT 15,
			size INTEGER DEFAULT 100,
			query TEXT DEFAULT "{}",
			enabled INTEGER DEFAULT 0
		)`,
	}
	for _, ddl := range tables {
		if _, err := s.db.Exec(ddl); err != nil {
			return err
		}
	}
	alterTableMigrations := []string{
		"ALTER TABLE webhook_config ADD COLUMN platform TEXT DEFAULT 'feishu'",
		"ALTER TABLE webhook_config ADD COLUMN secret TEXT DEFAULT ''",
		"ALTER TABLE webhook_config ADD COLUMN enabled INTEGER DEFAULT 1",
		"ALTER TABLE webhook_config ADD COLUMN template TEXT DEFAULT ''",
		"ALTER TABLE webhook_config ADD COLUMN name TEXT DEFAULT ''",
		"ALTER TABLE alert_rules ADD COLUMN webhook_id INTEGER DEFAULT 0",
		"ALTER TABLE es_config ADD COLUMN size INTEGER DEFAULT 100",
		"ALTER TABLE hosts ADD COLUMN project TEXT DEFAULT ''",
		"ALTER TABLE hosts ADD COLUMN ssh_credential_id TEXT DEFAULT ''",
		"ALTER TABLE ssh_credentials ADD COLUMN port INTEGER DEFAULT 22",
	}
	for _, sql := range alterTableMigrations {
		s.db.Exec(sql)
	}
	s.db.Exec(`CREATE TABLE IF NOT EXISTS es_config (
		id INTEGER PRIMARY KEY,
		address TEXT DEFAULT '',
		username TEXT DEFAULT '',
		password TEXT DEFAULT '',
		"index" TEXT DEFAULT '',
		interval INTEGER DEFAULT 15,
		size INTEGER DEFAULT 100,
		query TEXT DEFAULT '{}',
		enabled INTEGER DEFAULT 0
	)`)
	s.db.Exec("DELETE FROM assignments WHERE host_id NOT IN (SELECT id FROM hosts)")
	oldSeedRoles := []string{"role-redis", "role-mysql", "role-nginx", "role-postgresql", "role-ssh", "role-k8s", "role-rabbitmq"}
	for _, rid := range oldSeedRoles {
		s.db.Exec("DELETE FROM assignments WHERE role_id = ?", rid)
		s.db.Exec("DELETE FROM roles WHERE id = ?", rid)
	}
	return s.seedDefaults()
}

func (s *SQLiteStore) seedDefaults() error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM roles").Scan(&count)
	if err != nil || count > 0 {
		return err
	}
	defaults := []model.Role{
		{ID: "role-icmp", Name: "ICMP", Type: "ICMP", Timeout: 5},
	}
	for _, r := range defaults {
		_, err := s.db.Exec("INSERT INTO roles (id, name, type, port, path, timeout) VALUES (?, ?, ?, ?, ?, ?)", r.ID, r.Name, r.Type, r.Port, r.Path, r.Timeout)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) ListAlertRules() ([]model.AlertRule, error) {
	rows, err := s.db.Query("SELECT id, name, enabled, keywords, exclude_keywords, level, regex_pattern, cooldown, message_template, webhook_id, created_at, updated_at FROM alert_rules")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	rules := make([]model.AlertRule, 0)
	for rows.Next() {
		var r model.AlertRule
		if err := rows.Scan(&r.ID, &r.Name, &r.Enabled, &r.Keywords, &r.ExcludeKeywords, &r.Level, &r.RegexPattern, &r.Cooldown, &r.MessageTemplate, &r.WebhookID, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (s *SQLiteStore) SaveAlertRule(r *model.AlertRule) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO alert_rules (id, name, enabled, keywords, exclude_keywords, level, regex_pattern, cooldown, message_template, webhook_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		r.ID, r.Name, model.BoolToInt(r.Enabled), r.Keywords, r.ExcludeKeywords, r.Level, r.RegexPattern, r.Cooldown, r.MessageTemplate, r.WebhookID, r.CreatedAt, r.UpdatedAt)
	return err
}

func (s *SQLiteStore) GetAlertRule(id string) (*model.AlertRule, error) {
	r := &model.AlertRule{}
	var enabledVal int
	err := s.db.QueryRow("SELECT id, name, enabled, keywords, exclude_keywords, level, regex_pattern, cooldown, message_template, webhook_id FROM alert_rules WHERE id = ?", id).Scan(&r.ID, &r.Name, &enabledVal, &r.Keywords, &r.ExcludeKeywords, &r.Level, &r.RegexPattern, &r.Cooldown, &r.MessageTemplate, &r.WebhookID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = model.IntToBool(enabledVal)
	return r, nil
}

func (s *SQLiteStore) DeleteAlertRule(id string) error {
	_, err := s.db.Exec("DELETE FROM alert_rules WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) GetWebhookConfig(id int) (*model.WebhookConfig, error) {
	c := &model.WebhookConfig{}
	var enabledVal int
	err := s.db.QueryRow("SELECT id, name, platform, url, secret, enabled, max_retries, mention_type, mention_users, rate_limit, rate_limit_per_second, ring_buffer_size, template FROM webhook_config WHERE id = ?", id).Scan(&c.ID, &c.Name, &c.Platform, &c.URL, &c.Secret, &enabledVal, &c.MaxRetries, &c.MentionType, &c.MentionUsers, &c.RateLimit, &c.RateLimitPerSecond, &c.RingBufferSize, &c.Template)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Enabled = model.IntToBool(enabledVal)
	return c, nil
}

func (s *SQLiteStore) ListWebhookConfigs() ([]model.WebhookConfig, error) {
	rows, err := s.db.Query("SELECT id, name, platform, url, secret, enabled, max_retries, mention_type, mention_users, rate_limit, rate_limit_per_second, ring_buffer_size, template FROM webhook_config ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []model.WebhookConfig
	for rows.Next() {
		var c model.WebhookConfig
		var enabledVal int
		if err := rows.Scan(&c.ID, &c.Name, &c.Platform, &c.URL, &c.Secret, &enabledVal, &c.MaxRetries, &c.MentionType, &c.MentionUsers, &c.RateLimit, &c.RateLimitPerSecond, &c.RingBufferSize, &c.Template); err != nil {
			return nil, err
		}
		c.Enabled = model.IntToBool(enabledVal)
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

func (s *SQLiteStore) SaveWebhookConfig(c *model.WebhookConfig) error {
	if c.ID > 0 {
		_, err := s.db.Exec("UPDATE webhook_config SET name=?, platform=?, url=?, secret=?, enabled=?, max_retries=?, mention_type=?, mention_users=?, rate_limit=?, rate_limit_per_second=?, ring_buffer_size=?, template=? WHERE id=?",
			c.Name, c.Platform, c.URL, c.Secret, model.BoolToInt(c.Enabled), c.MaxRetries, c.MentionType, c.MentionUsers, c.RateLimit, c.RateLimitPerSecond, c.RingBufferSize, c.Template, c.ID)
		return err
	}
	res, err := s.db.Exec("INSERT INTO webhook_config (name, platform, url, secret, enabled, max_retries, mention_type, mention_users, rate_limit, rate_limit_per_second, ring_buffer_size, template) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		c.Name, c.Platform, c.URL, c.Secret, model.BoolToInt(c.Enabled), c.MaxRetries, c.MentionType, c.MentionUsers, c.RateLimit, c.RateLimitPerSecond, c.RingBufferSize, c.Template)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = int(id)
	return nil
}

func (s *SQLiteStore) DeleteWebhookConfig(id int) error {
	_, err := s.db.Exec("DELETE FROM webhook_config WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) SaveLimitedAlert(a *model.LimitedAlert) error {
	_, err := s.db.Exec("INSERT INTO limited_alert_logs (rule_name, message, level, source, timestamp, limited_at, summary) VALUES (?, ?, ?, ?, ?, ?, ?)",
		a.RuleName, a.Message, a.Level, a.Source, a.Timestamp, a.LimitedAt, a.Summary)
	return err
}

func (s *SQLiteStore) ListLimitedAlerts(limit, offset int) ([]model.LimitedAlert, error) {
	rows, err := s.db.Query("SELECT id, rule_name, message, level, source, timestamp, limited_at, summary FROM limited_alert_logs ORDER BY id DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var alerts []model.LimitedAlert
	for rows.Next() {
		var a model.LimitedAlert
		if err := rows.Scan(&a.ID, &a.RuleName, &a.Message, &a.Level, &a.Source, &a.Timestamp, &a.LimitedAt, &a.Summary); err != nil {
			return nil, err
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *SQLiteStore) CountLimitedAlerts() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM limited_alert_logs").Scan(&n)
	return n, err
}

func (s *SQLiteStore) LoadLimitedAlertsForRetry(limit int) ([]model.LimitedAlert, error) {
	return s.ListLimitedAlerts(limit, 0)
}

func (s *SQLiteStore) ClearLimitedAlerts() error {
	_, err := s.db.Exec("DELETE FROM limited_alert_logs")
	return err
}

func (s *SQLiteStore) DeleteOldLimitedAlerts(before time.Time) (int64, error) {
	res, err := s.db.Exec("DELETE FROM limited_alert_logs WHERE limited_at < ?", before.Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *SQLiteStore) ListHosts() ([]model.Host, error) {
	rows, err := s.db.Query("SELECT id, ip, hostname, project, cpu, memory, disk, status, maintenance, ssh_credential_id, last_check_time FROM hosts")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []model.Host
	for rows.Next() {
		var h model.Host
		var maint int
		var lct string
		if err := rows.Scan(&h.ID, &h.IP, &h.Hostname, &h.Project, &h.CPU, &h.Memory, &h.Disk, &h.Status, &maint, &h.SSHCredentialID, &lct); err != nil {
			return nil, err
		}
		h.Maintenance = maint != 0
		if lct != "" {
			h.LastCheckTime, _ = time.Parse("2006-01-02 15:04:05", lct)
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (s *SQLiteStore) GetHost(id string) (*model.Host, error) {
	var h model.Host
	var maint int
	var lct string
	err := s.db.QueryRow("SELECT id, ip, hostname, project, cpu, memory, disk, status, maintenance, ssh_credential_id, last_check_time FROM hosts WHERE id = ?", id).Scan(&h.ID, &h.IP, &h.Hostname, &h.Project, &h.CPU, &h.Memory, &h.Disk, &h.Status, &maint, &h.SSHCredentialID, &lct)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	h.Maintenance = maint != 0
	if lct != "" {
		h.LastCheckTime, _ = time.Parse("2006-01-02 15:04:05", lct)
	}
	return &h, nil
}

func (s *SQLiteStore) AddHost(h *model.Host) error {
	lct := ""
	if !h.LastCheckTime.IsZero() {
		lct = h.LastCheckTime.Format("2006-01-02 15:04:05")
	}
	_, err := s.db.Exec("INSERT INTO hosts (id, ip, hostname, project, cpu, memory, disk, status, maintenance, ssh_credential_id, last_check_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		h.ID, h.IP, h.Hostname, h.Project, h.CPU, h.Memory, h.Disk, h.Status, model.BoolToInt(h.Maintenance), h.SSHCredentialID, lct)
	return err
}

func (s *SQLiteStore) UpdateHost(h *model.Host) error {
	lct := ""
	if !h.LastCheckTime.IsZero() {
		lct = h.LastCheckTime.Format("2006-01-02 15:04:05")
	}
	_, err := s.db.Exec("UPDATE hosts SET ip=?, hostname=?, project=?, cpu=?, memory=?, disk=?, status=?, maintenance=?, ssh_credential_id=?, last_check_time=? WHERE id=?",
		h.IP, h.Hostname, h.Project, h.CPU, h.Memory, h.Disk, h.Status, model.BoolToInt(h.Maintenance), h.SSHCredentialID, lct, h.ID)
	return err
}

func (s *SQLiteStore) UpdateHostStatus(id string, status int, checkTime time.Time) error {
	_, err := s.db.Exec("UPDATE hosts SET status = ?, last_check_time = ? WHERE id = ?", status, checkTime.Format("2006-01-02 15:04:05"), id)
	return err
}

func (s *SQLiteStore) UpdateHostMaintenance(id string, maintenance bool) error {
	_, err := s.db.Exec("UPDATE hosts SET maintenance = ? WHERE id = ?", model.BoolToInt(maintenance), id)
	return err
}

func (s *SQLiteStore) DeleteHost(id string) error {
	_, err := s.db.Exec("DELETE FROM assignments WHERE host_id = ?", id)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("DELETE FROM hosts WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) ListRoles() ([]model.Role, error) {
	rows, err := s.db.Query("SELECT id, name, type, port, path, timeout FROM roles")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var roles []model.Role
	for rows.Next() {
		var r model.Role
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Port, &r.Path, &r.Timeout); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

func (s *SQLiteStore) GetRole(id string) (*model.Role, error) {
	var r model.Role
	err := s.db.QueryRow("SELECT id, name, type, port, path, timeout FROM roles WHERE id = ?", id).Scan(&r.ID, &r.Name, &r.Type, &r.Port, &r.Path, &r.Timeout)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *SQLiteStore) AddRole(r *model.Role) error {
	_, err := s.db.Exec("INSERT INTO roles (id, name, type, port, path, timeout) VALUES (?, ?, ?, ?, ?, ?)", r.ID, r.Name, r.Type, r.Port, r.Path, r.Timeout)
	return err
}

func (s *SQLiteStore) DeleteRole(id string) error {
	s.db.Exec("DELETE FROM assignments WHERE role_id = ?", id)
	_, err := s.db.Exec("DELETE FROM roles WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdateRole(r *model.Role) error {
	_, err := s.db.Exec("UPDATE roles SET name=?, type=?, port=?, path=?, timeout=? WHERE id=?", r.Name, r.Type, r.Port, r.Path, r.Timeout, r.ID)
	return err
}

func (s *SQLiteStore) ListAssignments() ([]model.Assignment, error) {
	rows, err := s.db.Query("SELECT host_id, role_id, status, status_code, last_check_time, error_message, override_port, override_path, consecutive_failures FROM assignments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var as []model.Assignment
	for rows.Next() {
		var a model.Assignment
		var lct string
		var op sql.NullInt64
		if err := rows.Scan(&a.HostID, &a.RoleID, &a.Status, &a.StatusCode, &lct, &a.ErrorMessage, &op, &a.OverridePath, &a.ConsecutiveFailures); err != nil {
			return nil, err
		}
		if lct != "" {
			a.LastCheckTime, _ = time.Parse("2006-01-02 15:04:05", lct)
		}
		if op.Valid {
			v := int(op.Int64)
			a.OverridePort = &v
		}
		as = append(as, a)
	}
	return as, rows.Err()
}

func (s *SQLiteStore) GetAssignment(hostID, roleID string) (*model.Assignment, error) {
	var a model.Assignment
	var lct string
	var op sql.NullInt64
	err := s.db.QueryRow("SELECT host_id, role_id, status, status_code, last_check_time, error_message, override_port, override_path, consecutive_failures FROM assignments WHERE host_id = ? AND role_id = ?", hostID, roleID).Scan(&a.HostID, &a.RoleID, &a.Status, &a.StatusCode, &lct, &a.ErrorMessage, &op, &a.OverridePath, &a.ConsecutiveFailures)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if lct != "" {
		a.LastCheckTime, _ = time.Parse("2006-01-02 15:04:05", lct)
	}
	if op.Valid {
		v := int(op.Int64)
		a.OverridePort = &v
	}
	return &a, nil
}

func (s *SQLiteStore) DeleteAssignment(hostID, roleID string) error {
	_, err := s.db.Exec("DELETE FROM assignments WHERE host_id = ? AND role_id = ?", hostID, roleID)
	return err
}

func (s *SQLiteStore) AddAssignment(a *model.Assignment) error {
	lct := ""
	if !a.LastCheckTime.IsZero() {
		lct = a.LastCheckTime.Format("2006-01-02 15:04:05")
	}
	var op interface{}
	if a.OverridePort != nil {
		op = *a.OverridePort
	}
	_, err := s.db.Exec("INSERT INTO assignments (host_id, role_id, status, status_code, last_check_time, error_message, override_port, override_path, consecutive_failures) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		a.HostID, a.RoleID, a.Status, a.StatusCode, lct, a.ErrorMessage, op, a.OverridePath, a.ConsecutiveFailures)
	return err
}

func (s *SQLiteStore) UpdateAssignmentStatus(hostID, roleID string, status, statusCode int, errMsg string, checkTime time.Time) error {
	_, err := s.db.Exec("UPDATE assignments SET status = ?, status_code = ?, error_message = ?, last_check_time = ? WHERE host_id = ? AND role_id = ?",
		status, statusCode, errMsg, checkTime.Format("2006-01-02 15:04:05"), hostID, roleID)
	return err
}

func (s *SQLiteStore) UpdateAssignmentConsecutiveFailures(hostID, roleID string, failures int) error {
	_, err := s.db.Exec("UPDATE assignments SET consecutive_failures = ? WHERE host_id = ? AND role_id = ?", failures, hostID, roleID)
	return err
}

func (s *SQLiteStore) ListSSHCredentials() ([]model.SSHCredential, error) {
	rows, err := s.db.Query("SELECT id, label, username, auth_method, password, private_key, port FROM ssh_credentials")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var creds []model.SSHCredential
	for rows.Next() {
		var c model.SSHCredential
		if err := rows.Scan(&c.ID, &c.Label, &c.Username, &c.AuthMethod, &c.Password, &c.PrivateKey, &c.Port); err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

func (s *SQLiteStore) GetSSHCredential(id string) (*model.SSHCredential, error) {
	var c model.SSHCredential
	err := s.db.QueryRow("SELECT id, label, username, auth_method, password, private_key, port FROM ssh_credentials WHERE id = ?", id).Scan(&c.ID, &c.Label, &c.Username, &c.AuthMethod, &c.Password, &c.PrivateKey, &c.Port)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *SQLiteStore) AddSSHCredential(c *model.SSHCredential) (string, error) {
	if c.ID == "" {
		c.ID = uuid.New().String()
	}
	if c.Port == 0 {
		c.Port = 22
	}
	_, err := s.db.Exec("INSERT INTO ssh_credentials (id, label, username, auth_method, password, private_key, port) VALUES (?, ?, ?, ?, ?, ?, ?)",
		c.ID, c.Label, c.Username, c.AuthMethod, c.Password, c.PrivateKey, c.Port)
	return c.ID, err
}

func (s *SQLiteStore) DeleteSSHCredential(id string) error {
	_, err := s.db.Exec("DELETE FROM ssh_credentials WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) GetESConfig() (*model.ESConfig, error) {
	var c model.ESConfig
	var query string
	err := s.db.QueryRow("SELECT id, address, username, password, \"index\", interval, size, query, enabled FROM es_config WHERE id = 1").Scan(&c.ID, &c.Address, &c.Username, &c.Password, &c.Index, &c.Interval, &c.Size, &query, &c.Enabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Query = query
	return &c, nil
}

func (s *SQLiteStore) SaveESConfig(c *model.ESConfig) error {
	_, err := s.db.Exec(`INSERT INTO es_config (id, address, username, password, "index", interval, size, query, enabled) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET address=excluded.address, username=excluded.username, password=excluded.password, "index"=excluded."index", interval=excluded.interval, size=excluded.size, query=excluded.query, enabled=excluded.enabled`,
		c.Address, c.Username, c.Password, c.Index, c.Interval, c.Size, c.Query, c.Enabled)
	return err
}

func (s *SQLiteStore) GetUser(username string) (*model.User, error) {
	var u model.User
	err := s.db.QueryRow("SELECT username, password_hash FROM users WHERE username = ?", username).Scan(&u.Username, &u.PasswordHash)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *SQLiteStore) SaveUser(u *model.User) error {
	_, err := s.db.Exec("INSERT OR REPLACE INTO users (username, password_hash) VALUES (?, ?)", u.Username, u.PasswordHash)
	return err
}

func (s *SQLiteStore) UpdatePassword(username, passwordHash string) error {
	_, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE username = ?", passwordHash, username)
	return err
}

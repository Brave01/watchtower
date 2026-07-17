package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"watchtower/internal/dashboard"
	"watchtower/internal/model"
	"watchtower/pkg/utils"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type apiResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	hosts, _ := h.store.ListHosts()
	assignments, _ := h.store.ListAssignments()
	roles, _ := h.store.ListRoles()

	roleMap := make(map[string]model.Role)
	for _, role := range roles {
		roleMap[role.ID] = role
	}

	data := model.DashboardData{Hosts: make([]model.HostWithRoles, 0)}
	for _, hst := range hosts {
		hwr := model.HostWithRoles{Host: hst, Roles: make([]model.Assignment, 0)}
		for _, a := range assignments {
			if a.HostID == hst.ID {
				if role, ok := roleMap[a.RoleID]; ok {
					if role.Type == model.ProbeTypeICMP {
						hwr.IsAlive = a.Status == model.HostStatusUp
						continue
					}
					a.Role = &role
				}
				hwr.Roles = append(hwr.Roles, a)
			}
		}
		data.Hosts = append(data.Hosts, hwr)
	}

	total := len(hosts)
	healthy := 0
	unhealthy := 0
	maintenance := 0
	roleUnhealthy := 0
	for _, hst := range hosts {
		if hst.Maintenance {
			maintenance++
		}
	}
	for _, hwr := range data.Hosts {
		if hwr.Host.Maintenance {
			continue
		}
		if hwr.IsAlive {
			healthy++
		} else {
			unhealthy++
		}
		if hwr.IsAlive && len(hwr.Roles) > 0 {
			for _, a := range hwr.Roles {
				if a.Status == model.HostStatusDown {
					roleUnhealthy++
					break
				}
			}
		}
	}

	displayRoles := make([]model.Role, 0, len(roles))
	for _, r := range roles {
		if r.Type != model.ProbeTypeICMP {
			displayRoles = append(displayRoles, r)
		}
	}

	resp := map[string]interface{}{
		"hosts": data.Hosts,
		"roles": displayRoles,
		"stats": map[string]int{
			"total":          total,
			"healthy":        healthy,
			"unhealthy":      unhealthy,
			"maintenance":    maintenance,
			"role_unhealthy": roleUnhealthy,
		},
	}
	utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Data: resp})
}

func (h *DashboardHandler) HandleHosts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			IP              string `json:"ip"`
			Hostname        string `json:"hostname"`
			Project         string `json:"project"`
			SSHCredentialID string `json:"ssh_credential_id"`
			CPU             string `json:"cpu"`
			Memory          string `json:"memory"`
			Disk            string `json:"disk"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
			return
		}
		if req.IP == "" || req.Hostname == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "IP 和主机名不能为空"})
			return
		}
		host := &model.Host{
			ID:              uuid.New().String(),
			IP:              req.IP,
			Hostname:        req.Hostname,
			Project:         req.Project,
			SSHCredentialID: req.SSHCredentialID,
			CPU:             req.CPU,
			Memory:          req.Memory,
			Disk:            req.Disk,
			Status:          model.HostStatusUnknown,
		}
		if err := h.store.AddHost(host); err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		icmpRole, _ := h.store.GetRole(model.RoleIDICMP)
		if icmpRole != nil {
			h.store.AddAssignment(&model.Assignment{
				HostID: host.ID,
				RoleID: model.RoleIDICMP,
				Status: model.HostStatusUnknown,
			})
		}
		if h.scheduler != nil && h.scheduler.ProbeOne != nil {
			go h.scheduler.ProbeOne(host.ID)
		}
		go func() {
			results := dashboard.CollectHosts([]model.Host{*host}, h.store)
			if len(results) > 0 && results[0].Success {
				r := results[0]
				if r.Hostname != "" {
					host.Hostname = r.Hostname
				}
				if r.CPU != "" {
					host.CPU = r.CPU
				}
				if r.Memory != "" {
					host.Memory = r.Memory
				}
				if r.Disk != "" {
					host.Disk = r.Disk
				}
				h.store.UpdateHost(host)
			}
		}()
		utils.WriteJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "主机已添加", Data: host})

	case http.MethodGet:
		hosts, _ := h.store.ListHosts()
		if hosts == nil {
			hosts = make([]model.Host, 0)
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Data: hosts})

	case http.MethodPut:
		var req struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Type    string `json:"type"`
			Port    int    `json:"port"`
			Path    string `json:"path"`
			Timeout int    `json:"timeout"`
		}
		id := r.URL.Query().Get("id")
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
			return
		}
		if id == "" {
			id = req.ID
		}
		if id == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "缺少 id 参数"})
			return
		}
		existing, _ := h.store.GetRole(id)
		if existing == nil {
			utils.WriteJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "角色不存在"})
			return
		}
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Type != "" {
			existing.Type = req.Type
		}
		if req.Port > 0 {
			existing.Port = req.Port
		}
		if req.Timeout > 0 {
			existing.Timeout = req.Timeout
		}
		existing.Path = req.Path
		if err := h.store.UpdateRole(existing); err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "角色已更新", Data: existing})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if err := h.store.DeleteHost(id); err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "主机已删除"})

	default:
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
	}
}

func (h *DashboardHandler) HandleHostUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	var req struct {
		ID              string `json:"id"`
		IP              string `json:"ip"`
		Hostname        string `json:"hostname"`
		Project         string `json:"project"`
		SSHCredentialID string `json:"ssh_credential_id"`
		CPU             string `json:"cpu"`
		Memory          string `json:"memory"`
		Disk            string `json:"disk"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
		return
	}
	host, err := h.store.GetHost(req.ID)
	if err != nil {
		utils.WriteJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "主机不存在"})
		return
	}
	if req.IP != "" {
		host.IP = req.IP
	}
	if req.Hostname != "" {
		host.Hostname = req.Hostname
	}
	if req.Project != "" {
		host.Project = req.Project
	}
	if req.SSHCredentialID != "" {
		host.SSHCredentialID = req.SSHCredentialID
	}
	if req.CPU != "" {
		host.CPU = req.CPU
	}
	if req.Memory != "" {
		host.Memory = req.Memory
	}
	if req.Disk != "" {
		host.Disk = req.Disk
	}
	if err := h.store.UpdateHost(host); err != nil {
		utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
		return
	}
	utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "主机已更新", Data: host})
}

func (h *DashboardHandler) HandleBatchHosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	var req struct {
		Hosts []struct {
			Hostname        string `json:"hostname"`
			IP              string `json:"ip"`
			Project         string `json:"project"`
			SSHCredentialID string `json:"ssh_credential_id"`
			CPU             string `json:"cpu"`
			Memory          string `json:"memory"`
			Disk            string `json:"disk"`
		} `json:"hosts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
		return
	}
	var results []*model.Host
	var errors []string
	for _, hr := range req.Hosts {
		if hr.IP == "" || hr.Hostname == "" {
			errors = append(errors, hr.Hostname+": IP 和主机名不能为空")
			continue
		}
		host := &model.Host{
			ID:              uuid.New().String(),
			IP:              hr.IP,
			Hostname:        hr.Hostname,
			Project:         hr.Project,
			SSHCredentialID: hr.SSHCredentialID,
			CPU:             hr.CPU,
			Memory:          hr.Memory,
			Disk:            hr.Disk,
			Status:          model.HostStatusUnknown,
		}
		if err := h.store.AddHost(host); err != nil {
			errors = append(errors, hr.Hostname+": "+err.Error())
			continue
		}
		icmpRole, _ := h.store.GetRole(model.RoleIDICMP)
		if icmpRole != nil {
			h.store.AddAssignment(&model.Assignment{
				HostID: host.ID,
				RoleID: model.RoleIDICMP,
				Status: model.HostStatusUnknown,
			})
		}
		results = append(results, host)
	}
	resp := map[string]interface{}{
		"count":  len(results),
		"hosts":  results,
		"errors": errors,
	}
	utils.WriteJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "批量添加完成", Data: resp})
}

func (h *DashboardHandler) HandleMaintenance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	var req struct {
		HostID string `json:"host_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
		return
	}
	host, err := h.store.GetHost(req.HostID)
	if err != nil || host == nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "主机不存在"})
		return
	}
	if err := h.store.UpdateHostMaintenance(req.HostID, !host.Maintenance); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: err.Error()})
		return
	}
	updated, _ := h.store.GetHost(req.HostID)
	msg := "维护模式已关闭"
	if updated.Maintenance {
		msg = "维护模式已开启"
	}
	utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: msg, Data: updated})
}

func (h *DashboardHandler) HandleRoles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Port    int    `json:"port"`
			Path    string `json:"path"`
			Timeout int    `json:"timeout"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
			return
		}
		if req.Name == "" || req.Type == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "名称和类型不能为空"})
			return
		}
		roleType := req.Type
		switch roleType {
		case model.ProbeTypeICMP:
		case model.ProbeTypeTCP, model.ProbeTypeHTTP, model.ProbeTypeSSH:
			if req.Port == 0 {
				utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: roleType + " 探测必须指定端口"})
				return
			}
		default:
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "探测类型必须是 ICMP、TCP、HTTP 或 SSH"})
			return
		}
		if req.Timeout <= 0 {
			req.Timeout = 5
		}
		role := &model.Role{
			ID:      uuid.New().String(),
			Name:    req.Name,
			Type:    roleType,
			Port:    req.Port,
			Path:    req.Path,
			Timeout: req.Timeout,
		}
		if err := h.store.AddRole(role); err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "角色已添加", Data: role})

	case http.MethodGet:
		roles, _ := h.store.ListRoles()
		if roles == nil {
			roles = make([]model.Role, 0)
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Data: roles})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "缺少 id 参数"})
			return
		}
		role, _ := h.store.GetRole(id)
		if role == nil {
			utils.WriteJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "角色不存在"})
			return
		}
		if role.Type == model.ProbeTypeICMP {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "不能删除 ICMP 存活检测角色"})
			return
		}
		if err := h.store.DeleteRole(id); err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "角色已删除"})

	default:
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
	}
}

func (h *DashboardHandler) HandleBatchRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	var req struct {
		Roles []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Port    int    `json:"port"`
			Path    string `json:"path"`
			Timeout int    `json:"timeout"`
		} `json:"roles"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
		return
	}
	var results []*model.Role
	var errors []string
	for _, rr := range req.Roles {
		if rr.Name == "" || rr.Type == "" {
			errors = append(errors, rr.Name+": 名称和类型不能为空")
			continue
		}
		if rr.Timeout <= 0 {
			rr.Timeout = 5
		}
		role := &model.Role{
			ID:      uuid.New().String(),
			Name:    rr.Name,
			Type:    rr.Type,
			Port:    rr.Port,
			Path:    rr.Path,
			Timeout: rr.Timeout,
		}
		if err := h.store.AddRole(role); err != nil {
			errors = append(errors, rr.Name+": "+err.Error())
			continue
		}
		results = append(results, role)
	}
	resp := map[string]interface{}{
		"count":  len(results),
		"roles":  results,
		"errors": errors,
	}
	utils.WriteJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "批量添加完成", Data: resp})
}

func (h *DashboardHandler) HandleAssign(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			HostID string `json:"host_id"`
			RoleID string `json:"role_id"`
			Port   *int   `json:"port,omitempty"`
			Path   string `json:"path,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
			return
		}
		if req.HostID == "" || req.RoleID == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "host_id 和 role_id 不能为空"})
			return
		}
		host, _ := h.store.GetHost(req.HostID)
		if host == nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "主机不存在"})
			return
		}
		role, _ := h.store.GetRole(req.RoleID)
		if role == nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "角色不存在"})
			return
		}
		existing, _ := h.store.GetAssignment(req.HostID, req.RoleID)
		if existing != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "该主机已绑定此角色"})
			return
		}
		var overridePort *int
		if req.Port != nil && *req.Port > 0 {
			overridePort = req.Port
		}
		if err := h.store.AddAssignment(&model.Assignment{
			HostID:        req.HostID,
			RoleID:        req.RoleID,
			Status:        model.HostStatusUnknown,
			OverridePort:  overridePort,
			OverridePath:  req.Path,
			LastCheckTime: time.Time{},
		}); err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		if h.scheduler != nil && h.scheduler.ProbeOne != nil {
			go h.scheduler.ProbeOne(req.HostID)
		}
		utils.WriteJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "角色已指派"})

	case http.MethodDelete:
		hostID := r.URL.Query().Get("host_id")
		roleID := r.URL.Query().Get("role_id")
		if hostID == "" || roleID == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "host_id 和 role_id 不能为空"})
			return
		}
		if err := h.store.DeleteAssignment(hostID, roleID); err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "角色已取消指派"})

	default:
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
	}
}

func (h *DashboardHandler) HandleBatchAssign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	var req struct {
		HostIDs []string `json:"host_ids"`
		RoleID  string   `json:"role_id"`
		Port    *int     `json:"port,omitempty"`
		Path    string   `json:"path,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
		return
	}
	if len(req.HostIDs) == 0 || req.RoleID == "" {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "host_ids 和 role_id 不能为空"})
		return
	}
	role, _ := h.store.GetRole(req.RoleID)
	if role == nil {
		utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "角色不存在"})
		return
	}
	var overridePort *int
	if req.Port != nil && *req.Port > 0 {
		overridePort = req.Port
	}
	count := 0
	for _, hostID := range req.HostIDs {
		host, _ := h.store.GetHost(hostID)
		if host == nil {
			continue
		}
		existing, _ := h.store.GetAssignment(hostID, req.RoleID)
		if existing != nil {
			continue
		}
		if err := h.store.AddAssignment(&model.Assignment{
			HostID:        hostID,
			RoleID:        req.RoleID,
			Status:        model.HostStatusUnknown,
			OverridePort:  overridePort,
			OverridePath:  req.Path,
			LastCheckTime: time.Time{},
		}); err == nil {
			count++
			if h.scheduler != nil && h.scheduler.ProbeOne != nil {
				go h.scheduler.ProbeOne(hostID)
			}
		}
	}
	utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "批量指派成功", Data: map[string]int{"count": count}})
}

func (h *DashboardHandler) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	if h.scheduler != nil && h.scheduler.Trigger != nil {
		h.scheduler.Trigger()
	}
	utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "探测已触发"})
}

func (h *DashboardHandler) HandleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	var req struct {
		HostID string `json:"host_id"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	hosts, err := h.store.ListHosts()
	if err != nil {
		utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: "获取主机列表失败: " + err.Error()})
		return
	}

	var targetHosts []model.Host
	if req.HostID != "" {
		for _, hst := range hosts {
			if hst.ID == req.HostID {
				targetHosts = append(targetHosts, hst)
				break
			}
		}
		if len(targetHosts) == 0 {
			utils.WriteJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "主机不存在"})
			return
		}
	} else {
		targetHosts = hosts
	}

	go func() {
		results := dashboard.CollectHosts(targetHosts, h.store)
		for _, res := range results {
			if !res.Success {
				log.Printf("[Collect] 主机 %s 采集失败: %s", res.HostID, res.Error)
				continue
			}
			host, err := h.store.GetHost(res.HostID)
			if err != nil || host == nil {
				continue
			}
			if res.Hostname != "" {
				host.Hostname = res.Hostname
			}
			if res.CPU != "" {
				host.CPU = res.CPU
			}
			if res.Memory != "" {
				host.Memory = res.Memory
			}
			if res.Disk != "" {
				host.Disk = res.Disk
			}
			if err := h.store.UpdateHost(host); err != nil {
				log.Printf("[Collect] 主机 %s 更新失败: %v", res.HostID, err)
			} else {
				log.Printf("[Collect] 主机 %s 采集更新完成 (hostname=%s cpu=%s mem=%s disk=%s)",
					res.HostID, res.Hostname, res.CPU, res.Memory, res.Disk)
			}
		}
	}()

	utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "采集任务已触发", Data: map[string]int{"count": len(targetHosts)}})
}

func (h *DashboardHandler) HandleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		return
	}
	hosts, _ := h.store.ListHosts()
	assignments, _ := h.store.ListAssignments()
	roles, _ := h.store.ListRoles()

	roleMap := make(map[string]model.Role)
	for _, role := range roles {
		roleMap[role.ID] = role
	}

	type diskPart struct {
		Mount string
		Size  string
	}
	hostParts := make(map[string][]diskPart)
	mountSet := make(map[string]bool)
	for _, hst := range hosts {
		var parts []diskPart
		if hst.Disk != "" {
			for _, seg := range strings.Split(hst.Disk, ",") {
				seg = strings.TrimSpace(seg)
				if seg == "" {
					continue
				}
				kv := strings.SplitN(seg, ":", 2)
				mount := strings.TrimSpace(kv[0])
				size := ""
				if len(kv) > 1 {
					size = strings.TrimSpace(kv[1])
				}
				if mount != "" {
					parts = append(parts, diskPart{Mount: mount, Size: size})
					mountSet[mount] = true
				}
			}
		}
		hostParts[hst.ID] = parts
	}
	mountOrder := make([]string, 0, len(mountSet))
	for m := range mountSet {
		mountOrder = append(mountOrder, m)
	}
	sort.Strings(mountOrder)

	f := excelize.NewFile()
	defer f.Close()

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true, Size: 12, Color: "#FFFFFF"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"4472C4"}},
	})

	sheet := "主机列表"
	f.SetSheetName("Sheet1", sheet)

	baseHeaders := []string{"主机名称", "主机地址", "状态", "CPU(核)", "内存(GB)", "项目"}
	diskHeaders := make([]string, len(mountOrder))
	for i, mount := range mountOrder {
		if mount == "/" {
			diskHeaders[i] = "磁盘(系统盘)"
		} else {
			diskHeaders[i] = "磁盘(" + mount + ")"
		}
	}
	tailHeaders := []string{"角色", "维护模式"}
	allHeaders := append(append(append([]string{}, baseHeaders...), diskHeaders...), tailHeaders...)

	baseWidths := []float64{20, 20, 10, 10, 10, 16}
	diskWidth := 14.0
	tailWidths := []float64{30, 12}

	for i, hdr := range allHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, hdr)
		f.SetCellStyle(sheet, cell, cell, headerStyle)
		col, _ := excelize.ColumnNumberToName(i + 1)
		if i < len(baseWidths) {
			f.SetColWidth(sheet, col, col, baseWidths[i])
		} else if i < len(baseWidths)+len(diskHeaders) {
			f.SetColWidth(sheet, col, col, diskWidth)
		} else {
			f.SetColWidth(sheet, col, col, tailWidths[i-len(baseWidths)-len(diskHeaders)])
		}
	}

	for rowIdx, hst := range hosts {
		row := rowIdx + 2
		col := 1

		statusStr := "未知"
		switch hst.Status {
		case model.HostStatusUp:
			statusStr = "健康"
		case model.HostStatusDown:
			statusStr = "异常"
		case model.HostStatusWarning:
			statusStr = "警告"
		}

		var roleNames []string
		for _, a := range assignments {
			if a.HostID == hst.ID {
				if role, ok := roleMap[a.RoleID]; ok {
					roleNames = append(roleNames, role.Name)
				}
			}
		}
		roleStr := ""
		for i, name := range roleNames {
			if i > 0 {
				roleStr += ", "
			}
			roleStr += name
		}
		maintStr := "否"
		if hst.Maintenance {
			maintStr = "是"
		}

		setCell := func(val string) {
			cell, _ := excelize.CoordinatesToCellName(col, row)
			f.SetCellValue(sheet, cell, val)
			col++
		}

		setCell(hst.Hostname)
		setCell(hst.IP)
		setCell(statusStr)
		setCell(hst.CPU)
		setCell(hst.Memory)
		setCell(hst.Project)

		parts := hostParts[hst.ID]
		for _, mount := range mountOrder {
			sizeVal := "-"
			for _, p := range parts {
				if p.Mount == mount {
					sizeVal = p.Size
					if sizeVal == "" {
						sizeVal = "-"
					}
					break
				}
			}
			setCell(sizeVal)
		}

		setCell(roleStr)
		setCell(maintStr)
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=hosts_export_"+time.Now().Format("20060102_150405")+".xlsx")
	if err := f.Write(w); err != nil {
		utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: "导出失败: " + err.Error()})
		return
	}
}

func (h *DashboardHandler) HandleSSHCredential(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, _ := h.store.ListSSHCredentials()
		masked := make([]model.SSHCredential, 0, len(list))
		for _, c := range list {
			m := c
			if m.Password != "" {
				m.Password = "******"
			}
			if m.PrivateKey != "" {
				if len(m.PrivateKey) > 40 {
					m.PrivateKey = m.PrivateKey[:40] + "...(已加密)"
				} else {
					m.PrivateKey = "******"
				}
			}
			masked = append(masked, m)
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Data: masked})

	case http.MethodPost:
		var cred model.SSHCredential
		if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
			return
		}
		if cred.Username == "" || cred.AuthMethod == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "用户名和认证方式不能为空"})
			return
		}
		id, err := h.store.AddSSHCredential(&cred)
		if err != nil {
			utils.WriteJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "SSH 凭据已添加", Data: map[string]string{"id": id}})

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			utils.WriteJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "缺少 id 参数"})
			return
		}
		if err := h.store.DeleteSSHCredential(id); err != nil {
			utils.WriteJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: err.Error()})
			return
		}
		utils.WriteJSON(w, http.StatusOK, apiResponse{Success: true, Message: "凭据已删除"})

	default:
		utils.WriteJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
	}
}

func (h *DashboardHandler) HandleSSHWebSocket(w http.ResponseWriter, r *http.Request) {
	dashHandle := dashboard.HandleSSHWebSocket(h.store)
	dashHandle(w, r)
}

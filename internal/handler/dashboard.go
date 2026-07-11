package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"watchtower/internal/dashboard"
	"watchtower/internal/store"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type DashboardDeps struct {
	Store     store.Store
	Scheduler *dashboard.Scheduler
}

func RegisterDashboard(mux *http.ServeMux, deps *DashboardDeps) {
	mux.HandleFunc("/api/dashboard", handleDashboard(deps))
	mux.HandleFunc("/api/hosts", handleHosts(deps))
	mux.HandleFunc("/api/hosts/update", handleHostUpdate(deps))
	mux.HandleFunc("/api/hosts/batch", handleBatchHosts(deps))
	mux.HandleFunc("/api/hosts/export", handleExport(deps))
	mux.HandleFunc("/api/hosts/maintenance", handleMaintenance(deps))
	mux.HandleFunc("/api/roles", handleRoles(deps))
	mux.HandleFunc("/api/roles/batch", handleBatchRoles(deps))
	mux.HandleFunc("/api/assign", handleAssign(deps))
	mux.HandleFunc("/api/assign/batch", handleBatchAssign(deps))
	mux.HandleFunc("/api/refresh", handleRefresh(deps))
	mux.HandleFunc("/api/ssh-credential", handleSSHCredential(deps))
	mux.HandleFunc("/api/ssh/ws", dashboard.HandleSSHWebSocket(deps.Store))
}

type apiResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

// ---------- GET /api/dashboard ----------
func handleDashboard(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		hosts, _ := deps.Store.ListHosts()
		assignments, _ := deps.Store.ListAssignments()
		roles, _ := deps.Store.ListRoles()

		roleMap := make(map[string]store.Role)
		for _, role := range roles {
			roleMap[role.ID] = role
		}

		data := store.DashboardData{Hosts: make([]store.HostWithRoles, 0)}
		for _, h := range hosts {
			hwr := store.HostWithRoles{Host: h, Roles: make([]store.Assignment, 0)}
			for _, a := range assignments {
				if a.HostID == h.ID {
					if role, ok := roleMap[a.RoleID]; ok {
						if role.Type == store.ProbeTypeICMP {
							// ICMP 存活状态单独记录，不混入普通角色
							hwr.IsAlive = a.Status == store.HostStatusUp
							continue
						}
						a.Role = &role
					}
					hwr.Roles = append(hwr.Roles, a)
				}
			}
			data.Hosts = append(data.Hosts, hwr)
		}

		// 统计时基于 IsAlive 计算健康/异常，排除维护中
		total := len(hosts)
		healthy := 0
		unhealthy := 0
		maintenance := 0
		roleUnhealthy := 0 // ICMP 在线但角色探针异常
		for _, h := range hosts {
			if h.Maintenance {
				maintenance++
				continue
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
			// 角色异常：ICMP 在线但某个角色探针异常
			if hwr.IsAlive && len(hwr.Roles) > 0 {
				for _, a := range hwr.Roles {
					if a.Status == store.HostStatusDown {
						roleUnhealthy++
						break
					}
				}
			}
		}

		// 返回的角色列表排除 ICMP
		displayRoles := make([]store.Role, 0, len(roles))
		for _, r := range roles {
			if r.Type != store.ProbeTypeICMP {
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
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: resp})
	}
}

// ---------- POST/GET/DELETE /api/hosts ----------
func handleHosts(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
				CPU      string `json:"cpu"`
				Memory   string `json:"memory"`
				Disk     string `json:"disk"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
				return
			}
			if req.IP == "" || req.Hostname == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "IP 和主机名不能为空"})
				return
			}
			host := &store.Host{
				ID:       uuid.New().String(),
				IP:       req.IP,
				Hostname: req.Hostname,
				CPU:      req.CPU,
				Memory:   req.Memory,
				Disk:     req.Disk,
				Status:   store.HostStatusUnknown,
			}
			if err := deps.Store.AddHost(host); err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			// 自动绑定 ICMP
			icmpRole, _ := deps.Store.GetRole(store.RoleIDICMP)
			if icmpRole != nil {
				deps.Store.AddAssignment(&store.Assignment{
					HostID: host.ID,
					RoleID: store.RoleIDICMP,
					Status: store.HostStatusUnknown,
				})
			}
			if deps.Scheduler != nil {
				go deps.Scheduler.ProbeHost(host.ID)
			}
			writeJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "主机已添加", Data: host})

		case http.MethodGet:
			hosts, _ := deps.Store.ListHosts()
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: hosts})

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
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
				return
			}
			if id == "" {
				id = req.ID
			}
			if id == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "缺少 id 参数"})
				return
			}
			existing, _ := deps.Store.GetRole(id)
			if existing == nil {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "角色不存在"})
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
			existing.Path = req.Path // 允许清空
			if err := deps.Store.UpdateRole(existing); err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "角色已更新", Data: existing})

		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if err := deps.Store.DeleteHost(id); err != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "主机已删除"})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		}
	}
}

// ---------- POST /api/hosts/update ----------
func handleHostUpdate(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		var req struct {
			ID       string `json:"id"`
			IP       string `json:"ip"`
			Hostname string `json:"hostname"`
			CPU      string `json:"cpu"`
			Memory   string `json:"memory"`
			Disk     string `json:"disk"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
			return
		}
		host, err := deps.Store.GetHost(req.ID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "主机不存在"})
			return
		}
		if req.IP != "" {
			host.IP = req.IP
		}
		if req.Hostname != "" {
			host.Hostname = req.Hostname
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
		if err := deps.Store.UpdateHost(host); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "主机已更新", Data: host})
	}
}

// ---------- POST /api/hosts/batch ----------
func handleBatchHosts(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		var req struct {
			Hosts []struct {
				Hostname string `json:"hostname"`
				IP       string `json:"ip"`
				CPU      string `json:"cpu"`
				Memory   string `json:"memory"`
				Disk     string `json:"disk"`
			} `json:"hosts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
			return
		}
		var results []*store.Host
		var errors []string
		for _, hr := range req.Hosts {
			if hr.IP == "" || hr.Hostname == "" {
				errors = append(errors, hr.Hostname+": IP 和主机名不能为空")
				continue
			}
			host := &store.Host{
				ID:       uuid.New().String(),
				IP:       hr.IP,
				Hostname: hr.Hostname,
				CPU:      hr.CPU,
				Memory:   hr.Memory,
				Disk:     hr.Disk,
				Status:   store.HostStatusUnknown,
			}
			if err := deps.Store.AddHost(host); err != nil {
				errors = append(errors, hr.Hostname+": "+err.Error())
				continue
			}
			icmpRole, _ := deps.Store.GetRole(store.RoleIDICMP)
			if icmpRole != nil {
				deps.Store.AddAssignment(&store.Assignment{
					HostID: host.ID,
					RoleID: store.RoleIDICMP,
					Status: store.HostStatusUnknown,
				})
			}
			results = append(results, host)
		}
		resp := map[string]interface{}{
			"count":  len(results),
			"hosts":  results,
			"errors": errors,
		}
		writeJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "批量添加完成", Data: resp})
	}
}

// ---------- POST /api/hosts/maintenance ----------
func handleMaintenance(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		var req struct {
			HostID string `json:"host_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
			return
		}
		host, err := deps.Store.GetHost(req.HostID)
		if err != nil || host == nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "主机不存在"})
			return
		}
		if err := deps.Store.UpdateHostMaintenance(req.HostID, !host.Maintenance); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: err.Error()})
			return
		}
		updated, _ := deps.Store.GetHost(req.HostID)
		msg := "维护模式已关闭"
		if updated.Maintenance {
			msg = "维护模式已开启"
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: msg, Data: updated})
	}
}

// ---------- POST/GET/DELETE /api/roles ----------
func handleRoles(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
				return
			}
			if req.Name == "" || req.Type == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "名称和类型不能为空"})
				return
			}
			roleType := req.Type
			switch roleType {
			case store.ProbeTypeICMP:
			case store.ProbeTypeTCP, store.ProbeTypeHTTP, store.ProbeTypeSSH:
				if req.Port == 0 {
					writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: roleType + " 探测必须指定端口"})
					return
				}
			default:
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "探测类型必须是 ICMP、TCP、HTTP 或 SSH"})
				return
			}
			if req.Timeout <= 0 {
				req.Timeout = 5
			}
			role := &store.Role{
				ID:      uuid.New().String(),
				Name:    req.Name,
				Type:    roleType,
				Port:    req.Port,
				Path:    req.Path,
				Timeout: req.Timeout,
			}
			if err := deps.Store.AddRole(role); err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "角色已添加", Data: role})

		case http.MethodGet:
			roles, _ := deps.Store.ListRoles()
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: roles})

		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "缺少 id 参数"})
				return
			}
			// 不允许删除 ICMP 角色
			role, _ := deps.Store.GetRole(id)
			if role == nil {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: "角色不存在"})
				return
			}
			if role.Type == store.ProbeTypeICMP {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "不能删除 ICMP 存活检测角色"})
				return
			}
			if err := deps.Store.DeleteRole(id); err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "角色已删除"})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		}
	}
}

// ---------- POST /api/roles/batch ----------
func handleBatchRoles(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
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
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON: " + err.Error()})
			return
		}
		var results []*store.Role
		var errors []string
		for _, rr := range req.Roles {
			if rr.Name == "" || rr.Type == "" {
				errors = append(errors, rr.Name+": 名称和类型不能为空")
				continue
			}
			if rr.Timeout <= 0 {
				rr.Timeout = 5
			}
			role := &store.Role{
				ID:      uuid.New().String(),
				Name:    rr.Name,
				Type:    rr.Type,
				Port:    rr.Port,
				Path:    rr.Path,
				Timeout: rr.Timeout,
			}
			if err := deps.Store.AddRole(role); err != nil {
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
		writeJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "批量添加完成", Data: resp})
	}
}

// ---------- POST/DELETE /api/assign ----------
func handleAssign(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var req struct {
				HostID string `json:"host_id"`
				RoleID string `json:"role_id"`
				Port   *int   `json:"port,omitempty"`
				Path   string `json:"path,omitempty"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
				return
			}
			if req.HostID == "" || req.RoleID == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "host_id 和 role_id 不能为空"})
				return
			}

			host, _ := deps.Store.GetHost(req.HostID)
			if host == nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "主机不存在"})
				return
			}
			role, _ := deps.Store.GetRole(req.RoleID)
			if role == nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "角色不存在"})
				return
			}
			existing, _ := deps.Store.GetAssignment(req.HostID, req.RoleID)
			if existing != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "该主机已绑定此角色"})
				return
			}

			var overridePort *int
			if req.Port != nil && *req.Port > 0 {
				overridePort = req.Port
			}
			if err := deps.Store.AddAssignment(&store.Assignment{
				HostID:        req.HostID,
				RoleID:        req.RoleID,
				Status:        store.HostStatusUnknown,
				OverridePort:  overridePort,
				OverridePath:  req.Path,
				LastCheckTime: time.Time{},
			}); err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			if deps.Scheduler != nil {
				go deps.Scheduler.ProbeHost(req.HostID)
			}
			writeJSON(w, http.StatusCreated, apiResponse{Success: true, Message: "角色已指派"})

		case http.MethodDelete:
			hostID := r.URL.Query().Get("host_id")
			roleID := r.URL.Query().Get("role_id")
			if hostID == "" || roleID == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "host_id 和 role_id 不能为空"})
				return
			}
			if err := deps.Store.DeleteAssignment(hostID, roleID); err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "角色已取消指派"})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		}
	}
}

// ---------- POST /api/assign/batch ----------
func handleBatchAssign(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		var req struct {
			HostIDs []string `json:"host_ids"`
			RoleID  string   `json:"role_id"`
			Port    *int     `json:"port,omitempty"`
			Path    string   `json:"path,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
			return
		}
		if len(req.HostIDs) == 0 || req.RoleID == "" {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "host_ids 和 role_id 不能为空"})
			return
		}
		role, _ := deps.Store.GetRole(req.RoleID)
		if role == nil {
			writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "角色不存在"})
			return
		}
		var overridePort *int
		if req.Port != nil && *req.Port > 0 {
			overridePort = req.Port
		}
		count := 0
		for _, hostID := range req.HostIDs {
			host, _ := deps.Store.GetHost(hostID)
			if host == nil {
				continue
			}
			existing, _ := deps.Store.GetAssignment(hostID, req.RoleID)
			if existing != nil {
				continue
			}
			if err := deps.Store.AddAssignment(&store.Assignment{
				HostID:        hostID,
				RoleID:        req.RoleID,
				Status:        store.HostStatusUnknown,
				OverridePort:  overridePort,
				OverridePath:  req.Path,
				LastCheckTime: time.Time{},
			}); err == nil {
				count++
				if deps.Scheduler != nil {
					go deps.Scheduler.ProbeHost(hostID)
				}
			}
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "批量指派成功", Data: map[string]int{"count": count}})
	}
}

// ---------- GET /api/refresh ----------
func handleRefresh(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		if deps.Scheduler != nil {
			deps.Scheduler.Trigger()
		}
		writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "探测已触发"})
	}
}

// ---------- GET /api/hosts/export ----------
func handleExport(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
			return
		}
		hosts, _ := deps.Store.ListHosts()
		assignments, _ := deps.Store.ListAssignments()
		roles, _ := deps.Store.ListRoles()

		roleMap := make(map[string]store.Role)
		for _, role := range roles {
			roleMap[role.ID] = role
		}

		// 收集磁盘分区信息
		type diskPart struct {
			Mount string
			Size  string
		}
		hostParts := make(map[string][]diskPart)
		mountSet := make(map[string]bool)
		for _, h := range hosts {
			var parts []diskPart
			if h.Disk != "" {
				for _, seg := range strings.Split(h.Disk, ",") {
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
			hostParts[h.ID] = parts
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

		// ---- Sheet 1: 主机列表，磁盘分区按挂载点拆分为独立列 ----
		sheet := "主机列表"
		f.SetSheetName("Sheet1", sheet)

		// 基础列：主机名称/主机地址/状态/CPU/内存
		baseHeaders := []string{"主机名称", "主机地址", "状态", "CPU(核)", "内存(GB)"}
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

		// 基础列宽度
		baseWidths := []float64{20, 20, 10, 10, 10}
		diskWidth := 14.0
		tailWidths := []float64{30, 12}

		for i, h := range allHeaders {
			cell, _ := excelize.CoordinatesToCellName(i+1, 1)
			f.SetCellValue(sheet, cell, h)
			f.SetCellStyle(sheet, cell, cell, headerStyle)
			// 设置列宽
			col, _ := excelize.ColumnNumberToName(i + 1)
			if i < len(baseWidths) {
				f.SetColWidth(sheet, col, col, baseWidths[i])
			} else if i < len(baseWidths)+len(diskHeaders) {
				f.SetColWidth(sheet, col, col, diskWidth)
			} else {
				f.SetColWidth(sheet, col, col, tailWidths[i-len(baseWidths)-len(diskHeaders)])
			}
		}

		for rowIdx, h := range hosts {
			row := rowIdx + 2
			col := 1

			statusStr := "未知"
			switch h.Status {
			case store.HostStatusUp:
				statusStr = "健康"
			case store.HostStatusDown:
				statusStr = "异常"
			case store.HostStatusWarning:
				statusStr = "警告"
			}

			var roleNames []string
			for _, a := range assignments {
				if a.HostID == h.ID {
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
			if h.Maintenance {
				maintStr = "是"
			}

			setCell := func(val string) {
				cell, _ := excelize.CoordinatesToCellName(col, row)
				f.SetCellValue(sheet, cell, val)
				col++
			}

			// 基础列
			setCell(h.Hostname)
			setCell(h.IP)
			setCell(statusStr)
			setCell(h.CPU)
			setCell(h.Memory)

			// 磁盘分区列
			parts := hostParts[h.ID]
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

			// 尾部列
			setCell(roleStr)
			setCell(maintStr)
		}

		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", "attachment; filename=hosts_export.xlsx")
		if err := f.Write(w); err != nil {
			writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: "导出失败: " + err.Error()})
			return
		}
	}
}

// ---------- GET/POST/DELETE /api/ssh-credential ----------
func handleSSHCredential(deps *DashboardDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list, _ := deps.Store.ListSSHCredentials()
			// 脱敏
			masked := make([]store.SSHCredential, 0, len(list))
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
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Data: masked})

		case http.MethodPost:
			var cred store.SSHCredential
			if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "无效 JSON"})
				return
			}
			if cred.Username == "" || cred.AuthMethod == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "用户名和认证方式不能为空"})
				return
			}
			id, err := deps.Store.AddSSHCredential(&cred)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "SSH 凭据已添加", Data: map[string]string{"id": id}})

		case http.MethodDelete:
			id := r.URL.Query().Get("id")
			if id == "" {
				writeJSON(w, http.StatusBadRequest, apiResponse{Success: false, Message: "缺少 id 参数"})
				return
			}
			if err := deps.Store.DeleteSSHCredential(id); err != nil {
				writeJSON(w, http.StatusNotFound, apiResponse{Success: false, Message: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, apiResponse{Success: true, Message: "凭据已删除"})

		default:
			writeJSON(w, http.StatusMethodNotAllowed, apiResponse{Success: false})
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

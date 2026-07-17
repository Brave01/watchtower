package dashboard

import (
	"fmt"
	"log"
	"sync"
	"time"

	"watchtower/internal/model"
	"watchtower/internal/store"
)

type Scheduler struct {
	store      store.Store
	interval   time.Duration
	MaxRetries int
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

func NewScheduler(st store.Store, interval time.Duration) *Scheduler {
	return &Scheduler{
		store:      st,
		interval:   interval,
		MaxRetries: 2,
		stopCh:     make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.probeAll()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.probeAll()
			case <-s.stopCh:
				log.Println("[INFO] [Scheduler] 停止探测调度")
				return
			}
		}
	}()
	log.Printf("[INFO] [Scheduler] 定时探测已启动，间隔: %v，最大重试: %d", s.interval, s.MaxRetries)
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *Scheduler) Trigger() {
	go s.probeAll()
}

func (s *Scheduler) ProbeHost(hostID string) {
	log.Printf("[INFO] [Scheduler] 开始单主机探测: host=%s", hostID)
	assignments, err := s.store.ListAssignments()
	if err != nil {
		log.Printf("[ERROR] [Scheduler] 单主机探测失败: host=%s, 获取分配列表失败: %s", hostID, err)
		return
	}
	var hostAssignments []model.Assignment
	for _, a := range assignments {
		if a.HostID == hostID {
			hostAssignments = append(hostAssignments, a)
		}
	}
	if len(hostAssignments) == 0 {
		log.Printf("[WARN] [Scheduler] 单主机探测跳过: host=%s, 无分配角色", hostID)
		return
	}
	creds, _ := s.store.ListSSHCredentials()
	var sshCred *model.SSHCredential
	if len(creds) > 0 {
		sshCred = &creds[0]
	}
	now := time.Now()
	for _, a := range hostAssignments {
		host, _ := s.store.GetHost(a.HostID)
		if host == nil || host.Maintenance {
			continue
		}
		role, _ := s.store.GetRole(a.RoleID)
		if role == nil {
			s.store.DeleteAssignment(a.HostID, a.RoleID)
			log.Printf("[WARN] [Scheduler] 角色不存在（可能已被删除），已清理分配: host=%s role=%s", a.HostID, a.RoleID)
			continue
		}
		pr := Probe(host.IP, role, &a, sshCred)
		s.handleProbeResult(hostID, a.RoleID, pr, now)
	}
	s.updateHostsSummary()
	log.Printf("[INFO] [Scheduler] 单主机探测完成: host=%s", hostID)
}

func (s *Scheduler) probeAll() {
	assignments, err := s.store.ListAssignments()
	if err != nil || len(assignments) == 0 {
		return
	}
	log.Printf("[INFO] [Scheduler] 开始探测 %d 个服务分配...", len(assignments))

	creds, _ := s.store.ListSSHCredentials()
	var sshCred *model.SSHCredential
	if len(creds) > 0 {
		sshCred = &creds[0]
	}

	var wg sync.WaitGroup
	type result struct {
		hostID string
		roleID string
		res    *model.ProbeResult
	}
	ch := make(chan result, len(assignments))

	for _, a := range assignments {
		host, _ := s.store.GetHost(a.HostID)
		if host == nil || host.Maintenance {
			continue
		}
		wg.Add(1)
		go func(a model.Assignment) {
			defer wg.Done()
			role, _ := s.store.GetRole(a.RoleID)
			if role == nil {
				s.store.DeleteAssignment(a.HostID, a.RoleID)
				log.Printf("[WARN] [Scheduler] 角色不存在（可能已被删除），已清理分配: host=%s role=%s", a.HostID, a.RoleID)
				return
			}
			host, _ := s.store.GetHost(a.HostID)
			if host == nil {
				return
			}
			pr := Probe(host.IP, role, &a, sshCred)
			ch <- result{hostID: a.HostID, roleID: a.RoleID, res: pr}
		}(a)
	}

	go func() { wg.Wait(); close(ch) }()

	now := time.Now()
	for r := range ch {
		s.handleProbeResult(r.hostID, r.roleID, r.res, now)
	}
	s.updateHostsSummary()
	log.Printf("[INFO] [Scheduler] 全量探测完成: 共 %d 个分配", len(assignments))
}

func (s *Scheduler) handleProbeResult(hostID, roleID string, pr *model.ProbeResult, now time.Time) {
	a, err := s.store.GetAssignment(hostID, roleID)
	if err != nil || a == nil {
		return
	}

	// 获取主机名和角色名用于日志
	hostName := hostID
	if host, _ := s.store.GetHost(hostID); host != nil {
		hostName = host.Hostname
	}
	roleName := roleID
	if role, _ := s.store.GetRole(roleID); role != nil {
		roleName = role.Name
	}
	serviceLabel := fmt.Sprintf("%s(%s)", hostName, roleName)

	if pr.Status == model.HostStatusUp {
		s.store.UpdateAssignmentConsecutiveFailures(hostID, roleID, 0)
		s.store.UpdateAssignmentStatus(hostID, roleID, model.HostStatusUp, pr.StatusCode, "", now)
		log.Printf("[INFO] [探测] %s - 成功", serviceLabel)
		return
	}

	failures := a.ConsecutiveFailures + 1
	s.store.UpdateAssignmentConsecutiveFailures(hostID, roleID, failures)

	if failures >= s.MaxRetries {
		s.store.UpdateAssignmentStatus(hostID, roleID, model.HostStatusDown, pr.StatusCode, pr.Error, now)
		log.Printf("[ERROR] [探测] %s - 失败: %s", serviceLabel, pr.Error)
	} else {
		s.store.UpdateAssignmentStatus(hostID, roleID, model.HostStatusWarning, pr.StatusCode,
			fmt.Sprintf("第%d次重试中: %s", failures, pr.Error), now)
		log.Printf("[WARN] [探测] %s - 第%d次重试: %s", serviceLabel, failures, pr.Error)
	}
}

func (s *Scheduler) updateHostsSummary() {
	hosts, _ := s.store.ListHosts()
	assignments, _ := s.store.ListAssignments()
	for _, h := range hosts {
		if h.Maintenance {
			continue
		}
		// 主机存活状态仅由 ICMP 分配决定，不受其他角色影响
		icmpStatus := model.HostStatusUnknown
		for _, a := range assignments {
			if a.HostID == h.ID && a.RoleID == "role-icmp" {
				icmpStatus = a.Status
				break
			}
		}
		s.store.UpdateHostStatus(h.ID, icmpStatus, time.Now())
	}
}

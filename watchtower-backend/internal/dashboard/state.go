package dashboard

import (
	"strings"
	"sync"

	"github.com/google/uuid"
)

// BatchHostStatus 单台主机的采集状态
type BatchHostStatus struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"` // pending / collecting / success / failed
	CPU      string `json:"cpu,omitempty"`
	Memory   string `json:"memory,omitempty"`
	Disk     string `json:"disk,omitempty"`
	Error    string `json:"error,omitempty"`
}

// BatchState 一次批量添加的整体状态
type BatchState struct {
	BatchID   string             `json:"batch_id"`
	Total     int                `json:"total"`
	Created   int                `json:"created"`
	Completed int                `json:"completed"`
	Hosts     []*BatchHostStatus `json:"hosts"`
	Done      bool               `json:"done"`
}

var (
	batchStates   = make(map[string]*BatchState)
	batchStatesMu sync.RWMutex
)

// NewBatchID 生成短 batchID
func NewBatchID() string {
	return "batch" + uuid.New().String()[:11]
}

// GetBatchState 获取批次状态
func GetBatchState(batchID string) *BatchState {
	batchStatesMu.RLock()
	defer batchStatesMu.RUnlock()
	return batchStates[batchID]
}

// SetBatchState 设置批次状态
func SetBatchState(s *BatchState) {
	batchStatesMu.Lock()
	defer batchStatesMu.Unlock()
	batchStates[s.BatchID] = s
}

// UpdateHostStatus 更新单台主机的采集状态
func UpdateHostStatus(batchID, ip, status, hostname, cpu, memory, disk, errMsg string) *BatchState {
	batchStatesMu.Lock()
	defer batchStatesMu.Unlock()
	bs, ok := batchStates[batchID]
	if !ok {
		return nil
	}
	for _, h := range bs.Hosts {
		if h.IP == ip {
			h.Status = status
			if hostname != "" {
				h.Hostname = hostname
			}
			if cpu != "" {
				h.CPU = cpu
			}
			if memory != "" {
				h.Memory = memory
			}
			if disk != "" {
				h.Disk = disk
			}
			if errMsg != "" {
				h.Error = errMsg
			}
			if status == "success" || status == "failed" {
				bs.Completed++
			}
			break
		}
	}
	if bs.Completed >= bs.Total {
		bs.Done = true
	}
	return bs
}

// SetBatchDone 标记批次完成
func SetBatchDone(batchID string) {
	batchStatesMu.Lock()
	defer batchStatesMu.Unlock()
	if bs, ok := batchStates[batchID]; ok {
		bs.Done = true
	}
}

// ParseIPs 解析用户输入的 IP 文本，支持换行和逗号分隔
func ParseIPs(input string) []string {
	var ips []string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				ips = append(ips, p)
			}
		}
	}
	return ips
}

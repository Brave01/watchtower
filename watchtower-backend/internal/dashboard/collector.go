package dashboard

import (
	"fmt"
	"log"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"watchtower/internal/model"
	"watchtower/internal/store"

	"golang.org/x/crypto/ssh"
)

const (
	collectTimeout    = 15 * time.Second
	collectCmdTimeout = 10 * time.Second
	maxConcurrent     = 5
)

// 虚拟文件系统挂载点正则：tmpfs, devtmpfs, overlay, shm, proc, sysfs, cgroup 等
var virtualFSPattern = regexp.MustCompile(`^(/dev|/sys|/proc|/run|/var/lib/docker|/var/lib/containerd|overlay|shm|tmpfs|devtmpfs|cgroup)`)

// CollectResult 采集结果
type CollectResult struct {
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
	CPU      string `json:"cpu"`
	Memory   string `json:"memory"`
	Disk     string `json:"disk"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// alignMemory 内存对齐：≤4G 向上取 2 的倍数；>4G 向上取 4 的倍数；最小为 2
func alignMemory(n int) int {
	if n <= 1 {
		return 2
	}
	if n <= 4 {
		if n%2 == 0 {
			return n
		}
		return n + 1
	}
	if n%4 == 0 {
		return n
	}
	return n + (4 - n%4)
}

// alignDisk 磁盘 size 向上取 2 的倍数
func alignDisk(n int) int {
	if n%2 == 0 {
		return n
	}
	return n + 1
}

// parseSize 解析带单位的大小字符串，如 "50G"、"200G"、"2T"，返回 GB 单位的整数
func parseSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	if strings.HasSuffix(s, "T") {
		v, err := strconv.Atoi(strings.TrimSuffix(s, "T"))
		if err != nil {
			return 0, err
		}
		return v * 1024, nil
	}
	if strings.HasSuffix(s, "G") {
		v, err := strconv.Atoi(strings.TrimSuffix(s, "G"))
		if err != nil {
			return 0, err
		}
		return v, nil
	}
	if strings.HasSuffix(s, "M") {
		v, err := strconv.Atoi(strings.TrimSuffix(s, "M"))
		if err != nil {
			return 0, err
		}
		return v / 1024, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// dialSSH 建立 SSH 连接并返回 client
func dialSSH(ip string, port int, cred *model.SSHCredential) (*ssh.Client, error) {
	if cred == nil || cred.Username == "" {
		return nil, fmt.Errorf("未配置 SSH 凭据")
	}
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	sshConfig := &ssh.ClientConfig{
		User:            cred.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         collectTimeout,
	}
	switch cred.AuthMethod {
	case "password":
		sshConfig.Auth = []ssh.AuthMethod{ssh.Password(cred.Password)}
	case "key":
		signer, err := ssh.ParsePrivateKey([]byte(cred.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("解析 SSH 密钥失败: %w", err)
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	default:
		return nil, fmt.Errorf("不支持的认证方式: %s", cred.AuthMethod)
	}
	return ssh.Dial("tcp", addr, sshConfig)
}

// runSSHCommand 在 SSH 连接上执行单条命令，返回 stdout 输出（不含 stderr）
func runSSHCommand(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建会话失败: %w", err)
	}
	defer session.Close()
	output, err := session.Output(cmd)
	if err != nil {
		return "", fmt.Errorf("命令执行失败: %w, output: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

// runSSHCommandWithSudo 执行命令，失败时自动用 sudo 重试
func runSSHCommandWithSudo(client *ssh.Client, cmd string) (string, error) {
	out, err := runSSHCommand(client, cmd)
	if err == nil {
		return out, nil
	}
	// 失败时尝试 sudo
	out, sudoErr := runSSHCommand(client, "sudo "+cmd)
	if sudoErr == nil {
		return out, nil
	}
	// 返回原始错误
	return "", err
}

// collectHost 对单台主机执行 SSH 采集，返回采集结果
func collectHost(host *model.Host, cred *model.SSHCredential) *CollectResult {
	result := &CollectResult{
		HostID:  host.ID,
		Success: false,
	}

	if cred == nil || cred.Username == "" {
		result.Error = "未配置 SSH 凭据"
		return result
	}

	// 使用凭据配置的端口，默认 22
	port := cred.Port
	if port == 0 {
		port = 22
	}

	client, err := dialSSH(host.IP, port, cred)
	if err != nil {
		result.Error = fmt.Sprintf("SSH 连接失败: %v", err)
		return result
	}
	defer client.Close()

	// 1. 采集主机名
	hostname, err := runSSHCommandWithSudo(client, "hostname -s 2>/dev/null")
	if err != nil {
		log.Printf("[Collect] %s hostname 采集失败: %v", host.IP, err)
		hostname = ""
	}

	// 2. 采集 CPU 核数
	cpuStr, err := runSSHCommandWithSudo(client, "nproc 2>/dev/null")
	cpu := ""
	if err == nil {
		cpu = strings.TrimSpace(cpuStr)
	} else {
		log.Printf("[Collect] %s CPU 采集失败: %v", host.IP, err)
	}

	// 3. 采集内存 (GB)
	memOut, err := runSSHCommandWithSudo(client, "free -g 2>/dev/null | awk 'NR==2{print $2}'")
	mem := ""
	if err == nil {
		memStr := strings.TrimSpace(memOut)
		if memInt, parseErr := strconv.Atoi(memStr); parseErr == nil {
			mem = fmt.Sprintf("%d", alignMemory(memInt))
		} else {
			log.Printf("[Collect] %s 内存解析失败: %v", host.IP, parseErr)
		}
	} else {
		log.Printf("[Collect] %s 内存采集失败: %v", host.IP, err)
	}

	// 4. 采集磁盘分区
	diskOut, err := runSSHCommandWithSudo(client, "df -h --output=target,size 2>/dev/null | tail -n +2")
	disk := ""
	if err == nil {
		var parts []string
		for _, line := range strings.Split(diskOut, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			mount := fields[0]
			sizeStr := fields[len(fields)-1]

			// 过滤虚拟文件系统
			if virtualFSPattern.MatchString(mount) {
				continue
			}
			// 只保留以 / 开头的真实挂载点
			if !strings.HasPrefix(mount, "/") {
				continue
			}

			sizeGB, parseErr := parseSize(sizeStr)
			if parseErr != nil {
				continue
			}
			sizeGB = alignDisk(sizeGB)
			parts = append(parts, mount+":"+fmt.Sprintf("%d", sizeGB))
		}
		disk = strings.Join(parts, ",")
	} else {
		log.Printf("[Collect] %s 磁盘采集失败: %v", host.IP, err)
	}

	result.Hostname = hostname
	result.CPU = cpu
	result.Memory = mem
	result.Disk = disk
	result.Success = true
	return result
}

// CollectOne 对单台主机执行 SSH 采集（导出版本）
func CollectOne(host *model.Host, cred *model.SSHCredential) *CollectResult {
	return collectHost(host, cred)
}

// CollectHosts 采集指定主机列表，支持并发限制
func CollectHosts(hosts []model.Host, st store.Store) []*CollectResult {
	limiter := make(chan struct{}, maxConcurrent)
	var mu sync.Mutex
	var results []*CollectResult
	var wg sync.WaitGroup

	for i := range hosts {
		wg.Add(1)
		limiter <- struct{}{}
		go func(h *model.Host) {
			defer wg.Done()
			defer func() { <-limiter }()

			// 优先使用主机绑定的凭据，其次取第一个凭据
			var sshCred *model.SSHCredential
			if h.SSHCredentialID != "" {
				cred, _ := st.GetSSHCredential(h.SSHCredentialID)
				if cred != nil {
					sshCred = cred
				}
			}
			if sshCred == nil {
				creds, _ := st.ListSSHCredentials()
				if len(creds) > 0 {
					sshCred = &creds[0]
				}
			}

			r := collectHost(h, sshCred)
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}(&hosts[i])
	}
	wg.Wait()
	return results
}

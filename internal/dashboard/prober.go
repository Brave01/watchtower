package dashboard

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"watchtower/internal/store"

	"golang.org/x/crypto/ssh"
)

func ProbeTCP(ip string, port int, timeout int) *store.ProbeResult {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, time.Duration(timeout)*time.Second)
	if err != nil {
		return &store.ProbeResult{Status: store.HostStatusDown, Error: err.Error()}
	}
	conn.Close()
	return &store.ProbeResult{Status: store.HostStatusUp}
}

func ProbeHTTP(ip string, port int, path string, timeout int) *store.ProbeResult {
	url := fmt.Sprintf("http://%s%s", net.JoinHostPort(ip, fmt.Sprintf("%d", port)), path)
	client := &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return &store.ProbeResult{Status: store.HostStatusDown, Error: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return &store.ProbeResult{Status: store.HostStatusUp, StatusCode: resp.StatusCode}
	}
	return &store.ProbeResult{
		Status: store.HostStatusDown, StatusCode: resp.StatusCode,
		Error: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}

func findPing() string {
	candidates := []string{"/sbin/ping", "/usr/sbin/ping", "/bin/ping", "/usr/bin/ping"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "ping"
}

func ProbeICMP(ip string, timeout int) *store.ProbeResult {
	pingPath := findPing()
	cmd := exec.Command(pingPath, "-c", "3", "-W", fmt.Sprintf("%d", timeout), ip)
	output, err := cmd.CombinedOutput()
	if err != nil {
		errMsg := strings.TrimSpace(string(output))
		if errMsg == "" {
			errMsg = err.Error()
		}
		return &store.ProbeResult{Status: store.HostStatusDown, Error: errMsg}
	}
	return &store.ProbeResult{Status: store.HostStatusUp}
}

func ProbeSSH(ip string, port int, timeout int, cred *store.SSHCredential) *store.ProbeResult {
	if cred == nil || cred.Username == "" {
		return &store.ProbeResult{Status: store.HostStatusDown, Error: "未配置 SSH 凭据"}
	}
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	sshConfig := &ssh.ClientConfig{
		User:            cred.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(timeout) * time.Second,
	}
	switch cred.AuthMethod {
	case "password":
		sshConfig.Auth = []ssh.AuthMethod{ssh.Password(cred.Password)}
	case "key":
		var signer ssh.Signer
		var err error
		if strings.Contains(cred.PrivateKey, "-----") {
			signer, err = ssh.ParsePrivateKey([]byte(cred.PrivateKey))
		} else {
			return &store.ProbeResult{Status: store.HostStatusDown, Error: "SSH 密钥格式无效"}
		}
		if err != nil {
			return &store.ProbeResult{Status: store.HostStatusDown, Error: "解析 SSH 密钥失败: " + err.Error()}
		}
		sshConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	default:
		return &store.ProbeResult{Status: store.HostStatusDown, Error: "不支持的认证方式: " + cred.AuthMethod}
	}

	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return &store.ProbeResult{Status: store.HostStatusDown, Error: "SSH 连接失败: " + err.Error()}
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return &store.ProbeResult{Status: store.HostStatusDown, Error: "SSH 会话创建失败: " + err.Error()}
	}
	defer session.Close()
	output, err := session.CombinedOutput("whoami")
	if err != nil {
		return &store.ProbeResult{Status: store.HostStatusDown, Error: "SSH 命令执行失败: " + err.Error()}
	}
	return &store.ProbeResult{
		Status:     store.HostStatusUp,
		StatusCode: 0,
		Error:      strings.TrimSpace(string(output)),
	}
}

func ResolveProbeParams(role *store.Role, assignment *store.Assignment) (port int, path string) {
	port = role.Port
	path = role.Path
	if assignment.OverridePort != nil {
		port = *assignment.OverridePort
	}
	if assignment.OverridePath != "" {
		path = assignment.OverridePath
	}
	return
}

func Probe(hostIP string, role *store.Role, assignment *store.Assignment, sshCred *store.SSHCredential) *store.ProbeResult {
	port, path := ResolveProbeParams(role, assignment)
	switch role.Type {
	case store.ProbeTypeICMP:
		return ProbeICMP(hostIP, role.Timeout)
	case store.ProbeTypeTCP:
		return ProbeTCP(hostIP, port, role.Timeout)
	case store.ProbeTypeHTTP:
		return ProbeHTTP(hostIP, port, path, role.Timeout)
	case store.ProbeTypeSSH:
		return ProbeSSH(hostIP, port, role.Timeout, sshCred)
	default:
		return &store.ProbeResult{Status: store.HostStatusDown, Error: "unknown type: " + role.Type}
	}
}

package dashboard

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"watchtower/internal/model"
	"watchtower/internal/store"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

type sshAuthInfo struct {
	Type       string `json:"type"`
	CredID     string `json:"cred_id,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
	AuthMethod string `json:"auth_method,omitempty"`
}

type sshSession struct {
	client  *ssh.Client
	session *ssh.Session
}

type sshAuthError struct{ msg string }

func (e *sshAuthError) Error() string { return e.msg }

func fmtErr(s string) error {
	return &sshAuthError{msg: s}
}

func HandleSSHWebSocket(st store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hostID := r.URL.Query().Get("host_id")
		if hostID == "" {
			http.Error(w, "missing host_id", http.StatusBadRequest)
			return
		}

		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("[SSH-WS] 升级 WebSocket 失败: %v", err)
			return
		}
		defer conn.Close()

		host, err := st.GetHost(hostID)
		if err != nil || host == nil {
			writeWS(conn, "error: 主机不存在")
			return
		}

		port := 22
		credID := r.URL.Query().Get("cred_id")
		var auth *sshAuthInfo

		if credID != "" {
			cred, err := st.GetSSHCredential(credID)
			if err != nil || cred == nil {
				log.Printf("[SSH-WS] cred_id=%s 凭据不存在: %v", credID, err)
				writeWS(conn, "error: 凭据不存在，请重新选择")
				return
			}
			if cred.Port > 0 {
				port = cred.Port
			}
			log.Printf("[SSH-WS] 使用凭据 %s, user=%s, method=%s, port=%d, host=%s", credID, cred.Username, cred.AuthMethod, port, host.IP)
			auth = &sshAuthInfo{
				Type:       "cred_id",
				CredID:     credID,
				Username:   cred.Username,
				AuthMethod: cred.AuthMethod,
				Password:   cred.Password,
				PrivateKey: cred.PrivateKey,
			}
		} else {
			// 没有cred_id时尝试URL port或自动解析
			portStr := r.URL.Query().Get("port")
			if portStr != "" {
				if p, err := strconv.Atoi(portStr); err == nil && p > 0 && p <= 65535 {
					port = p
				}
			} else {
				port = resolveSSHPort(st, hostID)
			}
			writeWS(conn, "\x1b[33m等待认证...\x1b[0m")
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var incoming sshAuthInfo
			if err := json.Unmarshal(msg, &incoming); err != nil {
				writeWS(conn, "error: 认证信息格式错误")
				return
			}
			if incoming.Type == "cred_id" && incoming.CredID != "" {
				cred, err := st.GetSSHCredential(incoming.CredID)
				if err != nil || cred == nil {
					writeWS(conn, "error: 凭据不存在")
					return
				}
				incoming.Username = cred.Username
				incoming.AuthMethod = cred.AuthMethod
				incoming.Password = cred.Password
				incoming.PrivateKey = cred.PrivateKey
			}
			if incoming.Username == "" {
				writeWS(conn, "error: 用户名不能为空")
				return
			}
			auth = &incoming
		}

		log.Printf("[SSH-WS] 正在连接 %s:%d (user=%s, method=%s)", host.IP, port, auth.Username, auth.AuthMethod)
		sshSession, err := startSSHSession(host.IP, port, auth)
		if err != nil {
			log.Printf("[SSH-WS] 连接失败: %v", err)
			writeWS(conn, "error: "+err.Error())
			return
		}
		log.Printf("[SSH-WS] 连接成功 %s:%d", host.IP, port)
		defer sshSession.client.Close()
		defer sshSession.session.Close()

		writeWS(conn, "\x1b[32m✓ SSH 连接成功！\x1b[0m")
		pipeWebSocketToSSH(conn, sshSession)
	}
}

func startSSHSession(ip string, port int, auth *sshAuthInfo) (*sshSession, error) {
	addr := net.JoinHostPort(ip, strconv.Itoa(port))

	var authMethods []ssh.AuthMethod
	switch auth.AuthMethod {
	case "password":
		if auth.Password == "" {
			return nil, fmtErr("SSH 密码不能为空")
		}
		authMethods = []ssh.AuthMethod{ssh.Password(auth.Password)}
	case "key":
		if auth.PrivateKey == "" {
			return nil, fmtErr("SSH 私钥不能为空")
		}
		signer, err := ssh.ParsePrivateKey([]byte(auth.PrivateKey))
		if err != nil {
			return nil, fmtErr("解析私钥失败: " + err.Error())
		}
		authMethods = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	default:
		return nil, fmtErr("不支持的认证方式: " + auth.AuthMethod)
	}

	config := &ssh.ClientConfig{
		User:            auth.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmtErr("SSH 连接失败: " + err.Error())
	}

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmtErr("SSH 会话创建失败: " + err.Error())
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", 40, 120, modes); err != nil {
		client.Close()
		session.Close()
		return nil, fmtErr("PTY 请求失败: " + err.Error())
	}

	return &sshSession{client: client, session: session}, nil
}

func pipeWebSocketToSSH(conn *websocket.Conn, s *sshSession) {
	stdout, _ := s.session.StdoutPipe()
	stdin, _ := s.session.StdinPipe()

	if err := s.session.Shell(); err != nil {
		writeWS(conn, "启动 Shell 失败: "+err.Error())
		return
	}

	done := make(chan struct{})

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()

	go func() {
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				break
			}
			var resize struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if json.Unmarshal(message, &resize) == nil && resize.Type == "resize" && resize.Cols > 0 && resize.Rows > 0 {
				s.session.WindowChange(resize.Rows, resize.Cols)
				continue
			}
			stdin.Write(message)
		}
		stdin.Close()
	}()

	s.session.Wait()
	<-done
}

func resolveSSHPort(st store.Store, hostID string) int {
	port := 22
	assignments, err := st.ListAssignments()
	if err != nil {
		return port
	}
	for _, a := range assignments {
		if a.HostID != hostID {
			continue
		}
		role, err := st.GetRole(a.RoleID)
		if err != nil || role == nil {
			continue
		}
		if role.Type == model.ProbeTypeSSH {
			if a.OverridePort != nil {
				port = *a.OverridePort
			} else {
				port = role.Port
			}
			break
		}
	}
	return port
}

func writeWS(conn *websocket.Conn, msg string) {
	conn.WriteMessage(websocket.TextMessage, []byte(msg+"\r\n"))
}

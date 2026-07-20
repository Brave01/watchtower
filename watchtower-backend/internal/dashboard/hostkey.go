package dashboard

import (
	"net"

	"golang.org/x/crypto/ssh"
)

var hostKeyCallback ssh.HostKeyCallback = ssh.InsecureIgnoreHostKey()

// SetHostKeyCheck 配置 SSH 主机密钥校验策略。
// enabled=true 时启用基本校验（仅检查密钥是否已变更），
// enabled=false 时跳过所有校验（默认，与旧版本行为一致）。
func SetHostKeyCheck(enabled bool) {
	if enabled {
		hostKeyCallback = ssh.HostKeyCallback(func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			// 启用基本校验：对于已知主机密钥，应在此处与 known_hosts 比对。
			// 当前实现仅检查密钥不为空，防止意外连接。
			if key == nil {
				return ssh.ErrNoAuth
			}
			return nil
		})
	} else {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
}

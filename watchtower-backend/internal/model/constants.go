package model

import "encoding/json"

const (
	HostStatusUnknown = 0
	HostStatusUp      = 1
	HostStatusDown    = 2
	HostStatusWarning = 3
	HostStatusMuted   = 4
)

const (
	ProbeTypeICMP = "ICMP"
	ProbeTypeTCP  = "TCP"
	ProbeTypeHTTP = "HTTP"
	ProbeTypeSSH  = "SSH"
)

const RoleIDICMP = "role-icmp"

func JoinStrings(items []string) string {
	data, _ := json.Marshal(items)
	return string(data)
}
func SplitStrings(s string) []string {
	var items []string
	if s == "" {
		return items
	}
	json.Unmarshal([]byte(s), &items)
	return items
}
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
func IntToBool(i int) bool {
	return i != 0
}

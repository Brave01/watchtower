package webui

import "embed"

// StaticFS 内嵌两个服务共用的登录页、导航脚本与主题样式。
// 各服务的 main.go 通过 http.FileServer(http.FS(StaticFS)) 或直接读取具体文件对外暴露。
//
//go:embed static
var StaticFS embed.FS

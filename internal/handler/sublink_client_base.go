package handler

import "strings"

// Shadowrocket / Loon 客户端配置的共享基础定义。

// clientFormat 标识目标客户端。
type clientFormat int

const (
	formatShadowrocket clientFormat = iota
	formatLoon
)

// clientConfigMode 节点处理方式。
type clientConfigMode int

const (
	modeEmbedded  clientConfigMode = iota // 节点写在配置里
	modeSeparated                         // 节点由订阅动态提供
)

// clientTestURL 默认测速地址。
const clientTestURL = "http://www.gstatic.com/generate_204"

// sanitizeClientName 清理节点名：逗号破坏字段分隔，等号破坏 key/value，
// # 是注释起始，都必须去掉。
func sanitizeClientName(name string) string {
	r := strings.NewReplacer(",", " ", "=", "-", "#", "", "\n", " ", "\r", "")
	out := strings.TrimSpace(r.Replace(name))
	if out == "" {
		return "node"
	}
	return out
}

// loonQuote 给 Loon 的字符串字段加引号。
func loonQuote(v string) string { return `"` + strings.ReplaceAll(v, `"`, "") + `"` }

// boolStr 输出 true/false。
func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

package handler

import (
	"fmt"
	"strings"

	"github.com/sboard-go/sboard/internal/database"
)

// Surge 客户端配置生成。
//
// Surge 的节点行与 Shadowrocket 形似但字段不同，容易踩的差异：
//
//	Shadowsocks   用 encrypt-method，不是 method
//	VMess         用 username=<UUID>，没有 alterId
//	TUIC          必须用 tuic-v5 关键字，配 uuid + password 两个字段
//	Hysteria 2    用 password，不是 auth
//	SOCKS5        用户名密码是位置参数，且关键字是 socks5 不是 socks
//	TLS 的 SNI     用 sni，不是 peer
//	WS 头         多个头用竖线分隔 Host:a|X-b:c
//
// 不支持的协议与传输直接跳过该节点，不生成半残的配置行——
// Surge 遇到无法解析的 [Proxy] 行会整条丢弃并告警：
//
//	VLESS         Surge 官方协议表里没有 VLESS
//	naiveproxy    从未支持
//	Reality       无此能力
//	gRPC / http   Surge 只有 ws 传输
//
// 因此 Surge 用户拿到的节点数可能少于其他客户端，这是协议能力差异，不是漏节点。

// surgeTLS 追加 TLS 相关字段。tlsFlag=false 用于 trojan / hysteria2 / tuic /
// anytls 这类本身就跑在 TLS 上的协议——它们不接受也不需要在行内写 tls=true。
func surgeTLS(kv *[]string, node *database.InboundNode, tlsFlag bool) {
	if tlsFlag {
		if !node.TlsEnabled {
			return
		}
		*kv = append(*kv, "tls=true")
	}
	if node.ServerName != "" {
		*kv = append(*kv, "sni="+node.ServerName)
	}
	// 本项目使用伪装证书，客户端需跳过校验
	*kv = append(*kv, "skip-cert-verify=true")
}

// surgeWS 追加 ws 传输字段。返回 false 表示该传输方式 Surge 不支持。
func surgeWS(kv *[]string, enabled bool, transportType, wsPath, transportHost string) bool {
	if !enabled || transportType == "" || transportType == "tcp" {
		return true
	}
	if transportType != "ws" && transportType != "websocket" {
		return false
	}
	*kv = append(*kv, "ws=true")
	path := wsPath
	if path == "" {
		path = "/"
	}
	*kv = append(*kv, "ws-path="+path)
	if transportHost != "" {
		*kv = append(*kv, "ws-headers=Host:"+transportHost)
	}
	return true
}

// buildSurgeProxyLine 构建 Surge 节点行。返回空串表示 Surge 不支持该节点。
func buildSurgeProxyLine(name, host string, port int, node *database.InboundNode, user *database.ProxyUser, uuid string) string {
	if node.RealityEnabled {
		return ""
	}

	var kv []string
	var head string

	switch node.Protocol {
	case "shadowsocks":
		method := node.SsMethod
		if method == "" {
			method = "aes-256-gcm"
		}
		head = fmt.Sprintf("ss, %s, %d, encrypt-method=%s, password=%s",
			host, port, method, getSS2022Password(node, uuid))

	case "vmess":
		head = fmt.Sprintf("vmess, %s, %d, username=%s", host, port, uuid)
		if !surgeWS(&kv, node.TransportEnabled, node.TransportType, node.WsPath, node.TransportHost) {
			return ""
		}
		surgeTLS(&kv, node, true)

	case "trojan":
		head = fmt.Sprintf("trojan, %s, %d, password=%s", host, port, uuid)
		if !surgeWS(&kv, node.TransportEnabled, node.TransportType, node.WsPath, node.TransportHost) {
			return ""
		}
		// trojan 天然走 TLS，行内不再写 tls=true
		surgeTLS(&kv, node, false)

	case "hysteria2":
		pwd := node.Hy2Password
		if pwd == "" {
			pwd = uuid
		}
		head = fmt.Sprintf("hysteria2, %s, %d, password=%s", host, port, pwd)
		surgeTLS(&kv, node, false)
		if node.Hy2Obfs != "" && node.Hy2ObfsPassword != "" {
			kv = append(kv, "obfs-password="+node.Hy2ObfsPassword)
		}

	case "tuic":
		pwd := node.TuicPassword
		if pwd == "" {
			pwd = uuid
		}
		head = fmt.Sprintf("tuic-v5, %s, %d, uuid=%s, password=%s", host, port, uuid, pwd)
		surgeTLS(&kv, node, false)
		kv = append(kv, "alpn=h3")

	case "anytls":
		head = fmt.Sprintf("anytls, %s, %d, password=%s", host, port, uuid)
		surgeTLS(&kv, node, false)

	case "socks":
		// 用户名密码是位置参数，不写成 key=value
		head = fmt.Sprintf("socks5, %s, %d, %s, %s", host, port, user.Name, uuid)

	case "vless":
		// Surge 官方协议表没有 VLESS
		return ""

	case "naive":
		return ""

	default:
		return ""
	}

	if len(kv) > 0 {
		return fmt.Sprintf("%s = %s, %s", name, head, strings.Join(kv, ", "))
	}
	return fmt.Sprintf("%s = %s", name, head)
}

// buildSurgeProxyLineFromExternal 外部节点的 Surge 行。
func buildSurgeProxyLineFromExternal(name, host string, ext *database.ExternalNode) string {
	if ext.RealityEnabled {
		return ""
	}

	var kv []string
	var head string

	switch ext.Protocol {
	case "shadowsocks":
		method := ext.SsMethod
		if method == "" {
			method = "aes-256-gcm"
		}
		pwd := ext.SsPassword
		if pwd == "" {
			pwd = ext.UUID
		}
		head = fmt.Sprintf("ss, %s, %d, encrypt-method=%s, password=%s", host, ext.Port, method, pwd)

	case "vmess":
		head = fmt.Sprintf("vmess, %s, %d, username=%s", host, ext.Port, ext.UUID)
		if !surgeWS(&kv, ext.TransportEnabled, ext.TransportType, ext.WsPath, ext.TransportHost) {
			return ""
		}
		if ext.TlsEnabled {
			kv = append(kv, "tls=true")
		}
		if ext.ServerName != "" {
			kv = append(kv, "sni="+ext.ServerName)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "trojan":
		head = fmt.Sprintf("trojan, %s, %d, password=%s", host, ext.Port, ext.UUID)
		if !surgeWS(&kv, ext.TransportEnabled, ext.TransportType, ext.WsPath, ext.TransportHost) {
			return ""
		}
		if ext.ServerName != "" {
			kv = append(kv, "sni="+ext.ServerName)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "hysteria2":
		pwd := ext.Hy2Password
		if pwd == "" {
			pwd = ext.UUID
		}
		head = fmt.Sprintf("hysteria2, %s, %d, password=%s", host, ext.Port, pwd)
		if ext.ServerName != "" {
			kv = append(kv, "sni="+ext.ServerName)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "tuic":
		pwd := ext.TuicPassword
		if pwd == "" {
			pwd = ext.UUID
		}
		head = fmt.Sprintf("tuic-v5, %s, %d, uuid=%s, password=%s", host, ext.Port, ext.UUID, pwd)
		if ext.ServerName != "" {
			kv = append(kv, "sni="+ext.ServerName)
		}
		kv = append(kv, "skip-cert-verify=true", "alpn=h3")

	case "anytls":
		head = fmt.Sprintf("anytls, %s, %d, password=%s", host, ext.Port, ext.UUID)
		if ext.ServerName != "" {
			kv = append(kv, "sni="+ext.ServerName)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "socks":
		// 与 Shadowrocket 一致：用户名为 UUID，密码取 Password
		head = fmt.Sprintf("socks5, %s, %d, %s, %s", host, ext.Port, ext.UUID, ext.Password)

	case "vless", "naive":
		return ""

	default:
		return ""
	}

	if len(kv) > 0 {
		return fmt.Sprintf("%s = %s, %s", name, head, strings.Join(kv, ", "))
	}
	return fmt.Sprintf("%s = %s", name, head)
}

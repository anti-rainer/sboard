package handler

import (
	"fmt"
	"strings"

	"github.com/sboard-go/sboard/internal/database"
)

// Shadowrocket 的节点行格式：name = type, host, port, key=value...
// Loon 的节点行格式：name = Type, host, port, <位置参数>, key=value...

// buildShadowrocketProxyLine 构建 Shadowrocket 节点行。
func buildShadowrocketProxyLine(name, host string, port int, node *database.InboundNode, user *database.ProxyUser, uuid string) string {
	tls := node.TlsEnabled || node.RealityEnabled
	sni := node.ServerName
	var kv []string

	// TLS 相关通用字段
	addTLS := func() {
		if tls {
			kv = append(kv, "tls=true")
			if sni != "" {
				kv = append(kv, "peer="+sni)
			}
			// 本项目使用伪装证书，客户端需跳过校验
			kv = append(kv, "skip-cert-verify=true")
		}
	}
	addTransport := func() {
		if !node.TransportEnabled || node.TransportType == "" || node.TransportType == "tcp" {
			return
		}
		switch node.TransportType {
		case "ws", "websocket":
			kv = append(kv, "ws=true")
			path := node.WsPath
			if path == "" {
				path = "/"
			}
			kv = append(kv, "ws-path="+path)
			if node.TransportHost != "" {
				kv = append(kv, "ws-headers=Host:"+node.TransportHost)
			}
		case "grpc":
			kv = append(kv, "grpc=true")
			svc := node.GrpcService
			if svc == "" {
				svc = "grpc"
			}
			kv = append(kv, "grpc-service-name="+svc)
		case "http", "h2":
			kv = append(kv, "obfs=http")
			host2 := node.TransportHost
			if host2 != "" {
				kv = append(kv, "obfs-host="+host2)
			}
			path := node.WsPath
			if path == "" {
				path = "/"
			}
			kv = append(kv, "obfs-uri="+path)
		}
	}

	var head string
	switch node.Protocol {
	case "shadowsocks":
		method := node.SsMethod
		if method == "" {
			method = "aes-256-gcm"
		}
		head = fmt.Sprintf("ss, %s, %d, password=%s, method=%s", host, port, getSS2022Password(node, uuid), method)

	case "vmess":
		head = fmt.Sprintf("vmess, %s, %d, password=%s", host, port, uuid)
		addTransport()
		addTLS()
		// VMess 的加密方式在 Shadowrocket 里可选，默认 auto
		kv = append(kv, "alterId=0")

	case "vless":
		head = fmt.Sprintf("vless, %s, %d, password=%s", host, port, uuid)
		if node.Flow != "" {
			kv = append(kv, "flow="+node.Flow)
		}
		addTransport()
		addTLS()
		if node.RealityEnabled {
			kv = append(kv, "reality=true")
			if node.RealityPubkey != "" {
				kv = append(kv, "public-key="+node.RealityPubkey)
			}
			if node.RealityShortId != "" {
				kv = append(kv, "short-id="+node.RealityShortId)
			}
		}

	case "trojan":
		head = fmt.Sprintf("trojan, %s, %d, password=%s", host, port, uuid)
		addTransport()
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "hysteria2":
		pwd := node.Hy2Password
		if pwd == "" {
			pwd = uuid
		}
		head = fmt.Sprintf("hysteria2, %s, %d, auth=%s", host, port, pwd)
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true", "udp=1")
		if node.Hy2Obfs != "" && node.Hy2ObfsPassword != "" {
			kv = append(kv, "obfs=salamander", "obfs-password="+node.Hy2ObfsPassword)
		}

	case "tuic":
		pwd := node.TuicPassword
		if pwd == "" {
			pwd = uuid
		}
		congestion := node.TuicCongestion
		if congestion == "" {
			congestion = "bbr"
		}
		head = fmt.Sprintf("tuic, %s, %d, password=%s:%s", host, port, uuid, pwd)
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true", "udp=1", "congestion-controller="+congestion, "udp-relay-mode=native", "alpn=h3")

	case "anytls":
		head = fmt.Sprintf("anytls, %s, %d, password=%s", host, port, uuid)
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "socks":
		head = fmt.Sprintf("socks, %s, %d, %s:%s", host, port, user.Name, uuid)

	case "naive":
		// Shadowrocket 不支持 naiveproxy
		return ""

	default:
		return ""
	}

	if len(kv) > 0 {
		return fmt.Sprintf("%s = %s, %s", name, head, strings.Join(kv, ", "))
	}
	return fmt.Sprintf("%s = %s", name, head)
}

// buildShadowrocketProxyLineFromExternal 外部节点的 Shadowrocket 行。
func buildShadowrocketProxyLineFromExternal(name, host string, ext *database.ExternalNode) string {
	tls := ext.TlsEnabled || ext.RealityEnabled
	sni := ext.ServerName
	var kv []string

	addTLS := func() {
		if tls {
			kv = append(kv, "tls=true")
			if sni != "" {
				kv = append(kv, "peer="+sni)
			}
			kv = append(kv, "skip-cert-verify=true")
		}
	}
	addTransport := func() {
		if !ext.TransportEnabled || ext.TransportType == "" || ext.TransportType == "tcp" {
			return
		}
		switch ext.TransportType {
		case "ws", "websocket":
			kv = append(kv, "ws=true")
			path := ext.WsPath
			if path == "" {
				path = "/"
			}
			kv = append(kv, "ws-path="+path)
			if ext.TransportHost != "" {
				kv = append(kv, "ws-headers=Host:"+ext.TransportHost)
			}
		case "grpc":
			kv = append(kv, "grpc=true")
			svc := ext.GrpcService
			if svc == "" {
				svc = "grpc"
			}
			kv = append(kv, "grpc-service-name="+svc)
		}
	}

	var head string
	switch ext.Protocol {
	case "shadowsocks":
		method := ext.SsMethod
		if method == "" {
			method = "aes-256-gcm"
		}
		head = fmt.Sprintf("ss, %s, %d, password=%s, method=%s", host, ext.Port, ext.SsPassword, method)

	case "vmess":
		head = fmt.Sprintf("vmess, %s, %d, password=%s", host, ext.Port, ext.UUID)
		addTransport()
		addTLS()
		kv = append(kv, "alterId=0")

	case "vless":
		head = fmt.Sprintf("vless, %s, %d, password=%s", host, ext.Port, ext.UUID)
		if ext.Flow != "" {
			kv = append(kv, "flow="+ext.Flow)
		}
		addTransport()
		addTLS()

	case "trojan":
		pwd := ext.Password
		if pwd == "" {
			pwd = ext.UUID
		}
		head = fmt.Sprintf("trojan, %s, %d, password=%s", host, ext.Port, pwd)
		addTransport()
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "hysteria2":
		pwd := ext.Hy2Password
		if pwd == "" {
			pwd = ext.Password
		}
		head = fmt.Sprintf("hysteria2, %s, %d, auth=%s", host, ext.Port, pwd)
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true", "udp=1")
		if ext.Hy2Obfs != "" && ext.Hy2ObfsPassword != "" {
			kv = append(kv, "obfs=salamander", "obfs-password="+ext.Hy2ObfsPassword)
		}

	case "tuic":
		pwd := ext.TuicPassword
		if pwd == "" {
			pwd = ext.Password
		}
		congestion := ext.TuicCongestion
		if congestion == "" {
			congestion = "bbr"
		}
		head = fmt.Sprintf("tuic, %s, %d, password=%s:%s", host, ext.Port, ext.UUID, pwd)
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true", "udp=1", "congestion-controller="+congestion, "udp-relay-mode=native", "alpn=h3")

	case "anytls":
		pwd := ext.Password
		if pwd == "" {
			pwd = ext.UUID
		}
		head = fmt.Sprintf("anytls, %s, %d, password=%s", host, ext.Port, pwd)
		if sni != "" {
			kv = append(kv, "peer="+sni)
		}
		kv = append(kv, "skip-cert-verify=true")

	case "socks":
		uname := ext.UUID
		head = fmt.Sprintf("socks, %s, %d, %s:%s", host, ext.Port, uname, ext.Password)

	default:
		return ""
	}

	if len(kv) > 0 {
		return fmt.Sprintf("%s = %s, %s", name, head, strings.Join(kv, ", "))
	}
	return fmt.Sprintf("%s = %s", name, head)
}

// buildLoonProxyLine 构建 Loon 节点行。
func buildLoonProxyLine(name, host string, port int, node *database.InboundNode, user *database.ProxyUser, uuid string) string {
	tls := node.TlsEnabled || node.RealityEnabled
	sni := node.ServerName

	addTransport := func() []string {
		if !node.TransportEnabled || node.TransportType == "" || node.TransportType == "tcp" {
			return nil
		}
		var parts []string
		switch node.TransportType {
		case "ws", "websocket":
			parts = append(parts, "transport=ws")
			path := node.WsPath
			if path == "" {
				path = "/"
			}
			parts = append(parts, "path="+path)
			if node.TransportHost != "" {
				parts = append(parts, "host="+node.TransportHost)
			}
		case "grpc":
			parts = append(parts, "transport=grpc")
			svc := node.GrpcService
			if svc == "" {
				svc = "grpc"
			}
			parts = append(parts, "servicename="+svc)
		case "http", "h2":
			parts = append(parts, "transport=http")
			path := node.WsPath
			if path == "" {
				path = "/"
			}
			parts = append(parts, "path="+path)
			if node.TransportHost != "" {
				parts = append(parts, "host="+node.TransportHost)
			}
		}
		return parts
	}

	var parts []string

	switch node.Protocol {
	case "shadowsocks":
		method := node.SsMethod
		if method == "" {
			method = "aes-256-gcm"
		}
		parts = []string{"Shadowsocks", host, fmt.Sprint(port), method, loonQuote(getSS2022Password(node, uuid))}
		parts = append(parts, "udp=true")

	case "vmess":
		parts = []string{"vmess", host, fmt.Sprint(port), "auto", loonQuote(uuid)}
		parts = append(parts, addTransport()...)
		parts = append(parts, "over-tls="+boolStr(tls))
		if tls && sni != "" {
			parts = append(parts, "tls-name="+sni, "skip-cert-verify=true")
		}

	case "vless":
		parts = []string{"VLESS", host, fmt.Sprint(port), loonQuote(uuid)}
		if node.Flow != "" {
			parts = append(parts, "flow="+node.Flow)
		}
		parts = append(parts, addTransport()...)
		if sni != "" {
			parts = append(parts, "tls-name="+sni)
		}
		parts = append(parts, "skip-cert-verify=true")

	case "trojan":
		parts = []string{"trojan", host, fmt.Sprint(port), loonQuote(uuid)}
		parts = append(parts, addTransport()...)
		if sni != "" {
			parts = append(parts, "tls-name="+sni)
		}
		parts = append(parts, "skip-cert-verify=true", "udp=true")

	case "hysteria2":
		pwd := node.Hy2Password
		if pwd == "" {
			pwd = uuid
		}
		parts = []string{"Hysteria2", host, fmt.Sprint(port), loonQuote(pwd)}
		if sni != "" {
			parts = append(parts, "sni="+sni)
		}
		parts = append(parts, "skip-cert-verify=true")
		if node.Hy2Obfs != "" && node.Hy2ObfsPassword != "" {
			parts = append(parts, "obfs=salamander", "obfs-password="+loonQuote(node.Hy2ObfsPassword))
		}

	case "tuic":
		pwd := node.TuicPassword
		if pwd == "" {
			pwd = uuid
		}
		parts = []string{"TUIC", host, fmt.Sprint(port), loonQuote(uuid), loonQuote(pwd)}
		if sni != "" {
			parts = append(parts, "sni="+sni)
		}
		congestion := node.TuicCongestion
		if congestion == "" {
			congestion = "bbr"
		}
		parts = append(parts, "skip-cert-verify=true", "congestion-control="+congestion, "udp-relay-mode=native")

	case "anytls":
		parts = []string{"anytls", host, fmt.Sprint(port), loonQuote(uuid)}
		if sni != "" {
			parts = append(parts, "sni="+sni)
		}
		parts = append(parts, "skip-cert-verify=true")

	case "socks":
		parts = []string{"Socks5", host, fmt.Sprint(port), user.Name, loonQuote(uuid)}

	case "naive":
		// Loon 不支持 naiveproxy
		return ""

	default:
		return ""
	}

	return fmt.Sprintf("%s = %s", name, strings.Join(parts, ","))
}

// buildLoonProxyLineFromExternal 外部节点的 Loon 行。
func buildLoonProxyLineFromExternal(name, host string, ext *database.ExternalNode) string {
	tls := ext.TlsEnabled || ext.RealityEnabled
	sni := ext.ServerName

	addTransport := func() []string {
		if !ext.TransportEnabled || ext.TransportType == "" || ext.TransportType == "tcp" {
			return nil
		}
		var parts []string
		switch ext.TransportType {
		case "ws", "websocket":
			parts = append(parts, "transport=ws")
			path := ext.WsPath
			if path == "" {
				path = "/"
			}
			parts = append(parts, "path="+path)
			if ext.TransportHost != "" {
				parts = append(parts, "host="+ext.TransportHost)
			}
		case "grpc":
			parts = append(parts, "transport=grpc")
			svc := ext.GrpcService
			if svc == "" {
				svc = "grpc"
			}
			parts = append(parts, "servicename="+svc)
		}
		return parts
	}

	var parts []string

	switch ext.Protocol {
	case "shadowsocks":
		method := ext.SsMethod
		if method == "" {
			method = "aes-256-gcm"
		}
		parts = []string{"Shadowsocks", host, fmt.Sprint(ext.Port), method, loonQuote(ext.SsPassword)}
		parts = append(parts, "udp=true")

	case "vmess":
		parts = []string{"vmess", host, fmt.Sprint(ext.Port), "auto", loonQuote(ext.UUID)}
		parts = append(parts, addTransport()...)
		parts = append(parts, "over-tls="+boolStr(tls))
		if tls && sni != "" {
			parts = append(parts, "tls-name="+sni, "skip-cert-verify=true")
		}

	case "vless":
		parts = []string{"VLESS", host, fmt.Sprint(ext.Port), loonQuote(ext.UUID)}
		if ext.Flow != "" {
			parts = append(parts, "flow="+ext.Flow)
		}
		parts = append(parts, addTransport()...)
		if sni != "" {
			parts = append(parts, "tls-name="+sni)
		}
		parts = append(parts, "skip-cert-verify=true")

	case "trojan":
		pwd := ext.Password
		if pwd == "" {
			pwd = ext.UUID
		}
		parts = []string{"trojan", host, fmt.Sprint(ext.Port), loonQuote(pwd)}
		parts = append(parts, addTransport()...)
		if sni != "" {
			parts = append(parts, "tls-name="+sni)
		}
		parts = append(parts, "skip-cert-verify=true", "udp=true")

	case "hysteria2":
		pwd := ext.Hy2Password
		if pwd == "" {
			pwd = ext.Password
		}
		parts = []string{"Hysteria2", host, fmt.Sprint(ext.Port), loonQuote(pwd)}
		if sni != "" {
			parts = append(parts, "sni="+sni)
		}
		parts = append(parts, "skip-cert-verify=true")
		if ext.Hy2Obfs != "" && ext.Hy2ObfsPassword != "" {
			parts = append(parts, "obfs=salamander", "obfs-password="+loonQuote(ext.Hy2ObfsPassword))
		}

	case "tuic":
		pwd := ext.TuicPassword
		if pwd == "" {
			pwd = ext.Password
		}
		congestion := ext.TuicCongestion
		if congestion == "" {
			congestion = "bbr"
		}
		parts = []string{"TUIC", host, fmt.Sprint(ext.Port), loonQuote(ext.UUID), loonQuote(pwd)}
		if sni != "" {
			parts = append(parts, "sni="+sni)
		}
		parts = append(parts, "skip-cert-verify=true", "congestion-control="+congestion, "udp-relay-mode=native")

	case "anytls":
		pwd := ext.Password
		if pwd == "" {
			pwd = ext.UUID
		}
		parts = []string{"anytls", host, fmt.Sprint(ext.Port), loonQuote(pwd)}
		if sni != "" {
			parts = append(parts, "sni="+sni)
		}
		parts = append(parts, "skip-cert-verify=true")

	case "socks":
		parts = []string{"Socks5", host, fmt.Sprint(ext.Port), ext.UUID, loonQuote(ext.Password)}

	default:
		return ""
	}

	return fmt.Sprintf("%s = %s", name, strings.Join(parts, ","))
}

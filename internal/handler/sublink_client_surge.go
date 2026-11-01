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
		// Shadowsocks 的 UDP 转发不是自动的，必须显式开启。
		// 不开的话 VoWiFi 的 IKE/NAT-T 走 UDP 会被 REJECT。
		kv = append(kv, "udp-relay=true")

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
		// 同 Shadowsocks：SOCKS5 的 UDP 转发必须显式开启
		kv = append(kv, "udp-relay=true")

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
		kv = append(kv, "udp-relay=true")

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
		kv = append(kv, "udp-relay=true")

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


// generateSurgeNodeList 生成供策略组 policy-path 引用的纯节点清单。
//
// Surge 文档对 policy-path 资源有两种接受形式：
//   - 一份完整的 Surge 配置（取其中的 [Proxy] 段）
//   - 一份 policy 列表（每行一条，格式与 [Proxy] 段相同）
//
// 这里用后者，不带段头，最不容易产生歧义。
//
// 配置与节点分离后，节点增删、等级调整只影响本清单，
// 用户不需要重新导入配置文件，Surge 按 update-interval 自动拉取。
func generateSurgeNodeList(servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (string, error) {
	nodes := buildAllClientNodes(formatSurge, servers, nodeConfigs, user, lv, externalNodes)

	var b strings.Builder
	if len(nodes) == 0 {
		// 空清单会让 Surge 的策略组变成空组，这里至少留一条注释说明原因
		b.WriteString("# 暂无 Surge 可用的节点（VLESS / Reality / gRPC 不被支持）\n")
		return b.String(), nil
	}
	for _, n := range nodes {
		b.WriteString(n.Line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// surgeInternetTestURL 是 Surge 的 internet-test-url，同时也是 DIRECT
// 策略的测速地址，必须选境内可达的，否则 DIRECT 永远显示失败。
const surgeInternetTestURL = "http://www.baidu.com"

// surgeProviderInterval policy-path 的资源刷新间隔（秒）。
const surgeProviderInterval = 86400

// buildSurgeGroupsWithProvider 用 policy-path 从订阅拉节点来构造策略组。
//
// Surge 与 Shadowrocket / Loon 最大的不同就在这里：它允许策略组从外部 URL
// 导入节点，所以配置里不必内嵌节点，首页/策略组里的节点能随订阅自动更新。
//
// 每个组这样写：
//
//	国外 = select, policy-path=<订阅地址>, update-interval=86400, policy-regex-filter=<地区正则>
//
// policy-regex-filter 只作用于 policy-path / include-all-proxies 导入的成员，
// 不作用于显式列出的成员——所以国内的 DIRECT 直接列在组里即可。
//
// 若某组按当前节点算出来是空的，就不挂 policy-path，退化成只有 DIRECT，
// 避免生成空组：Surge 遇到没有任何成员的组会整份配置报错。
func buildSurgeGroupsWithProvider(cfg *MihomoSubscriptionConfig, providerURL string, allNodes []string, countryNodes map[string][]string) []string {
	if cfg == nil {
		return nil
	}

	var out []string
	for _, pg := range cfg.Config.ProxyGroups {
		name := sanitizeClientName(pg.Name)
		if name == "" {
			continue
		}

		// 成员由 Surge 在运行时按 policy-regex-filter 从订阅里筛，
		// 这里不预计算成员列表。
		groupType := "select"
		if pg.Type == "url-test" || pg.Type == "fallback" {
			groupType = pg.Type
		}

		parts := []string{groupType}

		// 国内组显式列 DIRECT，让用户能选直连
		isCN := pg.FilterMode == "geoip-cn" && strings.TrimSpace(pg.Filter) == "cn"
		if isCN {
			parts = append(parts, "DIRECT")
		}

		// 生成期先按当前节点算一遍成员：Surge 遇到没有成员的策略组会整份
		// 配置报错，所以筛不出节点时退化成只有 DIRECT 的选择组。
		// 成员本身由 Surge 在运行时按 policy-regex-filter 重新筛，
		// 这里只用作「会不会是空组」的判断。
		currentMembers := resolveGroupMembers(pg, allNodes, countryNodes)

		filter := groupNameRegex(pg, currentMembers)

		// 只有在「组里没有任何显式成员、且当前筛不出节点」时才退化。
		// 国内组显式带 DIRECT，永远不会是空组，所以要保留 policy-path——
		// 否则以后出现国内节点时不会自动收进来，得重新生成配置。
		emptyRisk := !isCN && len(currentMembers) == 0
		if providerURL == "" || filter == "" || emptyRisk {
			if !isCN {
				parts = append(parts, "DIRECT")
			}
			out = append(out, fmt.Sprintf("%s = %s", name, strings.Join(parts, ", ")))
			continue
		}

		parts = append(parts,
			"policy-path="+providerURL,
			fmt.Sprintf("update-interval=%d", surgeProviderInterval),
			"policy-regex-filter="+filter,
		)

		if groupType == "url-test" || groupType == "fallback" {
			url := pg.URL
			if url == "" {
				url = clientTestURL
			}
			interval := pg.Interval
			if interval <= 0 {
				interval = 600
			}
			tolerance := pg.Tolerance
			if tolerance <= 0 {
				tolerance = 50
			}
			parts = append(parts,
				"url="+url,
				fmt.Sprintf("interval=%d", interval),
				fmt.Sprintf("tolerance=%d", tolerance),
			)
		}

		out = append(out, fmt.Sprintf("%s = %s", name, strings.Join(parts, ", ")))
	}
	return out
}

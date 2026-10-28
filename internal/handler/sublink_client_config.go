package handler

import (
	"fmt"
	"strings"

	"github.com/sboard-go/sboard/internal/database"
)

// Shadowrocket / Loon 完整配置生成。
//
// 两者都是 INI 风格的 .conf：
//
//	[General]       全局设置（DNS、bypass、ipv6、update-url）
//	[Proxy]         节点定义
//	[Proxy Group]   策略组（select / url-test / fallback / load-balance）
//	[Rule]          分流规则
//
// 差异点：
//   - Loon 的节点行用逗号分隔的位置参数（Shadowsocks,host,port,method,"pwd"）
//   - Shadowrocket 用 key=value（ss, host, port, password=pwd, method=aes-256-gcm）
//   - Loon 支持 [Remote Rule] 段；Shadowrocket 统一用 RULE-SET,<url>,<policy>
//   - 协议名大小写与写法不同（Loon: Shadowsocks/vmess/VLESS/trojan/Hysteria2/TUIC
//     Shadowrocket: ss/vmess/vless/trojan/hysteria2/tuic/anytls）
//
// 策略组和规则都从面板的 mihomo 配置模板读取（策略组定义、规则列表是同一套），
// 这样三端订阅的行为一致，改一处即可全部生效。

// clientFormat 标识目标客户端。
type clientFormat int

const (
	formatShadowrocket clientFormat = iota
	formatLoon
)

const (
	// 默认测速地址
	clientTestURL = "http://www.gstatic.com/generate_204"

	// blackmatrix7 的规则集仓库，Shadowrocket 与 Loon 都能直接吃
	ruleSetBase = "https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule"
)

// ruleSetURLs 把面板里的规则集名映射到 Shadowrocket / Loon 可用的远程 url。
// key 为面板规则里 RULE-SET 的 value。
var ruleSetURLs = map[string]struct{ sr, loon string }{
	"AI": {
		sr:   ruleSetBase + "/Shadowrocket/OpenAI/OpenAI.list",
		loon: ruleSetBase + "/Loon/OpenAI/OpenAI.list",
	},
	"proxy_domain": {
		sr:   ruleSetBase + "/Shadowrocket/Global/Global.list",
		loon: ruleSetBase + "/Loon/Global/Global.list",
	},
	"cn_ip": {
		sr:   ruleSetBase + "/Shadowrocket/ChinaIPs/ChinaIPs.list",
		loon: ruleSetBase + "/Loon/ChinaIPs/ChinaIPs.list",
	},
	"cn_domain": {
		sr:   ruleSetBase + "/Shadowrocket/ChinaMax/ChinaMax.list",
		loon: ruleSetBase + "/Loon/ChinaMax/ChinaMax.list",
	},
	"private_ip": {
		sr:   ruleSetBase + "/Shadowrocket/Lan/Lan.list",
		loon: ruleSetBase + "/Loon/Lan/Lan.list",
	},
	"private_domain": {
		sr:   ruleSetBase + "/Shadowrocket/Lan/Lan.list",
		loon: ruleSetBase + "/Loon/Lan/Lan.list",
	},
}

// generateClientConfig 生成 Shadowrocket 或 Loon 的完整配置。
func generateClientConfig(fmtType clientFormat, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode, subURL string) (string, error) {
	// 策略组与规则取自面板的 mihomo 模板
	cfg, err := GetEnabledMihomoConfig()
	if err != nil {
		return "", fmt.Errorf("获取订阅配置失败: %v", err)
	}

	// 1. 收集节点行
	nodeLines, nodeNames := collectClientNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes)
	if len(nodeNames) == 0 {
		nodeNames = []string{"DIRECT"}
	}

	// 2. 按国家代码归集，供地区策略组使用
	countryNodes := collectClientCountryNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes)

	var b strings.Builder

	// ---------- [General] ----------
	b.WriteString("[General]\n")
	if fmtType == formatLoon {
		b.WriteString("ip-mode = ipv4-only\n")
		b.WriteString("dns-server = system, 223.5.5.5, 119.29.29.29, 8.8.8.8, 1.1.1.1\n")
		b.WriteString("proxy-test-url = " + clientTestURL + "\n")
		b.WriteString("test-timeout = 5\n")
		b.WriteString("allow-wifi-access = false\n")
		// VoWiFi 需要放行 UDP 500/4500；关掉 stun 屏蔽会误伤 IMS
		b.WriteString("disable-stun = false\n")
	} else {
		b.WriteString("ipv6 = false\n")
		b.WriteString("bypass-system = true\n")
		b.WriteString("dns-server = system, 223.5.5.5, 119.29.29.29, 8.8.8.8, 1.1.1.1\n")
		b.WriteString("proxy-test-url = " + clientTestURL + "\n")
		b.WriteString("test-timeout = 5\n")
	}
	b.WriteString("skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local, captive.apple.com\n")
	b.WriteString("bypass-tun = 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.0.0.0/24, 192.0.2.0/24, 192.88.99.0/24, 192.168.0.0/16, 198.18.0.0/15, 198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/4, 255.255.255.255/32\n")
	if subURL != "" {
		b.WriteString("update-url = " + subURL + "\n")
	}
	b.WriteString("\n")

	// ---------- [Proxy] ----------
	b.WriteString("[Proxy]\n")
	for _, line := range nodeLines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// ---------- [Proxy Group] ----------
	b.WriteString("[Proxy Group]\n")
	groups := buildClientProxyGroups(cfg, nodeNames, countryNodes, fmtType)
	for _, g := range groups {
		b.WriteString(g)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// ---------- 规则 ----------
	if fmtType == formatLoon {
		writeLoonRules(&b, cfg, groups)
	} else {
		writeShadowrocketRules(&b, cfg)
	}

	return b.String(), nil
}

// ---------- 节点构建 ----------

type clientNode struct {
	Name     string
	Line     string
	Country  string // 用于地区策略组筛选
	Overseas bool
}

// collectClientNodes 收集所有节点行与名称。
func collectClientNodes(fmtType clientFormat, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (lines []string, names []string) {
	nodes := buildAllClientNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes)
	for _, n := range nodes {
		lines = append(lines, n.Line)
		names = append(names, n.Name)
	}
	return
}

// collectClientCountryNodes 按国家归集节点名。
func collectClientCountryNodes(fmtType clientFormat, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) map[string][]string {
	out := make(map[string][]string)
	for _, n := range buildAllClientNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes) {
		if n.Country != "" {
			out[n.Country] = append(out[n.Country], n.Name)
		}
	}
	return out
}

// buildAllClientNodes 构建全部节点（直连 + 落地 + 外部）。
func buildAllClientNodes(fmtType clientFormat, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) []clientNode {
	var out []clientNode
	seen := make(map[string]bool)

	add := func(n clientNode) {
		if n.Line == "" || n.Name == "" {
			return
		}
		if seen[n.Name] {
			for i := 2; ; i++ {
				cand := fmt.Sprintf("%s %d", n.Name, i)
				if !seen[cand] {
					n.Name = cand
					break
				}
			}
		}
		seen[n.Name] = true
		out = append(out, n)
	}

	for _, swn := range servers {
		for _, node := range swn.Nodes {
			if !node.Enabled {
				continue
			}
			var nc *database.ServerNodeConfig
			if sc, ok := nodeConfigs[swn.Server.ID]; ok {
				nc = sc[node.ID]
			}

			// 直连节点
			cc := getCountryCode(getNodeEffectiveIP(&swn.Server, &node, nc))
			if n := buildClientNode(fmtType, &swn.Server, &node, nc, user, lv, nil); n.Line != "" {
				n.Country = cc
				add(n)
			}

			// 落地出站节点
			for _, ob := range swn.Outbounds {
				extraUUID, err := user.EnsureExtraUUID(ob.Slot)
				if err != nil || extraUUID == "" {
					continue
				}
				opts := &ProxyCustomOptions{
					CustomName: swn.Server.Name + "-" + ob.Remark,
					CustomUUID: extraUUID,
				}
				// 落地出站的国家：手工值优先，留空回退入口 IP 推断
				obCC := strings.ToUpper(strings.TrimSpace(ob.Country))
				if obCC == "" {
					obCC = getCountryCode(ob.Host)
				}
				if n := buildClientNode(fmtType, &swn.Server, &node, nc, user, lv, opts); n.Line != "" {
					n.Country = obCC
					add(n)
				}
			}
		}
	}

	// 外部节点
	for _, ext := range externalNodes {
		if !ext.Enabled {
			continue
		}
		if n := buildClientNodeFromExternal(fmtType, &ext); n.Line != "" {
			n.Country = strings.ToUpper(strings.TrimSpace(ext.Country))
			add(n)
		}
	}

	return out
}

// buildClientNode 构建单个节点行。
func buildClientNode(fmtType clientFormat, server *database.Server, node *database.InboundNode, nc *database.ServerNodeConfig, user *database.ProxyUser, lv int, opts *ProxyCustomOptions) clientNode {
	p := initProxyBuildParams(server, node, nc, user, lv, opts)
	name := sanitizeClientName(p.Name)
	host := formatHostForURL(p.Host)

	var line string
	if fmtType == formatLoon {
		line = buildLoonProxyLine(name, host, p.Port, node, user, p.UUID)
	} else {
		line = buildShadowrocketProxyLine(name, host, p.Port, node, user, p.UUID)
	}
	return clientNode{Name: name, Line: line}
}

// buildClientNodeFromExternal 从外部节点构建。
func buildClientNodeFromExternal(fmtType clientFormat, ext *database.ExternalNode) clientNode {
	name := sanitizeClientName(ext.Name)
	host := formatHostForURL(ext.Host)

	var line string
	if fmtType == formatLoon {
		line = buildLoonProxyLineFromExternal(name, host, ext)
	} else {
		line = buildShadowrocketProxyLineFromExternal(name, host, ext)
	}
	return clientNode{Name: name, Line: line}
}

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

func loonQuote(v string) string { return `"` + strings.ReplaceAll(v, `"`, "") + `"` }
func boolStr(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

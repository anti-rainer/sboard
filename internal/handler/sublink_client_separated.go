package handler

import (
	"fmt"
	"strings"

	"github.com/sboard-go/sboard/internal/database"
)

// Shadowrocket / Loon 配置生成（配置与节点分离模式）。
//
// 同一个面板同时提供两类地址：
//
//	配置地址（填客户端「远程配置」）→ 只有策略组 + 规则，不含节点内容
//	节点地址（配置内部引用）        → 纯节点清单
//
// 节点变化时客户端刷新即可，不必重新导入配置。
//
// 两个客户端机制不同：
//
//	Loon 有 [Remote Proxy] 段可直接写订阅 URL，并用 [Remote Filter] 从中
//	按名字筛出子集给规则用：
//	    [Remote Proxy]
//	    SBoard = https://面板/subscribe/<uuid>?format=loon-nodes&node=trojan
//	    [Remote Filter]
//	    HK-VoWiFi = NameRegex, SBoard, FilterKey = ^(hkix-hytron)$
//	    [Proxy Group]
//	    Proxy = select, Auto, SBoard, HK-VoWiFi
//
//	Shadowrocket 的 [Proxy] 不支持 URL 引用——官方文档明确要求节点在 App 内
//	「添加订阅」，[Proxy] 可留空，策略组用 policy-regex-filter 匹配 App 里
//	的节点池。所以小火箭需要两次操作：配置填远程配置框，节点订阅填首页。
//
// 对应格式：
//
//	format=loon-conf    Loon 完整配置，节点动态引用
//	format=loon-nodes   Loon 节点清单（[Remote Proxy] 拉取用）
//	format=sr-conf      Shadowrocket 完整配置，不含节点
//
// 原 format=loon / shadowrocket 保持「内嵌节点」行为不变。

// clientConfigMode 节点处理方式。
type clientConfigMode int

const (
	modeEmbedded  clientConfigMode = iota // 节点写在配置里
	modeSeparated                         // 节点由订阅动态提供
)

// generateClientConfig2 生成客户端配置。
//
//	selfURL    配置自身地址，写入 update-url
//	nodeSubURL 节点订阅地址，分离模式下被引用
func generateClientConfig2(fmtType clientFormat, mode clientConfigMode, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode, selfURL, nodeSubURL string) (string, error) {
	cfg, err := GetEnabledMihomoConfig()
	if err != nil {
		return "", fmt.Errorf("获取订阅配置失败: %v", err)
	}

	nodeLines, nodeNames := collectClientNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes)
	countryNodes := collectClientCountryNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes)
	if len(nodeNames) == 0 {
		nodeNames = []string{"DIRECT"}
	}

	var b strings.Builder

	writeClientGeneral(&b, fmtType, selfURL)

	// ---------- 节点区 ----------
	switch {
	case mode == modeEmbedded:
		b.WriteString("[Proxy]\n")
		for _, line := range nodeLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")

	case fmtType == formatLoon:
		b.WriteString("[Remote Proxy]\n")
		b.WriteString("SBoard = " + nodeSubURL + "\n")
		b.WriteString("\n")
		b.WriteString("[Proxy]\n")
		b.WriteString("# 节点由上方 [Remote Proxy] 的订阅提供\n")
		b.WriteString("\n")

	default:
		// Shadowrocket 的 [Proxy] 不接受 URL
		b.WriteString("[Proxy]\n")
		b.WriteString("# 小火箭无法在配置里引用订阅 URL。请分两步：\n")
		b.WriteString("#   1. 本地址填入「配置」页的远程配置\n")
		b.WriteString("#   2. 下面的节点地址在「首页 → 添加节点 → 订阅」里添加\n")
		b.WriteString("#      " + nodeSubURL + "\n")
		b.WriteString("# 配置里的策略组会按节点名自动匹配该订阅。\n")
		b.WriteString("\n")
	}

	// ---------- Loon [Remote Filter] ----------
	var filterNames []string
	if mode == modeSeparated && fmtType == formatLoon && cfg != nil {
		filters := buildLoonRemoteFilters(cfg, "SBoard", nodeNames, countryNodes)
		if len(filters) > 0 {
			b.WriteString("[Remote Filter]\n")
			for _, f := range filters {
				b.WriteString(f.line)
				b.WriteString("\n")
				filterNames = append(filterNames, f.name)
			}
			b.WriteString("\n")
		}
	}

	// ---------- [Proxy Group] ----------
	b.WriteString("[Proxy Group]\n")
	var groups []string
	switch {
	case mode == modeEmbedded:
		groups = buildClientProxyGroups(cfg, nodeNames, countryNodes, fmtType)
	case fmtType == formatLoon:
		groups = buildLoonGroupsSeparated(cfg, "SBoard", filterNames)
	default:
		groups = buildShadowrocketGroupsSeparated(cfg, clientGroupNames(cfg))
	}
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

// clientGroupNames 返回配置里除内置组之外的策略组名。
func clientGroupNames(cfg *MihomoSubscriptionConfig) []string {
	if cfg == nil {
		return nil
	}
	var out []string
	for _, pg := range cfg.Config.ProxyGroups {
		if name := sanitizeClientName(pg.Name); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// loonFilter 一条 [Remote Filter] 定义。
type loonFilter struct {
	name string
	line string
}

// writeClientGeneral 输出 [General] 段。
func writeClientGeneral(b *strings.Builder, fmtType clientFormat, selfURL string) {
	b.WriteString("[General]\n")
	if fmtType == formatLoon {
		b.WriteString("ip-mode = ipv4-only\n")
		b.WriteString("dns-server = system, 223.5.5.5, 119.29.29.29, 8.8.8.8, 1.1.1.1\n")
		b.WriteString("proxy-test-url = " + clientTestURL + "\n")
		b.WriteString("test-timeout = 5\n")
		b.WriteString("allow-wifi-access = false\n")
		// VoWiFi 依赖 UDP 500/4500，禁用 STUN 会误伤 IMS
		b.WriteString("disable-stun = false\n")
	} else {
		b.WriteString("ipv6 = false\n")
		b.WriteString("bypass-system = true\n")
		b.WriteString("dns-server = system, 223.5.5.5, 119.29.29.29, 8.8.8.8, 1.1.1.1\n")
		b.WriteString("fallback-dns-server = system\n")
		b.WriteString("private-ip-answer = true\n")
		b.WriteString("proxy-test-url = " + clientTestURL + "\n")
		b.WriteString("test-timeout = 5\n")
	}
	b.WriteString("skip-proxy = 127.0.0.1, 192.168.0.0/16, 10.0.0.0/8, 172.16.0.0/12, 100.64.0.0/10, localhost, *.local, captive.apple.com\n")
	b.WriteString("bypass-tun = 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.0.0.0/24, 192.0.2.0/24, 192.88.99.0/24, 192.168.0.0/16, 198.18.0.0/15, 198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/4, 255.255.255.255/32\n")
	if selfURL != "" {
		b.WriteString("update-url = " + selfURL + "\n")
	}
	b.WriteString("\n")
}

// buildLoonRemoteFilters 用 [Remote Filter] 从订阅里筛出节点子集。
//
// 这里刻意用 NodeSelect 而不是 NameRegex 精确匹配节点名：
// 分离模式下配置是静态的，而节点由客户端在运行时拉取，节点名可能因
// 等级调整、服务器改名而变化。把名字写死在配置里，一旦名字变了筛选就
// 落空。NodeSelect 让用户在 Loon UI 里为该筛选勾选节点，配置不需要知道
// 具体名字，节点变化也不会让配置失效。
func buildLoonRemoteFilters(cfg *MihomoSubscriptionConfig, remoteAlias string, allNodes []string, countryNodes map[string][]string) []loonFilter {
	if cfg == nil {
		return nil
	}
	var out []loonFilter
	seen := make(map[string]bool)
	for _, pg := range cfg.Config.ProxyGroups {
		name := sanitizeClientName(pg.Name)
		if name == "" || seen[name] || name == remoteAlias {
			continue
		}
		// 该策略组在当前配置下没有任何节点时跳过：
		// Loon 对匹配不到节点的筛选会告警，且规则指向它也无意义。
		members := resolveGroupMembers(pg, allNodes, countryNodes)
		if len(members) == 0 {
			continue
		}
		seen[name] = true
		out = append(out, loonFilter{
			name: name,
			line: fmt.Sprintf("%s = NodeSelect, %s", name, remoteAlias),
		})
	}
	return out
}

// buildLoonGroupsSeparated 生成分离模式的 Loon 策略组。
//
// 成员直接用 [Remote Proxy] 别名和 [Remote Filter] 名——两者都能当策略引用。
func buildLoonGroupsSeparated(cfg *MihomoSubscriptionConfig, remoteAlias string, filterNames []string) []string {
	// 主组：全部节点 + 各筛选组
	members := []string{"Auto", "DIRECT", remoteAlias}
	members = append(members, filterNames...)

	out := []string{
		"Proxy = select, " + strings.Join(members, ", "),
		"Auto = url-test, " + remoteAlias + ", url=" + clientTestURL + ", interval=600, tolerance=50",
	}

	// 每个筛选组再给一个同名 select 组，便于手动指定和测速
	for _, name := range filterNames {
		out = append(out,
			fmt.Sprintf("%s-Auto = url-test, %s, url=%s, interval=600, tolerance=50", name, name, clientTestURL))
	}
	return out
}

// buildShadowrocketGroupsSeparated 生成分离模式的 Shadowrocket 策略组。
//
// 分离模式下配置不知道 App 里实际有哪些节点，因此各策略组一律用
// policy-regex-filter=.* 覆盖全部节点，由用户在小火箭 UI 里为每个组
// 勾选具体节点。地区组也照此处理——把国家代码写进正则匹配不到节点名
// （节点名是「香港」「shandun-de-英国nat」这类），只会得到空组。
func buildShadowrocketGroupsSeparated(cfg *MihomoSubscriptionConfig, groupNames []string) []string {
	out := []string{
		"PROXY = select, policy-regex-filter=.*",
		"Auto = url-test, policy-regex-filter=.*, url=" + clientTestURL + ", interval=600, tolerance=50",
	}
	for _, name := range groupNames {
		if name == "" || name == "PROXY" || name == "Auto" {
			continue
		}
		out = append(out, fmt.Sprintf("%s = select, policy-regex-filter=.*", name))
	}
	return out
}

// buildExactNameRegex 把节点名列表拼成精确匹配正则。
func buildExactNameRegex(names []string) string {
	if len(names) == 0 {
		return ".*"
	}
	escaped := make([]string, 0, len(names))
	for _, n := range names {
		escaped = append(escaped, regexpQuoteMeta(n))
	}
	return "^(" + strings.Join(escaped, "|") + ")$"
}

// regexpQuoteMeta 转义正则元字符。
func regexpQuoteMeta(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '.', '+', '*', '?', '(', ')', '|', '[', ']', '{', '}', '^', '$':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

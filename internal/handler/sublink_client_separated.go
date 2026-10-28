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
		groups = buildShadowrocketGroupsSeparated(cfg)
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

// buildLoonRemoteFilters 用 [Remote Filter] + NameRegex 从订阅里筛出节点子集。
//
// 正则按「地区关键词」构造，而不是写死具体节点名：
// 节点名形如「香港」「hkix-hytron」「shandun-de-英国nat」，把国家对应的
// 中英文关键词与常见缩写做成交替正则，节点增删或改名后依然能匹配到。
// 若订阅里完全没有该地区的节点，跳过以免 Loon 告警。
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
		// Loon 对匹配不到节点的筛选会告警，规则指向它也无意义。
		members := resolveGroupMembers(pg, allNodes, countryNodes)
		if len(members) == 0 {
			continue
		}
		seen[name] = true
		// geoip-cn 的国内组：筛选条目改名为 <name>-Nodes，把 <name> 让给
		// 带 DIRECT 的 select 组（规则里引用的是 <name>）。
		filterName := name
		if pg.FilterMode == "geoip-cn" && strings.TrimSpace(pg.Filter) == "cn" {
			filterName = name + "-Nodes"
		}
		out = append(out, loonFilter{
			name: filterName,
			line: fmt.Sprintf("%s = NameRegex, %s, FilterKey = %s", filterName, remoteAlias, groupNameRegex(pg, members)),
		})
	}
	return out
}

// groupNameRegex 生成按地区筛节点的正则。
//
// 策略：一律输出「地区关键词正则」，而不是精确节点名。
// 节点名形如「香港」「hkix-hytron」「shandun-de-英国nat」，把国家对应的
// 中英文关键词与常见缩写做成交替正则后，节点增删、改名、调整等级都还能
// 匹配到；写死具体名字则一旦改名就筛空。
//
// members 是当前解析出的节点，仅在所有关键词策略都失败时兜底使用。
func groupNameRegex(pg MihomoProxyGroup, members []string) string {
	switch pg.FilterMode {
	case "regex":
		// 用户自己写了正则，直接采用
		if pg.Filter != "" && pg.Filter != ".*" {
			return pg.Filter
		}

	case "geoip-country":
		var alts []string
		for _, raw := range strings.Split(pg.Filter, ",") {
			c := strings.ToUpper(strings.TrimSpace(raw))
			if c == "" {
				continue
			}
			alts = append(alts, countryKeywords(c)...)
		}
		if len(alts) > 0 {
			return "(?i)" + strings.Join(dedupeStrings(alts), "|")
		}

	case "geoip-cn":
		// filter="cn" 取国内；"!cn" 取海外（用负向前瞻排除国内关键词）
		if strings.TrimSpace(pg.Filter) == "cn" {
			var alts []string
			alts = append(alts, countryKeywords("CN")...)
			if len(alts) > 0 {
				return "(?i)" + strings.Join(dedupeStrings(alts), "|")
			}
		} else {
			// 海外：排除含国内关键词的节点
			cnWords := dedupeStrings(countryKeywords("CN"))
			if len(cnWords) > 0 {
				return "(?i)^((?!(?:" + strings.Join(cnWords, "|") + ")).)*$"
			}
		}
	}

	// 兜底：精确匹配当前已知节点名
	return buildExactNameRegex(members)
}

// countryKeywords 国家代码 → 节点名里可能出现的关键词。
// 覆盖中文名、英文名与常见缩写，兼顾大小写不敏感匹配。
func countryKeywords(code string) []string {
	switch code {
	case "HK", "UK", "GB":
		return map[string][]string{
			"HK": {"香港", "HKG", "hkix", "HK"},
			"UK": {"英国", "UK", "GBR", "GB", "London", "伦敦"},
			"GB": {"英国", "UK", "GBR", "GB", "London", "伦敦"},
		}[code]
	case "CZ":
		return []string{"捷克", "CZ", "CZE", "Czech", "Praha"}
	case "US":
		return []string{"美国", "US", "USA", "United States", "Los Angeles", "洛杉矶", "Seattle", "西雅图"}
	case "JP":
		return []string{"日本", "JP", "JPN", "Japan", "Tokyo", "东京", "大阪", "Osaka"}
	case "SG":
		return []string{"新加坡", "SG", "SGP", "Singapore", "狮城"}
	case "DE":
		return []string{"德国", "DE", "DEU", "Germany", "Frankfurt", "法兰克福"}
	case "KR":
		return []string{"韩国", "KR", "KOR", "Korea", "Seoul", "首尔"}
	case "TW":
		return []string{"台湾", "TW", "TWN", "Taiwan", "台北"}
	case "CN":
		return []string{"上海", "北京", "深圳", "广州", "家宽", "CN", "CHN", "China"}
	}
	return []string{code}
}

// buildLoonGroupsSeparated 生成分离模式的 Loon 策略组。
//
// 成员直接用 [Remote Proxy] 别名和 [Remote Filter] 名——两者都能当策略引用。
func buildLoonGroupsSeparated(cfg *MihomoSubscriptionConfig, remoteAlias string, filterNames []string) []string {
	// 主组：自动测速、直连、节点订阅、各筛选组
	members := []string{"Auto", "DIRECT", remoteAlias}
	members = append(members, filterNames...)

	out := []string{
		"Proxy = select, " + strings.Join(members, ", "),
		"Auto = url-test, " + remoteAlias + ", url=" + clientTestURL + ", interval=600, tolerance=50",
	}

	// 国内组必须能选 DIRECT（mihomo 里该组成员是 [DIRECT, 国内节点]）。
	// [Remote Filter] 只能筛选节点、无法混入 DIRECT，所以国内组单独声明为
	// 显式 select 组：DIRECT + 对应筛选条目。策略名与规则引用保持一致——
	// 规则里写的是「国内」，这里用「国内-Direct」会引用不到，故沿用原名并
	// 通过重命名筛选条目的方式腾出「国内」这个名字。
	if cfg != nil {
		for _, pg := range cfg.Config.ProxyGroups {
			if pg.FilterMode != "geoip-cn" || strings.TrimSpace(pg.Filter) != "cn" {
				continue
			}
			name := sanitizeClientName(pg.Name)
			if name == "" {
				continue
			}
			out = append(out, fmt.Sprintf("%s = select, DIRECT, %s-Nodes", name, name))
		}
	}

	// 国内筛选组必须能选 DIRECT：与 mihomo 一致，否则国内流量还得走代理。
	// [Remote Filter] 已经声明了同名条目，[Rule] 直接引用它即可；
	// 这里不再额外造 -Auto 组，避免策略名重复和混淆。
	return out
}

// buildShadowrocketGroupsSeparated 生成分离模式的 Shadowrocket 策略组。
//
// 用 policy-regex-filter 按地区关键词正则筛节点（与小火箭 UI 里的
// 「策略组-正则筛选」等价），节点增删或改名后依然匹配得到。
func buildShadowrocketGroupsSeparated(cfg *MihomoSubscriptionConfig) []string {
	out := []string{
		"PROXY = select, policy-regex-filter=.*",
		"Auto = url-test, policy-regex-filter=.*, url=" + clientTestURL + ", interval=600, tolerance=50",
	}
	if cfg == nil {
		return out
	}
	for _, pg := range cfg.Config.ProxyGroups {
		name := sanitizeClientName(pg.Name)
		if name == "" || name == "PROXY" || name == "Auto" {
			continue
		}
		pattern := groupNameRegex(pg, nil)

		// 国内组要能选 DIRECT：mihomo 里该组成员是 [DIRECT, 国内节点]。
		// Shadowrocket 用 policy-regex-filter 时无法混入 DIRECT，
		// 所以给国内组单独写成显式的 select 组并带上 DIRECT。
		if pg.FilterMode == "geoip-cn" && strings.TrimSpace(pg.Filter) == "cn" {
			out = append(out, fmt.Sprintf("%s = select, DIRECT", name))
			continue
		}
		out = append(out, fmt.Sprintf("%s = select, policy-regex-filter=%s", name, pattern))
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

package handler

import (
	"fmt"
	"regexp"
	"strings"
)

// 策略组与规则生成。
//
// 直接读取面板 mihomo 模板里的 proxyGroups 与 rules：
//   - 策略组：geoip-cn / geoip-country / regex 过滤模式都翻译成对等的客户端组
//   - 规则：DOMAIN-SUFFIX / DOMAIN-KEYWORD / DST-PORT / RULE-SET / GEOIP / MATCH
//     按原顺序输出，OR 组合规则展开成多条
//
// 这样 VoWiFi 分流（HK/UK/CZ 地区组 + mccXXX.pub.3gppnetwork.org 规则）在
// Shadowrocket 和 Loon 上同样是按国家选节点，与 mihomo 行为一致。

// buildClientProxyGroups 生成策略组行列表。
func buildClientProxyGroups(cfg *MihomoSubscriptionConfig, allNodes []string, countryNodes map[string][]string, fmtType clientFormat) []string {
	var out []string

	// 是否 Loon（决定语法细节）
	isLoon := fmtType == formatLoon

	// 主选择组：包含所有节点
	mainGroup := "Proxy"
	if !isLoon {
		mainGroup = "PROXY"
	}
	parts := []string{"select"}
	// 自动测速组放最前，方便默认选它
	autoName := "Auto"
	parts = append(parts, autoName, "DIRECT")
	parts = append(parts, allNodes...)
	out = append(out, fmt.Sprintf("%s = %s", mainGroup, strings.Join(parts, ", ")))

	// 自动测速组
	autoParts := []string{"url-test"}
	autoParts = append(autoParts, allNodes...)
	autoParts = append(autoParts, "url="+clientTestURL, "interval=600", "tolerance=50")
	out = append(out, fmt.Sprintf("%s = %s", autoName, strings.Join(autoParts, ", ")))

	if cfg == nil {
		return out
	}

	// 按模板里的策略组逐个翻译
	for _, pg := range cfg.Config.ProxyGroups {
		name := sanitizeClientName(pg.Name)
		if name == "" {
			continue
		}
		// 跳过与内置组重名的
		if name == mainGroup || name == autoName {
			continue
		}

		// 算出该组包含哪些节点
		members := resolveGroupMembers(pg, allNodes, countryNodes)
		if len(members) == 0 {
			// 地区组没有对应节点时，指向 DIRECT 而不是空组
			// （空组会让客户端拒绝加载整份配置）
			members = []string{"DIRECT"}
		}

		// 组类型：模板用 url-test 就跟随，否则 select
		groupType := "select"
		if pg.Type == "url-test" || pg.Type == "fallback" {
			groupType = pg.Type
		}

		line := buildGroupLine(name, groupType, members, pg)
		out = append(out, line)
	}

	return out
}

// buildGroupLine 拼一行策略组定义。
func buildGroupLine(name, groupType string, members []string, pg MihomoProxyGroup) string {
	parts := []string{groupType}
	parts = append(parts, members...)

	if groupType == "url-test" || groupType == "fallback" || groupType == "load-balance" {
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
	return fmt.Sprintf("%s = %s", name, strings.Join(parts, ", "))
}

// resolveGroupMembers 按模板策略组的过滤条件筛出节点名。
//
// 支持的 filterMode：
//   geoip-cn      filter="cn" 取国内，filter="!cn" 取海外
//   geoip-country filter="HK" 或 "US,GB,DE"，UK 自动映射到 GB
//   regex         按节点名正则
//   all / ""      全部节点
func resolveGroupMembers(pg MihomoProxyGroup, allNodes []string, countryNodes map[string][]string) []string {
	switch pg.FilterMode {
	case "geoip-country":
		var out []string
		for _, c := range strings.Split(pg.Filter, ",") {
			c = strings.ToUpper(strings.TrimSpace(c))
			if c == "" {
				continue
			}
			if c == "UK" {
				c = "GB"
			}
			if names, ok := countryNodes[c]; ok {
				out = append(out, names...)
			}
		}
		return dedupeStrings(out)

	case "geoip-cn":
		f := strings.TrimSpace(pg.Filter)
		if f == "cn" || f == "CN" {
			return countryNodes["CN"]
		}
		// !cn 或其它：全部节点里排除国内的
		cn := make(map[string]bool)
		for _, n := range countryNodes["CN"] {
			cn[n] = true
		}
		var out []string
		for _, n := range allNodes {
			if !cn[n] {
				out = append(out, n)
			}
		}
		return out

	case "regex":
		if pg.Filter == "" || pg.Filter == ".*" {
			return allNodes
		}
		// 用真正的正则匹配，支持 (?i) 忽略大小写、锚点、交替等写法。
		// 原来只做了 strings.Contains，像 "(?i)hk|香港" 这种模式永远匹配不到。
		re, err := regexp.Compile(pg.Filter)
		if err != nil {
			// 模式写错时退化为子串匹配，避免整份订阅生成失败
			var out []string
			for _, n := range allNodes {
				if strings.Contains(n, pg.Filter) {
					out = append(out, n)
				}
			}
			return out
		}
		var out []string
		for _, n := range allNodes {
			if re.MatchString(n) {
				out = append(out, n)
			}
		}
		return out

	default:
		// 未指定过滤：默认全部节点（与 mihomo 生成器行为一致）
		return allNodes
	}
}

// dedupeStrings 去重并保持顺序。
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ---------- 规则输出 ----------

// clientRule 表示一条已翻译好的客户端规则。
type clientRule struct {
	Line   string
	Policy string // 用于 Remote Rule 段
	Tag    string
	URL    string
}

// translateRules 把面板规则翻译成客户端规则行。
// isLoon 决定 RULE-SET 的输出位置（Loon 放 [Remote Rule] 段，Shadowrocket 内联）。
func translateRules(cfg *MihomoSubscriptionConfig, isLoon bool) (inline []string, remote []clientRule) {
	if cfg == nil {
		return
	}

	for _, r := range cfg.Config.Rules {
		policy := sanitizeClientName(r.Outbound)
		if policy == "" {
			policy = "DIRECT"
		}

		switch strings.ToUpper(r.Type) {
		case "DOMAIN-SUFFIX":
			inline = append(inline, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", r.Value, policy))

		case "DOMAIN":
			inline = append(inline, fmt.Sprintf("DOMAIN,%s,%s", r.Value, policy))

		case "DOMAIN-KEYWORD":
			inline = append(inline, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", r.Value, policy))

		case "IP-CIDR", "IP-CIDR6":
			inline = append(inline, fmt.Sprintf("%s,%s,%s,no-resolve", strings.ToUpper(r.Type), r.Value, policy))

		case "DST-PORT":
			// mihomo 用斜杠表示端口段（"500/4500"），Shadowrocket / Loon 要求
			// 逗号分隔的端口列表（"500,4500"），直接搬运会被解析成非法端口。
			ports := strings.Split(r.Value, "/")
			for i := range ports {
				ports[i] = strings.TrimSpace(ports[i])
			}
			inline = append(inline, fmt.Sprintf("DST-PORT,%s,%s", strings.Join(ports, ","), policy))

		case "GEOIP":
			inline = append(inline, fmt.Sprintf("GEOIP,%s,%s", r.Value, policy))

		case "MATCH", "FINAL":
			// FINAL 必须最后输出，先暂存由调用方追加
			inline = append(inline, fmt.Sprintf("FINAL,%s", policy))

		case "RULE-SET":
			if mapped, ok := ruleSetURLs[r.Value]; ok {
				url := mapped.sr
				if isLoon {
					url = mapped.loon
				}
				if isLoon {
					// Loon 走 [Remote Rule] 段
					remote = append(remote, clientRule{
						Policy: policy,
						Tag:    sanitizeClientName(r.Value),
						URL:    url,
					})
				} else {
					// Shadowrocket 内联 RULE-SET
					noResolve := ""
					if strings.Contains(strings.ToLower(r.Value), "ip") {
						noResolve = ",no-resolve"
					}
					inline = append(inline, fmt.Sprintf("RULE-SET,%s,%s%s", url, policy, noResolve))
				}
			} else if isLoon {
				// 未知规则集：Loon 没有对应 url 就跳过，避免加载失败
				continue
			}

		case "OR":
			// OR 组合规则展开成多条同策略的规则
			for _, sr := range r.SubRules {
				switch strings.ToUpper(sr.Type) {
				case "DOMAIN-SUFFIX":
					inline = append(inline, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", sr.Value, policy))
				case "DOMAIN":
					inline = append(inline, fmt.Sprintf("DOMAIN,%s,%s", sr.Value, policy))
				case "DOMAIN-KEYWORD":
					inline = append(inline, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", sr.Value, policy))
				case "RULE-SET":
					if mapped, ok := ruleSetURLs[sr.Value]; ok {
						url := mapped.sr
						if isLoon {
							url = mapped.loon
							remote = append(remote, clientRule{
								Policy: policy,
								Tag:    sanitizeClientName(sr.Value),
								URL:    url,
							})
						} else {
							noResolve := ""
							if strings.Contains(strings.ToLower(sr.Value), "ip") {
								noResolve = ",no-resolve"
							}
							inline = append(inline, fmt.Sprintf("RULE-SET,%s,%s%s", url, policy, noResolve))
						}
					}
				}
			}
		}
	}

	return
}

// writeShadowrocketRules 输出 Shadowrocket 的 [Rule] 段。
func writeShadowrocketRules(b *strings.Builder, cfg *MihomoSubscriptionConfig) {
	inline, _ := translateRules(cfg, false)

	b.WriteString("[Rule]\n")

	// 把 FINAL 挪到最后
	var finals []string
	var others []string
	for _, line := range inline {
		if strings.HasPrefix(line, "FINAL,") {
			finals = append(finals, line)
		} else {
			others = append(others, line)
		}
	}

	// 局域网直连放最前
	b.WriteString("IP-CIDR,192.168.0.0/16,DIRECT,no-resolve\n")
	b.WriteString("IP-CIDR,10.0.0.0/8,DIRECT,no-resolve\n")
	b.WriteString("IP-CIDR,172.16.0.0/12,DIRECT,no-resolve\n")
	b.WriteString("IP-CIDR,127.0.0.0/8,DIRECT,no-resolve\n")
	for _, line := range others {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if len(finals) == 0 {
		b.WriteString("FINAL,DIRECT\n")
	} else {
		for _, f := range finals {
			b.WriteString(f)
			b.WriteString("\n")
		}
	}
}

// writeLoonRules 输出 Loon 的 [Remote Rule] 与 [Rule] 段。
func writeLoonRules(b *strings.Builder, cfg *MihomoSubscriptionConfig, _ []string) {
	inline, remote := translateRules(cfg, true)

	if len(remote) > 0 {
		b.WriteString("[Remote Rule]\n")
		for _, r := range remote {
			b.WriteString(fmt.Sprintf("%s, policy = %s, tag = %s, enabled = true\n", r.URL, r.Policy, r.Tag))
		}
		b.WriteString("\n")
	}

	b.WriteString("[Rule]\n")

	// 直接引用已声明的 remote tag
	for _, r := range remote {
		b.WriteString(fmt.Sprintf("RULE-SET,%s,%s\n", r.Tag, r.Policy))
	}

	var finals []string
	for _, line := range inline {
		if strings.HasPrefix(line, "FINAL,") {
			finals = append(finals, line)
			continue
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	// 局域网直连 + 国内直连
	b.WriteString("IP-CIDR,192.168.0.0/16,DIRECT,no-resolve\n")
	b.WriteString("IP-CIDR,10.0.0.0/8,DIRECT,no-resolve\n")
	b.WriteString("IP-CIDR,172.16.0.0/12,DIRECT,no-resolve\n")
	b.WriteString("GEOIP,CN,DIRECT\n")

	if len(finals) == 0 {
		b.WriteString("FINAL,DIRECT\n")
	} else {
		for _, f := range finals {
			b.WriteString(f)
			b.WriteString("\n")
		}
	}
}

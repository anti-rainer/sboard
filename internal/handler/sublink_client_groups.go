package handler

import (
	"fmt"
	"regexp"
	"strings"
)

// 策略组与规则生成。
//
// 直接读取面板 mihomo 模板里的 proxyGroups 与 rules，保证三端行为一致。

// buildClientProxyGroups 生成策略组行列表。
//
// 严格按面板 mihomo 模板里的 proxyGroups 逐条输出，不额外添加
// Proxy / PROXY / Auto 之类的组——mihomo 里没有的组这里也不应该有。
func buildClientProxyGroups(cfg *MihomoSubscriptionConfig, allNodes []string, countryNodes map[string][]string, fmtType clientFormat) []string {
	var out []string

	if cfg == nil {
		return out
	}

	for _, pg := range cfg.Config.ProxyGroups {
		name := sanitizeClientName(pg.Name)
		if name == "" {
			continue
		}

		members := resolveGroupMembers(pg, allNodes, countryNodes)

		// 与 mihomo 一致：国内组必须含 DIRECT，否则国内流量还得走代理。
		// mihomo 是在成员列表最前面插入 DIRECT。
		if pg.FilterMode == "geoip-cn" && strings.TrimSpace(pg.Filter) == "cn" {
			if len(members) == 0 || members[0] != "DIRECT" {
				members = append([]string{"DIRECT"}, members...)
			}
		}

		if len(members) == 0 {
			members = []string{"DIRECT"}
		}

		groupType := "select"
		if pg.Type == "url-test" || pg.Type == "fallback" {
			groupType = pg.Type
		}

		out = append(out, buildGroupLine(name, groupType, members, pg))
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
			return dedupeStrings(countryNodes["CN"])
		}
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
		re, err := regexp.Compile(pg.Filter)
		if err != nil {
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

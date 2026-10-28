package handler

import (
	"fmt"
	"strings"
)

// 客户端规则生成：严格复现 mihomo 的规则顺序与规则集。
//
// mihomo 用 QuixoticHeart/rule-set 的 .mrs（专用二进制格式），iOS 客户端
// 读不了。同一仓库为各客户端出了对应格式，内容同源：
//
//	ruleset/loon/<名字>.list           Loon 可直接引用
//	ruleset/shadowrocket/<名字>.list   Shadowrocket 可直接引用
//	ruleset/meta/domain/<名字>.list    域名规则（通用）
//	ruleset/meta/ipcidr/<名字>.list    IP 段规则（通用）
//
// 映射面板里的 ruleProvider tag 到这些路径，分流结果与 mihomo 一致。

// 规则集基础地址。走 ghfast.top 加速：这些 .list 都在 GitHub raw，
// 国内直连很慢，小火箭导入配置时会因此长时间卡在「添加中」。
// 加速前缀可被系统设置 rule_set_proxy 覆盖，留空则直连。
const (
	ruleSetRepoBase   = "https://raw.githubusercontent.com/QuixoticHeart/rule-set/refs/heads/ruleset"
	defaultRuleSetProxy = "https://ghfast.top/"
)

// getRuleSetProxy 取规则集加速前缀，可在面板设置里改；设为 "none" 表示直连。
func getRuleSetProxy() string {
	p := GetSetting("rule_set_proxy")
	if p == "" {
		p = defaultRuleSetProxy
	}
	if p == "none" || p == "-" {
		return ""
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// buildRuleSetURL 拼接规则集地址，带加速前缀。
func buildRuleSetURL(path string) string {
	raw := ruleSetRepoBase + "/" + path
	if proxy := getRuleSetProxy(); proxy != "" {
		return proxy + raw
	}
	return raw
}

// ruleSetSpec 规则集在两种客户端里的路径。
type ruleSetSpec struct {
	LoonPath string
	SRPath   string
	IsIP     bool // IP 段规则集，引用时需 no-resolve
}

// ruleSetSpecs 面板 ruleProvider tag → 客户端规则集路径。
var ruleSetSpecs = map[string]ruleSetSpec{
	"AI": {
		LoonPath: "loon/ai.list",
		SRPath:   "shadowrocket/ai.list",
	},
	"cn_domain": {
		LoonPath: "loon/cn.list",
		SRPath:   "shadowrocket/cn.list",
	},
	"cn_ip": {
		LoonPath: "loon/cncidr.list",
		SRPath:   "shadowrocket/cncidr.list",
		IsIP:     true,
	},
	"private_domain": {
		LoonPath: "loon/private.list",
		SRPath:   "shadowrocket/private.list",
	},
	"private_ip": {
		// 这个仓库的 loon 目录没有 private 的 IP 段文件，用 meta 通用格式
		LoonPath: "meta/ipcidr/private.list",
		SRPath:   "meta/ipcidr/private.list",
		IsIP:     true,
	},
	"proxy_domain": {
		LoonPath: "loon/proxy.list",
		SRPath:   "shadowrocket/proxy.list",
	},
}

// clientRuleSet 一条待声明的规则集。
type clientRuleSet struct {
	Tag  string
	URL  string
	IsIP bool
}

// resolveRuleSet 按 tag 取客户端规则集地址。
func resolveRuleSet(tag string, isLoon bool) (clientRuleSet, bool) {
	spec, ok := ruleSetSpecs[tag]
	if !ok {
		return clientRuleSet{}, false
	}
	path := spec.SRPath
	if isLoon {
		path = spec.LoonPath
	}
	if path == "" {
		return clientRuleSet{}, false
	}
	return clientRuleSet{Tag: tag, URL: buildRuleSetURL(path), IsIP: spec.IsIP}, true
}

// translateRulesToClient 把面板规则按原顺序翻译成客户端规则。
//
// 关键点：顺序完全照抄 mihomo。面板里 VoWiFi 的域名规则排在 proxy_domain
// 规则集之前，若客户端把它们挪到后面，mccXXX.pub.3gppnetwork.org 会先被
// proxy_domain 命中，VoWiFi 分流就失效了。
//
// 返回 inline（直接写在 [Rule]）与 remote（Loon 的 [Remote Rule] 声明）。
func translateRulesToClient(cfg *MihomoSubscriptionConfig, isLoon bool) (inline []string, remote []clientRuleSet) {
	if cfg == nil {
		return
	}

	declared := make(map[string]bool)

	emitRuleSet := func(tag, policy string) {
		rs, ok := resolveRuleSet(tag, isLoon)
		if !ok {
			return
		}
		if isLoon {
			if !declared[tag] {
				declared[tag] = true
				remote = append(remote, rs)
			}
			inline = append(inline, fmt.Sprintf("RULE-SET,%s,%s", tag, policy))
			return
		}
		noResolve := ""
		if rs.IsIP {
			noResolve = ",no-resolve"
		}
		inline = append(inline, fmt.Sprintf("RULE-SET,%s,%s%s", rs.URL, policy, noResolve))
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
		case "IP-CIDR":
			inline = append(inline, fmt.Sprintf("IP-CIDR,%s,%s,no-resolve", r.Value, policy))
		case "IP-CIDR6":
			inline = append(inline, fmt.Sprintf("IP-CIDR6,%s,%s,no-resolve", r.Value, policy))
		case "DST-PORT":
			// mihomo 用 "500/4500" 表示端口段，但 Shadowrocket / Loon 的
			// DST-PORT 只接受单个端口，逗号分隔会被当成非法值整条失效。
			// 一行一个端口展开成多条规则。
			for _, port := range strings.Split(r.Value, "/") {
				port = strings.TrimSpace(port)
				if port == "" {
					continue
				}
				inline = append(inline, fmt.Sprintf("DST-PORT,%s,%s", port, policy))
			}
		case "GEOIP":
			inline = append(inline, fmt.Sprintf("GEOIP,%s,%s", r.Value, policy))
		case "MATCH", "FINAL":
			inline = append(inline, fmt.Sprintf("FINAL,%s", policy))
		case "RULE-SET":
			emitRuleSet(r.Value, policy)
		case "OR":
			for _, sr := range r.SubRules {
				switch strings.ToUpper(sr.Type) {
				case "DOMAIN-SUFFIX":
					inline = append(inline, fmt.Sprintf("DOMAIN-SUFFIX,%s,%s", sr.Value, policy))
				case "DOMAIN":
					inline = append(inline, fmt.Sprintf("DOMAIN,%s,%s", sr.Value, policy))
				case "DOMAIN-KEYWORD":
					inline = append(inline, fmt.Sprintf("DOMAIN-KEYWORD,%s,%s", sr.Value, policy))
				case "RULE-SET":
					emitRuleSet(sr.Value, policy)
				}
			}
		}
	}

	return
}

// splitFinal 把 FINAL 行挑出，保证排在最后。
func splitFinal(lines []string) (body []string, finals []string) {
	for _, l := range lines {
		if strings.HasPrefix(l, "FINAL,") {
			finals = append(finals, l)
		} else {
			body = append(body, l)
		}
	}
	return
}

// writeRuleSection 输出 [Rule] 段。顺序保持 mihomo 原样，FINAL 最后。
func writeRuleSection(b *strings.Builder, inline []string) {
	body, finals := splitFinal(inline)
	b.WriteString("[Rule]\n")
	for _, line := range body {
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

// writeShadowrocketRules 输出 Shadowrocket 的 [Rule] 段。
func writeShadowrocketRules(b *strings.Builder, cfg *MihomoSubscriptionConfig) {
	inline, _ := translateRulesToClient(cfg, false)
	writeRuleSection(b, inline)
}

// writeLoonRules 输出 Loon 的 [Remote Rule] 与 [Rule] 段。
func writeLoonRules(b *strings.Builder, cfg *MihomoSubscriptionConfig, _ []string) {
	inline, remote := translateRulesToClient(cfg, true)

	if len(remote) > 0 {
		b.WriteString("[Remote Rule]\n")
		for _, rs := range remote {
			policy := ruleSetPolicy(inline, rs.Tag)
			b.WriteString(fmt.Sprintf("%s, policy = %s, tag = %s, enabled = true\n", rs.URL, policy, rs.Tag))
		}
		b.WriteString("\n")
	}

	writeRuleSection(b, inline)
}

// ruleSetPolicy 找出该 tag 在 [Rule] 里使用的策略。
func ruleSetPolicy(inline []string, tag string) string {
	prefix := "RULE-SET," + tag + ","
	for _, l := range inline {
		if strings.HasPrefix(l, prefix) {
			return strings.TrimPrefix(l, prefix)
		}
	}
	return "DIRECT"
}

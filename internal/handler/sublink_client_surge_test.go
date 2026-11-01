package handler

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sboard-go/sboard/internal/database"
)

// Surge 节点行测试。
//
// Surge 只能在 iOS / Mac 上验证，本机无法实跑，所以这里逐字段核对官方
// 语法（manual.nssurge.com）：
//
//	ss          用 encrypt-method，不是 method
//	vmess       用 username=<UUID>，没有 alterId
//	tuic        必须 tuic-v5 + uuid + password
//	hysteria2   用 password，不是 auth
//	socks5      用户名密码是位置参数
//	TLS SNI     用 sni，不是 peer
//	不支持的协议 返回空串（调用方丢弃该节点）

func newSurgeTestNode(protocol string) *database.InboundNode {
	n := &database.InboundNode{
		Protocol:        protocol,
		TlsEnabled:      true,
		ServerName:      "down.dingtalk.com",
		TransportType:   "tcp",
		SsMethod:        "aes-256-gcm",
		SsPassword:      "sspass",
		Hy2Password:     "hy2pass",
		Hy2Obfs:         "salamander",
		Hy2ObfsPassword: "obfspass",
		TuicPassword:    "tuicpass",
	}
	return n
}

func TestBuildSurgeProxyLine(t *testing.T) {
	user := &database.ProxyUser{Name: "u1"}
	uuid := "11111111-2222-3333-4444-555555555555"

	tests := []struct {
		name      string
		protocol  string
		wantHead  string   // 期望包含的片段
		mustHave  []string // 必须出现的字段
		mustNot   []string // 不能出现的字段（客户端不接受）
		wantEmpty bool
	}{
		{
			name: "shadowsocks 用 encrypt-method", protocol: "shadowsocks",
			wantHead: "ss, 1.2.3.4, 8388",
			mustHave: []string{"encrypt-method=aes-256-gcm", "password=sspass", "udp-relay=true"},
			mustNot:  []string{", method="},
		},
		{
			name: "vmess 用 username 且无 alterId", protocol: "vmess",
			wantHead: "vmess, 1.2.3.4, 8388",
			mustHave: []string{"username=" + uuid, "tls=true", "sni=down.dingtalk.com", "skip-cert-verify=true"},
			mustNot:  []string{"alterId", "peer=", "password="},
		},
		{
			name: "trojan 不写 tls=true", protocol: "trojan",
			wantHead: "trojan, 1.2.3.4, 8388",
			mustHave: []string{"password=" + uuid, "sni=down.dingtalk.com", "skip-cert-verify=true"},
			mustNot:  []string{"tls=true", "peer="},
		},
		{
			name: "hysteria2 用 password 不是 auth", protocol: "hysteria2",
			wantHead: "hysteria2, 1.2.3.4, 8388",
			mustHave: []string{"password=hy2pass", "obfs-password=obfspass"},
			mustNot:  []string{"auth="},
		},
		{
			name: "tuic 必须 tuic-v5 且 uuid+password", protocol: "tuic",
			wantHead: "tuic-v5, 1.2.3.4, 8388",
			mustHave: []string{"uuid=" + uuid, "password=tuicpass", "alpn=h3"},
			mustNot:  []string{"congestion-controller"},
		},
		{
			name: "anytls 用 password", protocol: "anytls",
			wantHead: "anytls, 1.2.3.4, 8388",
			mustHave: []string{"password=" + uuid},
			mustNot:  []string{"tls=true"},
		},
		{
			name: "socks 用 socks5 与位置参数", protocol: "socks",
			wantHead: "socks5, 1.2.3.4, 8388, u1, ",
			mustHave: []string{"udp-relay=true"},
			mustNot:  []string{"socks,", "username="},
		},
		{
			name: "vless Surge 不支持", protocol: "vless", wantEmpty: true,
		},
		{
			name: "naive Surge 不支持", protocol: "naive", wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := newSurgeTestNode(tc.protocol)
			got := buildSurgeProxyLine("测试节点", "1.2.3.4", 8388, node, user, uuid)

			if tc.wantEmpty {
				if got != "" {
					t.Fatalf("Surge 应跳过该协议，却生成了: %s", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("生成了空行")
			}
			if !strings.Contains(got, tc.wantHead) {
				t.Errorf("头部不符\n期望包含: %s\n实际: %s", tc.wantHead, got)
			}
			for _, w := range tc.mustHave {
				if !strings.Contains(got, w) {
					t.Errorf("缺少字段 %q\n实际: %s", w, got)
				}
			}
			for _, w := range tc.mustNot {
				if strings.Contains(got, w) {
					t.Errorf("出现非法字段 %q\n实际: %s", w, got)
				}
			}
		})
	}
}

// Surge 不支持 Reality 与 ws 之外的传输，这些节点必须整条跳过，
// 否则 Surge 载入配置时会丢弃该行并告警。
func TestBuildSurgeSkipsUnsupported(t *testing.T) {
	user := &database.ProxyUser{Name: "u1"}
	uuid := "11111111-2222-3333-4444-555555555555"

	reality := newSurgeTestNode("vless")
	reality.RealityEnabled = true
	reality.RealityPubkey = "pubkey"
	if got := buildSurgeProxyLine("n", "1.2.3.4", 8388, reality, user, uuid); got != "" {
		t.Errorf("Reality 节点应跳过，实际: %s", got)
	}

	for _, tr := range []string{"grpc", "http", "httpupgrade"} {
		n := newSurgeTestNode("vmess")
		n.TransportEnabled = true
		n.TransportType = tr
		if got := buildSurgeProxyLine("n", "1.2.3.4", 8388, n, user, uuid); got != "" {
			t.Errorf("传输 %s Surge 不支持，应跳过，实际: %s", tr, got)
		}
	}

	// ws 必须支持，且多值头用竖线分隔的写法
	ws := newSurgeTestNode("vmess")
	ws.TransportEnabled = true
	ws.TransportType = "ws"
	ws.WsPath = "/v2"
	ws.TransportHost = "example.com"
	got := buildSurgeProxyLine("n", "1.2.3.4", 8388, ws, user, uuid)
	for _, w := range []string{"ws=true", "ws-path=/v2", "ws-headers=Host:example.com"} {
		if !strings.Contains(got, w) {
			t.Errorf("ws 节点缺少 %q\n实际: %s", w, got)
		}
	}
}

// IPv6 入口的 host 在 .conf 里必须写成裸地址。
//
// Surge 对 [Proxy] 行的 hostname 字段做严格校验，[2406:...] 会直接报
// 「字段 hostname 的值无效」并拒绝加载整份配置；小火箭 / Loon 同理，
// 方括号只是 URL 里的写法。URL 场景（v2ray 链接）仍然要加括号。
func TestClientConfIPv6HostWithoutBrackets(t *testing.T) {
	const v6 = "2406:da18:1cab:e00:e734:b603:81fb:3862"
	user := &database.ProxyUser{Name: "u1"}
	uuid := "11111111-2222-3333-4444-555555555555"

	node := newSurgeTestNode("trojan")
	// 直接验证三个客户端的建行函数拿到裸 host 时的输出
	for name, build := range map[string]func() string{
		"surge": func() string {
			return buildSurgeProxyLine("n", v6, 18808, node, user, uuid)
		},
		"shadowrocket": func() string {
			return buildShadowrocketProxyLine("n", v6, 18808, node, user, uuid)
		},
		"loon": func() string {
			return buildLoonProxyLine("n", v6, 18808, node, user, uuid)
		},
	} {
		got := build()
		if got == "" {
			t.Fatalf("%s 生成了空行", name)
		}
		if strings.Contains(got, "[") || strings.Contains(got, "]") {
			t.Errorf("%s 的 host 不该带方括号: %s", name, got)
		}
		if !strings.Contains(got, v6) {
			t.Errorf("%s 缺少裸 IPv6 地址: %s", name, got)
		}
	}

	// 对照：buildClientNode 必须传裸 host（不能走 formatHostForURL）
	if got := formatHostForURL(v6); got != "["+v6+"]" {
		t.Errorf("URL 场景仍应加方括号，实际: %s", got)
	}
}

// VoWiFi 是 UDP 流量（IKE 500 / NAT-T 4500），策略必须能转发 UDP。
//
// Surge 各协议的 UDP 转发能力不同：
//
//	Trojan / VMess / TUIC / Hysteria2 / AnyTLS  自动支持
//	Shadowsocks / SOCKS5                        必须显式 udp-relay=true
//	HTTP / HTTPS                                不支持
//
// 漏掉 udp-relay 时这些 UDP 包会被 udp-policy-not-supported-behaviour
// 处理（新版本默认 REJECT），Wi-Fi Calling 直接不通。
func TestSurgeUDPRelayCoverage(t *testing.T) {
	user := &database.ProxyUser{Name: "u1"}
	uuid := "11111111-2222-3333-4444-555555555555"

	// 需要显式开启的两种协议
	for _, proto := range []string{"shadowsocks", "socks"} {
		n := newSurgeTestNode(proto)
		got := buildSurgeProxyLine("n", "1.2.3.4", 8388, n, user, uuid)
		if !strings.Contains(got, "udp-relay=true") {
			t.Errorf("%s 必须显式 udp-relay=true，否则 VoWiFi 的 UDP 不通\n实际: %s", proto, got)
		}
	}

	// 自动支持 UDP 的协议：不写 udp-relay 也能转发，写了也不算错，
	// 这里只断言节点行能正常生成
	for _, proto := range []string{"trojan", "vmess", "tuic", "hysteria2", "anytls"} {
		n := newSurgeTestNode(proto)
		if got := buildSurgeProxyLine("n", "1.2.3.4", 8388, n, user, uuid); got == "" {
			t.Errorf("%s 应生成节点行（该协议自动支持 UDP）", proto)
		}
	}
}

// 名字里没有国家关键词的节点，也必须按 GeoIP 结果进组。
//
// 回归用例来自用户实测：la-megabox 的入口 104.194.70.123 GeoIP 是 US，
// 名字里却没有任何美国关键词。小火箭 / Loon 用服务端算好的成员列表，
// 组里正常包含它；Surge 只靠 policy-regex-filter 匹配名字，于是漏掉。
// 所以 geoip-country 组的过滤正则必须并入「精确成员名」。
func TestSurgeGeoGroupFilterIncludesExactNames(t *testing.T) {
	pg := MihomoProxyGroup{Name: "AI", Type: "select", FilterMode: "geoip-country", Filter: "US,JP,SG"}
	members := []string{"aws-jp", "la-megabox"}

	filter := surgeGroupFilter(pg, members)
	if filter == "" {
		t.Fatal("过滤正则不该为空")
	}

	re, err := regexp.Compile(filter)
	if err != nil {
		t.Fatalf("生成的正则无法编译: %v\n%s", err, filter)
	}
	for _, n := range members {
		if !re.MatchString(n) {
			t.Errorf("成员 %q 应被匹配到\n正则: %s", n, filter)
		}
	}
	// 名字含国家关键词的、当前不在成员里的节点，也允许自动进组
	if !re.MatchString("美国-新节点") {
		t.Errorf("含国家关键词的新节点应自动进组\n正则: %s", filter)
	}
	// 不该把无关节点卷进来
	if re.MatchString("hkix-hytron") {
		t.Errorf("HK 节点不该进 AI 组\n正则: %s", filter)
	}
}

// regex 模式的组保持用户自己写的正则，不并入精确名单。
func TestSurgeRegexModeGroupKeepsUserFilter(t *testing.T) {
	pg := MihomoProxyGroup{Name: "HK-VoWiFi", Type: "select", FilterMode: "regex", Filter: "(?i)香港|HKG|hkix|HK"}
	members := []string{"hkix-hytron"}

	got := surgeGroupFilter(pg, members)
	if got != "(?i)香港|HKG|hkix|HK" {
		t.Errorf("regex 模式应原样使用用户的过滤条件，实际: %s", got)
	}
}

// Surge 用 policy-path 从订阅拉节点，不内嵌 [Proxy]。
//
// 这是 Surge 独有的能力：配置里只有策略组和规则，节点由订阅动态提供，
// 用户等级调整或节点增删都不需要重新导入配置。
func TestSurgeGroupsUsePolicyPath(t *testing.T) {
	const provider = "https://example.com/subscribe/uuid/trojan/surge-provider/sboard.conf"

	cfg := &MihomoSubscriptionConfig{}
	cfg.Config.ProxyGroups = []MihomoProxyGroup{
		{Name: "国外", Type: "select", FilterMode: "geoip-cn", Filter: "!cn"},
		{Name: "国内", Type: "select", FilterMode: "geoip-cn", Filter: "cn"},
		{Name: "HK-VoWiFi", Type: "select", FilterMode: "geoip-country", Filter: "HK"},
	}

	// 当前只有香港节点：国外组能筛到，国内组筛不到但带 DIRECT，AI 类组无节点
	allNodes := []string{"香港", "hkix-hytron"}
	countryNodes := map[string][]string{"HK": {"香港", "hkix-hytron"}}

	groups := buildSurgeGroupsWithProvider(cfg, provider, allNodes, countryNodes)
	joined := strings.Join(groups, "\n")

	// 国外组：有节点，必须带 policy-path
	if !strings.Contains(joined, "国外 = select, policy-path="+provider) {
		t.Errorf("国外组应带 policy-path\n%s", joined)
	}
	// 国内组：显式带 DIRECT，即使当前没有国内节点也要保留 policy-path，
	// 这样以后出现国内节点会自动收进来
	if !strings.Contains(joined, "国内 = select, DIRECT, policy-path="+provider) {
		t.Errorf("国内组应保留 policy-path 并含 DIRECT\n%s", joined)
	}
	// 两组都要有刷新间隔和正则筛选
	if strings.Count(joined, "update-interval=86400") < 2 {
		t.Errorf("policy-path 组缺少 update-interval\n%s", joined)
	}
	if !strings.Contains(joined, "policy-regex-filter=") {
		t.Errorf("policy-path 组缺少 policy-regex-filter\n%s", joined)
	}

	// 筛不到节点的组必须退化成 DIRECT，不能留空组（Surge 会拒绝加载）
	emptyCfg := &MihomoSubscriptionConfig{}
	emptyCfg.Config.ProxyGroups = []MihomoProxyGroup{
		{Name: "AI", Type: "select", FilterMode: "geoip-country", Filter: "US,JP"},
	}
	emptyGroups := buildSurgeGroupsWithProvider(emptyCfg, provider, allNodes, countryNodes)
	if len(emptyGroups) != 1 || emptyGroups[0] != "AI = select, DIRECT" {
		t.Errorf("筛不到节点时应退化为只有 DIRECT，实际: %v", emptyGroups)
	}
}

// surge-provider 清单只输出节点行，不带段头。
func TestSurgeNodeListHasNoSectionHeader(t *testing.T) {
	user := &database.ProxyUser{Name: "u1", UUID: "11111111-2222-3333-4444-555555555555"}
	_ = user
	// 空数据时给出说明性注释而不是空文件
	out, err := generateSurgeNodeList(nil, nil, user, 1, nil)
	if err != nil {
		t.Fatalf("生成清单失败: %v", err)
	}
	if out == "" {
		t.Error("空清单也应给出注释，不能返回空字符串")
	}
	if strings.Contains(out, "[Proxy]") {
		t.Errorf("policy-path 清单不该带段头: %s", out)
	}
}

// Surge 的 [General] 不能出现 Shadowrocket 的键。
func TestSurgeGeneralKeys(t *testing.T) {
	var b strings.Builder
	writeClientGeneral(&b, formatSurge, "https://example.com/x.conf")
	out := b.String()

	for _, bad := range []string{"bypass-tun", "fallback-dns-server", "private-ip-answer", "update-url"} {
		if strings.Contains(out, bad) {
			t.Errorf("[General] 出现 Surge 不识别的键 %q\n%s", bad, out)
		}
	}
	for _, good := range []string{"loglevel", "dns-server", "skip-proxy", "proxy-test-url", "test-timeout"} {
		if !strings.Contains(out, good) {
			t.Errorf("[General] 缺少 %q\n%s", good, out)
		}
	}

	// VoWiFi 要能被代理，必须让 Surge 接管蜂窝服务流量。
	// include-cellular-services 必须与 include-all-networks 同时开启，
	// 否则手册明确说明不生效。
	for _, mustHave := range []string{"include-all-networks = true", "include-cellular-services = true"} {
		if !strings.Contains(out, mustHave) {
			t.Errorf("[General] 缺少 VoWiFi 接管开关 %q\n%s", mustHave, out)
		}
	}

	// DIRECT 策略的测速地址必须是境内可达的，否则测速面板永远显示失败
	if !strings.Contains(out, "internet-test-url = "+surgeInternetTestURL) {
		t.Errorf("[General] internet-test-url 应为境内地址 %q\n%s", surgeInternetTestURL, out)
	}
	// 兜底行为显式声明，避免静默直连泄露
	if !strings.Contains(out, "udp-policy-not-supported-behaviour = REJECT") {
		t.Errorf("[General] 应显式声明 udp-policy-not-supported-behaviour=REJECT\n%s", out)
	}
	// skip-proxy 不能含 CGNAT 网段，否则运营商 IMS/ePDG 流量会绕过代理
	if strings.Contains(out, "100.64.0.0/10") {
		t.Errorf("[General] skip-proxy 含 100.64.0.0/10，会挡住 VoWiFi\n%s", out)
	}
	// 境内不需要被污染的公共 DNS
	for _, bad := range []string{"8.8.8.8", "1.1.1.1"} {
		if strings.Contains(out, bad) {
			t.Errorf("[General] dns-server 不该含 %s（境内被污染）\n%s", bad, out)
		}
	}
}

// 规则层：Surge 用 DEST-PORT，其它客户端用 DST-PORT；私有 IP 段用内置 LAN。
func TestSurgeRuleTranslation(t *testing.T) {
	// resolveRuleSet 会读 rule_set_proxy 设置，需要可用的数据库
	if err := database.Init(filepath.Join(t.TempDir(), "surge-test.db")); err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	cfg := &MihomoSubscriptionConfig{}
	cfg.Config.Rules = []MihomoRule{
		{Type: "DST-PORT", Value: "500/4500", Outbound: "wificall"},
		{Type: "RULE-SET", Value: "private_ip", Outbound: "DIRECT"},
		{Type: "RULE-SET", Value: "cn_ip", Outbound: "国内"},
		{Type: "MATCH", Outbound: "国外"},
	}

	surge, _ := translateRulesToClient(cfg, formatSurge)
	joined := strings.Join(surge, "\n")
	for _, w := range []string{"DEST-PORT,500,wificall", "DEST-PORT,4500,wificall", "RULE-SET,LAN,DIRECT"} {
		if !strings.Contains(joined, w) {
			t.Errorf("Surge 规则缺少 %q\n%s", w, joined)
		}
	}
	if strings.Contains(joined, "DST-PORT,") {
		t.Errorf("Surge 不该出现 DST-PORT\n%s", joined)
	}

	sr, _ := translateRulesToClient(cfg, formatShadowrocket)
	srJoined := strings.Join(sr, "\n")
	if !strings.Contains(srJoined, "DST-PORT,500,wificall") {
		t.Errorf("Shadowrocket 应保持 DST-PORT\n%s", srJoined)
	}
	if strings.Contains(srJoined, "RULE-SET,LAN,") {
		t.Errorf("Shadowrocket 不该用 Surge 内置 LAN\n%s", srJoined)
	}
}

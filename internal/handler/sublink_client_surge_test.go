package handler

import (
	"path/filepath"
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
		Protocol:       protocol,
		TlsEnabled:     true,
		ServerName:     "down.dingtalk.com",
		TransportType:  "tcp",
		SsMethod:       "aes-256-gcm",
		SsPassword:     "sspass",
		Hy2Password:    "hy2pass",
		Hy2Obfs:        "salamander",
		Hy2ObfsPassword: "obfspass",
		TuicPassword:   "tuicpass",
	}
	return n
}

func TestBuildSurgeProxyLine(t *testing.T) {
	user := &database.ProxyUser{Name: "u1"}
	uuid := "11111111-2222-3333-4444-555555555555"

	tests := []struct {
		name     string
		protocol string
		wantHead string   // 期望包含的片段
		mustHave []string // 必须出现的字段
		mustNot  []string // 不能出现的字段（客户端不接受）
		wantEmpty bool
	}{
		{
			name: "shadowsocks 用 encrypt-method", protocol: "shadowsocks",
			wantHead: "ss, 1.2.3.4, 8388",
			mustHave: []string{"encrypt-method=aes-256-gcm", "password=sspass"},
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

package handler

import (
	"encoding/json"
	"fmt"

	"github.com/sboard-go/sboard/internal/database"
	"gopkg.in/yaml.v3"
)

// Provider 文件生成。
//
// provider 是客户端侧的「节点池」机制：主配置只管策略组和规则，节点从
// provider 地址动态拉取。节点增删时客户端刷新 provider 即可，主配置不用动。
//
// ===== mihomo =====
// 官方原生支持，字段稳定：
//
//	proxy-providers:
//	  sboard:
//	    type: http
//	    url: <面板>/subscribe/<uuid>/<node>/mihomo/sboard.yaml
//	    interval: 3600
//	    path: ./providers/sboard.yaml
//	    health-check: {enable: true, url: ..., interval: 300}
//
//	provider 文件内容：{proxies: [...]}
//
// ===== sing-box =====
// 官方 1.14 已无 provider（实测 providers / outbound_providers /
// selector.providers 全报 unknown field）。支持它的是 reF1nd/sing-box
// 的 reF1nd-stable 分支，见 option/provider.go 与 provider/parser/。
//
// 主配置 providers 元素（注意 type 只有 local / remote）：
//
//	"providers": [{
//	  "type": "remote",
//	  "tag": "sboard",
//	  "url": "...",
//	  "path": "./providers/sboard.json",
//	  "update_interval": "24h",      // 最小 1h，默认 24h
//	  "health_check": {"enabled": true, "url": "...", "interval": "10m"},
//	  "exclude": "", "include": "",   // 正则过滤节点
//	  "override_dialer": {}, "override_tls": {}, "override_anytls": {}
//	}]
//
//	selector 用 providers 字段引用：{"type":"selector","tag":"Proxy",
//	  "providers":["sboard"],"outbounds":["direct"]}
//
// provider 文件本身是标准 sing-box 配置，parser 只读 outbounds 与
// endpoints 两个键（_SingBoxDocument），并主动跳过
// direct/block/dns/selector/urltest/pass 这六种类型：
//
//	{ "outbounds": [...] }
//
// 所以这里只输出真实节点，不放组和内建出站。

// generateMihomoProvider 生成 mihomo proxy-provider 文件。
// 结构：{proxies: [...]}
func generateMihomoProvider(servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (string, error) {
	var proxies []MihomoProxy

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
			if p := buildMihomoProxy(&swn.Server, &node, nc, user, lv, nil); p != nil {
				proxies = append(proxies, p)
			}

			// 落地出站节点
			for _, ob := range swn.Outbounds {
				extraUUID, err := user.EnsureExtraUUID(ob.Slot)
				if err != nil {
					return "", fmt.Errorf("生成落地出站 UUID 失败: %w", err)
				}
				if extraUUID == "" {
					continue
				}
				opts := &ProxyCustomOptions{
					CustomName: swn.Server.Name + "-" + ob.Remark,
					CustomUUID: extraUUID,
				}
				if p := buildMihomoProxy(&swn.Server, &node, nc, user, lv, opts); p != nil {
					proxies = append(proxies, p)
				}
			}
		}
	}

	// 外部节点
	for _, ext := range externalNodes {
		if p := buildMihomoProxyFromExternal(&ext); p != nil {
			proxies = append(proxies, p)
		}
	}

	doc := map[string]interface{}{"proxies": proxies}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// generateSingBoxProvider 生成 sing-box provider 文件。
//
// 结构：{outbounds: [...]}
//
// 只输出真实节点。reF1nd-stable 的 provider/parser/sing_box.go 会把
// direct/block/dns/selector/urltest/pass 过滤掉，放进去也是无效内容。
func generateSingBoxProvider(servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (string, error) {
	var outbounds []map[string]interface{}

	for _, swn := range servers {
		for _, node := range swn.Nodes {
			if !node.Enabled {
				continue
			}
			var nc *database.ServerNodeConfig
			if sc, ok := nodeConfigs[swn.Server.ID]; ok {
				nc = sc[node.ID]
			}

			if ob := buildSingBoxOutbound(&swn.Server, &node, nc, user, lv, nil); ob != nil {
				outbounds = append(outbounds, ob)
			}

			for _, ob := range swn.Outbounds {
				extraUUID, err := user.EnsureExtraUUID(ob.Slot)
				if err != nil {
					return "", fmt.Errorf("生成落地出站 UUID 失败: %w", err)
				}
				if extraUUID == "" {
					continue
				}
				opts := &ProxyCustomOptions{
					CustomName: swn.Server.Name + "-" + ob.Remark,
					CustomUUID: extraUUID,
				}
				if o := buildSingBoxOutbound(&swn.Server, &node, nc, user, lv, opts); o != nil {
					outbounds = append(outbounds, o)
				}
			}
		}
	}

	for _, ext := range externalNodes {
		if o := buildSingBoxOutboundFromExternal(&ext); o != nil {
			outbounds = append(outbounds, o)
		}
	}

	doc := map[string]interface{}{"outbounds": outbounds}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

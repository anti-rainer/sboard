package handler

import (
	"fmt"
	"strings"

	"github.com/sboard-go/sboard/internal/database"
)

// 客户端节点收集：把服务器、落地出站、外部节点统一转成客户端节点行。

// clientNode 一个待输出的客户端节点。
type clientNode struct {
	Name    string
	Line    string
	Country string // 用于地区策略组筛选
}

// buildAllClientNodes 构建全部节点（直连 + 落地出站 + 外部节点）。
func buildAllClientNodes(fmtType clientFormat, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) []clientNode {
	var out []clientNode
	seen := make(map[string]bool)

	add := func(n clientNode) {
		if n.Line == "" || n.Name == "" {
			return
		}
		// 重名会让客户端只保留后者，加序号区分
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

			// 直连节点（主 UUID）
			cc := getCountryCode(getNodeEffectiveIP(&swn.Server, &node, nc))
			if n := buildClientNode(fmtType, &swn.Server, &node, nc, user, lv, nil); n.Line != "" {
				n.Country = cc
				add(n)
			}

			// 落地出站节点（额外 UUID）
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

// collectClientNodes 收集节点行与名称。
func collectClientNodes(fmtType clientFormat, servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (lines []string, names []string) {
	for _, n := range buildAllClientNodes(fmtType, servers, nodeConfigs, user, lv, externalNodes) {
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

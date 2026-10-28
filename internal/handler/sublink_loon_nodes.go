package handler

import (
	"fmt"
	"strings"

	"github.com/sboard-go/sboard/internal/database"
)

// generateLoonNodeList 生成供 Loon [Remote Proxy] 引用的纯节点清单。
//
// Loon 的节点订阅可以接受：
//   - Loon 标准节点行（name = Type,host,port,...）
//   - 常见节点 URI（ss:// vmess:// trojan:// ...）
//   - Base64 编码的 URI 列表
//
// 这里输出 Loon 标准节点行，带 [Proxy] 段头，是 Loon 最能直接识别的形式。
// 配置与节点分离后，用户在 [Remote Proxy] 里引用这个地址；
// 节点增删只影响本清单，配置文件无需重新导入。
func generateLoonNodeList(servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (string, error) {
	nodes := buildAllClientNodes(formatLoon, servers, nodeConfigs, user, lv, externalNodes)

	var b strings.Builder
	b.WriteString("[Proxy]\n")
	if len(nodes) == 0 {
		b.WriteString("# 暂无可用节点\n")
		return b.String(), nil
	}
	for _, n := range nodes {
		b.WriteString(n.Line)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// loonNodeListMeta 供调试：返回节点数量与名称，便于排查筛选问题。
func loonNodeListMeta(servers []ServerWithNodes, nodeConfigs map[uint]map[uint]*database.ServerNodeConfig, user *database.ProxyUser, lv int, externalNodes []database.ExternalNode) (int, []string) {
	nodes := buildAllClientNodes(formatLoon, servers, nodeConfigs, user, lv, externalNodes)
	names := make([]string, 0, len(nodes))
	for _, n := range nodes {
		names = append(names, n.Name)
	}
	return len(nodes), names
}

var _ = fmt.Sprintf

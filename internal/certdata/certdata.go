package certdata

import _ "embed"

// ServerCert 是服务端内置的伪装证书（自签，CN=*.dingtalk.com，有效期 90 天）。
//
// 面板生成客户端配置时要把这张证书内联进 naive 出站：
// naive 出站不支持 insecure（FATAL: insecure is not supported on naive
// outbound），只能靠内联 certificate 建立信任。
//
// 有效期必须 ≤ 90 天：naive 走 Chromium 的 cronet 证书栈，超过限制会报
//   cert validity too long
// 实测 180 天可行、270 天不行。
//
//go:embed server.crt
var ServerCert string

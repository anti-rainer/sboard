/**
 * 233boy 全部节点格式识别测试（33 种）
 *
 * 格式来源是两个脚本里唯一生成分享链接的函数 info()：
 *   233boy/sing-box  src/core.sh L1364-1471   (protocol_list 21 项)
 *   233boy/Xray      src/core.sh L1548-1624   (protocol_list 独有 5 项)
 *
 * 33 种 = sing-box 21 项 + Xray 独有 5 项 + ss 另外 6 种加密方式
 *        + AnyTLS 的域名/IP 两种写法
 *
 * 关键背景一：233boy 默认 password = uuid（sing-box core.sh L1171/1177/
 *   L1187/L1208 全是 `[[ ! $password ]] && password=$uuid`），所以密码
 *   本来就是 UUID，解析器绝不能把它当 base64 去解。
 *
 * 关键背景二：带 host 的 TLS 分支（Caddy 反代）里，URL 的 authority
 *   直接就是伪装域名（`@$host:$is_https_port`），且查询串里没有 sni，
 *   只有 host=。所以 sni 必须能从 host 参数回退出来，否则 TLS 节点
 *   导入后 server_name 是空的。
 *
 * 跑法：npx esbuild plink-233boy-test.ts --bundle --platform=node \
 *        --format=cjs --outfile=/tmp/p2.cjs && node /tmp/p2.cjs
 */
import { parseProxyLink, type ParsedProxy } from '../src/utils/proxyLink'

const UUID = 'ed5e3abd-da84-4653-8bc3-efe1dffc6fa0'
const ADDR = '23.81.36.175'
const HOST = 'cdn.example.com'
const b64 = (s: string) => Buffer.from(s).toString('base64')
/** xray 脚本会 sed 's#/#%2F#g' 把 path 百分号编码 */
const enc = (p: string) => p.replace(/\//g, '%2F')
const vmess = (o: Record<string, unknown>) => 'vmess://' + b64(JSON.stringify(o))

interface Case {
  no: number
  what: string
  link: string
  want: Partial<ParsedProxy>
}

const CASES: Case[] = [
  // ═══ sing-box 脚本：VMess 家族（全部走 JSON 模板） ═══════════
  {
    no: 1, what: 'sing-box · VMess-WS（无 host，纯 ws）',
    link: vmess({ v: 2, ps: `233boy-ws-${ADDR}`, add: ADDR, port: '8443', id: UUID, aid: '0', net: 'ws', type: 'none' }),
    want: { protocol: 'vmess', uuid: UUID, host: ADDR, port: 8443, network: 'ws', tls: false },
  },
  {
    no: 2, what: 'sing-box · VMess-TCP',
    link: vmess({ v: 2, ps: `233boy-tcp-${ADDR}`, add: ADDR, port: '8443', id: UUID, aid: '0', net: 'tcp', type: 'none' }),
    want: { network: '', tls: false },
  },
  {
    no: 3, what: 'sing-box · VMess-HTTP（net 被脚本改成 tcp，type=http）',
    link: vmess({ v: 2, ps: `233boy-tcp http-${ADDR}`, add: ADDR, port: '8443', id: UUID, aid: '0', net: 'tcp', type: 'http' }),
    want: { network: 'http' },
  },
  {
    no: 4, what: 'sing-box · VMess-QUIC（net=quic + tls + alpn=h3）',
    link: vmess({ v: 2, ps: `233boy-quic-${ADDR}`, add: ADDR, port: '8443', id: UUID, aid: '0', net: 'quic', type: 'none', tls: 'tls', alpn: 'h3' }),
    want: { network: 'quic', tls: true, alpn: 'h3' },
  },
  {
    no: 5, what: 'sing-box · VMess-H2-TLS（Caddy 分支）',
    link: vmess({ v: 2, ps: `233boy-h2-${HOST}`, add: ADDR, port: '443', id: UUID, aid: '0', net: 'h2', host: HOST, path: '/h2', tls: 'tls' }),
    want: { network: 'h2', wsHost: HOST, wsPath: '/h2', tls: true, sni: HOST },
  },
  {
    no: 6, what: 'sing-box · VMess-WS-TLS（Caddy 分支）',
    link: vmess({ v: 2, ps: `233boy-ws-${HOST}`, add: ADDR, port: '443', id: UUID, aid: '0', net: 'ws', host: HOST, path: '/ws', tls: 'tls' }),
    want: { network: 'ws', wsHost: HOST, wsPath: '/ws', tls: true, sni: HOST },
  },
  {
    no: 7, what: 'sing-box · VMess-HTTPUpgrade-TLS',
    link: vmess({ v: 2, ps: `233boy-httpupgrade-${HOST}`, add: ADDR, port: '443', id: UUID, aid: '0', net: 'httpupgrade', host: HOST, path: '/hu', tls: 'tls' }),
    want: { network: 'httpupgrade', wsHost: HOST, wsPath: '/hu', tls: true },
  },

  // ═══ sing-box 脚本：VLESS 家族 ═════════════════════════════
  {
    no: 8, what: 'sing-box · VLESS-WS-TLS（无 sni 参数，靠 host= 回退）',
    link: `vless://${UUID}@${HOST}:443?encryption=none&security=tls&type=ws&host=${HOST}&path=/ws#233boy-ws-${HOST}`,
    want: { protocol: 'vless', uuid: UUID, host: HOST, port: 443, network: 'ws', wsPath: '/ws', wsHost: HOST, tls: true, sni: HOST },
  },
  {
    no: 9, what: 'sing-box · VLESS-H2-TLS',
    link: `vless://${UUID}@${HOST}:443?encryption=none&security=tls&type=h2&host=${HOST}&path=/h2#233boy-h2-${HOST}`,
    want: { network: 'h2', wsPath: '/h2', wsHost: HOST, tls: true, sni: HOST },
  },
  {
    no: 10, what: 'sing-box · VLESS-HTTPUpgrade-TLS',
    link: `vless://${UUID}@${HOST}:443?encryption=none&security=tls&type=httpupgrade&host=${HOST}&path=/hu#233boy-httpupgrade-${HOST}`,
    want: { network: 'httpupgrade', wsPath: '/hu', tls: true },
  },
  {
    no: 11, what: 'sing-box · VLESS-REALITY（flow=xtls-rprx-vision, type=tcp）',
    link: `vless://${UUID}@${ADDR}:443?encryption=none&security=reality&flow=xtls-rprx-vision&type=tcp&sni=www.amazon.com&pbk=Z84J2t2dQmS2sVd8pQrXvA&fp=chrome#233boy-reality-${ADDR}`,
    want: {
      reality: true, tls: true, network: '', flow: 'xtls-rprx-vision',
      sni: 'www.amazon.com', pubKey: 'Z84J2t2dQmS2sVd8pQrXvA', fingerprint: 'chrome',
    },
  },
  {
    no: 12, what: 'sing-box · VLESS-HTTP2-REALITY（flow 为空, type=h2）',
    link: `vless://${UUID}@${ADDR}:443?encryption=none&security=reality&flow=&type=h2&sni=www.amazon.com&pbk=Z84J2t2dQmS2sVd8pQrXvA&fp=chrome#233boy-reality-${ADDR}`,
    want: { reality: true, network: 'h2', flow: '', sni: 'www.amazon.com' },
  },

  // ═══ sing-box 脚本：Trojan 家族 ════════════════════════════
  {
    no: 13, what: 'sing-box · Trojan-WS-TLS（userinfo 是 uuid，core.sh L1375）',
    link: `trojan://${UUID}@${HOST}:443?encryption=none&security=tls&type=ws&host=${HOST}&path=/ws#233boy-ws-${HOST}`,
    want: { protocol: 'trojan', password: UUID, network: 'ws', wsPath: '/ws', sni: HOST, tls: true },
  },
  {
    no: 14, what: 'sing-box · Trojan-H2-TLS',
    link: `trojan://${UUID}@${HOST}:443?encryption=none&security=tls&type=h2&host=${HOST}&path=/h2#233boy-h2-${HOST}`,
    want: { password: UUID, network: 'h2', wsPath: '/h2' },
  },
  {
    no: 15, what: 'sing-box · Trojan-HTTPUpgrade-TLS',
    link: `trojan://${UUID}@${HOST}:443?encryption=none&security=tls&type=httpupgrade&host=${HOST}&path=/hu#233boy-httpupgrade-${HOST}`,
    want: { password: UUID, network: 'httpupgrade', wsPath: '/hu' },
  },
  {
    no: 16, what: 'sing-box · Trojan（裸链接，type=tcp&insecure=1）',
    link: `trojan://${UUID}@${ADDR}:8443?type=tcp&security=tls&insecure=1&allowInsecure=1#233boy-trojan-${ADDR}`,
    want: { password: UUID, host: ADDR, port: 8443, network: '', tls: true, skipCertVerify: true },
  },

  // ═══ sing-box 脚本：其余协议 ══════════════════════════════
  {
    no: 17, what: 'sing-box · Hysteria2（用户报障的那条）',
    link: `hysteria2://${UUID}@${ADDR}:55213?alpn=h3&insecure=1&allowInsecure=1&pinSHA256=E05D86291412D758D792D8E9979291275A11309D4264B00610C30C04BCEC52B7#233boy-hysteria2-${ADDR}`,
    want: { protocol: 'hysteria2', password: UUID, host: ADDR, port: 55213, alpn: 'h3', tls: true, skipCertVerify: true },
  },
  {
    no: 18, what: 'sing-box · TUIC（uuid:password + alpn=h3 + congestion_control）',
    link: `tuic://${UUID}:${UUID}@${ADDR}:443?alpn=h3&insecure=1&allowInsecure=1&congestion_control=bbr#233boy-tuic-${ADDR}`,
    want: { protocol: 'tuic', uuid: UUID, password: UUID, alpn: 'h3', tls: true, skipCertVerify: true },
  },
  {
    no: 19, what: 'sing-box · AnyTLS（走域名，无 insecure）',
    link: `anytls://${UUID}@anytls.example.com:8443#233boy-anytls-anytls.example.com`,
    want: { protocol: 'anytls', password: UUID, host: 'anytls.example.com', tls: true },
  },
  {
    no: 20, what: 'sing-box · AnyTLS（裸 IP，insecure=1&allowInsecure=1）',
    link: `anytls://${UUID}@${ADDR}:8443?insecure=1&allowInsecure=1#233boy-anytls-${ADDR}`,
    want: { protocol: 'anytls', password: UUID, tls: true, skipCertVerify: true },
  },
  {
    no: 21, what: 'sing-box · Socks（base64(user:pass)，用户名默认 233boy）',
    link: `socks://${b64(`233boy:${UUID}`)}@${ADDR}:1080#233boy-socks-${ADDR}`,
    want: { protocol: 'socks5', username: '233boy', password: UUID, host: ADDR, port: 1080 },
  },

  // ═══ Xray 脚本独有 5 项 ═══════════════════════════════════
  {
    no: 22, what: 'xray · VMess-mKCP（net=kcp，path 是 seed，不是 ws path）',
    link: vmess({ v: 2, ps: `233boy-kcp-${ADDR}`, add: ADDR, port: '8443', id: UUID, aid: '0', net: 'kcp', type: 'none', path: 'seed123' }),
    want: { network: 'kcp', tls: false, wsPath: '' },
  },
  {
    no: 23, what: 'xray · VMess-gRPC-TLS',
    link: vmess({ v: 2, ps: `233boy-grpc-${HOST}`, add: ADDR, port: '443', id: UUID, aid: '0', net: 'grpc', host: HOST, path: 'gunsvc', tls: 'tls' }),
    want: { network: 'grpc', grpcService: 'gunsvc', wsHost: HOST, tls: true, sni: HOST },
  },
  {
    no: 24, what: 'xray · VLESS-gRPC-TLS（serviceName=，path 已 %2F 编码）',
    link: `vless://${UUID}@${HOST}:443?encryption=none&security=tls&type=grpc&host=${HOST}&serviceName=${enc('/gunsvc')}#233boy-grpc-${HOST}`,
    want: { protocol: 'vless', network: 'grpc', grpcService: '/gunsvc', tls: true, sni: HOST },
  },
  {
    no: 25, what: 'xray · Trojan-gRPC-TLS',
    link: `trojan://${UUID}@${HOST}:443?encryption=none&security=tls&type=grpc&host=${HOST}&serviceName=${enc('/tjsvc')}#233boy-grpc-${HOST}`,
    want: { protocol: 'trojan', password: UUID, network: 'grpc', grpcService: '/tjsvc' },
  },
  {
    no: 26, what: 'xray · VLESS-XHTTP-TLS（path 已 %2F 编码）',
    link: `vless://${UUID}@${HOST}:443?encryption=none&security=tls&type=xhttp&host=${HOST}&path=${enc('/xhttp')}#233boy-xhttp-${HOST}`,
    want: { network: 'xhttp', wsPath: '/xhttp', wsHost: HOST, tls: true },
  },

  // ═══ Shadowsocks：7 种加密方式（链接形状相同，密码不同） ═══
  {
    no: 27, what: 'ss · aes-128-gcm',
    link: `ss://${b64(`aes-128-gcm:${UUID}`)}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { protocol: 'shadowsocks', method: 'aes-128-gcm', password: UUID },
  },
  {
    no: 28, what: 'ss · aes-256-gcm',
    link: `ss://${b64(`aes-256-gcm:${UUID}`)}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { method: 'aes-256-gcm', password: UUID },
  },
  {
    no: 29, what: 'ss · chacha20-ietf-poly1305',
    link: `ss://${b64(`chacha20-ietf-poly1305:${UUID}`)}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { method: 'chacha20-ietf-poly1305', password: UUID },
  },
  {
    no: 30, what: 'ss · xchacha20-ietf-poly1305',
    link: `ss://${b64(`xchacha20-ietf-poly1305:${UUID}`)}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { method: 'xchacha20-ietf-poly1305', password: UUID },
  },
  {
    no: 31, what: 'ss2022 · 2022-blake3-aes-128-gcm（密码是 16 字节 base64）',
    link: `ss://${b64('2022-blake3-aes-128-gcm:YWJjZGVmZ2hpamtsbW5v')}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { method: '2022-blake3-aes-128-gcm', password: 'YWJjZGVmZ2hpamtsbW5v' },
  },
  {
    no: 32, what: 'ss2022 · 2022-blake3-aes-256-gcm（密码是 32 字节 base64）',
    link: `ss://${b64('2022-blake3-aes-256-gcm:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=')}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { method: '2022-blake3-aes-256-gcm', password: 'YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=' },
  },
  {
    no: 33, what: 'ss2022 · 2022-blake3-chacha20-poly1305',
    link: `ss://${b64('2022-blake3-chacha20-poly1305:YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=')}@${ADDR}:8080#233boy-ss-${ADDR}`,
    want: { method: '2022-blake3-chacha20-poly1305', password: 'YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXoxMjM0NTY=' },
  },
]

let pass = 0
const fails: string[] = []

for (const c of CASES) {
  let got: ParsedProxy
  try {
    got = parseProxyLink(c.link)
  } catch (e: any) {
    fails.push(`#${c.no} ${c.what}\n    抛异常: ${e.message}`)
    continue
  }
  const bad: string[] = []
  for (const [k, v] of Object.entries(c.want)) {
    const actual = (got as any)[k]
    if (actual !== v) bad.push(`${k}: 期望 ${JSON.stringify(v)}，实际 ${JSON.stringify(actual)}`)
  }
  if (bad.length) fails.push(`#${c.no} ${c.what}\n    ${bad.join('\n    ')}`)
  else pass++
}

console.log(`\n233boy 全部格式：用例 ${CASES.length} / 通过 ${pass} / 失败 ${fails.length}`)
if (fails.length) {
  console.log('\n失败明细:')
  for (const f of fails) console.log('  ✗ ' + f)
  process.exitCode = 1
}

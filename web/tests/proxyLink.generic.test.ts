/**
 * 快速识别解析器回归测试
 *
 * 用例全部取自真实客户端导出的链接格式（v2rayN / Nekoray / Clash Meta /
 * 233boy 脚本 / sing-box / Surge），只断言解析器应当给出的关键字段。
 *
 * 跑法：npx esbuild plink-test.ts --bundle --platform=node --format=cjs \
 *        --outfile=/tmp/pt.cjs && node /tmp/pt.cjs
 */
import { parseProxyLink, type ParsedProxy } from '../src/utils/proxyLink'

interface Case {
  /** 用例说明 */
  what: string
  link: string
  /** 期望的关键字段，未列出的不校验 */
  want: Partial<ParsedProxy>
}

const CASES: Case[] = [
  // ── hysteria2 ──────────────────────────────────────────────
  {
    what: 'hysteria2：userinfo 是裸 UUID（233boy 脚本导出）+ alpn/insecure/pinSHA256',
    link: 'hysteria2://ed5e3abd-da84-4653-8bc3-efe1dffc6fa0@23.81.36.175:55213?alpn=h3&insecure=1&allowInsecure=1&pinSHA256=E05D86291412D758D792D8E9979291275A11309D4264B00610C30C04BCEC52B7#233boy-hysteria2-23.81.36.175',
    want: {
      protocol: 'hysteria2',
      host: '23.81.36.175',
      port: 55213,
      password: 'ed5e3abd-da84-4653-8bc3-efe1dffc6fa0',
      alpn: 'h3',
      tls: true,
      skipCertVerify: true,
      name: '233boy-hysteria2-23.81.36.175',
    },
  },
  {
    what: 'hysteria2：hy2:// 简写 + obfs + sni',
    link: 'hy2://somepass@example.com:8443?obfs=salamander&obfs-password=secret&sni=cdn.example.com&alpn=h3#hy2-obfs',
    want: {
      protocol: 'hysteria2', host: 'example.com', port: 8443,
      password: 'somepass', obfs: 'salamander', obfsPassword: 'secret',
      sni: 'cdn.example.com', alpn: 'h3',
    },
  },
  {
    what: 'hysteria2：密码含 URL 编码字符',
    link: 'hysteria2://p%40ss%3Aword@1.2.3.4:443?sni=a.com#enc',
    want: { protocol: 'hysteria2', password: 'p@ss:word', host: '1.2.3.4' },
  },
  {
    what: 'hysteria2：带上下行带宽',
    link: 'hysteria2://pass@h.com:443?upmbps=100&downmbps=500#bw',
    want: { upMbps: 100, downMbps: 500 },
  },
  {
    what: 'hysteria2：insecure=0 应关掉跳过校验',
    link: 'hysteria2://pass@h.com:443?insecure=0&sni=a.com#verify',
    want: { skipCertVerify: false },
  },

  // ── vless ─────────────────────────────────────────────────
  {
    what: 'vless Reality + vision',
    link: 'vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?security=reality&pbk=PUBKEY&sid=ab12&sni=www.microsoft.com&fp=chrome&type=tcp&flow=xtls-rprx-vision#reality',
    want: {
      protocol: 'vless', uuid: '11111111-2222-3333-4444-555555555555',
      reality: true, pubKey: 'PUBKEY', shortId: 'ab12',
      sni: 'www.microsoft.com', fingerprint: 'chrome', flow: 'xtls-rprx-vision',
    },
  },
  {
    what: 'vless ws + tls + path/host',
    link: 'vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=ws&path=%2Fws&host=cdn.com&security=tls&sni=cdn.com&fp=chrome#ws',
    want: {
      protocol: 'vless', network: 'ws', wsPath: '/ws', wsHost: 'cdn.com',
      tls: true, sni: 'cdn.com',
    },
  },
  {
    what: 'vless grpc',
    link: 'vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=grpc&serviceName=gunsvc&security=tls&sni=cdn.com#grpc',
    want: { protocol: 'vless', network: 'grpc', grpcService: 'gunsvc' },
  },
  {
    what: 'vless h2',
    link: 'vless://11111111-2222-3333-4444-555555555555@1.2.3.4:443?type=h2&path=%2Fh2&host=cdn.com&security=tls#h2',
    want: { network: 'h2', wsPath: '/h2', wsHost: 'cdn.com' },
  },
  {
    what: 'vless userinfo 为 none:uuid',
    link: 'vless://none:11111111-2222-3333-4444-555555555555@1.2.3.4:443?security=tls#noneuuid',
    want: { uuid: '11111111-2222-3333-4444-555555555555' },
  },
  {
    what: 'vless 无 TLS（security=none）',
    link: 'vless://11111111-2222-3333-4444-555555555555@1.2.3.4:80?security=none&type=tcp#plain',
    want: { tls: false },
  },

  // ── trojan ────────────────────────────────────────────────
  {
    what: 'trojan 标准',
    link: 'trojan://mypass@1.2.3.4:443?sni=a.com&allowInsecure=1#tj',
    want: { protocol: 'trojan', password: 'mypass', tls: true, sni: 'a.com', skipCertVerify: true },
  },
  {
    what: 'trojan ws',
    link: 'trojan://mypass@1.2.3.4:443?type=ws&path=%2Ftj&host=cdn.com&sni=cdn.com#tj-ws',
    want: { network: 'ws', wsPath: '/tj', wsHost: 'cdn.com' },
  },
  {
    what: 'trojan 密码是假的 base64（含 - 的 UUID）',
    link: 'trojan://aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee@1.2.3.4:443#tjuuid',
    want: { password: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee' },
  },

  // ── shadowsocks ───────────────────────────────────────────
  {
    what: 'ss base64(method:password)@host:port',
    link: 'ss://YWVzLTI1Ni1nY206cGFzc3dvcmQ@1.2.3.4:8388#ss1',
    want: { protocol: 'shadowsocks', method: 'aes-256-gcm', password: 'password', port: 8388 },
  },
  {
    what: 'ss 整体 base64',
    link: 'ss://YWVzLTI1Ni1nY206cGFzc3dvcmRAMS4yLjMuNDo4Mzg4#ss2',
    want: { protocol: 'shadowsocks', method: 'aes-256-gcm', password: 'password', host: '1.2.3.4', port: 8388 },
  },
  {
    what: 'ss 明文 method:password',
    link: 'ss://aes-256-gcm:password@1.2.3.4:8388#ss3',
    want: { method: 'aes-256-gcm', password: 'password' },
  },

  // ── vmess ─────────────────────────────────────────────────
  {
    what: 'vmess v2rayN JSON + ws',
    link: 'vmess://' + Buffer.from(JSON.stringify({
      v: '2', ps: 'vm-ws', add: '1.2.3.4', port: '443', id: '11111111-2222-3333-4444-555555555555',
      aid: '0', scy: 'auto', net: 'ws', type: 'none', host: 'cdn.com', path: '/vm',
      tls: 'tls', sni: 'cdn.com',
    })).toString('base64'),
    want: {
      protocol: 'vmess', host: '1.2.3.4', port: 443,
      uuid: '11111111-2222-3333-4444-555555555555',
      network: 'ws', wsPath: '/vm', wsHost: 'cdn.com', tls: true, sni: 'cdn.com',
    },
  },
  {
    what: 'vmess net=tcp + type=http（HTTP 伪装）',
    link: 'vmess://' + Buffer.from(JSON.stringify({
      v: '2', ps: 'vm-http', add: '1.2.3.4', port: '80', id: '11111111-2222-3333-4444-555555555555',
      net: 'tcp', type: 'http', host: 'cdn.com', path: '/h',
    })).toString('base64'),
    want: { network: 'http' },
  },

  // ── tuic / anytls / naive / socks5 ────────────────────────
  {
    what: 'tuic v5',
    link: 'tuic://11111111-2222-3333-4444-555555555555:pass@1.2.3.4:443?congestion_control=bbr&alpn=h3&sni=a.com#tuic',
    want: { protocol: 'tuic', uuid: '11111111-2222-3333-4444-555555555555', password: 'pass', tls: true },
  },
  {
    what: 'anytls',
    link: 'anytls://pass@1.2.3.4:443?sni=a.com&insecure=1#anytls',
    want: { protocol: 'anytls', password: 'pass', tls: true, skipCertVerify: true },
  },
  {
    what: 'socks5 带认证',
    link: 'socks5://user:pwd@1.2.3.4:1080#socks',
    want: { protocol: 'socks5', username: 'user', password: 'pwd', port: 1080 },
  },
]

let pass = 0
const fails: string[] = []

for (const c of CASES) {
  let got: ParsedProxy
  try {
    got = parseProxyLink(c.link)
  } catch (e: any) {
    fails.push(`${c.what}\n    抛异常: ${e.message}`)
    continue
  }
  const bad: string[] = []
  for (const [k, v] of Object.entries(c.want)) {
    const actual = (got as any)[k]
    if (actual !== v) bad.push(`${k}: 期望 ${JSON.stringify(v)}，实际 ${JSON.stringify(actual)}`)
  }
  if (bad.length) fails.push(`${c.what}\n    ${bad.join('\n    ')}`)
  else pass++
}

console.log(`\n用例 ${CASES.length} / 通过 ${pass} / 失败 ${fails.length}`)
if (fails.length) {
  console.log('\n失败明细:')
  for (const f of fails) console.log('  ✗ ' + f)
  process.exitCode = 1
}

/**
 * 代理节点链接解析器
 *
 * 统一处理各家分享链接格式，兼容常见的非标准变体：
 * - 标准形式：  vless://uuid@host:port?params#name
 * - base64 包裹 userinfo：vless://base64(none:uuid@host:port)?params   （部分面板/机场导出）
 * - 整体 base64：ss://base64(method:password@host:port)#name
 * - v2rayN vmess JSON
 *
 * 参数别名：sni/peer/serverName/servername、fp/fingerprint、pbk/publicKey、
 * sid/shortId、xtls→flow、remarks→名称 等。
 */

export type ProxyProtocol =
  | 'vmess'
  | 'vless'
  | 'trojan'
  | 'shadowsocks'
  | 'hysteria2'
  | 'anytls'
  | 'socks5'

export interface ParsedProxy {
  protocol: ProxyProtocol
  name: string
  host: string
  port: number

  // 认证
  uuid: string
  password: string
  method: string // shadowsocks 加密方式
  username: string // socks5 用户名

  // TLS
  tls: boolean
  sni: string
  alpn: string
  fingerprint: string

  // Reality
  reality: boolean
  pubKey: string
  shortId: string
  flow: string

  // 传输层
  network: string // '' | 'ws' | 'grpc' | 'h2' | 'http' | 'httpupgrade'
  wsPath: string
  wsHost: string
  grpcService: string

  // Hysteria2
  obfs: string
  obfsPassword: string
  upMbps: number
  downMbps: number
}

const EMPTY: ParsedProxy = {
  protocol: 'vless',
  name: '',
  host: '',
  port: 443,
  uuid: '',
  password: '',
  method: '',
  username: '',
  tls: false,
  sni: '',
  alpn: '',
  fingerprint: '',
  reality: false,
  pubKey: '',
  shortId: '',
  flow: '',
  network: '',
  wsPath: '',
  wsHost: '',
  grpcService: '',
  obfs: '',
  obfsPassword: '',
  upMbps: 0,
  downMbps: 0,
}

/** 宽松 base64 解码：自动补齐 padding，兼容 URL-safe 与标准字母表 */
export function decodeBase64Flexible(input: string): string {
  let s = input.trim().replace(/-/g, '+').replace(/_/g, '/')
  // 去掉可能存在的空白
  s = s.replace(/\s+/g, '')
  const pad = s.length % 4
  if (pad === 2) s += '=='
  else if (pad === 3) s += '='
  else if (pad === 1) s += '==='

  const binary = atob(s)
  // 还原 UTF-8
  try {
    const bytes = Uint8Array.from(binary, (c) => c.charCodeAt(0))
    return new TextDecoder('utf-8').decode(bytes)
  } catch {
    return binary
  }
}

/** 判断字符串是否像 base64（避免把普通 host 误判） */
function looksLikeBase64(s: string): boolean {
  if (!s || s.length < 8) return false
  return /^[A-Za-z0-9+/\-_]+=*$/.test(s)
}

/** 从查询参数里按别名顺序取第一个非空值 */
function pick(params: URLSearchParams, ...keys: string[]): string {
  for (const k of keys) {
    const v = params.get(k)
    if (v !== null && v !== '') return v
  }
  return ''
}

function truthy(v: string): boolean {
  if (!v) return false
  const s = v.toLowerCase()
  return s === '1' || s === 'true' || s === 'yes' || s === 'tls' || s === 'reality'
}

/** 解析 host:port，支持 IPv6 方括号形式 */
function splitHostPort(value: string, defaultPort: number): { host: string; port: number } {
  let host = value
  let port = defaultPort

  const bracket = value.match(/^\[([^\]]+)\]:(\d+)$/)
  if (bracket) {
    return { host: bracket[1], port: parseInt(bracket[2], 10) || defaultPort }
  }
  const idx = value.lastIndexOf(':')
  if (idx > -1 && !value.slice(idx + 1).includes('/')) {
    const p = parseInt(value.slice(idx + 1), 10)
    if (!Number.isNaN(p)) {
      host = value.slice(0, idx)
      port = p
    }
  }
  return { host, port }
}

/** 应用通用查询参数到结果上 */
function applyParams(out: ParsedProxy, params: URLSearchParams) {
  const security = pick(params, 'security').toLowerCase()
  const pbk = pick(params, 'pbk', 'publicKey', 'public_key')
  const sid = pick(params, 'sid', 'shortId', 'short_id')

  // TLS / Reality 判定：显式 security 优先，其次依据 tls/peer/pbk 等参数推断
  if (security === 'reality' || (pbk && security !== 'none')) {
    out.reality = true
    out.tls = true
    out.pubKey = pbk
    out.shortId = sid
  } else if (security === 'tls' || truthy(pick(params, 'tls'))) {
    out.tls = true
  } else if (security === 'none') {
    out.tls = false
  } else if (pick(params, 'peer', 'sni', 'serverName', 'servername')) {
    out.tls = true
  }

  out.sni = pick(params, 'sni', 'peer', 'serverName', 'servername')
  out.fingerprint = pick(params, 'fp', 'fingerprint') || (out.reality || out.tls ? 'chrome' : '')
  out.alpn = pick(params, 'alpn')
  out.flow = pick(params, 'flow')

  // xtls=1/2 是旧面板的 flow 简写，2 对应 xtls-rprx-vision
  if (!out.flow) {
    const xtls = pick(params, 'xtls')
    if (xtls === '2') out.flow = 'xtls-rprx-vision'
    else if (xtls === '1') out.flow = 'xtls-rprx-direct'
  }

  // 传输层
  const net = pick(params, 'type', 'net', 'network').toLowerCase()
  if (net && net !== 'tcp' && net !== 'none') {
    out.network = net === 'h2' ? 'h2' : net
  }
  out.wsPath = pick(params, 'path')
  out.wsHost = pick(params, 'host')
  out.grpcService = pick(params, 'serviceName', 'service_name')

  // hysteria2
  const obfs = pick(params, 'obfs')
  if (obfs) {
    out.obfs = obfs
    out.obfsPassword = pick(params, 'obfs-password', 'obfsPassword', 'obfsParam')
  }
  const up = pick(params, 'upmbps', 'up')
  const down = pick(params, 'downmbps', 'down')
  if (up) out.upMbps = parseInt(up, 10) || 0
  if (down) out.downMbps = parseInt(down, 10) || 0

  // 名称：remarks 参数优先（部分面板用参数而非 fragment）
  const remarks = pick(params, 'remarks', 'remark', 'name')
  if (remarks) out.name = remarks
}

/** 解析 vmess://base64(JSON) */
function parseVmess(link: string): ParsedProxy {
  const out: ParsedProxy = { ...EMPTY, protocol: 'vmess', port: 443 }
  const body = link.slice('vmess://'.length).trim()
  const json = decodeBase64Flexible(body)
  const cfg = JSON.parse(json)

  out.name = cfg.ps || ''
  out.host = cfg.add || ''
  out.port = parseInt(cfg.port, 10) || 443
  out.uuid = cfg.id || ''
  out.method = cfg.scy || 'auto'

  const net = (cfg.net || 'tcp').toLowerCase()
  if (net && net !== 'tcp') out.network = net
  out.wsPath = cfg.path || ''
  out.wsHost = cfg.host || ''
  if (net === 'grpc') out.grpcService = cfg.path || ''

  const tlsMode = (cfg.tls || '').toLowerCase()
  if (tlsMode === 'tls' || tlsMode === 'reality') {
    out.tls = true
    out.sni = cfg.sni || ''
    out.fingerprint = cfg.fp || 'chrome'
    out.alpn = cfg.alpn || ''
    if (tlsMode === 'reality') {
      out.reality = true
      out.pubKey = cfg.pbk || ''
      out.shortId = cfg.sid || ''
    }
  }
  if (cfg.flow) out.flow = cfg.flow

  return out
}

/** 解析 ss:// 三种常见形式 */
function parseShadowsocks(link: string): ParsedProxy {
  const out: ParsedProxy = { ...EMPTY, protocol: 'shadowsocks', port: 8388 }
  let body = link.slice('ss://'.length).trim()

  // 分离 fragment
  let fragment = ''
  const hashIdx = body.indexOf('#')
  if (hashIdx > -1) {
    fragment = body.slice(hashIdx + 1)
    body = body.slice(0, hashIdx)
  }
  // 分离查询串（plugin 等）
  let query = ''
  const qIdx = body.indexOf('?')
  if (qIdx > -1) {
    query = body.slice(qIdx + 1)
    body = body.slice(0, qIdx)
  }

  let userinfo = ''
  let hostport = ''

  if (body.includes('@')) {
    // 形式：base64(method:password)@host:port
    const at = body.lastIndexOf('@')
    userinfo = body.slice(0, at)
    hostport = body.slice(at + 1)
    if (!userinfo.includes(':')) {
      userinfo = decodeBase64Flexible(userinfo)
    }
  } else {
    // 形式：base64(method:password@host:port)
    const decoded = decodeBase64Flexible(body)
    const at = decoded.lastIndexOf('@')
    if (at === -1) throw new Error('无法解析 Shadowsocks 链接')
    userinfo = decoded.slice(0, at)
    hostport = decoded.slice(at + 1)
  }

  const colon = userinfo.indexOf(':')
  if (colon === -1) throw new Error('Shadowsocks 缺少加密方式或密码')
  out.method = userinfo.slice(0, colon)
  out.password = userinfo.slice(colon + 1)

  const hp = splitHostPort(hostport, 8388)
  out.host = hp.host
  out.port = hp.port

  if (fragment) out.name = decodeURIComponent(fragment)

  if (query) {
    const params = new URLSearchParams(query)
    const plugin = params.get('plugin') || ''
    // obfs-local;obfs=tls;obfs-host=xxx
    if (plugin.startsWith('obfs')) {
      const parts = plugin.split(';')
      for (const part of parts) {
        const [k, v] = part.split('=')
        if (k === 'obfs') out.obfs = v || ''
        if (k === 'obfs-host') out.wsHost = v || ''
      }
    }
  }

  return out
}

/**
 * 解析节点链接，返回归一化结构。
 * 无法识别时抛出 Error（调用方负责提示）。
 */
export function parseProxyLink(rawLink: string): ParsedProxy {
  const link = (rawLink || '').trim()
  if (!link) throw new Error('链接为空')

  const lower = link.toLowerCase()
  if (lower.startsWith('vmess://')) return parseVmess(link)
  if (lower.startsWith('ss://')) return parseShadowsocks(link)

  const schemes: Array<[string, ProxyProtocol]> = [
    ['vless://', 'vless'],
    ['trojan://', 'trojan'],
    ['hysteria2://', 'hysteria2'],
    ['hy2://', 'hysteria2'],
    ['anytls://', 'anytls'],
    ['socks5://', 'socks5'],
    ['socks://', 'socks5'],
  ]

  let protocol: ProxyProtocol | null = null
  let body = ''
  for (const [prefix, proto] of schemes) {
    if (lower.startsWith(prefix)) {
      protocol = proto
      body = link.slice(prefix.length)
      break
    }
  }
  if (!protocol) throw new Error('不支持的链接格式')

  const out: ParsedProxy = { ...EMPTY, protocol }
  if (protocol === 'hysteria2') out.port = 443
  if (protocol === 'socks5') out.port = 1080

  // 分离 fragment / query
  let fragment = ''
  const hashIdx = body.indexOf('#')
  if (hashIdx > -1) {
    fragment = body.slice(hashIdx + 1)
    body = body.slice(0, hashIdx)
  }
  let query = ''
  const qIdx = body.indexOf('?')
  if (qIdx > -1) {
    query = body.slice(qIdx + 1)
    body = body.slice(0, qIdx)
  }
  const params = new URLSearchParams(query)

  // 拆分 userinfo@hostport，处理 base64 包裹 userinfo 的变体
  const at = body.lastIndexOf('@')
  if (at === -1) {
    // 没有 @ 时尝试整体 base64 解码后再拆（部分导出工具）
    const decoded = looksLikeBase64(body) ? decodeBase64Flexible(body) : ''
    if (decoded.includes('@')) {
      const dat = decoded.lastIndexOf('@')
      const info = decoded.slice(0, dat)
      const hp = splitHostPort(decoded.slice(dat + 1), out.port)
      out.host = hp.host
      out.port = hp.port
      assignUserinfo(out, info)
    } else {
      throw new Error('链接缺少服务器地址')
    }
  } else {
    let userinfo = body.slice(0, at)
    const hostport = body.slice(at + 1)

    // 关键兼容：userinfo 可能是 base64(none:uuid) 或 base64(password)
    if (!userinfo.includes(':') && looksLikeBase64(userinfo)) {
      const decoded = decodeBase64Flexible(userinfo)
      if (decoded.includes(':')) {
        userinfo = decoded
      }
    }

    const hp = splitHostPort(hostport, out.port)
    out.host = hp.host
    out.port = hp.port
    assignUserinfo(out, userinfo)
  }

  if (fragment) out.name = decodeURIComponent(fragment)

  applyParams(out, params)

  // trojan / hysteria2 / anytls 默认走 TLS
  if (protocol === 'trojan' || protocol === 'hysteria2' || protocol === 'anytls') {
    if (!out.reality) out.tls = true
    if (!out.sni) out.sni = params.get('peer') || params.get('sni') || out.host
  }
  // vless 若带 pbk 已在上方置为 reality
  if (!out.name) out.name = out.host

  return out
}

/** 把 userinfo 拆成 uuid / password / username */
function assignUserinfo(out: ParsedProxy, userinfo: string) {
  if (!userinfo) return
  const colon = userinfo.indexOf(':')

  switch (out.protocol) {
    case 'vless': {
      // 形式可能是 uuid 或 none:uuid（encryption:uuid）
      if (colon > -1) {
        const head = userinfo.slice(0, colon)
        const tail = userinfo.slice(colon + 1)
        // 若 head 不是 UUID 而 tail 是，则 head 是 encryption 前缀
        out.uuid = isUuid(tail) ? tail : userinfo
        if (head && head !== 'none' && isUuid(head)) out.uuid = head
      } else {
        out.uuid = userinfo
      }
      break
    }
    case 'socks5': {
      if (colon > -1) {
        out.username = decodeURIComponent(userinfo.slice(0, colon))
        out.password = decodeURIComponent(userinfo.slice(colon + 1))
      } else {
        out.username = decodeURIComponent(userinfo)
      }
      break
    }
    default:
      // trojan / hysteria2 / anytls：整个 userinfo 就是密码
      out.password = decodeURIComponent(userinfo)
      break
  }
}

function isUuid(v: string): boolean {
  return /^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$/.test(v)
}

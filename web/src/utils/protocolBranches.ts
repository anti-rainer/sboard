/**
 * 协议分支定义
 *
 * 交互模型：先选主协议，再在弹框顶部的胶囊里选该协议的具体分支。
 * 一个「分支」= 传输层 + 安全层（以及 SS/Hy2 的算法变体）的预设组合，
 * 它不引入新的存储字段，只是把既有字段按预设写好，用户仍可继续微调。
 *
 * 分支覆盖范围对齐 233boy/sing-box 的协议列表，并按其能力边界取舍：
 * VMess-QUIC 在 sing-box 里没有对应传输层实现，故不提供。
 */

export type MainProtocol =
  | 'vless'
  | 'vmess'
  | 'trojan'
  | 'hysteria2'
  | 'tuic'
  | 'anytls'
  | 'shadowsocks'
  | 'naive'
  | 'socks'

/** 分支对表单字段的预设（各页面自行映射到自己的字段名） */
export interface BranchPatch {
  tls?: boolean
  reality?: boolean
  /** 传输层类型；null 表示关闭传输层 */
  transport?: string | null
  flow?: string
  ssMethod?: string
  /** '' | 'salamander' */
  hy2Obfs?: string
  /** TUIC 拥塞控制 */
  congestion?: string
}

export interface ProtocolBranch {
  id: string
  label: string
  hint?: string
  patch: BranchPatch
}

export interface MainProtocolMeta {
  id: MainProtocol
  label: string
  /** 该协议强制 TLS（不可关闭） */
  forceTls?: boolean
  /** 该协议不支持传输层 */
  noTransport?: boolean
  branches: ProtocolBranch[]
}

/** 供各页面渲染主协议下拉 */
export const MAIN_PROTOCOLS: MainProtocolMeta[] = [
  {
    id: 'vless',
    label: 'VLESS',
    branches: [
      {
        id: 'reality',
        label: 'REALITY',
        hint: 'VLESS-REALITY，需伪装站点与 X25519 密钥对',
        patch: { tls: true, reality: true, transport: null, flow: 'xtls-rprx-vision' },
      },
      {
        id: 'reality-h2',
        label: 'REALITY-H2',
        hint: 'VLESS-HTTP2-REALITY',
        patch: { tls: true, reality: true, transport: 'h2' },
      },
      {
        id: 'tls',
        label: 'TCP-TLS',
        hint: '标准 TLS + Vision 流控',
        patch: { tls: true, reality: false, transport: null, flow: 'xtls-rprx-vision' },
      },
      {
        id: 'ws-tls',
        label: 'WS-TLS',
        hint: 'WebSocket over TLS',
        patch: { tls: true, reality: false, transport: 'ws', flow: '' },
      },
      {
        id: 'h2-tls',
        label: 'H2-TLS',
        hint: 'HTTP/2 over TLS',
        patch: { tls: true, reality: false, transport: 'h2', flow: '' },
      },
      {
        id: 'hu-tls',
        label: 'HTTPUpgrade-TLS',
        patch: { tls: true, reality: false, transport: 'httpupgrade', flow: '' },
      },
      {
        id: 'grpc-tls',
        label: 'gRPC-TLS',
        patch: { tls: true, reality: false, transport: 'grpc', flow: '' },
      },
    ],
  },
  {
    id: 'vmess',
    label: 'VMess',
    branches: [
      { id: 'tcp', label: 'TCP', hint: '无 TLS，最轻量', patch: { tls: false, reality: false, transport: null } },
      { id: 'http', label: 'HTTP', hint: 'HTTP 伪装，无 TLS', patch: { tls: false, reality: false, transport: 'http' } },
      { id: 'ws', label: 'WS', hint: 'WebSocket，无 TLS', patch: { tls: false, reality: false, transport: 'ws' } },
      { id: 'quic', label: 'QUIC', hint: 'QUIC 传输层（官方 1.14.4 支持）', patch: { tls: true, reality: false, transport: 'quic' } },
      { id: 'ws-tls', label: 'WS-TLS', patch: { tls: true, reality: false, transport: 'ws' } },
      { id: 'h2-tls', label: 'H2-TLS', patch: { tls: true, reality: false, transport: 'h2' } },
      { id: 'hu-tls', label: 'HTTPUpgrade-TLS', patch: { tls: true, reality: false, transport: 'httpupgrade' } },
    ],
  },
  {
    id: 'trojan',
    label: 'Trojan',
    forceTls: true,
    branches: [
      { id: 'tls', label: 'TLS', hint: '标准 Trojan over TLS', patch: { tls: true, reality: false, transport: null } },
      { id: 'ws-tls', label: 'WS-TLS', patch: { tls: true, reality: false, transport: 'ws' } },
      { id: 'h2-tls', label: 'H2-TLS', patch: { tls: true, reality: false, transport: 'h2' } },
      { id: 'hu-tls', label: 'HTTPUpgrade-TLS', patch: { tls: true, reality: false, transport: 'httpupgrade' } },
      { id: 'grpc-tls', label: 'gRPC-TLS', patch: { tls: true, reality: false, transport: 'grpc' } },
    ],
  },
  {
    id: 'hysteria2',
    label: 'Hysteria2',
    forceTls: true,
    noTransport: true,
    branches: [
      { id: 'plain', label: '标准', hint: '基于 QUIC 的 Hysteria2', patch: { tls: true, reality: false, transport: null, hy2Obfs: '' } },
      { id: 'obfs', label: 'Salamander 混淆', hint: '额外一层混淆，抗特征识别', patch: { tls: true, reality: false, transport: null, hy2Obfs: 'salamander' } },
    ],
  },
  {
    id: 'tuic',
    label: 'TUIC',
    forceTls: true,
    noTransport: true,
    branches: [
      { id: 'plain', label: '标准', hint: '基于 QUIC，ALPN h3', patch: { tls: true, reality: false, transport: null, congestion: 'bbr' } },
      { id: 'cubic', label: 'CUBIC', hint: '拥塞控制改为 cubic', patch: { tls: true, reality: false, transport: null, congestion: 'cubic' } },
    ],
  },
  {
    id: 'anytls',
    label: 'AnyTLS',
    forceTls: true,
    noTransport: true,
    branches: [
      { id: 'plain', label: '标准', hint: 'AnyTLS over TLS', patch: { tls: true, reality: false, transport: null } },
    ],
  },
  {
    id: 'shadowsocks',
    label: 'Shadowsocks',
    noTransport: true,
    // SS 的分支维度就是加密方式本身（对齐 233boy 的 ss_method_list 全部 7 项）
    branches: [
      { id: 'aes-128-gcm', label: 'AES-128-GCM', hint: '传统加密，单用户共享密码', patch: { tls: false, reality: false, transport: null, ssMethod: 'aes-128-gcm' } },
      { id: 'aes-256-gcm', label: 'AES-256-GCM', hint: '传统加密，单用户共享密码', patch: { tls: false, reality: false, transport: null, ssMethod: 'aes-256-gcm' } },
      { id: 'chacha20-ietf-poly1305', label: 'ChaCha20-IETF', hint: '传统加密，ARM 设备友好', patch: { tls: false, reality: false, transport: null, ssMethod: 'chacha20-ietf-poly1305' } },
      { id: 'xchacha20-ietf-poly1305', label: 'XChaCha20-IETF', hint: '传统加密，扩展 nonce', patch: { tls: false, reality: false, transport: null, ssMethod: 'xchacha20-ietf-poly1305' } },
      { id: '2022-blake3-aes-128-gcm', label: '2022-AES128', hint: 'SS2022，支持多用户独立密钥', patch: { tls: false, reality: false, transport: null, ssMethod: '2022-blake3-aes-128-gcm' } },
      { id: '2022-blake3-aes-256-gcm', label: '2022-AES256', hint: 'SS2022，支持多用户独立密钥', patch: { tls: false, reality: false, transport: null, ssMethod: '2022-blake3-aes-256-gcm' } },
      { id: '2022-blake3-chacha20-poly1305', label: '2022-ChaCha20', hint: 'SS2022，ARM 优化', patch: { tls: false, reality: false, transport: null, ssMethod: '2022-blake3-chacha20-poly1305' } },
    ],
  },
  {
    id: 'naive',
    label: 'Naive',
    forceTls: true,
    noTransport: true,
    branches: [
      { id: 'plain', label: '标准', hint: 'Naive over HTTPS', patch: { tls: true, reality: false, transport: null } },
    ],
  },
  {
    id: 'socks',
    label: 'Socks',
    noTransport: true,
    branches: [
      { id: 'plain', label: '标准', hint: 'Socks5，用户名 + 密码', patch: { tls: false, reality: false, transport: null } },
    ],
  },
]

const PROTOCOL_MAP = new Map<string, MainProtocolMeta>(MAIN_PROTOCOLS.map((p) => [p.id, p]))

export function getProtocolMeta(protocol: string): MainProtocolMeta | undefined {
  return PROTOCOL_MAP.get(protocol)
}

export function getBranches(protocol: string): ProtocolBranch[] {
  return PROTOCOL_MAP.get(protocol)?.branches ?? []
}

/**
 * 依据当前表单状态推断命中的分支 id。
 * 用于编辑已有节点时把胶囊高亮到对应分支。
 */
export function detectBranch(
  protocol: string,
  state: { tls: boolean; reality: boolean; transport: string; flow: string; ssMethod?: string; hy2Obfs?: string; congestion?: string },
): string | null {
  const branches = getBranches(protocol)
  for (const b of branches) {
    const p = b.patch
    if (p.tls !== undefined && p.tls !== state.tls) continue
    if (p.reality !== undefined && p.reality !== state.reality) continue

    const wantTransport = p.transport === null ? '' : (p.transport ?? '')
    const normTransport = state.transport === 'tcp' ? '' : state.transport
    if ((p.transport !== undefined) && wantTransport !== normTransport) continue

    if (p.flow !== undefined && (p.flow || '') !== (state.flow || '')) continue
    if (p.ssMethod !== undefined && p.ssMethod !== (state.ssMethod || '')) continue
    if (p.hy2Obfs !== undefined && (p.hy2Obfs || '') !== (state.hy2Obfs || '')) continue
    if (p.congestion !== undefined && p.congestion !== (state.congestion || '')) continue

    return b.id
  }
  return null
}

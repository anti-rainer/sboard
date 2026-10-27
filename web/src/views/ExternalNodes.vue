<template>
  <div class="container-fluid mt-4">
    <div class="d-flex justify-content-between mb-4">
      <h2>外部节点</h2>
      <button class="btn btn-primary" @click="openAddModal">
        <i class="bi bi-plus-lg"></i> 添加外部节点
      </button>
    </div>

    <div class="row">
      <div v-for="node in nodes" :key="node.id" class="col-xl-3 col-lg-4 col-md-6">
        <div class="card node-card mb-3">
          <div class="card-body">
            <div class="d-flex justify-content-between align-items-start mb-2">
              <h6 class="card-title mb-0">{{ node.name }}</h6>
              <span :class="['badge', node.enabled ? 'bg-success' : 'bg-secondary']">
                {{ node.enabled ? '启用' : '禁用' }}
              </span>
            </div>
            <div class="small mb-2">
              <div class="d-flex justify-content-between">
                <span class="text-muted">协议:</span>
                <span class="text-primary fw-bold">{{ node.protocol.toUpperCase() }}</span>
              </div>
              <div class="d-flex justify-content-between">
                <span class="text-muted">地址:</span>
                <span>{{ node.host }}:{{ node.port }}</span>
              </div>
              <div class="d-flex justify-content-between" v-if="node.tls_enabled">
                <span class="text-muted">TLS:</span>
                <span class="text-success">
                  <i class="bi bi-shield-check"></i>
                  {{ node.reality_enabled ? 'Reality' : '启用' }}
                </span>
              </div>
            </div>
            <div class="btn-group btn-group-sm w-100">
              <button class="btn btn-outline-primary" @click="openEditModal(node)">
                <i class="bi bi-pencil"></i> 编辑
              </button>
              <button class="btn btn-outline-danger" @click="confirmDelete(node)">
                <i class="bi bi-trash"></i> 删除
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- 空状态 -->
      <div v-if="nodes.length === 0 && !loading" class="col-12 text-center py-5 text-muted">
        <i class="bi bi-inbox" style="font-size: 3rem;"></i>
        <p class="mt-2">暂无外部节点</p>
        <p class="small">添加未被 SBoard Agent 管理的第三方节点</p>
      </div>
    </div>

    <!-- 加载中 -->
    <div v-if="loading" class="text-center py-5">
      <div class="spinner-border" role="status"></div>
    </div>

    <!-- 添加/编辑模态框 -->
    <div class="modal fade" id="extNodeModal" tabindex="-1" ref="modalEl">
      <div class="modal-dialog modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">{{ editingId ? '编辑外部节点' : '添加外部节点' }}</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <!-- 快速识别：粘贴节点链接自动填充表单 -->
            <div class="mb-3">
              <label class="form-label">快速识别</label>
              <div class="input-group">
                <input
                  type="text"
                  class="form-control"
                  v-model="importLink"
                  placeholder="粘贴节点链接：vless:// vmess:// trojan:// ss:// hysteria2:// tuic:// anytls:// socks5://"
                  @keyup.enter="parseImportLink"
                >
                <button class="btn btn-outline-primary" type="button" @click="parseImportLink">识别</button>
              </div>
              <div class="form-text">支持标准链接与 base64 包裹变体（含 Reality 参数 pbk/sid/fp）</div>
            </div>

            <!-- 基本信息 -->
            <div class="row mb-3">
              <div class="col-md-8">
                <label class="form-label">节点名称</label>
                <input type="text" class="form-control" v-model="form.name" placeholder="RackNerd-US">
              </div>
              <div class="col-md-4">
                <label class="form-label">协议</label>
                <select class="form-select" v-model="form.protocol" @change="onProtocolChange">
                  <option value="">请选择</option>
                  <option v-for="p in MAIN_PROTOCOLS" :key="p.id" :value="p.id">{{ p.label }}</option>
                </select>
              </div>
            </div>

            <!-- 协议分支：先选主协议，再选具体分支 -->
            <div class="mb-3" v-if="branches.length">
              <label class="form-label">协议分支 <span class="text-muted small">（先选主协议，再选具体分支）</span></label>
              <div class="d-flex flex-wrap gap-2">
                <button type="button" v-for="b in branches" :key="b.id"
                        class="btn btn-sm rounded-pill px-3"
                        :class="activeBranch === b.id ? 'btn-primary' : 'btn-outline-secondary'"
                        :title="b.hint || b.label"
                        @click="applyBranch(b)">
                  {{ b.label }}
                </button>
              </div>
              <small class="text-muted d-block mt-1" v-if="activeBranchHint">
                <i class="bi bi-info-circle"></i> {{ activeBranchHint }}
              </small>
            </div>

            <div class="row mb-3">
              <div class="col-md-6">
                <label class="form-label">主机地址</label>
                <input type="text" class="form-control" v-model="form.host" placeholder="1.2.3.4 或 example.com">
              </div>
              <div class="col-md-3">
                <label class="form-label">端口</label>
                <input type="number" class="form-control" v-model.number="form.port">
              </div>
              <div class="col-md-3">
                <label class="form-label">级别</label>
                <input type="number" class="form-control" v-model.number="form.level" min="1" max="5">
              </div>
            </div>

            <!-- 认证信息（Socks 在下方 Socks5 设置里单独填写用户名/密码） -->
            <div class="mb-3" v-if="form.protocol && !isSocks">
              <label class="form-label">{{ protocolAuthLabel }}</label>
              <input type="text" class="form-control" v-model="form.uuid" :placeholder="protocolAuthPlaceholder">
            </div>

            <!-- TLS 设置（Socks 不支持；TUIC 强制启用） -->
            <div v-if="tlsVisible" class="card mb-3">
              <div class="card-header">TLS 设置</div>
              <div class="card-body">
                <div class="form-check form-switch mb-2" v-if="!isTuic">
                  <input class="form-check-input" type="checkbox" v-model="form.tls_enabled" id="extTlsEnabled">
                  <label class="form-check-label" for="extTlsEnabled">启用 TLS</label>
                </div>
                <div class="alert alert-secondary py-2 mb-2 small" v-else>
                  <i class="bi bi-lock-fill"></i> TUIC 基于 QUIC，强制启用 TLS（ALPN 固定 h3）
                </div>
                <template v-if="form.tls_enabled">
                  <div class="row mb-2">
                    <div class="col-md-6">
                      <label class="form-label">Server Name (SNI)</label>
                      <input type="text" class="form-control" v-model="form.server_name">
                    </div>
                    <div class="col-md-6">
                      <label class="form-label">ALPN</label>
                      <input type="text" class="form-control" v-model="form.alpn" placeholder="h2,http/1.1">
                    </div>
                  </div>
                  <div class="form-check form-switch mb-2" v-if="form.protocol === 'vless'">
                    <input class="form-check-input" type="checkbox" v-model="form.reality_enabled" id="extReality">
                    <label class="form-check-label" for="extReality">REALITY</label>
                  </div>
                  <template v-if="form.reality_enabled">
                    <div class="row mb-2">
                      <div class="col-md-4">
                        <label class="form-label">PubKey</label>
                        <input type="text" class="form-control" v-model="form.reality_pubkey">
                      </div>
                      <div class="col-md-4">
                        <label class="form-label">Short ID</label>
                        <input type="text" class="form-control" v-model="form.reality_short_id">
                      </div>
                      <div class="col-md-4">
                        <label class="form-label">Flow</label>
                        <input type="text" class="form-control" v-model="form.flow">
                      </div>
                    </div>
                  </template>
                </template>
              </div>
            </div>

            <!-- Shadowsocks 特有 -->
            <div v-if="form.protocol === 'shadowsocks'" class="card mb-3">
              <div class="card-header">Shadowsocks 设置</div>
              <div class="card-body">
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">加密方式</label>
                    <select class="form-select" v-model="form.ss_method">
                      <option value="">请选择</option>
                      <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm</option>
                      <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
                      <option value="2022-blake3-chacha20-poly1305">2022-blake3-chacha20-poly1305</option>
                      <option value="aes-256-gcm">aes-256-gcm</option>
                      <option value="aes-128-gcm">aes-128-gcm</option>
                      <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
                    </select>
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">密码</label>
                    <input type="text" class="form-control" v-model="form.ss_password">
                  </div>
                </div>
              </div>
            </div>

            <!-- Hysteria2 特有 -->
            <div v-if="form.protocol === 'hysteria2'" class="card mb-3">
              <div class="card-header">Hysteria2 设置</div>
              <div class="card-body">
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">密码</label>
                    <input type="text" class="form-control" v-model="form.hy2_password">
                  </div>
                </div>
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">上行速度 (Mbps)</label>
                    <input type="number" class="form-control" v-model.number="form.hy2_up_mbps" min="1">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">下行速度 (Mbps)</label>
                    <input type="number" class="form-control" v-model.number="form.hy2_down_mbps" min="1">
                  </div>
                </div>
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">混淆类型</label>
                    <select class="form-select" v-model="form.hy2_obfs">
                      <option value="">无</option>
                      <option value="salamander">salamander</option>
                    </select>
                  </div>
                  <div class="col-md-6" v-if="form.hy2_obfs === 'salamander'">
                    <label class="form-label">混淆密码</label>
                    <input type="text" class="form-control" v-model="form.hy2_obfs_password">
                  </div>
                </div>
              </div>
            </div>

            <!-- TUIC 特有 -->
            <div v-if="isTuic" class="card mb-3">
              <div class="card-header">TUIC 设置</div>
              <div class="card-body">
                <div class="alert alert-info py-2 mb-3 small">
                  <i class="bi bi-info-circle"></i>
                  TUIC 基于 QUIC（UDP），强制启用 TLS、ALPN 固定 h3，不支持 Reality 与传输层。
                </div>
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">认证密码</label>
                    <input type="text" class="form-control" v-model="form.tuic_password" placeholder="留空则使用 UUID">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">拥塞控制</label>
                    <select class="form-select" v-model="form.tuic_congestion">
                      <option value="bbr">bbr（推荐）</option>
                      <option value="cubic">cubic</option>
                      <option value="new_reno">new_reno</option>
                    </select>
                  </div>
                </div>
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">UDP 转发模式</label>
                    <select class="form-select" v-model="form.tuic_udp_relay_mode">
                      <option value="native">native（原生，推荐）</option>
                      <option value="quic">quic（QUIC 隧道）</option>
                    </select>
                  </div>
                </div>
              </div>
            </div>

            <!-- Socks 特有 -->
            <div v-if="isSocks" class="card mb-3">
              <div class="card-header">Socks5 设置</div>
              <div class="card-body">
                <div class="alert alert-info py-2 mb-3 small">
                  <i class="bi bi-info-circle"></i>
                  后端复用字段存储 Socks5 凭据：<strong>用户名写入 UUID 字段</strong>，<strong>密码写入 Password 字段</strong>。
                  Socks5 不支持 TLS、Reality 与传输层。
                </div>
                <div class="row mb-2">
                  <div class="col-md-6">
                    <label class="form-label">用户名</label>
                    <input type="text" class="form-control" v-model="form.uuid" placeholder="socks-user">
                  </div>
                  <div class="col-md-6">
                    <label class="form-label">密码</label>
                    <input type="text" class="form-control" v-model="form.password" placeholder="your-password">
                  </div>
                </div>
              </div>
            </div>

            <!-- 传输层设置（Socks 不支持） -->
            <div v-if="transportVisible" class="card mb-3">
              <div class="card-header">传输层设置</div>
              <div class="card-body">
                <div class="form-check form-switch mb-2">
                  <input class="form-check-input" type="checkbox" v-model="form.transport_enabled" id="extTransport">
                  <label class="form-check-label" for="extTransport">启用传输层</label>
                </div>
                <template v-if="form.transport_enabled">
                  <div class="row mb-2">
                    <div class="col-md-6">
                      <label class="form-label">传输类型</label>
                      <select class="form-select" v-model="form.transport_type">
                        <option value="ws">WebSocket</option>
                        <option value="grpc">gRPC</option>
                        <option value="httpupgrade">HTTPUpgrade</option>
                      </select>
                    </div>
                    <div class="col-md-6">
                      <label class="form-label">Host (transport)</label>
                      <input type="text" class="form-control" v-model="form.transport_host">
                    </div>
                  </div>
                  <div class="row mb-2">
                    <div class="col-md-6">
                      <label class="form-label">Path</label>
                      <input type="text" class="form-control" v-model="form.ws_path" placeholder="/ws">
                    </div>
                    <div class="col-md-6" v-if="form.transport_type === 'grpc'">
                      <label class="form-label">Service Name</label>
                      <input type="text" class="form-control" v-model="form.grpc_service">
                    </div>
                  </div>
                </template>
              </div>
            </div>

            <!-- 其他 -->
            <div class="row mb-3">
              <div class="col-md-4">
                <label class="form-label">国家代码</label>
                <input type="text" class="form-control" v-model="form.country" placeholder="US" maxlength="2">
              </div>
              <div class="col-md-4">
                <label class="form-label">排序</label>
                <input type="number" class="form-control" v-model.number="form.sort_order">
              </div>
              <div class="col-md-4">
                <label class="form-label">状态</label>
                <div class="form-check form-switch mt-2">
                  <input class="form-check-input" type="checkbox" v-model="form.enabled" id="extEnabled">
                  <label class="form-check-label" for="extEnabled">启用</label>
                </div>
              </div>
            </div>
            <div class="mb-3">
              <label class="form-label">备注</label>
              <input type="text" class="form-control" v-model="form.notes" placeholder="可选备注">
            </div>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
            <button type="button" class="btn btn-primary" @click="saveNode" :disabled="saving">
              <span v-if="saving" class="spinner-border spinner-border-sm me-1"></span>
              <i v-else class="bi bi-check-lg"></i>
              保存
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 删除确认 -->
    <div class="modal fade" id="deleteModal" tabindex="-1" ref="deleteModalEl">
      <div class="modal-dialog">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">确认删除</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <p>确定要删除外部节点 <strong>{{ deleteTarget?.name }}</strong> 吗？此操作不可撤销。</p>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
            <button type="button" class="btn btn-danger" @click="doDelete" :disabled="saving">
              <span v-if="saving" class="spinner-border spinner-border-sm me-1"></span>
              删除
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, inject } from 'vue'
import { Modal } from 'bootstrap'
import { getExternalNodes, createExternalNode, updateExternalNode, deleteExternalNode, type ExternalNode } from '../api'
import { parseProxyLink, type ParsedProxy } from '../utils/proxyLink'
import { MAIN_PROTOCOLS, getBranches, detectBranch, type ProtocolBranch } from '../utils/protocolBranches'

const showToast = inject<any>('showToast')

const nodes = ref<ExternalNode[]>([])
const loading = ref(true)
const saving = ref(false)
const editingId = ref<number | null>(null)
const deleteTarget = ref<ExternalNode | null>(null)
const importLink = ref('')

let modal: Modal | null = null
let deleteModal: Modal | null = null
const modalEl = ref<HTMLElement | null>(null)
const deleteModalEl = ref<HTMLElement | null>(null)

/** 外部节点表单：后端已支持 TUIC / Socks 字段，api 类型暂未声明，这里补上 */
type ExternalNodeForm = Partial<ExternalNode> & {
  tuic_password?: string
  tuic_congestion?: string
  tuic_udp_relay_mode?: string
  /** Socks5 密码（用户名复用 uuid 字段） */
  password?: string
}

const defaultForm = (): ExternalNodeForm => ({
  name: '',
  protocol: '',
  host: '',
  port: 443,
  uuid: '',
  password: '',
  tls_enabled: false,
  server_name: '',
  alpn: '',
  reality_enabled: false,
  reality_pubkey: '',
  reality_short_id: '',
  transport_enabled: false,
  transport_type: 'ws',
  ws_path: '',
  grpc_service: '',
  transport_host: '',
  flow: '',
  ss_method: '',
  ss_password: '',
  hy2_password: '',
  hy2_up_mbps: 100,
  hy2_down_mbps: 100,
  hy2_obfs: '',
  hy2_obfs_password: '',
  tuic_password: '',
  tuic_congestion: 'bbr',
  tuic_udp_relay_mode: 'native',
  level: 1,
  enabled: true,
  sort_order: 0,
  country: '',
  notes: ''
})

const form = ref<ExternalNodeForm>(defaultForm())

const isTuic = computed(() => form.value.protocol === 'tuic')
const isSocks = computed(() => form.value.protocol === 'socks')
/** TLS 卡：Socks 不支持；TUIC 强制启用 */
const tlsVisible = computed(() => ['vmess', 'vless', 'trojan', 'anytls', 'tuic'].includes(form.value.protocol || ''))
/** 传输层卡：仅 VMess/VLESS/Trojan，Socks 不支持 */
const transportVisible = computed(
  () => ['vmess', 'vless', 'trojan'].includes(form.value.protocol || '') && !isSocks.value
)

// ===== 协议分支 =====
const branches = computed(() => getBranches(form.value.protocol || ''))
const activeBranch = ref<string | null>(null)
const activeBranchHint = computed(() => {
  const b = branches.value.find((x) => x.id === activeBranch.value)
  return b?.hint || ''
})

/** 把分支预设写入表单（不引入新字段，只按预设填既有字段） */
function applyBranch(b: ProtocolBranch) {
  activeBranch.value = b.id
  const f = form.value
  const p = b.patch
  if (p.tls !== undefined) f.tls_enabled = p.tls
  if (p.reality !== undefined) f.reality_enabled = p.reality
  if (p.transport !== undefined) {
    if (p.transport === null) {
      f.transport_enabled = false
    } else {
      f.transport_enabled = true
      f.transport_type = p.transport
    }
  }
  if (p.flow !== undefined) f.flow = p.flow
  if (p.ssMethod !== undefined) f.ss_method = p.ssMethod
  if (p.hy2Obfs !== undefined) f.hy2_obfs = p.hy2Obfs
  if (p.congestion !== undefined) f.tuic_congestion = p.congestion
  enforceProtocolConstraints()
}

/** 协议硬约束：TUIC 强制 TLS；Socks 不支持 TLS / Reality / 传输层 */
function enforceProtocolConstraints() {
  const f = form.value
  if (f.protocol === 'tuic') {
    f.tls_enabled = true
    f.reality_enabled = false
    f.transport_enabled = false
    if (!f.tuic_congestion) f.tuic_congestion = 'bbr'
    if (!f.tuic_udp_relay_mode) f.tuic_udp_relay_mode = 'native'
  }
  if (f.protocol === 'socks') {
    f.tls_enabled = false
    f.reality_enabled = false
    f.transport_enabled = false
  }
}

/** 依据当前表单状态反推命中的分支（编辑已有节点时高亮） */
function syncActiveBranch() {
  const f = form.value
  activeBranch.value = detectBranch(f.protocol || '', {
    tls: !!f.tls_enabled,
    reality: !!f.reality_enabled,
    transport: f.transport_enabled ? f.transport_type || '' : '',
    flow: f.flow || '',
    ssMethod: f.ss_method,
    hy2Obfs: f.hy2_obfs,
    congestion: f.tuic_congestion
  })
}

/** 切换协议：先套硬约束，再落到该协议的首个分支 */
function onProtocolChange() {
  enforceProtocolConstraints()
  const list = branches.value
  if (list.length > 0) {
    applyBranch(list[0])
  } else {
    activeBranch.value = null
  }
}

const protocolAuthLabel = computed(() => {
  const m: Record<string, string> = {
    vmess: 'UUID',
    vless: 'UUID',
    trojan: '密码',
    shadowsocks: 'UUID (用户标识)',
    hysteria2: 'UUID (用户标识)',
    anytls: '密码',
    tuic: 'UUID (用户标识)',
    socks: '用户名'
  }
  return m[form.value.protocol || ''] || '认证信息'
})

const protocolAuthPlaceholder = computed(() => {
  const m: Record<string, string> = {
    vmess: '00000000-0000-0000-0000-000000000000',
    vless: '00000000-0000-0000-0000-000000000000',
    trojan: 'your-password',
    anytls: 'your-password',
    shadowsocks: '00000000-0000-...',
    hysteria2: '00000000-0000-...',
    tuic: '00000000-0000-...',
    socks: 'socks-user'
  }
  return m[form.value.protocol || ''] || ''
})

const loadNodes = async () => {
  loading.value = true
  try {
    const res = await getExternalNodes()
    nodes.value = res.data.data || []
  } catch (e: any) {
    showToast?.('error', '错误', '加载外部节点失败')
  } finally {
    loading.value = false
  }
}

// 解析粘贴的节点链接并填充表单
const parseImportLink = () => {
  const link = importLink.value.trim()
  if (!link) {
    showToast?.('warning', '提示', '请先粘贴节点链接')
    return
  }

  try {
    const p = parseProxyLink(link)
    applyParsedToForm(p)
    showToast?.('success', '成功', `已识别 ${p.protocol} 节点`)
    importLink.value = ''
  } catch (e: any) {
    showToast?.('error', '识别失败', e.message || '链接格式错误')
  }
}

// 把解析结果写入外部节点表单
const applyParsedToForm = (p: ParsedProxy) => {
  const v = form.value
  if (p.name) v.name = p.name
  // socks5 链接映射到后端的 socks 协议
  v.protocol = p.protocol === 'socks5' ? 'socks' : p.protocol
  v.host = p.host
  v.port = p.port
  // Socks5：用户名为 uuid 字段，密码为 password 字段
  v.uuid = p.protocol === 'socks5' ? (p.username || p.uuid) : (p.uuid || p.password)
  v.password = p.protocol === 'socks5' ? p.password : ''
  v.tls_enabled = p.tls
  v.server_name = p.sni
  v.alpn = p.alpn
  v.reality_enabled = p.reality
  v.reality_pubkey = p.pubKey
  v.reality_short_id = p.shortId
  v.flow = p.flow

  if (p.protocol === 'shadowsocks') {
    v.ss_method = p.method
    v.ss_password = p.password
  }
  if (p.protocol === 'hysteria2') {
    v.hy2_password = p.password
    if (p.upMbps) v.hy2_up_mbps = p.upMbps
    if (p.downMbps) v.hy2_down_mbps = p.downMbps
    v.hy2_obfs = p.obfs
    v.hy2_obfs_password = p.obfsPassword
  }
  if (p.protocol === 'tuic') {
    v.tuic_password = p.password
    if (!v.tuic_congestion) v.tuic_congestion = 'bbr'
    if (!v.tuic_udp_relay_mode) v.tuic_udp_relay_mode = 'native'
  }

  // 传输层
  if (p.network && p.network !== 'tcp') {
    v.transport_enabled = true
    v.transport_type = p.network === 'h2' ? 'h2' : p.network
    v.ws_path = p.wsPath
    v.transport_host = p.wsHost
    v.grpc_service = p.grpcService
  } else {
    v.transport_enabled = false
  }

  // 识别结果同样受协议硬约束，并同步分支高亮
  enforceProtocolConstraints()
  syncActiveBranch()
}

const openAddModal = () => {
  editingId.value = null
  form.value = defaultForm()
  activeBranch.value = null
  if (modal) modal.show()
}

const openEditModal = (node: ExternalNode) => {
  editingId.value = node.id
  form.value = { ...defaultForm(), ...node }
  enforceProtocolConstraints()
  syncActiveBranch()
  if (modal) modal.show()
}

const saveNode = async () => {
  if (!form.value.name || !form.value.protocol || !form.value.host || !form.value.port) {
    showToast?.('error', '错误', '请填写名称、协议、主机和端口')
    return
  }
  saving.value = true
  try {
    // 显式带上 TUIC / Socks 字段，避免后端 map 更新时丢失
    const payload: ExternalNodeForm = {
      ...form.value,
      tuic_password: form.value.tuic_password || '',
      tuic_congestion: form.value.tuic_congestion || 'bbr',
      tuic_udp_relay_mode: form.value.tuic_udp_relay_mode || 'native',
      password: form.value.password || ''
    }
    if (editingId.value) {
      await updateExternalNode(editingId.value, payload)
      showToast?.('success', '成功', '外部节点已更新')
    } else {
      await createExternalNode(payload)
      showToast?.('success', '成功', '外部节点已添加')
    }
    modal?.hide()
    await loadNodes()
  } catch (e: any) {
    showToast?.('error', '错误', e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const confirmDelete = (node: ExternalNode) => {
  deleteTarget.value = node
  if (deleteModal) deleteModal.show()
}

const doDelete = async () => {
  if (!deleteTarget.value) return
  saving.value = true
  try {
    await deleteExternalNode(deleteTarget.value.id)
    showToast?.('success', '成功', '外部节点已删除')
    deleteModal?.hide()
    await loadNodes()
  } catch (e: any) {
    showToast?.('error', '错误', e.response?.data?.error || '删除失败')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadNodes()
  if (modalEl.value) modal = new Modal(modalEl.value)
  if (deleteModalEl.value) deleteModal = new Modal(deleteModalEl.value)
})
</script>

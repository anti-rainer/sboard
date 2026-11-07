<template>
  <div class="container-fluid mt-4">
    <div class="sb-page-head">
      <div>
        <h4><i class="bi bi-diagram-3"></i> 节点管理</h4>
        <div class="sb-subtitle">共 {{ nodes.length }} 个入站节点</div>
      </div>
      <div class="sb-toolbar">
        <button class="btn btn-sm btn-secondary" @click="showConfigModal">
          <i class="bi bi-file-code"></i> 查看配置
        </button>
        <button class="btn btn-sm btn-primary" @click="openAddModal">
          <i class="bi bi-plus-lg"></i> 添加节点
        </button>
      </div>
    </div>

    <div id="nodesList" class="row g-3">
      <div 
        v-for="node in nodes" 
        :key="node.id" 
        class="col-xl-3 col-lg-4 col-md-6"
      >
        <div class="card node-card">
          <div class="card-header">
            <span class="node-tag" :title="node.tag">{{ node.tag }}</span>
            <div class="node-head-right">
              <span :class="['badge', node.enabled ? 'bg-success' : 'bg-secondary']">
                {{ node.enabled ? '启用' : '禁用' }}
              </span>
              <button class="sb-icon-btn sb-icon-btn-xs" @click="openEditModal(node)" title="编辑">
                <i class="bi bi-pencil"></i>
              </button>
              <button class="sb-icon-btn sb-icon-btn-xs sb-icon-btn-danger" @click="confirmDelete(node)" title="删除">
                <i class="bi bi-trash"></i>
              </button>
            </div>
          </div>
          <div class="card-body">
            <div class="node-meta">
              <span class="node-proto">{{ node.protocol.toUpperCase() }}</span>
              <span class="node-port">:{{ node.port }}</span>
            </div>
            <div class="node-tags" v-if="node.tls_enabled || (node.transport_enabled && node.transport_type)">
              <span v-if="node.tls_enabled" class="node-chip">
                <i class="bi bi-shield-check"></i>
                {{ node.reality_enabled ? 'Reality' : 'TLS' }}
              </span>
              <span v-if="node.transport_enabled && node.transport_type" class="node-chip">
                <i class="bi bi-arrow-left-right"></i>
                {{ node.transport_type.toUpperCase() }}
              </span>
            </div>
          </div>
        </div>
      </div>
      
      <div v-if="nodes.length === 0" class="col-12">
        <div class="empty-state">
          <i class="bi bi-diagram-3"></i>
          <p>还没有节点，点击右上角「添加节点」开始</p>
        </div>
      </div>
    </div>

    <!-- 节点模态框 -->
    <div class="modal fade" id="nodeModal" tabindex="-1" ref="nodeModalEl">
      <div class="modal-dialog modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 id="modalTitle">{{ isEditing ? '编辑节点' : '添加节点' }}</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <form @submit.prevent="saveNode">
            <div class="modal-body">
              <!-- ═══ 基础信息 ═══ -->
              <div class="sb-form-section">
                <div class="sb-form-section-title">
                  <i class="bi bi-hdd-network"></i> 基础信息
                </div>
                <div class="row g-3">
                  <div class="col-sm-6">
                    <label class="form-label">标签 <span class="sb-req">*</span></label>
                    <input type="text" class="form-control" v-model="formData.tag" required>
                  </div>
                  <div class="col-sm-6">
                    <label class="form-label">协议</label>
                    <select class="form-select" v-model="formData.protocol" @change="onProtocolChange">
                      <option v-for="p in MAIN_PROTOCOLS" :key="p.id" :value="p.id">{{ p.label }}</option>
                    </select>
                  </div>
                  <div class="col-sm-8" v-if="branches.length">
                    <label class="form-label">协议分支</label>
                    <div class="branch-pills">
                      <button type="button" v-for="b in branches" :key="b.id"
                              class="branch-pill"
                              :class="{ active: activeBranch === b.id }"
                              :title="b.hint || b.label"
                              @click="applyBranch(b)">
                        {{ b.label }}
                      </button>
                    </div>
                  </div>
                  <div class="col-sm-4" v-if="branches.length">
                    <label class="form-label">端口 <span class="sb-req">*</span></label>
                    <input type="number" class="form-control" v-model.number="formData.port" min="1" max="65535" required>
                  </div>
                  <div class="col-12" v-if="!branches.length">
                    <div class="row g-3">
                      <div class="col-sm-6">
                        <label class="form-label">端口 <span class="sb-req">*</span></label>
                        <input type="number" class="form-control" v-model.number="formData.port" min="1" max="65535" required>
                      </div>
                      <div class="col-sm-6">
                        <label class="form-label">监听地址</label>
                        <input type="text" class="form-control" v-model="formData.listen"
                               placeholder=":: （留空默认监听全部地址）">
                      </div>
                    </div>
                  </div>
                  <div class="col-12" v-if="branches.length">
                    <label class="form-label">监听地址</label>
                    <input type="text" class="form-control" v-model="formData.listen"
                           placeholder=":: （留空默认监听全部地址）">
                  </div>
                  <div class="col-12" v-if="activeBranchHint">
                    <div class="sb-hint">
                      <i class="bi bi-info-circle"></i> {{ activeBranchHint }}
                    </div>
                  </div>
                </div>
              </div>

              <!-- ═══ 传输安全 ═══ -->
              <div class="sb-form-section"
                   v-if="!['shadowsocks','socks'].includes(formData.protocol)">
                <div class="sb-form-section-title">
                  <i class="bi bi-shield-lock"></i> 传输安全
                  <span class="sb-form-section-note">
                    {{ formData.protocol === 'naive' ? 'naive 强制 TLS' :
                       (['hysteria2','tuic'].includes(formData.protocol) ? 'QUIC 系强制 TLS' : '') }}
                  </span>
                </div>

                <div class="sb-switch-row" v-if="!['naive','hysteria2','tuic'].includes(formData.protocol)">
                  <div class="form-check form-switch mb-0">
                    <input class="form-check-input" type="checkbox" role="switch"
                           v-model="formData.tls_enabled" id="tlsEnabled">
                    <label class="form-check-label" for="tlsEnabled">启用 TLS</label>
                  </div>
                  <div class="form-check form-switch mb-0"
                       v-if="formData.protocol !== 'naive' && (formData.tls_enabled || formData.reality_enabled)">
                    <input class="form-check-input" type="checkbox" role="switch"
                           v-model="formData.reality_enabled" id="realityEnabled">
                    <label class="form-check-label" for="realityEnabled">启用 Reality</label>
                  </div>
                </div>

                <div v-if="(formData.tls_enabled || ['naive','hysteria2','tuic'].includes(formData.protocol))"
                     id="tlsFields" class="row g-3 mt-1">
                  <div class="col-sm-6">
                    <label class="form-label">Server Name</label>
                    <input type="text" class="form-control" v-model="formData.server_name">
                  </div>

                  <!-- 证书字段（Reality 启用时隐藏） -->
                  <template v-if="!formData.reality_enabled">
                    <div class="col-sm-3">
                      <label class="form-label">证书路径</label>
                      <input type="text" class="form-control" v-model="formData.cert_path">
                    </div>
                    <div class="col-sm-3">
                      <label class="form-label">密钥路径</label>
                      <input type="text" class="form-control" v-model="formData.key_path">
                    </div>
                  </template>

                  <!-- Reality 字段 -->
                  <template v-if="formData.reality_enabled && formData.protocol !== 'naive'">
                    <div class="col-12">
                      <label class="form-label">Reality 握手服务器</label>
                      <input type="text" class="form-control" v-model="formData.reality_server"
                             placeholder="www.apple.com">
                      <div class="sb-hint">用于伪装的真实网站，格式 host:port 或 host（默认 443）</div>
                    </div>
                    <div class="col-12">
                      <label class="form-label d-flex align-items-center justify-content-between">
                        <span>密钥对</span>
                        <button type="button" class="btn btn-sm btn-outline-primary"
                                @click="generateRealityKeys" :disabled="generatingRealityKeys">
                          <span v-if="generatingRealityKeys" class="spinner-border spinner-border-sm me-1"></span>
                          <i v-else class="bi bi-key"></i>
                          {{ generatingRealityKeys ? '生成中...' : '生成密钥对' }}
                        </button>
                      </label>
                      <div class="row g-2">
                        <div class="col-12 col-sm-5">
                          <input type="text" class="form-control" v-model.trim="formData.reality_pubkey"
                                 placeholder="Public Key" autocomplete="off">
                        </div>
                        <div class="col-12 col-sm-5">
                          <input type="text" class="form-control" v-model.trim="formData.reality_privkey"
                                 placeholder="Private Key（部署时使用）" autocomplete="off">
                        </div>
                        <div class="col-12 col-sm-2">
                          <input type="text" class="form-control" v-model.trim="formData.reality_short_id"
                                 placeholder="Short ID" maxlength="16" autocomplete="off">
                        </div>
                      </div>
                      <div class="sb-hint">留空时保存自动生成 Short ID；私钥将在部署时写入服务器配置</div>
                    </div>
                  </template>
                </div>
              </div>

              <!-- ═══ 传输层 ═══ -->
              <div class="sb-form-section"
                   v-if="!['shadowsocks','hysteria2','naive','socks','tuic'].includes(formData.protocol)">
                <div class="sb-form-section-title">
                  <i class="bi bi-diagram-3"></i> 传输层
                </div>
                <div class="sb-switch-row" v-if="formData.transport_enabled">
                  <div class="form-check form-switch mb-0">
                    <input class="form-check-input" type="checkbox" role="switch"
                           v-model="formData.transport_enabled" id="transportEnabled">
                    <label class="form-check-label" for="transportEnabled">启用传输层</label>
                  </div>
                </div>
                <div class="sb-switch-row" v-else>
                  <div class="form-check form-switch mb-0">
                    <input class="form-check-input" type="checkbox" role="switch"
                           v-model="formData.transport_enabled" id="transportEnabled">
                    <label class="form-check-label" for="transportEnabled">启用传输层</label>
                  </div>
                </div>

                <div v-if="formData.transport_enabled" id="transportFields" class="row g-3 mt-1">
                  <div class="col-sm-5">
                    <label class="form-label">传输层类型</label>
                    <select class="form-select" v-model="formData.transport_type">
                      <option value="ws">WebSocket (ws)</option>
                      <option value="grpc">gRPC</option>
                      <option value="h2">HTTP/2 (h2)</option>
                      <option value="httpupgrade">HTTPUpgrade</option>
                      <option value="quic">QUIC</option>
                    </select>
                  </div>
                  <template v-if="formData.transport_type === 'ws' || formData.transport_type === 'httpupgrade'">
                    <div class="col-sm-3">
                      <label class="form-label">Path</label>
                      <input type="text" class="form-control" v-model="formData.ws_path" placeholder="/path">
                    </div>
                    <div class="col-sm-4">
                      <label class="form-label">Host（伪装域名）</label>
                      <input type="text" class="form-control" v-model="formData.transport_host"
                             placeholder="cdn.example.com">
                    </div>
                  </template>
                  <template v-if="formData.transport_type === 'grpc'">
                    <div class="col-sm-7">
                      <label class="form-label">gRPC Service Name</label>
                      <input type="text" class="form-control" v-model="formData.grpc_service">
                    </div>
                  </template>
                </div>
              </div>

              <!-- Flow 配置 (仅VLESS) -->
              <div v-if="formData.protocol === 'vless' && !formData.transport_enabled" id="flowFields"
                   class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-signpost-split"></i> XTLS</div>
                <div class="row g-3">
                  <div class="col-sm-6">
                    <label class="form-label">Flow</label>
                    <select class="form-select" v-model="formData.flow">
                      <option value="">无</option>
                      <option value="xtls-rprx-vision">xtls-rprx-vision（推荐）</option>
                      <option value="xtls-rprx-direct">xtls-rprx-direct（已弃用）</option>
                    </select>
                    <div class="sb-hint">仅在不使用传输层时可用</div>
                  </div>
                </div>
              </div>

              <!-- ═══ Shadowsocks ═══ -->
              <div v-if="formData.protocol === 'shadowsocks'" id="shadowsocksFields"
                   class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-lock"></i> Shadowsocks</div>
                <div class="row g-3">
                  <div class="col-sm-6">
                    <label class="form-label">加密方法 <span class="sb-req">*</span></label>
                    <select class="form-select" v-model="formData.ss_method">
                      <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm（32 字节，推荐）</option>
                      <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm（16 字节）</option>
                    </select>
                  </div>
                  <div class="col-12">
                    <div class="sb-hint">
                      <i class="bi bi-info-circle"></i>
                      SS2022 支持多用户独立密钥，每个用户密码独立、到期自动失效；UDP 已默认启用。
                    </div>
                  </div>
                </div>
              </div>

              <!-- ═══ Hysteria2 ═══ -->
              <div v-if="formData.protocol === 'hysteria2'" id="hysteria2Fields"
                   class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-lightning-charge"></i> Hysteria2</div>
                <div class="row g-3">
                  <div class="col-sm-6">
                    <label class="form-label">上行带宽 (Mbps)</label>
                    <input type="number" class="form-control" v-model.number="formData.hy2_up_mbps" min="0" max="10000">
                  </div>
                  <div class="col-sm-6">
                    <label class="form-label">下行带宽 (Mbps)</label>
                    <input type="number" class="form-control" v-model.number="formData.hy2_down_mbps" min="0" max="10000">
                  </div>
                  <div class="col-sm-6">
                    <label class="form-label">混淆类型</label>
                    <select class="form-select" v-model="formData.hy2_obfs">
                      <option value="">不启用混淆</option>
                      <option value="salamander">Salamander（推荐）</option>
                    </select>
                  </div>
                  <div class="col-sm-6" v-if="formData.hy2_obfs === 'salamander'" id="hy2ObfsFields">
                    <label class="form-label">混淆密码 <span class="sb-req">*</span></label>
                    <input type="text" class="form-control" v-model="formData.hy2_obfs_password"
                           placeholder="与认证密码不同">
                  </div>
                  <div class="col-12">
                    <div class="sb-hint">
                      <i class="bi bi-info-circle"></i>
                      带宽填 0 表示不限速（与 233boy 默认一致）；每个用户使用各自的密码（即用户 UUID）。
                    </div>
                  </div>
                </div>
              </div>

              <!-- ═══ TUIC ═══ -->
              <div v-if="formData.protocol === 'tuic'" id="tuicFields"
                   class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-lightning"></i> TUIC</div>
                <div class="row g-3">
                  <div class="col-sm-6">
                    <label class="form-label">拥塞控制算法</label>
                    <select class="form-select" v-model="formData.tuic_congestion">
                      <option value="bbr">bbr（推荐）</option>
                      <option value="cubic">cubic</option>
                      <option value="new_reno">new_reno</option>
                    </select>
                  </div>
                  <div class="col-sm-6 d-flex align-items-end">
                    <div class="form-check form-switch mb-0">
                      <input class="form-check-input" type="checkbox" v-model="formData.tuic_zero_rtt"
                             id="tuicZeroRtt" role="switch">
                      <label class="form-check-label" for="tuicZeroRtt">启用 0-RTT 握手</label>
                    </div>
                  </div>
                  <div class="col-12">
                    <div class="sb-hint">
                      <i class="bi bi-info-circle"></i>
                      基于 QUIC（UDP），ALPN 固定 h3；每个用户使用各自的密码（即用户 UUID）。
                    </div>
                  </div>
                </div>
              </div>

              <!-- ═══ Socks ═══ -->
              <div v-if="formData.protocol === 'socks'" id="socksFields"
                   class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-plug"></i> Socks5</div>
                <div class="sb-hint">
                  <i class="bi bi-info-circle"></i>
                  每个用户独立认证：用户名 = 用户名称，密码 = 用户 UUID；不支持 TLS 与传输层。
                </div>
              </div>

              <!-- ═══ Naive ═══ -->
              <div v-if="formData.protocol === 'naive'" id="naiveFields"
                   class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-incognito"></i> Naive</div>
                <div class="sb-hint">
                  <i class="bi bi-info-circle"></i>
                  基于 HTTP/2 或 HTTP/3（QUIC），强制 TLS；每个用户独立 username + password。
                  客户端：naiveproxy、sing-box 1.13.0+。
                </div>
              </div>

              <!-- ═══ 备注与状态 ═══ -->
              <div class="sb-form-section">
                <div class="sb-form-section-title"><i class="bi bi-card-text"></i> 备注与状态</div>
                <div class="row g-3">
                  <div class="col-12">
                    <label class="form-label">备注</label>
                    <textarea class="form-control" v-model="formData.notes" rows="2"
                              placeholder="可选，输入节点的备注信息"></textarea>
                  </div>
                  <div class="col-12">
                    <div class="form-check form-switch mb-0">
                      <input class="form-check-input" type="checkbox" role="switch"
                             v-model="formData.enabled" id="nodeEnabled">
                      <label class="form-check-label" for="nodeEnabled">启用节点</label>
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <div class="modal-footer">
              <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
              <button type="submit" class="btn btn-primary" :disabled="saving">
                <span v-if="saving" class="spinner-border spinner-border-sm me-1"></span>
                保存
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>

    <!-- 配置文件查看模态框 -->
    <div class="modal fade" id="configModal" tabindex="-1" ref="configModalEl">
      <div class="modal-dialog modal-dialog-scrollable modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title"><i class="bi bi-file-code"></i> 服务器配置文件</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <!-- 配置类型选择 -->
            <div class="btn-group w-100 mb-3" role="group">
              <input type="radio" class="btn-check" name="configType" id="configSingbox" value="sing-box" v-model="configType">
              <label class="btn btn-outline-success" for="configSingbox">
                <i class="bi bi-box-seam"></i> Sing-Box
              </label>
            </div>
            
            <!-- 配置内容显示 -->
            <div v-if="configLoading" class="text-center py-4">
              <div class="spinner-border text-primary" role="status">
                <span class="visually-hidden">Loading...</span>
              </div>
              <p class="mt-2 text-muted">加载配置中...</p>
            </div>
            <pre id="configContent" v-else style="white-space: pre-wrap; word-break: break-word; background: #1e1e1e; color: #d4d4d4; padding: 1rem; border-radius: 8px; max-height: 450px; overflow-y: auto; font-family: 'Consolas', 'Monaco', monospace; font-size: 0.85rem;">{{ configContent }}</pre>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-primary" @click="copyConfig">
              <i class="bi bi-clipboard"></i> 复制配置
            </button>
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">关闭</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 删除确认模态框 -->
    <div class="modal fade" id="deleteModal" tabindex="-1" ref="deleteModalEl">
      <div class="modal-dialog modal-dialog-centered">
        <div class="modal-content border-0 shadow-lg">
          <div class="modal-header bg-danger text-white border-0">
            <h5 class="modal-title">
              <i class="bi bi-exclamation-triangle-fill me-2"></i>
              确认删除
            </h5>
            <button type="button" class="btn-close btn-close-white" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body py-4">
            <div class="text-center mb-3">
              <i class="bi bi-trash3 text-danger" style="font-size: 3rem;"></i>
            </div>
            <p class="text-center mb-2 fs-5">
              确定要删除节点 <strong class="text-danger">"{{ deleteTarget?.tag }}"</strong> 吗？
            </p>
            <p class="text-center text-muted small mb-0">
              <i class="bi bi-info-circle me-1"></i>此操作不可撤销！
            </p>
          </div>
          <div class="modal-footer border-0 justify-content-center pb-4">
            <button type="button" class="btn btn-secondary px-4" data-bs-dismiss="modal">
              <i class="bi bi-x-lg me-1"></i>取消
            </button>
            <button type="button" class="btn btn-danger px-4" @click="doDelete" :disabled="deleting">
              <span v-if="deleting" class="spinner-border spinner-border-sm me-1"></span>
              <i v-else class="bi bi-trash3 me-1"></i>确认删除
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, inject, watch, nextTick } from 'vue'
import { MAIN_PROTOCOLS, getBranches, detectBranch, type ProtocolBranch } from '@/utils/protocolBranches'
import { Modal } from 'bootstrap'
import { getNodes, createNode, updateNode, deleteNode, getConfig, generateRealityKeys as apiGenerateRealityKeys, type Node } from '@/api'

const showToast = inject<(type: 'success' | 'error' | 'warning' | 'info', title: string, message: string) => void>('showToast')!

const nodes = ref<Node[]>([])
const loading = ref(false)

// 模态框
const nodeModalEl = ref<HTMLElement | null>(null)
const configModalEl = ref<HTMLElement | null>(null)
const deleteModalEl = ref<HTMLElement | null>(null)
let nodeModal: Modal | null = null
let configModal: Modal | null = null
let deleteModal: Modal | null = null

// 表单
const isEditing = ref(false)
const saving = ref(false)
const generatingRealityKeys = ref(false)
const formData = ref(getDefaultFormData())

// 删除
const deleteTarget = ref<Node | null>(null)
const deleting = ref(false)

// 配置查看
const configType = ref('sing-box')
const configContent = ref('')
const configLoading = ref(false)

// 协议分支
const branches = computed(() => getBranches(formData.value.protocol))
const activeBranch = ref<string | null>(null)
const activeBranchHint = computed(() => {
  const b = branches.value.find((x) => x.id === activeBranch.value)
  return b?.hint || ''
})

/** 把分支预设写入表单（不引入新字段，只按预设填既有字段） */
function applyBranch(b: ProtocolBranch) {
  activeBranch.value = b.id
  const f = formData.value
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
  if (p.ssObfs !== undefined) f.ss_obfs_mode = p.ssObfs
  if (p.hy2Obfs !== undefined) f.hy2_obfs = p.hy2Obfs
  if (p.congestion !== undefined) f.tuic_congestion = p.congestion
  // 窄屏上分支胶囊是单行横向滚动，选中项可能在可视区外，
  // 主动滚进来，否则用户看不到当前选的是哪个。
  nextTick(() => {
    const el = document.querySelector('.branch-pills .branch-pill.active') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest', inline: 'center' })
    updatePillsScrollable()
  })
}

/** 标记分支胶囊是否有横向溢出，用于决定是否显示右侧渐隐提示 */
function updatePillsScrollable() {
  const box = document.querySelector('.branch-pills') as HTMLElement | null
  if (!box) return
  box.classList.toggle('is-scrollable', box.scrollWidth > box.clientWidth + 1)
}

/** 依据当前表单状态反推命中的分支 */
function syncActiveBranch() {
  const f = formData.value
  activeBranch.value = detectBranch(f.protocol, {
    tls: f.tls_enabled,
    reality: f.reality_enabled,
    transport: f.transport_enabled ? f.transport_type : '',
    flow: f.flow,
    ssMethod: f.ss_method,
    hy2Obfs: f.hy2_obfs,
    congestion: f.tuic_congestion,
  })
}

function getDefaultFormData() {
  return {
    id: 0,
    tag: '',
    protocol: 'trojan',
    listen: '::',
    port: 443,
    tls_enabled: false,
    server_name: 'down.dingtalk.com',
    cert_path: 'server.crt',
    key_path: 'server.key',
    reality_enabled: false,
    reality_server: '',
    reality_pubkey: '',
    reality_privkey: '',
    reality_short_id: '',
    transport_enabled: false,
    transport_type: 'ws',
    ws_path: '/',
    transport_host: '',
    grpc_service: 'GunService',
    flow: '',
    ss_method: '2022-blake3-aes-256-gcm',
    ss_password: '',
    hy2_up_mbps: 0,
    hy2_down_mbps: 0,
    hy2_obfs: '',
    hy2_obfs_password: '',
    tuic_congestion: 'bbr',
    tuic_zero_rtt: false,
    notes: '',
    enabled: true
  }
}

onMounted(async () => {
  await loadNodes()
  nodeModal = new Modal(nodeModalEl.value!)
  configModal = new Modal(configModalEl.value!)
  deleteModal = new Modal(deleteModalEl.value!)
})

// 监听配置类型变化
watch(configType, () => {
  if (configModal) {
    loadConfig()
  }
})

async function loadNodes() {
  loading.value = true
  try {
    const res = await getNodes()
    nodes.value = res.data.data.nodes || []
  } catch (error) {
    showToast('error', '错误', '加载节点列表失败')
  } finally {
    loading.value = false
  }
}

function onProtocolChange() {
  // Shadowsocks 不支持 TLS
  if (formData.value.protocol === 'shadowsocks') {
    formData.value.tls_enabled = false
    formData.value.transport_enabled = false
  }
  // Hysteria2 必须启用 TLS，不支持传输层
  if (formData.value.protocol === 'hysteria2') {
    formData.value.tls_enabled = true
    formData.value.transport_enabled = false
  }
  // Naive 必须启用 TLS，不支持传输层和 Reality
  if (formData.value.protocol === 'naive') {
    formData.value.tls_enabled = true
    formData.value.transport_enabled = false
    formData.value.reality_enabled = false
  }
  // TUIC 基于 QUIC，强制 TLS，不支持传输层与 Reality
  if (formData.value.protocol === 'tuic') {
    formData.value.tls_enabled = true
    formData.value.transport_enabled = false
    formData.value.reality_enabled = false
  }
  // Socks 不支持 TLS / 传输层 / Reality
  if (formData.value.protocol === 'socks') {
    formData.value.tls_enabled = false
    formData.value.transport_enabled = false
    formData.value.reality_enabled = false
  }
  // 切换协议后自动套用该协议的首个分支，避免表单停留在上一种协议的组合
  const list = getBranches(formData.value.protocol)
  if (list.length > 0) {
    applyBranch(list[0])
  } else {
    activeBranch.value = null
  }
}

async function generateRealityKeys() {
  if (generatingRealityKeys.value) return
  generatingRealityKeys.value = true
  try {
    const res = await apiGenerateRealityKeys()
    const keys = res.data.data
    formData.value.reality_pubkey = keys.public_key
    formData.value.reality_privkey = keys.private_key
    formData.value.reality_short_id = keys.short_id
    showToast('success', '成功', '已生成 Reality 密钥对')
  } catch (err: any) {
    showToast('error', '错误', '生成 Reality 密钥对失败: ' + (err.response?.data?.error || err.message))
  } finally {
    generatingRealityKeys.value = false
  }
}

function openAddModal() {
  isEditing.value = false
  formData.value = getDefaultFormData()
  // 默认套用第一个分支：既让分支胶囊有明确选中态，
  // 也避免建出该协议不合法的组合（如 trojan 不带 TLS）
  const first = branches.value[0]
  if (first) applyBranch(first)
  else syncActiveBranch()
  nodeModal?.show()
}

function openEditModal(node: Node) {
  isEditing.value = true
  formData.value = {
    id: node.id,
    tag: node.tag,
    protocol: node.protocol,
    listen: node.listen || '::',
    port: node.port,
    tls_enabled: node.tls_enabled,
    server_name: node.server_name || '',
    cert_path: node.cert_path || '',
    key_path: node.key_path || '',
    reality_enabled: node.reality_enabled,
    reality_server: node.reality_server || '',
    reality_pubkey: node.reality_pubkey || '',
    reality_privkey: node.reality_privkey || '',
    reality_short_id: node.reality_short_id || '',
    transport_enabled: node.transport_enabled,
    transport_type: node.transport_type || 'ws',
    ws_path: node.ws_path || '/',
    transport_host: node.transport_host || '',
    grpc_service: node.grpc_service || 'GunService',
    flow: node.flow || '',
    ss_method: node.ss_method || '2022-blake3-aes-256-gcm',
    ss_password: node.ss_password || '',
    hy2_up_mbps: node.hy2_up_mbps || 0,
    hy2_down_mbps: node.hy2_down_mbps || 0,
    hy2_obfs: node.hy2_obfs || '',
    hy2_obfs_password: node.hy2_obfs_password || '',
    tuic_congestion: (node as any).tuic_congestion || 'bbr',
    tuic_zero_rtt: (node as any).tuic_zero_rtt || false,
    notes: node.notes || '',
    enabled: node.enabled
  }
  syncActiveBranch()
  nodeModal?.show()
}

async function saveNode() {
  if (!formData.value.tag) {
    showToast('warning', '警告', '请输入节点标签')
    return
  }
  if (!formData.value.port) {
    showToast('warning', '警告', '请输入端口')
    return
  }
  if (formData.value.reality_enabled && formData.value.protocol !== 'naive') {
    if (!formData.value.reality_server || !formData.value.reality_pubkey || !formData.value.reality_privkey) {
      showToast('warning', '警告', '启用 Reality 时请填写握手服务器、公钥和私钥')
      return
    }
  }
  // Hysteria2 选了混淆就必须给密码：缺密码时客户端会直接崩
  //   FATAL initialize outbound: missing obfs password
  // 而节点在面板上看着完全正常，所以必须在这里拦住。
  if (formData.value.protocol === 'hysteria2'
      && formData.value.hy2_obfs
      && !formData.value.hy2_obfs_password) {
    showToast('warning', '警告', '启用 Salamander 混淆时必须填写混淆密码')
    return
  }

  saving.value = true
  try {
    const data: any = {
      tag: formData.value.tag,
      protocol: formData.value.protocol,
      listen: formData.value.listen,
      port: formData.value.port,
      tls_enabled: formData.value.tls_enabled || formData.value.reality_enabled,
      server_name: formData.value.server_name,
      cert_path: formData.value.cert_path,
      key_path: formData.value.key_path,
      reality_enabled: formData.value.reality_enabled,
      reality_server: formData.value.reality_server,
      reality_pubkey: formData.value.reality_pubkey,
      reality_privkey: formData.value.reality_privkey,
      reality_short_id: formData.value.reality_short_id,
      transport_enabled: formData.value.transport_enabled,
      transport_type: formData.value.transport_type,
      ws_path: formData.value.ws_path,
      transport_host: formData.value.transport_host,
      grpc_service: formData.value.grpc_service,
      flow: formData.value.flow,
      ss_method: formData.value.ss_method,
      ss_password: formData.value.ss_password,
      hy2_up_mbps: formData.value.hy2_up_mbps,
      hy2_down_mbps: formData.value.hy2_down_mbps,
      hy2_obfs: formData.value.hy2_obfs,
      hy2_obfs_password: formData.value.hy2_obfs_password,
      tuic_congestion: formData.value.tuic_congestion,
      tuic_zero_rtt: formData.value.tuic_zero_rtt,
      notes: formData.value.notes,
      enabled: formData.value.enabled
    }

    if (isEditing.value) {
      await updateNode(formData.value.id, data)
      showToast('success', '成功', '节点已更新')
    } else {
      await createNode(data)
      showToast('success', '成功', '节点已添加')
    }
    
    nodeModal?.hide()
    await loadNodes()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

function confirmDelete(node: Node) {
  deleteTarget.value = node
  deleteModal?.show()
}

async function doDelete() {
  if (!deleteTarget.value) return
  
  deleting.value = true
  try {
    await deleteNode(deleteTarget.value.id)
    showToast('success', '成功', '节点已删除')
    deleteModal?.hide()
    await loadNodes()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '删除失败')
  } finally {
    deleting.value = false
  }
}

async function showConfigModal() {
  configModal?.show()
  await loadConfig()
}

async function loadConfig() {
  configLoading.value = true
  try {
    const res = await getConfig(configType.value)
    configContent.value = res.data.data?.config || ''
  } catch (error) {
    configContent.value = '加载配置失败'
    showToast('error', '错误', '加载配置失败')
  } finally {
    configLoading.value = false
  }
}

function copyConfig() {
  navigator.clipboard.writeText(configContent.value).then(() => {
    showToast('success', '成功', '配置已复制到剪贴板')
  }).catch(() => {
    showToast('error', '错误', '复制失败')
  })
}
</script>

<style scoped>

</style>

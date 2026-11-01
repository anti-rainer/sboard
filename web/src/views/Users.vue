<template>
  <div class="container-fluid mt-4">
    <div class="sb-page-head">
      <div>
        <h4><i class="bi bi-people"></i> 用户管理</h4>
        <div class="sb-subtitle">共 {{ users.length }} 个用户 · 按等级分组</div>
      </div>
      <button class="btn btn-primary" @click="openAddModal">
        <i class="bi bi-plus-lg"></i> 添加用户
      </button>
    </div>

    <div id="usersList" class="row g-3">
      <!-- 按等级分组显示 -->
      <template v-for="level in [1, 2, 3]" :key="level">
        <template v-if="getUsersByLevel(level).length > 0">
          <div class="col-12">
            <div class="sb-group-title">
              <i class="bi bi-layers"></i> 等级 {{ level }}
            </div>
          </div>
          <div 
            v-for="user in getUsersByLevel(level)" 
            :key="user.id" 
            class="col-xl-3 col-lg-4 col-md-6"
            :style="openDropdownId === user.id ? { position: 'relative', zIndex: 1000 } : {}"
          >
            <div class="card user-card">
              <div class="card-header">
                <span class="user-name">
                  <i class="bi bi-person-badge"></i> {{ user.name }}
                </span>
                <span :class="['badge', isExpired(user) ? 'bg-danger' : 'bg-success']">
                  {{ isExpired(user) ? '已过期' : '有效' }}
                </span>
              </div>
              <div class="card-body">
                <div class="sb-meta">
                  <span>到期时间</span>
                  <strong :class="isExpired(user) ? 'text-danger' : ''">
                    {{ user.expiry_date }}
                  </strong>
                </div>
                <div class="btn-group btn-group-sm w-100 mb-2">
                  <button class="btn btn-outline-primary" @click="openEditModal(user)">
                    <i class="bi bi-pencil"></i> 编辑
                  </button>
                  <button class="btn btn-outline-danger" @click="confirmDelete(user)">
                    <i class="bi bi-trash"></i> 删除
                  </button>
                </div>
                <!-- 订阅链接：弹框选择，避免下拉框在卡片底部被挤出视口 -->
                <button
                  v-if="nodes.length > 0"
                  class="btn btn-sm btn-primary w-100"
                  type="button"
                  @click="openSubModal(user)"
                >
                  <i class="bi bi-link-45deg"></i> 订阅链接
                </button>
              </div>
            </div>
          </div>
        </template>
      </template>
      
      <div v-if="users.length === 0" class="col-12 text-center py-5">
        <p class="text-muted">暂无用户</p>
      </div>
    </div>

    <!-- 订阅链接弹框 -->
    <div class="modal fade" id="subLinkModal" tabindex="-1" ref="subModalEl">
      <div class="modal-dialog modal-dialog-centered modal-dialog-scrollable">
        <div class="modal-content">
          <div class="modal-header">
            <div>
              <h5 class="modal-title mb-0">
                <i class="bi bi-link-45deg"></i> 订阅链接
              </h5>
              <div class="sub-modal-sub" v-if="subTarget">
                {{ subTarget.name }}
                <span class="text-muted">· {{ nodes.length }} 个节点</span>
              </div>
            </div>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <template v-if="nodes.length > 0">
              <!-- 节点切换：顶部横向标签 -->
              <div class="node-tabs" role="tablist">
                <button
                  v-for="node in nodes"
                  :key="node.id"
                  class="node-tab"
                  :class="{ active: activeNodeTag === node.tag }"
                  type="button"
                  role="tab"
                  :aria-selected="activeNodeTag === node.tag"
                  @click="activeNodeTag = node.tag"
                >
                  <i class="bi bi-hdd-network"></i>
                  {{ node.tag }}
                </button>
              </div>

              <!-- 当前节点的复制项 -->
              <div class="sub-links" v-if="activeNodeTag">
                <!-- 原始订阅：三格式自适应，带节点 -->
                <button class="sub-link" type="button"
                        @click="copySubLink(subTarget!, activeNodeTag)">
                  <span class="sub-link-main">
                    <i class="bi bi-magic"></i>
                    原始订阅
                  </span>
                  <span class="sub-link-hint">自适应 mihomo / sing-box / v2ray</span>
                </button>
                <!-- 客户端配置与 provider -->
                <button v-for="item in clientFiles" :key="item.client" class="sub-link" type="button"
                        @click="copyClientFile(subTarget!, activeNodeTag, item)">
                  <span class="sub-link-main">
                    <i :class="item.icon"></i>
                    {{ item.label }}
                  </span>
                  <span class="sub-link-hint">{{ item.hint }}</span>
                </button>
              </div>
            </template>
            <div v-else class="empty-state">
              <i class="bi bi-diagram-3"></i>
              <p>还没有节点，先到「节点管理」添加</p>
            </div>
          </div>
          <div class="modal-footer">
            <span class="sub-modal-tip">
              <i class="bi bi-info-circle"></i> 点击任意一项即可复制链接
            </span>
            <button type="button" class="btn btn-secondary btn-sm" data-bs-dismiss="modal">关闭</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 用户模态框 -->
    <div class="modal fade" id="userModal" tabindex="-1" ref="userModalEl">
      <div class="modal-dialog">
        <div class="modal-content">
          <div class="modal-header">
            <h5 id="modalTitle">{{ isEditing ? '编辑用户' : '添加用户' }}</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <form @submit.prevent="saveUser">
            <div class="modal-body">
              <div class="mb-3">
                <label class="form-label">名称 <span class="text-danger">*</span></label>
                <input 
                  type="text" 
                  class="form-control" 
                  v-model="formData.name" 
                  required
                  placeholder="请输入用户名称"
                >
              </div>
              <div class="mb-3">
                <label class="form-label">UUID</label>
                <div class="input-group">
                  <input 
                    type="text" 
                    class="form-control" 
                    v-model="formData.uuid" 
                    placeholder="留空自动生成"
                  >
                  <button class="btn btn-outline-secondary" type="button" @click="generateUUID">
                    <i class="bi bi-arrow-repeat"></i>
                  </button>
                </div>
                <small class="text-muted">留空将自动生成UUID</small>
              </div>
              <div class="mb-3">
                <label class="form-label">等级 <span class="text-danger">*</span></label>
                <select class="form-select" v-model.number="formData.level" required>
                  <option :value="1">等级 1</option>
                  <option :value="2">等级 2</option>
                  <option :value="3">等级 3</option>
                </select>
              </div>
              <div class="mb-3">
                <label class="form-label">到期日期 <span class="text-danger">*</span></label>
                <input 
                  type="date" 
                  class="form-control" 
                  v-model="formData.expiry_date" 
                  required
                  :min="minDate"
                >
              </div>
              <div class="mb-3">
                <label class="form-label">DNS 解析策略</label>
                <select class="form-select" v-model="formData.dns_resolve">
                  <option value="default">默认 (使用服务器设置)</option>
                  <option value="ipv4">仅 IPv4</option>
                  <option value="ipv6">仅 IPv6</option>
                </select>
                <small class="text-muted">用于订阅时的 DNS 解析，"默认"将使用服务器设置</small>
              </div>
              <div class="form-check" id="enabledCheckbox" :style="{ display: isEditing ? 'block' : 'none' }">
                <input 
                  class="form-check-input" 
                  type="checkbox" 
                  v-model="formData.enabled" 
                  id="userEnabled"
                >
                <label class="form-check-label" for="userEnabled">启用用户</label>
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

    <!-- 删除确认模态框 -->
    <div class="modal fade" id="deleteConfirmModal" tabindex="-1" ref="deleteModalEl">
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
              确定要删除用户 <strong class="text-danger">"{{ deleteTarget?.name }}"</strong> 吗？
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
import { ref, onMounted, inject, computed } from 'vue'
import { Modal } from 'bootstrap'
import { getUsers, createUser, updateUser, deleteUser, getNodes, type User, type Node } from '@/api'

const showToast = inject<(type: 'success' | 'error' | 'warning' | 'info', title: string, message: string) => void>('showToast')!

const users = ref<User[]>([])
const nodes = ref<Node[]>([])
const loading = ref(false)
// 订阅链接弹框的目标用户
const subTarget = ref<User | null>(null)
// 弹框里当前选中的节点标签
const activeNodeTag = ref<string>('')

// 模态框
const userModalEl = ref<HTMLElement | null>(null)
const deleteModalEl = ref<HTMLElement | null>(null)
const subModalEl = ref<HTMLElement | null>(null)
let userModal: Modal | null = null
let deleteModal: Modal | null = null
let subModal: Modal | null = null

// 表单
const isEditing = ref(false)
const saving = ref(false)
const formData = ref({
  id: 0,
  name: '',
  uuid: '',
  level: 1,
  expiry_date: '',
  dns_resolve: 'default',
  enabled: true
})

// 删除
const deleteTarget = ref<User | null>(null)
const deleting = ref(false)

// 计算最小日期
const minDate = computed(() => new Date().toISOString().split('T')[0])

onMounted(async () => {
  await loadData()
  userModal = new Modal(userModalEl.value!)
  deleteModal = new Modal(deleteModalEl.value!)
  subModal = new Modal(subModalEl.value!)
})

async function loadData() {
  loading.value = true
  try {
    const [usersRes, nodesRes] = await Promise.all([getUsers(), getNodes()])
    users.value = usersRes.data.data.users || []
    nodes.value = nodesRes.data.data.nodes || []
  } catch (error) {
    showToast('error', '错误', '加载数据失败')
  } finally {
    loading.value = false
  }
}

function getUsersByLevel(level: number) {
  return users.value.filter(u => u.level === level)
}

function isExpired(user: User) {
  const today = new Date()
  const todayString = [today.getFullYear(), today.getMonth() + 1, today.getDate()]
    .map((value, index) => index === 0 ? String(value) : String(value).padStart(2, '0'))
    .join('-')
  return Boolean(user.expiry_date) && user.expiry_date < todayString
}

function openSubModal(user: User) {
  subTarget.value = user
  // 默认选中第一个节点，省掉一次点击
  activeNodeTag.value = nodes.value[0]?.tag || ''
  subModal?.show()
}

// 路径式客户端配置。client 段由后端映射到具体格式：
//   mihomo / sing-box -> provider 文件（只含节点）
//   shadowrocket / loon -> 分流配置（策略组 + 规则）
// 末段文件名只为客户端识别后缀，后端按 client 段决定输出。
const clientFiles = [
  { client: 'mihomo',       file: 'sboard.yaml', label: 'mihomo · provider',  icon: 'bi bi-filetype-yml',   hint: '仅节点，供订阅引用' },
  { client: 'sing-box',     file: 'sboard.json', label: 'sing-box · provider', icon: 'bi bi-filetype-json', hint: '仅节点，供订阅引用' },
  { client: 'shadowrocket', file: 'sboard.conf', label: '小火箭 · 分流配置',   icon: 'bi bi-rocket',         hint: '策略组 + 规则，导入即用' },
  { client: 'loon',         file: 'sboard.conf', label: 'Loon · 分流配置',     icon: 'bi bi-phone',          hint: '策略组 + 规则，导入即用' },
  { client: 'surge',        file: 'sboard.conf', label: 'Surge · 分流配置',    icon: 'bi bi-lightning-charge', hint: '策略组 + 规则，导入即用' }
]

function copyClientFile(user: User, nodeTag: string, item: { client: string; file: string; label: string; hint: string }) {
  // 链接格式保持原样；客户端备注名由响应头 Content-Disposition 决定
  const url = `${window.location.origin}/subscribe/${user.uuid}/${encodeURIComponent(nodeTag)}/${item.client}/${item.file}`
  copyToClipboard(url).then(() => {
    showToast('success', '成功', `已复制 ${item.label}`)
  }).catch(() => {
    showToast('error', '错误', '复制失败')
  })
  openNodeMenu.value = null
  openDropdownId.value = null
}

// 通用复制函数
function copyToClipboard(text: string): Promise<void> {
  return new Promise((resolve, reject) => {
    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(resolve).catch(reject)
    } else {
      try {
        const textarea = document.createElement('textarea')
        textarea.value = text
        textarea.style.position = 'fixed'
        textarea.style.left = '-9999px'
        document.body.appendChild(textarea)
        textarea.select()
        document.execCommand('copy')
        document.body.removeChild(textarea)
        resolve()
      } catch (err) {
        reject(err)
      }
    }
  })
}

function copySubLink(user: User, nodeTag: string) {
  // 原始订阅：UA 自适应，下发 mihomo / sing-box / v2ray 三种标准格式，
  // 节点内嵌。适合绝大多数客户端直接导入。
  const url = `${window.location.origin}/subscribe/${user.uuid}?node=${encodeURIComponent(nodeTag)}`
  copyToClipboard(url).then(() => {
    showToast('success', '成功', `已复制原始订阅: ${nodeTag}`)
  }).catch(() => {
    showToast('error', '错误', '复制失败')
  })
  openNodeMenu.value = null
  openDropdownId.value = null
}

function generateUUID() {
  formData.value.uuid = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = Math.random() * 16 | 0
    const v = c === 'x' ? r : (r & 0x3 | 0x8)
    return v.toString(16)
  })
}

function openAddModal() {
  isEditing.value = false
  formData.value = {
    id: 0,
    name: '',
    uuid: '',
    level: 1,
    expiry_date: minDate.value,
    dns_resolve: 'default',
    enabled: true
  }
  userModal?.show()
}

function openEditModal(user: User) {
  isEditing.value = true
  formData.value = {
    id: user.id,
    name: user.name,
    uuid: user.uuid,
    level: user.level,
    expiry_date: user.expiry_date,
    dns_resolve: user.dns_resolve || 'default',
    enabled: user.enabled === 1 || user.enabled === true
  }
  userModal?.show()
}

async function saveUser() {
  if (!formData.value.name) {
    showToast('warning', '警告', '请输入用户名称')
    return
  }
  if (!formData.value.expiry_date) {
    showToast('warning', '警告', '请选择到期日期')
    return
  }

  saving.value = true
  try {
    const data = {
      name: formData.value.name,
      uuid: formData.value.uuid || undefined,
      level: formData.value.level,
      expiry_date: formData.value.expiry_date,
      dns_resolve: formData.value.dns_resolve,
      enabled: formData.value.enabled ? 1 : 0
    }

    if (isEditing.value) {
      await updateUser(formData.value.id, data)
      showToast('success', '成功', '用户已更新')
    } else {
      await createUser(data)
      showToast('success', '成功', '用户已添加')
    }
    
    userModal?.hide()
    await loadData()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

function confirmDelete(user: User) {
  deleteTarget.value = user
  deleteModal?.show()
}

async function doDelete() {
  if (!deleteTarget.value) return
  
  deleting.value = true
  try {
    await deleteUser(deleteTarget.value.id)
    showToast('success', '成功', '用户已删除')
    deleteModal?.hide()
    await loadData()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '删除失败')
  } finally {
    deleting.value = false
  }
}
</script>

<style scoped>
/* 弹框副标题 */
.sub-modal-sub {
  font-size: 0.8125rem;
  color: var(--sb-text);
  font-weight: 550;
  margin-top: 0.15rem;
}

/* 节点标签：横向排布，可左右滚动不换行 */
.node-tabs {
  display: flex;
  gap: 0.35rem;
  margin-bottom: 0.85rem;
  padding-bottom: 0.6rem;
  border-bottom: 1px solid var(--sb-border);
  overflow-x: auto;
  scrollbar-width: none;
  -webkit-overflow-scrolling: touch;
}

.node-tabs::-webkit-scrollbar {
  display: none;
}

.node-tab {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  flex: 0 0 auto;
  padding: 0.35rem 0.75rem;
  font-size: 0.8125rem;
  font-weight: 550;
  color: var(--sb-text-muted);
  background: var(--sb-surface-2);
  border: 1px solid var(--sb-border);
  border-radius: var(--sb-radius-pill);
  cursor: pointer;
  white-space: nowrap;
  transition: background var(--sb-dur) var(--sb-ease),
    color var(--sb-dur) var(--sb-ease),
    border-color var(--sb-dur) var(--sb-ease);
}

.node-tab i {
  font-size: 0.85rem;
  opacity: 0.85;
}

.node-tab:hover {
  background: var(--sb-surface-3);
  color: var(--sb-text);
  border-color: var(--sb-border-strong);
}

.node-tab.active {
  background: var(--sb-accent);
  border-color: var(--sb-accent);
  color: #fff;
  font-weight: 650;
}

.node-tab.active i {
  opacity: 1;
}

@media (prefers-color-scheme: dark) {
  .node-tab.active {
    color: #0a0f1c;
  }
}

.node-tab:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--sb-accent-ring);
}

/* 链接项：整行可点，主标题 + 说明两行 */
.sub-links {
  display: grid;
  gap: 0.35rem;
}

.sub-link {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  width: 100%;
  padding: 0.5rem 0.7rem;
  text-align: left;
  background: var(--sb-surface-2);
  border: 1px solid var(--sb-border);
  border-radius: var(--sb-radius-sm);
  color: var(--sb-text);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: background var(--sb-dur) var(--sb-ease),
    border-color var(--sb-dur) var(--sb-ease),
    transform var(--sb-dur) var(--sb-ease);
}

.sub-link:hover {
  background: var(--sb-accent-soft);
  border-color: color-mix(in srgb, var(--sb-accent) 35%, var(--sb-border));
}

.sub-link:active {
  transform: scale(0.995);
}

.sub-link:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px var(--sb-accent-ring);
}

.sub-link-main {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  font-weight: 550;
  min-width: 0;
}

.sub-link-main i {
  color: var(--sb-accent);
  font-size: 0.95rem;
  flex: 0 0 auto;
}

.sub-link-hint {
  font-size: 0.72rem;
  color: var(--sb-text-subtle);
  text-align: right;
  flex: 0 0 auto;
}

@media (max-width: 420px) {
  .sub-link {
    flex-direction: column;
    align-items: flex-start;
    gap: 0.15rem;
  }

  .sub-link-hint {
    text-align: left;
  }
}

.sub-modal-tip {
  margin-right: auto;
  font-size: 0.78rem;
  color: var(--sb-text-subtle);
}
</style>

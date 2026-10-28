<template>
  <div class="container-fluid mt-4">
    <div class="d-flex justify-content-between mb-4">
      <h2>用户管理</h2>
      <button class="btn btn-primary" @click="openAddModal">
        <i class="bi bi-plus-lg"></i> 添加用户
      </button>
    </div>

    <div id="usersList" class="row">
      <!-- 按等级分组显示 -->
      <template v-for="level in [1, 2, 3]" :key="level">
        <template v-if="getUsersByLevel(level).length > 0">
          <div class="col-12 mb-3">
            <h5>等级 {{ level }}</h5>
          </div>
          <div 
            v-for="user in getUsersByLevel(level)" 
            :key="user.id" 
            class="col-xl-3 col-lg-4 col-md-6"
            :style="openDropdownId === user.id ? { position: 'relative', zIndex: 1000 } : {}"
          >
            <div class="card user-card">
              <div class="card-body">
                <div class="d-flex justify-content-between align-items-start mb-2">
                  <h6 class="card-title mb-0">{{ user.name }}</h6>
                  <span :class="['badge', isExpired(user) ? 'bg-danger' : 'bg-success']">
                    {{ isExpired(user) ? '已过期' : '有效' }}
                  </span>
                </div>
                <p class="small mb-2">
                  <span class="d-flex justify-content-between">
                    <span class="text-muted">到期时间:</span>
                    <span :class="isExpired(user) ? 'text-danger' : 'text-success'">
                      {{ user.expiry_date }}
                    </span>
                  </span>
                </p>
                <div class="btn-group btn-group-sm w-100 mb-2">
                  <button class="btn btn-outline-primary" @click="openEditModal(user)">
                    <i class="bi bi-pencil"></i> 编辑
                  </button>
                  <button class="btn btn-outline-danger" @click="confirmDelete(user)">
                    <i class="bi bi-trash"></i> 删除
                  </button>
                </div>
                <!-- 订阅链接下拉框 -->
                <div class="dropdown w-100" v-if="nodes.length > 0">
                  <button 
                    class="btn btn-sm btn-success dropdown-toggle w-100" 
                    type="button" 
                    @click.stop="toggleDropdown(user.id)"
                  >
                    <i class="bi bi-link-45deg"></i> 订阅链接
                  </button>
                  <ul 
                    class="dropdown-menu w-100 shadow sub-link-menu" 
                    :class="{ show: openDropdownId === user.id }"
                    :style="openDropdownId === user.id ? { display: 'block' } : {}"
                  >
                    <template v-for="node in nodes" :key="node.id">
                      <!-- 每个节点：展开成子菜单 -->
                      <li class="dropdown-submenu"
                          :class="{ 'is-expanded': openNodeMenu === user.id + '|' + node.tag }">
                        <a
                          class="dropdown-item small d-flex justify-content-between align-items-center"
                          :class="{ 'is-open': openNodeMenu === user.id + '|' + node.tag }"
                          href="#"
                          @click.prevent.stop="toggleNodeMenu(user.id, node.tag)"
                        >
                          <span><i class="bi bi-hdd-network"></i> {{ node.tag }}</span>
                          <i class="bi small"
                             :class="openNodeMenu === user.id + '|' + node.tag ? 'bi-chevron-down' : 'bi-chevron-right'"></i>
                        </a>
                        <ul
                          v-show="openNodeMenu === user.id + '|' + node.tag"
                          class="node-submenu"
                        >
                          <!-- 原始订阅：三格式自适应，带节点 -->
                          <li>
                            <a class="node-link" href="#"
                               @click.prevent="copySubLink(user, node.tag)">
                              <i class="bi bi-magic"></i>
                              <span>原始订阅 <span class="text-muted">（自适应）</span></span>
                            </a>
                          </li>
                          <!-- 客户端配置与 provider -->
                          <li v-for="item in clientFiles" :key="item.client">
                            <a class="node-link" href="#"
                               @click.prevent="copyClientFile(user, node.tag, item)">
                              <i :class="item.icon"></i>
                              <span>{{ item.label }}</span>
                            </a>
                          </li>
                        </ul>
                      </li>
                    </template>
                  </ul>
                </div>
              </div>
            </div>
          </div>
        </template>
      </template>
      
      <div v-if="users.length === 0" class="col-12 text-center py-5">
        <p class="text-muted">暂无用户</p>
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
import { ref, onMounted, onUnmounted, inject, computed } from 'vue'
import { Modal } from 'bootstrap'
import { getUsers, createUser, updateUser, deleteUser, getNodes, type User, type Node } from '@/api'

const showToast = inject<(type: 'success' | 'error' | 'warning' | 'info', title: string, message: string) => void>('showToast')!

const users = ref<User[]>([])
const nodes = ref<Node[]>([])
const loading = ref(false)
const openDropdownId = ref<number | null>(null)
// 展开中的节点子菜单，形如 "用户ID|节点tag"
const openNodeMenu = ref<string | null>(null)

// 模态框
const userModalEl = ref<HTMLElement | null>(null)
const deleteModalEl = ref<HTMLElement | null>(null)
let userModal: Modal | null = null
let deleteModal: Modal | null = null

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

// 点击外部关闭下拉框
const handleClickOutside = (e: MouseEvent) => {
  if (openDropdownId.value !== null) {
    const target = e.target as HTMLElement
    if (!target.closest('.dropdown')) {
      openDropdownId.value = null
      openNodeMenu.value = null
    }
  }
}

onMounted(async () => {
  await loadData()
  userModal = new Modal(userModalEl.value!)
  deleteModal = new Modal(deleteModalEl.value!)
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
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

function toggleDropdown(userId: number) {
  const next = openDropdownId.value === userId ? null : userId
  openDropdownId.value = next
  // 收起主菜单时同步收起子菜单
  if (next === null) openNodeMenu.value = null
}

function toggleNodeMenu(userId: number, nodeTag: string) {
  const key = userId + '|' + nodeTag
  const opening = openNodeMenu.value !== key
  openNodeMenu.value = opening ? key : null

  // 展开后把该节点滚进可视区：子菜单有 5 项，容器限高时
  // 靠后的节点展开后底部会被裁掉，用户以为点不到。
  if (opening) {
    nextTick(() => {
      const el = document.querySelector('.dropdown-submenu.is-expanded')
      if (el && typeof el.scrollIntoView === 'function') {
        el.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
      }
    })
  }
}

// 路径式客户端配置。client 段由后端映射到具体格式：
//   mihomo / sing-box -> provider 文件（只含节点）
//   shadowrocket / loon -> 分流配置（策略组 + 规则）
// 末段文件名只为客户端识别后缀，后端按 client 段决定输出。
const clientFiles = [
  { client: 'mihomo',      file: 'sboard.yaml', label: 'mihomo · provider',   icon: 'bi bi-filetype-yml' },
  { client: 'sing-box',    file: 'sboard.json', label: 'sing-box · provider', icon: 'bi bi-filetype-json' },
  { client: 'shadowrocket', file: 'sboard.conf', label: '小火箭 · 分流配置',    icon: 'bi bi-rocket' },
  { client: 'loon',        file: 'sboard.conf', label: 'Loon · 分流配置',      icon: 'bi bi-phone' }
]

function copyClientFile(user: User, nodeTag: string, item: { client: string; file: string; label: string }) {
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
.user-card {
  background: white;
  border: none;
  border-radius: 16px;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.08);
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  /* overflow: hidden; 移除这个，否则下拉菜单会被裁剪 */
  animation: fadeInUp 0.5s ease-out;
  margin-bottom: 1rem;
}

.user-card::before {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  width: 100%;
  height: 4px;
  background: linear-gradient(90deg, #667eea, #764ba2);
  transform: scaleX(0);
  transition: transform 0.3s ease;
}

.user-card:hover {
  transform: translateY(-5px);
  box-shadow: 0 12px 30px rgba(0, 0, 0, 0.12);
}

.user-card:hover::before {
  transform: scaleX(1);
}

@keyframes fadeInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.dropdown-menu.show {
  position: absolute;
  inset: 0px auto auto 0px;
  margin: 0px;
  transform: translate(0px, 38px);
  z-index: 1050;
}
/* 节点项：块级元素，子菜单展开时把后续节点往下挤开 */
.dropdown-submenu {
  display: block;
}
/* 子菜单：完全自绘，不用 Bootstrap 的 .dropdown-menu。
   关键点——.dropdown-menu 自带 position:absolute，会把子菜单浮到父菜单
   之上，视觉上「盖住」后面的节点（vless 被顶进 trojan 的展开板）。
   这里用普通块级流，靠 max-height 过渡展开。 */
.node-submenu {
  list-style: none;
  margin: 0;
  padding: 0 0 0 0.9rem;
  border-left: 2px solid rgba(25, 135, 84, 0.55);
  background: rgba(255, 255, 255, 0.04);
  overflow: hidden;
}
.node-submenu > li {
  display: block;
}
.node-submenu .node-link {
  display: flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.34rem 0.5rem;
  font-size: 0.8rem;
  color: var(--bs-body-color);
  text-decoration: none;
  border-radius: 0.25rem;
  white-space: normal;
  word-break: break-word;
  line-height: 1.25;
}
.node-submenu .node-link:hover,
.node-submenu .node-link:active {
  background: rgba(25, 135, 84, 0.18);
  color: var(--bs-body-color);
}
.node-submenu .node-link i {
  flex: 0 0 auto;
  opacity: 0.85;
}
</style>

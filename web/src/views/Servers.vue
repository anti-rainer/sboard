<template>
  <div class="container-fluid mt-4">
    <div class="sb-page-head">
      <div>
        <h4><i class="bi bi-hdd-stack"></i> 服务器</h4>
        <div class="sb-subtitle">共 {{ servers.length }} 台 · 拖动卡片左上角手柄可调整顺序</div>
      </div>
      <div class="sb-toolbar">
        <button class="btn btn-sm btn-secondary" @click="panelUpdate" :disabled="updatingPanel"
                title="更新面板自身：下载新版本二进制并重启服务">
          <span v-if="updatingPanel" class="spinner-border spinner-border-sm"></span>
          <i v-else class="bi bi-arrow-up-circle"></i> 面板更新
        </button>
        <button class="btn btn-sm btn-secondary" @click="updateAllAgents" :disabled="updatingAgents">
          <span v-if="updatingAgents" class="spinner-border spinner-border-sm"></span>
          <i v-else class="bi bi-cloud-download"></i> Agent 更新
        </button>
        <button class="btn btn-sm btn-secondary" @click="deployAll" :disabled="deploying">
          <span v-if="deploying" class="spinner-border spinner-border-sm"></span>
          <i v-else class="bi bi-arrow-clockwise"></i> 部署
        </button>
        <button class="btn btn-sm btn-primary" @click="openAddModal">
          <i class="bi bi-plus-lg"></i> 添加
        </button>
      </div>
    </div>

    <div id="serversList" class="row g-3">
      <div 
        v-for="(server, index) in servers" 
        :key="server.id" 
        class="col-xl-3 col-lg-4 col-md-6"
        :draggable="isDragging"
        @dragstart="handleDragStart($event, index)"
        @dragover.prevent="handleDragOver($event, index)"
        @dragend="handleDragEnd"
        @drop="handleDrop($event, index)"
      >
        <div class="server-card" :class="{ 'dragging': dragIndex === index, 'drag-over': dragOverIndex === index }">
          <div class="server-header">
            <div class="server-title-row">
              <div class="server-title-wrap">
                <span
                  class="drag-handle"
                  @mousedown="startDrag"
                  @mouseup="endDrag"
                  @mouseleave="endDrag"
                  @touchstart="startDrag"
                  @touchend="endDrag"
                  title="拖动排序"
                >
                  <i class="bi bi-grip-vertical"></i>
                </span>
                <div class="server-title">{{ server.name }}</div>
              </div>
              <div class="server-badges">
                <span :class="['badge', server.enabled ? 'bg-success' : 'bg-secondary']">
                  {{ server.enabled ? '启用' : '禁用' }}
                </span>
                <!-- Agent 在线状态 -->
                <span :class="['badge', server.agent_online ? 'bg-primary' : 'bg-warning']"
                      :title="server.agent_online ? `在线 - ${server.agent_id}` : '等待连接'">
                  <i :class="['bi', server.agent_online ? 'bi-wifi' : 'bi-wifi-off']"></i>
                  {{ server.agent_online ? '在线' : '未连接' }}
                </span>
              </div>
            </div>
            <!-- IPv4 行：右侧顶格显示 Agent 版本 -->
            <div class="server-host server-host-row">
              <span class="server-host-addr">
                <i class="bi bi-hdd"></i> {{ maskIP(server.host) || '等待 Agent 上报' }}
              </span>
              <!-- 版本落后时的升级按钮：橙色圆圈 + 向上箭头，不占文字宽度。
                   点击只给这一台下发 Agent 自更新指令。 -->
              <button v-if="hashMismatch(server)" class="sb-upgrade-dot"
                      :disabled="upgradingAgent === server.id"
                      :title="mismatchTip(server)"
                      @click.stop="upgradeAgent(server)">
                <span v-if="upgradingAgent === server.id" class="spinner-border spinner-border-sm"></span>
                <i v-else class="bi bi-arrow-up"></i>
              </button>
              <span v-if="server.agent_version" class="sb-agent-ver"
                    :title="`架构: ${server.agent_arch || '-'}\n构建标识: ${server.agent_build_id || '-'}\nsha256: ${server.agent_hash || '-'}\n构建时间: ${server.agent_built_at || '-'}`">
                {{ server.agent_version }}
              </span>
            </div>
            <!-- 第二行 IP（有 IPv6 时显示），不再承载版本信息 -->
            <div v-if="server.host_ipv6" class="server-host server-host-row">
              <span class="server-host-addr">
                <i class="bi bi-hdd-stack"></i> {{ maskIP(server.host_ipv6) }}
              </span>
            </div>
          </div>
          
          <!-- 实时监控数据 -->
          <div v-if="server.agent_online" class="monitor-panel mb-2">
            <!-- CPU 和内存 -->
            <div class="row g-1 mb-1">
              <div class="col-6">
                <div class="monitor-item">
                  <div class="d-flex justify-content-between align-items-center">
                    <span class="monitor-label">CPU</span>
                    <span class="monitor-value text-primary">{{ (server.cpu_usage || 0).toFixed(1) }}%</span>
                  </div>
                  <div class="progress" style="height: 3px;">
                    <div class="progress-bar bg-primary" :style="{ width: (server.cpu_usage || 0) + '%' }"></div>
                  </div>
                </div>
              </div>
              <div class="col-6">
                <div class="monitor-item">
                  <div class="d-flex justify-content-between align-items-center">
                    <span class="monitor-label">内存</span>
                    <span class="monitor-value text-success">{{ (server.mem_usage || 0).toFixed(1) }}%</span>
                  </div>
                  <div class="progress" style="height: 3px;">
                    <div class="progress-bar bg-success" :style="{ width: (server.mem_usage || 0) + '%' }"></div>
                  </div>
                </div>
              </div>
            </div>
            <!-- 网速 -->
            <div class="row g-1">
              <div class="col-6">
                <div class="speed-item">
                  <i class="bi bi-arrow-down text-info"></i>
                  <span class="speed-value">{{ formatSpeed(server.net_in || 0) }}</span>
                  <span class="traffic-value">{{ formatBytes(server.monthly_out || 0) }}</span>
                </div>
              </div>
              <div class="col-6">
                <div class="speed-item">
                  <i class="bi bi-arrow-up text-warning"></i>
                  <span class="speed-value">{{ formatSpeed(server.net_out || 0) }}</span>
                  <span class="traffic-value">{{ formatBytes(server.monthly_in || 0) }}</span>
                </div>
              </div>
            </div>
          </div>
          
          <!-- 离线时显示基本信息 -->
          <div v-else class="offline-info mb-2">
            <span class="meta-key">分类</span>
            <span :class="getCategoryClass(server.category)">{{ getCategoryName(server.category) }}</span>
          </div>
          
          <div class="card-actions">
            <button class="sb-icon-btn" @click="openEditModal(server)" title="编辑">
              <i class="bi bi-pencil"></i>
            </button>
            <button class="sb-icon-btn" @click="deployFolder(server)" :disabled="server.deployingFolder" title="部署">
              <span v-if="server.deployingFolder" class="spinner-border spinner-border-sm"></span>
              <i v-else class="bi bi-cloud-upload"></i>
            </button>
            <button class="sb-icon-btn sb-icon-btn-danger" @click="confirmDelete(server)" title="删除">
              <i class="bi bi-trash"></i>
            </button>
          </div>
        </div>
      </div>
      
      <div v-if="servers.length === 0" class="col-12 text-center py-5">
        <p class="text-muted">暂无服务器</p>
      </div>
    </div>

    <!-- 添加/编辑服务器模态框 -->
    <div class="modal fade" id="serverModal" tabindex="-1" ref="serverModalEl">
      <div class="modal-dialog modal-dialog-centered modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">{{ isEditing ? '编辑服务器' : '添加服务器' }}</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="saveServer">
              <!-- Agent 状态信息 (编辑时显示，放在最上面) -->
              <div class="card bg-light mb-3" v-if="isEditing">
                <div class="card-body py-2">
                  <div class="row small">
                    <div class="col-6">
                      <span class="text-muted">Agent 状态:</span>
                      <span :class="formData.agent_online ? 'text-success' : 'text-danger'" class="ms-2">
                        <i :class="formData.agent_online ? 'bi-wifi' : 'bi-wifi-off'"></i>
                        {{ formData.agent_online ? '在线' : '离线' }}
                      </span>
                    </div>
                    <div class="col-6" v-if="formData.agent_id">
                      <span class="text-muted">Agent ID:</span>
                      <span class="ms-2 font-monospace">{{ formData.agent_id }}</span>
                    </div>
                  </div>
                  <div class="small mt-1">
                    <div v-if="formData.host" class="mb-1">
                      <span class="text-muted">IPv4:</span>
                      <span class="ms-2">{{ formData.host }}</span>
                    </div>
                    <div v-if="formData.host_ipv6">
                      <span class="text-muted">IPv6:</span>
                      <span class="ms-2">{{ formData.host_ipv6 }}</span>
                    </div>
                  </div>
                  <div class="mt-2" v-if="formData.agent_token">
                    <div class="d-flex align-items-center mb-1">
                      <span class="text-muted small">部署命令:</span>
                      <span class="ms-2 text-muted small">(点击复制)</span>
                    </div>
                    <div 
                      class="deploy-command-box"
                      @click="copyDeployCommand"
                      role="button"
                      tabindex="0"
                      title="点击复制部署命令"
                    >
                      <code class="deploy-command-text">{{ agentDeployCommand }}</code>
                      <i class="bi bi-clipboard copy-icon"></i>
                    </div>
                  </div>
                </div>
              </div>

              <div class="row">
                <div class="col-md-4">
                  <div class="mb-3">
                    <label class="form-label">备注名称 <span class="text-danger">*</span></label>
                    <input type="text" class="form-control" v-model="formData.name" required placeholder="如: 香港HKT-01">
                  </div>
                </div>
                <div class="col-md-5">
                  <div class="mb-3">
                    <label class="form-label">节点域名 <small class="text-muted">(可选)</small></label>
                    <input type="text" class="form-control" v-model="formData.node_domain" placeholder="如: hk01.example.com">
                  </div>
                </div>
                <div class="col-md-3">
                  <div class="mb-3">
                    <label class="form-label">订阅地址来源</label>
                    <select class="form-select" v-model="formData.dns_resolve">
                      <option value="none">节点域名</option>
                      <option value="ipv4">节点 IPv4</option>
                      <option value="ipv6">节点 IPv6</option>
                    </select>
                  </div>
                </div>
              </div>
              <div class="row">
                <div class="col-md-4">
                  <div class="mb-3">
                    <label class="form-label">服务器分类 <span class="text-danger">*</span></label>
                    <select class="form-select" v-model="formData.category">
                      <option value="direct">线路</option>
                      <option value="relay">落地</option>
                      <option value="home">自用</option>
                    </select>
                  </div>
                </div>
                <div class="col-md-4">
                  <div class="mb-3">
                    <label class="form-label">节点名一</label>
                    <input type="text" class="form-control" v-model="formData.node_1" placeholder="节点标识">
                  </div>
                </div>
              </div>
              <div class="row">
                <div class="col-md-4">
                  <div class="mb-3">
                    <label class="form-label">节点名二</label>
                    <input type="text" class="form-control" v-model="formData.node_2" placeholder="节点标识">
                  </div>
                </div>
                <div class="col-md-4">
                  <div class="mb-3">
                    <label class="form-label">节点名三</label>
                    <input type="text" class="form-control" v-model="formData.node_3" placeholder="节点标识">
                  </div>
                </div>
                <div class="col-md-4">
                  <div class="mb-3">
                    <label class="form-label">状态</label>
                    <div class="form-check form-switch mt-2">
                      <input class="form-check-input" type="checkbox" v-model="formData.enabled" id="serverEnabled">
                      <label class="form-check-label" for="serverEnabled">{{ formData.enabled ? '启用' : '禁用' }}</label>
                    </div>
                  </div>
                </div>
              </div>
              
              <!-- 节点配置区域（仅编辑时显示） -->
              <div v-if="isEditing" class="mt-4 border-top pt-3">
                <!-- Tab 导航 -->
                <ul class="nav nav-tabs mb-3" role="tablist">
                  <li class="nav-item" role="presentation">
                    <button 
                      class="nav-link" 
                      :class="{ active: serverConfigTab === 'forward' }"
                      @click="serverConfigTab = 'forward'"
                      type="button"
                    >
                      <i class="bi bi-arrow-left-right"></i> 端口转发
                    </button>
                  </li>
                  <li class="nav-item" role="presentation">
                    <button 
                      class="nav-link" 
                      :class="{ active: serverConfigTab === 'outbound' }"
                      @click="serverConfigTab = 'outbound'"
                      type="button"
                    >
                      <i class="bi bi-box-arrow-right"></i> 落地出站
                      <span v-if="serverOutbounds.length > 0" class="badge bg-primary ms-1">{{ serverOutbounds.length }}</span>
                    </button>
                  </li>
                </ul>

                <!-- 端口转发 Tab 内容 -->
                <div v-if="serverConfigTab === 'forward'">
                  <p class="text-muted small mb-3">配置入站节点的端口转发（用于订阅地址替换）</p>
                  <div class="d-flex flex-wrap gap-2">
                    <button 
                      v-for="node in availableNodes" 
                      :key="node.id" 
                      type="button"
                      class="btn btn-sm position-relative"
                      :class="isForwardEnabled(node) ? 'btn-success' : 'btn-outline-secondary'"
                      @click="openNodeConfigModal(node)"
                    >
                      {{ node.tag }}
                      <span 
                        v-if="isForwardEnabled(node)" 
                        class="position-absolute translate-middle badge rounded-circle bg-success p-1"
                        style="top: 4px; right: -2px; width: 8px; height: 8px;"
                      ></span>
                    </button>
                  </div>
                </div>

                <!-- 落地出站 Tab 内容 -->
                <div v-if="serverConfigTab === 'outbound'">
                  <div class="d-flex justify-content-between align-items-center mb-3">
                    <p class="text-muted small mb-0">配置落地服务器出站（最多10个，订阅时自动为用户生成对应认证 UUID）</p>
                    <button 
                      class="btn btn-sm btn-primary" 
                      @click="openAddOutboundModal"
                      :disabled="serverOutbounds.length >= 10"
                    >
                      <i class="bi bi-plus-lg"></i> 添加出站
                    </button>
                  </div>
                  <div v-if="serverOutbounds.length === 0" class="text-center py-4 text-muted">
                    <i class="bi bi-box-arrow-right" style="font-size: 2rem;"></i>
                    <p class="mt-2 mb-0">暂无落地出站配置</p>
                  </div>
                  <div v-else class="d-flex flex-wrap gap-2">
                    <div 
                      v-for="outbound in serverOutbounds" 
                      :key="outbound.slot" 
                      class="outbound-card"
                      :class="{ 'outbound-disabled': !outbound.enabled }"
                    >
                      <div class="outbound-header">
                        <span class="outbound-slot">{{ outbound.slot }}</span>
                        <span class="outbound-name">{{ outbound.remark || '未命名' }}</span>
                        <div class="form-check form-switch form-switch-sm mb-0">
                          <input 
                            class="form-check-input" 
                            type="checkbox" 
                            :checked="outbound.enabled"
                            @change="toggleOutbound(outbound)"
                          >
                        </div>
                      </div>
                      <div class="outbound-info">
                        <small class="text-muted">{{ outbound.protocol }} | {{ outbound.host }}:{{ outbound.port }}</small>
                      </div>
                      <div class="outbound-actions">
                        <button class="btn btn-sm btn-outline-primary" @click="openEditOutboundModal(outbound)" title="编辑">
                          <i class="bi bi-pencil"></i>
                        </button>
                        <button class="btn btn-sm btn-outline-danger" @click="deleteOutbound(outbound)" title="删除">
                          <i class="bi bi-trash"></i>
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
            <button type="button" class="btn btn-primary" @click="saveServer" :disabled="saving">
              <span v-if="saving" class="spinner-border spinner-border-sm me-1"></span>
              保存
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 部署结果模态框 -->
    <div class="modal fade" id="deployModal" tabindex="-1" ref="deployModalEl">
      <div class="modal-dialog modal-dialog-scrollable modal-lg">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">部署日志</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <pre id="deployOutput" style="white-space: pre-wrap; word-break: break-word; background: #f8f9fa; padding: 1rem; border-radius: 5px; max-height: 500px; overflow-y: auto;">{{ deployOutput }}</pre>
          </div>
          <div class="modal-footer">
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
              确定要删除服务器 <strong class="text-danger">"{{ deleteTarget?.name }}"</strong> 吗？
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

    <!-- 端口转发配置模态框 -->
    <div class="modal fade" id="nodeConfigModal" tabindex="-1" ref="nodeConfigModalEl">
      <div class="modal-dialog modal-dialog-centered">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title"><i class="bi bi-arrow-left-right"></i> 端口转发配置 - {{ selectedNode?.tag }}</h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="saveNodeConfig">
              <div class="mb-3">
                <div class="form-check form-switch">
                  <input class="form-check-input" type="checkbox" v-model="nodeConfigData.forward_enabled" id="forwardEnabled">
                  <label class="form-check-label" for="forwardEnabled">启用端口转发</label>
                </div>
              </div>
              <div v-if="nodeConfigData.forward_enabled">
                <small class="text-muted d-block mb-3">用于订阅地址替换（将节点地址替换为转发地址）</small>
                <div class="row">
                  <div class="col-8">
                    <div class="mb-3">
                      <label class="form-label">转发地址 (IP/域名)</label>
                      <input type="text" class="form-control" v-model="nodeConfigData.forward_host" placeholder="例: 1.2.3.4 或 forward.example.com">
                    </div>
                  </div>
                  <div class="col-4">
                    <div class="mb-3">
                      <label class="form-label">转发端口</label>
                      <input type="number" class="form-control" v-model.number="nodeConfigData.forward_port" min="1" max="65535" placeholder="端口">
                    </div>
                  </div>
                </div>
              </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
            <button type="button" class="btn btn-primary" @click="saveNodeConfig">保存</button>
          </div>
        </div>
      </div>
    </div>

    <!-- 落地出站配置模态框 -->
    <div class="modal fade" id="outboundModal" tabindex="-1" ref="outboundModalEl">
      <div class="modal-dialog modal-dialog-centered">
        <div class="modal-content">
          <div class="modal-header">
            <h5 class="modal-title">
              <i class="bi bi-box-arrow-right"></i> 
              {{ isEditingOutbound ? '编辑落地出站' : '添加落地出站' }}
            </h5>
            <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
          </div>
          <div class="modal-body">
            <form @submit.prevent="saveOutbound">
              <!-- 备注 -->
              <div class="mb-3">
                <label class="form-label">备注 <span class="text-danger">*</span></label>
                <input type="text" class="form-control" v-model="outboundData.remark" placeholder="显示为按钮名称，如：美国落地">
              </div>

              <!-- 快速识别：粘贴节点链接自动填充表单 -->
              <div class="mb-3">
                <label class="form-label">快速识别</label>
                <div class="input-group">
                  <input
                    type="text"
                    class="form-control"
                    v-model="outboundImportLink"
                    placeholder="粘贴节点链接：vless:// vmess:// trojan:// ss:// hysteria2:// tuic:// anytls:// socks5://"
                    @keyup.enter="parseOutboundImportLink"
                  >
                  <button class="btn btn-outline-primary" type="button" @click="parseOutboundImportLink">识别</button>
                </div>
                <div class="form-text">
                  支持标准链接与 base64 包裹变体（含 Reality 参数 pbk/sid/fp、ws 路径、ALPN）
                </div>
              </div>

              <div class="row">
                <div class="col-4 mb-3">
                  <label class="form-label">协议</label>
                  <select class="form-select" v-model="outboundData.protocol" @change="onOutboundProtocolChange">
                    <option v-for="p in MAIN_PROTOCOLS" :key="p.id" :value="p.id">{{ p.label }}</option>
                  </select>
                </div>
                <div class="col-5 mb-3">
                  <label class="form-label">出站地址</label>
                  <input type="text" class="form-control" v-model="outboundData.host" placeholder="落地服务器 IP/域名">
                </div>
                <div class="col-3 mb-3">
                  <label class="form-label">端口</label>
                  <input type="number" class="form-control" v-model.number="outboundData.port" min="1" max="65535">
                </div>
                <div class="col-2 mb-3">
                  <label class="form-label">
                    出口国家
                    <i class="bi bi-question-circle text-muted" title="落地节点真实所在国家（两位代码，如 CZ）。出站地址是入口，实际出口可能在别国，留空则按入口 IP 推断。"></i>
                  </label>
                  <input type="text" class="form-control" v-model="outboundData.country"
                         placeholder="CZ" maxlength="2" style="text-transform: uppercase">
                </div>
              </div>

              <!-- 协议分支：胶囊选择 -->
              <div class="mb-3" v-if="outboundBranches.length">
                <label class="form-label">协议分支 <span class="text-muted small">（先选主协议，再选具体分支）</span></label>
                <div class="branch-pills">
                  <button type="button" v-for="b in outboundBranches" :key="b.id"
                          class="branch-pill"
                          :class="{ active: activeOutboundBranch === b.id }"
                          :title="b.hint || b.label"
                          @click="applyOutboundBranch(b)">
                    {{ b.label }}
                  </button>
                </div>
                <small class="text-muted d-block mt-1" v-if="activeOutboundBranchHint">
                  <i class="bi bi-info-circle"></i> {{ activeOutboundBranchHint }}
                </small>
              </div>

              <!-- Shadowsocks 配置 -->
              <div class="row" v-if="outboundData.protocol === 'shadowsocks'">
                <div class="col-6 mb-3">
                  <label class="form-label">加密方式</label>
                  <select class="form-select" v-model="outboundData.method">
                    <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm（32 字节）</option>
                    <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm（16 字节）</option>
                    <option value="2022-blake3-chacha20-poly1305">2022-blake3-chacha20-poly1305（32 字节）</option>
                  </select>
                </div>
                <div class="col-6 mb-3">
                  <label class="form-label">密码</label>
                  <input type="text" class="form-control" v-model="outboundData.password" placeholder="密码">
                </div>
              </div>

              <!-- Trojan/AnyTLS 配置 -->
              <div class="row" v-if="outboundData.protocol === 'trojan' || outboundData.protocol === 'anytls' || outboundData.protocol === 'naive'">
                <div class="col-6 mb-3">
                  <label class="form-label">密码</label>
                  <input type="text" class="form-control" v-model="outboundData.password" placeholder="密码">
                </div>
                <div class="col-6 mb-3">
                  <label class="form-label">SNI (可选)</label>
                  <input type="text" class="form-control" v-model="outboundData.sni" placeholder="TLS SNI">
                </div>
              </div>

              <!-- SOCKS 配置 -->
              <div v-if="outboundData.protocol === 'socks' || outboundData.protocol === 'socks5'">
                <div class="row">
                  <div class="col-6 mb-3">
                    <label class="form-label">用户名 (可选)</label>
                    <input type="text" class="form-control" v-model="outboundData.username" placeholder="用户名">
                  </div>
                  <div class="col-6 mb-3">
                    <label class="form-label">密码 (可选)</label>
                    <input type="text" class="form-control" v-model="outboundData.password" placeholder="密码">
                  </div>
                </div>
                <div class="alert alert-light border py-2 px-3 small mb-3">
                  <i class="bi bi-info-circle"></i> Socks5 出站仅支持「用户名 + 密码」认证，不支持 TLS、Reality 与传输层；两者都留空即为免认证。
                </div>
              </div>

              <!-- VLESS 配置 -->
              <div v-if="outboundData.protocol === 'vless'">
                <div class="row">
                  <div class="col-8 mb-3">
                    <label class="form-label">UUID</label>
                    <input type="text" class="form-control" v-model="outboundData.uuid" placeholder="VLESS UUID">
                  </div>
                  <div class="col-4 mb-3">
                    <label class="form-label">Flow (可选)</label>
                    <select class="form-select" v-model="outboundData.flow">
                      <option value="">无</option>
                      <option value="xtls-rprx-vision">xtls-rprx-vision</option>
                    </select>
                  </div>
                </div>
                <div class="row">
                  <div class="col-4 mb-3" v-if="outboundSupportsTls">
                    <label class="form-label">安全类型</label>
                    <select class="form-select" v-model="outboundVlessTlsType">
                      <option value="none">无</option>
                      <option value="tls">TLS</option>
                      <option value="reality">Reality</option>
                    </select>
                  </div>
                  <div class="col-4 mb-3" v-if="outboundData.tls || outboundData.reality">
                    <label class="form-label">SNI</label>
                    <input type="text" class="form-control" v-model="outboundData.sni" placeholder="TLS SNI">
                  </div>
                  <div class="col-4 mb-3" v-if="outboundData.tls || outboundData.reality">
                    <label class="form-label">Fingerprint</label>
                    <select class="form-select" v-model="outboundData.fp">
                      <option value="chrome">chrome</option>
                      <option value="firefox">firefox</option>
                      <option value="safari">safari</option>
                      <option value="edge">edge</option>
                      <option value="random">random</option>
                    </select>
                  </div>
                </div>
                <div class="row" v-if="outboundData.reality">
                  <div class="col-8 mb-3">
                    <label class="form-label">Public Key</label>
                    <input type="text" class="form-control" v-model="outboundData.pub_key" placeholder="Reality Public Key">
                  </div>
                  <div class="col-4 mb-3">
                    <label class="form-label">Short ID</label>
                    <input type="text" class="form-control" v-model="outboundData.short_id" placeholder="Short ID">
                  </div>
                </div>
              </div>

              <!-- VMess 配置 -->
              <div v-if="outboundData.protocol === 'vmess'">
                <div class="row">
                  <div class="col-6 mb-3">
                    <label class="form-label">UUID</label>
                    <input type="text" class="form-control" v-model="outboundData.uuid" placeholder="VMess UUID">
                  </div>
                  <div class="col-3 mb-3">
                    <label class="form-label">加密方式</label>
                    <select class="form-select" v-model="outboundData.security">
                      <option value="auto">auto</option>
                      <option value="aes-128-gcm">aes-128-gcm</option>
                      <option value="chacha20-poly1305">chacha20-poly1305</option>
                      <option value="none">none</option>
                      <option value="zero">zero</option>
                    </select>
                  </div>
                  <div class="col-3 mb-3">
                    <label class="form-label">Alter ID</label>
                    <input type="number" class="form-control" v-model.number="outboundData.alter_id" min="0" placeholder="0">
                  </div>
                </div>
                <div class="row" v-if="outboundSupportsTls">
                  <div class="col-4 mb-3">
                    <div class="form-check form-switch mt-4">
                      <input type="checkbox" class="form-check-input" v-model="outboundData.tls" id="vmessTls">
                      <label class="form-check-label" for="vmessTls">启用 TLS</label>
                    </div>
                  </div>
                  <div class="col-8 mb-3" v-if="outboundData.tls">
                    <label class="form-label">SNI (可选)</label>
                    <input type="text" class="form-control" v-model="outboundData.sni" placeholder="TLS SNI">
                  </div>
                </div>
              </div>


              <!-- 传输层与 ALPN：VLESS / VMess / Trojan / AnyTLS 共用 -->
              <div class="row" v-if="outboundSupportsTransport">
                <div class="col-4 mb-3">
                  <label class="form-label">传输层</label>
                  <select class="form-select" v-model="outboundData.network">
                    <option value="">tcp (默认)</option>
                    <option value="ws">WebSocket</option>
                    <option value="grpc">gRPC</option>
                    <option value="h2">HTTP/2 (h2)</option>
                    <option value="httpupgrade">HTTPUpgrade</option>
                  </select>
                </div>
                <div class="col-4 mb-3" v-if="outboundData.network === 'ws' || outboundData.network === 'httpupgrade'">
                  <label class="form-label">WS Path</label>
                  <input type="text" class="form-control" v-model="outboundData.ws_path" placeholder="/">
                </div>
                <div class="col-4 mb-3" v-if="outboundData.network === 'grpc'">
                  <label class="form-label">gRPC 服务名</label>
                  <input type="text" class="form-control" v-model="outboundData.ws_path" placeholder="服务名，留空用默认">
                </div>
                <div class="col-4 mb-3" v-if="outboundData.network === 'ws' || outboundData.network === 'httpupgrade'">
                  <label class="form-label">WS Host</label>
                  <input type="text" class="form-control" v-model="outboundData.ws_host" placeholder="可选">
                </div>
                <div class="col-4 mb-3 d-flex align-items-end">
                  <div class="form-check form-switch mb-2">
                    <input class="form-check-input" type="checkbox" id="outboundSkipCert"
                           v-model="outboundData.skip_cert_verify">
                    <label class="form-check-label" for="outboundSkipCert">
                      跳过证书校验
                      <i class="bi bi-question-circle text-muted" title="对方是自签或伪装证书时需要开启；正规证书可关闭以提高安全性。"></i>
                    </label>
                  </div>
                </div>
                <div class="col-4 mb-3">
                  <label class="form-label">
                    ALPN
                    <i class="bi bi-question-circle text-muted" title="TLS ALPN，逗号分隔。ws over TLS 常见 http/1.1；QUIC 系（TUIC/Hysteria2）固定 h3。留空由服务端协商。"></i>
                  </label>
                  <input type="text" class="form-control" v-model="outboundData.alpn" placeholder="http/1.1 或 h3,h2">
                </div>
              </div>

              <!-- Hysteria2 配置 -->
              <div v-if="outboundData.protocol === 'hysteria2'">
                <div class="row">
                  <div class="col-6 mb-3">
                    <label class="form-label">密码</label>
                    <input type="text" class="form-control" v-model="outboundData.password" placeholder="Hysteria2 密码">
                  </div>
                  <div class="col-6 mb-3">
                    <label class="form-label">SNI (可选)</label>
                    <input type="text" class="form-control" v-model="outboundData.sni" placeholder="TLS SNI">
                  </div>
                </div>
                <div class="row">
                  <div class="col-4 mb-3">
                    <label class="form-label">Obfs 类型 (可选)</label>
                    <select class="form-select" v-model="outboundData.obfs">
                      <option value="">无</option>
                      <option value="salamander">salamander</option>
                    </select>
                  </div>
                  <div class="col-8 mb-3" v-if="outboundData.obfs">
                    <label class="form-label">Obfs 密码</label>
                    <input type="text" class="form-control" v-model="outboundData.obfs_pwd" placeholder="Obfs 密码">
                  </div>
                </div>
              </div>

              <!-- TUIC 配置 -->
              <div v-if="outboundData.protocol === 'tuic'">
                <div class="row">
                  <div class="col-6 mb-3">
                    <label class="form-label">UUID</label>
                    <input type="text" class="form-control" v-model="outboundData.uuid" placeholder="TUIC UUID">
                  </div>
                  <div class="col-6 mb-3">
                    <label class="form-label">密码</label>
                    <input type="text" class="form-control" v-model="outboundData.password" placeholder="TUIC 密码">
                  </div>
                </div>
                <div class="row">
                  <div class="col-4 mb-3">
                    <label class="form-label">拥塞控制</label>
                    <select class="form-select" v-model="outboundData.tuic_congestion">
                      <option value="bbr">bbr</option>
                      <option value="cubic">cubic</option>
                      <option value="new_reno">new_reno</option>
                    </select>
                  </div>
                  <div class="col-4 mb-3">
                    <label class="form-label">UDP 转发模式</label>
                    <select class="form-select" v-model="outboundData.tuic_udp_relay_mode">
                      <option value="native">native</option>
                      <option value="quic">quic</option>
                    </select>
                  </div>
                  <div class="col-4 mb-3">
                    <label class="form-label">SNI (可选)</label>
                    <input type="text" class="form-control" v-model="outboundData.sni" placeholder="TLS SNI">
                  </div>
                </div>
                <div class="alert alert-light border py-2 px-3 small mb-3">
                  <i class="bi bi-lock"></i> TUIC 基于 QUIC，TLS 强制开启，不支持 Reality 与传输层。
                </div>
              </div>
            </form>
          </div>
          <div class="modal-footer">
            <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">取消</button>
            <button type="button" class="btn btn-primary" @click="saveOutbound" :disabled="savingOutbound">
              <span v-if="savingOutbound" class="spinner-border spinner-border-sm me-1"></span>
              保存
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
import { getServers, createServer, updateServer, deleteServer, getNodes, deployServer, deployAll as apiDeployAll, updateAllAgents as apiUpdateAllAgents, updateServerAgent, panelSelfUpdate, getPanelUpdateStatus, regenerateAgentToken, getServersStatus, reorderServers, getNodeConfigs, saveNodeConfig as saveNodeConfigApi, getServerOutbounds, createServerOutbound, updateServerOutbound, deleteServerOutbound, toggleServerOutbound, type Server, type Node, type ServerStatus, type NodeConfig, type ServerOutbound } from '@/api'
import { parseProxyLink, type ParsedProxy } from '../utils/proxyLink'
import { MAIN_PROTOCOLS, getBranches, detectBranch, type ProtocolBranch } from '@/utils/protocolBranches'

const showToast = inject<(type: 'success' | 'error' | 'warning' | 'info', title: string, message: string) => void>('showToast')!

interface ServerWithState extends Server {
  deploying?: boolean
  deployingFolder?: boolean
}

const servers = ref<ServerWithState[]>([])
const availableNodes = ref<Node[]>([])
const loading = ref(false)

// 模态框
const serverModalEl = ref<HTMLElement | null>(null)
const deployModalEl = ref<HTMLElement | null>(null)
const deleteModalEl = ref<HTMLElement | null>(null)
const nodeConfigModalEl = ref<HTMLElement | null>(null)
const outboundModalEl = ref<HTMLElement | null>(null)
let serverModal: Modal | null = null
let deployModal: Modal | null = null
let deleteModal: Modal | null = null
let nodeConfigModal: Modal | null = null
let outboundModal: Modal | null = null

// 表单
const isEditing = ref(false)
const saving = ref(false)
const formData = ref(getDefaultFormData())

// 删除
const deleteTarget = ref<Server | null>(null)
const deleting = ref(false)

// 部署
const deploying = ref(false)
const deployOutput = ref('')
const updatingAgents = ref(false)

// 拖拽排序相关
const isDragging = ref(false)
const dragIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)
let dragTimer: number | null = null

function startDrag() {
  // 长按 200ms 后启用拖拽
  dragTimer = window.setTimeout(() => {
    isDragging.value = true
  }, 200)
}

function endDrag() {
  if (dragTimer) {
    clearTimeout(dragTimer)
    dragTimer = null
  }
}

function handleDragStart(e: DragEvent, index: number) {
  if (!isDragging.value) {
    e.preventDefault()
    return
  }
  dragIndex.value = index
  e.dataTransfer!.effectAllowed = 'move'
}

function handleDragOver(e: DragEvent, index: number) {
  if (!isDragging.value || dragIndex.value === null) return
  dragOverIndex.value = index
}

function handleDrop(e: DragEvent, index: number) {
  if (!isDragging.value || dragIndex.value === null || dragIndex.value === index) return
  
  // 交换位置
  const draggedServer = servers.value[dragIndex.value]
  servers.value.splice(dragIndex.value, 1)
  servers.value.splice(index, 0, draggedServer)
  
  // 保存新顺序到后端
  saveServerOrderDebounced()
}

function handleDragEnd() {
  isDragging.value = false
  dragIndex.value = null
  dragOverIndex.value = null
}

let orderSaveTimer: ReturnType<typeof setTimeout> | null = null

async function saveServerOrder() {
  try {
    const ids = servers.value.map(s => s.id)
    await reorderServers(ids)
  } catch (error) {
    showToast('error', '错误', '保存排序失败')
  }
}

// 带 500ms debounce 的保存排序（拖拽时调用）
function saveServerOrderDebounced() {
  if (orderSaveTimer) clearTimeout(orderSaveTimer)
  orderSaveTimer = setTimeout(saveServerOrder, 500)
}

// GitHub 脚本地址 (使用加速域名，万能入口自动检测系统)
const GITHUB_SCRIPT_URL = 'https://v6.gh-proxy.org/https://raw.githubusercontent.com/anti-rainer/sboard/main/scripts/install-agent-auto.sh'

// 计算 Agent 部署命令
const agentDeployCommand = computed(() => {
  if (!formData.value.agent_token) return ''
  const panelUrl = window.location.origin
  return `curl -fsSL ${GITHUB_SCRIPT_URL} | bash -s -- --token ${formData.value.agent_token} --panel ${panelUrl}`
})

// 节点配置
const selectedNode = ref<Node | null>(null)
const nodeConfigData = ref({
  listen_port: 0,
  forward_enabled: false,
  forward_host: '',
  forward_port: 0,
  outbound_enabled: false,
  outbound_protocol: '',
  outbound_host: '',
  outbound_port: 0,
  outbound_password: '',
  outbound_method: '',
  outbound_username: '',
  outbound_sni: '',
  // VLESS/VMess 配置
  outbound_uuid: '',
  outbound_flow: '',
  outbound_security: 'auto',
  outbound_alter_id: 0,
  outbound_tls: false,
  outbound_reality: false,
  outbound_pub_key: '',
  outbound_short_id: '',
  outbound_fp: 'chrome',
  // Hysteria2 配置
  outbound_obfs: '',
  outbound_obfs_pwd: '',
  // 传输层配置 (VMess/VLESS)
  outbound_network: '',
  outbound_ws_path: '',
  outbound_ws_host: ''
})

// 当前服务器的节点配置缓存 (key 为 nodeId)
const currentServerNodeConfigs = ref<Record<string, NodeConfig>>({})

// 导入链接
const importLink = ref('')

// 服务器配置Tab
const serverConfigTab = ref<'forward' | 'outbound'>('forward')

// 落地出站管理
const serverOutbounds = ref<ServerOutbound[]>([])
const isEditingOutbound = ref(false)
const savingOutbound = ref(false)
const outboundImportLink = ref('')
const outboundData = ref<OutboundForm>(getDefaultOutboundData())

/** 落地出站表单：在 ServerOutbound 基础上补上后端已支持、但 API 类型里尚未声明的 TUIC 字段 */
type OutboundForm = Partial<ServerOutbound> & {
  tuic_congestion?: string
  tuic_udp_relay_mode?: string
  /** 落地节点真实出口国家（两位代码）。出站地址是入口，实际出口可能在别国 */
  country?: string
}

/** 兼容历史别名：ss→shadowsocks、socks5→socks（与后端归一化保持一致） */
function normalizeOutboundProtocol(protocol?: string): string {
  if (protocol === 'ss') return 'shadowsocks'
  if (protocol === 'socks5') return 'socks'
  return protocol || 'shadowsocks'
}

function getDefaultOutboundData(): OutboundForm {
  return {
    slot: 0,
    enabled: true,
    remark: '',
    protocol: 'shadowsocks',
    host: '',
    port: 443,
    country: '',
    password: '',
    method: '2022-blake3-aes-128-gcm',
    username: '',
    sni: '',
    uuid: '',
    flow: '',
    security: 'auto',
    alter_id: 0,
    tls: false,
    reality: false,
    pub_key: '',
    short_id: '',
    fp: 'chrome',
    obfs: '',
    obfs_pwd: '',
    network: '',
    ws_path: '',
    ws_host: '',
    alpn: '',
    skip_cert_verify: true,
    tuic_congestion: 'bbr',
    tuic_udp_relay_mode: 'native'
  }
}

// 落地出站 VLESS TLS 类型计算属性
const outboundVlessTlsType = computed({
  get() {
    if (outboundData.value.reality) return 'reality'
    if (outboundData.value.tls) return 'tls'
    return 'none'
  },
  set(value: string) {
    outboundData.value.tls = value === 'tls'
    outboundData.value.reality = value === 'reality'
    syncActiveOutboundBranch()
  }
})

// 落地出站：协议分支（先选主协议，再选该协议的具体分支）
const outboundBranches = computed(() => getBranches(outboundData.value.protocol || ''))
const activeOutboundBranch = ref<string | null>(null)
const activeOutboundBranchHint = computed(() => {
  const b = outboundBranches.value.find((x) => x.id === activeOutboundBranch.value)
  return b?.hint || ''
})

// Socks 不支持 TLS/Reality/传输层；只有 VLESS/VMess 有传输层
const outboundSupportsTls = computed(() => !['socks', 'socks5'].includes(outboundData.value.protocol || ''))
// 这几种协议都支持 ws / grpc / httpupgrade 传输层，
// 之前只列了 vless/vmess，选 trojan 时传输层输入框整块消失，
// 粘贴 ws 节点链接后解析出的参数在界面上既看不见也改不了。
const outboundSupportsTransport = computed(() =>
  ['vless', 'vmess', 'trojan', 'anytls'].includes(outboundData.value.protocol || ''))

/** 把分支预设写入出站表单（不引入新字段，只按预设填既有字段） */
function applyOutboundBranch(b: ProtocolBranch) {
  activeOutboundBranch.value = b.id
  const v = outboundData.value
  const p = b.patch
  if (p.tls !== undefined) v.tls = p.tls
  if (p.reality !== undefined) v.reality = p.reality
  if (p.transport !== undefined) {
    // 出站表单用 http 表示 HTTP/2，其余传输层名称与契约一致
    v.network = p.transport === null ? '' : (p.transport === 'h2' ? 'http' : p.transport)
  }
  if (p.flow !== undefined) v.flow = p.flow
  if (p.ssMethod !== undefined) v.method = p.ssMethod
  if (p.hy2Obfs !== undefined) v.obfs = p.hy2Obfs
  if (p.congestion !== undefined) v.tuic_congestion = p.congestion
}

/** 依据当前表单状态反推命中的分支（编辑已有落地时高亮） */
function syncActiveOutboundBranch() {
  const v = outboundData.value
  const network = v.network || ''
  const base = {
    tls: !!v.tls,
    reality: !!v.reality,
    flow: v.flow || '',
    ssMethod: v.method || '',
    hy2Obfs: v.obfs || '',
    congestion: v.tuic_congestion || ''
  }
  // 契约里 HTTP/2 的写法不统一（VLESS/Trojan 用 h2，VMess 用 http），两种都试一次
  activeOutboundBranch.value =
    detectBranch(v.protocol || '', { ...base, transport: network === 'http' ? 'h2' : network }) ||
    (network === 'http' ? detectBranch(v.protocol || '', { ...base, transport: 'http' }) : null)
}

/** 切换主协议：先收敛能力边界，再套用该协议的首个分支 */
function onOutboundProtocolChange() {
  const v = outboundData.value
  const protocol = v.protocol || ''
  if (protocol === 'tuic') {
    // TUIC 基于 QUIC，TLS 强制开启，不支持 Reality 与传输层
    v.tls = true
    v.reality = false
    v.network = ''
    if (!v.tuic_congestion) v.tuic_congestion = 'bbr'
    if (!v.tuic_udp_relay_mode) v.tuic_udp_relay_mode = 'native'
  } else if (protocol === 'socks' || protocol === 'socks5') {
    // Socks 不支持 TLS / Reality / 传输层
    v.tls = false
    v.reality = false
    v.network = ''
  }
  const list = getBranches(protocol)
  if (list.length > 0) {
    applyOutboundBranch(list[0])
  } else {
    activeOutboundBranch.value = null
  }
}

// 加载服务器的落地出站列表
async function loadServerOutbounds() {
  if (!formData.value.id) {
    serverOutbounds.value = []
    return
  }
  try {
    const res = await getServerOutbounds(formData.value.id)
    serverOutbounds.value = res.data.data || []
  } catch (error) {
    console.error('加载落地出站失败:', error)
    serverOutbounds.value = []
  }
}

// 打开添加落地出站模态框
function openAddOutboundModal() {
  if (!formData.value.id) {
    showToast('warning', '警告', '请先保存服务器后再添加落地出站')
    return
  }
  if (serverOutbounds.value.length >= 10) {
    showToast('warning', '警告', '最多只能添加 10 个落地出站')
    return
  }
  isEditingOutbound.value = false
  outboundData.value = getDefaultOutboundData()
  outboundImportLink.value = ''
  // 新增时套用默认协议的首个分支，保证胶囊与字段一致
  const list = getBranches(outboundData.value.protocol || '')
  if (list.length > 0) {
    applyOutboundBranch(list[0])
  } else {
    activeOutboundBranch.value = null
  }
  outboundModal?.show()
}

// 打开编辑落地出站模态框
function openEditOutboundModal(outbound: ServerOutbound) {
  isEditingOutbound.value = true
  const extra = outbound as OutboundForm
  const protocol = normalizeOutboundProtocol(outbound.protocol)
  outboundData.value = {
    ...outbound,
    protocol,
    // TUIC 基于 QUIC，TLS 恒为开启
    tls: protocol === 'tuic' ? true : outbound.tls,
    tuic_congestion: extra.tuic_congestion || 'bbr',
    tuic_udp_relay_mode: extra.tuic_udp_relay_mode || 'native'
  }
  outboundImportLink.value = ''
  // 编辑时按已存字段反推分支，高亮到命中的胶囊
  syncActiveOutboundBranch()
  outboundModal?.show()
}

// 保存落地出站
async function saveOutbound() {
  if (!outboundData.value.remark) {
    showToast('warning', '警告', '请填写备注名称')
    return
  }
  if (!outboundData.value.host) {
    showToast('warning', '警告', '请填写出站地址')
    return
  }
  
  savingOutbound.value = true
  try {
    // 显式带上 TUIC 字段，避免后端落回默认值
    const payload: OutboundForm = {
      ...outboundData.value,
      tuic_congestion: outboundData.value.tuic_congestion || 'bbr',
      tuic_udp_relay_mode: outboundData.value.tuic_udp_relay_mode || 'native'
    }
    if (isEditingOutbound.value && outboundData.value.slot) {
      await updateServerOutbound(formData.value.id, outboundData.value.slot, payload)
      showToast('success', '成功', '落地出站已更新')
    } else {
      await createServerOutbound(formData.value.id, payload)
      showToast('success', '成功', '落地出站已添加')
    }
    outboundModal?.hide()
    await loadServerOutbounds()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '保存失败')
  } finally {
    savingOutbound.value = false
  }
}

// 删除落地出站
async function deleteOutbound(outbound: ServerOutbound) {
  if (!confirm(`确定要删除"${outbound.remark}"吗？`)) return
  
  try {
    await deleteServerOutbound(formData.value.id, outbound.slot)
    showToast('success', '成功', '落地出站已删除')
    await loadServerOutbounds()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '删除失败')
  }
}

// 切换落地出站启用状态
async function toggleOutbound(outbound: ServerOutbound) {
  try {
    await toggleServerOutbound(formData.value.id, outbound.slot)
    outbound.enabled = !outbound.enabled
    showToast('success', '成功', `落地出站已${outbound.enabled ? '启用' : '禁用'}`)
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '操作失败')
  }
}

// 解析落地出站导入链接
function parseOutboundImportLink() {
  const link = outboundImportLink.value.trim()
  if (!link) {
    showToast('warning', '提示', '请输入节点链接')
    return
  }

  try {
    const p = parseProxyLink(link)
    applyParsedToOutbound(p)
    showToast('success', '成功', `已识别 ${p.protocol} 节点`)
    outboundImportLink.value = ''
  } catch (error: any) {
    showToast('error', '解析失败', error.message || '链接格式错误')
  }
}

// 把解析结果写入落地出站表单（保留已选槽位与启用状态）
function applyParsedToOutbound(p: ParsedProxy) {
  const v = outboundData.value
  // 解析器输出 socks5/ss 等历史别名，统一归一化到主协议 id
  v.protocol = normalizeOutboundProtocol(p.protocol)
  v.host = p.host
  v.port = p.port
  v.uuid = p.uuid
  v.password = p.password
  v.username = p.username
  if (p.protocol === 'vmess') {
    v.security = p.method || 'auto'
  } else if (p.method) {
    v.method = p.method
  }
  v.sni = p.sni
  v.flow = p.flow
  v.tls = p.tls
  v.reality = p.reality
  v.pub_key = p.pubKey
  v.short_id = p.shortId
  v.fp = p.fingerprint || 'chrome'
  v.obfs = p.obfs
  v.obfs_pwd = p.obfsPassword
  v.network = p.network
  v.ws_path = p.wsPath
  v.ws_host = p.wsHost
  v.alpn = p.alpn
  v.skip_cert_verify = p.skipCertVerify
  if (!v.remark && p.name) {
    v.remark = p.name
  }
  // TUIC 的拥塞控制与 UDP 转发模式链接里未必带，给默认值保证可用
  if (v.protocol === 'tuic') {
    v.tuic_congestion = v.tuic_congestion || 'bbr'
    v.tuic_udp_relay_mode = v.tuic_udp_relay_mode || 'native'
    v.tls = true
  }
  // 导入后同步分支胶囊高亮
  syncActiveOutboundBranch()
}

// 解析导入链接（写入节点配置的出站字段）
function parseImportLink() {
  const link = importLink.value.trim()
  if (!link) {
    showToast('warning', '提示', '请输入节点链接')
    return
  }

  try {
    const p = parseProxyLink(link)
    applyParsedToNodeConfig(p)
    showToast('success', '成功', `已识别 ${p.protocol} 节点`)
    importLink.value = ''
  } catch (error: any) {
    showToast('error', '解析失败', error.message || '链接格式错误')
  }
}

// 把解析结果写入节点配置的出站字段
function applyParsedToNodeConfig(p: ParsedProxy) {
  const v = nodeConfigData.value
  v.outbound_protocol = p.protocol
  v.outbound_host = p.host
  v.outbound_port = p.port
  v.outbound_uuid = p.uuid
  v.outbound_password = p.password
  v.outbound_username = p.username
  if (p.protocol === 'vmess') {
    v.outbound_security = p.method || 'auto'
  } else if (p.method) {
    v.outbound_method = p.method
  }
  v.outbound_sni = p.sni
  v.outbound_flow = p.flow
  v.outbound_tls = p.tls
  v.outbound_reality = p.reality
  v.outbound_pub_key = p.pubKey
  v.outbound_short_id = p.shortId
  v.outbound_fp = p.fingerprint || 'chrome'
  v.outbound_obfs = p.obfs
  v.outbound_obfs_pwd = p.obfsPassword
  v.outbound_network = p.network
  v.outbound_ws_path = p.wsPath
  v.outbound_ws_host = p.wsHost
}

function getDefaultFormData() {
  return {
    id: 0,
    name: '',
    host: '',
    host_ipv6: '',
    node_domain: '',
    dns_resolve: 'ipv4',
    category: 'direct',
    node_1: '',
    node_2: '',
    node_3: '',
    enabled: true,
    agent_token: '',
    agent_id: '',
    agent_online: false
  }
}

// 定时刷新状态
let statusRefreshTimer: number | null = null

/** 面板自身的构建批次标识，作为判断 agent 是否同版本的基准。
 *
 * 必须在 onMounted 之前声明：<script setup> 编译后是普通函数体，
 * const 不提升，放在 onMounted 之后会被访问到暂时性死区，
 * 异常被 catch 吞掉后值恒为空、版本不一致永远检测不到。
 */
const panelBuildId = ref('')

onMounted(async () => {
  // 先取面板自身的构建批次，作为判断 agent 版本一致性的基准
  try {
    const r = await fetch('/api/version', { cache: 'no-store' })
    if (r.ok) {
      const info = await r.json()
      if (info?.build_id) panelBuildId.value = info.build_id
    }
  } catch { /* 取不到就不做一致性判断，避免误报 */ }

  await loadData()
  serverModal = new Modal(serverModalEl.value!)
  deployModal = new Modal(deployModalEl.value!)
  deleteModal = new Modal(deleteModalEl.value!)
  nodeConfigModal = new Modal(nodeConfigModalEl.value!)
  outboundModal = new Modal(outboundModalEl.value!)
  
  // 启动定时刷新状态（每2秒）
  startStatusRefresh()
})

onUnmounted(() => {
  // 清理定时器
  stopStatusRefresh()
})

function startStatusRefresh() {
  if (statusRefreshTimer) return
  statusRefreshTimer = window.setInterval(refreshStatus, 2000)
}

function stopStatusRefresh() {
  if (statusRefreshTimer) {
    clearInterval(statusRefreshTimer)
    statusRefreshTimer = null
  }
}

// 刷新服务器状态（不影响其他交互）
async function refreshStatus() {
  try {
    const res = await getServersStatus()
    const statuses = res.data.data.statuses || []
    
    // 只更新状态数据，不重新渲染整个列表
    statuses.forEach((status: ServerStatus) => {
      const server = servers.value.find(s => s.id === status.id)
      if (server) {
        server.agent_online = status.agent_online
        server.cpu_usage = status.cpu_usage
        server.mem_usage = status.mem_usage
        server.disk_usage = status.disk_usage
        server.net_in = status.net_in
        server.net_out = status.net_out
        server.monthly_in = status.monthly_in
        server.monthly_out = status.monthly_out
      }
    })
  } catch {
    // 静默失败，不影响用户操作
  }
}

// 隐藏 IP 后三段，只显示第一段
function maskIP(ip: string | undefined): string {
  if (!ip) return ''
  // 处理 IPv4
  if (ip.includes('.')) {
    const parts = ip.split('.')
    if (parts.length === 4) {
      return `${parts[0]}.*.*.*`
    }
  }
  // 处理 IPv6
  if (ip.includes(':')) {
    const parts = ip.split(':')
    if (parts.length >= 2) {
      return `${parts[0]}:${parts[1]}:****`
    }
  }
  return ip
}

// 格式化网速显示
function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec < 1024) return `${bytesPerSec}B/s`
  if (bytesPerSec < 1024 * 1024) return `${(bytesPerSec / 1024).toFixed(1)}KB/s`
  return `${(bytesPerSec / 1024 / 1024).toFixed(2)}MB/s`
}

// 格式化流量显示
function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(2)}MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)}GB`
}

async function loadData() {
  loading.value = true
  try {
    const [serversRes, nodesRes] = await Promise.all([getServers(), getNodes()])
    servers.value = (serversRes.data.data.servers || []).map((s: Server) => ({
      ...s,
      deploying: false,
      deployingFolder: false
    }))
    availableNodes.value = nodesRes.data.data.nodes || []
  } catch (error) {
    showToast('error', '错误', '加载数据失败')
  } finally {
    loading.value = false
  }
}

function getCategoryName(category: string) {
  const names: Record<string, string> = {
    direct: '线路',
    relay: '落地',
    home: '自用'
  }
  return names[category] || category
}

function getCategoryClass(category: string) {
  const classes: Record<string, string> = {
    direct: 'text-primary',
    relay: 'text-success',
    home: 'text-warning'
  }
  return classes[category] || ''
}

function isNodeConfigured(node: Node) {
  const config = currentServerNodeConfigs.value[node.id.toString()]
  return config && (config.forward_enabled || config.outbound_enabled)
}

// 检查节点是否启用了端口转发
function isForwardEnabled(node: Node) {
  const config = currentServerNodeConfigs.value[node.id.toString()]
  return config?.forward_enabled || false
}

// 检查节点是否启用了落地出站
function isOutboundEnabled(node: Node) {
  const config = currentServerNodeConfigs.value[node.id.toString()]
  return config?.outbound_enabled || false
}

function openAddModal() {
  isEditing.value = false
  formData.value = getDefaultFormData()
  currentServerNodeConfigs.value = {} // 新建服务器时清空节点配置缓存
  serverModal?.show()
}

async function openEditModal(server: Server) {
  isEditing.value = true
  formData.value = {
    id: server.id,
    name: server.name,
    host: server.host || '',
    host_ipv6: server.host_ipv6 || '',
    node_domain: server.node_domain || '',
    dns_resolve: server.dns_resolve || 'ipv4',
    category: server.category,
    node_1: server.node_1 || '',
    node_2: server.node_2 || '',
    node_3: server.node_3 || '',
    enabled: server.enabled === 1 || server.enabled === true,
    agent_token: server.agent_token || '',
    agent_id: server.agent_id || '',
    agent_online: server.agent_online || false
  }
  
  // 重置 tab 状态
  serverConfigTab.value = 'forward'
  
  // 加载服务器的节点配置
  currentServerNodeConfigs.value = {}
  try {
    const res = await getNodeConfigs(server.id)
    currentServerNodeConfigs.value = res.data.data || {}
  } catch (error) {
    console.error('加载节点配置失败:', error)
  }
  
  // 加载落地出站
  await loadServerOutbounds()
  
  serverModal?.show()
}

async function saveServer() {
  if (!formData.value.name) {
    showToast('warning', '警告', '请填写备注名称')
    return
  }

  saving.value = true
  try {
    const data: any = {
      name: formData.value.name,
      node_domain: formData.value.node_domain,
      dns_resolve: formData.value.dns_resolve,
      category: formData.value.category,
      node_1: formData.value.node_1,
      node_2: formData.value.node_2,
      node_3: formData.value.node_3,
      enabled: formData.value.enabled ? 1 : 0
    }

    if (isEditing.value) {
      await updateServer(formData.value.id, data)
      showToast('success', '成功', '服务器已更新')
      serverModal?.hide()
    } else {
      const res = await createServer(data)
      // 创建成功后，切换到编辑模式，显示部署命令
      const newServer = res.data.data
      formData.value.id = newServer.id
      formData.value.agent_token = newServer.agent_token
      isEditing.value = true
      showToast('success', '成功', '服务器已添加，请复制下方的部署命令')
    }
    
    await loadData()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

function confirmDelete(server: Server) {
  deleteTarget.value = server
  deleteModal?.show()
}

async function doDelete() {
  if (!deleteTarget.value) return
  
  deleting.value = true
  try {
    await deleteServer(deleteTarget.value.id)
    showToast('success', '成功', '服务器已删除')
    deleteModal?.hide()
    await loadData()
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '删除失败')
  } finally {
    deleting.value = false
  }
}

async function deployFolder(server: ServerWithState) {
  server.deployingFolder = true
  try {
    const res = await deployServer(server.id)
    deployOutput.value = res.data.data?.output || '部署完成'
    deployModal?.show()
    showToast('success', '成功', `${server.name} 部署完成`)
  } catch (error: any) {
    deployOutput.value = error.response?.data?.error || '部署失败'
    deployModal?.show()
    showToast('error', '错误', `${server.name} 部署失败`)
  } finally {
    server.deployingFolder = false
  }
}

/** 正在升级 Agent 的服务器 ID（0 表示没有） */
const upgradingAgent = ref(0)

/** 单台服务器的 Agent 升级 */
async function upgradeAgent(server: ServerWithState) {
  upgradingAgent.value = server.id
  try {
    const res = await updateServerAgent(server.id)
    showToast('success', '已下发', res.data.data?.message || '更新指令已发送')
  } catch (error: any) {
    showToast('error', '失败', error.response?.data?.error || '下发更新指令失败')
  } finally {
    // Agent 下载替换需要时间，留一小段再解锁按钮（避免重复点击）
    window.setTimeout(() => { upgradingAgent.value = 0 }, 8000)
  }
}

/** 面板自我更新中 */
const updatingPanel = ref(false)

/** 触发面板自我更新 */
async function panelUpdate() {
  if (!confirm('将下载最新版本并重启面板服务，期间几秒内无法访问。确定继续？')) return

  updatingPanel.value = true
  try {
    const res = await panelSelfUpdate()
    showToast('success', '已启动', res.data.data?.message || '面板更新已启动，服务即将重启')
    pollPanelUpdate()
  } catch (error: any) {
    updatingPanel.value = false
    showToast('error', '错误', error.response?.data?.error || '面板更新启动失败')
  }
}

/** 轮询更新状态：更新会重启服务，探测到服务恢复即算完成 */
function pollPanelUpdate() {
  let attempts = 0
  const timer = window.setInterval(async () => {
    attempts++
    try {
      const res = await getPanelUpdateStatus()
      const st = res.data.data
      if (st?.finished) {
        window.clearInterval(timer)
        updatingPanel.value = false
        showToast(st.success ? 'success' : 'error',
          st.success ? '更新完成' : '更新失败', st.message || '')
      }
    } catch {
      // 服务重启中，请求会失败——这本身说明更新在进行，继续等
    }
    // 最多等 5 分钟
    if (attempts > 60) {
      window.clearInterval(timer)
      updatingPanel.value = false
      showToast('warning', '提示', '更新状态未知，请手动刷新页面确认')
    }
  }, 5000)
}

async function updateAllAgents() {
  updatingAgents.value = true
  
  try {
    const res = await apiUpdateAllAgents()
    const data = res.data.data
    
    showToast('success', '成功', `${data.message}（共 ${data.total} 个存活 Agent）`)
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || 'Agent 更新失败')
  } finally {
    updatingAgents.value = false
  }
}

async function deployAll() {
  deploying.value = true
  
  try {
    const res = await apiDeployAll()
    const data = res.data.data
    
    showToast('success', '成功', `已向 ${data.total} 个在线服务器广播部署`)
  } catch (error: any) {
    showToast('error', '错误', error.response?.data?.error || '部署失败')
  } finally {
    deploying.value = false
  }
}

async function openNodeConfigModal(node: Node) {
  selectedNode.value = node
  // 重置为默认值（仅端口转发）
  nodeConfigData.value = {
    listen_port: node.port,
    forward_enabled: false,
    forward_host: '',
    forward_port: 0,
    // 保留这些字段以兼容旧数据，但不再在UI中使用
    outbound_enabled: false,
    outbound_protocol: '',
    outbound_host: '',
    outbound_port: 0,
    outbound_password: '',
    outbound_method: '',
    outbound_username: '',
    outbound_sni: '',
    outbound_uuid: '',
    outbound_flow: '',
    outbound_security: 'auto',
    outbound_alter_id: 0,
    outbound_tls: false,
    outbound_reality: false,
    outbound_pub_key: '',
    outbound_short_id: '',
    outbound_fp: 'chrome',
    outbound_obfs: '',
    outbound_obfs_pwd: '',
    outbound_network: '',
    outbound_ws_path: '',
    outbound_ws_host: ''
  }
  
  // 尝试加载已有配置
  if (formData.value.id) {
    try {
      const res = await getNodeConfigs(formData.value.id)
      // 后端返回的是 map 格式，key 为 nodeId
      const existing = res.data.data?.[node.id.toString()] as NodeConfig | undefined
      if (existing) {
        nodeConfigData.value.listen_port = existing.listen_port || node.port
        nodeConfigData.value.forward_enabled = existing.forward_enabled || false
        nodeConfigData.value.forward_host = existing.forward_host || ''
        nodeConfigData.value.forward_port = existing.forward_port || 0
      }
    } catch (error) {
      console.error('加载节点配置失败:', error)
    }
  }
  
  nodeConfigModal?.show()
}

async function saveNodeConfig() {
  if (!formData.value.id || !selectedNode.value) {
    showToast('error', '错误', '请先保存服务器')
    return
  }
  
  try {
    await saveNodeConfigApi(formData.value.id, selectedNode.value.id, nodeConfigData.value)
    
    // 更新本地缓存
    currentServerNodeConfigs.value[selectedNode.value.id.toString()] = {
      ...nodeConfigData.value,
      node_id: selectedNode.value.id,
      server_id: formData.value.id
    } as NodeConfig
    
    showToast('success', '成功', '节点配置已保存')
    nodeConfigModal?.hide()
  } catch (error: any) {
    console.error('保存节点配置失败:', error)
    showToast('error', '错误', error.response?.data?.message || '保存失败')
  }
}

// Agent 相关函数
async function copyAgentToken() {
  if (formData.value.agent_token) {
    try {
      await copyToClipboard(formData.value.agent_token)
      showToast('success', '成功', 'Agent Token 已复制到剪贴板')
    } catch (error) {
      showToast('error', '错误', '复制失败')
    }
  }
}

// 通用复制函数
async function copyToClipboard(text: string) {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text)
  } else {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.left = '-9999px'
    document.body.appendChild(textarea)
    textarea.select()
    document.execCommand('copy')
    document.body.removeChild(textarea)
  }
}

/** 面板自身的构建批次标识，作为判断 agent 是否同版本的基准 */
/** 该机器的 agent 是否与面板不在同一构建批次。
 *
 * 用构建批次而不是二进制 hash：同一批次下 amd64 与 arm64 的二进制
 * hash 必然不同，按 hash 比较会把「正常架构差异」误报成版本不一致
 * （实例：x86 主机都是 4a7a8218…，ARM 路由是 a86819cc…，均属最新）。
 */
/** 不一致标签的悬浮说明 */
function mismatchTip(server: ServerWithState): string {
  return `agent 构建标识: ${server.agent_build_id || '未上报'}\n`
    + `面板构建标识: ${panelBuildId || '未知'}\n`
    + '两者不同说明该机器未更新到当前发布'
}

function hashMismatch(server: ServerWithState): boolean {
  const agentId = server.agent_build_id
  // 面板还没拿到自己的 build_id，或 agent 尚未上报 → 不做判断，避免误报
  if (!panelBuildId.value || !agentId) return false
  return agentId !== panelBuildId.value
}

async function copyDeployCommand() {
  const command = agentDeployCommand.value
  if (!command) {
    showToast('warning', '警告', '部署命令为空，请确保服务器已保存')
    return
  }
  try {
    await copyToClipboard(command)
    showToast('success', '成功', '部署命令已复制到剪贴板')
  } catch (error) {
    showToast('error', '错误', '复制失败，请手动复制')
    console.error('Copy failed:', error)
  }
}
</script>

<style scoped>
.server-card {
  background: var(--sb-surface);
  border: 1px solid var(--sb-border);
  border-radius: var(--sb-radius);
  padding: 1.05rem 1rem 1rem;
  box-shadow: var(--sb-shadow-xs);
  transition: box-shadow var(--sb-dur) var(--sb-ease),
    border-color var(--sb-dur) var(--sb-ease),
    transform var(--sb-dur) var(--sb-ease);
  position: relative;
  overflow: hidden;
}

.server-card:hover {
  transform: translateY(-2px);
  border-color: var(--sb-border-strong);
  box-shadow: var(--sb-shadow-md);
}

.server-card.dragging {
  opacity: 0.5;
  transform: scale(0.95) !important;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.server-card.drag-over {
  border: 2px dashed var(--sb-accent);
  background: var(--sb-accent-soft);
}

.server-title-wrap {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  min-width: 0;
}

.drag-handle {
  width: 1rem;
  height: 1.15rem;
  display: grid;
  place-items: center;
  flex: 0 0 auto;
  cursor: grab;
  color: var(--sb-text-subtle);
  border-radius: 5px;
  opacity: 0.3;
  z-index: 2;
  transition: opacity var(--sb-dur) var(--sb-ease),
    background var(--sb-dur) var(--sb-ease),
    color var(--sb-dur) var(--sb-ease);
  user-select: none;
}

.server-card:hover .drag-handle { opacity: 1; }

.drag-handle:hover {
  background: var(--sb-surface-3);
  color: var(--sb-accent);
}

.drag-handle:active {
  cursor: grabbing;
  color: var(--sb-accent-hover);
}

.drag-handle i {
  font-size: 0.8rem;
}

.server-header {
  margin-bottom: 0.9rem;
}

.server-title-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-bottom: 0.35rem;
}

.server-badges {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  flex-wrap: wrap;
}

.server-title {
  font-size: 1.0625rem;
  font-weight: 650;
  letter-spacing: -0.015em;
  color: var(--sb-text);
  margin: 0;
}

.server-host {
  color: var(--sb-text-muted);
  font-size: 0.875rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.card-actions {
  display: flex;
  gap: 0.4rem;
  margin-top: 0.9rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--sb-border);
}

.card-actions .sb-icon-btn {
  flex: 1;
}

/* 监控面板样式 */
.monitor-panel {
  background: var(--sb-surface-2);
  border-radius: 8px;
  padding: 0.5rem;
}

.monitor-item {
  padding: 0.25rem 0;
}

.monitor-label {
  font-size: 0.7rem;
  color: var(--sb-text-muted);
}

.monitor-value {
  font-size: 0.75rem;
  font-weight: 600;
}

.monitor-panel .progress {
  background-color: var(--sb-border);
  margin-top: 2px;
}

.speed-item {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  font-size: 0.875rem;
  padding: 0.25rem 0;
}

.speed-item i {
  font-size: 0.85rem;
}

.speed-value {
  font-weight: 500;
  font-family: 'Consolas', 'Monaco', monospace;
}

.traffic-value {
  font-size: 0.875rem;
  font-family: 'Consolas', 'Monaco', monospace;
  font-weight: 500;
  margin-left: auto;
}

.offline-info {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.65rem;
  border-radius: var(--sb-radius-sm);
  font-size: 0.8125rem;
  background: var(--sb-surface-2);
  border-radius: 8px;
  padding: 0.5rem;
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

/* 部署命令框样式 */
.deploy-command-box {
  position: relative;
  background: var(--sb-surface-2);
  border: 1px solid var(--sb-border);
  border-radius: 8px;
  padding: 0.75rem 2.5rem 0.75rem 0.75rem;
  cursor: pointer;
  transition: all 0.3s ease;
  overflow: hidden;
}

.deploy-command-box:hover {
  background: var(--sb-accent-soft);
  border-color: var(--sb-accent);
  box-shadow: 0 2px 8px rgba(59, 130, 246, 0.15);
}

.deploy-command-box:active {
  transform: scale(0.98);
}

.deploy-command-text {
  display: block;
  font-size: 0.8rem;
  color: var(--sb-text);
  word-break: break-all;
  white-space: pre-wrap;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  line-height: 1.4;
  padding-right: 1rem;
}

.copy-icon {
  position: absolute;
  right: 0.75rem;
  top: 50%;
  transform: translateY(-50%);
  color: var(--sb-text-muted);
  font-size: 1rem;
  transition: all 0.3s ease;
}

.deploy-command-box:hover .copy-icon {
  color: var(--sb-accent);
  transform: translateY(-50%) scale(1.1);
}

/* 落地出站卡片样式 */
.outbound-card {
  background: var(--sb-surface-2);
  border: 1px solid var(--sb-border);
  border-radius: 12px;
  padding: 1rem;
  transition: all 0.3s ease;
}

.outbound-card:hover {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
  border-color: var(--sb-border-strong);
}

.outbound-card.outbound-disabled {
  opacity: 0.6;
  background: var(--sb-surface-3);
}

.outbound-card .card-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--sb-text);
  margin-bottom: 0.25rem;
}

.outbound-card .card-subtitle {
  font-size: 0.8rem;
  color: var(--sb-text-muted);
}

.outbound-card .btn-group .btn {
  padding: 0.25rem 0.5rem;
  font-size: 0.8rem;
}
</style>

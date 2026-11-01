<template>
  <div class="login-wrapper">
    <div class="login-container">
      <div class="card login-card shadow-lg">
        <div class="card-body p-4 p-sm-5">
          <div class="text-center mb-4">
            <div class="login-mark"><i class="bi bi-shield-lock-fill"></i></div>
            <h2 class="login-title">SBoard</h2>
            <p class="login-sub">登录以继续管理面板</p>
          </div>
          
          <!-- 密码登录表单（如果未禁用） -->
          <form v-if="!disablePasswordLogin" @submit.prevent="handleLogin">
            <div class="mb-3">
              <label class="form-label">用户名</label>
              <div class="login-field">
                <i class="bi bi-person"></i>
                <input
                  type="text"
                  class="form-control"
                  v-model="username"
                  placeholder="用户名"
                  autocomplete="username"
                  required
                >
              </div>
            </div>
            
            <div class="mb-4">
              <label class="form-label">密码</label>
              <div class="login-field">
                <i class="bi bi-lock"></i>
                <input
                  type="password"
                  class="form-control"
                  v-model="password"
                  placeholder="密码"
                  autocomplete="current-password"
                  required
                >
              </div>
            </div>
            
            <div v-if="error" class="alert alert-danger py-2 mb-3">
              <i class="bi bi-exclamation-circle"></i> {{ error }}
            </div>
            
            <button type="submit" class="btn btn-primary w-100" :disabled="loading">
              <span v-if="loading" class="spinner-border spinner-border-sm me-2"></span>
              <i v-else class="bi bi-box-arrow-in-right me-2"></i>
              {{ loading ? '登录中...' : '登录' }}
            </button>
          </form>

          <!-- 密码登录已禁用提示 -->
          <div v-if="disablePasswordLogin && oauthProviders.length > 0" class="text-center mb-3">
            <div v-if="error" class="alert alert-danger py-2 mb-3">
              <i class="bi bi-exclamation-circle"></i> {{ error }}
            </div>
            <p class="text-muted mb-0">
              <i class="bi bi-info-circle me-1"></i>
              密码登录已禁用，请使用以下方式登录
            </p>
          </div>

          <!-- OAuth 登录分隔线 -->
          <div v-if="oauthProviders.length > 0 && !disablePasswordLogin" class="divider my-4">
            <span class="divider-text">或</span>
          </div>

          <!-- OAuth 提供商按钮 -->
          <div v-if="oauthProviders.length > 0" class="oauth-buttons">
            <button 
              v-for="provider in oauthProviders"
              :key="provider.name"
              type="button"
              class="btn btn-outline-secondary w-100 mb-2"
              :disabled="oauthLoading"
              @click="handleOAuthLogin(provider.name)"
            >
              <i :class="'bi bi-' + provider.icon + ' me-2'"></i>
              使用 {{ provider.label }} 登录
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { login, getOAuthProviders, getGitHubLoginUrl } from '@/api'

interface OAuthProvider {
  name: string
  label: string
  icon: string
  enabled: boolean
}

const emit = defineEmits(['login-success'])
const router = useRouter()
const route = useRoute()

const username = ref('')
const password = ref('')
const loading = ref(false)
const oauthLoading = ref(false)
const error = ref('')
const oauthProviders = ref<OAuthProvider[]>([])
const disablePasswordLogin = ref(false)

// 获取 OAuth 提供商列表
const fetchOAuthProviders = async () => {
  try {
    const response = await getOAuthProviders()
    oauthProviders.value = response.data.data || []
    disablePasswordLogin.value = response.data.disable_password_login || false
  } catch (e) {
    // OAuth 未配置时忽略错误
    console.log('OAuth providers not available')
  }
}

// 处理 OAuth 登录错误（从 URL 参数）
const handleOAuthError = () => {
  const errorParam = route.query.error as string
  if (errorParam) {
    const errorMessages: Record<string, string> = {
      'oauth_disabled': 'OAuth 登录未启用',
      'invalid_state': '登录请求已过期，请重试',
      'token_exchange_failed': '获取令牌失败，请重试',
      'user_fetch_failed': '获取用户信息失败',
      'user_not_allowed': '您的账户未被授权登录',
      'user_create_failed': '创建用户失败',
      'token_generate_failed': '生成令牌失败'
    }
    error.value = errorMessages[errorParam] || '登录失败：' + errorParam
    // 清除 URL 参数
    router.replace({ query: {} })
  }
}

onMounted(() => {
  // 清除过期 token
  const token = localStorage.getItem('token')
  if (token) {
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      if (payload.exp && Date.now() >= payload.exp * 1000) {
        localStorage.removeItem('token')
      }
    } catch {}
  }
  fetchOAuthProviders()
  handleOAuthError()
})

const handleLogin = async () => {
  if (!username.value || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  
  loading.value = true
  error.value = ''
  
  try {
    const response = await login(username.value, password.value)
    localStorage.setItem('token', response.data.data.token)
    emit('login-success')
    router.push('/users')
  } catch (e: any) {
    error.value = e.response?.data?.error || '登录失败，请检查用户名和密码'
  } finally {
    loading.value = false
  }
}

const handleOAuthLogin = async (provider: string) => {
  oauthLoading.value = true
  error.value = ''
  
  try {
    if (provider === 'github') {
      const response = await getGitHubLoginUrl()
      // 兼容两种响应格式: { data: { url: "..." } } 或 { data: "..." }
      const url = typeof response.data.data === 'string' 
        ? response.data.data 
        : response.data.data?.url
      if (response.data.success && url) {
        window.location.href = url
      }
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || 'OAuth 登录失败'
    oauthLoading.value = false
  }
}
</script>

<style scoped>
.login-wrapper {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  position: relative;
  overflow: hidden;
  background: var(--sb-bg);
}

/* 背景：双色光晕 + 网格点阵，代替原来的紫色渐变动画 */
.login-wrapper::before {
  content: "";
  position: absolute;
  inset: -20%;
  background-image:
    radial-gradient(680px 420px at 22% 18%, rgba(79, 70, 229, 0.16), transparent 62%),
    radial-gradient(620px 400px at 82% 78%, rgba(34, 211, 238, 0.13), transparent 60%);
  pointer-events: none;
}

.login-wrapper::after {
  content: "";
  position: absolute;
  inset: 0;
  background-image: radial-gradient(
    color-mix(in srgb, var(--sb-text-subtle) 32%, transparent) 1px,
    transparent 1px
  );
  background-size: 22px 22px;
  mask-image: radial-gradient(circle at 50% 45%, #000 0%, transparent 72%);
  -webkit-mask-image: radial-gradient(circle at 50% 45%, #000 0%, transparent 72%);
  opacity: 0.55;
  pointer-events: none;
}

.login-container {
  position: relative;
  z-index: 1;
  width: 100%;
  max-width: 400px;
}

.login-card {
  border: 1px solid var(--sb-border);
  border-radius: var(--sb-radius-lg);
  background: var(--sb-surface);
  box-shadow: var(--sb-shadow-lg);
  overflow: hidden;
}

.login-card:hover {
  border-color: var(--sb-border);
  box-shadow: var(--sb-shadow-lg);
}

.login-mark {
  width: 3rem;
  height: 3rem;
  margin: 0 auto 0.9rem;
  display: grid;
  place-items: center;
  border-radius: 14px;
  background: linear-gradient(135deg, var(--sb-accent), #22d3ee);
  color: #fff;
  font-size: 1.35rem;
  box-shadow: 0 8px 22px var(--sb-accent-ring);
}

.login-title {
  margin: 0;
  font-size: 1.5rem;
  font-weight: 700;
  letter-spacing: -0.03em;
  color: var(--sb-text);
}

.login-sub {
  margin: 0.3rem 0 0;
  font-size: 0.8375rem;
  color: var(--sb-text-muted);
}

/* 图标内嵌式输入框 */
.login-field {
  position: relative;
}

.login-field > i {
  position: absolute;
  left: 0.8rem;
  top: 50%;
  transform: translateY(-50%);
  color: var(--sb-text-subtle);
  font-size: 0.95rem;
  pointer-events: none;
  transition: color 160ms cubic-bezier(0.4, 0, 0.2, 1);
}

.login-field .form-control {
  height: 44px;
  padding-left: 2.4rem;
  background: var(--sb-surface-2);
}

.login-field .form-control:focus {
  background: var(--sb-surface);
}

.login-field:focus-within > i {
  color: var(--sb-accent);
}

.login-card .btn-primary {
  height: 44px;
  font-weight: 600;
}

/* OAuth 分隔线 */
.divider {
  display: flex;
  align-items: center;
  text-align: center;
  gap: 0.75rem;
}

.divider::before,
.divider::after {
  content: "";
  flex: 1;
  height: 1px;
  background: var(--sb-border);
}

.divider-text {
  color: var(--sb-text-subtle);
  font-size: 0.78rem;
  flex: 0 0 auto;
}

/* OAuth 按钮 */
.oauth-buttons .btn {
  height: 42px;
  justify-content: center;
  background: var(--sb-surface);
  border: 1px solid var(--sb-border-strong);
  color: var(--sb-text);
}

.oauth-buttons .btn:hover {
  background: var(--sb-surface-2);
  border-color: var(--sb-border-strong);
  color: var(--sb-text);
}

.alert-danger {
  background: var(--sb-danger-soft);
  border: 1px solid color-mix(in srgb, var(--sb-danger) 28%, transparent);
  color: var(--sb-danger);
  font-size: 0.8375rem;
  border-radius: var(--sb-radius-sm);
}
</style>

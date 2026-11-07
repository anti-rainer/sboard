// 前端版本自检：防止 CDN 缓存旧资源导致用户停留在旧界面。
//
// 两个保障层次：
//   1. 服务端侧（internal/handler/frontend_cache.go）：
//      index.html 强制 no-store，assets 带 hash 长缓存，未命中返回 404 而非回退 HTML。
//   2. 前端侧（这里）：
//      页面加载后定期拉 /api/version（不走 CDN 缓存），若服务端构建时间与
//      页面首次加载时记录的不一致，说明已发布新版本 —— 提示用户刷新。
//
// 第 2 层专门对付「CF 边缘节点仍持有旧 index.html」的窗口期：
// 即使首屏是旧的，用户也能在几秒内收到更新提示，而不是一直用旧版。

import { ref } from 'vue'

/** 本次页面加载时观察到的服务端构建时间 */
let loadedBuildAt = ''

/** 是否检测到服务端已更新 */
export const updateAvailable = ref(false)

/** 是否正在检查（避免并发） */
let checking = false

interface VersionInfo {
  version?: string
  build_at?: string
  agent_hash?: string
}

/** 拉取服务端版本信息，不走缓存 */
async function fetchVersion(): Promise<VersionInfo | null> {
  try {
    const res = await fetch('/api/version', {
      cache: 'no-store',
      headers: { 'Cache-Control': 'no-cache' },
    })
    if (!res.ok) return null
    return await res.json()
  } catch {
    return null
  }
}

/** 初始化：记录基线并开始轮询 */
export function startVersionWatch(intervalMs = 60_000): void {
  const tick = async () => {
    if (checking || document.visibilityState === 'hidden') return
    checking = true
    try {
      const info = await fetchVersion()
      const buildAt = info?.build_at || ''
      if (!buildAt) return
      if (!loadedBuildAt) {
        loadedBuildAt = buildAt
        return
      }
      if (buildAt !== loadedBuildAt) {
        updateAvailable.value = true
      }
    } finally {
      checking = false
    }
  }

  // 立即建立基线，之后定时检查
  void tick()
  window.setInterval(tick, intervalMs)

  // 切回标签页时立刻查一次（用户可能长时间挂着旧页面）
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') void tick()
  })
}

/** 重新加载页面（拿最新资源） */
export function reloadToUpdate(): void {
  // 加时间戳参数，绕开 CDN 对 index.html 的残留缓存
  const url = new URL(window.location.href)
  url.searchParams.set('_v', Date.now().toString(36))
  window.location.replace(url.toString())
}

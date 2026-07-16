// Global toast state (shared via a simple event bus)
const toastListeners = []
export function onToast(fn) {
  toastListeners.push(fn)
  return () => {
    const i = toastListeners.indexOf(fn)
    if (i >= 0) toastListeners.splice(i, 1)
  }
}
function emitToast(message, type = 'info') {
  toastListeners.forEach(fn => fn(message, type))
}

export async function api(path, options = {}) {
  try {
    const res = await fetch(path, {
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json', ...options.headers },
      ...options,
    })
    // 401 → 触发全局未认证事件
    if (res.status === 401) {
      emitUnauthorized()
      throw new Error('未登录或会话已过期')
    }
    const data = await res.json()
    if (!res.ok) throw new Error(data.error || data.message || 'HTTP ' + res.status)
    // 自动解包 {success, data} 包装（后端部分接口使用此格式）
    if (data && typeof data === 'object' && 'success' in data && 'data' in data) {
      return data.data
    }
    return data
  } catch (e) {
    emitToast(e.message, 'error')
    throw e
  }
}

export function showToast(message, type = 'info') {
  emitToast(message, type)
}

// ---------- 认证 ----------

const unauthListeners = []
export function onUnauthorized(fn) {
  unauthListeners.push(fn)
  return () => {
    const i = unauthListeners.indexOf(fn)
    if (i >= 0) unauthListeners.splice(i, 1)
  }
}
function emitUnauthorized() {
  unauthListeners.forEach(fn => fn())
}

/** 检查当前是否已登录 */
export async function checkAuth() {
  const res = await fetch('/api/auth/me', { credentials: 'same-origin' })
  const data = await res.json()
  return data
}

/** 执行登录 */
export async function login(username, password) {
  const res = await fetch('/api/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ username, password }),
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || '登录失败')
  return data
}

/** 登出 */
export async function logout() {
  await fetch('/api/auth/logout', {
    method: 'POST',
    credentials: 'same-origin',
  })
}

/** 修改密码 */
export async function changePassword(oldPassword, newPassword) {
  const res = await fetch('/api/auth/change-password', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
  })
  const data = await res.json()
  if (!res.ok) throw new Error(data.error || '修改密码失败')
  return data
}

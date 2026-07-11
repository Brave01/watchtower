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

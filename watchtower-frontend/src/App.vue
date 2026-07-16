<template>
  <Login v-if="!authenticated" @logged-in="onLoggedIn" />
  <div v-else class="app-layout">
    <!-- Sidebar -->
    <aside class="sidebar">
      <div class="sidebar-logo">S</div>
      <nav class="sidebar-nav">
        <button v-for="tab in tabs" :key="tab.id" class="sidebar-btn" :class="{active: activeTab===tab.id}" :title="tab.label" @click="activeTab=tab.id">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" v-html="tab.icon"></svg>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <button class="sidebar-btn" title="修改密码" @click="showChangePwd = true">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/></svg>
        </button>
        <button class="sidebar-btn" title="退出登录" @click="handleLogout">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
        </button>
        <button class="theme-toggle" @click="toggleTheme" :title="theme === 'dark' ? '切换亮色主题' : '切换暗色主题'">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" v-if="theme==='dark'"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" v-else><circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/></svg>
        </button>
      </div>
    </aside>

    <!-- Main content -->
    <main class="main-content">
      <Dashboard v-if="activeTab==='dashboard'"/>
      <Hosts v-if="activeTab==='hosts'"/>
      <LogMonitor v-if="activeTab==='logs'"/>
      <Diagram v-if="activeTab==='diagram'"/>
    </main>

    <!-- Change Password Modal -->
    <div v-if="showChangePwd" class="modal-overlay" @click.self="showChangePwd = false">
      <div class="modal" style="max-width:400px">
        <div class="modal-header">
          <h3 class="modal-title">修改密码</h3>
          <button class="modal-close" @click="showChangePwd = false">&times;</button>
        </div>
        <div class="modal-body">
          <div v-if="pwdError" class="login-error" style="margin-bottom:16px">{{ pwdError }}</div>
          <div v-if="pwdSuccess" class="login-success" style="margin-bottom:16px">{{ pwdSuccess }}</div>
          <div class="form-group">
            <label class="form-label">当前密码</label>
            <input class="form-input" type="password" v-model="pwdForm.old" placeholder="输入当前密码" @keyup.enter="submitChangePwd" />
          </div>
          <div class="form-group">
            <label class="form-label">新密码</label>
            <input class="form-input" type="password" v-model="pwdForm.new1" placeholder="输入新密码" @keyup.enter="submitChangePwd" />
          </div>
          <div class="form-group">
            <label class="form-label">确认新密码</label>
            <input class="form-input" type="password" v-model="pwdForm.new2" placeholder="再次输入新密码" @keyup.enter="submitChangePwd" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showChangePwd = false">取消</button>
          <button class="btn btn-primary" @click="submitChangePwd" :disabled="pwdLoading">{{ pwdLoading ? '修改中...' : '确认修改' }}</button>
        </div>
      </div>
    </div>

    <!-- Toasts -->
    <div class="toast-container">
      <div v-for="(t,i) in toasts" :key="i" class="toast" :class="'toast-'+t.type" @click="toasts.splice(i,1)">{{ t.message }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, reactive } from 'vue'
import Login from './views/Login.vue'
import Dashboard from './views/Dashboard.vue'
import Hosts from './views/Hosts.vue'
import LogMonitor from './views/LogMonitor.vue'
import Diagram from './views/Diagram.vue'
import { onToast, checkAuth, logout, onUnauthorized, changePassword } from './api.js'

const tabs = [
  { id: 'dashboard', label: '仪表盘', icon: '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>' },
  { id: 'hosts', label: '服务器', icon: '<rect x="2" y="2" width="20" height="8" rx="2" ry="2"/><rect x="2" y="14" width="20" height="8" rx="2" ry="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/>' },
  { id: 'logs', label: '日志', icon: '<polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/>' },
  { id: 'diagram', label: '架构图', icon: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>' },
]

const activeTab = ref('dashboard')
const theme = ref(localStorage.getItem('scm-theme') || 'light')
const toasts = ref([])
const authenticated = ref(false)
const showChangePwd = ref(false)
const pwdLoading = ref(false)
const pwdError = ref('')
const pwdSuccess = ref('')
const pwdForm = reactive({ old: '', new1: '', new2: '' })

// Theme
function toggleTheme() {
  theme.value = theme.value === 'dark' ? 'light' : 'dark'
  document.documentElement.setAttribute('data-theme', theme.value)
  localStorage.setItem('scm-theme', theme.value)
}

// Toast system
let toastCleanup
onMounted(() => {
  toastCleanup = onToast((message, type) => {
    toasts.value.push({ message, type })
    setTimeout(() => toasts.value.shift(), 3500)
  })
})
onUnmounted(() => toastCleanup?.())

// 认证：未登录时显示登录页
let unauthCleanup
onMounted(() => {
  checkAuth().then(data => {
    authenticated.value = data.authenticated === true
  })
  unauthCleanup = onUnauthorized(() => {
    authenticated.value = false
  })
})
onUnmounted(() => unauthCleanup?.())

function onLoggedIn() {
  authenticated.value = true
}

async function handleLogout() {
  await logout()
  authenticated.value = false
}

async function submitChangePwd() {
  pwdError.value = ''
  pwdSuccess.value = ''
  if (!pwdForm.old || !pwdForm.new1 || !pwdForm.new2) {
    pwdError.value = '请填写所有字段'
    return
  }
  if (pwdForm.new1 !== pwdForm.new2) {
    pwdError.value = '两次输入的新密码不一致'
    return
  }
  if (pwdForm.new1.length < 4) {
    pwdError.value = '新密码长度不能少于4位'
    return
  }
  pwdLoading.value = true
  try {
    await changePassword(pwdForm.old, pwdForm.new1)
    pwdSuccess.value = '密码修改成功'
    pwdForm.old = ''
    pwdForm.new1 = ''
    pwdForm.new2 = ''
    setTimeout(() => { showChangePwd.value = false; pwdSuccess.value = '' }, 1200)
  } catch (e) {
    pwdError.value = e.message
  } finally {
    pwdLoading.value = false
  }
}
</script>

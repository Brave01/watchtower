<template>
  <div class="login-wrapper">
    <form class="login-card" @submit.prevent="handleLogin">
      <div class="login-logo">S</div>
      <h1 class="login-title">瞭望塔 Watchtower</h1>
      <p class="login-subtitle">服务器控制管理平台</p>
      <div v-if="error" class="login-error">{{ error }}</div>
      <div class="login-field">
        <label for="login-user">账号</label>
        <input id="login-user" v-model.trim="username" type="text" placeholder="admin" autocomplete="username" ref="userInput" />
      </div>
      <div class="login-field">
        <label for="login-pass">密码</label>
        <input id="login-pass" v-model="password" type="password" placeholder="••••••" autocomplete="current-password" />
      </div>
      <button class="login-btn" type="submit" :disabled="logging">
        {{ logging ? '登录中...' : '登录' }}
      </button>
    </form>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { login } from '../api.js'

const emit = defineEmits(['logged-in'])

const username = ref('')
const password = ref('')
const error = ref('')
const logging = ref(false)
const userInput = ref(null)

onMounted(() => {
  userInput.value?.focus()
})

async function handleLogin() {
  error.value = ''
  if (!username.value || !password.value) {
    error.value = '请输入账号和密码'
    return
  }
  logging.value = true
  try {
    await login(username.value, password.value)
    emit('logged-in')
  } catch (e) {
    error.value = e.message
  } finally {
    logging.value = false
  }
}
</script>

<style scoped>
.login-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background: var(--content-bg, #f8fafc);
  padding: 24px;
}
.login-card {
  width: 380px;
  background: var(--card-bg, #ffffff);
  border: 1px solid var(--border-color, #e2e8f0);
  border-radius: 16px;
  padding: 40px 32px 32px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.08);
  text-align: center;
}
.login-logo {
  width: 48px;
  height: 48px;
  margin: 0 auto 16px;
  background: linear-gradient(135deg, #3b82f6, #8b5cf6);
  border-radius: 12px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-weight: 700;
  font-size: 20px;
  color: #fff;
}
.login-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--text-primary, #1e293b);
  margin: 0 0 4px;
}
.login-subtitle {
  font-size: 14px;
  color: var(--text-secondary, #64748b);
  margin: 0 0 28px;
}
.login-error {
  margin-bottom: 16px;
}
.login-field {
  margin-bottom: 18px;
  text-align: left;
}
.login-field label {
  display: block;
  font-size: 13px;
  font-weight: 500;
  color: var(--text-secondary, #64748b);
  margin-bottom: 6px;
}
.login-field input {
  width: 100%;
  padding: 10px 14px;
  border: 1px solid var(--border-color, #e2e8f0);
  border-radius: 8px;
  font-size: 14px;
  background: var(--input-bg, #ffffff);
  color: var(--text-primary, #1e293b);
  transition: border-color 0.15s;
  font-family: 'Inter', sans-serif;
}
.login-field input:focus {
  outline: none;
  border-color: #3b82f6;
  box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
}
.login-btn {
  width: 100%;
  padding: 11px 14px;
  border: none;
  border-radius: 8px;
  background: #3b82f6;
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.15s;
  margin-top: 4px;
}
.login-btn:hover { background: #2563eb; }
.login-btn:disabled { opacity: 0.6; cursor: not-allowed; }
</style>

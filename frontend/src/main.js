import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

// Global error handler for debugging
window.addEventListener('error', (e) => {
  document.body.innerHTML = `<div style="padding:40px;font-family:monospace;white-space:pre-wrap;background:#1a1b26;color:#f7768e;min-height:100vh">
    <h2 style="color:#ff5370">JS Error</h2>
    <div>${e.message || e.error?.message || JSON.stringify(e)}</div>
    <div style="margin-top:12px;color:#9aa5ce;font-size:12px">${e.error?.stack || ''}</div>
  </div>`
  console.error('Global error:', e)
})
window.addEventListener('unhandledrejection', (e) => {
  document.body.innerHTML = `<div style="padding:40px;font-family:monospace;white-space:pre-wrap;background:#1a1b26;color:#f7768e;min-height:100vh">
    <h2 style="color:#ff5370">Unhandled Promise Rejection</h2>
    <div>${e.reason?.message || JSON.stringify(e.reason)}</div>
    <div style="margin-top:12px;color:#9aa5ce;font-size:12px">${e.reason?.stack || ''}</div>
  </div>`
  console.error('Unhandled rejection:', e)
})

const app = createApp(App)
app.config.errorHandler = (err, instance, info) => {
  document.body.innerHTML = `<div style="padding:40px;font-family:monospace;white-space:pre-wrap;background:#1a1b26;color:#f7768e;min-height:100vh">
    <h2 style="color:#ff5370">Vue Error: ${info}</h2>
    <div>${err.message}</div>
    <div style="margin-top:12px;color:#9aa5ce;font-size:12px">${err.stack || ''}</div>
  </div>`
}
app.mount('#app')

// Remove loading overlay
const el = document.getElementById('scm-loading')
if (el) el.classList.add('hidden')

<template>
  <div>
    <div class="tab-header">
      <h1 class="tab-title">仪表盘</h1>
      <p class="tab-subtitle">服务状态概览</p>
    </div>

    <!-- Stats -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(59,130,246,.12);color:#3b82f6">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="2" width="20" height="8" rx="2"/><rect x="2" y="14" width="20" height="8" rx="2"/><line x1="6" y1="6" x2="6.01" y2="6"/><line x1="6" y1="18" x2="6.01" y2="18"/></svg>
        </div>
        <div class="stat-label">总主机数</div>
        <div class="stat-value">{{ stats.total || 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(34,197,94,.12);color:#22c55e">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>
        </div>
        <div class="stat-label">在线</div>
        <div class="stat-value" style="color:#22c55e">{{ stats.online || 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(239,68,68,.12);color:#ef4444">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
        </div>
        <div class="stat-label">离线</div>
        <div class="stat-value" style="color:#ef4444">{{ stats.offline || 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(245,158,11,.12);color:#f59e0b">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
        </div>
        <div class="stat-label">角色异常</div>
        <div class="stat-value" style="color:#f59e0b">{{ stats.role_unhealthy || 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(148,163,184,.12);color:#94a3b8">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><path d="M16 16s-1.5-2-4-2-4 2-4 2"/><line x1="9" y1="9" x2="9.01" y2="9"/><line x1="15" y1="9" x2="15.01" y2="9"/></svg>
        </div>
        <div class="stat-label">告警</div>
        <div class="stat-value" style="color:#94a3b8">{{ stats.alerts || 0 }}</div>
      </div>
    </div>

    <!-- Alerts -->
    <div class="table-wrap" v-if="alertGroups.length > 0">
      <div class="alert-group" v-for="(group,gi) in alertGroups" :key="gi">
        <div class="alert-group-title">{{ group.name }}</div>
        <div class="alert-item" v-for="(item,ii) in group.alerts" :key="ii">
          <div><span :class="'level-tag level-'+item.level">{{ item.level }}</span> <span class="alert-msg">{{ item.message }}</span></div>
          <span class="alert-time">{{ item.time }}</span>
        </div>
      </div>
    </div>
    <div v-else class="empty-state"><p>暂无告警</p></div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api.js'

const stats = ref({ total: 0, online: 0, offline: 0, alerts: 0, role_unhealthy: 0 })
const alertGroups = ref([])
let pollTimer = null

async function load() {
  try {
    const data = await api('/api/dashboard')
    const s = data.stats || {}
    stats.value = {
      total: s.total || 0,
      online: s.healthy || 0,
      offline: s.unhealthy || 0,
      alerts: 0,  // 仪表盘不含告警数据，请查看日志监控
    }
    // 按角色分组的主机列表
    if (data.hosts && data.hosts.length > 0) {
      const byRole = {}
      data.hosts.forEach(hwr => {
        const hostInfo = hwr.host || hwr
        if (hwr.roles && hwr.roles.length > 0) {
          hwr.roles.forEach(a => {
            const roleName = (a.role && a.role.name) || a.role_id || 'unknown'
            if (!byRole[roleName]) byRole[roleName] = []
            byRole[roleName].push({
              message: (hostInfo.hostname || hostInfo.ip || '?') + ' (' + hostInfo.ip + ')',
              time: hostInfo.last_check_time || '',
              level: hwr.is_alive ? 'info' : 'warning',
            })
          })
        }
      })
      alertGroups.value = Object.entries(byRole).map(([name, alerts]) => ({ name, alerts }))
    }
  } catch(e) { /* api() shows toast */ }
}

onMounted(() => {
  load()
  pollTimer = setInterval(load, 15000)
})
onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<template>
  <div>
    <div class="tab-header">
      <h1 class="tab-title">日志监控</h1>
      <p class="tab-subtitle">告警规则与实时日志流</p>
    </div>

    <!-- Stats -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(59,130,246,.12);color:#3b82f6">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
        </div>
        <div class="stat-label">告警规则</div>
        <div class="stat-value">{{ stats.rule_count || 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(139,92,246,.12);color:#8b5cf6">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 12h-4l-3 9L9 3l-3 9H2"/></svg>
        </div>
        <div class="stat-label">总计发送</div>
        <div class="stat-value">{{ stats.total_sent || 0 }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(245,158,11,.12);color:#f59e0b">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>
        </div>
        <div class="stat-label">令牌剩余(分钟/秒)</div>
        <div class="stat-value">{{ stats.rate_limit_remaining ?? '-' }} / {{ stats.rate_limit_remaining_sec ?? '-' }}</div>
      </div>
      <div class="stat-card">
        <div class="stat-icon" style="background:rgba(34,197,94,.12);color:#22c55e">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M23 7l-7 5 7 5V7z"/><rect x="1" y="5" width="15" height="14" rx="2" ry="2"/></svg>
        </div>
        <div class="stat-label">WebSocket 连接</div>
        <div class="stat-value">{{ stats.ws_clients || 0 }}</div>
      </div>
    </div>

    <!-- Actions -->
    <div class="action-bar">
      <button class="btn btn-primary" @click="openAddRule">+ 添加规则</button>
      <button class="btn" @click="openWebhookModal">Webhook 配置</button>
      <button class="btn" @click="testWebhook">测试 Webhook</button>
      <button class="btn" @click="clearLimited">清除限流</button>
      <button class="btn" @click="openESModal">ES 配置</button>
      <span :class="'connection-badge '+(esStatus==='connected'?'connected':'disconnected')" style="margin-left:auto">
        <span class="badge-dot"></span>
        日志服务: {{ esStatus==='connected' ? '已连接' : '未连接' }}
      </span>
    </div>

    <!-- Sub tabs -->
    <div class="sub-tabs">
      <button class="sub-tab" :class="{active:subTab==='rules'}" @click="subTab='rules'">告警规则</button>
      <button class="sub-tab" :class="{active:subTab==='realtime'}" @click="switchToRealtime">实时日志</button>
      <button class="sub-tab" :class="{active:subTab==='limited'}" @click="subTab='limited'">限流缓存</button>
    </div>

    <!-- Rules tab -->
    <div v-if="subTab==='rules'">
      <div class="table-wrap">
        <table>
          <thead><tr><th>规则名称</th><th>关键词</th><th>级别</th><th>冷却(秒)</th><th>状态</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="rule in rules" :key="rule.id">
              <td style="font-weight:600">{{ rule.name }}</td>
              <td><span v-for="(kw,kwi) in (rule.keywords||[])" :key="kwi" class="tag tag-blue" style="margin:1px 3px 1px 0">{{ kw }}</span></td>
              <td><span :class="'level-tag level-'+(rule.level||'info').toLowerCase()">{{ rule.level || 'info' }}</span></td>
              <td>{{ rule.cooldown||0 }}</td>
              <td><span :class="rule.enabled!==false?'tag tag-green':'tag tag-gray'">{{ rule.enabled!==false?'启用':'禁用' }}</span></td>
              <td>
                <div style="display:flex;gap:4px">
                  <button class="btn btn-sm" :style="{color:rule.enabled!==false?'#f44336':'#4caf50',borderColor:rule.enabled!==false?'#f44336':'#4caf50'}" @click="toggleRule(rule)">{{ rule.enabled!==false?'禁用':'启用' }}</button>
                  <button class="btn btn-sm" @click="editRule(rule)">编辑</button>
                  <button class="btn btn-sm btn-danger" @click="deleteRule(rule)">删除</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-if="rules.length===0" class="empty-state"><p>暂无告警规则</p></div>
    </div>

    <!-- Real-time logs tab -->
    <div v-if="subTab==='realtime'">
      <!-- Mini stats row -->
      <div class="stats-grid" style="grid-template-columns:repeat(4,1fr);margin-bottom:12px">
        <div class="stat-card" style="padding:10px 14px">
          <div class="stat-label">最近1分钟日志</div>
          <div class="stat-value" style="font-size:20px">{{ recentLogCount }}</div>
        </div>
        <div class="stat-card" style="padding:10px 14px">
          <div class="stat-label">错误数</div>
          <div class="stat-value" style="font-size:20px;color:#ef4444">{{ errorCount }}</div>
        </div>
        <div class="stat-card" style="padding:10px 14px">
          <div class="stat-label">告警数</div>
          <div class="stat-value" style="font-size:20px;color:#f59e0b">{{ alertCount }}</div>
        </div>
        <div class="stat-card" style="padding:10px 14px">
          <div class="stat-label">匹配规则数</div>
          <div class="stat-value" style="font-size:20px;color:#8b5cf6">{{ matchRuleCount }}</div>
        </div>
      </div>
      <!-- Filters and controls -->
      <div class="log-actions">
        <span :class="'connection-badge '+(wsConnected?'connected':(wsConnecting?'connecting':'disconnected'))">
          <span class="badge-dot"></span>
          {{ wsConnected ? '已连接' : (wsConnecting ? '连接中...' : '未连接') }}
        </span>
        <select class="form-select" v-model="levelFilter" style="width:auto;padding:4px 10px;font-size:12px">
          <option value="">全部</option>
          <option value="info">Info</option>
          <option value="warning">Warning</option>
          <option value="error">Error</option>
          <option value="critical">Critical</option>
          <option value="debug">Debug</option>
        </select>
        <input class="form-input" v-model="sourceFilter" placeholder="过滤来源..." style="width:140px;padding:4px 10px;font-size:12px" />
        <button class="btn btn-sm" @click="clearLogs">清空</button>
        <span style="font-size:12px;color:var(--text-secondary)">共 {{ filteredLogs.length }} / {{ logEvents.length }} 条</span>
        <button class="btn btn-sm" :class="autoScroll?'btn-primary':''" @click="autoScroll=!autoScroll" style="margin-left:auto">
          <svg v-if="autoScroll" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 18 17 24 11 18"/><polyline points="17 24 17 8" style="opacity:.4"/></svg>
          <svg v-else width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 6 17 0 11 6"/><polyline points="17 0 17 16" style="opacity:.4"/></svg>
          {{ autoScroll ? '自动滚动' : '已暂停' }}
        </button>
      </div>
      <div class="log-panel" ref="logPanelRef">
        <div v-for="(e,i) in filteredLogs" :key="i" class="log-entry"
          :class="['log-type-'+e.type, e.isAlert?'log-entry-alert':'', e.ruleName?'log-entry-match':'']"
        >
          <span class="log-time">{{ e.time }}</span>
          <span v-if="e.level" :class="'log-level log-level-'+e.level">{{ e.level }}</span>
          <span v-if="e.source" class="log-source">{{ e.source }}</span>
          <span class="log-message">{{ e.data }}</span>
        </div>
        <div v-if="filteredLogs.length===0" class="empty-state"><p>等待日志数据...</p></div>
      </div>
    </div>

    <!-- Limited alerts tab -->
    <div v-if="subTab==='limited'">
      <div v-for="(g,gIdx) in limitedAlerts" :key="gIdx" class="alert-group">
        <div class="alert-group-title">{{ g.rule_name||'未知规则' }}<span style="font-weight:400;font-size:12px;margin-left:8px;color:var(--text-secondary)">共 {{ (g.alerts||[]).length }} 条</span></div>
        <div style="background:var(--card-bg);border:1px solid var(--border-color);border-radius:8px;overflow:hidden">
          <div v-for="(a,aIdx) in (g.alerts||[])" :key="aIdx" class="alert-item">
            <span class="alert-msg">{{ a.message }}</span>
            <span class="alert-time">{{ a.time }}</span>
          </div>
        </div>
      </div>
      <div v-if="limitedAlerts.length===0" class="empty-state"><p>暂无限流缓存数据</p></div>
    </div>

    <!-- Add/Edit Rule Modal -->
    <div v-if="showRuleModal" class="modal-overlay" @click.self="showRuleModal=false">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <span class="modal-title">{{ editingRule ? '编辑规则' : '添加规则' }}</span>
          <button class="modal-close" @click="showRuleModal=false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">规则名称</label>
            <input class="form-input" v-model="ruleForm.name" placeholder="例如: 错误关键词告警" />
          </div>
          <div class="form-group">
            <label class="form-label">关键词（逗号分隔）</label>
            <input class="form-input" v-model="ruleForm.keywords" placeholder="error, fatal, exception" />
          </div>
          <div class="form-group">
            <label class="form-label">告警级别</label>
            <select class="form-select" v-model="ruleForm.level">
              <option value="info">Info</option>
              <option value="warning">Warning</option>
              <option value="error">Error</option>
              <option value="critical">Critical</option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">冷却时间（秒）</label>
            <input class="form-input" v-model.number="ruleForm.cooldown" type="number" />
          </div>
          <div class="form-group">
            <label class="form-checkbox-label">
              <input class="form-checkbox" type="checkbox" v-model="ruleForm.enabled" /> 启用
            </label>
          </div>
          <div class="form-group" style="grid-column:1/3">
            <label class="form-label">通知 Webhook</label>
            <select class="form-select" v-model="ruleForm.webhook_id">
              <option :value="0">默认 Webhook</option>
              <option v-for="wh in webhookList" :key="wh.id" :value="wh.id">{{ wh.name || wh.platform + '(' + wh.url + ')' }}</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showRuleModal=false">取消</button>
          <button class="btn btn-primary" @click="submitRule">保存</button>
        </div>
      </div>
    </div>

    <!-- Webhook Config Modal -->
    <div v-if="showWebhookModal" class="modal-overlay" @click.self="showWebhookModal=false">
      <div class="modal" @click.stop style="max-width:680px">
        <div class="modal-header">
          <span class="modal-title">Webhook 配置</span>
          <button class="modal-close" @click="showWebhookModal=false">&times;</button>
        </div>
        <div class="modal-body" style="max-height:60vh;overflow-y:auto">
          <!-- Webhook list -->
          <div v-if="webhookList.length > 0" style="margin-bottom:20px">
            <h4 style="margin:0 0 12px;font-size:13px;color:var(--text-secondary)">已配置的 Webhook</h4>
            <div v-for="wh in webhookList" :key="wh.id" class="webhook-card" style="border:1px solid var(--border-color);border-radius:8px;padding:14px;margin-bottom:8px">
              <div style="display:flex;align-items:center;justify-content:space-between;margin-bottom:10px">
                <div style="display:flex;align-items:center;gap:8px">
                  <strong>{{ wh.name || '未命名' }}</strong>
                  <span class="tag tag-blue">{{ wh.platform }}</span>
                  <span :class="wh.enabled!==false?'tag tag-green':'tag tag-gray'">{{ wh.enabled!==false?'启用':'禁用' }}</span>
                </div>
                <div style="display:flex;gap:6px">
                  <button class="btn btn-sm" @click="editWebhook(wh)">编辑</button>
                  <button class="btn btn-sm btn-danger" @click="deleteWebhook(wh)">删除</button>
                </div>
              </div>
              <div style="font-size:12px;color:var(--text-secondary);line-height:1.8">
                <div>URL: {{ wh.url || '-' }}</div>
                <div>限流: {{ wh.rate_limit || 0 }}/分钟 {{ wh.rate_limit_per_second || 0 }}/秒</div>
              </div>
            </div>
          </div>
          <div v-else style="text-align:center;padding:20px 0;color:var(--text-secondary);font-size:13px">暂无 Webhook 配置，请添加</div>

          <!-- Add/Edit form -->
          <div style="border-top:1px solid var(--border-color);padding-top:20px">
            <h4 style="margin:0 0 16px;font-size:14px">{{ editingWebhook ? '编辑 Webhook' : '添加 Webhook' }}</h4>
            <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px">
              <div class="form-group">
                <label class="form-label">名称</label>
                <input class="form-input" v-model="whForm.name" placeholder="例如: 飞书告警群" />
              </div>
              <div class="form-group">
                <label class="form-label">平台类型</label>
                <select class="form-select" v-model="whForm.platform">
                  <option value="feishu">飞书</option>
                  <option value="dingtalk">钉钉</option>
                  <option value="wechat">企业微信</option>
                  <option value="custom">自定义</option>
                </select>
              </div>
              <div class="form-group" style="grid-column:1/3">
                <label class="form-label">Webhook URL</label>
                <input class="form-input" v-model="whForm.url" placeholder="https://hooks.example.com/webhook" />
              </div>
              <div class="form-group">
                <label class="form-label">Secret（签名密钥）</label>
                <input class="form-input" v-model="whForm.secret" placeholder="可选" />
              </div>
              <div class="form-group">
                <label class="form-label">@提及</label>
                <select class="form-select" v-model="whForm.mention_type">
                  <option value="none">不@</option>
                  <option value="all">@所有人</option>
                  <option value="specific">@特定人员</option>
                </select>
              </div>
              <div v-if="whForm.mention_type==='specific'" class="form-group" style="grid-column:1/3">
                <label class="form-label">@人员（open_id，多个用逗号分隔）</label>
                <input class="form-input" v-model="whForm.mention_users" placeholder="ou_xxx1, ou_xxx2" />
              </div>
              <div class="form-group" style="display:flex;gap:12px">
                <div style="flex:1">
                  <label class="form-label">限流（每分钟）</label>
                  <input class="form-input" v-model.number="whForm.rate_limit" type="number" placeholder="0=不限" />
                </div>
                <div style="flex:1">
                  <label class="form-label">限流（每秒）</label>
                  <input class="form-input" v-model.number="whForm.rate_limit_per_second" type="number" placeholder="0=不限" />
                </div>
              </div>
              <div class="form-group" style="display:flex;align-items:flex-end;padding-bottom:4px">
                <label class="form-checkbox-label">
                  <input class="form-checkbox" type="checkbox" v-model="whForm.enabled" /> 启用
                </label>
              </div>
            </div>
            <div class="form-group" style="margin-top:4px">
              <label class="form-label">告警模板</label>
              <textarea class="form-input" v-model="whForm.template" rows="3" placeholder="自定义告警消息模板（留空使用默认）"></textarea>
            </div>
            <div style="display:flex;align-items:center;gap:8px;margin-top:8px">
              <button class="btn btn-sm" @click="testWebhookTemplate">测试模板</button>
              <span v-if="whTestResult" style="font-size:12px;color:var(--text-secondary)">{{ whTestResult }}</span>
            </div>
            <div style="margin-top:16px;display:flex;gap:8px;align-items:center;border-top:1px solid var(--border-color);padding-top:16px">
              <button class="btn btn-primary" @click="saveWebhook">{{ editingWebhook ? '更新' : '添加' }}</button>
              <button v-if="editingWebhook" class="btn" @click="cancelEditWebhook">取消编辑</button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- ES Config Modal -->
    <div v-if="showESModal" class="modal-overlay" @click.self="showESModal=false">
      <div class="modal" @click.stop style="max-width:480px">
        <div class="modal-header">
          <span class="modal-title">ES 日志服务配置</span>
          <span :class="'connection-badge '+(esStatus==='connected'?'connected':'disconnected')">
            <span class="badge-dot"></span>
            {{ esStatus==='connected' ? '已连接' : '未连接' }}
          </span>
          <button class="modal-close" @click="showESModal=false">&times;</button>
        </div>
        <div class="modal-body">
          <div v-if="!esConfig" style="text-align:center;color:var(--text-secondary);padding:16px 0">加载中...</div>
          <template v-else>
            <div class="form-group">
              <label class="form-label">ES 地址</label>
              <input class="form-input" v-model="esConfig.address" placeholder="http://10.0.0.1:9200" />
            </div>
            <div class="form-group">
              <label class="form-label">用户名</label>
              <input class="form-input" v-model="esConfig.username" placeholder="elastic" />
            </div>
            <div class="form-group">
              <label class="form-label">密码</label>
              <input class="form-input" v-model="esConfig.password" type="password" placeholder="密码" />
            </div>
            <div class="form-group">
              <label class="form-label">索引名</label>
              <input class="form-input" v-model="esConfig.index" placeholder="logs-*" />
            </div>
            <div class="form-group">
              <label class="form-label">轮询间隔（秒）</label>
              <input class="form-input" v-model.number="esConfig.interval" type="number" />
            </div>
            <div class="form-group">
              <label class="form-label">每次查询最大日志数</label>
              <input class="form-input" v-model.number="esConfig.size" type="number" placeholder="默认 100" />
            </div>
            <div class="form-group">
              <label class="form-checkbox-label">
                <input class="form-checkbox" type="checkbox" v-model="esConfig.enabled" /> 启用日志轮询
              </label>
            </div>
          </template>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showESModal=false">取消</button>
          <button class="btn btn-primary" @click="saveESConfig" :disabled="!esConfig">保存</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { api, showToast } from '../api.js'

const subTab = ref('rules')
const rules = ref([])
const limitedAlerts = ref([])
const stats = ref({})
const logEvents = ref([])
const logPanelRef = ref(null)

// WebSocket
const wsConnected = ref(false)
const wsConnecting = ref(false)
let ws = null

// Log filters & controls
const levelFilter = ref('')
const sourceFilter = ref('')
const autoScroll = ref(true)

// Computed: filtered logs
const filteredLogs = computed(() => {
  let list = logEvents.value
  if (levelFilter.value) {
    list = list.filter(e => e.level === levelFilter.value)
  }
  if (sourceFilter.value) {
    const q = sourceFilter.value.toLowerCase()
    list = list.filter(e => (e.source || '').toLowerCase().includes(q))
  }
  return list
})

// Computed: stats from logEvents
const recentLogCount = computed(() => {
  const cutoff = Date.now() - 60000
  return logEvents.value.filter(e => e.ts && e.ts >= cutoff).length
})
const errorCount = computed(() => {
  return logEvents.value.filter(e => e.level === 'error' || e.level === 'critical').length
})
const alertCount = computed(() => {
  return logEvents.value.filter(e => e.isAlert).length
})
const matchRuleCount = computed(() => {
  const rules = new Set(logEvents.value.filter(e => e.ruleName).map(e => e.ruleName))
  return rules.size
})

// Rule modal
const showRuleModal = ref(false)
const editingRule = ref(null)
const ruleForm = ref({ name: '', keywords: '', level: 'error', cooldown: 60, enabled: true, webhook_id: 0 })

// Webhook management
const showWebhookModal = ref(false)
const webhookList = ref([])
const editingWebhook = ref(null)
const webhookUrl = ref('')
const webhookType = ref('feishu')
const whForm = ref({ name: '', platform: 'feishu', url: '', secret: '', enabled: true, rate_limit: 0, rate_limit_per_second: 0, template: '' })
const whTestResult = ref('')

// ES Config
const showESModal = ref(false)
const esStatus = ref('disconnected')
const esConfig = ref(null)

let pollTimer = null

const PWD_PLACEHOLDER = '__PWD_PLACEHOLDER__'

async function loadESConfig() {
  try {
    const data = await api('/api/es/config')
    esStatus.value = data.status || 'disconnected'
    if (data.config) {
      esConfig.value = { ...data.config, password: PWD_PLACEHOLDER }
    } else {
      esConfig.value = { address: '', username: '', password: '', index: 'logs-*', interval: 15, size: 100, query: '', enabled: false }
    }
  } catch(e) {
    esStatus.value = 'disconnected'
  }
}

// 仅刷新 ES 连接状态，不覆盖配置（避免编辑时被轮询覆盖）
async function loadESStatus() {
  try {
    const data = await api('/api/es/config')
    esStatus.value = data.status || 'disconnected'
  } catch(e) {
    esStatus.value = 'disconnected'
  }
}

async function openESModal() {
  await loadESConfig()
  showESModal.value = true
}

async function saveESConfig() {
  if (!esConfig.value) return
  const body = { ...esConfig.value }
  // 密码未改动，不发送 password 字段（后端保留已有密码）
  if (body.password === PWD_PLACEHOLDER) {
    delete body.password
  }
  try {
    const resp = await api('/api/es/config', {
      method: 'POST',
      body: JSON.stringify(body)
    })
    showToast(resp.message || '配置已保存', resp.success !== false ? 'success' : 'error')
    if (resp.success !== false) {
      await loadESConfig()
      showESModal.value = false
    }
  } catch(e) { showToast('保存失败: ' + e.message, 'error') }
}

async function loadStats() {
  try {
    const data = await api('/api/stats')
    const rl = data.rate_limit
    const fmtRemaining = (v) => {
      if (v === undefined || v === null) return '-'
      if (v === 0 && rl && rl.limit_per_minute === 0) return '无限制'
      return Math.floor(v)
    }
    stats.value = {
      rule_count: data.rule_count || 0,
      ws_clients: data.ws_clients || 0,
      rate_limit_remaining: rl ? fmtRemaining(rl.remaining_minute) : '-',
      rate_limit_remaining_sec: rl ? fmtRemaining(rl.remaining_second) : '-',
      total_sent: rl?.total_sent || 0,
    }
  } catch(e) {}
}

async function loadRules() {
  try {
    const data = await api('/api/rules')
    rules.value = data.rules || []
  } catch(e) {}
}

async function loadLimited() {
  try {
    const data = await api('/api/webhook/limited-alerts')
    limitedAlerts.value = data.alerts || []
  } catch(e) {}
}

// WebSocket
function connectWS() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return
  wsConnecting.value = true
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  try {
    ws = new WebSocket(proto + '//' + location.host + '/ws')
    ws.onopen = () => { wsConnected.value = true; wsConnecting.value = false }
    ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        const now = new Date()
        const pad = n => String(n).padStart(2, '0')
        const time = `${now.getFullYear()}/${pad(now.getMonth()+1)}/${pad(now.getDate())} ${pad(now.getHours())}:${pad(now.getMinutes())}:${pad(now.getSeconds())}`
        const ts = now.getTime()
        const type = msg.type || 'info'
        let level = ''
        let source = ''
        let displayData = ''
        let isAlert = false
        let ruleName = ''
        if (msg.data && typeof msg.data === 'object') {
          const d = msg.data
          // FilteredLog 格式（log_match）
          if (d.parsed_log) {
            const pl = d.parsed_log
            level = (pl.level || '').toLowerCase()
            source = pl.source || ''
            ruleName = d.rule_name || ''
            isAlert = d.is_alert || false
            const tag = ruleName ? '[' + ruleName + '] ' : ''
            displayData = (isAlert ? '⚠ ' : '') + tag + (pl.message || pl.raw || '')
          } else {
            level = (d.level || '').toLowerCase()
            source = d.source || ''
            displayData = d.message || d.raw || JSON.stringify(d)
          }
        } else {
          displayData = msg.data || msg.message || JSON.stringify(msg)
        }
        logEvents.value.push({ ts, time, type, level, source, data: displayData, isAlert, ruleName })
        if (logEvents.value.length > 500) logEvents.value = logEvents.value.slice(-500)
        if (autoScroll.value) {
          nextTick(() => { if (logPanelRef.value) logPanelRef.value.scrollTop = logPanelRef.value.scrollHeight })
        }
      } catch(e) {
        const now2 = new Date()
        const pad2 = n => String(n).padStart(2, '0')
        logEvents.value.push({ ts: Date.now(), time: `${now2.getFullYear()}/${pad2(now2.getMonth()+1)}/${pad2(now2.getDate())} ${pad2(now2.getHours())}:${pad2(now2.getMinutes())}:${pad2(now2.getSeconds())}`, type: 'raw', level: '', source: '', data: e.data, isAlert: false, ruleName: '' })
      }
    }
    ws.onclose = () => { wsConnected.value = false; wsConnecting.value = false }
    ws.onerror = () => { wsConnected.value = false; wsConnecting.value = false }
  } catch(e) {
    wsConnecting.value = false
  }
}

function disconnectWS() {
  if (ws) { try { ws.close() } catch(e) {} ws = null }
  wsConnected.value = false
  wsConnecting.value = false
}

function switchToRealtime() {
  subTab.value = 'realtime'
  connectWS()
}

function clearLogs() { logEvents.value = [] }

// Rule CRUD
async function openAddRule() {
  editingRule.value = null
  ruleForm.value = { name: '', keywords: '', level: 'error', cooldown: 60, enabled: true, webhook_id: 0 }
  await loadWebhookList()
  showRuleModal.value = true
}

async function editRule(rule) {
  editingRule.value = rule
  let kw = []
  try { kw = JSON.parse(rule.keywords || '[]') } catch(e) { kw = (rule.keywords || '').split(',').map(s => s.trim()).filter(Boolean) }
  ruleForm.value = {
    name: rule.name,
    keywords: kw.join(', '),
    level: rule.level || 'error',
    cooldown: rule.cooldown || 60,
    enabled: rule.enabled !== false,
    webhook_id: rule.webhook_id || 0,
  }
  await loadWebhookList()
  showRuleModal.value = true
}

async function submitRule() {
  const keywordsArr = ruleForm.value.keywords.split(',').map(s => s.trim()).filter(Boolean)
  const body = {
    name: ruleForm.value.name,
    keywords: JSON.stringify(keywordsArr),
    level: ruleForm.value.level,
    cooldown: ruleForm.value.cooldown,
    enabled: ruleForm.value.enabled,
    webhook_id: ruleForm.value.webhook_id,
  }
  if (editingRule.value) {
    await api('/api/rules/update?id=' + editingRule.value.id, { method: 'POST', body: JSON.stringify(body) })
  } else {
    await api('/api/rules/update', { method: 'POST', body: JSON.stringify(body) })
  }
  showRuleModal.value = false
  showToast('规则已保存', 'success')
  await loadRules()
  await loadStats()
}

async function toggleRule(rule) {
  const enabled = rule.enabled !== false ? false : true
  await api('/api/rules/update?id=' + rule.id, { method: 'POST', body: JSON.stringify({ enabled }) })
  showToast(rule.enabled !== false ? '规则已禁用' : '规则已启用', 'success')
  await loadRules()
  await loadStats()
}

async function deleteRule(rule) {
  if (!confirm('确定删除规则 "' + rule.name + '" 吗？')) return
  await api('/api/rules/delete?id=' + rule.id, { method: 'POST' })
  showToast('规则已删除', 'success')
  await loadRules()
  await loadStats()
}

// Webhook management
async function loadWebhookList() {
  try {
    const data = await api('/api/webhook/config')
    webhookList.value = data.webhooks || []
  } catch(e) { webhookList.value = [] }
}

async function openWebhookModal() {
  await loadWebhookList()
  cancelEditWebhook()
  showWebhookModal.value = true
}

function cancelEditWebhook() {
  editingWebhook.value = null
  whForm.value = { name: '', platform: 'feishu', url: '', secret: '', mention_type: 'none', mention_users: '', enabled: true, rate_limit: 0, rate_limit_per_second: 0, template: '' }
  whTestResult.value = ''
}

function editWebhook(wh) {
  editingWebhook.value = wh
  whForm.value = {
    name: wh.name || '',
    platform: wh.platform || 'feishu',
    url: wh.url || '',
    secret: wh.secret || '',
    mention_type: wh.mention_type || 'none',
    mention_users: wh.mention_users || '',
    enabled: wh.enabled !== false,
    rate_limit: wh.rate_limit || 0,
    rate_limit_per_second: wh.rate_limit_per_second || 0,
    template: wh.template || '',
  }
  whTestResult.value = ''
}

async function saveWebhook() {
  if (!whForm.value.name) { showToast('请输入 Webhook 名称', 'error'); return }
  if (!whForm.value.url) { showToast('请输入 Webhook URL', 'error'); return }
  const body = { ...whForm.value }
  if (editingWebhook.value) {
    body.id = editingWebhook.value.id
  }
  const resp = await api('/api/webhook/config', { method: 'POST', body: JSON.stringify(body) })
  showToast(editingWebhook.value ? 'Webhook 已更新' : 'Webhook 已添加', 'success')
  await openWebhookModal()
}

async function deleteWebhook(wh) {
  if (!confirm('确定删除 Webhook "' + (wh.name || wh.url) + '" 吗？')) return
  await api('/api/webhook/config?id=' + wh.id, { method: 'DELETE' })
  showToast('Webhook 已删除', 'success')
  await openWebhookModal()
}

async function testWebhookTemplate() {
  if (!whForm.value.template) { showToast('请先填写模板内容', 'error'); return }
  whTestResult.value = '发送中...'
  try {
    const resp = await api('/api/webhook/test', { method: 'POST', body: JSON.stringify({ template: whForm.value.template, platform: whForm.value.platform }) })
    whTestResult.value = resp.message || '测试消息已发送'
  } catch(e) {
    whTestResult.value = '发送失败: ' + e.message
  }
}

async function testWebhook() {
  await api('/api/webhook/test', { method: 'POST' })
  showToast('测试消息已发送', 'success')
}

async function clearLimited() {
  await api('/api/webhook/limited-alerts/clear', { method: 'POST' })
  showToast('限流缓存已清除', 'success')
  await loadLimited()
}

onMounted(() => {
  loadStats()
  loadRules()
  loadLimited()
  loadESConfig()
  pollTimer = setInterval(() => { loadStats(); loadLimited(); loadESStatus() }, 10000)
})
onUnmounted(() => {
  disconnectWS()
  if (pollTimer) clearInterval(pollTimer)
})
</script>

<style scoped>
.log-entry-alert {
  background: rgba(239,68,68,.12) !important;
  border-left: 3px solid #ef4444;
}
.log-entry-alert:hover {
  background: rgba(239,68,68,.18) !important;
}
.log-entry-match {
  border-left: 3px solid #3b82f6;
}
.log-entry-match.log-entry-alert {
  border-left-color: #ef4444;
}
</style>

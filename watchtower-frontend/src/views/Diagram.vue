<template>
  <div :class="'diagram-mode-' + mode">
    <div class="tab-header">
      <h1 class="tab-title">架构图</h1>
      <p class="tab-subtitle">服务拓扑与依赖关系可视化</p>
    </div>

    <!-- Toolbar -->
    <div class="diagram-toolbar">
      <button class="btn" :class="{'btn-primary':mode==='select'}" @click="setMode('select')">选择</button>
      <button class="btn" :class="{'btn-primary':mode==='connect'}" @click="setMode('connect')">连线</button>
      <button class="btn" :class="{'btn-primary':mode==='group'}" @click="setMode('group')">分组框</button>
      <button class="btn" :class="{'btn-primary':mode==='text'}" @click="setMode('text')">文字</button>
      <button class="btn" :class="{'btn-primary':mode==='pan'}" @click="setMode('pan')">平移</button>
      <div style="flex:1"></div>
      <button class="btn" @click="autoLayout">自动布局</button>
      <button class="btn" @click="save">保存</button>
      <button class="btn" @click="clearCanvas">清除</button>
      <button class="btn" @click="exportImage">PDF</button>
    </div>

    <!-- Palette: Server nodes from API -->
    <div class="diagram-palette">
      <span class="palette-label">服务器节点:</span>
      <span v-for="svr in serverNodes" :key="svr.id" class="palette-item palette-server" draggable="true" @dragstart="onDragServerStart($event, svr)">
        {{ svr.data.label }}
      </span>
      <span v-if="serverNodes.length===0" style="color:#94a3b8;font-size:12px">暂无服务器</span>
    </div>
    <!-- Palette: Custom templates -->
    <div class="diagram-palette" style="margin-top:4px">
      <span class="palette-label">自定义:</span>
      <span v-for="tpl in customTemplates" :key="tpl.label" class="palette-item" draggable="true" @dragstart="onDragStart($event, tpl)">{{ tpl.label }}</span>
    </div>

    <!-- Mode hint -->
    <div v-if="mode==='text'" class="mode-hint">点击画布空白处添加文字标注</div>
    <div v-else-if="mode==='group'" class="mode-hint">点击画布空白处添加分组框，右键分组框可切换虚实线</div>
    <div v-else-if="mode==='connect'" class="mode-hint">点击源节点连接点 → 点击目标节点连接点</div>
    <div v-else-if="mode==='select'" class="mode-hint">选中节点/线段后按 Delete 键删除，或右键菜单操作</div>

    <!-- Vue Flow Canvas -->
    <div class="diagram-canvas-wrapper">
      <VueFlow
        ref="flowRef"
        v-model:nodes="nodes"
        v-model:edges="edges"
        :default-viewport="{ x: 0, y: 0, zoom: 1 }"
        :fit-view-on-init="true"
        :min-zoom="0.1"
        :max-zoom="2"
        :node-click-distance="5"
        :connection-mode="connectionMode"
        :nodes-draggable="nodesDraggable"
        :nodes-focusable="true"
        :edges-focusable="true"
        :edges-updatable="true"
        :snap-to-grid="true"
        :snap-grid="[20,20]"
        :delete-key-code="'Delete'"
        :multi-selection-key-code="'Shift'"
        @connect="onConnect"
        @edge-click="onEdgeClick"
        @pane-click="onPaneClick"
        @nodes-initialized="onNodesInit"
      >
        <Background :gap="20" size="1" :color="theme==='dark'?'#334155':'#e2e8f0'"/>
        <Controls position="bottom-right" :show-zoom="true" :show-fit-view="true" :show-interactive="true"/>
        <template #node-custom="nodeProps">
          <ServerNode v-bind="nodeProps" @contextmenu.prevent.stop="openNodeMenu($event, nodeProps)" />
        </template>
        <template #node-textlabel="nodeProps">
          <div class="text-label-node" :class="{selected:nodeProps.selected}" @dblclick="editTextLabel(nodeProps)" @contextmenu.prevent.stop="openLabelMenu($event, nodeProps)">
            <div v-if="editingLabelId===nodeProps.id" class="text-label-editing">
              <textarea v-model="editLabelText" ref="textInput" rows="2" @blur="commitTextEdit(nodeProps)" @keydown.enter.prevent="commitTextEdit(nodeProps)" @keydown.esc.prevent="cancelTextEdit" />
            </div>
            <div v-else class="text-label-display" :style="{color:nodeProps.data?.color||'#334155',fontSize:(nodeProps.data?.fontSize||14)+'px'}">{{ nodeProps.data?.text||nodeProps.data?.label||'文字' }}</div>
          </div>
        </template>
        <template #node-groupbox="nodeProps">
          <div class="groupbox-node"
            :class="{selected:nodeProps.selected}"
            :style="{
              width: (nodeProps.data?.width || 240) + 'px',
              height: (nodeProps.data?.height || 120) + 'px',
              background: nodeProps.data?.fillColor || 'rgba(148,163,184,.06)',
              borderColor: nodeProps.data?.borderColor || '#94a3b8',
              borderStyle: nodeProps.data?.dashedBorder !== false ? 'dashed' : 'solid',
            }"
            @dblclick="editGroupBoxLabel(nodeProps)"
            @contextmenu.prevent.stop="openGroupBoxMenu($event, nodeProps)">
            <div class="groupbox-label">{{ nodeProps.data?.label||'分组' }}</div>
            <div v-if="nodeProps.selected" class="resize-handle" @mousedown.stop="startResize($event, nodeProps)"></div>
          </div>
        </template>
        <!-- Edge context menu is handled via onNodesInit event delegation -->
      </VueFlow>
    </div>

    <!-- Context menu (unified: node / label / edge) -->
    <Teleport to="body">
      <div v-if="contextMenu.show" class="context-menu" :style="{left:contextMenu.x+'px',top:contextMenu.y+'px',position:'fixed',zIndex:9999}">
        <!-- Edge menu -->
        <template v-if="contextMenu.type==='edge'">
          <div class="context-menu-item" @click="deleteEdge">删除线段</div>
          <div class="context-menu-separator" />
          <div class="context-menu-item" @click="toggleEdgeDashed">{{ contextMenu.dashed ? '实线' : '虚线' }}</div>
          <div class="context-menu-item" @click="toggleEdgeArrow">{{ contextMenu.arrow ? '移除箭头' : '添加箭头' }}</div>
          <div class="context-menu-separator" />
          <div class="context-menu-item" @click="cycleEdgeType">路线: {{ contextMenu.edgeType }}</div>
          <div class="context-menu-separator" />
          <div class="context-menu-item" @click="contextMenu.show=false">取消</div>
        </template>
        <!-- Node (server) menu -->
        <template v-else-if="contextMenu.type==='node'">
          <div class="context-menu-item" @click="deleteNode">删除服务模块</div>
          <div class="context-menu-separator" />
          <div class="context-menu-item" @click="contextMenu.show=false">取消</div>
        </template>
        <!-- Label (text) menu -->
        <template v-else-if="contextMenu.type==='label'">
          <div class="context-menu-item" @click="deleteNode">删除文字</div>
          <div class="context-menu-item" @click="changeTextColor">切换颜色</div>
          <div class="context-menu-separator" />
          <div class="context-menu-item" @click="contextMenu.show=false">取消</div>
        </template>
        <!-- Groupbox menu -->
        <template v-else-if="contextMenu.type==='groupbox'">
          <div class="context-menu-item" @click="deleteNode">删除分组框</div>
          <div class="context-menu-item" @click="toggleGroupBoxDashed">{{ contextMenu.dashed ? '实线' : '虚线' }}</div>
          <div class="context-menu-item" @click="cycleGroupBoxFillColor">背景颜色</div>
          <div class="context-menu-item" @click="cycleGroupBoxBorderColor">边框颜色</div>
          <div class="context-menu-separator" />
          <div class="context-menu-item" @click="contextMenu.show=false">取消</div>
        </template>
      </div>
    </Teleport>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { VueFlow, useVueFlow, MarkerType } from '@vue-flow/core'
import { Background } from '@vue-flow/background'
import { Controls } from '@vue-flow/controls'
import { api, showToast } from '../api.js'
import ServerNode from '../components/ServerNode.vue'

const theme = ref(localStorage.getItem('scm-theme') || 'light')
const borderColor = computed(() => theme.value === 'dark' ? '#334155' : '#e2e8f0')

const flowRef = ref(null)
const nodes = ref([])
const edges = ref([])
const mode = ref(localStorage.getItem('scm-diagram-mode') || 'select')
const { fitView, screenToFlowCoordinate } = useVueFlow({ id: 'default' })

const connectionMode = computed(() => {
  if (mode.value === 'connect') return 'connect'
  return 'loose'
})
const nodesDraggable = computed(() => mode.value === 'select')

// Context menu
const contextMenu = ref({ show: false, x: 0, y: 0, type: 'node', id: null, edgeData: null, dashed: false, arrow: true, edgeType: 'smoothstep' })
function resetContextMenu() {
  contextMenu.value = { show: false, x: 0, y: 0, type: 'node', id: null, edgeData: null, dashed: false, arrow: true, edgeType: 'smoothstep' }
}

// Text label editing
const editingLabelId = ref(null)
const editLabelText = ref('')
const textInput = ref(null)

// Templates
const customTemplates = [
  { label: '新服务器', type: 'custom', data: { label: '新服务器', ip: '', role: 'worker', status: 'offline' } },
  { label: '数据库', type: 'custom', data: { label: '数据库', ip: '', role: 'database', status: 'offline' } },
  { label: '负载均衡', type: 'custom', data: { label: 'LB', ip: '', role: 'loadbalancer', status: 'offline' } },
]
const serverNodes = ref([]) // palette items for server nodes

let idCounter = 100
let hostData = []

// ---- Palette drag ----
function onDragStart(event, tpl) {
  event.dataTransfer.setData('application/vueflow', JSON.stringify(tpl))
  event.dataTransfer.effectAllowed = 'move'
}
function onDragServerStart(event, svr) {
  event.dataTransfer.setData('application/vueflow', JSON.stringify({ type: 'custom', data: svr.data }))
  event.dataTransfer.effectAllowed = 'move'
}

// ---- Connection ----
function onConnect(connection) {
  if (!connection.source || !connection.target) return
  if (connection.source === connection.target) return
  // 检查完全相同的连接（含 Handle ID），不同 Handle 之间允许多条线段
  const dup = edges.value.some(e =>
    e.source === connection.source &&
    e.target === connection.target &&
    e.sourceHandle === connection.sourceHandle &&
    e.targetHandle === connection.targetHandle
  )
  if (dup) return
  edges.value = [...edges.value, {
    id: 'e-' + connection.source + '-' + connection.target + '-' + Date.now(),
    source: connection.source,
    target: connection.target,
    sourceHandle: connection.sourceHandle,
    targetHandle: connection.targetHandle,
    type: 'smoothstep',
    animated: false,
    style: { stroke: '#94a3b8', strokeWidth: 2 },
    markerEnd: { type: MarkerType.ArrowClosed, color: '#94a3b8', width: 20, height: 20 },
    label: '',
  }]
}

// ---- Edge click: toggle dashed/solid ----
// Vue Flow @edge-click passes (edgeId: string, event: MouseEvent)
function onEdgeClick(edgeId, event) {
  const idx = edges.value.findIndex(e => e.id === edgeId)
  if (idx === -1) return
  const e = edges.value[idx]
  const currentDashed = !!e.style?.strokeDasharray
  const newStyle = { ...e.style, strokeDasharray: currentDashed ? 'none' : '5,5' }
  const newEdge = { ...e, style: newStyle }
  edges.value = [...edges.value.slice(0, idx), newEdge, ...edges.value.slice(idx + 1)]
  showToast(currentDashed ? '已切换为实线' : '已切换为虚线', 'info')
}

// ---- Edge right-click menu ----
function openEdgeMenu(event, edge) {
  event.preventDefault()
  const dashed = !!edge.style?.strokeDasharray
  const arrow = !!edge.markerEnd
  contextMenu.value = {
    show: true, x: event.clientX, y: event.clientY,
    type: 'edge', id: edge.id, edgeData: edge,
    dashed, arrow, edgeType: edge.type || 'smoothstep',
  }
}
function openGroupBoxMenu(event, nodeProps) {
  contextMenu.value = {
    show: true, x: event.clientX, y: event.clientY,
    type: 'groupbox', id: nodeProps.id, edgeData: null,
    dashed: nodeProps.data?.dashedBorder !== false, arrow: true, edgeType: 'smoothstep',
  }
}

// ---- Node right-click menu ----
function openNodeMenu(event, nodeProps) {
  contextMenu.value = {
    show: true, x: event.clientX, y: event.clientY,
    type: 'node', id: nodeProps.id, edgeData: null, dashed: false, arrow: true,
  }
}
function openLabelMenu(event, nodeProps) {
  contextMenu.value = {
    show: true, x: event.clientX, y: event.clientY,
    type: 'label', id: nodeProps.id, edgeData: null, dashed: false, arrow: true,
  }
}

// ---- Context menu actions ----
function deleteNode() {
  if (contextMenu.value.id) {
    const id = contextMenu.value.id
    nodes.value = nodes.value.filter(n => n.id !== id)
    edges.value = edges.value.filter(e => e.source !== id && e.target !== id)
  }
  resetContextMenu()
}
function deleteEdge() {
  if (contextMenu.value.id) {
    edges.value = edges.value.filter(e => e.id !== contextMenu.value.id)
  }
  resetContextMenu()
}
function toggleEdgeDashed() {
  if (!contextMenu.value.id) return
  const idx = edges.value.findIndex(e => e.id === contextMenu.value.id)
  if (idx === -1) return
  const e = edges.value[idx]
  const newDashed = !contextMenu.value.dashed
  edges.value[idx] = { ...e, style: { ...e.style, strokeDasharray: newDashed ? '5,5' : 'none' } }
  edges.value = [...edges.value]
  contextMenu.value.show = false
}
function toggleEdgeArrow() {
  if (!contextMenu.value.id) return
  const idx = edges.value.findIndex(e => e.id === contextMenu.value.id)
  if (idx === -1) return
  const e = edges.value[idx]
  const newArrow = !contextMenu.value.arrow
  edges.value[idx] = {
    ...e,
    markerEnd: newArrow
      ? { type: MarkerType.ArrowClosed, color: e.style?.stroke || '#94a3b8', width: 20, height: 20 }
      : undefined,
  }
  edges.value = [...edges.value]
  contextMenu.value.show = false
}
// Cycle edge type: smoothstep → bezier → straight → step → smoothstep
const EDGE_TYPES = ['smoothstep', 'bezier', 'straight', 'step']
function cycleEdgeType() {
  if (!contextMenu.value.id) return
  const idx = edges.value.findIndex(e => e.id === contextMenu.value.id)
  if (idx === -1) return
  const e = edges.value[idx]
  const curType = contextMenu.value.edgeType
  const nextIdx = (EDGE_TYPES.indexOf(curType) + 1) % EDGE_TYPES.length
  const newType = EDGE_TYPES[nextIdx]
  edges.value[idx] = { ...e, type: newType }
  edges.value = [...edges.value]
  contextMenu.value.edgeType = newType
  showToast('路线: ' + newType, 'info')
}
function toggleGroupBoxDashed() {
  const id = contextMenu.value.id
  if (!id) return
  const node = nodes.value.find(n => n.id === id)
  if (!node) return
  const newDashed = !contextMenu.value.dashed
  node.data = { ...node.data, dashedBorder: newDashed }
  nodes.value = [...nodes.value]
  contextMenu.value.show = false
}

const GROUPBOX_FILL_COLORS = [
  'rgba(148,163,184,.06)', 'rgba(59,130,246,.08)', 'rgba(239,68,68,.08)',
  'rgba(34,197,94,.08)', 'rgba(245,158,11,.08)', 'rgba(139,92,246,.08)',
]
function cycleGroupBoxFillColor() {
  const id = contextMenu.value.id
  if (!id) return
  const node = nodes.value.find(n => n.id === id)
  if (!node) return
  const cur = node.data?.fillColor || GROUPBOX_FILL_COLORS[0]
  const idx = GROUPBOX_FILL_COLORS.indexOf(cur)
  node.data = { ...node.data, fillColor: GROUPBOX_FILL_COLORS[(idx + 1) % GROUPBOX_FILL_COLORS.length] }
  nodes.value = [...nodes.value]
  contextMenu.value.show = false
}

const GROUPBOX_BORDER_COLORS = ['#94a3b8', '#3b82f6', '#ef4444', '#22c55e', '#f59e0b', '#8b5cf6']
function cycleGroupBoxBorderColor() {
  const id = contextMenu.value.id
  if (!id) return
  const node = nodes.value.find(n => n.id === id)
  if (!node) return
  const cur = node.data?.borderColor || GROUPBOX_BORDER_COLORS[0]
  const idx = GROUPBOX_BORDER_COLORS.indexOf(cur)
  node.data = { ...node.data, borderColor: GROUPBOX_BORDER_COLORS[(idx + 1) % GROUPBOX_BORDER_COLORS.length] }
  nodes.value = [...nodes.value]
  contextMenu.value.show = false
}

let resizeState = null
function startResize(event, nodeProps) {
  const node = nodes.value.find(n => n.id === nodeProps.id)
  if (!node) return
  const startX = event.clientX
  const startY = event.clientY
  const startW = node.data?.width || 240
  const startH = node.data?.height || 120
  resizeState = { nodeId: nodeProps.id, startX, startY, startW, startH }

  function onMouseMove(e) {
    if (!resizeState) return
    const n = nodes.value.find(n => n.id === resizeState.nodeId)
    if (!n) return
    const newW = Math.max(160, resizeState.startW + (e.clientX - resizeState.startX))
    const newH = Math.max(80, resizeState.startH + (e.clientY - resizeState.startY))
    n.data = { ...n.data, width: Math.round(newW), height: Math.round(newH) }
    nodes.value = [...nodes.value]
  }
  function onMouseUp() {
    resizeState = null
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

function changeTextColor() {
  const id = contextMenu.value.id
  if (!id) return
  const colors = ['#334155', '#3b82f6', '#ef4444', '#22c55e', '#f59e0b', '#8b5cf6']
  const node = nodes.value.find(n => n.id === id)
  if (node) {
    const cur = node.data?.color || '#334155'
    const idx = colors.indexOf(cur)
    node.data = { ...node.data, color: colors[(idx + 1) % colors.length] }
  }
  resetContextMenu()
}

// ---- Mode ----
function setMode(newMode) {
  mode.value = newMode
  localStorage.setItem('scm-diagram-mode', newMode)
}

// ---- Clear canvas ----
function clearCanvas() {
  if (!confirm('确定清除画布所有内容？')) return
  nodes.value = []
  edges.value = []
  resetContextMenu()
  localStorage.removeItem('scm-diagram-data')
  showToast('画布已清除', 'success')
}

// ---- Text label & Group box ----
function onPaneClick(event) {
  if (mode.value === 'text') {
    const pos = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const id = 'text-' + (idCounter++)
    nodes.value = [...nodes.value, {
      id, type: 'textlabel',
      position: { x: pos.x - 60, y: pos.y - 15 },
      data: { text: '双击编辑文字', fontSize: 14, color: '#334155' },
      draggable: true,
    }]
  } else if (mode.value === 'group') {
    const pos = screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
    const id = 'group-' + (idCounter++)
    nodes.value = [...nodes.value, {
      id, type: 'groupbox',
      position: { x: pos.x - 120, y: pos.y - 60 },
      data: { label: '分组名称', dashedBorder: true, width: 240, height: 120 },
      draggable: true,
    }]
  }
}
function editGroupBoxLabel(nodeProps) {
  editingLabelId.value = nodeProps.id
  editLabelText.value = nodeProps.data?.label || '分组'
  nextTick(() => { if (textInput.value) textInput.value.focus() })
  // Use a custom inline editor for group box label
  const newLabel = prompt('输入分组名称:', nodeProps.data?.label || '分组')
  if (newLabel !== null) {
    const node = nodes.value.find(n => n.id === nodeProps.id)
    if (node) node.data = { ...node.data, label: newLabel }
  }
}
function editTextLabel(nodeProps) {
  editingLabelId.value = nodeProps.id
  editLabelText.value = nodeProps.data?.text || nodeProps.data?.label || ''
  nextTick(() => { if (textInput.value) textInput.value.focus() })
}
function commitTextEdit(nodeProps) {
  if (!editingLabelId.value) return
  const node = nodes.value.find(n => n.id === editingLabelId.value)
  if (node) node.data = { ...node.data, text: editLabelText.value }
  editingLabelId.value = null
  editLabelText.value = ''
}
function cancelTextEdit() {
  editingLabelId.value = null
  editLabelText.value = ''
}

// ---- Load hosts ----
async function loadHosts() {
  try {
    // 从 dashboard API 获取带角色的主机数据
    const dashData = await api('/api/dashboard')
    const hostsWithRoles = dashData.hosts || []
    hostData = hostsWithRoles.map(hwr => hwr.host || hwr)
    // Build palette server items — 从 HostWithRoles 中提取角色名
    serverNodes.value = hostsWithRoles.map(hwr => {
      const host = hwr.host || hwr
      const roleName = extractRoleName(hwr)
      return {
        id: 'host-' + host.id,
        data: {
          label: host.hostname || host.ip || '',
          ip: host.ip || '',
          role: roleName,
          status: host.status === 1 ? 'online' : 'offline',
        }
      }
    })
    const saved = localStorage.getItem('scm-diagram-data')
    if (saved) {
      restore(saved)
    } else {
      buildNodes(hostsWithRoles)
    }
  } catch (e) {
    console.error('Failed to load hosts', e)
    const saved = localStorage.getItem('scm-diagram-data')
    if (saved) restore(saved)
  }
}

// 从 HostWithRoles 提取第一个非 ICMP 角色名称
function extractRoleName(hwr) {
  if (hwr.roles && hwr.roles.length > 0) {
    const assignment = hwr.roles[0]
    if (assignment.role && assignment.role.name) {
      return assignment.role.name
    }
  }
  return 'worker'
}

function buildNodes(hosts) {
  const cols = 4, spacingX = 250, spacingY = 140
  const newNodes = hosts.map((item, i) => {
    const col = i % cols, row = Math.floor(i / cols)
    const host = item.host || item
    const roleName = item.roles ? extractRoleName(item) : 'worker'
    return {
      id: 'host-' + host.id, type: 'custom',
      position: { x: 80 + col * spacingX, y: 80 + row * spacingY },
      data: { label: host.hostname || host.ip || '', ip: host.ip || '', role: roleName, status: host.status === 1 ? 'online' : 'offline', tags: [] },
      draggable: true,
    }
  })
  const saved = localStorage.getItem('scm-diagram-data')
  let textNodes = []
  if (saved) {
    try {
      const parsed = JSON.parse(saved)
      textNodes = (parsed.nodes || []).filter(n => n.type === 'textlabel')
      if (parsed.edges) {
        edges.value = parsed.edges.map(e => ({
          id: e.id || 'e-' + e.source + '-' + e.target,
          source: e.source, target: e.target,
          sourceHandle: e.sourceHandle, targetHandle: e.targetHandle,
          type: e.edgeType || 'smoothstep',
          style: { stroke: e.color || '#94a3b8', strokeWidth: 2, strokeDasharray: e.dashed ? '5,5' : 'none' },
          markerEnd: e.arrow === false ? undefined : { type: MarkerType.ArrowClosed, color: e.color || '#94a3b8', width: 20, height: 20 },
          label: e.label || '', animated: e.animated || false,
        }))
      }
    } catch(e) {}
  }
  nodes.value = [...newNodes, ...textNodes]
}

function restore(saved) {
  try {
    const parsed = JSON.parse(saved)
    const hostNodes = (parsed.nodes || []).filter(n => n.type !== 'textlabel')
    const textNodes = (parsed.nodes || []).filter(n => n.type === 'textlabel')
    const apiHosts = hostData.length > 0 ? hostData : []
    const apiNodes = apiHosts.map((item, i) => {
      const host = item.host || item
      const existing = hostNodes.find(n => n.id === 'host-' + host.id)
      const roleName = item.roles ? extractRoleName(item) : 'worker'
      return {
        id: 'host-' + host.id, type: 'custom',
        position: existing ? existing.position : { x: 80 + (i % 4) * 250, y: 80 + Math.floor(i / 4) * 140 },
        data: { label: host.hostname || host.ip || '', ip: host.ip || '', role: roleName, status: host.status === 1 ? 'online' : 'offline', tags: [] },
        draggable: true,
      }
    })
    const savedTextNodes = textNodes.map(n => ({ ...n, type: 'textlabel', draggable: true }))
    const savedEdges = (parsed.edges || []).map(e => ({
      id: e.id || 'e-' + e.source + '-' + e.target,
      source: e.source, target: e.target,
      sourceHandle: e.sourceHandle, targetHandle: e.targetHandle,
      type: e.edgeType || 'smoothstep',
      style: { stroke: e.color || '#94a3b8', strokeWidth: 2, strokeDasharray: e.dashed ? '5,5' : 'none' },
      markerEnd: e.arrow === false ? undefined : { type: MarkerType.ArrowClosed, color: e.color || '#94a3b8', width: 20, height: 20 },
      label: e.label || '', animated: e.animated || false,
    }))
    nodes.value = [...apiNodes, ...savedTextNodes]
    edges.value = savedEdges
  } catch (e) {
    console.error('Failed to restore diagram', e)
    buildNodes(hostData)
  }
}

function autoLayout() {
  const cols = 4, spacingX = 250, spacingY = 140
  nodes.value.forEach((node, i) => {
    if (node.type === 'textlabel') return
    const col = i % cols, row = Math.floor(i / cols)
    node.position = { x: 80 + col * spacingX, y: 80 + row * spacingY }
  })
}

function save() {
  const payload = {
    nodes: nodes.value.map(n => ({ id: n.id, type: n.type, position: n.position, data: n.data })),
    edges: edges.value.map(e => ({
      id: e.id, source: e.source, target: e.target,
      sourceHandle: e.sourceHandle, targetHandle: e.targetHandle,
      label: e.label || '',
      color: e.style?.stroke || '#94a3b8',
      dashed: !!e.style?.strokeDasharray,
      arrow: !!e.markerEnd,
      edgeType: e.type || 'smoothstep',
    })),
  }
  localStorage.setItem('scm-diagram-data', JSON.stringify(payload))
  showToast('架构图已保存到本地', 'success')
}

async function exportImage() {
  const wrapper = document.querySelector('.diagram-canvas-wrapper')
  if (!wrapper) { showToast('未找到画布元素', 'error'); return }
  try {
    // 添加导出样式：隐藏控件、去除背景和网格
    wrapper.classList.add('is-exporting')
    const flowEl = wrapper.querySelector('.vue-flow')

    // 等待样式生效
    await new Promise(r => setTimeout(r, 100))

    const { toPng } = await import('html-to-image')
    const dataUrl = await toPng(flowEl || wrapper, {
      backgroundColor: '#ffffff',
      pixelRatio: 2,
      style: { background: '#ffffff' },
    })
    wrapper.classList.remove('is-exporting')

    // 生成 PDF
    const { default: jsPDF } = await import('jspdf')
    const pdf = new jsPDF('l', 'mm', 'a4')
    const pdfWidth = pdf.internal.pageSize.getWidth()
    const pdfHeight = pdf.internal.pageSize.getHeight()

    const img = new Image()
    img.src = dataUrl
    await new Promise((resolve, reject) => {
      img.onload = resolve
      img.onerror = reject
    })

    const imgRatio = img.width / img.height
    const pageRatio = pdfWidth / pdfHeight
    let finalW, finalH
    if (imgRatio > pageRatio) {
      finalW = pdfWidth
      finalH = pdfWidth / imgRatio
    } else {
      finalH = pdfHeight
      finalW = pdfHeight * imgRatio
    }
    const x = (pdfWidth - finalW) / 2
    const y = (pdfHeight - finalH) / 2
    pdf.addImage(dataUrl, 'PNG', x, y, finalW, finalH)
    pdf.save('architecture-' + Date.now() + '.pdf')
    showToast('导出 PDF 成功', 'success')
  } catch(e) {
    console.error('Export failed', e)
    wrapper.classList.remove('is-exporting')
    showToast('导出失败: ' + e.message, 'error')
  }
}

// Click outside to close context menu
function onGlobalClick() {
  if (contextMenu.value.show) resetContextMenu()
}

// Edge context menu via event delegation on VueFlow pane
function onNodesInit() {
  // Attach contextmenu listener on edges via VueFlow pane element
  const pane = document.querySelector('.vue-flow__pane')
  if (pane) {
    pane.addEventListener('contextmenu', (ev) => {
      // Check if click is on an edge element
      const edgeEl = ev.target.closest('.vue-flow__edge')
      if (edgeEl) {
        const edgeId = edgeEl.getAttribute('data-id')
        if (edgeId) {
          const edge = edges.value.find(e => e.id === edgeId)
          if (edge) openEdgeMenu(ev, edge)
        }
      }
    })
  }
}

onMounted(() => {
  loadHosts()
  document.addEventListener('click', onGlobalClick)
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Delete' || e.key === 'Backspace') {
      // VueFlow handles selected nodes via delete-key-code, but we also need
      // to handle case where context menu is open
    }
  })

  // Drag from palette to canvas
  const flowEl = document.querySelector('.vue-flow')
  if (flowEl) {
    flowEl.addEventListener('dragover', (e) => e.preventDefault())
    flowEl.addEventListener('drop', (e) => {
      e.preventDefault()
      const raw = e.dataTransfer.getData('application/vueflow')
      if (!raw) return
      const tpl = JSON.parse(raw)
      const pos = screenToFlowCoordinate({ x: e.clientX, y: e.clientY })
      const id = 'node-' + (idCounter++)
      const label = tpl.data?.label || '新服务器'
      nodes.value = [...nodes.value, {
        id, type: 'custom',
        position: { x: pos.x - 60, y: pos.y - 25 },
        data: { ...tpl.data, label },
        draggable: true,
      }]
    })
  }
})

onUnmounted(() => {
  document.removeEventListener('click', onGlobalClick)
})
</script>

<style scoped>
.diagram-toolbar { display: flex; align-items: center; gap: 6px; padding: 12px 0; flex-wrap: wrap; }
.diagram-palette {
  display: flex; align-items: center; gap: 8px; padding: 8px 12px; margin-bottom: 4px;
  background: var(--card-bg, #ffffff); border: 1px solid var(--border-color, #e2e8f0);
  border-radius: 8px; font-size: 13px;
}
.palette-label { color: var(--text-secondary, #64748b); font-weight: 500; white-space: nowrap; }
.palette-item, .palette-server {
  cursor: grab; padding: 4px 12px; border-radius: 6px;
  background: var(--bg-secondary, #f1f5f9); color: var(--text-primary, #1e293b);
  border: 1px solid var(--border-color, #e2e8f0); font-size: 12px; font-weight: 500;
  transition: all .15s;
}
.palette-item:active, .palette-server:active { cursor: grabbing; }
.palette-item:hover, .palette-server:hover { border-color: var(--accent, #3b82f6); color: var(--accent, #3b82f6); }
.palette-server { background: rgba(59,130,246,.08); border-color: rgba(59,130,246,.2); }
.diagram-canvas-wrapper { height: 65vh; min-height: 400px; border: 1px solid var(--border-color, #e2e8f0); border-radius: 10px; overflow: hidden; background: var(--card-bg, #ffffff); }
.mode-hint { text-align: center; padding: 4px 12px; margin-bottom: 6px; font-size: 12px; color: var(--accent, #3b82f6); background: rgba(59,130,246,.08); border-radius: 6px; display: inline-block; }
.text-label-node { min-width: 80px; min-height: 30px; cursor: move; user-select: none; }
.text-label-node.selected { outline: 2px dashed #3b82f6; outline-offset: 4px; border-radius: 4px; }
.text-label-display { padding: 4px 8px; font-weight: 500; white-space: pre-wrap; line-height: 1.4; text-align: center; }
.text-label-editing textarea { width: 160px; padding: 4px 8px; border: 2px solid #3b82f6; border-radius: 6px; font-size: 14px; font-family: inherit; resize: none; background: var(--card-bg, #fff); color: var(--text-primary, #1e293b); outline: none; }
.context-menu { background: var(--card-bg, #ffffff); border: 1px solid var(--border-color, #e2e8f0); border-radius: 8px; box-shadow: 0 4px 16px rgba(0,0,0,.12); min-width: 130px; padding: 4px; }
.context-menu-item { padding: 6px 12px; font-size: 13px; cursor: pointer; border-radius: 4px; color: var(--text-primary, #1e293b); }
.context-menu-item:hover { background: var(--bg-secondary, #f1f5f9); color: var(--accent, #3b82f6); }
.context-menu-separator { height: 1px; background: var(--border-color, #e2e8f0); margin: 4px 0; }

/* Group box node */
.groupbox-node {
  min-width: 200px;
  min-height: 100px;
  border: 2px dashed #94a3b8;
  border-radius: 12px;
  background: rgba(148,163,184,.06);
  cursor: move;
  display: flex;
  align-items: flex-start;
  justify-content: flex-start;
  padding: 8px 12px;
  position: relative;
  box-sizing: border-box;
}
.groupbox-node.selected {
  border-color: var(--accent, #3b82f6);
  background: rgba(59,130,246,.04);
}
.groupbox-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary, #64748b);
  user-select: none;
}
.resize-handle {
  position: absolute;
  right: 0;
  bottom: 0;
  width: 12px;
  height: 12px;
  cursor: nw-resize;
  background: transparent;
  border-right: 2px solid var(--accent, #3b82f6);
  border-bottom: 2px solid var(--accent, #3b82f6);
  border-radius: 0 0 4px 0;
}
[data-theme="dark"] .groupbox-node {
  background: rgba(148,163,184,.04);
  border-color: #475569;
}
[data-theme="dark"] .groupbox-node.selected {
  border-color: var(--accent, #3b82f6);
  background: rgba(59,130,246,.06);
}

/* Export mode — hide controls and background grid */
.diagram-canvas-wrapper.is-exporting .vue-flow__controls {
  display: none !important;
}
.diagram-canvas-wrapper.is-exporting .vue-flow__background {
  display: none !important;
}
.diagram-canvas-wrapper.is-exporting .vue-flow {
  background: #ffffff !important;
}
.diagram-canvas-wrapper.is-exporting {
  background: #ffffff !important;
}
</style>

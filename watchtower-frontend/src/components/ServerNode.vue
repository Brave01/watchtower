<template>
  <div class="server-node" :class="[`status-${data.status}`, `role-${data.role}`, { 'selected': selected }]">
    <!-- 8 connection points: TL, TC, TR, RC, BR, BC, BL, LC -->
    <!-- Top row: TL + TC + TR -->
    <Handle type="target" position="top" id="t-tl" class="hdl-corner hdl-tl" />
    <Handle type="source" position="top" id="s-tl" class="hdl-corner hdl-tl" />
    <Handle type="target" position="top" id="t-tc" class="hdl-center" />
    <Handle type="source" position="top" id="s-tc" class="hdl-center" />
    <Handle type="target" position="top" id="t-tr" class="hdl-corner hdl-tr" />
    <Handle type="source" position="top" id="s-tr" class="hdl-corner hdl-tr" />

    <!-- Right: RC -->
    <Handle type="target" position="right" id="t-rc" class="hdl-center" />
    <Handle type="source" position="right" id="s-rc" class="hdl-center" />

    <!-- Bottom row: BR + BC + BL -->
    <Handle type="target" position="bottom" id="t-br" class="hdl-corner hdl-br" />
    <Handle type="source" position="bottom" id="s-br" class="hdl-corner hdl-br" />
    <Handle type="target" position="bottom" id="t-bc" class="hdl-center" />
    <Handle type="source" position="bottom" id="s-bc" class="hdl-center" />
    <Handle type="target" position="bottom" id="t-bl" class="hdl-corner hdl-bl" />
    <Handle type="source" position="bottom" id="s-bl" class="hdl-corner hdl-bl" />

    <!-- Left: LC -->
    <Handle type="target" position="left" id="t-lc" class="hdl-center" />
    <Handle type="source" position="left" id="s-lc" class="hdl-center" />

    <div class="node-header">
      <span class="node-led" :class="'led-' + statusColor"></span>
      <span class="node-hostname">{{ data.label }}</span>
    </div>

    <div class="node-body">
      <div class="node-ip" v-if="data.ip">{{ data.ip }}</div>
      <div class="node-tags">
        <span class="tag" :class="roleTagClass">{{ roleLabel }}</span>
        <span v-for="tag in data.tags" :key="tag" class="tag tag-gray">{{ tag }}</span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { Handle, useNodeId } from '@vue-flow/core'

const props = defineProps({
  id: { type: String, required: true },
  data: { type: Object, default: () => ({}) },
  type: { type: String, default: 'custom' },
  selected: { type: Boolean, default: false },
})

const nodeId = useNodeId()

const statusColor = computed(() => {
  switch (props.data.status) {
    case 'online': return 'green'
    case 'offline': return 'red'
    case 'warning': return 'yellow'
    default: return 'gray'
  }
})

const roleLabel = computed(() => {
  switch (props.data.role) {
    case 'worker': return '工作节点'
    case 'database': return '数据库'
    case 'loadbalancer': return '负载均衡'
    case 'master': return '主节点'
    default: return props.data.role || '未知'
  }
})

const roleTagClass = computed(() => {
  switch (props.data.role) {
    case 'worker': return 'tag-green'
    case 'database': return 'tag-blue'
    case 'loadbalancer': return 'tag-yellow'
    case 'master': return 'tag-red'
    default: return 'tag-gray'
  }
})
</script>

<style scoped>
.server-node {
  background: var(--card-bg, #ffffff);
  border: 2px solid var(--border-color, #e2e8f0);
  border-radius: 10px;
  padding: 0;
  min-width: 160px;
  font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
  transition: box-shadow 0.15s, border-color 0.15s;
  position: relative;
}
.server-node:hover {
  box-shadow: 0 4px 12px rgba(0,0,0,.08);
}
.server-node.selected {
  border-color: var(--accent, #3b82f6);
  box-shadow: 0 0 0 2px rgba(59,130,246,.2);
}

/* Status border accent */
.server-node.status-online {
  border-color: #22c55e;
}
.server-node.status-offline {
  border-color: #ef4444;
}
.server-node.status-warning {
  border-color: #f59e0b;
}

.node-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 12px 6px;
}
.node-led {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.node-led.led-green { background: #22c55e; box-shadow: 0 0 6px rgba(34,197,94,.5); }
.node-led.led-red { background: #ef4444; box-shadow: 0 0 6px rgba(239,68,68,.5); }
.node-led.led-yellow { background: #f59e0b; box-shadow: 0 0 6px rgba(245,158,11,.5); }
.node-led.led-gray { background: #94a3b8; }

.node-hostname {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary, #1e293b);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.node-body {
  padding: 0 12px 10px;
}
.node-ip {
  font-size: 11px;
  font-family: 'JetBrains Mono', monospace;
  color: var(--text-secondary, #64748b);
  margin-bottom: 6px;
}
.node-tags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}
.node-tags .tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 8px;
  border-radius: 4px;
  font-size: 10px;
  font-weight: 500;
}
.node-tags .tag-green { background: #dcfce7; color: #166534; }
.node-tags .tag-red { background: #fee2e2; color: #991b1b; }
.node-tags .tag-yellow { background: #fef3c7; color: #92400e; }
.node-tags .tag-blue { background: #dbeafe; color: #1e40af; }
.node-tags .tag-gray { background: #f1f5f9; color: #475569; }

/* Dark theme overrides */
[data-theme="dark"] .server-node {
  background: #1e293b;
  border-color: #334155;
}
[data-theme="dark"] .server-node:hover {
  box-shadow: 0 4px 12px rgba(0,0,0,.3);
}
[data-theme="dark"] .server-node.selected {
  border-color: var(--accent, #3b82f6);
}
[data-theme="dark"] .server-node.status-online {
  border-color: #22c55e;
}
[data-theme="dark"] .server-node.status-offline {
  border-color: #ef4444;
}
[data-theme="dark"] .server-node.status-warning {
  border-color: #f59e0b;
}
[data-theme="dark"] .node-tags .tag-green { background: rgba(34,197,94,.15); color: #4ade80; }
[data-theme="dark"] .node-tags .tag-red { background: rgba(239,68,68,.15); color: #f87171; }
[data-theme="dark"] .node-tags .tag-yellow { background: rgba(245,158,11,.15); color: #fbbf24; }
[data-theme="dark"] .node-tags .tag-blue { background: rgba(59,130,246,.15); color: #60a5fa; }
[data-theme="dark"] .node-tags .tag-gray { background: rgba(148,163,184,.15); color: #94a3b8; }

/* Handle base styles — hidden by default, show on hover */
.server-node :deep(.vue-flow__handle) {
  width: 12px;
  height: 12px;
  background: #94a3b8;
  border: 2px solid #fff;
  transition: opacity 0.15s, width 0.15s, height 0.15s;
  z-index: 10;
  cursor: crosshair;
  opacity: 0;
  pointer-events: none;
}
/* Show handles when node is hovered */
.server-node:hover :deep(.vue-flow__handle) {
  opacity: 1;
  pointer-events: all;
}
.server-node :deep(.vue-flow__handle:hover),
.server-node :deep(.vue-flow__handle.connecting),
.server-node :deep(.vue-flow__handle.connectable) {
  width: 16px;
  height: 16px;
  background: #3b82f6;
  border-color: #bfdbfe;
}
/* Corner handle positioning */
.server-node :deep(.hdl-tl) { left: 0 !important; }
.server-node :deep(.hdl-tr) { left: 100% !important; }
.server-node :deep(.hdl-bl) { left: 0 !important; }
.server-node :deep(.hdl-br) { left: 100% !important; }
</style>

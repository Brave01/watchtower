<template>
  <div>
    <div class="tab-header">
      <h1 class="tab-title">服务器</h1>
      <p class="tab-subtitle">管理所有主机的连接与状态</p>
    </div>

    <!-- Actions -->
    <div class="action-bar">
      <button class="btn btn-primary" @click="showAddModal=true">+ 添加主机</button>
      <button class="btn" @click="openBatchHostModal">批量添加主机</button>
      <button class="btn" @click="openAddRoleModal">添加角色</button>
      <button class="btn" @click="openAssignModal">分配角色</button>
      <button class="btn" @click="openBatchRoleModal">批量创建角色</button>
      <button class="btn btn-success" @click="downloadExcel">导出 Excel</button>
      <button class="btn" @click="openRoleManageModal">角色管理</button>
      <button class="btn" @click="openSSHCredModal">SSH 凭据</button>
      <button class="btn" @click="loadHosts; loadRoles">刷新</button>
      <div style="flex:1"></div>
      <span style="font-size:13px;color:var(--text-secondary)">共 {{ hosts.length }} 台</span>
    </div>

    <!-- Role filters -->
    <div v-if="roles.length > 0" class="role-filters">
      <button class="role-chip" :class="{active: activeRole === ''}" @click="activeRole=''">全部</button>
      <button v-for="r in roles" :key="r.id" class="role-chip" :class="{active: activeRole === r.name}" @click="activeRole=r.name">{{ roleDisplayName(r.name) }}</button>
    </div>

    <!-- Hosts grid -->
    <div class="hosts-grid">
      <div v-for="host in filteredHosts" :key="host.id" class="host-card">
        <div class="host-header">
          <span class="led" :class="'led-'+hostStatus(host)"></span>
          <div>
            <div class="host-name">{{ host.hostname || host.ip }}</div>
            <div class="host-addr">{{ host.ip || '-' }}</div>
          </div>
        </div>
        <div class="host-alive">
          <span class="alive-dot" :class="icmpOnline(host)?'dot-on':'dot-off'"></span>
          <span class="alive-text">存活: {{ icmpLabel(host) }}</span>
          <span v-if="host.maintenance" class="maint-badge">维护中</span>
        </div>
        <div v-if="host.roleProbes && host.roleProbes.length > 0" class="host-probes">
          <div v-for="rp in host.roleProbes" :key="rp.name" style="display:flex;align-items:center;gap:2px">
            <span class="probe-item" :class="'probe-'+probeStatusClass(rp.status)">
              {{ roleDisplayName(rp.name) }}: {{ probeLabel(rp.status) }}
            </span>
            <span v-if="rp.error" class="probe-error" :title="rp.error">ⓘ</span>
            <button class="probe-unbind" title="解除此角色" @click="unbindRole(host, rp.name)">✕</button>
          </div>
        </div>
        <div class="host-actions">
          <button class="btn btn-sm" @click="openTerminal(host)" :disabled="host.status !== 1">SSH</button>
          <button class="btn btn-sm" @click="pingHost(host)">Ping</button>
          <button class="btn btn-sm" @click="editHost(host)">编辑</button>
          <button class="btn btn-sm" :class="host.maintenance?'btn-success':'btn'" @click="toggleMaintenance(host)">{{ host.maintenance ? '恢复' : '维护' }}</button>
          <button class="btn btn-sm btn-danger" @click="deleteHost(host)">删除</button>
        </div>
      </div>
      <div v-if="hosts.length===0" class="empty-state"><p>暂无主机</p></div>
    </div>

    <!-- Add/Edit modal -->
    <div v-if="showAddModal||editingHost" class="modal-overlay" @click.self="closeModal">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <span class="modal-title">{{ editingHost ? '编辑主机' : '添加主机' }}</span>
          <button class="modal-close" @click="closeModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">主机地址</label>
            <input class="form-input" v-model="formAddr" placeholder="例如 10.3.1.100" />
          </div>
          <div class="form-group">
            <label class="form-label">主机名</label>
            <input class="form-input" v-model="formName" placeholder="可选" />
          </div>
          <div class="form-group">
            <label class="form-label">CPU</label>
            <div style="display:flex;gap:8px;align-items:center">
              <input class="form-input" v-model="formCPU" placeholder="例如 4" style="flex:1" />
              <span style="color:var(--text-secondary);font-size:13px;white-space:nowrap">核</span>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">内存</label>
            <div style="display:flex;gap:8px;align-items:center">
              <input class="form-input" v-model="formMemory" placeholder="例如 8" style="flex:1" />
              <span style="color:var(--text-secondary);font-size:13px;white-space:nowrap">GB</span>
            </div>
          </div>
          <div class="form-group">
            <label class="form-label">磁盘分区</label>
            <div class="table-wrap" style="margin-bottom:8px">
              <table>
                <thead>
                  <tr>
                    <th>挂载点</th>
                    <th>大小</th>
                    <th>单位</th>
                    <th style="width:10%">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(disk, idx) in formDisk" :key="idx">
                    <td><input class="form-input" v-model="disk.mount" placeholder="/data" style="font-size:13px" /></td>
                    <td><input class="form-input" v-model="disk.size" placeholder="200" style="font-size:13px;width:80px" type="number" min="0" /></td>
                    <td>
                      <select class="form-select" v-model="disk.unit" style="font-size:13px">
                        <option value="GB">GB</option>
                        <option value="TB">TB</option>
                      </select>
                    </td>
                    <td><button class="btn btn-sm btn-danger" @click="removeDiskRow(idx)" :disabled="formDisk.length<=1">删除</button></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <button class="btn btn-sm" @click="addDiskRow">+ 添加分区</button>
          </div>
          <div v-if="editingHost" class="form-group">
            <label class="form-label">角色</label>
            <select class="form-select" v-model="formRole">
              <option value="">不分配角色</option>
              <option v-for="r in roles" :key="r.id" :value="r.name">{{ roleDisplayName(r.name) }}</option>
            </select>
            <p style="font-size:11px;color:var(--text-secondary);margin-top:4px">当前角色: {{ roleDisplayName(formRole || '') }}</p>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="closeModal">取消</button>
          <button class="btn btn-primary" @click="saveHost">{{ editingHost ? '保存' : '添加' }}</button>
        </div>
      </div>
    </div>

    <!-- SSH Credential Selection modal -->
    <div v-if="showSSHSelectModal" class="modal-overlay" @click.self="cancelSSHConnect">
      <div class="modal" @click.stop>
        <div class="modal-header">
          <span class="modal-title">SSH 连接 - {{ (pendingTerminalHost.hostname || pendingTerminalHost.ip) }}</span>
          <button class="modal-close" @click="cancelSSHConnect">&times;</button>
        </div>
        <div class="modal-body" style="max-height:60vh;overflow-y:auto">
          <div v-if="sshSelectCredentials.length > 0" style="margin-bottom:16px">
            <label class="form-label" style="margin-bottom:6px">已保存的凭据</label>
            <div class="table-wrap">
              <table>
                <thead>
                  <tr><th></th><th>标签</th><th>用户名</th><th>认证方式</th></tr>
                </thead>
                <tbody>
                  <tr v-for="c in sshSelectCredentials" :key="c.id" style="cursor:pointer" :class="{ 'row-selected': sshSelectCredId === c.id }" @click="sshSelectCredId = c.id; sshManualUser=''; sshManualPass=''">
                    <td><input type="radio" :value="c.id" v-model="sshSelectCredId" /></td>
                    <td>{{ c.label }}</td>
                    <td>{{ c.username }}</td>
                    <td>{{ c.auth_method === 'password' ? '密码' : '密钥' }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
          <div style="border-top:1px solid var(--border-color);padding-top:16px">
            <label class="form-label" style="margin-bottom:6px">手动认证</label>
            <div style="display:flex;gap:8px;margin-bottom:8px">
              <input class="form-input" v-model="sshManualUser" placeholder="用户名" style="flex:1" @focus="sshSelectCredId=''" />
              <input class="form-input" v-model="sshManualPass" type="password" placeholder="密码" style="flex:1" @focus="sshSelectCredId=''" />
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="cancelSSHConnect">取消</button>
          <button class="btn btn-primary" @click="confirmSSHConnect" :disabled="!sshSelectCredId && !sshManualUser">连接</button>
        </div>
      </div>
    </div>

    <!-- SSH Terminal modal -->
    <div v-if="terminalHost" class="modal-overlay" @click.self="closeTerminal">
      <div class="modal modal-lg" @click.stop>
        <div class="modal-header">
          <span class="modal-title">SSH: {{ terminalHost.hostname || terminalHost.addr }}</span>
          <div style="display:flex;gap:8px;align-items:center">
            <span class="connection-badge" :class="termConnected?'connected':'disconnected'">
              <span class="badge-dot"></span>
              {{ termConnected ? '已连接' : '未连接' }}
            </span>
            <button class="modal-close" @click="closeTerminal">&times;</button>
          </div>
        </div>
        <div class="modal-body" style="display:flex;flex-direction:column">
          <div v-if="!termCredMode" style="display:flex;gap:8px;margin-bottom:8px">
            <input class="form-input" v-model="termUser" placeholder="用户名" style="flex:1" @keyup.enter="connectTerminal" />
            <input class="form-input" v-model="termPass" type="password" placeholder="密码" style="flex:1" @keyup.enter="connectTerminal" />
            <button class="btn btn-primary" @click="connectTerminal">{{ termConnected ? '重连' : '连接' }}</button>
            <button class="btn btn-danger" @click="disconnectTerminal" :disabled="!termConnected">断开</button>
          </div>
          <div v-if="termCredMode" style="display:flex;gap:8px;margin-bottom:8px;align-items:center">
            <span style="font-size:13px;color:var(--text-secondary)">已使用 SSH 凭据自动连接</span>
            <button class="btn btn-danger" @click="disconnectTerminal" :disabled="!termConnected">断开</button>
          </div>
          <div id="terminal-container" ref="termContainerRef" style="flex:1;min-height:300px;border-radius:8px;overflow:hidden"></div>
        </div>
      </div>
    </div>

    <!-- Batch Add Hosts modal -->
    <div v-if="showBatchHostModal" class="modal-overlay" @click.self="showBatchHostModal=false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <span class="modal-title">批量添加主机</span>
          <button class="modal-close" @click="showBatchHostModal=false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">主机列表</label>
            <div class="table-wrap" style="margin-bottom:12px">
              <table>
                <thead>
                  <tr>
                    <th>主机名称</th>
                    <th>主机地址</th>
                    <th>CPU</th>
                    <th>内存</th>
                    <th>磁盘</th>
                    <th style="width:6%">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, idx) in batchHostItems" :key="idx">
                    <td><input class="form-input" v-model="item.hostname" placeholder="web-server-01" style="font-size:13px"></td>
                    <td><input class="form-input" v-model="item.ip" placeholder="192.168.1.100" style="font-size:13px"></td>
                    <td><input class="form-input" v-model="item.cpu" placeholder="4核" style="font-size:13px;width:70px"></td>
                    <td><input class="form-input" v-model="item.memory" placeholder="8GB" style="font-size:13px;width:70px"></td>
                    <td><input class="form-input" v-model="item.disk" placeholder="/:50G" style="font-size:13px"></td>
                    <td><button class="btn btn-sm btn-danger" @click="removeBatchHost(idx)" :disabled="batchHostItems.length<=1">删除</button></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <button class="btn btn-sm" @click="addBatchHost">+ 添加一行</button>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showBatchHostModal=false">取消</button>
          <button class="btn btn-primary" @click="submitBatchHosts" :disabled="batchHostItems.length===0">批量添加</button>
        </div>
      </div>
    </div>

    <!-- Add/Edit Role modal -->
    <div v-if="showAddRoleModal" class="modal-overlay" @click.self="showAddRoleModal=false">
      <div class="modal">
        <div class="modal-header">
          <span class="modal-title">{{ editingRole ? '编辑角色' : '添加角色' }}</span>
          <button class="modal-close" @click="showAddRoleModal=false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">角色名称</label>
            <input class="form-input" v-model="roleFormName" placeholder="例如: database" />
          </div>
          <div class="form-group">
            <label class="form-label">探测类型</label>
            <select class="form-select" v-model="roleFormType" @change="onRoleTypeChange">
              <option value="">请选择类型</option>
              <option value="ICMP">ICMP（存活检测）</option>
              <option value="TCP">TCP（端口检测）</option>
              <option value="HTTP">HTTP（接口检测）</option>
              <option value="SSH">SSH（SSH检测）</option>
            </select>
          </div>
          <div v-if="roleFormType && roleFormType !== 'ICMP'" class="form-group">
            <label class="form-label">端口</label>
            <input class="form-input" v-model.number="roleFormPort" type="number" min="1" max="65535" placeholder="例如: 80" />
          </div>
          <div v-if="roleFormType === 'HTTP'" class="form-group">
            <label class="form-label">检测路径</label>
            <input class="form-input" v-model="roleFormPath" placeholder="/health" />
          </div>
          <div class="form-group">
            <label class="form-label">超时（秒）</label>
            <input class="form-input" v-model.number="roleFormTimeout" type="number" min="1" placeholder="5" />
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showAddRoleModal=false">取消</button>
          <button class="btn btn-primary" @click="submitRoleForm" :disabled="!roleFormName || !roleFormType">{{ editingRole ? '保存' : '添加' }}</button>
        </div>
      </div>
    </div>

    <!-- Assign Role modal -->
    <div v-if="showAssignModal" class="modal-overlay" @click.self="showAssignModal=false">
      <div class="modal">
        <div class="modal-header">
          <span class="modal-title">分配角色</span>
          <button class="modal-close" @click="showAssignModal=false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">主机</label>
            <select class="form-select" v-model="assignHostId">
              <option value="">请选择主机</option>
              <option v-for="h in hosts" :key="h.id" :value="h.id">{{ h.hostname || h.ip }} ({{ h.ip }})</option>
            </select>
          </div>
          <div class="form-group">
            <label class="form-label">角色</label>
            <select class="form-select" v-model="assignRoleName">
              <option value="">请选择角色</option>
              <option v-for="r in roles" :key="r.id" :value="r.name">{{ r.name }}</option>
            </select>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showAssignModal=false">取消</button>
          <button class="btn btn-primary" @click="submitAssign" :disabled="!assignHostId||!assignRoleName">分配</button>
        </div>
      </div>
    </div>

    <!-- Batch Create Roles modal -->
    <div v-if="showBatchRoleModal" class="modal-overlay" @click.self="showBatchRoleModal=false">
      <div class="modal modal-lg">
        <div class="modal-header">
          <span class="modal-title">批量创建角色</span>
          <button class="modal-close" @click="showBatchRoleModal=false">&times;</button>
        </div>
        <div class="modal-body">
          <div class="form-group">
            <label class="form-label">角色列表</label>
            <div class="table-wrap" style="margin-bottom:12px">
              <table>
                <thead>
                  <tr>
                    <th>名称</th>
                    <th>类型</th>
                    <th>端口</th>
                    <th>路径</th>
                    <th>超时(秒)</th>
                    <th style="width:10%">操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, idx) in batchRoleItems" :key="idx">
                    <td><input class="form-input" v-model="item.name" placeholder="mysql-probe" style="font-size:13px"></td>
                    <td>
                      <select class="form-select" v-model="item.type" style="font-size:13px">
                        <option value="ICMP">ICMP</option>
                        <option value="TCP">TCP</option>
                        <option value="HTTP">HTTP</option>
                        <option value="SSH">SSH</option>
                      </select>
                    </td>
                    <td><input class="form-input" v-model.number="item.port" type="number" min="0" placeholder="端口" style="font-size:13px;width:80px"></td>
                    <td><input class="form-input" v-model="item.path" placeholder="/health" style="font-size:13px"></td>
                    <td><input class="form-input" v-model.number="item.timeout" type="number" min="1" placeholder="5" style="font-size:13px;width:70px"></td>
                    <td><button class="btn btn-sm btn-danger" @click="removeBatchRole(idx)" :disabled="batchRoleItems.length<=1">删除</button></td>
                  </tr>
                </tbody>
              </table>
            </div>
            <button class="btn btn-sm" @click="addBatchRole">+ 添加一行</button>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showBatchRoleModal=false">取消</button>
          <button class="btn btn-primary" @click="submitBatchRoles" :disabled="batchRoleItems.length===0">批量创建</button>
        </div>
      </div>
    </div>

    <!-- Role Management modal -->
    <div v-if="showRoleManageModal" class="modal-overlay" @click.self="showRoleManageModal=false">
      <div class="modal" style="max-width:560px">
        <div class="modal-header">
          <span class="modal-title">角色管理</span>
          <button class="modal-close" @click="showRoleManageModal=false">&times;</button>
        </div>
        <div class="modal-body" style="max-height:60vh;overflow-y:auto">
          <div v-if="roles.length > 0" class="table-wrap">
            <table>
              <thead>
                <tr><th>名称</th><th>类型</th><th>端口</th><th>已分配主机数</th><th>操作</th></tr>
              </thead>
              <tbody>
                <tr v-for="r in roles" :key="r.id">
                  <td>{{ roleDisplayName(r.name) }}</td>
                  <td>{{ r.type }}</td>
                  <td>{{ r.port || '-' }}</td>
                  <td>{{ hostCountByRole(r.name) }}</td>
                  <td><button class="btn btn-sm" @click="editRole(r)">编辑</button> <button class="btn btn-sm btn-danger" @click="deleteRole(r)">删除</button></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div v-else style="text-align:center;color:var(--text-secondary);padding:24px 0">暂无角色</div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showRoleManageModal=false">关闭</button>
        </div>
      </div>
    </div>

    <!-- SSH Credential modal -->
    <div v-if="showSSHCredModal" class="modal-overlay" @click.self="showSSHCredModal=false">
      <div class="modal" style="max-width:560px">
        <div class="modal-header">
          <span class="modal-title">SSH 凭据管理</span>
          <button class="modal-close" @click="showSSHCredModal=false">&times;</button>
        </div>
        <div class="modal-body" style="max-height:60vh;overflow-y:auto">
          <div v-if="sshCredentials.length > 0" class="table-wrap" style="margin-bottom:16px">
            <table>
              <thead>
                <tr><th>标签</th><th>用户名</th><th>认证方式</th><th>操作</th></tr>
              </thead>
              <tbody>
                <tr v-for="c in sshCredentials" :key="c.id">
                  <td>{{ c.label }}</td>
                  <td>{{ c.username }}</td>
                  <td>{{ c.auth_method === 'password' ? '密码' : '密钥' }}</td>
                  <td><button class="btn btn-sm btn-danger" @click="deleteSSHCred(c.id)">删除</button></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div style="border-top:1px solid var(--border-color);padding-top:16px">
            <h3 style="font-size:14px;font-weight:600;margin-bottom:12px">添加凭据</h3>
            <div class="form-group">
              <label class="form-label">标签</label>
              <input class="form-input" v-model="sshCredLabel" placeholder="例如: 生产服务器密钥" />
            </div>
            <div class="form-group">
              <label class="form-label">用户名</label>
              <input class="form-input" v-model="sshCredUsername" placeholder="例如: root" />
            </div>
            <div class="form-group">
              <label class="form-label">认证方式</label>
              <select class="form-select" v-model="sshCredAuthMethod">
                <option value="password">密码</option>
                <option value="key">密钥</option>
              </select>
            </div>
            <div class="form-group" v-if="sshCredAuthMethod==='password'">
              <label class="form-label">密码</label>
              <input class="form-input" v-model="sshCredPassword" type="password" placeholder="SSH 密码" />
            </div>
            <div class="form-group" v-if="sshCredAuthMethod==='key'">
              <label class="form-label">私钥内容</label>
              <textarea class="form-input" v-model="sshCredPrivateKey" rows="4" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----&#10;...&#10;-----END OPENSSH PRIVATE KEY-----"></textarea>
            </div>
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn" @click="showSSHCredModal=false">关闭</button>
          <button class="btn btn-primary" @click="submitSSHCred" :disabled="!sshCredLabel||!sshCredUsername">添加凭据</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { api, showToast } from '../api.js'

// 后端 Host.status 是 int: 0=Unknown,1=Up,2=Down,3=Warning,4=Muted
const STATUS_MAP = { 0: 'gray', 1: 'green', 2: 'red', 3: 'yellow', 4: 'yellow' }
// ICMP 存活状态（LED 颜色和存活标签）
function hostStatus(host) { return STATUS_MAP[host.status] || 'gray' }
function icmpOnline(host) { return host.status === 1 }
function icmpLabel(host) { return host.status === 1 ? '在线' : host.status === 2 ? '离线' : '未知' }
// 角色探测状态（角色标签使用）
function roleOnline(host) { return host.roleProbeStatus === 1 }
function roleLabel(host) { return host.roleProbeStatus === 1 ? '正常' : host.roleProbeStatus === 2 ? '异常' : host.roleProbeStatus === 3 ? '重试中' : '待探测' }
function roleStatusClass(host) {
  if (host.roleProbeStatus === 1) return 'green'
  if (host.roleProbeStatus === 2) return 'red'
  if (host.roleProbeStatus === 3) return 'yellow'
  return 'gray'
}
// 通用的角色状态显示函数（接受 status 参数，用于多角色迭代）
function probeLabel(status) {
  return status === 1 ? '正常' : status === 2 ? '异常' : status === 3 ? '重试中' : '待探测'
}
function probeStatusClass(status) {
  if (status === 1) return 'green'
  if (status === 2) return 'red'
  if (status === 3) return 'yellow'
  return 'gray'
}

const ROLE_NAMES = {
  master: '主节点', worker: '工作节点', storage: '存储',
  monitor: '监控', loadbalancer: '负载均衡', database: '数据库',
}
function roleDisplayName(name) {
  return ROLE_NAMES[name] || name || '未知'
}

const hosts = ref([])
const roles = ref([])
const activeRole = ref('')

// Add/Edit modal
const showAddModal = ref(false)
const editingHost = ref(null)
const formAddr = ref('')
const formName = ref('')
const formCPU = ref('')
const formMemory = ref('')
const formDisk = ref([{ mount: '', size: '', unit: 'GB' }])
const formRole = ref('')
const origRoleName = ref('')  // 编辑时记录原始角色，用于检测变更

// Terminal
const terminalHost = ref(null)
const termContainerRef = ref(null)
const termUser = ref('root')
const termPass = ref('')
const termConnected = ref(false)
const termCredMode = ref(false)  // 是否使用凭据连接（隐藏手动输入框）
let terminal = null
let termSocket = null

// SSH Credential Selection
const showSSHSelectModal = ref(false)
const sshSelectCredentials = ref([])
const sshSelectCredId = ref('')
const sshManualUser = ref('root')
const sshManualPass = ref('')
const pendingTerminalHost = ref(null)

// Batch Add Hosts
const showBatchHostModal = ref(false)
const batchHostItems = ref([{ hostname: '', ip: '', cpu: '', memory: '', disk: '' }])

// Add Role
const showAddRoleModal = ref(false)
const editingRole = ref(null)
const roleFormName = ref('')
const roleFormType = ref('TCP')
const roleFormPort = ref(0)
const roleFormPath = ref('')
const roleFormTimeout = ref(5)

// Assign Role
const showAssignModal = ref(false)
const assignHostId = ref('')
const assignRoleName = ref('')

// Batch Create Roles
const showBatchRoleModal = ref(false)
const batchRoleItems = ref([{ name: '', type: 'TCP', port: 0, path: '/', timeout: 5 }])

// SSH Credential Management
const showSSHCredModal = ref(false)
const sshCredentials = ref([])
const sshCredLabel = ref('')
const sshCredUsername = ref('')
const sshCredAuthMethod = ref('password')
const sshCredPassword = ref('')
const sshCredPrivateKey = ref('')

// Role Management
const showRoleManageModal = ref(false)

function hostCountByRole(roleName) {
  return hosts.value.filter(h => h.roles && h.roles.includes(roleName)).length
}
function openRoleManageModal() {
  showRoleManageModal.value = true
}
async function deleteRole(role) {
  if (!confirm('确定删除角色 "' + roleDisplayName(role.name) + '" 吗？\n该角色将从所有主机解除分配。')) return
  try {
    await api('/api/roles?id=' + encodeURIComponent(role.id), { method: 'DELETE' })
    showToast('角色已删除', 'success')
    await loadRoles()
    await loadHosts()
  } catch(e) { showToast('删除失败: ' + e.message, 'error') }
}

// Computed
const filteredHosts = computed(() => {
  if (!activeRole.value) return hosts.value
  // 主机可能没有 roles 字段，需要安全访问
  return hosts.value.filter(h => {
    const hRoles = h.roles || []
    return hRoles.includes(activeRole.value)
  })
})

function closeModal() {
  showAddModal.value = false
  editingHost.value = null
  formAddr.value = ''
  formName.value = ''
  formCPU.value = ''
  formMemory.value = ''
  formDisk.value = [{ mount: '', size: '', unit: 'GB' }]
  formRole.value = ''
  origRoleName.value = ''
}

async function loadHosts() {
  try {
    const data = await api('/api/hosts')
    const rawHosts = Array.isArray(data) ? data : (data.hosts || data || [])
    // 从 dashboard 获取角色数据并合并
    try {
      const dashData = await api('/api/dashboard')
      const hostsWithRoles = dashData.hosts || []
      const roleMap = {}
      hostsWithRoles.forEach(hwr => {
        const host = hwr.host || hwr
        if (hwr.roles && hwr.roles.length > 0) {
          roleMap[host.id] = hwr.roles.map(a => a.role ? a.role.name : '').filter(Boolean)
        }
        // 收集所有非 ICMP 角色的探测状态
        const roleProbes = []
        const nonIcmpRoles = (hwr.roles || []).filter(a => a.role && a.role.type !== 'ICMP')
        nonIcmpRoles.forEach(a => {
          roleProbes.push({
            name: a.role ? a.role.name : '',
            status: a.status,
            error: a.error_message || '',
          })
        })
        host.roleProbes = roleProbes
        // 兼容旧的单角色字段
        host.roleProbeName = roleProbes.length > 0 ? roleProbes[0].name : ''
        host.roleProbeStatus = roleProbes.length > 0 ? roleProbes[0].status : host.status
        host.roleProbeError = roleProbes.length > 0 ? roleProbes[0].error : ''
      })
      rawHosts.forEach(h => {
        h.roles = roleMap[h.id] || []
        // 从 dashboard 数据中复制 roleProbes 到真正使用的 host 对象
        const dh = hostsWithRoles.reduce((acc, hwr) => {
          const obj = hwr.host || hwr
          return obj.id === h.id ? obj : acc
        }, null)
        if (dh && dh.roleProbes) {
          h.roleProbes = dh.roleProbes
          h.roleProbeName = dh.roleProbeName || ''
          h.roleProbeStatus = dh.roleProbeStatus != null ? dh.roleProbeStatus : h.status
          h.roleProbeError = dh.roleProbeError || ''
        }
      })
    } catch(e2) {}
    hosts.value = rawHosts
  } catch(e) {}
}

async function loadRoles() {
  try {
    const data = await api('/api/roles')
    const raw = Array.isArray(data) ? data : (data.roles || data || [])
    // 排除 ICMP 探测角色（仅用于后台存活检测，不向前端展示）
    roles.value = raw.filter(r => r.type !== 'ICMP')
  } catch(e) {}
}

async function saveHost() {
  if (!formAddr.value) { showToast('请输入主机地址', 'error'); return }
  // 将磁盘分区序列化为 mount:size,mount:size 格式
  const diskParts = formDisk.value
    .filter(d => d.mount && d.size)
    .map(d => d.mount + ':' + d.size)
    .join(',')
  const body = {
    ip: formAddr.value,
    hostname: formName.value || formAddr.value,
    cpu: formCPU.value || '',
    memory: formMemory.value || '',
    disk: diskParts,
  }
  if (editingHost.value) {
    await api('/api/hosts/update', { method: 'POST', body: JSON.stringify({ id: editingHost.value.id, ...body }) })
    // 处理角色变更：如果角色改变，删除旧分配并创建新分配
    if (editingHost.value) {
      const oldRoleName = origRoleName.value
      if (formRole.value) {
        // 选择了新角色
        const oldRole = oldRoleName && oldRoleName !== formRole.value ? roles.value.find(r => r.name === oldRoleName) : null
        const newRole = roles.value.find(r => r.name === formRole.value)
        if (newRole) {
          try {
            // 变更了角色：先删旧分配，再建新分配
            if (oldRole) {
              try {
                await api('/api/assign?host_id=' + encodeURIComponent(editingHost.value.id) + '&role_id=' + encodeURIComponent(oldRole.id), { method: 'DELETE' })
              } catch(e) { /* 忽略删除错误 */ }
            }
            // 创建新分配
            await api('/api/assign', { method: 'POST', body: JSON.stringify({ host_id: editingHost.value.id, role_id: newRole.id }) })
            // 分配角色后触发探测，更新状态
            await api('/api/refresh')
            setTimeout(loadHosts, 2000)
          } catch(e) {
            // 角色已存在等错误忽略
          }
        }
      } else if (oldRoleName) {
        // 清空角色：删除旧分配
        const oldRole = roles.value.find(r => r.name === oldRoleName)
        if (oldRole) {
          try {
            await api('/api/assign?host_id=' + encodeURIComponent(editingHost.value.id) + '&role_id=' + encodeURIComponent(oldRole.id), { method: 'DELETE' })
          } catch(e) { /* 忽略删除错误 */ }
        }
      }
    }
  } else {
    await api('/api/hosts', { method: 'POST', body: JSON.stringify(body) })
  }
  closeModal()
  await loadHosts()
}

function editHost(host) {
  editingHost.value = host
  formAddr.value = host.ip || ''
  formName.value = host.hostname || ''
  formCPU.value = host.cpu || ''
  formMemory.value = host.memory || ''
  formRole.value = (host.roles && host.roles.length > 0) ? host.roles[0] : ''
  origRoleName.value = formRole.value  // 记录原始角色
  // 从 host.disk 字符串反解析回表格
  if (host.disk) {
    const parts = host.disk.split(',').filter(s => s.trim())
    formDisk.value = parts.map(p => {
      const segs = p.split(':')
      return { mount: segs[0] || '', size: segs[1] || '', unit: segs[2] || 'GB' }
    })
  } else {
    formDisk.value = [{ mount: '', size: '', unit: 'GB' }]
  }
}

function addDiskRow() {
  formDisk.value.push({ mount: '', size: '', unit: 'GB' })
}
function removeDiskRow(idx) {
  if (formDisk.value.length > 1) formDisk.value.splice(idx, 1)
}

async function deleteHost(host) {
  if (!confirm('确定删除主机 ' + (host.hostname || host.ip) + ' 吗？')) return
  await api('/api/hosts?id=' + host.id, { method: 'DELETE' })
  await loadHosts()
}

async function unbindRole(host, roleName) {
  if (!roleName) return
  if (!confirm('确定解除 ' + (host.hostname || host.ip) + ' 绑定的角色 "' + roleDisplayName(roleName) + '" 吗？')) return
  const role = roles.value.find(r => r.name === roleName)
  if (!role) { showToast('未找到角色', 'error'); return }
  try {
    await api('/api/assign?host_id=' + encodeURIComponent(host.id) + '&role_id=' + encodeURIComponent(role.id), { method: 'DELETE' })
    showToast('角色已解除', 'success')
    await loadHosts()
  } catch(e) { showToast('解除失败: ' + e.message, 'error') }
}

async function pingHost(host) {
  try {
    await api('/api/refresh')
    showToast('已触发全量探测: ' + (host.hostname || host.ip), 'success')
    setTimeout(loadHosts, 3000)
  } catch(e) {}
}

// Maintenance toggle
async function toggleMaintenance(host) {
  try {
    await api('/api/hosts/maintenance', { method: 'POST', body: JSON.stringify({ host_id: host.id, maintenance: !host.maintenance }) })
    showToast(host.maintenance ? '已恢复' : '已设为维护模式', 'success')
  } catch(e) { showToast('操作失败', 'error') }
  await loadHosts()
}

// Batch Add Hosts
function openBatchHostModal() {
  batchHostItems.value = [{ hostname: '', ip: '', cpu: '', memory: '', disk: '' }]
  showBatchHostModal.value = true
}
function addBatchHost() {
  batchHostItems.value.push({ hostname: '', ip: '', cpu: '', memory: '', disk: '' })
}
function removeBatchHost(idx) {
  if (batchHostItems.value.length > 1) batchHostItems.value.splice(idx, 1)
}
async function submitBatchHosts() {
  const valid = batchHostItems.value.filter(h => h.hostname && h.ip)
  if (valid.length === 0) { showToast('请填写至少一条有效的主机数据', 'error'); return }
  const hostList = valid.map(h => ({ hostname: h.hostname, ip: h.ip, cpu: h.cpu || '', memory: h.memory || '', disk: h.disk || '' }))
  await api('/api/hosts/batch', { method: 'POST', body: JSON.stringify({ hosts: hostList }) })
  showBatchHostModal.value = false
  showToast('批量添加 ' + valid.length + ' 台主机完成', 'success')
  await loadHosts()
}

// Add/Edit Role
function openAddRoleModal() {
  resetRoleForm()
  showAddRoleModal.value = true
}
function editRole(role) {
  showRoleManageModal.value = false  // 关闭角色管理弹窗，避免遮挡
  editingRole.value = role
  roleFormName.value = role.name
  roleFormType.value = role.type
  roleFormPort.value = role.port || 0
  roleFormPath.value = role.path || ''
  roleFormTimeout.value = role.timeout || 5
  showAddRoleModal.value = true
}
function resetRoleForm() {
  editingRole.value = null
  roleFormName.value = ''
  roleFormType.value = 'TCP'
  roleFormPort.value = 0
  roleFormPath.value = ''
  roleFormTimeout.value = 5
}

// 根据类型设置默认端口
const roleTypeDefaults = { 'HTTP': 80, 'TCP': 0, 'SSH': 22, 'ICMP': 0 }
function onRoleTypeChange() {
  if (roleFormType.value && roleTypeDefaults[roleFormType.value]) {
    roleFormPort.value = roleTypeDefaults[roleFormType.value]
  }
}
async function submitRoleForm() {
  if (!roleFormName.value) { showToast('请输入角色名称', 'error'); return }
  if (!roleFormType.value) { showToast('请选择探测类型', 'error'); return }
  const body = {
    name: roleFormName.value,
    type: roleFormType.value,
    port: roleFormPort.value || 0,
    path: roleFormPath.value || '',
    timeout: roleFormTimeout.value || 5,
  }
  if (editingRole.value) {
    await api('/api/roles?id=' + encodeURIComponent(editingRole.value.id), { method: 'PUT', body: JSON.stringify(body) })
    showToast('角色已更新', 'success')
    showAddRoleModal.value = false
    // 重新打开角色管理弹窗
    await loadRoles()
    showRoleManageModal.value = true
    return
  } else {
    await api('/api/roles', { method: 'POST', body: JSON.stringify(body) })
    showToast('角色添加成功', 'success')
  }
  showAddRoleModal.value = false
  await loadRoles()
}

// Assign Role
function openAssignModal() {
  assignHostId.value = ''
  assignRoleName.value = ''
  showAssignModal.value = true
  // 确保角色列表已加载
  if (roles.value.length === 0) loadRoles()
}
async function submitAssign() {
  if (!assignHostId.value || !assignRoleName.value) { showToast('请选择主机和角色', 'error'); return }
  const role = roles.value.find(r => r.name === assignRoleName.value)
  if (!role) { showToast('未找到所选角色', 'error'); return }
  await api('/api/assign', { method: 'POST', body: JSON.stringify({ host_id: assignHostId.value, role_id: role.id }) })
  showAssignModal.value = false
  showToast('角色分配成功', 'success')
  await loadHosts()
}

// Batch Create Roles
function openBatchRoleModal() {
  batchRoleItems.value = [{ name: '', type: 'TCP', port: 0, path: '/', timeout: 5 }]
  showBatchRoleModal.value = true
}
function addBatchRole() {
  batchRoleItems.value.push({ name: '', type: 'TCP', port: 0, path: '/', timeout: 5 })
}
function removeBatchRole(idx) {
  if (batchRoleItems.value.length > 1) batchRoleItems.value.splice(idx, 1)
}
async function submitBatchRoles() {
  const valid = batchRoleItems.value.filter(r => r.name && ['ICMP', 'TCP', 'HTTP', 'SSH'].includes(r.type))
  if (valid.length === 0) { showToast('请填写有效的角色数据', 'error'); return }
  const roleList = valid.map(r => ({ name: r.name, type: r.type.toUpperCase(), port: r.port || 0, path: r.path || '', timeout: r.timeout || 5 }))
  await api('/api/roles/batch', { method: 'POST', body: JSON.stringify({ roles: roleList }) })
  showBatchRoleModal.value = false
  showToast('批量创建 ' + valid.length + ' 个角色完成', 'success')
  await loadRoles()
}

// SSH Credential Management
function openSSHCredModal() {
  sshCredLabel.value = ''
  sshCredUsername.value = ''
  sshCredAuthMethod.value = 'password'
  sshCredPassword.value = ''
  sshCredPrivateKey.value = ''
  loadSSHCredentials()
  showSSHCredModal.value = true
}
async function loadSSHCredentials() {
  try {
    const data = await api('/api/ssh-credential')
    sshCredentials.value = Array.isArray(data) ? data : (data || [])
  } catch(e) {
    sshCredentials.value = []
  }
}
async function submitSSHCred() {
  if (!sshCredLabel.value || !sshCredUsername.value) { showToast('标签和用户名不能为空', 'error'); return }
  const body = { label: sshCredLabel.value, username: sshCredUsername.value, auth_method: sshCredAuthMethod.value }
  if (sshCredAuthMethod.value === 'password') {
    body.password = sshCredPassword.value
  } else {
    body.private_key = sshCredPrivateKey.value
  }
  await api('/api/ssh-credential', { method: 'POST', body: JSON.stringify(body) })
  sshCredPassword.value = ''
  sshCredPrivateKey.value = ''
  showToast('SSH 凭据已添加', 'success')
  await loadSSHCredentials()
}
async function deleteSSHCred(id) {
  if (!confirm('确定删除该凭据？')) return
  await api('/api/ssh-credential?id=' + id, { method: 'DELETE' })
  showToast('凭据已删除', 'success')
  await loadSSHCredentials()
}

// Excel Export
async function downloadExcel() {
  try {
    const res = await fetch('/api/hosts/export', { credentials: 'same-origin' })
    if (!res.ok) { showToast('导出失败', 'error'); return }
    const blob = await res.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a'); a.href = url; a.download = 'hosts_export.xlsx'
    document.body.appendChild(a); a.click(); document.body.removeChild(a)
    URL.revokeObjectURL(url)
    showToast('导出成功', 'success')
  } catch(e) { showToast('导出失败: ' + e.message, 'error') }
}

// Terminal
async function openTerminal(host) {
  // 先加载凭据列表，弹出凭据选择对话框
  pendingTerminalHost.value = host
  sshSelectCredId.value = ''
  sshManualUser.value = 'root'
  sshManualPass.value = ''
  try {
    const data = await api('/api/ssh-credential')
    sshSelectCredentials.value = Array.isArray(data) ? data : (data || [])
  } catch(e) {
    sshSelectCredentials.value = []
  }
  showSSHSelectModal.value = true
}

function cancelSSHConnect() {
  showSSHSelectModal.value = false
  pendingTerminalHost.value = null
  sshSelectCredId.value = ''
  sshManualUser.value = 'root'
  sshManualPass.value = ''
}

async function confirmSSHConnect() {
  if (!pendingTerminalHost.value) return
  // 确定凭据
  if (sshSelectCredId.value) {
    // 使用已保存的凭据
    terminalHost.value = pendingTerminalHost.value
    termUser.value = ''  // 凭据模式下不需要手动输入
    termPass.value = ''
    termCredMode.value = true  // 标记为凭据模式，隐藏手动输入框
    showSSHSelectModal.value = false
    pendingTerminalHost.value = null
    await nextTick()
    await initTerminal()
    // 通过 cred_id 连接
    await nextTick()
    connectWithCred(sshSelectCredId.value)
  } else if (sshManualUser.value) {
    // 手动认证
    terminalHost.value = pendingTerminalHost.value
    termUser.value = sshManualUser.value
    termPass.value = sshManualPass.value
    termCredMode.value = false
    showSSHSelectModal.value = false
    pendingTerminalHost.value = null
    await nextTick()
    await initTerminal()
    await nextTick()
    connectTerminal()
  } else {
    showToast('请选择凭据或输入用户名', 'error')
  }
}

async function initTerminal() {
  await nextTick()
  const el = document.getElementById('terminal-container')
  if (!el) return
  const { Terminal } = await import('xterm')
  const { FitAddon } = await import('xterm-addon-fit')
  terminal = new Terminal({ cursorBlink: true, fontSize: 13, fontFamily: "'JetBrains Mono','Fira Code',monospace", theme: { background: '#1a1b26', foreground: '#c0caf5', cursor: '#c0caf5', selectionBackground: '#334155' } })
  const fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.open(el)
  fitAddon.fit()
  terminal.onData(data => { if (termSocket && termSocket.readyState === WebSocket.OPEN) termSocket.send(data) })
}

function connectWithCred(credId) {
  if (!terminalHost.value) return
  disconnectTerminal()
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = proto + '//' + location.host + '/api/ssh/ws?host_id=' + encodeURIComponent(terminalHost.value.id) + '&host=' + encodeURIComponent(terminalHost.value.ip) + '&port=22&cred_id=' + encodeURIComponent(credId)
  try {
    termSocket = new WebSocket(wsUrl)
    termSocket.binaryType = 'arraybuffer'
    termSocket.onopen = () => { termConnected.value = true; terminal.focus() }
    termSocket.onmessage = (e) => {
      if (terminal) {
        if (e.data instanceof ArrayBuffer) {
          terminal.write(new Uint8Array(e.data))
        } else {
          terminal.write(e.data)
        }
      }
    }
    termSocket.onerror = () => { showToast('SSH 连接失败', 'error'); termConnected.value = false }
    termSocket.onclose = () => { termConnected.value = false }
  } catch(e) { showToast('SSH 连接失败', 'error') }
}

function connectTerminal() {
  if (!terminalHost.value) return
  disconnectTerminal()
  const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const wsUrl = proto + '//' + location.host + '/api/ssh/ws?host_id=' + encodeURIComponent(terminalHost.value.id) + '&host=' + encodeURIComponent(terminalHost.value.ip) + '&port=22&user=' + encodeURIComponent(termUser.value) + '&pass=' + encodeURIComponent(termPass.value)
  try {
    termSocket = new WebSocket(wsUrl)
    termSocket.binaryType = 'arraybuffer'
    termSocket.onopen = () => {
      termConnected.value = true
      terminal.focus()
      // 手动认证：发送认证信息
      if (termUser.value) {
        termSocket.send(JSON.stringify({ type: 'manual', username: termUser.value, password: termPass.value, auth_method: 'password' }))
      }
    }
    termSocket.onmessage = (e) => {
      if (terminal) {
        if (e.data instanceof ArrayBuffer) {
          terminal.write(new Uint8Array(e.data))
        } else {
          terminal.write(e.data)
        }
      }
    }
    termSocket.onerror = () => { showToast('SSH 连接失败', 'error'); termConnected.value = false }
    termSocket.onclose = () => { termConnected.value = false }
  } catch(e) { showToast('SSH 连接失败', 'error') }
}

function disconnectTerminal() {
  if (termSocket) { try { termSocket.close() } catch(e) {} termSocket = null }
  termConnected.value = false
}

function closeTerminal() {
  disconnectTerminal()
  if (terminal) { terminal.dispose(); terminal = null }
  terminalHost.value = null
  pendingTerminalHost.value = null
  termCredMode.value = false
  sshSelectCredId.value = ''
  sshManualUser.value = 'root'
  sshManualPass.value = ''
}

onMounted(() => {
  loadHosts()
  loadRoles()
})
onUnmounted(() => { disconnectTerminal(); if (terminal) { terminal.dispose(); terminal = null } })
</script>

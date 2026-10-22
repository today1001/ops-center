<template>
  <div class="hv-manager">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>{{ meta.title }}</span>
          <div class="header-actions">
            <el-button size="small" :icon="Refresh" circle @click="loadHosts" :loading="loading" />
            <el-button type="primary" :icon="Plus" @click="openDialog()">添加主机</el-button>
          </div>
        </div>
      </template>

      <el-alert
        :title="meta.tip"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 15px;"
      />

      <el-table :data="hosts" v-loading="loading" border>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" min-width="150">
          <template #default="{ row }">
            <span class="hv-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="地址" min-width="160">
          <template #default="{ row }">
            <span class="mono">{{ row.ip }}</span>
            <span class="port">:{{ displayPort(row) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" min-width="110">
          <template #default="{ row }">
            <span :class="row.username ? '' : 'muted'">{{ row.username || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="认证域" width="100" v-if="type === 'pve'">
          <template #default="{ row }">
            <el-tag size="small" type="info" effect="plain">{{ row.realm || 'pam' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <StatusDot :status="statusOf(row).kind" />
            <span :style="{ color: statusOf(row).color, fontWeight: 500 }">
              {{ statusOf(row).text }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="最近检查" width="170">
          <template #default="{ row }">
            <span :class="row.last_check ? '' : 'muted'">{{ row.last_check || '未检查' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="330" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="success" plain :loading="row.__testing" @click="handleTest(row)">
              测试连接
            </el-button>
            <el-button size="small" plain :loading="row.__loading" @click="openInventory(row)">
              清单
            </el-button>
            <el-button size="small" type="primary" plain :icon="TopRight" @click="openConsole(row)">
              控制台
            </el-button>
            <el-button size="small" @click="openDialog(row)">编辑</el-button>
            <el-popconfirm title="确定删除该主机吗？" @confirm="handleDelete(row)">
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty :description="`暂无${meta.title}主机，点击右上角「添加主机」创建`" :image-size="60" />
        </template>
      </el-table>
    </el-card>

    <!-- 新增 / 编辑 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑主机' : '添加主机'" width="520px"
      :close-on-click-modal="false">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="例如: 生产ESXi-01" maxlength="50" show-word-limit />
        </el-form-item>
        <el-form-item label="IP 地址" prop="ip">
          <el-input v-model="form.ip" :placeholder="meta.ipPh" />
        </el-form-item>
        <el-form-item label="端口" prop="port">
          <el-input-number v-model="form.port" :min="0" :max="65535" :step="1" controls-position="right"
            style="width: 180px;" />
          <span class="form-hint">0 = 使用默认端口 {{ meta.defaultPort }}</span>
        </el-form-item>
        <el-form-item label="用户名" prop="username">
          <el-input v-model="form.username" :placeholder="meta.userPh" />
        </el-form-item>
        <el-form-item label="认证域" prop="realm" v-if="type === 'pve'">
          <el-select v-model="form.realm" style="width: 100%;">
            <el-option label="pam（本地账号，默认）" value="pam" />
            <el-option label="pve（PVE 内建用户）" value="pve" />
            <el-option label="ldap" value="ldap" />
            <el-option label="ad" value="ad" />
          </el-select>
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" show-password
            :placeholder="form.id ? '留空则保留原密码' : '主机登录密码'" autocomplete="new-password" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">保存</el-button>
      </template>
    </el-dialog>

    <!-- 清单 -->
    <el-dialog v-model="invVisible" :title="`资源清单 - ${invTitle}`" width="82%" top="6vh">
      <div v-if="invLoading" class="inv-loading" v-loading="invLoading" style="min-height: 200px;">
        <span class="inv-tip">{{ invTip }}</span>
      </div>

      <div v-else-if="inv && !inv.auth_ok" class="inv-error">
        <el-alert :title="inv.message || '连接失败'" type="error" show-icon :closable="false" />
      </div>

      <div v-else-if="inv && inv.nodes && inv.nodes.length">
        <el-tabs type="border-card">
          <el-tab-pane v-for="n in inv.nodes" :key="n.name" :label="n.name">
            <div v-if="n.server && (n.server.cpu_count || n.server.mem_total_gb)" class="server-card">
              <div class="sc-row">
                <span class="sc-title">{{ n.server.name || n.name }}</span>
                <span class="mono muted">{{ n.server.version }}</span>
                <el-tag size="small" effect="plain" :type="n.server.state === 'running' ? 'success' : 'info'">
                  {{ stateText(n.server.state) }}
                </el-tag>
                <span v-if="n.server.ip" class="mono muted">{{ n.server.ip }}</span>
              </div>
              <div class="sc-grid">
                <div class="sc-item">
                  <span class="sc-label">CPU</span>
                  <span class="sc-value">{{ n.server.cpu_count || '-' }} 核
                    <span v-if="n.server.cpu_usage > 0" class="sc-sub">使用 {{ n.server.cpu_usage.toFixed(1) }}%</span>
                  </span>
                </div>
                <div class="sc-item">
                  <span class="sc-label">内存</span>
                  <span class="sc-value">
                    {{ n.server.mem_used_gb.toFixed(1) }} / {{ n.server.mem_total_gb.toFixed(1) }} GB
                    <span v-if="n.server.mem_total_gb > 0" class="sc-sub">{{ n.server.mem_usage.toFixed(1) }}%</span>
                  </span>
                </div>
                <div class="sc-item">
                  <span class="sc-label">运行时长</span>
                  <span class="sc-value">{{ uptimeText(n.server.uptime_sec) }}</span>
                </div>
                <div class="sc-item">
                  <span class="sc-label">实例数</span>
                  <span class="sc-value">{{ (n.vms || []).length }}</span>
                </div>
              </div>
              <div v-if="n.server.disk_info" class="sc-note">{{ n.server.disk_info }}</div>
              <div v-if="n.server.notes && n.server.notes.length" class="sc-note">
                <span v-for="(note, i) in n.server.notes" :key="i" class="note-chip">{{ note }}</span>
              </div>
            </div>

            <div class="vm-title">虚拟机 / 容器（{{ (n.vms || []).length }}）</div>
            <el-table :data="n.vms || []" border size="small" style="width: 100%">
              <el-table-column prop="id" label="ID" width="70">
                <template #default="{ row }">
                  <span class="mono">{{ row.id }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="type" label="类型" width="70">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.type === 'lxc' ? 'warning' : 'primary'" effect="plain">
                    {{ row.type === 'lxc' ? 'LXC' : 'VM' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="name" label="名称" min-width="180" show-overflow-tooltip />
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <span :style="{ color: vmRunning(row) ? '#67C23A' : '#909399' }">
                    {{ stateText(row.state) }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column label="IP" min-width="130">
                <template #default="{ row }">
                  <span :class="row.ip ? 'mono' : 'muted'">{{ row.ip || '-' }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="cpus" label="vCPU" width="70" align="right" />
              <el-table-column label="内存" width="90" align="right">
                <template #default="{ row }">{{ row.mem_gb ? row.mem_gb.toFixed(1) + ' GB' : '-' }}</template>
              </el-table-column>
              <el-table-column label="操作系统" min-width="130" show-overflow-tooltip>
                <template #default="{ row }">
                  <span :class="row.os ? '' : 'muted'">{{ row.os || '-' }}</span>
                </template>
              </el-table-column>
              <template #empty>
                <el-empty description="该节点下没有虚拟机或容器" :image-size="50" />
              </template>
            </el-table>
          </el-tab-pane>
        </el-tabs>
      </div>

      <div v-else class="inv-error">
        <el-alert :title="inv && inv.message ? inv.message : '未获取到清单数据'" type="warning" show-icon
          :closable="false" />
      </div>

      <template #footer>
        <el-button @click="invVisible = false">关闭</el-button>
        <el-button type="primary" :loading="invLoading" @click="refreshInventory">刷新</el-button>
      </template>
    </el-dialog>

    <!-- 控制台（需管理员验证） -->
    <el-dialog v-model="consoleVisible" width="94%" top="3vh" :close-on-click-modal="false"
      :title="consoleHost ? `管理控制台 - ${consoleHost.name} (${consoleHost.ip})` : '管理控制台'">

      <div v-if="!consoleAuthed">
        <el-alert title="密码加密存储，普通用户不可见。请输入您的登录密码进行身份验证，验证通过后获取主机密码并打开厂商 Web 控制台。"
          type="warning" show-icon :closable="false" style="margin-bottom: 15px;" />
        <el-form label-width="90px" @submit.prevent>
          <el-form-item label="登录密码">
            <el-input v-model="consolePwd" type="password" show-password placeholder="您的 OpsCenter 登录密码"
              autocomplete="new-password" @keyup.enter="doConsoleAuth" style="width: 320px;" />
          </el-form-item>
        </el-form>
        <div style="margin-left: 90px;">
          <el-button type="primary" @click="doConsoleAuth" :loading="consoleLoading">验证并打开</el-button>
        </div>
      </div>

      <div v-else class="console-frame" v-loading="consoleLoading">
        <iframe v-if="consoleSrc" :src="consoleSrc" class="console-iframe" frameborder="0"></iframe>
        <el-empty v-else description="控制台加载中，部分厂商 Web 客户端可能限制嵌入显示" />
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, TopRight } from '@element-plus/icons-vue'
import { hypervisorAPI, toolsAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'

const route = useRoute()

// type 由路由 meta 决定：esxi | pve
const type = computed(() => route.meta.hvType === 'pve' ? 'pve' : 'esxi')

const meta = computed(() => type.value === 'pve' ? {
  title: 'PVE 管理',
  defaultPort: 8006,
  ipPh: '例如: 192.168.1.100',
  userPh: '例如: root',
  tip: '管理 Proxmox VE 节点：录入地址与 root@pam 账号密码，可测试连通性与认证、采集节点资源和虚拟机 / LXC 容器清单，并打开 PVE Web 控制台。密码加密存储，仅管理员验证身份后可见。'
} : {
  title: 'ESXi 管理',
  defaultPort: 443,
  ipPh: '例如: 192.168.1.200',
  userPh: '例如: root',
  tip: '管理 VMware ESXi 主机（需 ESXi 6.7 及以上，通过 vSphere REST API 采集）：录入地址与 root 账号密码，可测试连通性与认证、采集主机资源与虚拟机清单，并打开 ESXi Host Client。密码加密存储，仅管理员验证身份后可见。'
})

const hosts = ref([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const formRef = ref(null)

const form = reactive({
  id: 0,
  name: '',
  type: 'esxi',
  ip: '',
  port: 0,
  username: '',
  realm: 'pam',
  password: ''
})

const formRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  ip: [
    { required: true, message: '请输入 IP 地址', trigger: 'blur' },
    { pattern: /^(\d{1,3}\.){3}\d{1,3}$/, message: 'IP 格式不正确', trigger: 'blur' }
  ]
}

const loadHosts = async () => {
  loading.value = true
  try {
    hosts.value = await hypervisorAPI.getList(type.value)
  } catch (e) {
    ElMessage.error('加载主机列表失败')
  } finally {
    loading.value = false
  }
}

const displayPort = (row) => row.port > 0 ? row.port : (row.type === 'pve' ? 8006 : 443)

const openDialog = (row) => {
  Object.assign(form, {
    id: row ? row.id : 0,
    name: row ? row.name : '',
    type: type.value,
    ip: row ? row.ip : '',
    port: row ? (row.port || 0) : 0,
    username: row ? (row.username || '') : (type.value === 'pve' ? 'root' : 'root'),
    realm: row ? (row.realm || 'pam') : 'pam',
    password: ''
  })
  dialogVisible.value = true
}

const handleSubmit = async () => {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const data = {
      name: form.name,
      type: type.value,
      ip: form.ip,
      port: form.port || 0,
      username: form.username,
      realm: type.value === 'pve' ? (form.realm || 'pam') : '',
      password: form.password
    }
    if (form.id) {
      await hypervisorAPI.update(form.id, data)
      ElMessage.success('更新成功')
    } else {
      await hypervisorAPI.create(data)
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadHosts()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    submitting.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await hypervisorAPI.delete(row.id)
    ElMessage.success('删除成功')
    loadHosts()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

// 测试连接
const handleTest = async (row) => {
  row.__testing = true
  try {
    const res = await hypervisorAPI.test(row.id)
    if (res.auth_ok) {
      ElMessage.success(`连接成功：${res.nodes ? res.nodes.length : 0} 个节点，` +
        `${(res.nodes || []).reduce((s, n) => s + (n.vms || []).length, 0)} 个实例`)
    } else {
      ElMessage.warning(res.message || '连接失败')
    }
    loadHosts()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '测试失败')
  } finally {
    row.__testing = false
  }
}

// 清单
const invVisible = ref(false)
const invLoading = ref(false)
const invTip = ref('正在通过厂商 API 采集，视规模可能需要十几秒...')
const inv = ref(null)
const invHost = ref(null)

const invTitle = computed(() => invHost.value ?
  `${invHost.value.name} (${invHost.value.ip})` : '')

const openInventory = (row) => {
  invHost.value = row
  inv.value = null
  invTip.value = '正在通过厂商 API 采集，视规模可能需要十几秒...'
  invVisible.value = true
  refreshInventory()
}

const refreshInventory = async () => {
  if (!invHost.value) return
  invLoading.value = true
  inv.value = null
  try {
    inv.value = await hypervisorAPI.getInventory(invHost.value.id)
  } catch (e) {
    inv.value = { auth_ok: false, message: e.response?.data?.error || '采集失败' }
  } finally {
    invLoading.value = false
  }
}

// 控制台
const consoleVisible = ref(false)
const consoleLoading = ref(false)
const consolePwd = ref('')
const consoleAuthed = ref(false)
const consoleSrc = ref('')
const consoleHost = ref(null)

const openConsole = (row) => {
  consoleHost.value = row
  consolePwd.value = ''
  consoleAuthed.value = false
  consoleSrc.value = ''
  consoleVisible.value = true
}

const doConsoleAuth = async () => {
  if (!consolePwd.value) {
    ElMessage.warning('请输入登录密码')
    return
  }
  consoleLoading.value = true
  try {
    const { password } = await hypervisorAPI.getPassword(consoleHost.value.id, consolePwd.value)
    const host = consoleHost.value
    const port = displayPort(host)
    const isDefault = (type.value === 'esxi' && port === 443)
    const url = isDefault
      ? `https://${host.ip}/`
      : `https://${host.ip}:${port}/`
    const { tk } = await toolsAPI.createWebSession({
      url,
      user: host.username || '',
      pass: password || '',
      lang: 'zh_CN'
    })
    consoleSrc.value = `/px/${tk}/`
    consoleAuthed.value = true
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '打开控制台失败')
  } finally {
    consoleLoading.value = false
  }
}

// 状态与文案
const statusOf = (row) => {
  switch (row.status) {
    case 'online': return { kind: 'active', text: '在线', color: '#67C23A' }
    case 'auth_failed': return { kind: 'unreachable', text: '认证失败', color: '#E6A23C' }
    case 'offline': return { kind: 'offline', text: '离线', color: '#F56C6C' }
    default: return { kind: 'busy', text: '未知', color: '#909399' }
  }
}

const stateText = (s) => {
  const m = {
    running: '运行中', stopped: '已停止', paused: '已暂停', up: '在线',
    powered_on: '运行中', powered_off: '已关闭', not_powered_on: '未开启',
    unknown: '未知', '': '-'
  }
  if (!s) return '-'
  const v = String(s).toLowerCase().replace(/^guest_state_/, '')
  return m[v] || s
}

const vmRunning = (row) => {
  const s = String(row.state || '').toLowerCase()
  return s === 'running' || s === 'up'
}

const uptimeText = (sec) => {
  if (!sec) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  if (d > 0) return `${d} 天 ${h} 小时`
  if (h > 0) return `${h} 小时 ${m} 分`
  return `${m} 分钟`
}

loadHosts()
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.header-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.hv-name {
  font-weight: 500;
}

.mono {
  font-family: Consolas, Monaco, monospace;
}

.port {
  color: #909399;
  margin-left: 2px;
}

.muted {
  color: #909399;
}

.form-hint {
  color: #909399;
  font-size: 12px;
  margin-left: 10px;
}

.inv-tip {
  display: block;
  text-align: center;
  padding: 60px 0;
  color: #909399;
}

.inv-error {
  padding: 8px 0;
}

.server-card {
  border: 1px solid #ebeef5;
  border-radius: 6px;
  padding: 14px 16px;
  margin-bottom: 14px;
  background: #fafbfc;
}

.sc-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.sc-title {
  font-size: 15px;
  font-weight: 600;
}

.sc-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(190px, 1fr));
  gap: 10px 24px;
}

.sc-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sc-label {
  color: #909399;
  font-size: 12px;
}

.sc-value {
  font-weight: 500;
  font-size: 14px;
}

.sc-sub {
  color: #909399;
  font-weight: 400;
  font-size: 12px;
  margin-left: 4px;
}

.sc-note {
  margin-top: 10px;
  color: #606266;
  font-size: 13px;
}

.note-chip {
  display: inline-block;
  border: 1px solid #dcdfe6;
  border-radius: 3px;
  padding: 1px 7px;
  margin-right: 6px;
  font-size: 12px;
  color: #909399;
  background: #fff;
}

.vm-title {
  font-weight: 600;
  margin-bottom: 8px;
}

.console-frame {
  height: 78vh;
}

.console-iframe {
  width: 100%;
  height: 100%;
  border: 1px solid #ebeef5;
  border-radius: 4px;
}
</style>

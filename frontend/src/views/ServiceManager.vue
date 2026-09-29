<template>
  <div class="service-manager">
    <el-row :gutter="20">
      <!-- 左侧服务器列表 -->
      <el-col :span="6">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>服务器</span>
              <el-button size="small" :icon="Refresh" circle @click="loadServers" :loading="serversLoading" />
            </div>
          </template>
          <el-input v-model="serverKeyword" placeholder="搜索服务器..." clearable size="small" style="margin-bottom: 10px;" />
          <el-collapse v-model="activeGroups" class="server-collapse" v-loading="serversLoading">
            <el-collapse-item v-for="group in groupedServers" :key="group.name" :name="group.name">
              <template #title>
                <div class="group-title">
                  <span class="group-name">{{ group.name }}</span>
                  <el-tag size="small" type="info" class="group-count">{{ group.servers.length }}台</el-tag>
                </div>
              </template>
              <div class="server-list">
                <div
                  v-for="s in group.servers"
                  :key="s.id"
                  class="server-item"
                  :class="{ active: selectedServer && selectedServer.id === s.id }"
                  @click="selectServer(s)"
                >
                  <StatusDot :status="s.status" />
                  <div class="server-info">
                    <div class="server-name">{{ s.name }}</div>
                    <div class="server-ip">{{ s.ip }}</div>
                  </div>
                </div>
              </div>
            </el-collapse-item>
            <el-empty v-if="groupedServers.length === 0" description="暂无服务器" :image-size="60" />
          </el-collapse>
        </el-card>
      </el-col>

      <!-- 右侧服务列表 -->
      <el-col :span="18">
        <el-card>
          <template #header>
            <div class="card-header">
              <span v-if="selectedServer">服务管理 - {{ selectedServer.name }} ({{ selectedServer.ip }})</span>
              <span v-else>服务管理</span>
              <div class="header-actions">
                <el-button size="small" :icon="Refresh" @click="loadServices" :disabled="!selectedServer" :loading="servicesLoading">
                  刷新
                </el-button>
                <el-button size="small" type="primary" :icon="Plus" @click="openDialog()" :disabled="!selectedServer">
                  添加服务
                </el-button>
              </div>
            </div>
          </template>

          <el-empty v-if="!selectedServer" description="请先选择左侧的服务器" />
          <template v-else>
            <el-table :data="services" v-loading="servicesLoading" style="width: 100%">
              <el-table-column label="服务名称" min-width="140">
                <template #default="{ row }">
                  <span class="svc-name link" @click="openDetail(row)">{{ row.name }}</span>
                </template>
              </el-table-column>
              <el-table-column label="访问方式" width="95">
                <template #default="{ row }">
                  <el-tag size="small" :type="methodTagType(row.access_method)">{{ row.access_method }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="访问地址" min-width="150">
                <template #default="{ row }">
                  <span v-if="row.address">{{ row.address }}<template v-if="row.port">:{{ row.port }}</template></span>
                  <span v-else>{{ selectedServer.ip }}<template v-if="row.port">:{{ row.port }}</template></span>
                </template>
              </el-table-column>
              <el-table-column label="连通率" width="90">
                <template #default="{ row }">
                  <span :style="{ color: connInfo(row).color, fontWeight: 500 }">{{ connInfo(row).text }}</span>
                  <el-tooltip v-if="row.total_checks" :content="`成功 ${row.success_checks} / 共 ${row.total_checks} 次`">
                    <span class="conn-dot" :style="{ background: connInfo(row).color }"></span>
                  </el-tooltip>
                </template>
              </el-table-column>
              <el-table-column label="运行版本" min-width="110" show-overflow-tooltip>
                <template #default="{ row }">
                  <span :class="row.last_version ? '' : 'muted'">{{ row.last_version || '未检测' }}</span>
                </template>
              </el-table-column>
              <el-table-column label="运行状态" width="95">
                <template #default="{ row }">
                  <StatusDot :status="row.status === 'running' ? 'active' : (row.status === 'unknown' ? 'busy' : 'offline')" />
                  <span :style="{ color: svcStatusColor(row.status) }" class="svc-status">{{ svcStatusText(row.status) }}</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" fixed="right" width="460">
                <template #default="{ row }">
                  <el-button size="small" type="success" plain v-if="isWebMethod(row.access_method)" @click="openWebAccess(row)">
                    连接
                  </el-button>
                  <el-button size="small" type="success" plain v-if="isSshMethod(row.access_method)" @click="openSshAccess(row)">
                    连接
                  </el-button>
                  <el-button size="small" type="warning" plain v-if="isRdpMethod(row.access_method)" @click="openRdpAccess(row)">
                    远程桌面
                  </el-button>
                  <el-button size="small" type="warning" plain v-if="isRdpMethod(row.access_method)" @click="openRdpAccess(row, true)">
                    影子
                  </el-button>
                  <el-button size="small" type="warning" plain v-if="isSshMethod(row.access_method)" @click="openSshShadow(row)">
                    影子
                  </el-button>
                  <el-button size="small" @click="handleTest(row)" :loading="row._testing">检测</el-button>
                  <el-dropdown trigger="click">
                    <el-button size="small" type="primary" plain>编辑<i class="el-icon--right"><el-icon><ArrowDown /></el-icon></i></el-button>
                    <template #dropdown>
                      <el-dropdown-menu>
                        <el-dropdown-item @click="openDialog(row)">编辑</el-dropdown-item>
                        <el-dropdown-item @click="openDetail(row)">详情</el-dropdown-item>
                        <el-dropdown-item divided @click="handleDelete(row)" style="color: #F56C6C;">删除</el-dropdown-item>
                      </el-dropdown-menu>
                    </template>
                  </el-dropdown>
                </template>
              </el-table-column>
            </el-table>
          </template>
        </el-card>
      </el-col>
    </el-row>

    <!-- 服务详情抽屉 -->
    <el-drawer v-model="detailVisible" title="服务详情" size="46%" :destroy-on-close="true">
      <div v-loading="detailLoading">
        <template v-if="detail">
          <el-descriptions :column="2" border size="small" class="detail-desc">
            <el-descriptions-item label="服务名称">{{ detail.service.name }}</el-descriptions-item>
            <el-descriptions-item label="访问方式">
              <el-tag size="small" :type="methodTagType(detail.service.access_method)">{{ detail.service.access_method }}</el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="访问地址">{{ detail.service.address || detail.service.server_ip }}</el-descriptions-item>
            <el-descriptions-item label="端口">{{ detail.service.port || '-' }}</el-descriptions-item>
            <el-descriptions-item label="所属服务器">{{ detail.service.server_name }} ({{ detail.service.server_ip }})</el-descriptions-item>
            <el-descriptions-item label="用户名">{{ detail.service.username || '-' }}</el-descriptions-item>
            <el-descriptions-item label="运行状态">
              <StatusDot :status="detail.service.status === 'running' ? 'active' : (detail.service.status === 'unknown' ? 'busy' : 'offline')" />
              <span :style="{ color: svcStatusColor(detail.service.status) }">{{ svcStatusText(detail.service.status) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="备注">{{ detail.service.description || '-' }}</el-descriptions-item>
          </el-descriptions>

          <div class="detail-section">
            <div class="section-title">连通性分析</div>
            <div class="conn-panel">
              <div class="conn-rate">
                <div class="rate-value" :style="{ color: rateColor(detail.connectivity_rate) }">
                  {{ detail.total_checks ? detail.connectivity_rate.toFixed(0) + '%' : '暂无' }}
                </div>
                <div class="rate-label">连通率</div>
              </div>
              <div class="conn-stats">
                <div class="stat-item">
                  <div class="stat-value" style="color:#67C23A;">{{ detail.success_checks }}</div>
                  <div class="stat-label">成功次数</div>
                </div>
                <div class="stat-item">
                  <div class="stat-value" style="color:#F56C6C;">{{ detail.total_checks - detail.success_checks }}</div>
                  <div class="stat-label">失败次数</div>
                </div>
                <div class="stat-item">
                  <div class="stat-value">{{ detail.total_checks }}</div>
                  <div class="stat-label">检测次数</div>
                </div>
              </div>
              <el-progress
                class="conn-progress"
                :percentage="detail.total_checks ? Math.round(detail.connectivity_rate) : 0"
                :color="rateColor(detail.connectivity_rate)"
                :stroke-width="10"
              />
            </div>
            <div v-if="detail.checks && detail.checks.length" class="latest-info">
              最近检测：<b>{{ detail.checks[0].success ? '成功' : '失败' }}</b>，
              耗时 {{ detail.checks[0].response_time_ms }}ms<template v-if="detail.checks[0].version">，
              版本 {{ detail.checks[0].version }}</template>，
              {{ formatTime(detail.checks[0].created_at) }}
            </div>
          </div>

          <div class="detail-section">
            <div class="section-title">检测历史（最近 {{ detail.checks ? detail.checks.length : 0 }} 次）</div>
            <el-table :data="detail.checks" size="small" max-height="300">
              <el-table-column label="时间" width="160">
                <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
              </el-table-column>
              <el-table-column label="结果" width="70">
                <template #default="{ row }">
                  <span :style="{ color: row.success ? '#67C23A' : '#F56C6C', fontWeight: 500 }">
                    {{ row.success ? '成功' : '失败' }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column label="耗时" width="80">
                <template #default="{ row }">{{ row.response_time_ms }}ms</template>
              </el-table-column>
              <el-table-column label="版本" width="120" show-overflow-tooltip>
                <template #default="{ row }">
                  <span :class="row.version ? '' : 'muted'">{{ row.version || '-' }}</span>
                </template>
              </el-table-column>
              <el-table-column label="检测信息" show-overflow-tooltip>
                <template #default="{ row }">{{ row.message }}</template>
              </el-table-column>
            </el-table>
          </div>
        </template>
      </div>

      <template #footer>
        <div class="drawer-footer">
          <el-button @click="detailVisible = false">关闭</el-button>
          <el-button @click="handleDetailTest" :loading="detailTesting">重新检测</el-button>
          <el-button
            v-if="detail && isWebMethod(detail.service.access_method)"
            type="success"
            @click="openWebAccess(detail.service)"
          >
            <el-icon><TopRight /></el-icon>
            连接
          </el-button>
          <el-button
            v-if="detail && isSshMethod(detail.service.access_method)"
            type="success"
            @click="openSshAccess(detail.service)"
          >
            <el-icon><TopRight /></el-icon>
            连接
          </el-button>
          <el-button
            v-if="detail && isRdpMethod(detail.service.access_method)"
            type="warning"
            @click="openRdpAccess(detail.service)"
          >
            <el-icon><TopRight /></el-icon>
            远程桌面
          </el-button>
          <el-button
            v-if="detail && isRdpMethod(detail.service.access_method)"
            type="warning"
            plain
            @click="openRdpAccess(detail.service, true)"
          >
            <el-icon><TopRight /></el-icon>
            影子
          </el-button>
        </div>
      </template>
    </el-drawer>

    <!-- 添加/编辑对话框 -->
    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑服务' : '添加服务'" width="560px">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="90px">
        <el-form-item label="服务名称" prop="name">
          <el-input v-model="form.name" placeholder="例如: Nginx Web、MySQL数据库、远程桌面" />
        </el-form-item>
        <el-form-item label="访问方式" prop="access_method">
          <el-select v-model="form.access_method" style="width: 100%;">
            <el-option v-for="m in accessMethods" :key="m" :label="m" :value="m" />
          </el-select>
        </el-form-item>
        <el-form-item label="访问地址">
          <el-input v-model="form.address" :placeholder="`默认为服务器IP (${selectedServer ? selectedServer.ip : ''})`">
            <template #prepend v-if="form.access_method === 'HTTP' || form.access_method === 'HTTPS'">
              {{ form.access_method.toLowerCase() }}://
            </template>
          </el-input>
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="form.port" :min="0" :max="65535" placeholder="0 表示不填" />
          <span class="tip">0 表示不指定端口</span>
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="form.username" placeholder="服务登录用户名" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" show-password placeholder="服务登录密码" />
        </el-form-item>
        <el-form-item label="运行状态">
          <el-select v-model="form.status" style="width: 100%;">
            <el-option label="运行中" value="running" />
            <el-option label="已停止" value="stopped" />
            <el-option label="未知" value="unknown" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="备注信息" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">保存</el-button>
      </template>
    </el-dialog>

    <!-- 网页访问弹窗 -->
    <WebAccessDialog
      v-if="webService"
      :service="webService"
      @close="webService = null"
    />
    <SshTerminal
      v-if="sshService && sshToken"
      :host="sshService.server_ip"
      :port="sshService.port || 22"
      :user="sshService.username"
      :pass="sshService.password || ''"
      :token="sshToken"
      :shadow="sshService.shadow"
      @close="closeSsh"
    />
    <RdpTerminal
      v-if="rdpService && rdpSession"
      :host="rdpService.server_ip"
      :port="rdpService.port || 3389"
      :username="rdpService.username"
      :password="rdpService.password || ''"
      :session="rdpSession"
      @close="closeRdp"
    />
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, ArrowDown, TopRight } from '@element-plus/icons-vue'
import { serverAPI, settingsAPI, toolsAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'
import WebAccessDialog from '../components/WebAccessDialog.vue'
import SshTerminal from '../components/SshTerminal.vue'
import RdpTerminal from '../components/RdpTerminal.vue'

const servers = ref([])
const selectedServer = ref(null)
const services = ref([])
const serversLoading = ref(false)
const servicesLoading = ref(false)
const submitting = ref(false)
const serverKeyword = ref('')

// 访问方式从系统设置的端口列表获取
const portProfiles = ref([])
const accessMethods = computed(() => {
  const names = portProfiles.value.map(p => p.name)
  return [...new Set(names)]
})

const filteredServers = computed(() => {
  if (!serverKeyword.value) return servers.value
  const kw = serverKeyword.value.toLowerCase()
  return servers.value.filter(s => s.name.toLowerCase().includes(kw) || s.ip.includes(kw))
})

// 按分组聚合服务器（折叠展示）
const activeGroups = ref([])
const groupedServers = computed(() => {
  const groups = {}
  for (const s of filteredServers.value) {
    const name = s.group_name || '未分组'
    if (!groups[name]) groups[name] = []
    groups[name].push(s)
  }
  return Object.keys(groups).map(name => ({ name, servers: groups[name] }))
})

const dialogVisible = ref(false)
const formRef = ref(null)
const form = reactive({
  id: 0,
  name: '',
  access_method: 'WEB',
  address: '',
  port: 80,
  username: '',
  password: '',
  status: 'running',
  description: ''
})

// 选择访问方式时自动填充对应端口（从端口列表获取）
watch(() => form.access_method, (method) => {
  const profile = portProfiles.value.find(p => p.name === method)
  if (profile) {
    form.port = profile.port
  }
})

const formRules = {
  name: [{ required: true, message: '请输入服务名称', trigger: 'blur' }]
}

// ---- 服务详情 ----
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailTesting = ref(false)
const detail = ref(null)

// ---- 网页访问 ----
const webService = ref(null)
const sshService = ref(null)
const sshToken = ref('')
const rdpService = ref(null)
const rdpSession = ref(null)

const methodTagType = (method) => {
  const map = {
    'WEB': 'success', 'HTTP': 'primary', 'HTTPS': 'primary', 'API': 'primary',
    'MySQL': 'success', 'PostgreSQL': 'success', 'Redis': 'success',
    'RDP': 'warning', 'VNC': 'warning',
    'FTP': 'info', 'SFTP': 'info', 'SSH': 'info',
    '自定义': 'danger'
  }
  return map[method] || 'info'
}

const isWebMethod = (method) => ['WEB', 'HTTP', 'HTTPS'].includes(method)
const isSshMethod = (method) => ['SSH'].includes(method)
const isRdpMethod = (method) => ['RDP'].includes(method)

const svcStatusText = (status) => {
  const map = { running: '运行中', stopped: '已停止', unknown: '未知' }
  return map[status] || '未知'
}

const svcStatusColor = (status) => {
  const map = { running: '#67C23A', stopped: '#F56C6C', unknown: '#909399' }
  return map[status] || '#909399'
}

// 连通率展示（颜色区分）
const connInfo = (row) => {
  if (!row.total_checks) return { text: '暂无', color: '#909399' }
  const r = row.connectivity_rate || 0
  const color = r >= 80 ? '#67C23A' : r >= 50 ? '#E6A23C' : '#F56C6C'
  return { text: `${r.toFixed(0)}%`, color }
}

const rateColor = (rate) => {
  if (!rate && rate !== 0) return '#909399'
  return rate >= 80 ? '#67C23A' : rate >= 50 ? '#E6A23C' : '#F56C6C'
}

const formatTime = (t) => {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

const loadServers = async () => {
  serversLoading.value = true
  try {
    servers.value = await serverAPI.getList()
    // 保持当前选中
    if (selectedServer.value) {
      const still = servers.value.find(s => s.id === selectedServer.value.id)
      if (still) {
        selectedServer.value = still
      } else {
        selectedServer.value = null
        services.value = []
      }
    }
  } catch (error) {
    ElMessage.error('加载服务器列表失败')
  } finally {
    serversLoading.value = false
  }
}

const selectServer = (server) => {
  selectedServer.value = server
  loadServices()
}

const loadServices = async () => {
  if (!selectedServer.value) return
  servicesLoading.value = true
  try {
    services.value = await serverAPI.getServices(selectedServer.value.id)
  } catch (error) {
    ElMessage.error('加载服务列表失败')
  } finally {
    servicesLoading.value = false
  }
}

const openDialog = (row) => {
  if (row) {
    Object.assign(form, {
      id: row.id,
      name: row.name,
      access_method: row.access_method,
      address: row.address || '',
      port: row.port || 0,
      username: row.username || '',
      password: row.password || '',
      status: row.status || 'running',
      description: row.description || ''
    })
  } else {
    Object.assign(form, {
      id: 0,
      name: '',
      access_method: 'WEB',
      address: '',
      port: 80,
      username: '',
      password: '',
      status: 'running',
      description: ''
    })
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const data = { ...form }
    if (form.id) {
      await serverAPI.updateService(selectedServer.value.id, form.id, data)
      ElMessage.success('更新成功')
    } else {
      await serverAPI.createService(selectedServer.value.id, data)
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadServices()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    submitting.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await serverAPI.deleteService(selectedServer.value.id, row.id)
    ElMessage.success('删除成功')
    loadServices()
    if (detail.value && detail.value.service.id === row.id) {
      detailVisible.value = false
    }
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

// 列表内检测
const handleTest = async (row) => {
  row._testing = true
  try {
    const res = await serverAPI.testService(selectedServer.value.id, row.id)
    row.status = res.status
    row.total_checks = res.total_checks
    row.success_checks = res.success_checks
    row.connectivity_rate = res.connectivity_rate
    row.last_version = res.version
    if (res.ok) {
      ElMessage.success(`检测通过: ${res.output}`)
    } else {
      ElMessage.warning(`检测失败: ${res.output}`)
    }
    loadServices()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '检测失败')
  } finally {
    row._testing = false
  }
}

// 打开服务详情
const openDetail = async (row) => {
  detailVisible.value = true
  detailLoading.value = true
  try {
    detail.value = await serverAPI.getServiceDetail(selectedServer.value.id, row.id)
  } catch (error) {
    ElMessage.error('加载服务详情失败')
  } finally {
    detailLoading.value = false
  }
}

// 详情内重新检测
const handleDetailTest = async () => {
  if (!detail.value) return
  detailTesting.value = true
  try {
    const res = await serverAPI.testService(selectedServer.value.id, detail.value.service.id)
    ElMessage[res.ok ? 'success' : 'warning'](res.output)
    await openDetail({ id: detail.value.service.id })
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '检测失败')
  } finally {
    detailTesting.value = false
  }
}

// 打开网页访问弹窗
const openWebAccess = (svc) => {
  webService.value = { ...svc, server_ip: selectedServer.value ? selectedServer.value.ip : svc.server_ip }
}

const openSshAccess = async (svc) => {
  const ip = selectedServer.value ? selectedServer.value.ip : svc.server_ip
  const port = svc.port || 22
  const user = svc.username || ''
  const pass = svc.password || ''
  if (!user) {
    ElMessage.warning('该服务未配置SSH用户名')
    return
  }
  try {
    const res = await toolsAPI.createSshSession({ host: ip, port, user, pass })
    sshToken.value = res.token
    sshService.value = { ...svc, server_ip: ip }
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建SSH会话失败')
  }
}

const closeSsh = () => {
  sshService.value = null
  sshToken.value = ''
}

const openSshShadow = async (svc) => {
  const ip = selectedServer.value ? selectedServer.value.ip : svc.server_ip
  const port = svc.port || 22
  const user = svc.username || ''
  if (!user) {
    ElMessage.warning('该服务未配置SSH用户名')
    return
  }
  try {
    const res = await toolsAPI.createSshShadow({ host: ip, port, user })
    sshToken.value = res.token
    sshService.value = { ...svc, server_ip: ip, shadow: true }
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建SSH影子会话失败')
  }
}

const openRdpAccess = async (svc, shadow = false) => {
  const ip = selectedServer.value ? selectedServer.value.ip : svc.server_ip
  const username = svc.username || ''
  if (!username) {
    ElMessage.warning('该服务未配置RDP用户名')
    return
  }
  try {
    const res = await toolsAPI.createRdpSession({
      host: ip,
      port: svc.port || 3389,
      username,
      password: svc.password || '',
      shadow
    })
    rdpSession.value = res
    rdpService.value = { ...svc, server_ip: ip }
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建RDP会话失败')
  }
}

const closeRdp = () => {
  rdpService.value = null
  rdpSession.value = null
}

const loadPortProfiles = async () => {
  try {
    portProfiles.value = await settingsAPI.getPortProfiles()
  } catch (error) {
    console.error('加载端口列表失败:', error)
  }
}

onMounted(() => {
  loadPortProfiles()
  loadServers()
})
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
}

.server-collapse {
  border: none;
  background: transparent;
}

.server-collapse :deep(.el-collapse-item__header) {
  background: transparent;
  border-bottom: 1px solid #ebeef5;
  font-size: 13px;
  height: 38px;
}

.server-collapse :deep(.el-collapse-item__wrap) {
  background: transparent;
  border-bottom: none;
}

.group-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.group-name {
  font-weight: 500;
}

.server-list {
  max-height: 60vh;
  overflow-y: auto;
}

.server-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 4px;
  transition: background-color 0.2s;
}

.server-item:hover {
  background-color: #f5f7fa;
}

.server-item.active {
  background-color: #ecf5ff;
}

.server-info {
  flex: 1;
  overflow: hidden;
}

.server-name {
  font-size: 14px;
  font-weight: 500;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.server-ip {
  font-size: 12px;
  color: #909399;
}

.svc-name {
  font-weight: 500;
}

.svc-name.link {
  cursor: pointer;
  color: #409EFF;
}

.svc-name.link:hover {
  text-decoration: underline;
}

.pwd-text {
  color: #909399;
}

.svc-status {
  margin-left: 6px;
  font-size: 13px;
}

.conn-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-left: 6px;
  vertical-align: middle;
}

.muted {
  color: #909399;
}

.detail-desc {
  margin-bottom: 4px;
}

.detail-section {
  margin-top: 18px;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 10px;
  padding-bottom: 8px;
  border-bottom: 1px solid #ebeef5;
}

.conn-panel {
  display: flex;
  align-items: center;
  gap: 30px;
  padding: 14px 16px;
  background-color: #fafbfc;
  border-radius: 8px;
  flex-wrap: wrap;
}

.conn-rate {
  text-align: center;
}

.rate-value {
  font-size: 28px;
  font-weight: 700;
}

.rate-label {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.conn-stats {
  display: flex;
  gap: 26px;
}

.stat-item {
  text-align: center;
}

.stat-value {
  font-size: 18px;
  font-weight: 600;
}

.stat-label {
  font-size: 12px;
  color: #909399;
}

.conn-progress {
  flex: 1;
  min-width: 200px;
}

.latest-info {
  margin-top: 10px;
  font-size: 13px;
  color: #606266;
}

.drawer-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.tip {
  margin-left: 10px;
  font-size: 12px;
  color: #909399;
}
</style>

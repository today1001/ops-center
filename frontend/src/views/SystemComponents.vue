<template>
  <div class="system-components">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>系统支撑组件</span>
          <div class="header-actions">
            <el-input
              v-model="keyword"
              placeholder="搜索组件名称..."
              clearable
              style="width: 220px;"
            />
            <el-button type="primary" :icon="Refresh" @click="loadComponents" :loading="loading">刷新</el-button>
          </div>
        </div>
      </template>

      <el-alert
        title="此处列出当前系统的支撑组件，可查看运行状态、重启/停止并查看最新日志。"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 15px;"
      />

      <el-table :data="filteredComponents" v-loading="loading" size="default">
        <el-table-column label="组件名称" min-width="180">
          <template #default="{ row }">
            <div class="comp-name">{{ row.display_name }}</div>
            <div class="comp-unit" v-if="row.unit">{{ row.unit }}</div>
          </template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.type === 'core' ? 'danger' : 'primary'">
              {{ row.type === 'core' ? '核心' : '服务' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="描述" min-width="220" show-overflow-tooltip />
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <span class="status-text">
              <StatusDot :status="statusDot(row.status)" />
              <span :style="{ color: statusColor(row.status) }">{{ statusLabel(row.status) }}</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="PID/端口" width="110">
          <template #default="{ row }">
            <span v-if="row.pid">{{ row.pid }}<template v-if="row.port"> / {{ row.port }}</template></span>
            <span v-else class="comp-unit">-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" plain :disabled="!row.manageable || row.status !== 'running'" @click="handleRestart(row)">重启</el-button>
            <el-button size="small" type="danger" plain :disabled="!row.manageable || row.status !== 'running'" @click="handleStop(row)">停止</el-button>
            <el-button size="small" @click="viewLogs(row)">日志</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 日志对话框 -->
    <el-dialog v-model="logDialogVisible" :title="`${currentLog.name} - 最新日志`" width="760px" top="5vh">
      <div class="log-meta" v-if="currentLog.source">
        日志来源: {{ currentLog.source === 'journal' ? 'Journal（systemd）' : '日志文件' }}
      </div>
      <pre class="log-content" v-loading="logLoading">{{ logContent }}</pre>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { systemAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'

const loading = ref(false)
const keyword = ref('')
const components = ref([])

const logDialogVisible = ref(false)
const logLoading = ref(false)
const logContent = ref('')
const currentLog = reactive({ name: '', source: '' })

let timer = null

const filteredComponents = computed(() => {
  if (!keyword.value) return components.value
  const kw = keyword.value.toLowerCase()
  return components.value.filter(c =>
    c.display_name.toLowerCase().includes(kw) ||
    (c.unit || '').toLowerCase().includes(kw) ||
    (c.description || '').toLowerCase().includes(kw)
  )
})

const statusLabel = (status) => {
  switch (status) {
    case 'running': return '运行中'
    case 'stopped': return '已停止'
    case 'failed': return '异常'
    case 'activating': return '启动中'
    default: return '未知'
  }
}

const statusColor = (status) => {
  switch (status) {
    case 'running': return '#67C23A'
    case 'stopped': return '#909399'
    case 'failed': return '#F56C6C'
    case 'activating': return '#E6A23C'
    default: return '#909399'
  }
}

const statusDot = (status) => {
  if (status === 'running') return 'active'
  if (status === 'failed') return 'offline'
  return 'busy'
}

const loadComponents = async () => {
  loading.value = true
  try {
    const data = await systemAPI.getComponents()
    components.value = data
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '获取组件失败')
  } finally {
    loading.value = false
  }
}

const handleRestart = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要重启组件「${row.display_name}」吗？`,
      '重启确认',
      { type: 'warning', confirmButtonText: '重启', cancelButtonText: '取消' }
    )
  } catch (e) {
    return
  }
  try {
    const res = await systemAPI.restart(row.name)
    ElMessage.success(res.message || '重启成功')
    setTimeout(loadComponents, 2500)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '重启失败')
  }
}

const handleStop = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要停止组件「${row.display_name}」吗？停止后该组件将不可用！`,
      '停止确认',
      { type: 'error', confirmButtonText: '停止', cancelButtonText: '取消' }
    )
  } catch (e) {
    return
  }
  try {
    const res = await systemAPI.stop(row.name)
    ElMessage.success(res.message || '已停止')
    setTimeout(loadComponents, 2500)
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '停止失败')
  }
}

const viewLogs = async (row) => {
  currentLog.name = row.display_name
  currentLog.source = row.log_source
  logContent.value = '加载中...'
  logDialogVisible.value = true
  logLoading.value = true
  try {
    const res = await systemAPI.getLogs(row.name, 100)
    logContent.value = res.log || '（无日志内容）'
  } catch (error) {
    logContent.value = error.response?.data?.error || '读取日志失败'
  } finally {
    logLoading.value = false
  }
}

onMounted(() => {
  loadComponents()
  timer = setInterval(loadComponents, 10000)
})

onBeforeUnmount(() => {
  if (timer) clearInterval(timer)
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
  gap: 10px;
}

.comp-name {
  font-weight: 500;
}

.comp-unit {
  font-size: 12px;
  color: #909399;
}

.status-text {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.log-meta {
  font-size: 13px;
  color: #909399;
  margin-bottom: 8px;
}

.log-content {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  max-height: 560px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>

<template>
  <div class="ssh-access-tool">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>SSH终端</span>
          <div class="header-actions">
            <el-input
              v-model="keyword"
              placeholder="搜索服务器名称/IP..."
              clearable
              style="width: 240px;"
              size="small"
            />
            <el-button size="small" :icon="Refresh" circle @click="loadServers" :loading="loading" />
          </div>
        </div>
      </template>

      <el-alert
        title="选择服务器点击「连接」，将以WebSSH方式登录该服务器。支持复制粘贴、窗口自适应等常用功能。"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 15px;"
      />

      <el-table :data="filteredServers" v-loading="loading" style="width: 100%">
        <el-table-column label="服务器名称" min-width="150">
          <template #default="{ row }">
            <span class="srv-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="IP地址" min-width="140">
          <template #default="{ row }">
            <span class="srv-ip">{{ row.ip }}</span>
          </template>
        </el-table-column>
        <el-table-column label="SSH端口" width="90">
          <template #default="{ row }">
            <span>{{ row.port || 22 }}</span>
          </template>
        </el-table-column>
        <el-table-column label="用户名" width="110">
          <template #default="{ row }">
            <span>{{ row.username || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <StatusDot :status="row.status === 'active' ? 'active' : (row.status === 'offline' ? 'offline' : 'busy')" />
            <span :style="{ color: row.status === 'active' ? '#67C23A' : '#F56C6C' }" class="srv-status">
              {{ row.status === 'active' ? '在线' : (row.status === 'offline' ? '离线' : '未知') }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作系统" min-width="120" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="muted">{{ row.os || '未检测' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button
              size="small"
              type="success"
              plain
              :icon="Monitor"
              @click="openSsh(row)"
              :disabled="!row.username"
            >
              连接
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <SshTerminal
      v-if="sshTarget"
      :host="sshTarget.ip"
      :port="sshTarget.port || 22"
      :user="sshTarget.username"
      :pass="sshTarget.password || ''"
      :token="sshToken"
      @close="closeSsh"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Monitor } from '@element-plus/icons-vue'
import { serverAPI, toolsAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'
import SshTerminal from '../components/SshTerminal.vue'

const servers = ref([])
const loading = ref(false)
const keyword = ref('')
const sshTarget = ref(null)
const sshToken = ref('')

const filteredServers = computed(() => {
  if (!keyword.value) return servers.value
  const kw = keyword.value.toLowerCase()
  return servers.value.filter(s =>
    (s.name || '').toLowerCase().includes(kw) ||
    (s.ip || '').toLowerCase().includes(kw)
  )
})

const loadServers = async () => {
  loading.value = true
  try {
    servers.value = await serverAPI.getList()
  } catch (error) {
    ElMessage.error('加载服务器列表失败')
  } finally {
    loading.value = false
  }
}

const openSsh = async (row) => {
  if (!row.username) {
    ElMessage.warning('该服务器未配置SSH用户名')
    return
  }
  try {
    const res = await toolsAPI.createSshSession({
      host: row.ip,
      port: row.port || 22,
      user: row.username,
      pass: row.password || ''
    })
    sshToken.value = res.token
    sshTarget.value = { ...row }
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建SSH会话失败')
  }
}

const closeSsh = () => {
  sshTarget.value = null
  sshToken.value = ''
}

onMounted(loadServers)
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
  align-items: center;
}

.srv-name {
  font-weight: 500;
  color: #303133;
}

.srv-ip {
  font-family: monospace;
  font-size: 13px;
  color: #606266;
}

.srv-status {
  margin-left: 6px;
  font-size: 13px;
}

.muted {
  color: #909399;
}
</style>

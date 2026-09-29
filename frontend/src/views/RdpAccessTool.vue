<template>
  <div class="rdp-access-tool">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>远程桌面 (RDP)</span>
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
        title="选择 Windows 服务器点击「连接」，将通过 WebRDP 方式登录该服务器。支持鼠标、键盘、复制粘贴等常用功能。"
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
        <el-table-column label="RDP端口" width="100">
          <template #default="{ row }">
            <span>{{ row.rdp_port || 3389 }}</span>
          </template>
        </el-table-column>
        <el-table-column label="用户名" width="120">
          <template #default="{ row }">
            <span>{{ row.rdp_username || row.username || '-' }}</span>
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
              type="primary"
              plain
              :icon="Monitor"
              @click="openRdp(row)"
              :disabled="!row.rdp_username && !row.username"
            >
              连接
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <RdpTerminal
      v-if="rdpTarget"
      :host="rdpTarget.ip"
      :port="rdpTarget.rdp_port || 3389"
      :username="rdpTarget.rdp_username || rdpTarget.username"
      :password="rdpTarget.rdp_password || rdpTarget.password || ''"
      :session="rdpSession"
      @close="closeRdp"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, Monitor } from '@element-plus/icons-vue'
import { serverAPI, toolsAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'
import RdpTerminal from '../components/RdpTerminal.vue'

const servers = ref([])
const loading = ref(false)
const keyword = ref('')
const rdpTarget = ref(null)
const rdpSession = ref(null)

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

const openRdp = async (row) => {
  const username = row.rdp_username || row.username
  if (!username) {
    ElMessage.warning('该服务器未配置登录用户名')
    return
  }
  try {
    const res = await toolsAPI.createRdpSession({
      host: row.ip,
      port: row.rdp_port || 3389,
      username,
      password: row.rdp_password || row.password || ''
    })
    rdpSession.value = res
    rdpTarget.value = { ...row }
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建RDP会话失败')
  }
}

const closeRdp = () => {
  rdpTarget.value = null
  rdpSession.value = null
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

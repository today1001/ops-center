<template>
  <div class="web-access-tool">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>网页访问工具</span>
          <div class="header-actions">
            <el-input
              v-model="keyword"
              placeholder="搜索服务名称/地址/服务器..."
              clearable
              style="width: 260px;"
              size="small"
            />
            <el-button size="small" :icon="Refresh" circle @click="loadServices" :loading="loading" />
          </div>
        </div>
      </template>

      <el-alert
        title="选择一个WEB类服务点击「连接」，将以窗口框架访问该网页端口，并自动填写服务的用户名和密码。"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 15px;"
      />

      <el-table :data="filteredServices" v-loading="loading" style="width: 100%">
        <el-table-column label="所属服务器" min-width="140">
          <template #default="{ row }">
            <div class="svc-server">
              <span class="svc-server-name">{{ row.server_name }}</span>
              <span class="svc-server-ip">{{ row.server_ip }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="服务名称" min-width="130">
          <template #default="{ row }">
            <span class="svc-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column label="访问方式" width="95">
          <template #default="{ row }">
            <el-tag size="small" :type="methodTagType(row.access_method)">{{ row.access_method }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="访问地址" min-width="150">
          <template #default="{ row }">
            <span>{{ row.address || row.server_ip }}<template v-if="row.port">:{{ row.port }}</template></span>
          </template>
        </el-table-column>
        <el-table-column label="连通率" width="90">
          <template #default="{ row }">
            <span :style="{ color: connInfo(row).color, fontWeight: 500 }">{{ connInfo(row).text }}</span>
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
            <span :style="{ color: row.status === 'running' ? '#67C23A' : '#F56C6C' }" class="svc-status">
              {{ row.status === 'running' ? '运行中' : '已停止' }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="110">
          <template #default="{ row }">
            <el-button size="small" type="success" plain :icon="TopRight" @click="openWebAccess(row)">
              连接
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <WebAccessDialog
      v-if="webService"
      :service="webService"
      @close="webService = null"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, TopRight } from '@element-plus/icons-vue'
import { toolsAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'
import WebAccessDialog from '../components/WebAccessDialog.vue'

const services = ref([])
const loading = ref(false)
const keyword = ref('')
const webService = ref(null)

const methodTagType = (method) => {
  const map = { 'WEB': 'success', 'HTTP': 'primary', 'HTTPS': 'primary' }
  return map[method] || 'info'
}

const connInfo = (row) => {
  if (!row.total_checks) return { text: '暂无', color: '#909399' }
  const r = row.connectivity_rate || 0
  const color = r >= 80 ? '#67C23A' : r >= 50 ? '#E6A23C' : '#F56C6C'
  return { text: `${r.toFixed(0)}%`, color }
}

const filteredServices = computed(() => {
  if (!keyword.value) return services.value
  const kw = keyword.value.toLowerCase()
  return services.value.filter(s =>
    (s.name || '').toLowerCase().includes(kw) ||
    (s.address || '').toLowerCase().includes(kw) ||
    (s.server_ip || '').toLowerCase().includes(kw) ||
    (s.server_name || '').toLowerCase().includes(kw)
  )
})

const loadServices = async () => {
  loading.value = true
  try {
    services.value = await toolsAPI.getWebServices()
  } catch (error) {
    ElMessage.error('加载WEB服务列表失败')
  } finally {
    loading.value = false
  }
}

const openWebAccess = (row) => {
  webService.value = { ...row }
}

onMounted(loadServices)
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

.svc-server-name {
  font-weight: 500;
  color: #303133;
}

.svc-server-ip {
  margin-left: 6px;
  font-size: 12px;
  color: #909399;
}

.svc-name {
  font-weight: 500;
}

.svc-status {
  margin-left: 6px;
  font-size: 13px;
}

.muted {
  color: #909399;
}
</style>

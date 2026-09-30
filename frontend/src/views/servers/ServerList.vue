<template>
  <div class="server-list">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>服务器列表</span>
          <div class="header-actions">
            <el-button type="warning" @click="handleCollectMissing" :loading="collecting">
              <el-icon><Download /></el-icon>
              采集缺失
            </el-button>
            <el-button type="success" @click="handleRefreshAll" :loading="refreshing">
              <el-icon><Refresh /></el-icon>
              刷新全部
            </el-button>
            <el-button type="primary" @click="$router.push('/servers/add')">
              <el-icon><Plus /></el-icon>
              添加服务器
            </el-button>
          </div>
        </div>
      </template>

      <!-- 状态图例 -->
      <div class="legend-row">
        <StatusLegend />
      </div>

      <!-- 刷新进度条 -->
      <el-progress 
        v-if="refreshing" 
        :percentage="refreshProgress" 
        :status="refreshProgress === 100 ? 'success' : ''"
        style="margin-bottom: 20px;"
      />

      <div v-loading="loading">
        <el-empty v-if="servers.length === 0" description="暂无服务器，点击右上角添加" />
        <el-collapse v-else v-model="activeGroups">
          <el-collapse-item v-for="group in groupedServers" :key="group.name" :name="group.name">
            <template #title>
              <div class="group-title">
                <span class="group-name">{{ group.name }}</span>
                <el-tag size="small" type="info" class="group-count">{{ group.servers.length }}台</el-tag>
                <span class="group-status" v-if="group.name === '未分组'">（未设置分组的服务器）</span>
              </div>
            </template>

            <el-table :data="group.servers" style="width: 100%">
              <el-table-column label="名称" width="150">
                <template #default="{ row }">
                  <router-link :to="`/servers/${row.id}`" class="server-link">
                    {{ row.name }}
                  </router-link>
                </template>
              </el-table-column>
              <el-table-column prop="ip" label="IP地址" width="150" />
              <el-table-column label="虚拟IP" width="140">
                <template #default="{ row }">
                  <span v-if="row.virtual_ip" :style="{ color: row.virtual_online ? '#67C23A' : '#F56C6C' }">{{ row.virtual_ip }}</span>
                  <span v-else style="color: #c0c4cc;">-</span>
                </template>
              </el-table-column>
              <el-table-column prop="port" label="端口" width="80" />
              <el-table-column prop="username" label="用户名" width="120" />
              <el-table-column label="连接" width="80">
                <template #default="{ row }">
                  <el-tag size="small" :type="row.auth_type === 'key' ? 'warning' : 'info'">
                    {{ row.auth_type === 'key' ? 'Key' : 'SSH' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="100">
                <template #default="{ row }">
                  <span class="status-text">
                    <StatusDot :status="row.status" />
                    <span :style="{ color: statusColor(row.status) }">{{ statusLabel(row.status) }}</span>
                  </span>
                </template>
              </el-table-column>
              <el-table-column label="操作" fixed="right" width="220">
                <template #default="{ row }">
                  <div class="action-buttons">
                    <el-button size="small" @click="handleRefresh(row)" :loading="row._refreshing">
                      <el-icon><Refresh /></el-icon>
                    </el-button>
                    <el-button size="small" @click="handleTest(row)">测试</el-button>
                    <el-button size="small" type="primary" @click="handleEdit(row)">编辑</el-button>
                    <el-popconfirm
                      title="确定要删除这台服务器吗？"
                      @confirm="handleDelete(row)"
                    >
                      <template #reference>
                        <el-button size="small" type="danger">删除</el-button>
                      </template>
                    </el-popconfirm>
                  </div>
                </template>
              </el-table-column>
            </el-table>
          </el-collapse-item>
        </el-collapse>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Plus, Refresh, Download } from '@element-plus/icons-vue'
import { serverAPI, vnetAPI } from '../../api'
import StatusDot from '../../components/StatusDot.vue'
import StatusLegend from '../../components/StatusLegend.vue'

const router = useRouter()
const servers = ref([])
const loading = ref(false)
const refreshing = ref(false)
const collecting = ref(false)
const refreshProgress = ref(0)

// 折叠面板：默认全部折叠
const activeGroups = ref([])

const statusLabel = (status) => {
  const map = { active: '正常', busy: '忙碌', unreachable: '无法连接', offline: '掉线' }
  return map[status] || '未知'
}

const statusColor = (status) => {
  const map = { active: '#67C23A', busy: '#E6A23C', unreachable: '#409EFF', offline: '#F56C6C' }
  return map[status] || '#909399'
}

// 虚拟网络分组：easytier-<网络名>
const vnetNetworks = ref([])
const ipToInt = (ip) => ip.split('.').reduce((a, o) => ((a << 8) + (+o)) >>> 0, 0)
const cidrMatch = (ip, subnet) => {
  if (!ip || !subnet || !subnet.includes('/')) return false
  const parts = subnet.split('/')
  const mask = parts[1] === '0' ? 0 : ((~0 << (32 - +parts[1])) >>> 0)
  return (ipToInt(ip) & mask) === (ipToInt(parts[0]) & mask)
}
const loadVnetNetworks = async () => {
  try { vnetNetworks.value = await vnetAPI.getNetworks() } catch (e) { vnetNetworks.value = [] }
}

// 按分组聚合：局域网IP在原分组，虚拟网在 easytier-<网络名> 分组
const groupedServers = computed(() => {
  const groups = {}
  const order = []
  const push = (name, s) => {
    if (!groups[name]) { groups[name] = []; order.push(name) }
    groups[name].push(s)
  }
  for (const s of servers.value) {
    const net = vnetNetworks.value.find(n => n.name === s.virtual_network)
    if (s.virtual_network && net) {
      const gname = 'easytier-' + s.virtual_network
      if (cidrMatch(s.ip, net.subnet)) {
        // 主IP就在虚拟网段内：仅虚拟身份，只进虚拟分组
        push(gname, s)
      } else {
        // 双身份：局域网IP在原分组，虚拟IP在虚拟分组
        push(s.group_name || '未分组', s)
        push(gname, s)
      }
    } else {
      push(s.group_name || '未分组', s)
    }
  }
  return order.map(name => ({ name, servers: groups[name] }))
})

onMounted(() => {
  loadServers()
  loadVnetNetworks()
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

const handleTest = async (row) => {
  try {
    await serverAPI.testConnection(row.id)
    ElMessage.success('连接测试成功')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '连接测试失败')
  }
}

const handleRefresh = async (row) => {
  row._refreshing = true
  try {
    await serverAPI.refresh(row.id)
    ElMessage.success('刷新成功')
    loadServers()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '刷新失败')
  } finally {
    row._refreshing = false
  }
}

const handleCollectMissing = async () => {
  collecting.value = true
  try {
    const result = await serverAPI.collectMissing()
    if (result.total === 0) {
      ElMessage.info('所有服务器已有详情数据')
    } else {
      ElMessage.success(`开始采集 ${result.total} 台服务器的详情`)
      loadServers()
    }
  } catch (error) {
    ElMessage.error('采集失败')
  } finally {
    collecting.value = false
  }
}

const handleRefreshAll = async () => {
  refreshing.value = true
  refreshProgress.value = 0
  
  try {
    await serverAPI.refreshAll()
    
    // 模拟进度
    const total = servers.value.length
    let completed = 0
    
    const progressInterval = setInterval(() => {
      if (completed < total) {
        completed++
        refreshProgress.value = Math.round((completed / total) * 100)
      } else {
        clearInterval(progressInterval)
        refreshing.value = false
        loadServers()
        ElMessage.success('刷新完成')
      }
    }, 1000)
  } catch (error) {
    refreshing.value = false
    ElMessage.error('刷新失败')
  }
}

const handleEdit = (row) => {
  router.push(`/servers/${row.id}/edit`)
}

const handleDelete = async (row) => {
  try {
    await serverAPI.delete(row.id)
    ElMessage.success('删除成功')
    loadServers()
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.legend-row {
  border-bottom: 1px solid #ebeef5;
  margin-bottom: 15px;
}

.status-text {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.group-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.group-name {
  font-weight: 500;
}

.group-status {
  font-size: 12px;
  color: #909399;
}

.server-link {
  color: #409EFF;
  text-decoration: none;
}

.server-link:hover {
  text-decoration: underline;
}

.action-buttons {
  display: flex;
  gap: 6px;
  flex-wrap: nowrap;
}
</style>

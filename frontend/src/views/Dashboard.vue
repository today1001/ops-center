<template>
  <div class="dashboard">
    <!-- 服务器状态总览 -->
    <div class="stat-row">
      <el-card class="stat-card">
        <template #header>
          <div class="stat-header">
            <el-icon :size="24" color="#409EFF"><Connection /></el-icon>
            <span>服务器总数</span>
          </div>
        </template>
        <div class="stat-value">{{ stats.total }}</div>
      </el-card>
      
      <el-card class="stat-card">
        <template #header>
          <div class="stat-header">
            <StatusDot status="active" />
            <span>正常</span>
          </div>
        </template>
        <div class="stat-value" style="color: #67C23A;">{{ stats.active }}</div>
      </el-card>
      
      <el-card class="stat-card">
        <template #header>
          <div class="stat-header">
            <StatusDot status="busy" />
            <span>忙碌</span>
          </div>
        </template>
        <div class="stat-value" style="color: #E6A23C;">{{ stats.busy }}</div>
      </el-card>

      <el-card class="stat-card">
        <template #header>
          <div class="stat-header">
            <StatusDot status="unreachable" />
            <span>无法连接</span>
          </div>
        </template>
        <div class="stat-value" style="color: #409EFF;">{{ stats.unreachable }}</div>
      </el-card>

      <el-card class="stat-card">
        <template #header>
          <div class="stat-header">
            <StatusDot status="offline" />
            <span>掉线</span>
          </div>
        </template>
        <div class="stat-value" style="color: #F56C6C;">{{ stats.offline }}</div>
      </el-card>
    </div>

    <!-- 状态图例 -->
    <div class="legend-row">
      <StatusLegend />
    </div>

    <!-- 资源总量概览 -->
    <el-row :gutter="20" style="margin-top: 20px;">
      <el-col :span="6">
        <el-card class="resource-card">
          <template #header>
            <div class="res-header">
              <el-icon :size="18" color="#409EFF"><Cpu /></el-icon>
              <span>CPU 资源</span>
            </div>
          </template>
          <div class="res-total">{{ resource.total_cpu_cores }} <span class="res-unit">核心</span></div>
          <div class="res-desc">平均使用率 {{ resource.avg_cpu_usage }}%</div>
          <el-progress :percentage="resource.avg_cpu_usage" :color="usageColor(resource.avg_cpu_usage)" :stroke-width="14" style="margin-top: 10px;" />
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card class="resource-card">
          <template #header>
            <div class="res-header">
              <el-icon :size="18" color="#67C23A"><Monitor /></el-icon>
              <span>内存资源</span>
            </div>
          </template>
          <div class="res-total">{{ formatBytes(resource.total_memory) }}</div>
          <div class="res-desc">已使用 {{ formatBytes(resource.used_memory) }}（{{ resource.memory_usage }}%）</div>
          <el-progress :percentage="resource.memory_usage" :color="usageColor(resource.memory_usage)" :stroke-width="14" style="margin-top: 10px;" />
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card class="resource-card">
          <template #header>
            <div class="res-header">
              <el-icon :size="18" color="#E6A23C"><Coin /></el-icon>
              <span>磁盘资源</span>
            </div>
          </template>
          <div class="res-total">{{ formatBytes(resource.total_disk) }}</div>
          <div class="res-desc">已使用 {{ formatBytes(resource.used_disk) }}（{{ resource.disk_usage }}%）</div>
          <el-progress :percentage="resource.disk_usage" :color="usageColor(resource.disk_usage)" :stroke-width="14" style="margin-top: 10px;" />
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card class="resource-card">
          <template #header>
            <div class="res-header">
              <el-icon :size="18" color="#F56C6C"><Connection /></el-icon>
              <span>网络资源</span>
            </div>
          </template>
          <div class="res-total">{{ formatBandwidth(resource.total_network_speed) }}</div>
          <div class="res-desc">当前使用 {{ formatBandwidth(resource.total_network_rx + resource.total_network_tx) }}（{{ resource.network_usage }}%）</div>
          <el-progress :percentage="resource.network_usage" :color="usageColor(resource.network_usage)" :stroke-width="14" style="margin-top: 10px;" />
        </el-card>
      </el-col>
    </el-row>

    <!-- 利用率饼图 -->
    <el-row :gutter="20" style="margin-top: 20px;">
      <el-col :span="6">
        <el-card>
          <template #header>
            <span>CPU 利用率</span>
          </template>
          <div ref="cpuChartRef" class="chart"></div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <template #header>
            <span>内存利用率</span>
          </template>
          <div ref="memChartRef" class="chart"></div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <template #header>
            <span>磁盘利用率</span>
          </template>
          <div ref="diskChartRef" class="chart"></div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card>
          <template #header>
            <span>网络利用率</span>
          </template>
          <div ref="netChartRef" class="chart"></div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 利用率排行 -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <span>服务器利用率排行</span>
      </template>
      <el-tabs v-model="rankTab">
        <el-tab-pane label="CPU" name="cpu">
          <el-row :gutter="20">
            <el-col :span="12">
              <h4 class="rank-title">利用率最高 Top 5</h4>
              <el-table :data="rankings.cpu.top" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="CPU">
                  <template #default="{ row }">
                    <el-progress :percentage="row.cpu_usage" :color="usageColor(row.cpu_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
            <el-col :span="12">
              <h4 class="rank-title">利用率最低 Top 5</h4>
              <el-table :data="rankings.cpu.bottom" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="CPU">
                  <template #default="{ row }">
                    <el-progress :percentage="row.cpu_usage" :color="usageColor(row.cpu_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
          </el-row>
        </el-tab-pane>

        <el-tab-pane label="内存" name="memory">
          <el-row :gutter="20">
            <el-col :span="12">
              <h4 class="rank-title">利用率最高 Top 5</h4>
              <el-table :data="rankings.memory.top" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="内存">
                  <template #default="{ row }">
                    <el-progress :percentage="row.memory_usage" :color="usageColor(row.memory_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
            <el-col :span="12">
              <h4 class="rank-title">利用率最低 Top 5</h4>
              <el-table :data="rankings.memory.bottom" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="内存">
                  <template #default="{ row }">
                    <el-progress :percentage="row.memory_usage" :color="usageColor(row.memory_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
          </el-row>
        </el-tab-pane>

        <el-tab-pane label="磁盘" name="disk">
          <el-row :gutter="20">
            <el-col :span="12">
              <h4 class="rank-title">利用率最高 Top 5</h4>
              <el-table :data="rankings.disk.top" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="磁盘">
                  <template #default="{ row }">
                    <el-progress :percentage="row.disk_usage" :color="usageColor(row.disk_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
            <el-col :span="12">
              <h4 class="rank-title">利用率最低 Top 5</h4>
              <el-table :data="rankings.disk.bottom" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="磁盘">
                  <template #default="{ row }">
                    <el-progress :percentage="row.disk_usage" :color="usageColor(row.disk_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
          </el-row>
        </el-tab-pane>

        <el-tab-pane label="网络" name="network">
          <el-row :gutter="20">
            <el-col :span="12">
              <h4 class="rank-title">利用率最高 Top 5</h4>
              <el-table :data="rankings.network.top" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="网络">
                  <template #default="{ row }">
                    <el-progress :percentage="row.network_usage" :color="usageColor(row.network_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
            <el-col :span="12">
              <h4 class="rank-title">利用率最低 Top 5</h4>
              <el-table :data="rankings.network.bottom" size="small" max-height="260">
                <el-table-column label="名称" width="150">
                  <template #default="{ row }">
                    <router-link :to="`/servers/${row.id}`" class="server-link">{{ row.name }}</router-link>
                  </template>
                </el-table-column>
                <el-table-column prop="ip" label="IP地址" width="150" />
                <el-table-column label="网络">
                  <template #default="{ row }">
                    <el-progress :percentage="row.network_usage" :color="usageColor(row.network_usage)" />
                  </template>
                </el-table-column>
              </el-table>
            </el-col>
          </el-row>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-card class="recent-card" style="margin-top: 20px;">
      <template #header>
        <span>最近添加的服务器</span>
      </template>
      
      <el-table :data="recentServers" style="width: 100%">
        <el-table-column label="名称" width="150">
          <template #default="{ row }">
            <router-link :to="`/servers/${row.id}`" class="server-link">
              {{ row.name }}
            </router-link>
          </template>
        </el-table-column>
        <el-table-column prop="ip" label="IP地址" />
        <el-table-column label="连接" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.auth_type === 'key' ? 'warning' : 'info'">
              {{ row.auth_type === 'key' ? 'Key' : 'SSH' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="80">
          <template #default="{ row }">
            <StatusDot :status="row.status" />
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="添加时间">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { Connection, Cpu, Monitor, Coin } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { serverAPI } from '../api'
import StatusDot from '../components/StatusDot.vue'
import StatusLegend from '../components/StatusLegend.vue'

const stats = ref({
  total: 0,
  active: 0,
  busy: 0,
  unreachable: 0,
  offline: 0
})

const resource = reactive({
  total_cpu_cores: 0,
  avg_cpu_usage: 0,
  total_memory: 0,
  used_memory: 0,
  memory_usage: 0,
  total_disk: 0,
  used_disk: 0,
  disk_usage: 0,
  total_network_speed: 0,
  total_network_rx: 0,
  total_network_tx: 0,
  network_usage: 0
})

const recentServers = ref([])

const rankTab = ref('cpu')
const rankings = reactive({
  cpu: { top: [], bottom: [] },
  memory: { top: [], bottom: [] },
  disk: { top: [], bottom: [] },
  network: { top: [], bottom: [] }
})

const cpuChartRef = ref(null)
const memChartRef = ref(null)
const diskChartRef = ref(null)
const netChartRef = ref(null)
const charts = []

const usageColor = (usage) => {
  if (usage < 60) return '#67C23A'
  if (usage < 80) return '#E6A23C'
  return '#F56C6C'
}

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const formatBandwidth = (mbps) => {
  if (!mbps || mbps === 0) return '0 Mbps'
  if (mbps < 1000) return mbps + ' Mbps'
  return parseFloat((mbps / 1000).toFixed(2)) + ' Gbps'
}

const formatDate = (dateStr) => {
  if (!dateStr) return '-'
  const date = new Date(dateStr)
  return date.toLocaleDateString('zh-CN')
}

const renderPieChart = (el, used, total, name) => {
  if (!el) return null
  const usedPct = total > 0 ? (used / total * 100) : 0
  const freePct = 100 - usedPct
  const chart = echarts.init(el)
  chart.setOption({
    tooltip: {
      trigger: 'item',
      formatter: (p) => {
        if (p.name === '已使用') {
          return `${name} 已使用: ${formatBytes(used)} (${usedPct.toFixed(1)}%)`
        }
        return `${name} 可用: ${formatBytes(total - used)} (${freePct.toFixed(1)}%)`
      }
    },
    legend: {
      bottom: 0,
      textStyle: { fontSize: 12 }
    },
    series: [{
      type: 'pie',
      radius: ['45%', '70%'],
      center: ['50%', '45%'],
      avoidLabelOverlap: false,
      itemStyle: {
        borderRadius: 6,
        borderColor: '#fff',
        borderWidth: 2
      },
      label: {
        show: true,
        formatter: '{b}\n{d}%',
        fontSize: 12
      },
      emphasis: {
        label: { show: true, fontWeight: 'bold' }
      },
      data: [
        { value: usedPct, name: '已使用', itemStyle: { color: usageColor(usedPct) } },
        { value: freePct, name: '可用', itemStyle: { color: '#E1E8EE' } }
      ]
    }]
  })
  return chart
}

const renderPieCharts = async () => {
  await nextTick()
  // 内存饼图
  if (memChartRef.value) charts.push(renderPieChart(memChartRef.value, resource.used_memory, resource.total_memory, '内存'))
  // 磁盘饼图
  if (diskChartRef.value) charts.push(renderPieChart(diskChartRef.value, resource.used_disk, resource.total_disk, '磁盘'))
  // CPU 饼图（百分比形式）
  if (cpuChartRef.value) {
    const cpuUsed = resource.avg_cpu_usage
    const chart = echarts.init(cpuChartRef.value)
    chart.setOption({
      tooltip: { trigger: 'item', formatter: (p) => `${p.name}: ${p.value}%` },
      legend: { bottom: 0, textStyle: { fontSize: 12 } },
      series: [{
        type: 'pie',
        radius: ['45%', '70%'],
        center: ['50%', '45%'],
        itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
        label: { show: true, formatter: '{b}\n{d}%', fontSize: 12 },
        data: [
          { value: cpuUsed, name: '已使用', itemStyle: { color: usageColor(cpuUsed) } },
          { value: 100 - cpuUsed, name: '空闲', itemStyle: { color: '#E1E8EE' } }
        ]
      }]
    })
    charts.push(chart)
  }
  // 网络饼图（百分比形式）
  if (netChartRef.value) {
    const netUsage = resource.network_usage || 0
    const chart = echarts.init(netChartRef.value)
    chart.setOption({
      tooltip: { trigger: 'item', formatter: (p) => `${p.name}: ${p.value}%` },
      legend: { bottom: 0, textStyle: { fontSize: 12 } },
      series: [{
        type: 'pie',
        radius: ['45%', '70%'],
        center: ['50%', '45%'],
        itemStyle: { borderRadius: 6, borderColor: '#fff', borderWidth: 2 },
        label: { show: true, formatter: '{b}\n{d}%', fontSize: 12 },
        data: [
          { value: netUsage, name: '已使用', itemStyle: { color: usageColor(netUsage) } },
          { value: 100 - netUsage, name: '空闲', itemStyle: { color: '#E1E8EE' } }
        ]
      }]
    })
    charts.push(chart)
  }
}

const onResize = () => {
  charts.forEach(c => c && c.resize())
}

onMounted(async () => {
  try {
    const servers = await serverAPI.getList()
    stats.value.total = servers.length
    stats.value.active = servers.filter(s => s.status === 'active').length
    stats.value.busy = servers.filter(s => s.status === 'busy').length
    stats.value.unreachable = servers.filter(s => s.status === 'unreachable').length
    stats.value.offline = servers.filter(s => s.status === 'offline').length
    recentServers.value = servers.slice(0, 5)
  } catch (error) {
    console.error('Failed to load servers:', error)
  }

  try {
    const res = await serverAPI.getResourceStats()
    Object.assign(resource, res)
  } catch (error) {
    console.error('Failed to load resource stats:', error)
  }

  try {
    const res = await serverAPI.getRankings()
    Object.assign(rankings, res)
  } catch (error) {
    console.error('Failed to load rankings:', error)
  }

  window.addEventListener('resize', onResize)
  await renderPieCharts()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  charts.forEach(c => c && c.dispose())
})
</script>

<style scoped>
.stat-row {
  display: flex;
  gap: 20px;
}

.stat-card {
  flex: 1;
  text-align: center;
}

.legend-row {
  margin: 10px 0 0;
}

.stat-header {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.stat-value {
  font-size: 36px;
  font-weight: bold;
  color: #303133;
}

.resource-card {
  text-align: center;
}

.res-header {
  display: flex;
  align-items: center;
  gap: 8px;
}

.res-total {
  font-size: 28px;
  font-weight: bold;
  color: #303133;
}

.res-unit {
  font-size: 14px;
  color: #909399;
  font-weight: normal;
}

.res-desc {
  margin-top: 4px;
  font-size: 13px;
  color: #909399;
}

.chart {
  width: 100%;
  height: 260px;
}

.rank-title {
  margin: 0 0 10px;
  font-size: 14px;
  color: #606266;
}

.server-link {
  color: #409EFF;
  text-decoration: none;
}

.server-link:hover {
  text-decoration: underline;
}
</style>

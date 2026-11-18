<template>
  <div class="free-ip-manager">
    <!-- 定时扫描配置 -->
    <el-card style="margin-bottom: 16px">
      <template #header>
        <div class="card-header">
          <span>定时扫描</span>
          <span class="scan-meta" v-if="config.last_scan_at">上次落库扫描：{{ config.last_scan_at }}</span>
        </div>
      </template>
      <div class="config-row">
        <span>扫描间隔</span>
        <el-input-number v-model="config.interval_minutes" :min="config.min || 5" :max="config.max || 1440" :step="5" size="default" style="width: 160px" />
        <span class="unit">分钟</span>
        <el-button type="primary" :loading="savingConfig" @click="saveConfig">保存</el-button>
        <span class="config-tip">进程开机自启后按此间隔自动扫描并落库，修改无需重启</span>
      </div>
    </el-card>

    <el-card>
      <template #header>
        <div class="card-header">
          <el-tabs v-model="activeTab" style="flex: 1">
            <el-tab-pane label="扫描记录（落库）" name="records" />
            <el-tab-pane label="实时扫描" name="live" />
          </el-tabs>
          <el-button v-if="activeTab === 'live'" type="primary" @click="refreshLive" :loading="loading">
            <el-icon><Refresh /></el-icon>
            立即扫描
          </el-button>
        </div>
      </template>

      <!-- 落库记录 -->
      <template v-if="activeTab === 'records'">
        <div class="records-bar">
          <el-select v-model="selectedScanId" placeholder="选择扫描批次" style="width: 320px" @change="loadRecords">
            <el-option
              v-for="h in history"
              :key="h.id"
              :value="h.id"
              :label="`${h.scanned_at} · 空闲${h.free_count}个 · ${h.trigger === 'auto' ? '定时' : '手动'} · ${h.duration_ms}ms`"
            />
          </el-select>
          <span class="scan-meta" v-if="latest.id">
            共 {{ history.length }} 批记录 · 当前批次空闲 {{ records.length }} 个 / 候选 {{ latest.candidates }} 个
          </span>
        </div>

        <el-table :data="records" v-loading="loadingRecords" stripe>
          <el-table-column prop="ip" label="IP 地址" width="160" />
          <el-table-column prop="subnet" label="网段" width="160" />
          <el-table-column prop="mac" label="MAC 地址" width="180">
            <template #default="{ row }">
              <span v-if="row.mac">{{ row.mac }}</span>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="scanned_at" label="扫描时间" min-width="170" />
          <el-table-column label="操作" width="120">
            <template #default="{ row }">
              <el-button type="primary" size="small" @click="addIPs([row.ip])">添加</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="batch-actions" v-if="records.length > 0">
          <el-select v-model="selectedGroup" placeholder="选择目标分组" style="width: 200px; margin-right: 12px">
            <el-option label="默认分组" value="" />
            <el-option v-for="g in groupNames" :key="g" :label="g" :value="g" />
          </el-select>
          <el-button type="success" @click="addIPs(records.map(r => r.ip))">
            <el-icon><Plus /></el-icon>
            一键添加本批次全部 IP（{{ records.length }} 个）
          </el-button>
        </div>
      </template>

      <!-- 实时扫描 -->
      <template v-else>
        <el-alert
          v-if="scanInfo"
          :title="scanInfo"
          type="info"
          :closable="false"
          show-icon
          style="margin-bottom: 16px"
        />
        <el-table :data="liveIPs" v-loading="loading" stripe>
          <el-table-column prop="ip" label="IP 地址" width="160" />
          <el-table-column prop="subnet" label="网段" width="160" />
          <el-table-column prop="mac" label="MAC 地址" width="180">
            <template #default="{ row }">
              <span v-if="row.mac">{{ row.mac }}</span>
              <span v-else class="text-muted">-</span>
            </template>
          </el-table-column>
          <el-table-column prop="online" label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.online ? 'success' : 'info'" size="small">
                {{ row.online ? '在线' : '离线' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120">
            <template #default="{ row }">
              <el-button type="primary" size="small" @click="addIPs([row.ip])" :disabled="!row.online">
                添加
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <div class="batch-actions" v-if="onlineCount > 0">
          <el-select v-model="selectedGroup" placeholder="选择目标分组" style="width: 200px; margin-right: 12px">
            <el-option label="默认分组" value="" />
            <el-option v-for="g in groupNames" :key="g" :label="g" :value="g" />
          </el-select>
          <el-button type="success" @click="addIPs(liveIPs.filter(i => i.online).map(i => i.ip))">
            <el-icon><Plus /></el-icon>
            一键添加所有在线 IP（{{ onlineCount }} 个）
          </el-button>
        </div>
      </template>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus } from '@element-plus/icons-vue'
import { freeAPI, groupAPI } from '../api'

const activeTab = ref('records')

// 定时配置
const config = ref({ interval_minutes: 30, min: 5, max: 1440, last_scan_at: '' })
const savingConfig = ref(false)

// 落库记录
const records = ref([])
const history = ref([])
const latest = ref({})
const selectedScanId = ref(null)
const loadingRecords = ref(false)

// 实时扫描
const liveIPs = ref([])
const scanInfo = ref('')
const loading = ref(false)

// 添加
const selectedGroup = ref('')
const groupNames = ref([])

const onlineCount = computed(() => liveIPs.value.filter(i => i.online).length)

const loadConfig = async () => {
  try {
    config.value = { ...config.value, ...(await freeAPI.getScanConfig()) }
  } catch (e) { /* 配置读取失败不阻塞页面 */ }
}

const saveConfig = async () => {
  savingConfig.value = true
  try {
    await freeAPI.setScanConfig({ interval_minutes: config.value.interval_minutes })
    ElMessage.success(`已保存：每 ${config.value.interval_minutes} 分钟自动扫描一次`)
    loadConfig()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    savingConfig.value = false
  }
}

const loadRecords = async () => {
  loadingRecords.value = true
  try {
    const params = selectedScanId.value ? { scan_id: selectedScanId.value } : {}
    const res = await freeAPI.getRecords(params)
    records.value = res.records || []
    history.value = res.history || []
    latest.value = res.latest || {}
    if (!selectedScanId.value && latest.value.id) selectedScanId.value = latest.value.id
  } catch (e) {
    ElMessage.error('获取扫描记录失败')
  } finally {
    loadingRecords.value = false
  }
}

const refreshLive = async () => {
  loading.value = true
  try {
    const res = await freeAPI.getFreeIPs({ force: 1 })
    const scan = res.ips || {}
    liveIPs.value = scan.free || []
    const subnets = (scan.subnets || []).join('、')
    scanInfo.value = `扫描于 ${scan.scanned_at} · 网段: ${subnets || '-'} · 候选 ${scan.candidates} 个 · 在线空闲 ${liveIPs.value.length} 个 · 已登记 ${scan.known?.length || 0} 台 · 耗时 ${scan.duration_ms}ms`
    ElMessage.success(`扫描完成，发现 ${liveIPs.value.length} 个在线空闲 IP`)
    // 实时扫描结果已由后端落库，刷新历史
    loadRecords()
  } catch (err) {
    ElMessage.error('扫描失败: ' + (err.response?.data?.error || err.message))
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  try {
    const gs = await groupAPI.getList()
    groupNames.value = (gs || []).map(g => g.name)
  } catch (e) { /* 分组失败不阻塞 */ }
}

const addIPs = async (ips) => {
  if (!ips.length) {
    ElMessage.warning('没有可添加的 IP')
    return
  }
  try {
    await ElMessageBox.confirm(
      `确定将 ${ips.length} 个 IP 添加到服务器管理${selectedGroup.value ? `（分组：${selectedGroup.value}）` : ''}吗？`,
      '确认添加',
      { type: 'warning' }
    )
  } catch { return }
  try {
    const res = await freeAPI.addFreeIPs({ ips, group: selectedGroup.value })
    if (res.added > 0) {
      ElMessage.success(`成功添加 ${res.added} 个，跳过（已存在）${res.skipped} 个`)
      if (activeTab.value === 'live') refreshLive()
      else loadRecords()
    } else {
      ElMessage.warning('没有新的 IP 被添加（可能全部已存在）')
    }
  } catch (err) {
    ElMessage.error('添加失败: ' + (err.response?.data?.error || err.message))
  }
}

onMounted(() => {
  loadConfig()
  loadRecords()
  loadGroups()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.config-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.config-row > span:first-child {
  font-weight: 500;
}
.unit {
  color: #606266;
}
.config-tip {
  color: #909399;
  font-size: 12px;
  margin-left: 8px;
}
.scan-meta {
  color: #909399;
  font-size: 13px;
}
.records-bar {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
}
.batch-actions {
  margin-top: 16px;
  display: flex;
  align-items: center;
}
.text-muted {
  color: #999;
}
</style>

<template>
  <div class="upgrade-center">
    <!-- 配置 -->
    <el-card style="margin-bottom: 16px">
      <template #header>
        <div class="card-header"><h3>升级配置</h3></div>
      </template>
      <div class="config-row">
        <span class="cfg-label">升级源</span>
        <el-input v-model="cfg.source" placeholder="http://10.172.66.61:15801 或 local:/path" style="width: 340px" />
        <span class="cfg-label">检查间隔</span>
        <el-input-number v-model="cfg.interval_minutes" :min="10" :max="10080" :step="10" style="width: 140px" />
        <span class="unit">分钟</span>
        <el-button type="primary" :loading="savingCfg" @click="saveConfig">保存配置</el-button>
      </div>
      <div class="config-row" style="margin-top: 10px">
        <span class="cfg-label">工作区</span>
        <el-input v-model="cfg.ws_backend" size="small" style="width: 150px" placeholder="backend">
          <template #prepend>backend</template>
        </el-input>
        <el-input v-model="cfg.ws_frontend" size="small" style="width: 150px" placeholder="frontend">
          <template #prepend>frontend</template>
        </el-input>
        <el-input v-model="cfg.ws_mcp" size="small" style="width: 150px" placeholder="mcp">
          <template #prepend>mcp</template>
        </el-input>
        <span class="config-tip">发布文件按前缀区分：ops-center-*/frontend-*/mcp-server-*</span>
      </div>
      <div class="config-tip">
        升级源对接更新分发系统的工作区 oc-backend / oc-frontend / oc-mcp；local: 模式从服务器本地目录读取测试包。
        各模块策略独立：<b>自动</b>（定时检查到新版本即升级）、<b>手动</b>（提示新版本，点按钮升级）、<b>关闭</b>（忽略）。
        后端升级采用进程自替换，中断约 1 秒；若新版本 5 分钟内未确认启动将自动回滚。
      </div>
    </el-card>

    <!-- 模块卡片 -->
    <div class="module-grid">
      <el-card v-for="m in modules" :key="m.module" class="module-card" :class="{ 'has-update': m.has_update }">
        <template #header>
          <div class="card-header">
            <h3>{{ moduleLabel(m.module) }}</h3>
            <el-tag v-if="m.has_update" type="warning" effect="dark" size="small">有新版本</el-tag>
            <el-tag v-else type="success" size="small">最新</el-tag>
          </div>
        </template>
        <div class="ver-line"><span>当前版本</span><b>{{ m.current }}</b></div>
        <div class="ver-line"><span>最新版本</span><b>{{ m.latest || '-' }}</b></div>
        <div class="ver-line"><span>发布时间</span><span>{{ fmtTime(m.updated) || '-' }}</span></div>
        <div class="ver-line"><span>升级策略</span>
          <el-select :model-value="m.policy" size="small" style="width: 100px" @change="v => changePolicy(m.module, v)">
            <el-option label="自动" value="auto" />
            <el-option label="手动" value="manual" />
            <el-option label="关闭" value="off" />
          </el-select>
        </div>
        <div v-if="m.changelog" class="changelog">{{ m.changelog }}</div>
        <div v-if="m.error" class="err-line">{{ m.error }}</div>
        <el-button
          type="primary" style="width: 100%; margin-top: 10px"
          :disabled="!m.has_update || upgrading !== ''"
          :loading="upgrading === m.module"
          @click="runUpgrade(m.module)"
        >
          {{ upgrading === m.module ? '升级中…' : '立即升级' }}
        </el-button>
      </el-card>
    </div>

    <el-alert v-if="upgrading === 'backend'" title="后端升级中：进程自替换，页面将在服务重启后自动恢复" type="warning" show-icon :closable="false" style="margin: 12px 0" />

    <!-- 升级日志 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>升级日志</h3>
          <el-button size="small" @click="loadAll">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
        </div>
      </template>
      <el-table :data="logs" v-loading="loadingLogs" stripe size="small">
        <el-table-column label="时间" width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="模块" width="90">
          <template #default="{ row }">{{ moduleLabel(row.module) }}</template>
        </el-table-column>
        <el-table-column label="版本变更" width="180">
          <template #default="{ row }">{{ row.from_ver }} → {{ row.to_ver || '-' }}</template>
        </el-table-column>
        <el-table-column label="结果" width="110">
          <template #default="{ row }">
            <el-tag :type="logTag(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="mode" label="方式" width="80">
          <template #default="{ row }">{{ row.mode === 'auto' ? '自动' : '手动' }}</template>
        </el-table-column>
        <el-table-column prop="message" label="详情" min-width="220" show-overflow-tooltip />
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { upgradeAPI } from '../api'

const cfg = ref({ source: '', interval_minutes: 60, ws_backend: '', ws_frontend: '', ws_mcp: '' })
const modules = ref([])
const logs = ref([])
const savingCfg = ref(false)
const loadingLogs = ref(false)
const upgrading = ref('')
let pollTimer = null

const moduleLabel = (m) => ({ backend: '后端服务', frontend: '前端页面', mcp: 'MCP 服务器' }[m] || m)
const statusLabel = (s) => ({
  success: '成功', failed: '失败', rolled_back: '已回滚', restarting: '重启中', checking: '检查中'
}[s] || s)
const logTag = (s) => ({ success: 'success', failed: 'danger', rolled_back: 'warning', restarting: 'primary' }[s] || 'info')
const fmtTime = (t) => (t || '').replace('T', ' ').replace(/Z$/, '').replace(/\+00:00$/, '').replace(/\.\d+$/, '')

const loadAll = async () => {
  loadingLogs.value = true
  try {
    const st = await upgradeAPI.status()
    cfg.value = {
      source: st.source,
      interval_minutes: st.interval_minutes,
      ws_backend: st.workspaces?.backend || '',
      ws_frontend: st.workspaces?.frontend || '',
      ws_mcp: st.workspaces?.mcp || ''
    }
    modules.value = st.modules || []
    const lg = await upgradeAPI.logs()
    logs.value = lg.logs || []
  } catch (e) {
    // 服务可能正在升级重启，静默
  } finally {
    loadingLogs.value = false
  }
}

const saveConfig = async () => {
  savingCfg.value = true
  try {
    await upgradeAPI.config({
      source: cfg.value.source,
      interval_minutes: cfg.value.interval_minutes,
      workspaces: {
        backend: cfg.value.ws_backend,
        frontend: cfg.value.ws_frontend,
        mcp: cfg.value.ws_mcp
      }
    })
    ElMessage.success('配置已保存')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    savingCfg.value = false
  }
}

const changePolicy = async (module, policy) => {
  try {
    await upgradeAPI.config({ policies: { [module]: policy } })
    ElMessage.success(`${moduleLabel(module)}策略已设为 ${policy}`)
    loadAll()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '设置失败')
    loadAll()
  }
}

const runUpgrade = async (module) => {
  try {
    await ElMessageBox.confirm(
      `确定升级${moduleLabel(module)}到最新版本？${module === 'backend' ? '升级过程中服务将自动重启（中断约 1 秒）。' : ''}`,
      '升级确认',
      { type: 'warning' }
    )
  } catch { return }
  upgrading.value = module
  try {
    const res = await upgradeAPI.run({ module })
    ElMessage.success(res.message || '升级完成')
    upgrading.value = ''
    loadAll()
  } catch (e) {
    const msg = e.response?.data?.error || '升级失败'
    // backend 升级成功时服务重启会中断响应，不能当失败处理——进入轮询确认
    if (module === 'backend' && (e.code === 'ECONNABORTED' || !e.response)) {
      waitBackendBack()
      return
    }
    ElMessage.error(msg)
    upgrading.value = ''
    loadAll()
  }
}

// backend 升级触发进程重启：轮询直到服务恢复
const waitBackendBack = () => {
  clearInterval(pollTimer)
  pollTimer = setInterval(async () => {
    try {
      await upgradeAPI.status()
      clearInterval(pollTimer)
      upgrading.value = ''
      ElMessage.success('后端升级完成，服务已重启')
      // token 若因版本无关仍有效，直接刷新
      loadAll()
    } catch (e) { /* 继续等 */ }
  }, 3000)
}

onMounted(loadAll)
onUnmounted(() => clearInterval(pollTimer))
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.card-header h3 {
  margin: 0;
}
.config-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.cfg-label {
  font-weight: 500;
  color: #606266;
}
.unit {
  color: #606266;
}
.config-tip {
  color: #909399;
  font-size: 12px;
  margin-top: 10px;
  line-height: 1.6;
}
.module-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: 16px;
  margin-bottom: 16px;
}
.module-card.has-update {
  border-color: #e6a23c;
}
.ver-line {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 5px 0;
  font-size: 14px;
}
.ver-line > span:first-child {
  color: #909399;
}
.changelog {
  margin-top: 8px;
  padding: 8px;
  background: var(--bg-secondary, #f5f7fa);
  border-radius: 4px;
  font-size: 12px;
  color: #606266;
  max-height: 90px;
  overflow: auto;
  white-space: pre-wrap;
}
.err-line {
  margin-top: 8px;
  font-size: 12px;
  color: #f56c6c;
}
</style>

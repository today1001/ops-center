<template>
  <div class="system-settings">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>端口访问方式列表</span>
          <el-button type="primary" :icon="Plus" @click="openDialog()">添加端口</el-button>
        </div>
      </template>

      <el-alert
        title='此列表定义了服务管理中的"访问方式"。添加服务时，访问方式从该列表获取，选择后自动填充对应端口（如 WEB 默认 80）。'
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 15px;"
      />

      <el-table :data="portProfiles" v-loading="loading" border>
        <el-table-column prop="port" label="端口" width="120">
          <template #default="{ row }">
            <el-tag type="primary">{{ row.port }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="name" label="访问方式" width="160">
          <template #default="{ row }">
            <span class="svc-name">{{ row.name }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="说明" min-width="200" show-overflow-tooltip />
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDialog(row)">编辑</el-button>
            <el-popconfirm title="确定要删除该端口配置吗？" @confirm="handleDelete(row)">
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card style="margin-top: 20px;">
      <template #header>
        <div class="card-header">
          <span>界面设置</span>
        </div>
      </template>
      <el-row :gutter="20">
        <el-col :span="12">
          <el-form-item label="界面主题">
            <el-radio-group v-model="theme" @change="handleThemeChange">
              <el-radio-button :label="'light'">{{ t('settings.themeLight') }}</el-radio-button>
              <el-radio-button :label="'dark'">{{ t('settings.themeDark') }}</el-radio-button>
              <el-radio-button :label="'system'">{{ t('settings.themeSystem') }}</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </el-col>
        <el-col :span="12">
          <el-form-item label="界面语言">
            <el-radio-group v-model="locale" @change="handleLocaleChange">
              <el-radio-button :label="'zh-CN'">中文</el-radio-button>
              <el-radio-button :label="'en-US'">English</el-radio-button>
            </el-radio-group>
          </el-form-item>
        </el-col>
      </el-row>
    </el-card>

    <el-card style="margin-top: 20px;">
      <template #header>
        <div class="card-header">
          <span>MCP 智能体接口管理</span>
          <el-button size="small" :loading="mcpLoading" @click="loadMcpAll">刷新</el-button>
        </div>
      </template>

      <el-alert
        title="智能体通过 MCP 协议调用运维能力。明文密码永不下发：读取类不查凭据列，动作类只写空凭据。每次调用均记录审计日志。"
        type="info" show-icon :closable="false" style="margin-bottom: 15px;" />

      <el-descriptions :column="4" border size="small" style="margin-bottom: 15px;">
        <el-descriptions-item label="运行状态">
          <el-tag :type="mcpStatus.running ? 'success' : 'danger'" size="small">{{ mcpStatus.running ? '运行中' : '未运行' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="PID">{{ mcpStatus.pid || '-' }}</el-descriptions-item>
        <el-descriptions-item label="监听">{{ mcpStatus.listen }}</el-descriptions-item>
        <el-descriptions-item label="审计条数">{{ mcpStatus.audit_lines }}</el-descriptions-item>
      </el-descriptions>

      <el-divider content-position="left">危险等级开关（改完立即生效，无需重启）</el-divider>
      <el-row :gutter="15">
        <el-col :span="6" v-for="lv in mcpLevels" :key="lv.key">
          <div class="level-box" :class="{ 'level-off': !mcpStatus.levels[lv.key] }">
            <div class="level-head">
              <el-tag :type="lv.tag" size="small">{{ lv.key }}</el-tag>
              <span class="level-name">{{ lv.name }}</span>
            </div>
            <div class="level-desc">{{ lv.desc }}</div>
            <el-switch v-model="mcpStatus.levels[lv.key]" @change="v => setLevel(lv.key, v)" />
          </div>
        </el-col>
      </el-row>

      <el-divider content-position="left">写入类总开关</el-divider>
      <el-switch v-model="mcpStatus.actions_enabled" active-text="允许写入动作" inactive-text="只读模式" @change="v => setActions(v)" style="margin-bottom: 15px;" />

      <el-divider content-position="left">访问令牌（max_level 限制该令牌可调用的最高危险等级）</el-divider>
      <el-table :data="mcpConfig.tokens" border size="small">
        <el-table-column prop="remark" label="备注" min-width="130" />
        <el-table-column prop="max_level" label="最高级别" width="95">
          <template #default="{ row }">
            <el-tag :type="row.max_level === 'L3' ? 'warning' : 'info'" size="small">{{ row.max_level }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="key" label="Key" min-width="180" show-overflow-tooltip />
        <el-table-column prop="created_at" label="创建时间" width="150" />
        <el-table-column label="操作" width="90" fixed="right">
          <template #default="{ row }">
            <el-popconfirm title="确定吊销？使用它的智能体将立即失效。" width="240" @confirm="revokeToken(row)">
              <template #reference>
                <el-button size="small" type="danger">吊销</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
      <el-button size="small" type="primary" style="margin-top: 10px;" @click="tokenDialogVisible = true; newTokenKey = ''">新建 Token</el-button>

      <el-divider content-position="left">审计日志（最近 50 条）</el-divider>
      <el-table :data="mcpAudit" border size="small" max-height="300">
        <el-table-column prop="time" label="时间" width="155" />
        <el-table-column prop="tool" label="工具" width="185" />
        <el-table-column prop="status" label="状态" width="150">
          <template #default="{ row }">
            <el-tag :type="row.status === 'ok' ? 'success' : 'danger'" size="small">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="ms" label="耗时(ms)" width="85" />
        <el-table-column label="参数" min-width="180">
          <template #default="{ row }">
            <span class="audit-args">{{ JSON.stringify(row.args) }}</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="tokenDialogVisible" title="新建 MCP Token" width="520px">
      <el-form label-width="90px">
        <el-form-item label="备注">
          <el-input v-model="tokenForm.remark" placeholder="例如: 监控智能体" />
        </el-form-item>
        <el-form-item label="最高级别">
          <el-select v-model="tokenForm.max_level" style="width: 220px;">
            <el-option label="L0 只读查询" value="L0" />
            <el-option label="L1 探测扫描" value="L1" />
            <el-option label="L2 写入登记" value="L2" />
            <el-option label="L3 全部（含删除等高危）" value="L3" />
          </el-select>
        </el-form-item>
      </el-form>
      <div v-if="newTokenKey" class="new-token-box">
        <div style="margin-bottom: 5px;">
          <el-tag type="warning" size="small">仅显示一次</el-tag>
          请立即复制保存：
        </div>
        <div class="new-token-key">{{ newTokenKey }}</div>
      </div>
      <template #footer>
        <el-button @click="tokenDialogVisible = false">关闭</el-button>
        <el-button type="primary" :loading="tokenCreating" :disabled="!!newTokenKey" @click="createToken">生成</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑端口' : '添加端口'" width="480px">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="90px">
        <el-form-item label="端口" prop="port">
          <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 200px;" />
        </el-form-item>
        <el-form-item label="访问方式" prop="name">
          <el-input v-model="form.name" placeholder="例如: WEB、SSH、MySQL" />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.description" type="textarea" :rows="2" placeholder="端口用途说明（可选）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { settingsAPI } from '../api'
import api from '../api'
import { useI18n } from 'vue-i18n'
import { themeStore, setTheme } from '../theme.js'
import { localeStore, setLocale } from '../locale.js'

const { t } = useI18n()

const portProfiles = ref([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const formRef = ref(null)

const form = reactive({
  id: 0,
  port: 80,
  name: '',
  description: ''
})

const formRules = {
  port: [{ required: true, message: '请输入端口号', trigger: 'blur' }],
  name: [{ required: true, message: '请输入访问方式名称', trigger: 'blur' }]
}

const theme = ref(themeStore.theme)
const locale = ref(localeStore.locale)

const loadProfiles = async () => {
  loading.value = true
  try {
    portProfiles.value = await settingsAPI.getPortProfiles()
  } catch (error) {
    ElMessage.error('加载端口列表失败')
  } finally {
    loading.value = false
  }
}

const openDialog = (row) => {
  if (row) {
    Object.assign(form, { id: row.id, port: row.port, name: row.name, description: row.description || '' })
  } else {
    Object.assign(form, { id: 0, port: 80, name: '', description: '' })
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const data = { port: form.port, name: form.name, description: form.description }
    if (form.id) {
      await settingsAPI.updatePortProfile(form.id, data)
      ElMessage.success('更新成功')
    } else {
      await settingsAPI.createPortProfile(data)
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadProfiles()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    submitting = false
  }
}

const handleDelete = async (row) => {
  try {
    await settingsAPI.deletePortProfile(row.id)
    ElMessage.success('删除成功')
    loadProfiles()
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

const handleThemeChange = (value) => {
  setTheme(value)
  ElMessage.success('主题设置已更新')
}

const handleLocaleChange = (value) => {
  setLocale(value)
  ElMessage.success('语言设置已更新')
}


// ---- MCP 智能体接口管理 ----
const mcpStatus = ref({ running: false, pid: '', listen: '', audit_lines: 0, levels: {}, actions_enabled: true })
const mcpConfig = ref({ tokens: [] })
const mcpAudit = ref([])
const mcpLoading = ref(false)
const tokenDialogVisible = ref(false)
const tokenCreating = ref(false)
const newTokenKey = ref('')
const tokenForm = reactive({ remark: '', max_level: 'L1' })

const mcpLevels = [
  { key: 'L0', name: '只读查询', tag: 'info', desc: '查看服务器/服务/网络探测' },
  { key: 'L1', name: '主动探测', tag: 'primary', desc: '空闲IP扫描、端口扫描' },
  { key: 'L2', name: '写入登记', tag: 'warning', desc: '登记端口为服务、登记服务器' },
  { key: 'L3', name: '删除/高危', tag: 'danger', desc: '删除服务记录等（默认禁用）' },
]

const loadMcpAll = async () => {
  mcpLoading.value = true
  try {
    const [st, cfg] = await Promise.all([api.get('/mcp/status'), api.get('/mcp/config')])
    mcpStatus.value = { ...mcpStatus.value, ...st, levels: { ...st.levels } }
    mcpConfig.value = cfg
    const a = await api.get('/mcp/audit?lines=50')
    mcpAudit.value = (a.lines || []).map(l => { try { return JSON.parse(l) } catch { return null } }).filter(Boolean)
  } catch (e) {
    // 非管理员 403 时静默不展示
  } finally {
    mcpLoading.value = false
  }
}

const setLevel = async (lv, v) => {
  try {
    await api.post('/mcp/levels', { [lv]: v })
    ElMessage.success(lv + ' 级已' + (v ? '开启' : '禁用') + '，立即生效')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '设置失败')
    loadMcpAll()
  }
}

const setActions = async (v) => {
  try {
    await api.post('/mcp/actions', { enabled: v })
    ElMessage.success(v ? '已允许写入动作' : '已切换为只读模式')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '设置失败')
    loadMcpAll()
  }
}

const createToken = async () => {
  tokenCreating.value = true
  try {
    const r = await api.post('/mcp/tokens', { remark: tokenForm.remark, max_level: tokenForm.max_level })
    newTokenKey.value = r.key
    loadMcpAll()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '创建失败')
  } finally {
    tokenCreating.value = false
  }
}

const revokeToken = async (row) => {
  try {
    await api.post('/mcp/tokens/revoke', { key: row.key })
    ElMessage.success('已吊销')
    loadMcpAll()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '吊销失败')
  }
}

onMounted(() => {
  loadProfiles()
  loadMcpAll()
  // 从本地存储恢复设置
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme) {
    theme.value = savedTheme
  }
  const savedLocale = localStorage.getItem('locale')
  if (savedLocale) {
    locale.value = savedLocale
  }
})

onUnmounted(() => {
  // 持久化设置
  localStorage.setItem('theme', theme.value)
  localStorage.setItem('locale', locale.value)
})
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.svc-name {
  font-weight: 500;
}

.level-box {
  border: 1px solid var(--el-border-color-light);
  border-radius: 6px;
  padding: 10px 12px;
  transition: border-color 0.2s;
}
.level-box.level-off {
  opacity: 0.55;
  border-style: dashed;
}
.level-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 4px;
}
.level-name {
  font-weight: 600;
}
.level-desc {
  font-size: 12px;
  color: var(--el-text-color-secondary);
  margin-bottom: 8px;
  min-height: 18px;
}
.new-token-box {
  background: var(--el-fill-color-light);
  border: 1px dashed var(--el-color-warning);
  border-radius: 6px;
  padding: 12px;
}
.new-token-key {
  font-family: monospace;
  word-break: break-all;
  font-size: 13px;
  color: var(--el-color-warning-dark-2);
}
.audit-args {
  font-family: monospace;
  font-size: 12px;
}
</style>
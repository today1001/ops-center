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

onMounted(() => {
  loadProfiles()
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
</style>
<template>
  <div class="group-manager">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>服务器分组管理</span>
          <div class="header-actions">
            <el-button size="small" :icon="Refresh" circle @click="loadGroups" :loading="loading" />
            <el-button type="primary" :icon="Plus" @click="openDialog()">添加分组</el-button>
          </div>
        </div>
      </template>

      <el-alert
        title="分组用于对服务器进行分类管理。添加服务器时可在「服务器管理」中选择分组，此处可对分组进行新增、编辑（重命名）、删除。删除分组后，该组下的服务器将归入「未分组」。"
        type="info"
        show-icon
        :closable="false"
        style="margin-bottom: 15px;"
      />

      <el-table :data="groups" v-loading="loading" border>
        <el-table-column prop="id" label="ID" width="70" />
        <el-table-column prop="name" label="分组名称" min-width="160">
          <template #default="{ row }">
            <el-tag type="primary" effect="plain">{{ row.name }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="说明" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">
            <span :class="row.description ? '' : 'muted'">{{ row.description || '暂无说明' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="server_count" label="服务器数量" width="120">
          <template #default="{ row }">
            <span class="server-count">{{ row.server_count }} 台</span>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDialog(row)">编辑</el-button>
            <el-popconfirm
              :title="row.server_count > 0 ? `删除后该组下 ${row.server_count} 台服务器将归入未分组，确定删除？` : '确定要删除该分组吗？'"
              @confirm="handleDelete(row)"
            >
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
        <template #empty>
          <el-empty description="暂无分组，点击右上角「添加分组」创建" :image-size="60" />
        </template>
      </el-table>
    </el-card>

    <el-dialog v-model="dialogVisible" :title="form.id ? '编辑分组' : '添加分组'" width="480px">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="90px">
        <el-form-item label="分组名称" prop="name">
          <el-input v-model="form.name" placeholder="例如: 集群、数据库、宿主机" maxlength="50" show-word-limit />
        </el-form-item>
        <el-form-item label="说明">
          <el-input v-model="form.description" type="textarea" :rows="3" placeholder="分组用途说明（可选）" />
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
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { groupAPI } from '../api'

const groups = ref([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const formRef = ref(null)

const form = reactive({
  id: 0,
  name: '',
  description: ''
})

const formRules = {
  name: [{ required: true, message: '请输入分组名称', trigger: 'blur' }]
}

const loadGroups = async () => {
  loading.value = true
  try {
    groups.value = await groupAPI.getList()
  } catch (error) {
    ElMessage.error('加载分组列表失败')
  } finally {
    loading.value = false
  }
}

const openDialog = (row) => {
  if (row) {
    Object.assign(form, { id: row.id, name: row.name, description: row.description || '' })
  } else {
    Object.assign(form, { id: 0, name: '', description: '' })
  }
  dialogVisible.value = true
}

const handleSubmit = async () => {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    const data = { name: form.name, description: form.description }
    if (form.id) {
      await groupAPI.update(form.id, data)
      ElMessage.success('更新成功')
    } else {
      await groupAPI.create(data)
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    loadGroups()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    submitting.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await groupAPI.delete(row.id)
    ElMessage.success('删除成功')
    loadGroups()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '删除失败')
  }
}

const formatTime = (t) => {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

onMounted(loadGroups)
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.header-actions {
  display: flex;
  gap: 8px;
  align-items: center;
}

.server-count {
  font-weight: 500;
  color: #606266;
}

.muted {
  color: #909399;
}
</style>

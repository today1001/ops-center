<template>
  <div class="audit-logs">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>访问日志</h3>
          <div class="filters">
            <el-select v-model="filterAction" placeholder="动作类型" clearable size="default" style="width: 150px" @change="loadLogs(true)">
              <el-option label="登录成功" value="login" />
              <el-option label="登录失败" value="login_failed" />
              <el-option label="写操作" value="write" />
              <el-option label="读操作" value="access" />
              <el-option label="授权" value="grant" />
              <el-option label="回收" value="revoke" />
            </el-select>
            <el-input
              v-model="filterUsername"
              placeholder="按用户名过滤"
              clearable
              style="width: 180px"
              @clear="loadLogs(true)"
              @keyup.enter="loadLogs(true)"
            />
            <el-button @click="loadLogs(true)">
              <el-icon><Refresh /></el-icon>&nbsp;刷新
            </el-button>
          </div>
        </div>
      </template>

      <el-table :data="logs" v-loading="loading" stripe size="small">
        <el-table-column label="时间" width="170">
          <template #default="scope">{{ fmtTime(scope.row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="username" label="用户" width="100">
          <template #default="scope">{{ scope.row.username || '（未登录）' }}</template>
        </el-table-column>
        <el-table-column label="动作" width="100">
          <template #default="scope">
            <el-tag :type="actionTagType(scope.row.action)" size="small">{{ actionLabel(scope.row.action) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="method" label="方法" width="70" />
        <el-table-column prop="path" label="路径" min-width="220" show-overflow-tooltip />
        <el-table-column prop="ip" label="来源IP" width="130" />
        <el-table-column label="状态" width="70">
          <template #default="scope">
            <span :class="scope.row.status >= 400 ? 'st-err' : 'st-ok'">{{ scope.row.status || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column label="耗时" width="80">
          <template #default="scope">{{ scope.row.duration_ms != null ? scope.row.duration_ms + 'ms' : '-' }}</template>
        </el-table-column>
        <el-table-column prop="detail" label="详情" min-width="160" show-overflow-tooltip />
      </el-table>

      <div class="pager">
        <el-pagination
          layout="total, prev, pager, next, sizes"
          :total="total"
          :page-size="pageSize"
          :current-page="page"
          :page-sizes="[50, 100, 200]"
          @current-change="onPage"
          @size-change="onSize"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { auditAPI } from '../api'

const logs = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const loading = ref(false)
const filterAction = ref('')
const filterUsername = ref('')

const actionLabel = (a) => ({
  login: '登录成功', login_failed: '登录失败', write: '写操作',
  access: '读操作', grant: '授权', revoke: '回收'
}[a] || a)
const actionTagType = (a) => ({
  login: 'success', login_failed: 'danger', write: 'warning',
  access: 'info', grant: 'primary', revoke: 'danger'
}[a] || 'info')

const fmtTime = (t) => (t || '').replace('T', ' ').substring(0, 19)

const loadLogs = async (reset) => {
  if (reset) page.value = 1
  loading.value = true
  try {
    const data = await auditAPI.list({
      limit: pageSize.value,
      offset: (page.value - 1) * pageSize.value,
      action: filterAction.value || undefined,
      username: filterUsername.value || undefined
    })
    logs.value = data.logs || []
    total.value = data.total || 0
  } catch (e) {
    ElMessage.error('获取日志失败')
  } finally {
    loading.value = false
  }
}

const onPage = (p) => {
  page.value = p
  loadLogs(false)
}
const onSize = (s) => {
  pageSize.value = s
  loadLogs(true)
}

onMounted(() => loadLogs(true))
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
.filters {
  display: flex;
  gap: 8px;
  align-items: center;
}
.pager {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}
.st-ok {
  color: #67c23a;
  font-weight: 600;
}
.st-err {
  color: #f56c6c;
  font-weight: 600;
}
</style>

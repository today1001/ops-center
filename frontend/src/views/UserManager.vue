<template>
  <div class="user-manager">
    <el-card>
      <template #header>
        <div class="card-header">
          <h3>用户与权限管理</h3>
          <el-button type="primary" @click="openCreate">
            <el-icon><Plus /></el-icon>&nbsp;新建用户
          </el-button>
        </div>
      </template>

      <el-table :data="users" v-loading="loading" stripe>
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column label="角色" width="130">
          <template #default="scope">
            <el-tag :type="roleTagType(scope.row.role)" effect="dark" size="small">
              {{ roleLabel(scope.row.role) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="email" label="邮箱" min-width="150" show-overflow-tooltip />
        <el-table-column prop="group_count" label="授权分组" width="90" align="center" />
        <el-table-column prop="server_count" label="授权服务器" width="100" align="center" />
        <el-table-column label="创建时间" width="110">
          <template #default="scope">{{ fmtDate(scope.row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="scope">
            <el-button size="small" type="primary" plain @click="openPerm(scope.row)">授权</el-button>
            <el-button size="small" plain @click="openEdit(scope.row)">编辑</el-button>
            <el-button
              size="small" type="danger" plain
              :disabled="scope.row.id === currentUserId"
              @click="handleDelete(scope.row)"
            >删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新建/编辑用户 -->
    <el-dialog v-model="editVisible" :title="editing.id ? '编辑用户' : '新建用户'" width="460px">
      <el-form :model="editing" label-width="80px">
        <el-form-item label="用户名" required>
          <el-input v-model="editing.username" :disabled="!!editing.id" placeholder="登录用户名" />
        </el-form-item>
        <el-form-item :label="editing.id ? '重置密码' : '密码'" :required="!editing.id">
          <el-input v-model="editing.password" type="password" show-password
            :placeholder="editing.id ? '留空则不修改密码' : '至少 6 位'" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="editing.email" placeholder="选填" />
        </el-form-item>
        <el-form-item label="角色" required>
          <el-select v-model="editing.role" style="width: 100%">
            <el-option v-for="r in roleOptions" :key="r.value" :value="r.value" :label="r.label" />
          </el-select>
          <div class="role-hint">{{ roleHint(editing.role) }}</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
      </template>
    </el-dialog>

    <!-- 授权抽屉 -->
    <el-drawer v-model="permVisible" :title="`授权管理 - ${permUser.username}`" size="520px">
      <el-tabs v-model="permTab">
        <el-tab-pane label="分组授权" name="groups">
          <div class="perm-tip">勾选后该用户可见并管理分组内<b>全部</b>服务器</div>
          <el-checkbox-group v-model="grantedGroupIds" @change="onGroupChange">
            <div v-for="g in allGroups" :key="g.id" class="perm-row">
              <el-checkbox :value="g.id">
                <b>{{ g.name }}</b>
                <span class="perm-count">（{{ g.server_count }} 台）</span>
              </el-checkbox>
            </div>
            <el-empty v-if="!allGroups.length" description="暂无分组，请先到分组管理创建" :image-size="60" />
          </el-checkbox-group>
        </el-tab-pane>
        <el-tab-pane label="服务器授权" name="servers">
          <div class="perm-tip">勾选后该用户可访问指定服务器（viewer 角色只读）</div>
          <el-input v-model="serverFilter" placeholder="搜索服务器名称 / IP" clearable size="small" style="margin-bottom: 8px" />
          <el-checkbox-group v-model="grantedServerIds" @change="onServerChange">
            <div v-for="s in filteredServers" :key="s.id" class="perm-row">
              <el-checkbox :value="s.id">
                <b>{{ s.name }}</b>
                <span class="perm-count">（{{ s.ip }} · {{ s.status }}）</span>
              </el-checkbox>
            </div>
          </el-checkbox-group>
        </el-tab-pane>
      </el-tabs>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus } from '@element-plus/icons-vue'
import { userAdminAPI } from '../api'

const users = ref([])
const loading = ref(false)
const saving = ref(false)

const editVisible = ref(false)
const editing = ref({ id: 0, username: '', password: '', email: '', role: 'viewer' })

const permVisible = ref(false)
const permTab = ref('groups')
const permUser = ref({ id: 0, username: '' })
const allGroups = ref([])
const allServers = ref([])
const grantedGroupIds = ref([])
const grantedServerIds = ref([])
const serverFilter = ref('')

const currentUserId = ref(0)
// 基线快照：用于对比勾选变化（新增逐项授予，取消逐项移除）
const permGroupBaseline = ref([])
const permServerBaseline = ref([])

const roleOptions = [
  { value: 'admin', label: '超级管理员' },
  { value: 'group_admin', label: '分组管理员' },
  { value: 'server_admin', label: '服务器管理员' },
  { value: 'viewer', label: '只读观察者' }
]

const filteredServers = computed(() => {
  if (!serverFilter.value) return allServers.value
  const kw = serverFilter.value.toLowerCase()
  return allServers.value.filter(s =>
    s.name.toLowerCase().includes(kw) || s.ip.toLowerCase().includes(kw))
})

const roleLabel = (r) => (roleOptions.find(o => o.value === r) || {}).label || r
const roleTagType = (r) => ({
  admin: 'danger', group_admin: 'warning', server_admin: 'primary', viewer: 'info'
}[r] || 'info')
const roleHint = (r) => ({
  admin: '全部权限，含用户管理与系统设置',
  group_admin: '授权分组内全部服务器可管理',
  server_admin: '仅授权的服务器可管理',
  viewer: '仅授权的服务器可查看（只读）'
}[r] || '')

const fmtDate = (d) => (d || '').substring(0, 10)

const loadUsers = async () => {
  loading.value = true
  try {
    users.value = await userAdminAPI.listUsers()
  } catch (e) {
    ElMessage.error('获取用户列表失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  const u = JSON.parse(localStorage.getItem('user') || '{}')
  currentUserId.value = u.id || 0
  loadUsers()
})

const openCreate = () => {
  editing.value = { id: 0, username: '', password: '', email: '', role: 'viewer' }
  editVisible.value = true
}

const openEdit = (row) => {
  editing.value = { id: row.id, username: row.username, password: '', email: row.email || '', role: row.role }
  editVisible.value = true
}

const handleSave = async () => {
  saving.value = true
  try {
    if (editing.value.id) {
      const payload = { role: editing.value.role, email: editing.value.email }
      if (editing.value.password) payload.password = editing.value.password
      await userAdminAPI.updateUser(editing.value.id, payload)
      ElMessage.success('更新成功')
    } else {
      await userAdminAPI.createUser(editing.value)
      ElMessage.success('创建成功')
    }
    editVisible.value = false
    loadUsers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除用户「${row.username}」？其授权关系将一并清除。`, '删除确认', { type: 'warning' })
  } catch { return }
  try {
    await userAdminAPI.deleteUser(row.id)
    ElMessage.success('删除成功')
    loadUsers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

const openPerm = async (row) => {
  permUser.value = row
  permTab.value = 'groups'
  permVisible.value = true
  try {
    const data = await userAdminAPI.getPermissions(row.id)
    allGroups.value = data.all_groups || []
    allServers.value = data.all_servers || []
    grantedGroupIds.value = (data.groups || []).map(g => g.id)
    grantedServerIds.value = (data.servers || []).map(s => s.id)
    permGroupBaseline.value = [...grantedGroupIds.value]
    permServerBaseline.value = [...grantedServerIds.value]
  } catch (e) {
    ElMessage.error('获取授权信息失败')
  }
}

// 勾选变化：新增的逐项授予，取消的逐项移除（幂等）
const onGroupChange = async (ids) => {
  const before = new Set(permGroupBaseline.value)
  const now = new Set(ids)
  try {
    const added = ids.filter(i => !before.has(i))
    const removed = permGroupBaseline.value.filter(i => !now.has(i))
    if (added.length) await userAdminAPI.grantGroups(permUser.value.id, added)
    for (const gid of removed) await userAdminAPI.revokeGroup(permUser.value.id, gid)
    permGroupBaseline.value = [...ids]
    if (added.length || removed.length) ElMessage.success('授权已更新')
    loadUsers()
  } catch (e) {
    ElMessage.error('授权操作失败')
    grantedGroupIds.value = [...permGroupBaseline.value]
  }
}

const onServerChange = async (ids) => {
  const before = new Set(permServerBaseline.value)
  const now = new Set(ids)
  try {
    const added = ids.filter(i => !before.has(i))
    const removed = permServerBaseline.value.filter(i => !now.has(i))
    if (added.length) await userAdminAPI.grantServers(permUser.value.id, added)
    for (const sid of removed) await userAdminAPI.revokeServer(permUser.value.id, sid)
    permServerBaseline.value = [...ids]
    if (added.length || removed.length) ElMessage.success('授权已更新')
    loadUsers()
  } catch (e) {
    ElMessage.error('授权操作失败')
    grantedServerIds.value = [...permServerBaseline.value]
  }
}
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
.role-hint {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}
.perm-tip {
  font-size: 12px;
  color: #909399;
  margin-bottom: 10px;
}
.perm-row {
  padding: 4px 0;
}
.perm-count {
  color: #909399;
  font-size: 12px;
}
</style>

<template>
  <div class="vnet-page">
    <!-- 网络源管理 -->
    <el-card>
      <template #header>
        <div class="card-header">
          <span>虚拟网络源</span>
          <div class="header-actions">
            <el-button size="small" @click="doSync" :loading="syncing">立即同步</el-button>
            <el-button size="small" type="primary" :icon="Plus" @click="openNetworkDialog()">添加网络源</el-button>
          </div>
        </div>
      </template>

      <el-alert
        v-if="lastSync && lastSync.time"
        :type="lastSync.ok ? 'success' : 'error'"
        :closable="false"
        style="margin-bottom: 12px;"
      >
        <template #title>
          上次同步：{{ formatTime(lastSync.time) }}
          <template v-if="lastSync.changed && lastSync.changed.length">
            ，IP变化：{{ lastSync.changed.join('；') }}
          </template>
          <template v-if="lastSync.offline && lastSync.offline.length">
            ，离线：{{ lastSync.offline.join('、') }}
          </template>
          <template v-if="lastSync.error">，错误：{{ lastSync.error }}</template>
        </template>
      </el-alert>

      <el-table :data="networks" size="small" style="width: 100%">
        <el-table-column prop="name" label="名称" width="140" />
        <el-table-column prop="type" label="类型" width="150">
          <template #default="{ row }">
            <el-tag size="small">{{ typeLabel(row.type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="rpc_addr" label="连接地址" min-width="160" />
        <el-table-column prop="subnet" label="虚拟网段" width="150" />
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="140">
          <template #default="{ row }">
            <el-button size="small" @click="openNetworkDialog(row)">编辑</el-button>
            <el-popconfirm title="确定删除该网络源？" @confirm="handleDeleteNetwork(row)">
              <template #reference>
                <el-button size="small" type="danger">删除</el-button>
              </template>
            </el-popconfirm>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 在线节点 -->
    <el-card style="margin-top: 16px;">
      <template #header>
        <div class="card-header">
          <span>虚拟网络在线节点（实时）</span>
          <div class="header-actions">
            <el-button size="small" :icon="Refresh" circle @click="loadPeers" :loading="peersLoading" />
          </div>
        </div>
      </template>

      <el-table :data="peers" size="small" style="width: 100%" v-loading="peersLoading">
        <el-table-column prop="network" label="所属网络" width="120" />
        <el-table-column prop="hostname" label="主机标识" min-width="150" />
        <el-table-column prop="ip" label="当前虚拟IP" width="150" />
        <el-table-column label="绑定状态" min-width="180">
          <template #default="{ row }">
            <template v-if="row.bound">
              <el-tag size="small" type="success">{{ row.binding.server_name }}</el-tag>
              <el-button size="small" link type="danger" @click="handleUnbind(row)">解绑</el-button>
            </template>
            <template v-else>
              <el-tag size="small" type="warning">未绑定</el-tag>
              <el-button size="small" link type="primary" @click="openBindDialog(row)">绑定到服务器</el-button>
            </template>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="160" show-overflow-tooltip />
      </el-table>
      <div class="tip">绑定后，节点虚拟 IP 变化时自动更新对应服务器及其服务地址（每 60 秒自动同步）</div>
    </el-card>

    <!-- 网络源编辑对话框 -->
    <el-dialog v-model="networkDialog" :title="networkForm.id ? '编辑网络源' : '添加网络源'" width="480px">
      <el-form ref="networkFormRef" :model="networkForm" :rules="networkRules" label-width="90px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="networkForm.name" placeholder="例如: LtNet" />
        </el-form-item>
        <el-form-item label="类型" prop="type">
          <el-select v-model="networkForm.type" style="width: 100%;">
            <el-option label="EasyTier（本机 RPC）" value="easytier-local" />
            <el-option label="EasyTier（远程 RPC）" value="easytier-rpc" />
          </el-select>
        </el-form-item>
        <el-form-item label="RPC 地址" prop="rpc_addr">
          <el-input v-model="networkForm.rpc_addr" placeholder="例如: 127.0.0.1:15888 或 10.x.x.x:15888" />
        </el-form-item>
        <el-form-item label="虚拟网段" prop="subnet">
          <el-input v-model="networkForm.subnet" placeholder="例如: 10.10.10.0/24（用于识别虚拟IP）" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="networkForm.enabledBool" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="networkDialog = false">取消</el-button>
        <el-button type="primary" @click="handleSaveNetwork" :loading="saving">保存</el-button>
      </template>
    </el-dialog>

    <!-- 绑定对话框 -->
    <el-dialog v-model="bindDialog" title="绑定节点到服务器" width="420px">
      <el-form label-width="90px">
        <el-form-item label="节点">
          <span>{{ bindForm.hostname }}（{{ bindForm.ip }}）</span>
        </el-form-item>
        <el-form-item label="服务器">
          <el-select v-model="bindForm.server_id" filterable placeholder="选择服务器" style="width: 100%;">
            <el-option v-for="s in servers" :key="s.id" :label="s.name + ' (' + s.ip + ')'" :value="s.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="bindDialog = false">取消</el-button>
        <el-button type="primary" @click="handleBind" :loading="saving">绑定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { Plus, Refresh } from '@element-plus/icons-vue'
import { vnetAPI, serverAPI } from '../api'

const networks = ref([])
const peers = ref([])
const servers = ref([])
const peersLoading = ref(false)
const syncing = ref(false)
const saving = ref(false)
const lastSync = ref(null)
let refreshTimer = null

const networkDialog = ref(false)
const networkFormRef = ref(null)
const networkForm = reactive({ id: 0, name: '', type: 'easytier-local', rpc_addr: '127.0.0.1:15888', subnet: '', enabledBool: true })

const bindDialog = ref(false)
const bindForm = reactive({ network: '', hostname: '', ip: '', server_id: 0 })

const networkRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  rpc_addr: [{ required: true, message: '请输入连接地址', trigger: 'blur' }]
}

const typeLabel = (t) => {
  const map = { 'easytier-local': 'EasyTier 本机', 'easytier-rpc': 'EasyTier 远程' }
  return map[t] || t
}

const formatTime = (t) => {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

const loadNetworks = async () => {
  try {
    networks.value = await vnetAPI.getNetworks()
  } catch (e) {
    ElMessage.error('加载网络源失败')
  }
}

const loadPeers = async () => {
  peersLoading.value = true
  try {
    peers.value = await vnetAPI.getPeers()
  } catch (e) {
    // 静默
  } finally {
    peersLoading.value = false
  }
}

const loadLastSync = async () => {
  try {
    lastSync.value = await vnetAPI.getLastSync()
  } catch (e) {}
}

const loadServers = async () => {
  try {
    servers.value = await serverAPI.getList()
  } catch (e) {}
}

const doSync = async () => {
  syncing.value = true
  try {
    lastSync.value = await vnetAPI.sync()
    await loadPeers()
    await loadServers()
    ElMessage.success('同步完成')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '同步失败')
  } finally {
    syncing.value = false
  }
}

const openNetworkDialog = (row) => {
  if (row) {
    Object.assign(networkForm, { id: row.id, name: row.name, type: row.type, rpc_addr: row.rpc_addr, subnet: row.subnet, enabledBool: !!row.enabled })
  } else {
    Object.assign(networkForm, { id: 0, name: '', type: 'easytier-local', rpc_addr: '127.0.0.1:15888', subnet: '', enabledBool: true })
  }
  networkDialog.value = true
}

const handleSaveNetwork = async () => {
  const valid = await networkFormRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const data = { name: networkForm.name, type: networkForm.type, rpc_addr: networkForm.rpc_addr, subnet: networkForm.subnet, enabled: networkForm.enabledBool ? 1 : 0 }
    if (networkForm.id) {
      await vnetAPI.updateNetwork(networkForm.id, data)
      ElMessage.success('更新成功')
    } else {
      await vnetAPI.addNetwork(data)
      ElMessage.success('添加成功')
    }
    networkDialog.value = false
    loadNetworks()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const handleDeleteNetwork = async (row) => {
  try {
    await vnetAPI.deleteNetwork(row.id)
    ElMessage.success('删除成功')
    loadNetworks()
  } catch (e) {
    ElMessage.error('删除失败')
  }
}

const openBindDialog = (row) => {
  Object.assign(bindForm, { network: row.network, hostname: row.hostname, ip: row.ip, server_id: 0 })
  loadServers()
  bindDialog.value = true
}

const handleBind = async () => {
  if (!bindForm.server_id) {
    ElMessage.warning('请选择服务器')
    return
  }
  saving.value = true
  try {
    await vnetAPI.bind({ network: bindForm.network, hostname: bindForm.hostname, ip: bindForm.ip, server_id: bindForm.server_id })
    ElMessage.success('绑定成功')
    bindDialog.value = false
    loadPeers()
    loadServers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '绑定失败')
  } finally {
    saving.value = false
  }
}

const handleUnbind = async (row) => {
  if (!row.binding) return
  try {
    await vnetAPI.unbind({ server_id: row.binding.server_id })
    ElMessage.success('解绑成功')
    loadPeers()
    loadServers()
  } catch (e) {
    ElMessage.error('解绑失败')
  }
}

onMounted(() => {
  loadNetworks()
  loadPeers()
  loadLastSync()
  loadServers()
  refreshTimer = setInterval(() => { loadPeers(); loadLastSync() }, 30000)
})

onBeforeUnmount(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
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
}

.tip {
  margin-top: 10px;
  font-size: 12px;
  color: #909399;
}
</style>

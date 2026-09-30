<template>
  <div class="vnet-page">
    <!-- 常见虚拟网络 -->
    <el-card>
      <template #header>
        <div class="card-header"><span>常见虚拟网络</span></div>
      </template>
      <div class="provider-row">
        <div class="provider-card" @click="openJoinDialog()">
          <div class="provider-name">EasyTier</div>
          <div class="provider-desc">点击配置连接参数：已连接则纳入管理，未连接则自动创建 tun 和连接</div>
          <el-tag size="small" type="success" class="provider-tag">支持</el-tag>
        </div>
        <div class="provider-card disabled">
          <div class="provider-name">WireGuard</div>
          <div class="provider-desc">敬请期待</div>
          <el-tag size="small" type="info" class="provider-tag">规划中</el-tag>
        </div>
        <div class="provider-card disabled">
          <div class="provider-name">ZeroTier</div>
          <div class="provider-desc">敬请期待</div>
          <el-tag size="small" type="info" class="provider-tag">规划中</el-tag>
        </div>
      </div>
    </el-card>

    <!-- 已配置的连接 -->
    <el-card style="margin-top: 16px;">
      <template #header>
        <div class="card-header">
          <span>已配置的连接</span>
          <el-button size="small" link type="primary" @click="openManualDialog">手动添加RPC源</el-button>
        </div>
      </template>

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
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <template v-if="row.managed">
              <el-button size="small" type="warning" @click="handleStopInstance(row)">停止</el-button>
              <el-popconfirm title="停止并删除该实例及配置？" @confirm="handleDeleteInstance(row)">
                <template #reference>
                  <el-button size="small" type="danger">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
            <template v-else>
              <el-button size="small" @click="openManualDialog(row)">编辑</el-button>
              <el-popconfirm title="确定删除该网络源？" @confirm="handleDeleteNetwork(row)">
                <template #reference>
                  <el-button size="small" type="danger">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </template>
        </el-table-column>
      </el-table>
      <div class="tip">本平台创建的实例可停止/删除；外部已有网络（如 systemd 启动的）仅纳入管理与跟踪</div>
    </el-card>

    <!-- 在线节点 -->
    <el-card style="margin-top: 16px;">
      <template #header>
        <div class="card-header">
          <span>虚拟网络在线节点（实时）</span>
          <div class="header-actions">
            <el-button size="small" @click="doSync" :loading="syncing">立即同步</el-button>
            <el-button size="small" :icon="Refresh" circle @click="loadPeers" :loading="peersLoading" />
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
          <template v-if="lastSync.changed && lastSync.changed.length">，IP变化：{{ lastSync.changed.join('；') }}</template>
          <template v-if="lastSync.offline && lastSync.offline.length">，离线：{{ lastSync.offline.join('、') }}</template>
        </template>
      </el-alert>

      <el-table :data="peers" size="small" style="width: 100%" v-loading="peersLoading">
        <el-table-column prop="network" label="所属网络" width="120" />
        <el-table-column prop="hostname" label="主机标识" min-width="150" />
        <el-table-column prop="ip" label="当前虚拟IP" width="150" />
        <el-table-column label="绑定状态" min-width="220">
          <template #default="{ row }">
            <template v-if="row.bound">
              <el-tag size="small" type="success">{{ row.binding.server_name }}</el-tag>
              <el-button size="small" link type="danger" @click="handleUnbind(row)">解绑</el-button>
            </template>
            <template v-else>
              <el-tag size="small" type="warning">未绑定</el-tag>
              <el-button size="small" link type="primary" @click="openBindDialog(row)">绑定到已有</el-button>
              <el-button size="small" link type="success" @click="openAddServerDialog(row)">添加为服务器</el-button>
            </template>
          </template>
        </el-table-column>
        <el-table-column prop="version" label="版本" width="160" show-overflow-tooltip />
      </el-table>
      <div class="tip">未绑定节点可「添加为服务器」或「绑定到已有」；绑定后虚拟 IP 变化时自动更新对应服务器及其服务地址（每 60 秒自动同步）</div>
    </el-card>

    <!-- 加入/创建虚拟网络对话框 -->
    <el-dialog v-model="joinDialog" title="加入 / 创建虚拟网络（EasyTier）" width="520px">
      <el-form ref="joinFormRef" :model="joinForm" :rules="joinRules" label-width="110px">
        <el-form-item label="网络名称" prop="name">
          <el-input v-model="joinForm.name" placeholder="例如: ltnet（本机已连接该网络则直接纳入管理）" />
        </el-form-item>
        <el-form-item label="网络密钥" prop="secret">
          <el-input v-model="joinForm.secret" placeholder="虚拟网络密码" show-password />
        </el-form-item>
        <el-form-item label="本机虚拟IP" prop="local_ip">
          <el-input v-model="joinForm.local_ip" placeholder="例如: 10.10.20.1/24" />
        </el-form-item>
        <el-form-item label="对端节点">
          <el-input v-model="joinForm.peers" type="textarea" :rows="2" placeholder="对端URI，逗号分隔，可空。例如: udp://1.2.3.4:11010, tcp://5.6.7.8:22222" />
        </el-form-item>
        <el-form-item label="虚拟网段">
          <el-input v-model="joinForm.subnet" placeholder="例如: 10.10.20.0/24（用于识别虚拟IP）" />
        </el-form-item>
        <el-form-item label="开机自启">
          <el-switch v-model="joinForm.autostartBool" />
          <div class="tip">新建实例时生成 systemd 服务，服务器重启后自动重连</div>
        </el-form-item>
      </el-form>
      <div class="tip">保存时先检查本机是否已连接该网络：已连接则直接纳入管理；未连接则自动创建 tun 和 easytier 连接</div>
      <template #footer>
        <el-button @click="joinDialog = false">取消</el-button>
        <el-button type="primary" @click="handleJoin" :loading="saving">保存并连接</el-button>
      </template>
    </el-dialog>

    <!-- 手动添加RPC源对话框 -->
    <el-dialog v-model="manualDialog" :title="networkForm.id ? '编辑网络源' : '手动添加RPC源'" width="480px">
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
        <el-button @click="manualDialog = false">取消</el-button>
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

    <!-- 添加为服务器对话框 -->
    <el-dialog v-model="addServerDialog" title="从虚拟网络节点添加服务器" width="460px">
      <el-form label-width="90px">
        <el-form-item label="节点">
          <span>{{ addForm.hostname }}（{{ addForm.ip }}）</span>
        </el-form-item>
        <el-form-item label="服务器名称">
          <el-input v-model="addForm.name" />
        </el-form-item>
        <el-form-item label="操作系统">
          <el-select v-model="addForm.os" style="width: 100%;">
            <el-option label="Linux" value="Linux" />
            <el-option label="Windows" value="Windows" />
            <el-option label="macOS" value="macOS" />
          </el-select>
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="addForm.username" placeholder="SSH登录用户名" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="addForm.password" type="password" placeholder="SSH登录密码" show-password />
        </el-form-item>
      </el-form>
      <div class="tip">添加后自动创建SSH服务（指向当前虚拟IP），并纳入 easytier-{{ addForm.network }} 分组；虚拟IP变化时自动跟随更新</div>
      <template #footer>
        <el-button @click="addServerDialog = false">取消</el-button>
        <el-button type="primary" @click="handleAddServer" :loading="saving">添加</el-button>
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

// 加入/创建对话框
const joinDialog = ref(false)
const joinFormRef = ref(null)
const joinForm = reactive({ id: 0, name: '', secret: '', local_ip: '', peers: '', subnet: '', autostartBool: true })
const joinRules = {
  name: [{ required: true, message: '请输入网络名称', trigger: 'blur' }],
  local_ip: [{ required: true, message: '请输入本机虚拟IP', trigger: 'blur' }]
}

// 手动RPC源对话框
const manualDialog = ref(false)
const networkFormRef = ref(null)
const networkForm = reactive({ id: 0, name: '', type: 'easytier-local', rpc_addr: '127.0.0.1:15888', subnet: '', enabledBool: true })
const networkRules = {
  name: [{ required: true, message: '请输入名称', trigger: 'blur' }],
  rpc_addr: [{ required: true, message: '请输入连接地址', trigger: 'blur' }]
}

// 绑定 / 添加为服务器
const bindDialog = ref(false)
const bindForm = reactive({ network: '', hostname: '', ip: '', server_id: 0 })
const addServerDialog = ref(false)
const addForm = reactive({ network: '', hostname: '', ip: '', name: '', os: 'Linux', username: '', password: '' })

const typeLabel = (t) => {
  const map = { 'easytier-local': 'EasyTier 本机', 'easytier-rpc': 'EasyTier 远程' }
  return map[t] || t
}

const formatTime = (t) => {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

const loadNetworks = async () => {
  try { networks.value = await vnetAPI.getNetworks() } catch (e) {}
}
const loadPeers = async () => {
  peersLoading.value = true
  try { peers.value = await vnetAPI.getPeers() } catch (e) {} finally { peersLoading.value = false }
}
const loadLastSync = async () => {
  try { lastSync.value = await vnetAPI.getLastSync() } catch (e) {}
}
const loadServers = async () => {
  try { servers.value = await serverAPI.getList() } catch (e) {}
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

// 加入/创建
const openJoinDialog = () => {
  Object.assign(joinForm, { id: 0, name: '', secret: '', local_ip: '', peers: '', subnet: '', autostartBool: true })
  joinDialog.value = true
}

const handleJoin = async () => {
  const valid = await joinFormRef.value.validate().catch(() => false)
  if (!valid) return
  saving.value = true
  try {
    const res = await vnetAPI.join({
      name: joinForm.name, secret: joinForm.secret, local_ip: joinForm.local_ip,
      peers: joinForm.peers, subnet: joinForm.subnet, autostart: joinForm.autostartBool
    })
    ElMessage.success(res.message)
    joinDialog.value = false
    loadNetworks(); loadPeers(); loadServers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  } finally {
    saving.value = false
  }
}

// 手动RPC源
const openManualDialog = (row) => {
  if (row) {
    Object.assign(networkForm, { id: row.id, name: row.name, type: row.type, rpc_addr: row.rpc_addr, subnet: row.subnet, enabledBool: !!row.enabled })
  } else {
    Object.assign(networkForm, { id: 0, name: '', type: 'easytier-rpc', rpc_addr: '', subnet: '', enabledBool: true })
  }
  manualDialog.value = true
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
    manualDialog.value = false
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

// 托管实例停止/删除
const handleStopInstance = async (row) => {
  try {
    const d = await vnetAPI.stopInstance({ id: row.id })
    ElMessage.success(d.message || '已停止')
    loadNetworks()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '停止失败')
  }
}

const handleDeleteInstance = async (row) => {
  try {
    const d = await vnetAPI.deleteInstance({ id: row.id })
    ElMessage.success(d.message || '已删除')
    loadNetworks(); loadPeers(); loadServers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

// 绑定
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

// 添加为服务器
const openAddServerDialog = (row) => {
  Object.assign(addForm, {
    network: row.network, hostname: row.hostname, ip: row.ip,
    name: row.hostname, os: 'Linux', username: '', password: ''
  })
  addServerDialog.value = true
}

const handleAddServer = async () => {
  if (!addForm.name) {
    ElMessage.warning('请输入服务器名称')
    return
  }
  saving.value = true
  try {
    const defaultUser = addForm.os === 'Windows' ? 'administrator' : 'root'
    await serverAPI.create({
      name: addForm.name,
      ip: addForm.ip,
      username: addForm.username || defaultUser,
      password: addForm.password,
      os: addForm.os,
      virtual_network: addForm.network,
      virtual_identifier: addForm.hostname
    })
    ElMessage.success('服务器已添加（SSH服务已自动创建，纳入虚拟网络分组）')
    addServerDialog.value = false
    loadPeers()
    loadServers()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '添加失败')
  } finally {
    saving.value = false
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

.provider-row {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 12px;
}

.provider-card {
  position: relative;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 14px 16px;
  cursor: pointer;
  transition: all .2s;
  background: #fafbfc;
}

.provider-card:hover {
  border-color: #409eff;
  box-shadow: 0 2px 8px rgba(64, 158, 255, .15);
}

.provider-card.disabled {
  opacity: .55;
  cursor: not-allowed;
}

.provider-name {
  font-weight: 600;
  font-size: 15px;
  color: #303133;
}

.provider-desc {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.provider-tag {
  position: absolute;
  top: 10px;
  right: 10px;
}

.tip {
  margin-top: 10px;
  font-size: 12px;
  color: #909399;
}
</style>

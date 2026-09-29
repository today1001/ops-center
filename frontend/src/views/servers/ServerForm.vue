<template>
  <div class="server-form">
    <el-card>
      <template #header>
        <span>{{ isEdit ? '编辑服务器' : '添加服务器' }}</span>
      </template>

      <el-form
        ref="formRef"
        :model="form"
        :rules="rules"
        label-width="100px"
        v-loading="loading"
      >
        <!-- 添加模式切换 -->
        <el-form-item v-if="!isEdit" label="添加模式">
          <el-radio-group v-model="addMode">
            <el-radio label="single">单台添加</el-radio>
            <el-radio label="range">IP段批量</el-radio>
            <el-radio label="paste">粘贴识别</el-radio>
          </el-radio-group>
        </el-form-item>

        <!-- 名称（单台模式） -->
        <el-form-item v-if="addMode === 'single'" label="名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入服务器名称" />
        </el-form-item>

        <!-- 名称前缀（IP段模式） -->
        <el-form-item v-if="addMode === 'range'" label="名称前缀" prop="namePrefix">
          <el-input v-model="form.namePrefix" placeholder="例如: web-server，将自动编号为 web-server-1, web-server-2..." />
        </el-form-item>

        <!-- IP地址（单台模式） -->
        <el-form-item v-if="addMode === 'single'" label="IP地址" prop="ip">
          <el-input v-model="form.ip" placeholder="请输入IP地址" />
        </el-form-item>

        <!-- IP段（IP段模式） -->
        <el-form-item v-if="addMode === 'range'" label="IP段" prop="ipRange">
          <el-input v-model="form.ipRange" placeholder="例如: 192.168.1.100-192.168.1.110 或 192.168.1.100-110" />
          <div class="form-tip">格式: 起始IP-结束IP（支持完整IP或简写最后一位）</div>
        </el-form-item>

        <!-- 粘贴识别（粘贴模式） -->
        <el-form-item v-if="addMode === 'paste'" label="粘贴内容" prop="pasteText">
          <el-input
            v-model="form.pasteText"
            type="textarea"
            :rows="8"
            placeholder="粘贴服务器信息，每行一台，支持格式：&#10;192.168.1.100 root password123&#10;admin@192.168.1.101:22 pass456&#10;root@192.168.1.102 password789&#10;192.168.1.103"
          />
          <div class="form-tip">自动识别 IP、用户名、密码、端口。用户名未识别时默认 root，端口默认 22，系统默认 Linux</div>
        </el-form-item>

        <!-- 默认端口（批量/粘贴模式共用） -->
        <el-form-item v-if="addMode !== 'single'" label="默认端口" prop="port">
          <el-input-number v-model="form.port" :min="1" :max="65535" />
        </el-form-item>

        <!-- 默认用户名（批量/粘贴模式共用） -->
        <el-form-item v-if="addMode !== 'single'" label="默认用户名">
          <el-input v-model="form.username" placeholder="未识别用户名时的默认值（默认root）" />
        </el-form-item>

        <!-- 用户名（单台模式） -->
        <el-form-item v-if="addMode === 'single'" label="用户名">
          <el-input v-model="form.username" placeholder="SSH登录用户名（默认root）" />
          <div class="form-tip">留空默认为 root</div>
        </el-form-item>

        <!-- 端口（单台模式） -->
        <el-form-item v-if="addMode === 'single'" label="SSH端口">
          <el-input-number v-model="form.port" :min="1" :max="65535" />
          <div class="form-tip">默认 22</div>
        </el-form-item>

        <!-- 认证方式（单台模式） -->
        <el-form-item v-if="addMode === 'single'" label="认证方式">
          <el-radio-group v-model="authType">
            <el-radio label="password">密码</el-radio>
            <el-radio label="key">密钥</el-radio>
          </el-radio-group>
        </el-form-item>

        <!-- 密码（单台模式） -->
        <el-form-item v-if="addMode === 'single' && authType === 'password'" label="密码" prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="请输入SSH密码"
            show-password
          />
        </el-form-item>

        <!-- 私钥（单台模式） -->
        <el-form-item v-if="addMode === 'single' && authType === 'key'" label="私钥" prop="private_key">
          <el-input
            v-model="form.private_key"
            type="textarea"
            :rows="6"
            placeholder="请输入SSH私钥内容"
          />
        </el-form-item>

        <el-form-item label="操作系统">
          <el-select v-model="form.os" placeholder="请选择操作系统">
            <el-option label="Linux" value="Linux" />
            <el-option label="Windows" value="Windows" />
            <el-option label="macOS" value="macOS" />
          </el-select>
        </el-form-item>

        <el-form-item label="状态">
          <el-select v-model="form.status" placeholder="请选择状态">
            <el-option label="正常" value="active" />
            <el-option label="忙碌" value="busy" />
            <el-option label="无法连接" value="unreachable" />
            <el-option label="掉线" value="offline" />
          </el-select>
        </el-form-item>

        <!-- 分组 -->
        <el-form-item label="分组">
          <el-select
            v-model="form.group_name"
            placeholder="选择或输入分组"
            allow-create
            filterable
            clearable
            style="width: 100%;"
          >
            <el-option v-for="g in groupOptions" :key="g" :label="g" :value="g" />
          </el-select>
          <div class="form-tip">用于服务器列表分组展示，例如：集群、数据库、宿主机</div>
        </el-form-item>

        <!-- 预览（IP段模式） -->
        <el-form-item v-if="addMode === 'range' && previewIPs.length > 0" label="预览">
          <div class="ip-preview">
            <el-tag v-for="ip in previewIPs.slice(0, 10)" :key="ip" size="small" style="margin: 2px;">
              {{ ip }}
            </el-tag>
            <el-tag v-if="previewIPs.length > 10" size="small" type="info" style="margin: 2px;">
              ...共 {{ previewIPs.length }} 台
            </el-tag>
          </div>
        </el-form-item>

        <!-- 预览（粘贴模式） -->
        <el-form-item v-if="addMode === 'paste'" label="识别结果">
          <div v-if="parsedServers.length === 0" class="form-tip">粘贴内容后将自动识别并显示结果</div>
          <el-table v-else :data="parsedServers" size="small" max-height="300" border>
            <el-table-column prop="name" label="名称" width="140" />
            <el-table-column prop="ip" label="IP" width="130" />
            <el-table-column prop="port" label="端口" width="70" />
            <el-table-column prop="username" label="用户名" width="100" />
            <el-table-column prop="password" label="密码" />
            <el-table-column label="OS" width="80">
              <template #default="{ row }">
                <el-tag size="small">{{ row.os }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-form-item>

        <el-form-item>
          <el-button type="primary" @click="handleSubmit" :loading="submitting">
            {{ isEdit ? '保存' : submitLabel }}
          </el-button>
          <el-button @click="$router.back()">取消</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { serverAPI, groupAPI } from '../../api'

const route = useRoute()
const router = useRouter()
const formRef = ref(null)
const loading = ref(false)
const submitting = ref(false)
const authType = ref('password')
const addMode = ref('single')

const isEdit = computed(() => route.name === 'ServerEdit')

const form = reactive({
  name: '',
  ip: '',
  port: 22,
  username: '',
  password: '',
  private_key: '',
  os: 'Linux',
  status: 'active',
  auth_type: 'password',
  namePrefix: '',
  ipRange: '',
  pasteText: '',
  group_name: ''
})

const groupOptions = ref([])

// 加载已有分组供选择
const loadGroupOptions = async () => {
  try {
    const groups = await groupAPI.getList()
    groupOptions.value = groups.map(g => g.name)
  } catch (e) {
    // 忽略加载失败
  }
}

const rules = {
  name: [{ required: true, message: '请输入服务器名称', trigger: 'blur' }],
  namePrefix: [{ required: true, message: '请输入名称前缀', trigger: 'blur' }],
  ip: [{ required: true, message: '请输入IP地址', trigger: 'blur' }],
  ipRange: [{ required: true, message: '请输入IP段', trigger: 'blur' }],
  pasteText: [{ required: true, message: '请粘贴服务器信息', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  private_key: [{ required: true, message: '请输入私钥', trigger: 'blur' }]
}

// 解析IP段
const previewIPs = computed(() => {
  if (addMode.value !== 'range' || !form.ipRange) return []
  
  const range = form.ipRange.trim()
  const parts = range.split('-')
  if (parts.length !== 2) return []
  
  let startIP = parts[0].trim()
  let endPart = parts[1].trim()
  
  // 简写格式: 192.168.1.100-110
  if (!endPart.includes('.')) {
    const startParts = startIP.split('.')
    if (startParts.length !== 4) return []
    const prefix = startParts.slice(0, 3).join('.')
    startIP = `${prefix}.${startParts[3]}`
    endPart = `${prefix}.${endPart}`
  }
  
  const startParts = startIP.split('.').map(Number)
  const endParts = endPart.split('.').map(Number)
  
  if (startParts.length !== 4 || endParts.length !== 4) return []
  if (startParts.some(isNaN) || endParts.some(isNaN)) return []
  
  const ips = []
  const startNum = (startParts[0] << 24) + (startParts[1] << 16) + (startParts[2] << 8) + startParts[3]
  const endNum = (endParts[0] << 24) + (endParts[1] << 16) + (endParts[2] << 8) + endParts[3]
  
  if (startNum > endNum || endNum - startNum > 255) return []
  
  for (let i = startNum; i <= endNum; i++) {
    const ip = `${(i >> 24) & 255}.${(i >> 16) & 255}.${(i >> 8) & 255}.${i & 255}`
    ips.push(ip)
  }
  
  return ips
})

// 解析粘贴内容，自动识别IP/用户名/密码/端口
const parsePastedServers = (text) => {
  const lines = text.split('\n').map(l => l.trim()).filter(Boolean)
  const servers = []
  const ipRegex = /(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})(?::(\d+))?/
  
  for (const line of lines) {
    // 匹配IP（可能带端口）
    const ipMatch = line.match(ipRegex)
    if (!ipMatch) continue
    
    const ip = ipMatch[1]
    const port = ipMatch[2] ? parseInt(ipMatch[2]) : (form.port || 22)
    
    // 识别用户名：user@ 形式
    const atMatch = line.match(/([^@\s:]+)@/)
    const username = atMatch ? atMatch[1] : (form.username || 'root')
    
    // 提取密码：去除 user@ip:port 和裸IP 后剩余的token
    let rest = line
    rest = rest.replace(/[^@\s]+@[\d.]+(:\d+)?/g, ' ').trim()
    rest = rest.replace(/\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?/g, ' ').trim()
    const tokens = rest.split(/\s+/).filter(Boolean)
    const password = tokens.length > 0 ? tokens[0] : ''
    
    servers.push({
      name: ip,
      ip: ip,
      port: port,
      username: username,
      password: password,
      os: form.os || 'Linux',
      status: 'active',
      auth_type: 'password',
      group_name: form.group_name
    })
  }
  return servers
}

// 粘贴识别结果
const parsedServers = computed(() => {
  if (addMode.value !== 'paste' || !form.pasteText) return []
  return parsePastedServers(form.pasteText)
})

const submitLabel = computed(() => {
  if (addMode.value === 'single') return '添加'
  if (addMode.value === 'range') return `批量添加 (${previewIPs.value.length}台)`
  return `批量添加 (${parsedServers.value.length}台)`
})

onMounted(async () => {
  loadGroupOptions()
  if (isEdit.value) {
    loading.value = true
    try {
      const server = await serverAPI.getById(route.params.id)
      Object.assign(form, server)
      authType.value = server.auth_type || (server.private_key ? 'key' : 'password')
    } catch (error) {
      ElMessage.error('获取服务器信息失败')
      router.back()
    } finally {
      loading.value = false
    }
  }
})

const handleSubmit = async () => {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  // 粘贴模式下有未识别密码的行需拦截提示
  if (addMode.value === 'paste') {
    const noPass = parsedServers.value.filter(s => !s.password)
    if (noPass.length > 0) {
      ElMessage.warning(`有 ${noPass.length} 台服务器未识别到密码: ${noPass.map(s => s.ip).join(', ')}`)
      return
    }
    if (parsedServers.value.length === 0) {
      ElMessage.error('未识别到任何服务器，请检查粘贴格式')
      return
    }
  }

  submitting.value = true
  try {
    if (isEdit.value) {
      const data = { ...form }
      data.username = data.username || 'root'
      data.port = data.port || 22
      data.auth_type = authType.value
      if (authType.value === 'password') {
        delete data.private_key
      } else {
        delete data.password
      }
      await serverAPI.update(route.params.id, data)
      ElMessage.success('更新成功')
      router.push('/servers')
    } else if (addMode.value === 'range') {
      // IP段批量添加
      const servers = previewIPs.value.map((ip) => ({
        name: `${form.namePrefix}-${ip.split('.').pop()}`,
        ip: ip,
        port: form.port,
        username: form.username || 'root',
        password: form.password,
        private_key: '',
        os: form.os,
        status: form.status,
        auth_type: 'password',
        group_name: form.group_name
      }))
      
      const result = await serverAPI.createBatch(servers)
      ElMessage.success(`成功添加 ${result.total} 台服务器`)
      router.push('/servers')
    } else if (addMode.value === 'paste') {
      // 粘贴识别批量添加
      const result = await serverAPI.createBatch(parsedServers.value)
      ElMessage.success(`成功添加 ${result.total} 台服务器`)
      router.push('/servers')
    } else {
      // 单台添加
      const data = { ...form }
      data.username = data.username || 'root'
      data.port = data.port || 22
      data.auth_type = authType.value
      if (authType.value === 'password') {
        delete data.private_key
      } else {
        delete data.password
      }
      await serverAPI.create(data)
      ElMessage.success('添加成功')
      router.push('/servers')
    }
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '操作失败')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.server-form {
  max-width: 720px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.ip-preview {
  max-height: 120px;
  overflow-y: auto;
}
</style>

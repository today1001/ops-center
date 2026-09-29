<template>
  <el-dialog
    :model-value="true"
    :title="dialogTitle"
    width="92%"
    top="4vh"
    destroy-on-close
    :close-on-click-modal="false"
    @close="$emit('close')"
  >
    <!-- 检测到进行中的会话：可加入观看（同一页面，一人操作其他人观看） -->
    <el-alert
      v-if="sessionMatches.length > 0 && !joined"
      type="warning"
      show-icon
      :closable="false"
      style="margin-bottom: 10px;"
    >
      <template #title>
        检测到该地址有 {{ sessionMatches.length }} 个进行中的会话（可能有同事正在操作）
      </template>
      <div style="display: flex; gap: 8px; align-items: center; margin-top: 4px; flex-wrap: wrap;">
        <el-button size="small" type="warning" plain @click="joinSession(sessionMatches[0])">
          加入观看 / 接管控制
        </el-button>
        <span class="join-hint">加入后默认观看；页面顶部点「申请控制」可接管操作</span>
      </div>
    </el-alert>

    <div class="web-toolbar">
      <div class="toolbar-left">
        <el-radio-group v-model="scheme" size="small" @change="loadPage">
          <el-radio-button value="http">http</el-radio-button>
          <el-radio-button value="https">https</el-radio-button>
        </el-radio-group>
        <span class="toolbar-url">{{ proxyUrl }}</span>
      </div>
      <div class="toolbar-actions">
        <el-select v-model="lang" size="small" style="width:100px" @change="loadPage">
          <el-option label="中文" value="zh_CN" />
          <el-option label="English" value="en" />
        </el-select>
        <el-checkbox v-model="autoFill" @change="loadPage">自动填充账号密码</el-checkbox>
        <el-checkbox v-model="autoSubmit" @change="loadPage">自动提交登录</el-checkbox>
        <el-button size="small" :icon="Refresh" circle @click="loadPage" />
        <el-button size="small" type="primary" @click="openNewTab">
          <el-icon><TopRight /></el-icon>
          新窗口打开
        </el-button>
      </div>
    </div>
    <div class="web-frame" v-loading="loading">
      <iframe
        v-if="src"
        :src="src"
        class="web-iframe"
        frameborder="0"
        @load="loading = false"
      ></iframe>
      <el-empty v-else-if="!loading && error" :description="error" />
    </div>
  </el-dialog>
</template>

<script setup>
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh, TopRight } from '@element-plus/icons-vue'
import { toolsAPI } from '../api'

const props = defineProps({
  service: { type: Object, required: true }
})
const emit = defineEmits(['close'])

const src = ref('')
const loading = ref(true)
const error = ref('')
const autoFill = ref(true)
const autoSubmit = ref(false)
const lang = ref('zh_CN')
const proxyUrl = ref('')
const scheme = ref((props.service.access_method || '').toUpperCase() === 'HTTPS' ? 'https' : 'http')
const joined = ref(false)

// 进行中的网页会话（用于加入观看）
const activeSessions = ref([])

const dialogTitle = computed(() => {
  const s = props.service
  const host = s.address || s.server_ip || ''
  return `网页访问 - ${s.name} (${host}${s.port ? ':' + s.port : ''})`
})

// 组装目标地址
const buildTargetUrl = () => {
  const s = props.service
  let host = (s.address || s.server_ip || '').trim()
  if (!host) return ''

  if (/^https?:\/\//i.test(host)) {
    return host
  }

  const schemeNow = scheme.value
  const port = s.port || 0
  const isDefault = (schemeNow === 'http' && port === 80) || (schemeNow === 'https' && port === 443)
  if (port > 0 && !isDefault) {
    return `${schemeNow}://${host}:${port}/`
  }
  return `${schemeNow}://${host}/`
}

// 目标主机与已有会话匹配
const sessionMatches = computed(() => {
  const target = buildTargetUrl()
  if (!target) return []
  let host = ''
  try { host = new URL(target).host } catch (e) { return [] }
  return activeSessions.value.filter(s => {
    try { return new URL(s.base_url).host === host } catch (e) { return false }
  })
})

const loadSessions = async () => {
  try {
    const list = await toolsAPI.getWebSessions()
    activeSessions.value = Array.isArray(list) ? list : []
  } catch (e) {
    activeSessions.value = []
  }
}

// 加入已有会话（观看模式，页面内可申请控制）
const joinSession = (s) => {
  if (!s || !s.tk) return
  proxyUrl.value = s.base_url
  src.value = `/px/${s.tk}/`
  loading.value = true
  joined.value = true
}

const loadPage = async () => {
  const target = buildTargetUrl()
  if (!target) {
    error.value = '该服务没有可用的访问地址'
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  joined.value = false
  try {
    const s = props.service
    const { tk } = await toolsAPI.createWebSession({
      url: target,
      user: s.username || '',
      pass: s.password || '',
      lang: lang.value
    })
    proxyUrl.value = target
    src.value = `/px/${tk}/`
  } catch (e) {
    loading.value = false
    error.value = e.response?.data?.error || '创建网页会话失败'
    ElMessage.error(error.value)
  }
}

// 打开弹窗：先查会话列表，若有匹配的进行中会话则等待用户选择；否则直接新建
loadSessions().finally(() => {
  if (sessionMatches.value.length === 0) {
    loadPage()
  } else {
    loading.value = false
  }
})

const openNewTab = () => {
  if (src.value) {
    window.open(src.value, '_blank')
  }
}
</script>

<style scoped>
.web-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  background-color: #f5f7fa;
  border-radius: 6px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.toolbar-url {
  font-size: 12px;
  color: #606266;
  word-break: break-all;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.join-hint {
  font-size: 12px;
  color: #909399;
}

.web-frame {
  height: 72vh;
  border: 1px solid #e4e7ed;
  border-radius: 6px;
  overflow: hidden;
  position: relative;
  background-color: #fff;
}

.web-iframe {
  width: 100%;
  height: 100%;
  border: none;
  display: block;
}
</style>

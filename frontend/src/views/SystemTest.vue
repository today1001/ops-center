<template>
  <div class="system-test">
    <el-tabs v-model="activeTab" type="border-card">
      <!-- API 测试 -->
      <el-tab-pane label="API 测试" name="api">
        <el-form label-width="100px" style="max-width: 900px;">
          <el-form-item label="接口选择">
            <el-select v-model="apiSelection" style="width: 420px;" @change="onApiSelect">
              <el-option v-for="a in apiList" :key="a.name" :label="a.name" :value="a.name" />
              <el-option label="自定义接口" value="__custom__" />
            </el-select>
            <span class="tip">{{ selectedApi.desc }}</span>
          </el-form-item>

          <template v-if="apiSelection === '__custom__'">
            <el-form-item label="请求方法">
              <el-select v-model="apiMethod" style="width: 150px;">
                <el-option label="GET" value="GET" />
                <el-option label="POST" value="POST" />
                <el-option label="PUT" value="PUT" />
                <el-option label="DELETE" value="DELETE" />
              </el-select>
            </el-form-item>
            <el-form-item label="请求路径">
              <el-input v-model="apiPath" placeholder="/servers 或 /system/components" />
            </el-form-item>
            <el-form-item label="请求体(JSON)">
              <el-input v-model="apiBody" type="textarea" :rows="4" placeholder="POST/PUT 时填写 JSON 请求体（可选）" />
            </el-form-item>
          </template>

          <el-form-item label="请求信息" v-else>
            <el-tag>{{ selectedApi.method }}</el-tag>
            <code class="path-code">{{ selectedApi.path }}</code>
          </el-form-item>

          <el-form-item>
            <el-button type="primary" :loading="apiTesting" @click="runApiTest">执行测试</el-button>
            <el-button @click="clearApiResult">清空</el-button>
          </el-form-item>
        </el-form>

        <el-card v-if="apiResult" shadow="never" class="result-card">
          <template #header>
            <div class="result-header">
              <span>{{ apiResult.method }} {{ apiResult.path }}</span>
              <el-tag :type="apiResult.success ? 'success' : 'danger'" size="small">
                {{ apiResult.status }} · {{ apiResult.elapsed }}ms
              </el-tag>
            </div>
          </template>
          <pre class="result-body">{{ apiResult.body }}</pre>
        </el-card>
      </el-tab-pane>

      <!-- 网络工具 -->
      <el-tab-pane label="网络工具" name="net">
        <el-form label-width="100px" style="max-width: 700px;">
          <el-form-item label="工具类型">
            <el-radio-group v-model="netTool">
              <el-radio label="ping">Ping</el-radio>
              <el-radio label="port">TCP端口</el-radio>
              <el-radio label="dns">DNS解析</el-radio>
              <el-radio label="http">HTTP检查</el-radio>
            </el-radio-group>
          </el-form-item>

          <el-form-item label="目标主机">
            <el-input v-model="netHost" placeholder="IP 或 域名，如 10.172.66.211 或 baidu.com" />
          </el-form-item>

          <el-form-item v-if="netTool === 'ping'" label="次数">
            <el-input-number v-model="netCount" :min="1" :max="20" />
          </el-form-item>

          <el-form-item v-if="netTool === 'port'" label="端口">
            <el-input-number v-model="netPort" :min="1" :max="65535" />
          </el-form-item>

          <el-form-item v-if="netTool === 'http'" label="URL">
            <el-input v-model="netUrl" placeholder="https://example.com" style="width: 420px;" />
          </el-form-item>

          <el-form-item v-if="netTool === 'http'" label="方法">
            <el-select v-model="netMethod" style="width: 150px;">
              <el-option label="GET" value="GET" />
              <el-option label="HEAD" value="HEAD" />
              <el-option label="POST" value="POST" />
            </el-select>
          </el-form-item>

          <el-form-item>
            <el-button type="primary" :loading="netTesting" @click="runNetTest">执行测试</el-button>
          </el-form-item>
        </el-form>

        <el-card v-if="netResult" shadow="never" class="result-card">
          <template #header>
            <div class="result-header">
              <span>{{ netResultTitle }}</span>
              <el-tag :type="netResult.ok ? 'success' : 'warning'" size="small">完成</el-tag>
            </div>
          </template>
          <pre class="result-body">{{ netResult.output }}</pre>
        </el-card>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import { ref, reactive, computed } from 'vue'
import { ElMessage } from 'element-plus'
import api, { serverAPI, testAPI } from '../api'

const activeTab = ref('api')

// ---------- API 测试 ----------
const apiList = [
  { name: '获取当前用户', method: 'GET', path: '/auth/me', desc: '返回当前登录用户信息' },
  { name: '服务器列表', method: 'GET', path: '/servers', desc: '获取所有服务器' },
  { name: '服务器详情', method: 'GET', path: '/servers/{id}', desc: '获取第一台服务器基础信息' },
  { name: '服务器采集详情', method: 'GET', path: '/servers/{id}/detail', desc: '获取第一台服务器采集详情' },
  { name: '测试连接第一台服务器', method: 'POST', path: '/servers/{id}/test', desc: '测试第一台服务器SSH连接' },
  { name: '资源统计', method: 'GET', path: '/servers/stats/resource', desc: '服务器资源总量汇总' },
  { name: '利用率排行', method: 'GET', path: '/servers/stats/rankings', desc: '利用率最高/最低5台' },
  { name: '系统组件列表', method: 'GET', path: '/system/components', desc: '获取系统支撑组件' }
]

const apiSelection = ref(apiList[0].name)
const apiMethod = ref('GET')
const apiPath = ref('/servers')
const apiBody = ref('')

const apiTesting = ref(false)
const apiResult = ref(null)

const selectedApi = computed(() => {
  const found = apiList.find(a => a.name === apiSelection.value)
  return found || apiList[0]
})

const onApiSelect = (name) => {
  if (name === '__custom__') return
  apiMethod.value = selectedApi.value.method
}

const resolvePath = async (path) => {
  // 解析 {id} 模板为第一台服务器ID
  if (path.includes('{id}')) {
    try {
      const servers = await serverAPI.getList()
      if (servers.length === 0) return { path: path.replace('{id}', '1'), ok: false }
      return { path: path.replace('{id}', servers[0].id), ok: true }
    } catch (e) {
      return { path: path.replace('{id}', '1'), ok: false }
    }
  }
  return { path, ok: true }
}

const runApiTest = async () => {
  apiTesting.value = true
  apiResult.value = null
  const start = performance.now()
  try {
      let method, path, body
      if (apiSelection.value === '__custom__') {
        method = apiMethod.value
        path = apiPath.value.trim()
        body = apiBody.value.trim() || undefined
      } else {
        const sel = selectedApi.value
        method = sel.method
        const resolved = await resolvePath(sel.path)
        if (!resolved.ok) {
          ElMessage.warning('没有可用的服务器，无法解析 {id} 参数')
        }
        path = resolved.path
      }

      if (!path) {
        ElMessage.error('请输入请求路径')
        return
      }

      const resp = await api.request({
        method,
        url: path,
        data: body ? JSON.parse(body) : undefined
      })
      const elapsed = Math.round(performance.now() - start)

      apiResult.value = {
        method,
        path,
        status: 200,
        elapsed,
        success: true,
        body: JSON.stringify(resp, null, 2)
      }
    } catch (error) {
      const elapsed = Math.round(performance.now() - start)
      const resp = error.response
      apiResult.value = {
        method: apiSelection.value === '__custom__' ? apiMethod.value : selectedApi.value.method,
        path: apiPath.value || selectedApi.value.path,
        status: resp?.status || 'ERR',
        elapsed,
        success: false,
        body: resp ? JSON.stringify(resp.data || resp, null, 2) : error.message
      }
    } finally {
      apiTesting.value = false
    }
}

const clearApiResult = () => {
  apiResult.value = null
}

// ---------- 网络工具 ----------
const netTool = ref('ping')
const netHost = ref('')
const netCount = ref(4)
const netPort = ref(22)
const netUrl = ref('')
const netMethod = ref('GET')

const netTesting = ref(false)
const netResult = ref(null)

const netResultTitle = computed(() => {
  switch (netTool.value) {
    case 'ping': return `Ping ${netHost.value}`
    case 'port': return `TCP端口 ${netHost.value}:${netPort.value}`
    case 'dns': return `DNS解析 ${netHost.value}`
    case 'http': return `HTTP检查 ${netUrl.value}`
    default: return ''
  }
})

const runNetTest = async () => {
  netTesting.value = true
  netResult.value = null
  try {
    let res
    switch (netTool.value) {
      case 'ping':
        if (!netHost.value) throw new Error('请输入目标主机')
        res = await testAPI.ping(netHost.value, netCount.value)
        break
      case 'port':
        if (!netHost.value) throw new Error('请输入目标主机')
        res = await testAPI.port(netHost.value, netPort.value)
        break
      case 'dns':
        if (!netHost.value) throw new Error('请输入目标域名')
        res = await testAPI.dns(netHost.value)
        break
      case 'http':
        if (!netUrl.value) throw new Error('请输入URL')
        res = await testAPI.http(netUrl.value, netMethod.value)
        break
    }
    netResult.value = { ok: res.ok !== false, output: res.output }
  } catch (error) {
    netResult.value = { ok: false, output: error.response?.data?.error || error.message }
  } finally {
    netTesting.value = false
  }
}
</script>

<style scoped>
.tip {
  margin-left: 12px;
  font-size: 12px;
  color: #909399;
}

.path-code {
  margin-left: 12px;
  background: #f4f4f5;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: 13px;
  color: #409EFF;
}

.result-card {
  margin-top: 10px;
}

.result-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.result-body {
  background: #1e1e1e;
  color: #d4d4d4;
  padding: 12px;
  border-radius: 4px;
  max-height: 480px;
  overflow: auto;
  font-size: 12px;
  line-height: 1.6;
  margin: 0;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>

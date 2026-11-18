<template>
  <div class="api-doc-page">
    <!-- 标题与基础信息 -->
    <el-card class="head-card">
      <div class="head-row">
        <div class="head-title">
          <el-icon :size="26" color="#409EFF"><Connection /></el-icon>
          <div>
            <h2>API 接口文档</h2>
            <span class="head-sub">Ops Center REST API · 版本 v1.0</span>
          </div>
        </div>
        <el-button type="primary" plain @click="router.push('/dashboard')">
          <el-icon><Back /></el-icon>
          返回控制台
        </el-button>
      </div>

      <el-descriptions :column="3" border size="small" class="base-info">
        <el-descriptions-item label="API 根地址">{{ baseUrl }}/api</el-descriptions-item>
        <el-descriptions-item label="鉴权方式">Bearer Token（请求头 Authorization）</el-descriptions-item>
        <el-descriptions-item label="数据格式">JSON（Content-Type: application/json）</el-descriptions-item>
        <el-descriptions-item label="请求超时">30 秒（前端默认）</el-descriptions-item>
        <el-descriptions-item label="跨域支持">允许所有来源（CORS *）</el-descriptions-item>
        <el-descriptions-item label="服务器端口">{{ port }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- 调用格式总览 -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <span class="sec-title">调用格式</span>
      </template>

      <h3 class="sub-title">1. 登录获取 Token</h3>
      <p class="note">登录是唯一的公开接口，返回的 <code>token</code> 需保存在客户端，后续所有请求都要带上。</p>
<pre class="code-block">{{ loginExample }}</pre>
      <p class="note">返回：</p>
<pre class="code-block">{{ loginRespExample }}</pre>

      <h3 class="sub-title">2. 携带 Token 调用业务接口</h3>
<pre class="code-block">{{ authExample }}</pre>

      <h3 class="sub-title">3. 请求体格式</h3>
      <p class="note">POST / PUT 请求体为 JSON 对象；路径参数直接替换路径中的 <code>:id</code> 等占位符。</p>
<pre class="code-block">{{ bodyExample }}</pre>

      <h3 class="sub-title">4. 统一响应与错误格式</h3>
<pre class="code-block">{{ respExample }}</pre>

      <el-table :data="statusCodes" size="small" class="status-table">
        <el-table-column prop="code" label="HTTP 状态码" width="140">
          <template #default="{ row }">
            <el-tag size="small">{{ row.code }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="meaning" label="含义" />
      </el-table>

      <el-alert type="warning" :closable="false" style="margin-top: 16px;"
        title="敏感接口说明"
        description="GET /api/servers/:id/password 会返回服务器明文密码，必须先以管理员身份通过管理员密码验证；普通用户调用会被拒绝。" />
    </el-card>

    <!-- 接口清单 -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <div class="list-head">
          <span class="sec-title">接口清单</span>
          <el-input
            v-model="keyword"
            placeholder="搜索接口路径或说明"
            clearable
            size="small"
            style="width: 260px;"
          >
            <template #prefix><el-icon><Search /></el-icon></template>
          </el-input>
        </div>
      </template>

      <el-empty v-if="filteredGroups.length === 0" description="没有匹配的接口" />

      <template v-for="group in filteredGroups" :key="group.name">
        <div class="group-block">
          <div class="group-name">
            <el-tag size="small" type="info">{{ group.count }}</el-tag>
            {{ group.name }}
          </div>

          <el-collapse accordion>
            <el-collapse-item
              v-for="apiItem in group.apis"
              :key="apiItem.path"
              :name="apiItem.method + apiItem.path"
            >
              <template #title>
                <div class="api-title">
                  <el-tag
                    size="small"
                    :type="methodType(apiItem.method)"
                    effect="dark"
                    class="method-tag"
                  >{{ apiItem.method }}</el-tag>
                  <code class="api-path">{{ apiItem.path }}</code>
                  <span class="api-desc">{{ apiItem.desc }}</span>
                </div>
              </template>

              <div class="api-detail">
                <p class="note">{{ apiItem.desc }}</p>

                <template v-if="apiItem.params && apiItem.params.length">
                  <h4 class="mini-title">路径参数</h4>
                  <el-table :data="apiItem.params" size="small">
                    <el-table-column prop="name" label="参数" width="140" />
                    <el-table-column prop="desc" label="说明" />
                  </el-table>
                </template>

                <template v-if="apiItem.body">
                  <h4 class="mini-title">请求体示例</h4>
<pre class="code-block">{{ apiItem.body }}</pre>
                </template>

                <template v-if="apiItem.resp">
                  <h4 class="mini-title">响应示例</h4>
<pre class="code-block">{{ apiItem.resp }}</pre>
                </template>

                <h4 class="mini-title">curl 调用</h4>
<pre class="code-block">{{ curlOf(apiItem) }}</pre>
              </div>
            </el-collapse-item>
          </el-collapse>
        </div>
      </template>
    </el-card>

    <!-- 前端封装 -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <span class="sec-title">前端调用封装</span>
      </template>
      <p class="note">前端统一使用 axios 实例（<code>src/api/index.js</code>），<code>baseURL</code> 为 <code>/api</code>，
        请求拦截器自动附加 Token，响应拦截器直接返回 <code>response.data</code>。业务代码只需调用封装后的方法。</p>
<pre class="code-block">{{ frontendExample }}</pre>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { Connection, Back, Search } from '@element-plus/icons-vue'

const router = useRouter()

const origin = window.location.origin
const baseUrl = origin
const port = new URLSearchParams(window.location.search).get('port') || '7779'

const loginExample = `curl -X POST ${origin}/api/auth/login \\
  -H "Content-Type: application/json" \\
  -d '{"username":"admin","password":"your-password"}'`

const loginRespExample = `{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": {
    "id": 1,
    "username": "admin",
    "role": "admin"
  }
}`

const authExample = `# 携带 Token 请求受保护接口
curl -X GET ${origin}/api/servers \\
  -H "Authorization: Bearer <你的token>"`

const bodyExample = `POST /api/servers

{
  "name": "web-01",
  "ip": "10.10.10.220",
  "ssh_port": 22,
  "auth_type": "password",
  "username": "root",
  "password": "******",
  "group_name": "web",
  "tags": "prod"
}`

const respExample = `# 成功：直接返回数据
{
  "id": 12,
  "name": "web-01",
  "ip": "10.10.10.220",
  "status": "online",
  "cpu_usage": 23.5,
  "memory_usage": 41.2,
  "disk_usage": 55.0,
  "group_name": "web",
  "created_at": "2026-10-22 09:00:00"
}

# 失败：统一错误结构
{ "error": "参数错误: name 不能为空" }`

const statusCodes = [
  { code: '200', meaning: '成功' },
  { code: '400', meaning: '请求参数错误或请求体格式不对' },
  { code: '401', meaning: '未登录或 Token 失效（前端会跳回登录页）' },
  { code: '403', meaning: '已登录但无权限（如普通用户查明文密码）' },
  { code: '404', meaning: '资源不存在，或 /api 开头的路径未注册' },
  { code: '500', meaning: '服务端异常' }
]

const apiGroups = [
  {
    name: '认证',
    apis: [
      {
        method: 'POST', path: '/api/auth/login', desc: '用户登录，返回 Token（唯一公开接口）',
        body: `{ "username": "admin", "password": "your-password" }`,
        resp: `{ "token": "eyJ...", "user": { "id": 1, "username": "admin", "role": "admin" } }`
      },
      {
        method: 'GET', path: '/api/auth/me', desc: '获取当前登录用户信息',
        resp: `{ "id": 1, "username": "admin", "role": "admin" }`
      }
    ]
  },
  {
    name: '服务器管理',
    apis: [
      { method: 'GET', path: '/api/servers', desc: '获取所有服务器列表' },
      {
        method: 'POST', path: '/api/servers', desc: '新增一台服务器',
        body: `{ "name": "web-01", "ip": "10.10.10.220", "auth_type": "password", "username": "root", "password": "***" }`,
        resp: `{ "id": 13, "name": "web-01", "ip": "10.10.10.220" }`
      },
      { method: 'POST', path: '/api/servers/batch', desc: '批量新增服务器', body: `{ "servers": [ { "name": "...", "ip": "..." }, { "name": "...", "ip": "..." } ] }` },
      {
        method: 'GET', path: '/api/servers/:id', desc: '获取单台服务器信息',
        params: [{ name: 'id', desc: '服务器 ID' }]
      },
      { method: 'PUT', path: '/api/servers/:id', desc: '修改服务器信息（部分字段也可）', params: [{ name: 'id', desc: '服务器 ID' }], body: `{ "group_name": "web" }` },
      { method: 'DELETE', path: '/api/servers/:id', desc: '删除服务器', params: [{ name: 'id', desc: '服务器 ID' }] },
      {
        method: 'GET', path: '/api/servers/:id/password', desc: '获取服务器明文密码（管理员验证后）',
        params: [{ name: 'id', desc: '服务器 ID' }],
        resp: `{ "password": "plain-text-password" }`
      },
      { method: 'POST', path: '/api/servers/:id/test', desc: '测试服务器 SSH 连通性', params: [{ name: 'id', desc: '服务器 ID' }] },
      { method: 'GET', path: '/api/servers/:id/detail', desc: '获取服务器硬件/系统详情', params: [{ name: 'id', desc: '服务器 ID' }] },
      { method: 'POST', path: '/api/servers/:id/refresh', desc: '刷新单台服务器的资源采集数据', params: [{ name: 'id', desc: '服务器 ID' }] },
      { method: 'POST', path: '/api/servers/refresh-all', desc: '刷新所有服务器' },
      { method: 'POST', path: '/api/servers/collect-missing', desc: '补采缺失详情的服务器' },
      { method: 'GET', path: '/api/servers/stats/resource', desc: '资源总量统计（CPU/内存/磁盘/网络）' },
      { method: 'GET', path: '/api/servers/stats/rankings', desc: 'CPU/内存/磁盘/网络利用率排行' },
      { method: 'GET', path: '/api/servers/:id/virtual', desc: '获取服务器的虚拟 IP（EasyTier）', params: [{ name: 'id', desc: '服务器 ID' }] }
    ]
  },
  {
    name: '分组管理',
    apis: [
      { method: 'GET', path: '/api/groups', desc: '获取所有分组' },
      { method: 'POST', path: '/api/groups', desc: '新增分组', body: `{ "name": "web", "description": "Web 服务组" }` },
      { method: 'PUT', path: '/api/groups/:id', desc: '修改分组', params: [{ name: 'id', desc: '分组 ID' }] },
      { method: 'DELETE', path: '/api/groups/:id', desc: '删除分组', params: [{ name: 'id', desc: '分组 ID' }] }
    ]
  },
  {
    name: '服务管理',
    apis: [
      { method: 'GET', path: '/api/servers/:id/services', desc: '获取服务器下的服务列表', params: [{ name: 'id', desc: '服务器 ID' }] },
      {
        method: 'POST', path: '/api/servers/:id/services', desc: '新增服务', params: [{ name: 'id', desc: '服务器 ID' }],
        body: `{ "name": "Nginx", "port": 80, "access_method": "web", "username": "", "password": "" }`
      },
      { method: 'PUT', path: '/api/servers/:id/services/:serviceId', desc: '修改服务', params: [{ name: 'id', desc: '服务器 ID' }, { name: 'serviceId', desc: '服务 ID' }] },
      { method: 'DELETE', path: '/api/servers/:id/services/:serviceId', desc: '删除服务', params: [{ name: 'id', desc: '服务器 ID' }, { name: 'serviceId', desc: '服务 ID' }] },
      { method: 'POST', path: '/api/servers/:id/services/:serviceId/test', desc: '测试服务连通性', params: [{ name: 'id', desc: '服务器 ID' }, { name: 'serviceId', desc: '服务 ID' }] },
      { method: 'GET', path: '/api/servers/:id/services/:serviceId/detail', desc: '获取服务详情', params: [{ name: 'id', desc: '服务器 ID' }, { name: 'serviceId', desc: '服务 ID' }] }
    ]
  },
  {
    name: '系统组件',
    apis: [
      { method: 'GET', path: '/api/system/components', desc: '列出系统组件及运行状态（guacd、guacamole、easytier 等）' },
      { method: 'POST', path: '/api/system/components/:name/restart', desc: '重启组件', params: [{ name: 'name', desc: '组件名，如 guacd / guacamole' }] },
      { method: 'POST', path: '/api/system/components/:name/stop', desc: '停止组件', params: [{ name: 'name', desc: '组件名' }] },
      { method: 'GET', path: '/api/system/components/:name/logs', desc: '查看组件日志', params: [{ name: 'name', desc: '组件名' }] }
    ]
  },
  {
    name: '系统测试（网络工具）',
    apis: [
      { method: 'POST', path: '/api/system/test/ping', desc: 'Ping 测试', body: `{ "target": "10.10.10.220", "count": 4 }` },
      { method: 'POST', path: '/api/system/test/port', desc: '端口探测', body: `{ "host": "10.10.10.220", "ports": [22, 3389, 80] }` },
      { method: 'POST', path: '/api/system/test/dns', desc: 'DNS 解析测试', body: `{ "domain": "example.com" }` },
      { method: 'POST', path: '/api/system/test/http', desc: 'HTTP 请求测试', body: `{ "url": "http://10.10.10.220" }` }
    ]
  },
  {
    name: '端口配置',
    apis: [
      { method: 'GET', path: '/api/settings/ports', desc: '获取端口列表（服务录入时的候选端口）' },
      { method: 'POST', path: '/api/settings/ports', desc: '新增端口配置', body: `{ "name": "Nginx", "port": 80, "access_method": "web" }` },
      { method: 'PUT', path: '/api/settings/ports/:id', desc: '修改端口配置', params: [{ name: 'id', desc: '配置 ID' }] },
      { method: 'DELETE', path: '/api/settings/ports/:id', desc: '删除端口配置', params: [{ name: 'id', desc: '配置 ID' }] }
    ]
  },
  {
    name: '网页访问工具',
    apis: [
      { method: 'GET', path: '/api/tools/web-services', desc: '获取可网页访问的服务列表' },
      {
        method: 'POST', path: '/api/tools/web/session', desc: '创建网页会话（返回会话令牌 tk，用于 iframe 代理）',
        body: `{ "service_id": 1, "server_id": 13 }`,
        resp: `{ "tk": "a1b2c3...", "target": "http://10.10.10.220" }`
      },
      { method: 'GET', path: '/api/tools/web/sessions', desc: '列出未关闭的网页会话' },
      {
        method: 'GET/POST', path: '/api/tools/web', desc: '网页代理入口（用会话令牌访问目标站点）',
        body: `?session=<tk>`
      },
      {
        method: 'GET', path: '/api/tools/web/lock', desc: '查询会话的单控制器锁状态',
        resp: `{ "locked": true, "owner": "sess-123", "ip": "10.172.17.97" }`
      },
      {
        method: 'POST', path: '/api/tools/web/lock', desc: '抢锁 / 释放锁（单控制器模式：仅一人控制，其余只读）',
        body: `{ "session": "<tk>", "release": false }`
      },
      { method: 'GET', path: '/api/tools/webproxy-api', desc: 'ExtJS 接口代理（转发目标站点 /api2/json 请求）' },
      { method: 'ANY', path: '/px/:tk/*path', desc: '全路径反向代理：/px/{会话令牌}/原始路径 → 目标站点原始路径' }
    ]
  },
  {
    name: 'SSH 终端',
    apis: [
      {
        method: 'POST', path: '/api/ssh/session', desc: '创建 SSH 会话（返回 WebSocket 令牌）',
        body: `{ "host": "10.10.10.220", "port": 22, "username": "root", "password": "***" }`,
        resp: `{ "token": "tk-xxxx", "ws_url": "/ws/ssh/tk-xxxx" }`
      },
      {
        method: 'POST', path: '/api/ssh/shadow', desc: 'SSH 影子会话（只读关注他人已建立的会话，不影响对方操作）',
        body: `{ "target_token": "tk-xxxx" }`
      },
      { method: 'WebSocket', path: '/ws/ssh/:tk', desc: 'SSH 终端双向数据通道' }
    ]
  },
  {
    name: '远程桌面（RDP）',
    apis: [
      {
        method: 'POST', path: '/api/tools/rdp/session', desc: '创建 RDP 会话（Guacamole）',
        body: `{ "host": "10.10.10.220", "port": 3389, "username": "Administrator", "password": "***" }`,
        resp: `{ "guac_token": "uuid", "ws_url": "/guac/..." }`
      },
      { method: 'ANY', path: '/guac/*path', desc: 'Guacamole 反向代理（含 WebSocket 隧道）' },
      { method: '静态页', path: '/rdpguac.html', desc: 'RDP 客户端页面' }
    ]
  },
  {
    name: '虚拟网络（EasyTier）',
    apis: [
      { method: 'GET', path: '/api/vnet/networks', desc: '获取已定义的虚拟网络列表' },
      {
        method: 'POST', path: '/api/vnet/networks', desc: '新建虚拟网络定义',
        body: `{ "name": "ltnet", "network_name": "lt-net", "peer_ip": "1.2.3.4", "port": 22222, "password": "***", "cidr": "10.110.0.0/24" }`
      },
      { method: 'PUT', path: '/api/vnet/networks/:id', desc: '修改虚拟网络定义', params: [{ name: 'id', desc: '网络 ID' }] },
      { method: 'DELETE', path: '/api/vnet/networks/:id', desc: '删除虚拟网络定义', params: [{ name: 'id', desc: '网络 ID' }] },
      {
        method: 'GET', path: '/api/vnet/peers', desc: '获取网络中在线主机（含 EasyTier 虚拟 IP、MAC 等标识）',
        params: [{ name: 'network_id', desc: '网络 ID（查询参数）' }],
        resp: `{ "peers": [ { "virtual_ip": "10.110.0.15", "id": "abc123", "mac": "52:54:00:aa:bb:cc" } ] }`
      },
      { method: 'POST', path: '/api/vnet/sync', desc: '立即执行一次主机同步（刷新虚拟 IP 与服务绑定）' },
      {
        method: 'POST', path: '/api/vnet/join', desc: '加入指定虚拟网络（未加入则自动创建实例并加入）',
        body: `{ "network_id": 1 }`
      },
      { method: 'POST', path: '/api/vnet/instance/stop', desc: '停止虚拟网络实例', body: `{ "network_id": 1 }` },
      { method: 'POST', path: '/api/vnet/instance/delete', desc: '删除虚拟网络实例', body: `{ "network_id": 1 }` },
      { method: 'GET', path: '/api/vnet/last-sync', desc: '查看最近一次同步结果' },
      {
        method: 'POST', path: '/api/vnet/bind', desc: '把虚拟 IP 绑定到服务器（原分组为主，虚拟分组为辅）',
        body: `{ "server_id": 13, "network_id": 1, "virtual_ip": "10.110.0.15" }`
      },
      { method: 'POST', path: '/api/vnet/unbind', desc: '解绑虚拟 IP', body: `{ "server_id": 13, "network_id": 1 }` }
    ]
  },
  {
    name: '救援控制台',
    apis: [
      { method: 'GET', path: '/api/rescue/status', desc: '查看救援控制台状态' },
      { method: 'POST', path: '/api/rescue/backup-db', desc: '备份数据库' },
      { method: 'GET', path: '/api/rescue/log', desc: '查看救援日志' },
      { method: 'POST', path: '/api/rescue/restart', desc: '重启后端服务' },
      { method: 'POST', path: '/api/rescue/fix-assets', desc: '修复前端静态资源（白屏应急）' },
      { method: '静态页', path: '/rescue.html', desc: '不依赖前端构建产物的救援页面，主界面白屏时也可用' }
    ]
  },
  {
    name: '空闲 IP / 端口扫描',
    apis: [
      { method: 'GET', path: '/api/free/ips', desc: '扫描已有网段中在线但未登记为服务器的 IP（90 秒缓存，加 ?force=1 强制重扫）' },
      { method: 'POST', path: '/api/free/ips/add', desc: '把 IP 批量登记为受管服务器（幂等，已存在自动跳过）', body: `{ "ips": ["10.10.10.50", "10.10.10.218"], "group": "web" }` },
      { method: 'GET', path: '/api/servers/:id/free-ports', desc: 'SSH 到目标服务器扫描 TCP 监听端口（需该服务器已配置 SSH 凭据）', params: [{ name: 'id', desc: '服务器 ID' }] },
      { method: 'POST', path: '/api/servers/:id/free-ports/add', desc: '把扫描到的端口批量登记为服务（幂等）', params: [{ name: 'id', desc: '服务器 ID' }], body: `{ "ports": [22, 443] }` }
    ]
  },
  {
    name: 'MCP 管理（管理员）',
    apis: [
      { method: 'GET', path: '/api/mcp/status', desc: 'MCP 服务运行状态（运行中/PID/监听地址/级别开关/令牌数/审计条数）' },
      { method: 'GET', path: '/api/mcp/config', desc: '查看 MCP 配置：级别开关 + 令牌列表（key 明文仅管理员可见）' },
      { method: 'POST', path: '/api/mcp/levels', desc: '设置危险等级开关 L0-L3（热加载立即生效，L3 删除/高危默认禁用）', body: `{ "L0": true, "L1": true, "L2": true, "L3": false }` },
      { method: 'POST', path: '/api/mcp/actions', desc: '写入类总开关（false 时 L2 及以上全部拒绝）', body: `{ "enabled": false }` },
      { method: 'POST', path: '/api/mcp/tokens', desc: '新建访问令牌（key 仅返回一次；max_level 限定该令牌可调用的最高危险等级）', body: `{ "remark": "监控智能体", "max_level": "L1" }` },
      { method: 'POST', path: '/api/mcp/tokens/revoke', desc: '吊销令牌（至少保留一个）', body: `{ "key": "<完整 key>" }` },
      { method: 'GET', path: '/api/mcp/audit', desc: '审计日志最近 N 条（?lines=100，含被拒绝的调用记录）' }
    ]
  }
]

const frontendExample = `import { serverAPI, toolsAPI } from '../../api'

// 1. 登录（token 存入 localStorage，拦截器自动附加）
await authAPI.login({ username, password })

// 2. 读取服务器列表
const servers = await serverAPI.getList()

// 3. 新增服务器
await serverAPI.create({ name: 'web-01', ip: '10.10.10.220' })

// 4. 创建 RDP 会话
const session = await toolsAPI.createRdpSession({
  host: '10.10.10.220', port: 3389,
  username: 'Administrator', password: '***'
})`

const keyword = ref('')

const filteredGroups = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return apiGroups
  return apiGroups
    .map(g => ({
      ...g,
      apis: g.apis.filter(a =>
        a.path.toLowerCase().includes(kw) || a.desc.toLowerCase().includes(kw))
    }))
    .filter(g => g.apis.length > 0)
    .map(g => ({ ...g, count: g.apis.length }))
})

const methodType = (m) => {
  if (m === 'GET') return 'success'
  if (m === 'POST') return 'primary'
  if (m === 'PUT') return 'warning'
  if (m === 'DELETE') return 'danger'
  return 'info'
}

const curlOf = (apiItem) => {
  const path = apiItem.path.replace(/:[a-zA-Z_]+/g, '<参数>')
  const method = apiItem.method
  let cmd
  if (method === 'WebSocket') {
    cmd = `wscat -c wss://${origin.replace(/^http/, 'ws')}${path}`
  } else if (method === 'ANY' || method === '静态页') {
    cmd = `curl -X GET ${origin}${path}`
  } else {
    cmd = `curl -X ${method} ${origin}${path}`
  }
  cmd += ` \\\n  -H "Authorization: Bearer <token>"`
  if (apiItem.method !== 'GET' && apiItem.body) {
    cmd += ` \\\n  -H "Content-Type: application/json" \\\n  -d '${apiItem.body}'`
  }
  return cmd
}
</script>

<style scoped>
.api-doc-page {
  padding: 20px;
}

.head-card {
  border-radius: 8px;
}

.head-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 18px;
}

.head-title {
  display: flex;
  align-items: center;
  gap: 14px;
}

.head-title h2 {
  margin: 0;
  font-size: 20px;
  color: #303133;
}

.head-sub {
  font-size: 13px;
  color: #909399;
}

.base-info {
  margin-bottom: 4px;
}

.sec-title {
  font-weight: 600;
  font-size: 15px;
}

.list-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.sub-title {
  margin: 22px 0 8px;
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.note {
  margin: 6px 0 10px;
  color: #606266;
  line-height: 1.7;
  font-size: 13.5px;
}

.note code,
p code {
  background: #f0f2f5;
  padding: 1px 5px;
  border-radius: 3px;
  font-size: 12.5px;
  color: #c0392b;
}

.code-block {
  background: #1e1e2e;
  color: #d4d4d4;
  padding: 14px 16px;
  border-radius: 6px;
  font-family: 'Cascadia Code', Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.7;
  overflow-x: auto;
  white-space: pre;
  margin: 8px 0 14px;
}

.status-table {
  margin-top: 12px;
}

.group-block {
  margin-bottom: 18px;
}

.group-name {
  font-size: 14.5px;
  font-weight: 600;
  color: #303133;
  padding: 10px 14px;
  background: #f5f7fa;
  border-radius: 6px 6px 0 0;
  display: flex;
  align-items: center;
  gap: 10px;
}

.api-title {
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
}

.method-tag {
  min-width: 68px;
  text-align: center;
  flex-shrink: 0;
}

.api-path {
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  color: #303133;
  font-weight: 500;
}

.api-desc {
  font-size: 13px;
  color: #909399;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.api-detail .note {
  color: #606266;
}

.mini-title {
  margin: 14px 0 6px;
  font-size: 13px;
  font-weight: 600;
  color: #303133;
}

.api-detail .code-block {
  margin: 6px 0 10px;
}
</style>

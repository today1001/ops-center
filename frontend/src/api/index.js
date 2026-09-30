import axios from 'axios'

const api = axios.create({
  baseURL: '/api',
  timeout: 30000
})

// 请求拦截器
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器
api.interceptors.response.use(
  (response) => response.data,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// 认证相关API
export const authAPI = {
  login: (data) => api.post('/auth/login', data),
  getCurrentUser: () => api.get('/auth/me')
}

// 服务器相关API
export const serverAPI = {
  getList: () => api.get('/servers'),
  getById: (id) => api.get(`/servers/${id}`),
  create: (data) => api.post('/servers', data),
  createBatch: (servers) => api.post('/servers/batch', { servers }),
  update: (id, data) => api.put(`/servers/${id}`, data),
  delete: (id) => api.delete(`/servers/${id}`),
  testConnection: (id) => api.post(`/servers/${id}/test`),
  getDetail: (id) => api.get(`/servers/${id}/detail`),
  refresh: (id) => api.post(`/servers/${id}/refresh`),
  refreshAll: () => api.post('/servers/refresh-all'),
  collectMissing: () => api.post('/servers/collect-missing'),
  getResourceStats: () => api.get('/servers/stats/resource'),
  getRankings: () => api.get('/servers/stats/rankings'),
  // 服务管理
  getServices: (serverId) => api.get(`/servers/${serverId}/services`),
  createService: (serverId, data) => api.post(`/servers/${serverId}/services`, data),
  updateService: (serverId, serviceId, data) => api.put(`/servers/${serverId}/services/${serviceId}`, data),
  deleteService: (serverId, serviceId) => api.delete(`/servers/${serverId}/services/${serviceId}`),
  testService: (serverId, serviceId) => api.post(`/servers/${serverId}/services/${serviceId}/test`),
  getServiceDetail: (serverId, serviceId) => api.get(`/servers/${serverId}/services/${serviceId}/detail`)
}

// 虚拟网络API
export const vnetAPI = {
  getNetworks: () => api.get('/vnet/networks'),
  addNetwork: (data) => api.post('/vnet/networks', data),
  updateNetwork: (id, data) => api.put(`/vnet/networks/${id}`, data),
  deleteNetwork: (id) => api.delete(`/vnet/networks/${id}`),
  getPeers: () => api.get('/vnet/peers'),
  sync: () => api.post('/vnet/sync'),
  getLastSync: () => api.get('/vnet/last-sync'),
  join: (data) => api.post('/vnet/join', data),
  stopInstance: (data) => api.post('/vnet/instance/stop', data),
  deleteInstance: (data) => api.post('/vnet/instance/delete', data),
  bind: (data) => api.post('/vnet/bind', data),
  unbind: (data) => api.post('/vnet/unbind', data),
  getServerVirtual: (id) => api.get(`/servers/${id}/virtual`)
}

// 分组管理API
export const groupAPI = {
  getList: () => api.get('/groups'),
  create: (data) => api.post('/groups', data),
  update: (id, data) => api.put(`/groups/${id}`, data),
  delete: (id) => api.delete(`/groups/${id}`)
}

// 系统组件相关API
export const systemAPI = {
  getComponents: () => api.get('/system/components'),
  restart: (name) => api.post(`/system/components/${name}/restart`),
  stop: (name) => api.post(`/system/components/${name}/stop`),
  getLogs: (name, lines = 100) => api.get(`/system/components/${name}/logs`, { params: { lines } })
}

// 系统测试（网络工具）API
export const testAPI = {
  ping: (host, count) => api.post('/system/test/ping', { host, count }),
  port: (host, port) => api.post('/system/test/port', { host, port }),
  dns: (hostname) => api.post('/system/test/dns', { hostname }),
  http: (url, method) => api.post('/system/test/http', { url, method })
}

// 系统设置API
export const settingsAPI = {
  getPortProfiles: () => api.get('/settings/ports'),
  createPortProfile: (data) => api.post('/settings/ports', data),
  updatePortProfile: (id, data) => api.put(`/settings/ports/${id}`, data),
  deletePortProfile: (id) => api.delete(`/settings/ports/${id}`)
}

// 工具模块API
export const toolsAPI = {
  getWebServices: () => api.get('/tools/web-services'),
  createWebSession: (data) => api.post('/tools/web/session', data),
  createSshSession: (data) => api.post('/ssh/session', data),
  createSshShadow: (data) => api.post('/ssh/shadow', data),
  getWebSessions: () => api.get('/tools/web/sessions'),
  createRdpSession: (data) => api.post('/tools/rdp/session', data)
}

export default api

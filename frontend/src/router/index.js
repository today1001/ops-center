import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue')
  },
  {
    path: '/',
    name: 'Layout',
    component: () => import('../components/Layout.vue'),
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('../views/Dashboard.vue'),
        meta: { title: '仪表盘' }
      },
      {
        path: 'servers',
        name: 'ServerList',
        component: () => import('../views/servers/ServerList.vue'),
        meta: { title: '服务器管理' }
      },
      {
        path: 'servers/add',
        name: 'ServerAdd',
        component: () => import('../views/servers/ServerForm.vue'),
        meta: { title: '添加服务器' }
      },
      {
        path: 'servers/:id',
        name: 'ServerDetail',
        component: () => import('../views/servers/ServerDetail.vue'),
        meta: { title: '服务器详情' }
      },
      {
        path: 'servers/:id/edit',
        name: 'ServerEdit',
        component: () => import('../views/servers/ServerForm.vue'),
        meta: { title: '编辑服务器' }
      },
      {
        path: 'groups',
        name: 'GroupManager',
        component: () => import('../views/GroupManager.vue'),
        meta: { title: '分组管理' }
      },
      {
        path: 'components',
        name: 'SystemComponents',
        component: () => import('../views/SystemComponents.vue'),
        meta: { title: '系统组件' }
      },
      {
        path: 'services',
        name: 'ServiceManager',
        component: () => import('../views/ServiceManager.vue'),
        meta: { title: '服务管理' }
      },
      {
        path: 'system-test',
        name: 'SystemTest',
        component: () => import('../views/SystemTest.vue'),
        meta: { title: '系统测试' }
      },
      {
        path: 'tools/web',
        name: 'WebAccessTool',
        component: () => import('../views/WebAccessTool.vue'),
        meta: { title: '网页访问' }
      },
      {
        path: 'tools/ssh',
        name: 'SshAccessTool',
        component: () => import('../views/SshAccessTool.vue'),
        meta: { title: 'SSH终端' }
      },
      {
        path: 'tools/rdp',
        name: 'RdpAccessTool',
        component: () => import('../views/RdpAccessTool.vue'),
        meta: { title: '远程桌面' }
      },
      {
        path: 'vnet',
        name: 'VirtualNetwork',
        component: () => import('../views/VirtualNetwork.vue'),
        meta: { title: '虚拟网络' }
      },
      {
        path: 'hypervisors/esxi',
        name: 'EsxiManager',
        component: () => import('../views/HypervisorManager.vue'),
        meta: { title: 'ESXi 管理', hvType: 'esxi' }
      },
      {
        path: 'hypervisors/pve',
        name: 'PveManager',
        component: () => import('../views/HypervisorManager.vue'),
        meta: { title: 'PVE 管理', hvType: 'pve' }
      },
      {
        path: 'free-ip',
        name: 'FreeIPManager',
        component: () => import('../views/FreeIPManager.vue'),
        meta: { title: '空闲 IP' }
      },
      {
        path: 'docs',
        name: 'ApiDocumentation',
        component: () => import('../views/ApiDocumentation.vue'),
        meta: { title: 'API文档' }
      },
      {
        path: 'settings',
        name: 'SystemSettings',
        component: () => import('../views/SystemSettings.vue'),
        meta: { title: '系统设置' }
      },
      {
        path: 'users',
        name: 'UserManager',
        component: () => import('../views/UserManager.vue'),
        meta: { title: '用户管理', adminOnly: true }
      },
      {
        path: 'audit',
        name: 'AuditLogs',
        component: () => import('../views/AuditLogs.vue'),
        meta: { title: '访问日志', adminOnly: true }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 路由守卫
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')

  if (to.path !== '/login' && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/')
  } else {
    // 管理员专属页面：非 admin 直接回仪表盘
    if (to.meta?.adminOnly) {
      const user = JSON.parse(localStorage.getItem('user') || '{}')
      if (user.role !== 'admin') {
        next('/dashboard')
        return
      }
    }
    next()
  }
})

export default router

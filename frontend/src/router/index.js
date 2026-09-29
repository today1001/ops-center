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
        component: () => import('../views/Dashboard.vue')
      },
      {
        path: 'servers',
        name: 'ServerList',
        component: () => import('../views/servers/ServerList.vue')
      },
      {
        path: 'servers/add',
        name: 'ServerAdd',
        component: () => import('../views/servers/ServerForm.vue')
      },
      {
        path: 'servers/:id',
        name: 'ServerDetail',
        component: () => import('../views/servers/ServerDetail.vue')
      },
      {
        path: 'servers/:id/edit',
        name: 'ServerEdit',
        component: () => import('../views/servers/ServerForm.vue')
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
        path: 'settings',
        name: 'SystemSettings',
        component: () => import('../views/SystemSettings.vue'),
        meta: { title: '系统设置' }
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
    next()
  }
})

export default router

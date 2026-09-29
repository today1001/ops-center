<template>
  <el-container class="layout-container" :class="{ dark: actualTheme === 'dark' }">
    <el-aside width="200px" class="aside">
      <div class="logo" @click="router.push('/dashboard')" style="cursor: pointer;">
        <h2>Ops-Center</h2>
      </div>
      <el-menu
        :default-active="route.path"
        router
        class="menu"
      >
        <el-menu-item index="/dashboard">
          <el-icon><Monitor /></el-icon>
          <span>{{ t('nav.dashboard') }}</span>
        </el-menu-item>
        <el-menu-item index="/servers">
          <el-icon><Connection /></el-icon>
          <span>{{ t('nav.servers') }}</span>
        </el-menu-item>
        <el-menu-item index="/groups">
          <el-icon><FolderOpened /></el-icon>
          <span>{{ t('nav.groups') }}</span>
        </el-menu-item>
        <el-menu-item index="/services">
          <el-icon><Operation /></el-icon>
          <span>{{ t('nav.services') }}</span>
        </el-menu-item>
        <el-menu-item index="/components">
          <el-icon><Setting /></el-icon>
          <span>{{ t('nav.components') }}</span>
        </el-menu-item>
        <el-menu-item index="/system-test">
          <el-icon><Tools /></el-icon>
          <span>{{ t('nav.systemTest') }}</span>
        </el-menu-item>
        <el-sub-menu index="/tools">
          <template #title>
            <el-icon><MagicStick /></el-icon>
            <span>{{ t('nav.tools') }}</span>
          </template>
          <el-menu-item index="/tools/web">
            <el-icon><Monitor /></el-icon>
            <span>{{ t('nav.webAccess') }}</span>
          </el-menu-item>
          <el-menu-item index="/tools/ssh">
            <el-icon><Monitor /></el-icon>
            <span>{{ t('nav.sshAccess') }}</span>
          </el-menu-item>
          <el-menu-item index="/tools/rdp">
            <el-icon><Monitor /></el-icon>
            <span>{{ t('nav.rdpAccess') }}</span>
          </el-menu-item>
          <el-menu-item index="/vnet">
            <el-icon><Monitor /></el-icon>
            <span>{{ t('nav.vnet') }}</span>
          </el-menu-item>
        </el-sub-menu>
        <el-menu-item index="/settings">
          <el-icon><Setting /></el-icon>
          <span>{{ t('nav.settings') }}</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <div class="header-left">
          <el-breadcrumb separator="/">
            <el-breadcrumb-item :to="{ path: '/' }">{{ t('common.home') || '首页' }}</el-breadcrumb-item>
            <el-breadcrumb-item v-if="route.name !== 'Dashboard'">{{ route.meta.title }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <!-- 主题切换 -->
          <el-dropdown @command="handleThemeCommand" trigger="click">
            <div class="theme-switcher" @click.stop="toggleTheme" title="点击切换主题">
              <el-icon v-if="actualTheme === 'light'"><Sunny /></el-icon>
              <el-icon v-else-if="actualTheme === 'dark'"><Moon /></el-icon>
              <el-icon v-else><Monitor /></el-icon>
              <span class="theme-label">
                {{ themeStore.theme === 'light' ? t('settings.themeLight') : 
                   themeStore.theme === 'dark' ? t('settings.themeDark') : 
                   t('settings.themeSystem') }}
              </span>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="theme-light" :disabled="themeStore.theme === 'light'">
                  <el-icon><Sunny /></el-icon>
                  <span>{{ t('settings.themeLight') }}</span>
                </el-dropdown-item>
                <el-dropdown-item command="theme-dark" :disabled="themeStore.theme === 'dark'">
                  <el-icon><Moon /></el-icon>
                  <span>{{ t('settings.themeDark') }}</span>
                </el-dropdown-item>
                <el-dropdown-item command="theme-system" :disabled="themeStore.theme === 'system'">
                  <el-icon><Monitor /></el-icon>
                  <span>{{ t('settings.themeSystem') }}</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>

          <!-- 语言切换 -->
          <el-dropdown @command="handleLocaleCommand" trigger="click">
            <div class="locale-switcher" @click.stop="toggleLocale" title="点击切换语言">
              <el-icon><Position /></el-icon>
              <span>{{ localeStore.locale === 'zh-CN' ? '中文' : 'English' }}</span>
            </div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item v-for="loc in availableLocales" :key="loc.code" :command="'locale-' + loc.code" :disabled="localeStore.locale === loc.code">
                  <span>{{ loc.name }}</span>
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>

          <el-dropdown @command="handleCommand">
            <span class="user-info">
              <el-icon><User /></el-icon>
              {{ username }}
              <el-icon class="el-icon--right"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="logout">{{ t('layout.logout') }}</el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { 
  Monitor, Connection, Operation, Setting, Tools, 
  MagicStick, User, ArrowDown, FolderOpened, 
  Sunny, Moon, Position 
} from '@element-plus/icons-vue'
import { themeStore, actualTheme, toggleTheme, setTheme } from '../theme.js'
import { localeStore, availableLocales, toggleLocale, setLocale } from '../locale.js'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const username = ref('')

onMounted(() => {
  const user = JSON.parse(localStorage.getItem('user') || '{}')
  username.value = user.username || 'Admin'
})

const handleCommand = (command) => {
  if (command === 'logout') {
    localStorage.removeItem('token')
    localStorage.removeItem('user')
    router.push('/login')
  }
}

const handleThemeCommand = (command) => {
  if (command === 'theme-light') setTheme('light')
  else if (command === 'theme-dark') setTheme('dark')
  else if (command === 'theme-system') setTheme('system')
}

const handleLocaleCommand = (command) => {
  if (command === 'locale-zh-CN') setLocale('zh-CN')
  else if (command === 'locale-en-US') setLocale('en-US')
}
</script>

<style scoped>
.layout-container {
  height: 100vh;
  transition: background-color 0.3s ease, color 0.3s ease;
}

.aside {
  background-color: var(--sidebar-bg, #304156);
  color: var(--sidebar-text, #fff);
  transition: background-color 0.3s ease, color 0.3s ease;
}

.logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #263445;
  transition: background-color 0.3s ease;
}

.logo h2 {
  color: #fff;
  font-size: 18px;
  margin: 0;
}

.menu {
  border-right: none;
  background-color: var(--sidebar-bg, #304156);
  transition: background-color 0.3s ease;
}

.header {
  background-color: var(--header-bg, #fff);
  box-shadow: 0 1px 4px rgba(0, 21, 41, 0.08);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  height: 50px;
  transition: background-color 0.3s ease, box-shadow 0.3s ease;
}

.header-left {
  flex: 1;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.theme-switcher,
.locale-switcher {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 6px;
  cursor: pointer;
  color: var(--text-primary, #303133);
  transition: background-color 0.2s;
}

.theme-switcher:hover,
.locale-switcher:hover {
  background-color: var(--bg-hover, #f5f7fa);
}

.theme-label {
  font-size: 13px;
  font-weight: 500;
}

.user-info {
  display: flex;
  align-items: center;
  cursor: pointer;
  color: var(--text-regular, #606266);
}

.user-info .el-icon {
  font-size: 14px;
}

.main {
  background-color: var(--bg-secondary, #f0f2f5);
  padding: 20px;
  transition: background-color 0.3s ease;
}

.aside {
  transition: background-color 0.3s ease, color 0.3s ease;
}

.logo {
  transition: background-color 0.3s ease;
}

.menu {
  transition: background-color 0.3s ease;
}

.header {
  transition: background-color 0.3s ease, box-shadow 0.3s ease;
}

.main {
  transition: background-color 0.3s ease;
}

/* 暗色模式下的侧边栏 */
.layout-container.dark .aside {
  background-color: #141414;
}

.layout-container.dark .logo {
  background-color: #1a1a1a;
}

.layout-container.dark .menu {
  background-color: #141414;
}

.layout-container.dark .header {
  background-color: #1e1e1e;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.3);
}

.layout-container.dark .main {
  background-color: #1e1e1e;
}

.layout-container.dark .theme-switcher:hover,
.layout-container.dark .locale-switcher:hover {
  background-color: #2a2a2a;
}

/* 导航菜单字体颜色（深色侧边栏白色文字） */
.aside :deep(.el-menu-item) {
  color: #fff !important;
  --el-menu-text-color: #ffffff !important;
}

.aside :deep(.el-menu-item:hover) {
  color: #fff !important;
  background-color: #1f2d3d !important;
}

.aside :deep(.el-menu-item.is-active) {
  color: #409eff !important;
  background-color: #1f2d3d !important;
}

.aside :deep(.el-sub-menu .el-sub-menu__title) {
  color: #fff !important;
  --el-menu-text-color: #ffffff !important;
}

.aside :deep(.el-sub-menu .el-sub-menu__title:hover) {
  color: #fff !important;
  background-color: #1f2d3d !important;
}
</style>
import { reactive, computed, watch } from 'vue'

// 主题管理（不使用 Pinia，用 reactive 简化）
export const themeStore = reactive({
  theme: localStorage.getItem('theme') || 'system', // light, dark, system
})

// 计算实际应用的主题
export const actualTheme = computed(() => {
  if (themeStore.theme === 'system') {
    if (typeof window !== 'undefined' && window.matchMedia) {
      return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    }
    return 'light'
  }
  return themeStore.theme
})

// 设置主题
export function setTheme(newTheme) {
  if (['light', 'dark', 'system'].includes(newTheme)) {
    themeStore.theme = newTheme
    localStorage.setItem('theme', newTheme)
    applyTheme()
  }
}

// 应用主题到 document
export function applyTheme() {
  if (typeof document === 'undefined') return
  const theme = actualTheme.value
  document.documentElement.setAttribute('data-theme', theme)
  
  // 设置 Element Plus 的暗黑模式
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else {
    document.documentElement.classList.remove('dark')
  }
}

// 初始化主题
export function initTheme() {
  applyTheme()
  
  // 监听系统主题变化
  if (typeof window !== 'undefined' && window.matchMedia) {
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (themeStore.theme === 'system') {
        applyTheme()
      }
    })
  }
}

// 切换主题（循环：light -> dark -> system -> light）
export function toggleTheme() {
  const themes = ['light', 'dark', 'system']
  const currentIndex = themes.indexOf(themeStore.theme)
  const nextIndex = (currentIndex + 1) % themes.length
  setTheme(themes[nextIndex])
}

// 监听主题变化自动应用
if (typeof window !== 'undefined') {
  watch(actualTheme, () => {
    applyTheme()
  })
}

export default {
  themeStore,
  actualTheme,
  setTheme,
  applyTheme,
  initTheme,
  toggleTheme
}
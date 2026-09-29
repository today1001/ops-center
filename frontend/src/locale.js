import { reactive, watch } from 'vue'
import { setLocale as setI18nLocale, getLocale } from './i18n.js'

// 语言管理
export const localeStore = reactive({
  locale: localStorage.getItem('locale') || 'zh-CN', // zh-CN, en-US
})

export const availableLocales = [
  { code: 'zh-CN', name: '中文' },
  { code: 'en-US', name: 'English' }
]

// 设置语言
export function setLocale(localeCode) {
  if (['zh-CN', 'en-US'].includes(localeCode)) {
    localeStore.locale = localeCode
    localStorage.setItem('locale', localeCode)
    setI18nLocale(localeCode)
  }
}

// 切换语言
export function toggleLocale() {
  const locales = availableLocales.map(l => l.code)
  const currentIndex = locales.indexOf(localeStore.locale)
  const nextIndex = (currentIndex + 1) % locales.length
  setLocale(locales[nextIndex])
}

// 初始化
export function initLocale() {
  const saved = localStorage.getItem('locale') || 'zh-CN'
  setLocale(saved)
}

export function getCurrentLocale() {
  return getLocale()
}

export default {
  localeStore,
  availableLocales,
  setLocale,
  toggleLocale,
  initLocale,
  getCurrentLocale
}
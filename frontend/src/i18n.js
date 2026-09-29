import { createI18n } from 'vue-i18n'
import zhCN from './zh-CN.json'
import enUS from './en-US.json'

const messages = {
  'zh-CN': zhCN,
  'en-US': enUS
}

// 从 localStorage 获取保存的语言，默认中文
const savedLocale = localStorage.getItem('locale') || 'zh-CN'

const i18n = createI18n({
  legacy: false, // 使用 Composition API 模式
  locale: savedLocale,
  fallbackLocale: 'zh-CN',
  messages,
  globalInjection: true, // 全局注入 $t
  legacy: false
})

export default i18n

export function setLocale(locale) {
  if (messages[locale]) {
    i18n.global.locale.value = locale
    localStorage.setItem('locale', locale)
  }
}

export function getLocale() {
  return i18n.global.locale.value
}

export function getAvailableLocales() {
  return [
    { code: 'zh-CN', name: '中文' },
    { code: 'en-US', name: 'English' }
  ]
}
import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import 'element-plus/dist/index.css'
import './theme.css'  // 主题样式
import i18n from './i18n.js'
import { initTheme } from './theme.js'
import { initLocale } from './locale.js'
import App from './App.vue'
import router from './router'

// 初始化主题和语言
initTheme()
initLocale()

const app = createApp(App)

// 注册所有图标
for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus)
app.use(i18n)
app.use(router)
app.mount('#app')
<template>
  <el-dialog
    :model-value="true"
    :title="dialogTitle"
    width="95%"
    top="2vh"
    fullscreen
    destroy-on-close
    :close-on-click-modal="false"
    @close="handleClose"
  >
    <div class="rdp-terminal-toolbar">
      <div class="toolbar-left">
        <el-tag type="primary" size="small">RDP</el-tag>
        <span class="conn-info">{{ username }}@{{ host }}:{{ port }}</span>
        <el-tag type="success" size="small" effect="dark">guacamole</el-tag>
      </div>
      <div class="toolbar-actions">
        <el-button size="small" type="danger" plain @click="handleClose">
          <el-icon><Close /></el-icon>
          断开
        </el-button>
      </div>
    </div>
    <div class="rdp-terminal-container">
      <iframe
        :src="iframeSrc"
        class="rdp-iframe"
        frameborder="0"
        allow="clipboard-read; clipboard-write"
      ></iframe>
    </div>
  </el-dialog>
</template>

<script setup>
import { computed } from 'vue'
import { Close } from '@element-plus/icons-vue'

const props = defineProps({
  host: { type: String, required: true },
  port: { type: Number, default: 3389 },
  username: { type: String, required: true },
  password: { type: String, default: '' },
  session: { type: Object, required: true }
})
const emit = defineEmits(['close'])

const dialogTitle = `远程桌面 - ${props.username}@${props.host}:${props.port}`

const iframeSrc = computed(() => {
  const params = new URLSearchParams({
    host: props.host,
    port: String(props.port),
    user: props.username,
    password: props.password,
    width: '1280',
    height: '800'
  })
  return `/rdpguac.html?${params.toString()}`
})

const handleClose = () => {
  emit('close')
}
</script>

<style scoped>
.rdp-terminal-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 12px;
  background: #16213e;
  border-bottom: 1px solid #0f3460;
}
.toolbar-left {
  display: flex;
  align-items: center;
  gap: 8px;
}
.conn-info {
  font-size: 13px;
  color: #aaa;
}
.toolbar-actions {
  display: flex;
  gap: 8px;
}
.rdp-terminal-container {
  height: calc(100vh - 80px);
  background: #000;
}
.rdp-iframe {
  width: 100%;
  height: 100%;
  border: none;
}
</style>
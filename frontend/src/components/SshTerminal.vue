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
    <div class="ssh-terminal-toolbar">
      <div class="toolbar-left">
        <el-tag type="success" size="small">SSH</el-tag>
        <el-tag v-if="shadow" type="warning" size="small" effect="dark">观看模式（只读）</el-tag>
        <span class="conn-info">{{ user }}@{{ host }}:{{ port }}</span>
        <el-tag v-if="connected" type="success" size="small" effect="dark">已连接</el-tag>
        <el-tag v-else-if="connecting" type="warning" size="small" effect="dark">连接中...</el-tag>
        <el-tag v-else type="danger" size="small" effect="dark">已断开</el-tag>
      </div>
      <div class="toolbar-actions">
        <el-button size="small" @click="reconnect" :disabled="connecting">
          <el-icon><RefreshRight /></el-icon>
          重连
        </el-button>
        <el-button size="small" type="danger" plain @click="handleClose">
          <el-icon><Close /></el-icon>
          断开
        </el-button>
      </div>
    </div>
    <div ref="terminalRef" class="ssh-terminal-container"></div>
  </el-dialog>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { RefreshRight, Close } from '@element-plus/icons-vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

const props = defineProps({
  host: { type: String, required: true },
  port: { type: Number, default: 22 },
  user: { type: String, required: true },
  pass: { type: String, default: '' },
  token: { type: String, required: true },
  shadow: { type: Boolean, default: false }
})
const emit = defineEmits(['close'])

const terminalRef = ref(null)
const connected = ref(false)
const connecting = ref(false)

let terminal = null
let fitAddon = null
let ws = null
let onDataDisposable = null
let onBinaryDisposable = null
let resizeObs = null
let pingTimer = null
let unmounted = false

const dialogTitle = `SSH终端 - ${props.user}@${props.host}:${props.port}`

const waitForContainer = () => new Promise(resolve => {
  const el = terminalRef.value
  if (!el) return resolve()
  const check = () => {
    if (el.offsetWidth > 0 && el.offsetHeight > 0) return resolve()
    requestAnimationFrame(check)
  }
  requestAnimationFrame(check)
})

const initTerminal = async () => {
  await waitForContainer()

  if (terminal) terminal.dispose()

  terminal = new Terminal({
    cursorBlink: true,
    cursorStyle: 'bar',
    fontSize: 14,
    fontFamily: '"Cascadia Code", "Fira Code", "JetBrains Mono", "Menlo", "Consolas", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4',
      cursor: '#d4d4d4',
      cursorAccent: '#1e1e1e',
      selectionBackground: '#264f78',
      black: '#1e1e1e', red: '#f44747', green: '#6a9955', yellow: '#dcdcaa',
      blue: '#569cd6', magenta: '#c586c0', cyan: '#4ec9b0', white: '#d4d4d4',
      brightBlack: '#808080', brightRed: '#f44747', brightGreen: '#6a9955',
      brightYellow: '#dcdcaa', brightBlue: '#569cd6', brightMagenta: '#c586c0',
      brightCyan: '#4ec9b0', brightWhite: '#ffffff'
    },
    allowProposedApi: true,
    scrollback: 10000
  })

  fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.loadAddon(new WebLinksAddon())
  terminal.open(terminalRef.value)
  fitAddon.fit()

  resizeObs = new ResizeObserver(() => {
    if (fitAddon) {
      fitAddon.fit()
      if (connected.value) sendResize()
    }
  })
  resizeObs.observe(terminalRef.value)
}

const sendResize = () => {
  if (ws && ws.readyState === WebSocket.OPEN && terminal && fitAddon) {
    const dims = fitAddon.proposeDimensions()
    if (dims && dims.cols > 0 && dims.rows > 0) {
      ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }))
    }
  }
}

const doConnect = () => {
  if (unmounted) return
  if (connecting.value || connected.value) return
  if (!terminal || !fitAddon) return
  connecting.value = true

  const wsProtocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
  const dims = fitAddon.proposeDimensions()
  const cols = (dims && dims.cols > 0) ? dims.cols : 120
  const rows = (dims && dims.rows > 0) ? dims.rows : 40
  const wsUrl = `${wsProtocol}//${location.host}/ws/ssh/${props.token}?cols=${cols}&rows=${rows}`

  ws = new WebSocket(wsUrl)
  ws.binaryType = 'arraybuffer'

  ws.onopen = () => {
    if (unmounted) { ws.close(); return }
    connected.value = true
    connecting.value = false
    terminal.focus()
    if (pingTimer) clearInterval(pingTimer)
    pingTimer = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping' }))
      }
    }, 15000)
  }

  ws.onmessage = (event) => {
    if (unmounted || !terminal) return
    if (event.data instanceof ArrayBuffer) {
      terminal.write(new Uint8Array(event.data))
    } else {
      const text = event.data
      if (text && text.startsWith('{') && text.indexOf('"type"') !== -1) {
        return
      }
      terminal.write(text)
    }
  }

  ws.onerror = () => {
    if (!unmounted) connecting.value = false
  }

  ws.onclose = () => {
    if (pingTimer) { clearInterval(pingTimer); pingTimer = null }
    connected.value = false
    if (!unmounted) {
      connecting.value = false
      if (terminal) {
        terminal.write('\r\n\x1b[33m[连接已断开]\x1b[0m\r\n')
      }
    }
  }

  if (onDataDisposable) onDataDisposable.dispose()
  onDataDisposable = terminal.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(new TextEncoder().encode(data))
    }
  })

  if (onBinaryDisposable) onBinaryDisposable.dispose()
  onBinaryDisposable = terminal.onBinary((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(new TextEncoder().encode(data))
    }
  })
}

const reconnect = () => {
  if (pingTimer) { clearInterval(pingTimer); pingTimer = null }
  if (ws) { ws.close(); ws = null }
  connected.value = false
  connecting.value = false
  nextTick(() => {
    if (terminal) {
      terminal.clear()
      terminal.write('\x1b[33m正在重连...\x1b[0m\r\n')
    }
    doConnect()
  })
}

const handleClose = () => {
  if (pingTimer) { clearInterval(pingTimer); pingTimer = null }
  if (ws) { ws.close(); ws = null }
  connected.value = false
  emit('close')
}

onMounted(async () => {
  await nextTick()
  await initTerminal()
  if (terminal) {
    terminal.write('\x1b[33m正在连接...\x1b[0m\r\n')
  }
  doConnect()
})

onBeforeUnmount(() => {
  unmounted = true
  if (pingTimer) { clearInterval(pingTimer); pingTimer = null }
  if (onDataDisposable) onDataDisposable.dispose()
  if (onBinaryDisposable) onBinaryDisposable.dispose()
  if (resizeObs) resizeObs.disconnect()
  if (ws) { ws.close(); ws = null }
  if (terminal) { terminal.dispose(); terminal = null }
})
</script>

<style scoped>
.ssh-terminal-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  background-color: #f5f7fa;
  border-radius: 6px;
  margin-bottom: 10px;
}
.toolbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
}
.conn-info {
  font-size: 13px;
  font-family: monospace;
  color: #606266;
}
.toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.ssh-terminal-container {
  height: 82vh;
  background-color: #1e1e1e;
  border-radius: 6px;
  padding: 4px;
  overflow: hidden;
}
.ssh-terminal-container :deep(.xterm) {
  padding: 4px;
}
.ssh-terminal-container :deep(.xterm-viewport) {
  overflow-y: auto !important;
}
:deep(.el-dialog__body) {
  padding: 10px 16px 16px;
}
:deep(.el-dialog__header) {
  padding: 12px 16px;
  margin-right: 0;
}
</style>

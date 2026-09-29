<template>
  <div class="status-dot" :class="statusClass" :title="statusText"></div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  status: {
    type: String,
    default: 'offline'
  }
})

const statusClass = computed(() => {
  const classes = {
    'active': 'status-online',
    'busy': 'status-busy',
    'unreachable': 'status-unreachable',
    'offline': 'status-offline'
  }
  return classes[props.status] || 'status-offline'
})

const statusText = computed(() => {
  const texts = {
    'active': '正常',
    'busy': '忙碌',
    'unreachable': '无法连接',
    'offline': '掉线'
  }
  return texts[props.status] || '未知'
})
</script>

<style scoped>
.status-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  display: inline-block;
}

.status-online {
  background-color: #67C23A;
  box-shadow: 0 0 6px #67C23A;
}

.status-busy {
  background-color: #E6A23C;
  box-shadow: 0 0 6px #E6A23C;
}

.status-unreachable {
  background-color: #409EFF;
  box-shadow: 0 0 6px #409EFF;
}

.status-offline {
  background-color: #F56C6C;
  box-shadow: 0 0 6px #F56C6C;
}
</style>

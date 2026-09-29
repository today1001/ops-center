<template>
  <div class="status-legend">
    <span class="item"><i class="dot online"></i>正常</span>
    <span class="item"><i class="dot busy"></i>忙碌</span>
    <span class="item"><i class="dot unreachable"></i>无法连接</span>
    <span class="item"><i class="dot offline"></i>掉线</span>
  </div>
</template>

<script setup>
</script>

<style scoped>
.status-legend {
  display: flex;
  align-items: center;
  gap: 18px;
  font-size: 13px;
  color: #606266;
  padding: 8px 0;
}

.item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.dot {
  width: 11px;
  height: 11px;
  border-radius: 50%;
  display: inline-block;
}

.online { background-color: #67C23A; box-shadow: 0 0 4px #67C23A; }
.busy { background-color: #E6A23C; box-shadow: 0 0 4px #E6A23C; }
.unreachable { background-color: #409EFF; box-shadow: 0 0 4px #409EFF; }
.offline { background-color: #F56C6C; box-shadow: 0 0 4px #F56C6C; }
</style>
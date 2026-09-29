<template>
  <div class="server-detail">
    <el-page-header @back="$router.back()" :title="server.name || '服务器详情'">
      <template #extra>
        <el-button type="primary" @click="handleRefresh" :loading="refreshing">
          <el-icon><Refresh /></el-icon>
          {{ hasDetail ? '刷新信息' : '采集信息' }}
        </el-button>
      </template>
    </el-page-header>

    <!-- 无详情数据时的提示 -->
    <el-alert
      v-if="!hasDetail && !loading"
      title="暂未采集到服务器详情"
      description='请点击右上角"采集信息"按钮获取服务器系统信息'
      type="warning"
      show-icon
      style="margin-top: 20px;"
    />

    <el-row :gutter="20" style="margin-top: 20px;">
      <!-- 基础信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>基础信息</span>
              <StatusDot :status="server.status" />
            </div>
          </template>
          
          <el-descriptions :column="1" border>
            <el-descriptions-item label="服务器名称">{{ server.name }}</el-descriptions-item>
            <el-descriptions-item label="IP地址">{{ server.ip }}</el-descriptions-item>
            <el-descriptions-item label="端口">{{ server.port }}</el-descriptions-item>
            <el-descriptions-item label="用户名">{{ server.username }}</el-descriptions-item>
            <el-descriptions-item label="认证方式">
              <el-tag size="small" :type="server.auth_type === 'key' ? 'warning' : 'info'">
                {{ server.auth_type === 'key' ? '密钥' : '密码' }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="statusType">{{ statusText }}</el-tag>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <!-- 系统信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>系统信息</span>
          </template>
          
          <el-descriptions :column="1" border>
            <el-descriptions-item label="主机名">{{ detail.hostname || '-' }}</el-descriptions-item>
            <el-descriptions-item label="操作系统">{{ detail.os || '-' }}</el-descriptions-item>
            <el-descriptions-item label="系统版本">{{ detail.os_version || '-' }}</el-descriptions-item>
            <el-descriptions-item label="内核版本">{{ detail.kernel || '-' }}</el-descriptions-item>
            <el-descriptions-item label="运行时间">{{ detail.uptime || '-' }}</el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px;">
      <!-- 硬件信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>硬件信息</span>
          </template>
          
          <el-descriptions :column="1" border>
            <el-descriptions-item label="CPU型号">{{ detail.cpu_model || '-' }}</el-descriptions-item>
            <el-descriptions-item label="CPU核心数">{{ detail.cpu_cores || '-' }}</el-descriptions-item>
            <el-descriptions-item label="CPU使用率">
              <el-progress :percentage="detail.cpu_usage || 0" :color="getProgressColor(detail.cpu_usage)" />
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <!-- 内存信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>内存信息</span>
          </template>
          
          <el-descriptions :column="1" border>
            <el-descriptions-item label="总内存">{{ formatBytes(detail.memory_total) }}</el-descriptions-item>
            <el-descriptions-item label="已使用">{{ formatBytes(detail.memory_used) }}</el-descriptions-item>
            <el-descriptions-item label="使用率">
              <el-progress :percentage="detail.memory_usage || 0" :color="getProgressColor(detail.memory_usage)" />
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px;">
      <!-- 磁盘信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>磁盘信息</span>
          </template>
          
          <el-descriptions :column="1" border>
            <el-descriptions-item label="总磁盘">{{ formatBytes(detail.disk_total) }}</el-descriptions-item>
            <el-descriptions-item label="已使用">{{ formatBytes(detail.disk_used) }}</el-descriptions-item>
            <el-descriptions-item label="使用率">
              <el-progress :percentage="detail.disk_usage || 0" :color="getProgressColor(detail.disk_usage)" />
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>

      <!-- 网络信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>网络信息</span>
          </template>
          
          <el-descriptions :column="1" border>
            <el-descriptions-item label="IP地址">{{ detail.network_ip || '-' }}</el-descriptions-item>
            <el-descriptions-item label="MAC地址">{{ detail.network_mac || '-' }}</el-descriptions-item>
            <el-descriptions-item label="默认网关">{{ detail.network_gateway || '-' }}</el-descriptions-item>
            <el-descriptions-item label="DNS服务器">{{ detail.network_dns || '-' }}</el-descriptions-item>
            <el-descriptions-item label="网卡速率">{{ detail.network_speed ? detail.network_speed + ' Mbps' : '-' }}</el-descriptions-item>
            <el-descriptions-item label="接收速率">{{ detail.network_rx ? detail.network_rx + ' Mbps' : '-' }}</el-descriptions-item>
            <el-descriptions-item label="发送速率">{{ detail.network_tx ? detail.network_tx + ' Mbps' : '-' }}</el-descriptions-item>
            <el-descriptions-item label="网络利用率">
              <el-progress :percentage="detail.network_usage || 0" :color="getProgressColor(detail.network_usage)" />
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-top: 20px;">
      <!-- 端口信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>监听端口</span>
          </template>
          
          <div class="tag-list">
            <el-tooltip v-for="port in parsedPorts" :key="port" :content="getPortDescription(port)" placement="top">
              <el-tag size="small" style="margin: 2px; cursor: pointer;">
                {{ port }}
              </el-tag>
            </el-tooltip>
            <span v-if="parsedPorts.length === 0">-</span>
          </div>
        </el-card>
      </el-col>

      <!-- 服务信息 -->
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>运行服务</span>
          </template>
          
          <div class="tag-list">
            <el-tooltip v-for="service in parsedServices" :key="service" :content="getServiceDescription(service)" placement="top">
              <el-tag size="small" type="success" style="margin: 2px; cursor: pointer;">
                {{ service }}
              </el-tag>
            </el-tooltip>
            <span v-if="parsedServices.length === 0">-</span>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 最后检查时间 -->
    <el-card style="margin-top: 20px;">
      <template #header>
        <span>检查信息</span>
      </template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="最后检查时间">{{ formatTime(detail.last_checked) }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(detail.created_at) }}</el-descriptions-item>
      </el-descriptions>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { serverAPI } from '../../api'
import StatusDot from '../../components/StatusDot.vue'

const route = useRoute()
const router = useRouter()
const server = ref({})
const detail = ref({})
const refreshing = ref(false)
const loading = ref(false)

const hasDetail = computed(() => {
  return detail.value && detail.value.id
})

onMounted(async () => {
  await loadData()
})

const loadData = async () => {
  const id = route.params.id
  loading.value = true
  try {
    server.value = await serverAPI.getById(id)
    try {
      detail.value = await serverAPI.getDetail(id)
    } catch (e) {
      detail.value = {}
    }
  } catch (error) {
    ElMessage.error('加载服务器信息失败')
    router.back()
  } finally {
    loading.value = false
  }
}

const handleRefresh = async () => {
  refreshing.value = true
  try {
    await serverAPI.refresh(route.params.id)
    // 等待采集完成（最多等待30秒）
    let retries = 0
    const maxRetries = 30
    while (retries < maxRetries) {
      await new Promise(resolve => setTimeout(resolve, 1000))
      try {
        const newDetail = await serverAPI.getDetail(route.params.id)
        if (newDetail && newDetail.id) {
          detail.value = newDetail
          ElMessage.success('采集完成')
          break
        }
      } catch (e) {
        // 继续等待
      }
      retries++
    }
    if (retries >= maxRetries) {
      ElMessage.warning('采集超时，请稍后刷新页面查看')
    }
    await loadData()
  } catch (error) {
    ElMessage.error('刷新失败')
  } finally {
    refreshing.value = false
  }
}

const parsedPorts = computed(() => {
  try {
    return JSON.parse(detail.value.ports || '[]')
  } catch {
    return []
  }
})

const parsedServices = computed(() => {
  try {
    return JSON.parse(detail.value.services || '[]')
  } catch {
    return []
  }
})

const statusType = computed(() => {
  const types = {
    'active': 'success',
    'busy': 'warning',
    'unreachable': 'primary',
    'offline': 'danger'
  }
  return types[server.value.status] || 'info'
})

const statusText = computed(() => {
  const texts = {
    'active': '正常',
    'busy': '忙碌',
    'unreachable': '无法连接',
    'offline': '掉线'
  }
  return texts[server.value.status] || '未知'
})

const formatBytes = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const formatTime = (timeStr) => {
  if (!timeStr) return '-'
  return new Date(timeStr).toLocaleString('zh-CN')
}

const getProgressColor = (percentage) => {
  if (percentage < 60) return '#67C23A'
  if (percentage < 80) return '#E6A23C'
  return '#F56C6C'
}

const portDescriptions = {
  '21': 'FTP文件传输协议，用于文件上传和下载',
  '22': 'SSH安全外壳协议，用于远程登录和管理',
  '23': 'Telnet远程登录协议（不安全，建议使用SSH）',
  '25': 'SMTP简单邮件传输协议，用于发送邮件',
  '53': 'DNS域名系统，用于域名解析',
  '80': 'HTTP超文本传输协议，用于Web服务',
  '110': 'POP3邮局协议，用于接收邮件',
  '143': 'IMAP互联网消息访问协议，用于接收邮件',
  '443': 'HTTPS安全超文本传输协议，用于加密Web服务',
  '445': 'SMB服务器消息块，用于Windows文件共享',
  '993': 'IMAPS安全IMAP协议，用于加密邮件接收',
  '995': 'POP3S安全POP3协议，用于加密邮件接收',
  '1433': 'Microsoft SQL Server数据库默认端口',
  '1521': 'Oracle数据库默认端口',
  '3000': '常用开发服务器端口（Node.js、React等）',
  '3306': 'MySQL数据库默认端口',
  '3389': 'RDP远程桌面协议，用于Windows远程桌面',
  '5432': 'PostgreSQL数据库默认端口',
  '5900': 'VNC虚拟网络计算，用于远程桌面访问',
  '6379': 'Redis内存数据库默认端口',
  '8006': 'Proxmox VE管理界面端口',
  '8080': '常用HTTP代理或Web应用端口',
  '8088': '常用Web应用端口',
  '8089': '常用Web应用端口',
  '8090': '常用Web应用端口',
  '8091': '常用Web应用端口',
  '8093': '常用Web应用端口',
  '8443': '常用HTTPS替代端口',
  '8888': '常用Web服务或代理端口',
  '9090': '常用监控或管理界面端口',
  '9200': 'Elasticsearch搜索引擎默认端口',
  '27017': 'MongoDB数据库默认端口',
  '50000': 'SAP系统常用端口'
}

const serviceDescriptions = {
  'nginx.service': 'Nginx高性能Web服务器和反向代理',
  'apache2.service': 'Apache HTTP服务器',
  'httpd.service': 'Apache HTTP服务器（Red Hat系）',
  'sshd.service': 'SSH守护进程，提供安全远程登录',
  'mysql.service': 'MySQL数据库服务',
  'mysqld.service': 'MySQL数据库守护进程',
  'postgresql.service': 'PostgreSQL数据库服务',
  'redis.service': 'Redis内存数据库服务',
  'redis-server.service': 'Redis服务器进程',
  'docker.service': 'Docker容器引擎',
  'containerd.service': 'Containerd容器运行时',
  'kubelet.service': 'Kubernetes节点代理',
  'kube-proxy.service': 'Kubernetes网络代理',
  'etcd.service': 'etcd分布式键值存储（Kubernetes核心组件）',
  'cron.service': 'cron定时任务调度器',
  'crond.service': 'crond定时任务守护进程（Red Hat系）',
  'rsyslog.service': '系统日志服务',
  'systemd-journald.service': 'systemd日志管理服务',
  'firewalld.service': 'firewalld防火墙服务',
  'iptables.service': 'iptables防火墙服务',
  'ufw.service': 'Ubuntu简单防火墙',
  'postfix.service': 'Postfix邮件传输代理',
  'sendmail.service': 'Sendmail邮件传输代理',
  'dovecot.service': 'Dovecot邮件投递代理（IMAP/POP3）',
  'vsftpd.service': 'VSFTP安全FTP服务器',
  'proftpd.service': 'ProFTP文件传输协议服务器',
  'smb.service': 'Samba SMB文件共享服务',
  'nmb.service': 'Samba NetBIOS名称服务',
  'memcached.service': 'Memcached分布式内存缓存',
  'mongod.service': 'MongoDB数据库服务',
  'elasticsearch.service': 'Elasticsearch搜索引擎',
  'kibana.service': 'Kibana数据可视化平台',
  'logstash.service': 'Logstash日志处理管道',
  'prometheus.service': 'Prometheus监控系统',
  'grafana-server.service': 'Grafana数据可视化仪表盘',
  'node_exporter.service': 'Prometheus Node Exporter指标导出器',
  'chrony.service': 'Chrony网络时间协议（NTP）服务',
  'ntpd.service': 'NTP网络时间协议服务',
  'dbus.service': 'D-Bus进程间通信总线',
  'getty@tty1.service': '虚拟终端登录服务',
  'lxcfs.service': 'LXC文件系统（容器支持）',
  'lxc-monitord.service': 'LXC容器监控',
  'pve-cluster.service': 'Proxmox VE集群服务',
  'pve-firewall.service': 'Proxmox VE防火墙',
  'pve-ha-crm.service': 'Proxmox VE高可用资源管理器',
  'pve-container@.service': 'Proxmox VE LXC容器服务',
  'ksmtuned.service': '内核相同页合并（KSM）调优服务',
  'dm-event.service': '设备映射器事件监控服务',
  'easytier.service': 'EasyTier去中心化组网服务',
  'gvm-simulator.service': 'GVM（Greenbone）漏洞扫描模拟器',
  'proxmox-firewall.service': 'Proxmox防火墙服务',
  'php-fpm.service': 'PHP FastCGI进程管理器',
  'mongo.service': 'MongoDB数据库服务',
  'beanstalkd.service': 'Beanstalkd工作队列服务',
  'rabbitmq.service': 'RabbitMQ消息队列服务',
  'activemq.service': 'ActiveMQ消息队列服务',
  'consul.service': 'Consul服务发现和配置',
  'vault.service': 'Vault密钥管理服务',
  'nomad.service': 'Nomad工作负载调度器',
  'traefik.service': 'Traefik云原生反向代理',
  'haproxy.service': 'HAProxy高性能负载均衡器',
  'keepalived.service': 'Keepalived高可用性服务',
  'zabbix_agentd.service': 'Zabbix监控代理',
  'agentd.service': 'Zabbix监控代理',
  'snmpd.service': 'SNMP简单网络管理协议代理',
  'named.service': 'BIND DNS服务器',
  'dhcpd.service': 'DHCP服务器',
  'squid.service': 'Squid Web代理缓存服务器',
  'openvpn.service': 'OpenVPN虚拟专用网络',
  'wireguard.service': 'WireGuard VPN隧道',
  'strongswan.service': 'StrongSwan IPsec VPN',
  'frr.service': 'FRRouting动态路由协议套件',
  'zebra.service': 'Zebra路由守护进程',
  'multipathd.service': '多路径设备管理器',
  'udisks2.service': '磁盘管理服务',
  'accounts-daemon.service': 'AccountsService用户账户管理',
  'avahi-daemon.service': 'Avahi mDNS/DNS-SD服务发现',
  'cups.service': 'CUPS打印服务',
  'bluetooth.service': '蓝牙服务',
  'wpa_supplicant.service': 'WPA无线认证客户端',
  'NetworkManager.service': '网络管理器',
  'systemd-networkd.service': 'systemd网络管理',
  'systemd-resolved.service': 'systemd DNS解析器',
  'polkit.service': 'PolicyKit权限管理',
  'thermald.service': 'Intel热管理守护进程',
  'power-profiles-daemon.service': '电源配置管理',
  'switcheroo-control.service': 'GPU切换控制',
  'gpu-manager.service': 'GPU管理器',
  'snapd.service': 'Snap包管理服务',
  'multipathd.service': '多路径存储设备管理',
  'open-vm-tools.service': 'VMware虚拟机工具',
  'spice-vdagent.service': 'SPICE虚拟桌面代理',
  'qemu-guest-agent.service': 'QEMU虚拟机代理',
  'hv_kvp_daemon.service': 'Hyper-V键值对守护进程',
  'hv_vss_daemon.service': 'Hyper-V卷影复制服务',
  'waagent.service': 'Windows Azure Linux Agent'
}

const getPortDescription = (port) => {
  return portDescriptions[port] || `端口 ${port} - 未知服务`
}

const getServiceDescription = (service) => {
  const name = service.replace('.service', '')
  return serviceDescriptions[service] || serviceDescriptions[name + '.service'] || `服务 ${name} - 系统服务`
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.tag-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
</style>

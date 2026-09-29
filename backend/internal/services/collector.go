package services

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"

	"ops-center/internal/models"

	"golang.org/x/crypto/ssh"
)

// CollectSystemInfo 采集系统信息
func CollectSystemInfo(server *models.Server) (*models.ServerDetail, error) {
	log.Printf("[Collector] 开始采集服务器 %s (%s) 的系统信息", server.Name, server.IP)
	
	client, err := GetSSHClient(server)
	if err != nil {
		return nil, fmt.Errorf("SSH连接失败: %v", err)
	}
	defer client.Close()

	detail := &models.ServerDetail{
		ServerID: server.ID,
	}

	// 采集主机名
	detail.Hostname = executeCommand(client, "hostname")
	log.Printf("[Collector] 主机名: %s", detail.Hostname)

	// 采集操作系统信息
	osInfo := executeCommand(client, "cat /etc/os-release 2>/dev/null | grep PRETTY_NAME | cut -d'\"' -f2")
	if osInfo == "" {
		osInfo = executeCommand(client, "uname -s")
	}
	detail.OS = osInfo

	// 采集OS版本
	detail.OSVersion = executeCommand(client, "cat /etc/os-release 2>/dev/null | grep VERSION_ID | cut -d'\"' -f2")

	// 采集内核版本
	detail.Kernel = executeCommand(client, "uname -r")

	// 采集运行时间
	detail.Uptime = executeCommand(client, "uptime -p 2>/dev/null || uptime")

	// 采集CPU信息
	detail.CPUModel = executeCommand(client, "cat /proc/cpuinfo 2>/dev/null | grep 'model name' | head -1 | cut -d':' -f2 | xargs")
	coresStr := executeCommand(client, "nproc 2>/dev/null || echo 1")
	detail.CPUCores, _ = strconv.Atoi(coresStr)
	// 简化CPU使用率采集
	cpuUsageStr := executeCommand(client, "top -bn1 2>/dev/null | grep 'Cpu(s)' | awk '{print $2}' | cut -d'%' -f1")
	if cpuUsageStr == "" {
		cpuUsageStr = "0"
	}
	detail.CPUUsage, _ = strconv.ParseFloat(cpuUsageStr, 64)
	log.Printf("[Collector] CPU: %s, 核心数: %d", detail.CPUModel, detail.CPUCores)

	// 采集内存信息
	memInfo := executeCommand(client, "free -b 2>/dev/null | grep Mem || free | grep Mem")
	memFields := strings.Fields(memInfo)
	if len(memFields) >= 3 {
		detail.MemoryTotal, _ = strconv.ParseInt(memFields[1], 10, 64)
		detail.MemoryUsed, _ = strconv.ParseInt(memFields[2], 10, 64)
		if detail.MemoryTotal > 0 {
			detail.MemoryUsage = float64(detail.MemoryUsed) / float64(detail.MemoryTotal) * 100
		}
	}
	log.Printf("[Collector] 内存: %d / %d", detail.MemoryUsed, detail.MemoryTotal)

	// 采集磁盘信息
	diskInfo := executeCommand(client, "df -B1 / 2>/dev/null | tail -1")
	diskFields := strings.Fields(diskInfo)
	if len(diskFields) >= 4 {
		detail.DiskTotal, _ = strconv.ParseInt(diskFields[1], 10, 64)
		detail.DiskUsed, _ = strconv.ParseInt(diskFields[2], 10, 64)
		if detail.DiskTotal > 0 {
			detail.DiskUsage = float64(detail.DiskUsed) / float64(detail.DiskTotal) * 100
		}
	}
	log.Printf("[Collector] 磁盘: %d / %d", detail.DiskUsed, detail.DiskTotal)

	// 采集网络信息
	detail.NetworkIP = executeCommand(client, "hostname -I 2>/dev/null | awk '{print $1}'")
	detail.NetworkMAC = executeCommand(client, "ip link show 2>/dev/null | grep 'link/ether' | head -1 | awk '{print $2}'")
	
	// 采集网关信息
	gateway := executeCommand(client, "ip route show default 2>/dev/null | awk '/default/ {print $3}'")
	if gateway == "" {
		gateway = executeCommand(client, "route -n 2>/dev/null | grep '0.0.0.0' | head -1 | awk '{print $2}'")
	}
	detail.NetworkGateway = gateway
	
	// 采集DNS信息
	dns := executeCommand(client, "cat /etc/resolv.conf 2>/dev/null | grep '^nameserver' | awk '{print $2}' | head -3 | tr '\n' ',' | sed 's/,$//'")
	detail.NetworkDNS = dns
	
	// 采集网络速率和利用率
	netInfo := executeCommand(client, `iface=$(ip route show default 2>/dev/null | awk '/default/ {print $5}' | head -1); if [ -z "$iface" ]; then iface=$(ls /sys/class/net 2>/dev/null | grep -v lo | head -1); fi; if [ -z "$iface" ]; then echo ""; exit; fi; speed=$(cat /sys/class/net/$iface/speed 2>/dev/null || echo 0); if [ "$speed" = "-1" ] || [ "$speed" = "" ]; then speed=0; fi; rx1=$(cat /proc/net/dev | awk -v i="$iface" '$1 ~ i":" {print $2}'); tx1=$(cat /proc/net/dev | awk -v i="$iface" '$1 ~ i":" {print $10}'); sleep 1; rx2=$(cat /proc/net/dev | awk -v i="$iface" '$1 ~ i":" {print $2}'); tx2=$(cat /proc/net/dev | awk -v i="$iface" '$1 ~ i":" {print $10}'); rx=$(( (rx2 - rx1) * 8 / 1000000 )); tx=$(( (tx2 - tx1) * 8 / 1000000 )); echo "$iface|$speed|$rx|$tx"`)
	log.Printf("[Collector] 网络: %s", netInfo)
	if netInfo != "" {
		parts := strings.Split(netInfo, "|")
		if len(parts) == 4 {
			iface := parts[0]
			speed, _ := strconv.Atoi(parts[1])
			rx, _ := strconv.Atoi(parts[2])
			tx, _ := strconv.Atoi(parts[3])
			detail.NetworkSpeed = speed
			detail.NetworkRX = rx
			detail.NetworkTX = tx
			if speed > 0 {
				usage := float64(rx)
				if tx > rx {
					usage = float64(tx)
				}
				detail.NetworkUsage = usage / float64(speed) * 100
			}
			log.Printf("[Collector] 网卡%s 速率%dMbps RX:%dMbps TX:%dMbps 利用率:%.1f%%", iface, speed, rx, tx, detail.NetworkUsage)
		}
	}

	// 采集监听端口
	portsOutput := executeCommand(client, "ss -tlnp 2>/dev/null | awk 'NR>1 {print $4}' | rev | cut -d':' -f1 | rev | sort -u || netstat -tlnp 2>/dev/null | awk 'NR>2 {print $4}' | rev | cut -d':' -f1 | rev | sort -u")
	ports := strings.Split(strings.TrimSpace(portsOutput), "\n")
	if ports[0] == "" {
		ports = []string{}
	}
	portsJSON, _ := json.Marshal(ports)
	detail.Ports = string(portsJSON)
	log.Printf("[Collector] 端口: %v", ports)

	// 采集运行的服务
	servicesOutput := executeCommand(client, "systemctl list-units --type=service --state=running --no-pager 2>/dev/null | awk 'NR>1 {print $1}' | head -20")
	if servicesOutput == "" {
		servicesOutput = executeCommand(client, "ps aux 2>/dev/null | awk 'NR>1 {print $11}' | head -20")
	}
	services := strings.Split(strings.TrimSpace(servicesOutput), "\n")
	if services[0] == "" {
		services = []string{}
	}
	servicesJSON, _ := json.Marshal(services)
	detail.Services = string(servicesJSON)
	log.Printf("[Collector] 服务: %v", services)

	log.Printf("[Collector] 采集完成: %s", detail.Hostname)
	return detail, nil
}

// executeCommand 执行SSH命令并返回输出
func executeCommand(client *ssh.Client, command string) string {
	session, err := client.NewSession()
	if err != nil {
		log.Printf("[Collector] 创建会话失败: %v", err)
		return ""
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		// 命令执行失败不一定是错误，可能只是没有输出
		return ""
	}

	return strings.TrimSpace(string(output))
}

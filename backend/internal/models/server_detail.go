package models

import "time"

// ServerDetail 服务器详情模型
type ServerDetail struct {
	ID            int       `json:"id"`
	ServerID      int       `json:"server_id"`
	Hostname      string    `json:"hostname"`
	OS            string    `json:"os"`
	OSVersion     string    `json:"os_version"`
	Kernel        string    `json:"kernel"`
	Uptime        string    `json:"uptime"`
	CPUModel      string    `json:"cpu_model"`
	CPUCores      int       `json:"cpu_cores"`
	CPUUsage      float64   `json:"cpu_usage"`
	MemoryTotal   int64     `json:"memory_total"`
	MemoryUsed    int64     `json:"memory_used"`
	MemoryUsage   float64   `json:"memory_usage"`
	DiskTotal     int64     `json:"disk_total"`
	DiskUsed      int64     `json:"disk_used"`
	DiskUsage     float64   `json:"disk_usage"`
	NetworkIP     string    `json:"network_ip"`
	NetworkMAC    string    `json:"network_mac"`
	NetworkGateway string   `json:"network_gateway"`
	NetworkDNS    string    `json:"network_dns"`
	NetworkSpeed  int       `json:"network_speed"`  // 网卡速率 Mbps
	NetworkRX     int       `json:"network_rx"`     // 当前接收 Mbps
	NetworkTX     int       `json:"network_tx"`     // 当前发送 Mbps
	NetworkUsage  float64   `json:"network_usage"`  // 网络利用率 %
	Ports         string    `json:"ports"`         // JSON数组
	Services      string    `json:"services"`      // JSON数组
	LastChecked   time.Time `json:"last_checked"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ResourceStats 资源汇总统计
type ResourceStats struct {
	ServerCount  int `json:"server_count"`
	DetailCount  int `json:"detail_count"`
	// CPU
	TotalCPUCores int     `json:"total_cpu_cores"`
	AvgCPUUsage   float64 `json:"avg_cpu_usage"`
	// 内存
	TotalMemory int64   `json:"total_memory"`
	UsedMemory  int64   `json:"used_memory"`
	MemoryUsage float64 `json:"memory_usage"`
	// 磁盘
	TotalDisk int64   `json:"total_disk"`
	UsedDisk  int64   `json:"used_disk"`
	DiskUsage float64 `json:"disk_usage"`
	// 网络
	TotalNetworkSpeed int     `json:"total_network_speed"`
	TotalNetworkRX    int     `json:"total_network_rx"`
	TotalNetworkTX    int     `json:"total_network_tx"`
	NetworkUsage      float64 `json:"network_usage"`
}

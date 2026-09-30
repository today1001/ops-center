package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"ops-center/internal/models"
)

// ===================== 虚拟网络实例管理（创建/加入/停止/删除） =====================

const (
	etBin        = "/home/lt/ops-center/bin/easytier-core"
	etCli        = "/home/lt/ops-center/bin/easytier-cli"
	etConfigDir  = "/home/lt/ops-center/bin/networks"
	etLogDir     = "/home/lt/ops-center/bin/logs"
	etRpcBase    = 15889 // 本机实例 RPC 端口从 15889 起（15888 被系统 ltnet 占用）
)

type JoinRequest struct {
	Name     string `json:"name"`      // 网络名
	Secret   string `json:"secret"`    // 网络密钥
	LocalIP  string `json:"local_ip"`  // 本机虚拟IP，如 10.10.20.1/24
	Peers    string `json:"peers"`     // 对端节点URI，逗号分隔，可空
	Subnet   string `json:"subnet"`    // 虚拟网段，如 10.10.20.0/24
	Autostart bool  `json:"autostart"` // 开机自启
}

type JoinResult struct {
	Created   bool   `json:"created"`   // 是否新建了实例（false=本机已有该网络，直接接管）
	RPCAddr   string `json:"rpc_addr"`
	Message   string `json:"message"`
}

// etNodeInfo 本机实例信息（node 命令）
type etNodeInfo struct {
	PeerID   uint64 `json:"peer_id"`
	IPv4Addr string `json:"ipv4_addr"`
	Hostname string `json:"hostname"`
	Config   string `json:"config"`
}

// etQueryNode 查询指定 RPC 的节点信息（含运行配置）
func etQueryNode(rpcAddr string) (*etNodeInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, etCli, "--output", "json", "--rpc-portal", rpcAddr, "node")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var n etNodeInfo
	if err := json.Unmarshal(out, &n); err != nil {
		return nil, err
	}
	return &n, nil
}

// etNetworkNameOf 从节点配置中提取网络名
func etNetworkNameOf(rpcAddr string) string {
	n, err := etQueryNode(rpcAddr)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(n.Config, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "network_name") {
			return strings.Trim(strings.SplitN(line, "=", 2)[1], " \"")
		}
	}
	return ""
}

// freePort 找一个空闲 TCP 端口
func freePort(start int) int {
	for port := start; port < start+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			ln.Close()
			return port
		}
	}
	return 0
}

func sudoRun(script string) error {
	cmd := exec.Command("sudo", "-n", "sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Run()
}

// VnetJoinOrCreate 加入或新建虚拟网络实例
// 1) 遍历本机已有实例，若网络名相同则直接接管（注册 RPC 地址）
// 2) 否则生成配置并用 sudo 新建 easytier-core 实例（root 权限创建 tun）
func VnetJoinOrCreate(req JoinRequest) (*JoinResult, error) {
	if req.Name == "" || req.LocalIP == "" {
		return nil, fmt.Errorf("网络名和本机虚拟IP不能为空")
	}

	// 1. 查找本机是否已有该网络
	networks, err := models.GetVnetNetworks()
	if err != nil {
		return nil, err
	}
	for _, net := range networks {
		if net.Type != "easytier-local" && net.Type != "easytier-rpc" {
			continue
		}
		if name := etNetworkNameOf(net.RPCAddr); name == req.Name {
			// 本机已有该网络：确保已注册为网络源
			if net.Enabled != 1 {
				models.UpdateVnetNetwork(&models.VnetNetwork{ID: net.ID, Name: net.Name, Type: net.Type, RPCAddr: net.RPCAddr, Subnet: net.Subnet, Enabled: 1})
			}
			return &JoinResult{Created: false, RPCAddr: net.RPCAddr, Message: fmt.Sprintf("本机已连接网络 %s，已纳入管理", req.Name)}, nil
		}
	}

	// 2. 分配 RPC 端口与配置
	port := freePort(etRpcBase)
	if port == 0 {
		return nil, fmt.Errorf("无可用 RPC 端口")
	}
	rpcAddr := fmt.Sprintf("127.0.0.1:%d", port)
	if err := os.MkdirAll(etConfigDir, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(etLogDir, 0755); err != nil {
		return nil, err
	}
	cfgPath := fmt.Sprintf("%s/%s.toml", etConfigDir, req.Name)
	logPath := fmt.Sprintf("%s/%s.log", etLogDir, req.Name)

	hostname, _ := os.Hostname()
	cfg := fmt.Sprintf("hostname = %q\nipv4 = %q\n\n[network_identity]\nnetwork_name = %q\nnetwork_secret = %q\n",
		hostname, req.LocalIP, req.Name, req.Secret)
	for _, p := range strings.Split(req.Peers, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			cfg += fmt.Sprintf("\n[[peer]]\nuri = %q\n", p)
		}
	}
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		return nil, err
	}

	// 3. sudo 新建实例（root 权限创建 tun）
	script := fmt.Sprintf("setsid nohup %s -c %s --rpc-portal 127.0.0.1:%d > %s 2>&1 < /dev/null &",
		etBin, cfgPath, port, logPath)
	if err := sudoRun(script); err != nil {
		return nil, fmt.Errorf("启动 easytier 实例失败: %v", err)
	}

	// 4. 等待实例就绪并验证网络名
	var ok bool
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		if name := etNetworkNameOf(rpcAddr); name == req.Name {
			ok = true
			break
		}
	}
	if !ok {
		return nil, fmt.Errorf("实例已启动但未能确认网络 %s（请检查日志 %s）", req.Name, logPath)
	}

	// 5. 注册网络源（本平台托管，可停止/删除）
	if err := models.CreateVnetNetwork(&models.VnetNetwork{
		Name: req.Name, Type: "easytier-local", RPCAddr: rpcAddr, Subnet: req.Subnet, Enabled: 1, Managed: 1,
	}); err != nil {
		// 注册失败（如同名源已存在）不回滚实例，返回提示
		return &JoinResult{Created: true, RPCAddr: rpcAddr, Message: fmt.Sprintf("实例已运行，但注册网络源失败: %v", err)}, nil
	}

	// 6. 开机自启：生成 systemd 服务
	if req.Autostart {
		svc := fmt.Sprintf("[Unit]\nDescription=OpsCenter VNet %s\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=%s -c %s --rpc-portal 127.0.0.1:%d\nRestart=on-failure\nUser=root\n\n[Install]\nWantedBy=multi-user.target\n",
			req.Name, etBin, cfgPath, port)
		tmp := fmt.Sprintf("/tmp/opscenter-vnet-%s.service", req.Name)
		os.WriteFile(tmp, []byte(svc), 0644)
		svcFile := fmt.Sprintf("/etc/systemd/system/opscenter-vnet-%s.service", req.Name)
		sudoRun(fmt.Sprintf("mv %s %s && systemctl daemon-reload && systemctl enable opscenter-vnet-%s.service > /dev/null 2>&1", tmp, svcFile, req.Name))
	}

	return &JoinResult{Created: true, RPCAddr: rpcAddr, Message: fmt.Sprintf("网络 %s 创建成功（本机虚拟IP %s）", req.Name, req.LocalIP)}, nil
}

// VnetStopInstance 停止本机托管实例
func VnetStopInstance(net models.VnetNetwork) error {
	if net.Managed != 1 {
		return fmt.Errorf("仅可停止由本平台创建的实例")
	}
	cfgPath := fmt.Sprintf("%s/%s.toml", etConfigDir, net.Name)
	return sudoRun(fmt.Sprintf("pkill -f 'easytier-core [-]c %s' || true", cfgPath))
}

// VnetDeleteInstance 删除实例（停止 + 删配置 + 删服务）
func VnetDeleteInstance(net models.VnetNetwork) error {
	if net.Managed != 1 {
		return fmt.Errorf("仅可删除由本平台创建的实例")
	}
	cfgPath := fmt.Sprintf("%s/%s.toml", etConfigDir, net.Name)
	if err := sudoRun(fmt.Sprintf("pkill -f 'easytier-core [-]c %s' || true; rm -f %s; rm -f /etc/systemd/system/opscenter-vnet-%s.service; systemctl daemon-reload > /dev/null 2>&1 || true", cfgPath, cfgPath, net.Name)); err != nil {
		return err
	}
	return models.DeleteVnetNetwork(net.ID)
}

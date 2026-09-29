package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"ops-center/internal/models"
)

// ===================== 虚拟网络（EasyTier 先行） =====================

// VnetPeer 虚拟网络在线节点
type VnetPeer struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Version  string `json:"version"`
}

var (
	etBinCandidates = []string{"/home/lt/ops-center/bin/easytier-cli", "easytier-cli"}
	etTimeout       = 12 * time.Second
)

// easyTierListPeers 通过本机 easytier-cli 查询指定 RPC 的在线节点
func easyTierListPeers(rpcAddr string) ([]VnetPeer, error) {
	var bin string
	for _, c := range etBinCandidates {
		if p, err := exec.LookPath(c); err == nil {
			bin = p
			break
		} else {
			// LookPath 对绝对路径已存在文件也会成功；直接判断
			bin = c
			break
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), etTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--output", "json", "--rpc-portal", rpcAddr, "route")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("easytier-cli 执行失败: %v", err)
	}
	var entries []struct {
		IPv4     string `json:"ipv4"`
		Hostname string `json:"hostname"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("解析 route 输出失败: %v", err)
	}
	seen := make(map[string]bool)
	var peers []VnetPeer
	for _, e := range entries {
		ip := strings.SplitN(e.IPv4, "/", 2)[0]
		if ip == "" || seen[e.Hostname+"/"+ip] {
			continue
		}
		seen[e.Hostname+"/"+ip] = true
		peers = append(peers, VnetPeer{Hostname: e.Hostname, IP: ip, Version: e.Version})
	}
	return peers, nil
}

// ListPeersByNetwork 按网络源查节点
func ListPeersByNetwork(n models.VnetNetwork) ([]VnetPeer, error) {
	switch n.Type {
	case "easytier-local", "easytier-rpc":
		return easyTierListPeers(n.RPCAddr)
	default:
		return nil, fmt.Errorf("暂不支持的网络类型: %s", n.Type)
	}
}

// ===================== 同步引擎 =====================

type VnetSyncResult struct {
	Time      time.Time          `json:"time"`
	OK        bool               `json:"ok"`
	Error     string             `json:"error,omitempty"`
	Changed   []string           `json:"changed"`
	Offline   []string           `json:"offline"`
	Unmatched []map[string]any   `json:"unmatched"`
	Duration  int                `json:"duration_ms"`
}

var (
	vnetSyncMu     sync.Mutex
	vnetLastSync   *VnetSyncResult
	vnetSyncRunning bool
)

// GetLastVnetSync 最近一次同步结果
func GetLastVnetSync() *VnetSyncResult {
	vnetSyncMu.Lock()
	defer vnetSyncMu.Unlock()
	return vnetLastSync
}

// VnetSyncNow 立即同步全部启用的虚拟网络
func VnetSyncNow() (*VnetSyncResult, error) {
	vnetSyncMu.Lock()
	if vnetSyncRunning {
		vnetSyncMu.Unlock()
		return GetLastVnetSync(), fmt.Errorf("同步正在进行中")
	}
	vnetSyncRunning = true
	vnetSyncMu.Unlock()

	res := &VnetSyncResult{Time: time.Now(), Changed: []string{}, Offline: []string{}, Unmatched: []map[string]any{}}
	defer func() {
		vnetSyncMu.Lock()
		vnetLastSync = res
		vnetSyncRunning = false
		vnetSyncMu.Unlock()
	}()

	networks, err := models.GetVnetNetworks()
	if err != nil {
		res.Error = err.Error()
		return res, err
	}

	// 全部虚拟信息 + 服务器名
	svMap, err := models.GetAllServerVirtual()
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	servers, err := models.GetAllServersBasic()
	if err != nil {
		res.Error = err.Error()
		return res, err
	}
	nameOf := make(map[int]string)
	for _, s := range servers {
		nameOf[s.ID] = s.Name
	}

	start := time.Now()
	for _, net := range networks {
		if net.Enabled != 1 {
			continue
		}
		peers, err := ListPeersByNetwork(net)
		if err != nil {
			log.Printf("[VNET] 网络 %s 查询失败: %v", net.Name, err)
			continue
		}
		// 匹配到的 peer 集合（用于离线判定）
		matched := make(map[int]bool)
		for serverID, sv := range svMap {
			if sv.Network != net.Name {
				continue
			}
			identifier := sv.Identifier
			// 首次绑定：没有标识但有旧 IP → 按旧 IP 反查，采纳 hostname 为标识
			if identifier == "" && sv.IP != "" {
				for _, p := range peers {
					if p.IP == sv.IP {
						identifier = p.Hostname
						sv.Identifier = identifier
						log.Printf("[VNET] 服务器 %d 首次绑定: %s (按IP %s)", serverID, identifier, sv.IP)
						break
					}
				}
			}
			if identifier == "" {
				continue
			}
			// 按标识匹配（大小写不敏感）
			var hit *VnetPeer
			for i := range peers {
				if strings.EqualFold(peers[i].Hostname, identifier) {
					hit = &peers[i]
					break
				}
			}
			if hit == nil {
				// 离线
				if sv.Online != 0 || sv.IP != "" {
					if sv.Online != 0 {
						res.Offline = append(res.Offline, fmt.Sprintf("%s(%s)", nameOf[serverID], identifier))
					}
					sv.Online = 0
					models.UpsertServerVirtual(&sv)
				}
				continue
			}
			matched[serverID] = true
			sv.Online = 1
			if sv.IP != hit.IP {
				oldIP := sv.IP
				sv.IP = hit.IP
				n, _ := models.FollowServiceAddress(serverID, oldIP, hit.IP)
				if oldIP != "" {
					res.Changed = append(res.Changed, fmt.Sprintf("%s: %s → %s (联动服务 %d 个)", nameOf[serverID], oldIP, hit.IP, n))
				}
				log.Printf("[VNET] %s 虚拟IP变化: %s → %s", nameOf[serverID], oldIP, hit.IP)
			}
			models.UpsertServerVirtual(&sv)
		}
		// 未绑定的 peer
		for _, p := range peers {
			bound := false
			for serverID, sv := range svMap {
				if sv.Network == net.Name && strings.EqualFold(sv.Identifier, p.Hostname) {
					bound = true
					_ = serverID
					break
				}
			}
			if !bound {
				inSubnet := net.Subnet == "" || strings.HasPrefix(p.IP, strings.SplitN(net.Subnet, "/", 2)[0][:strings.LastIndex(net.Subnet, ".")])
				res.Unmatched = append(res.Unmatched, map[string]any{
					"network": net.Name, "hostname": p.Hostname, "ip": p.IP,
					"suggest_subnet": inSubnet,
				})
			}
		}
	}
	res.Duration = int(time.Since(start).Milliseconds())
	res.OK = true
	return res, nil
}

// StartVnetSyncLoop 后台定时同步（60 秒）
func StartVnetSyncLoop() {
	go func() {
		// 启动后先等 10 秒再首次同步
		time.Sleep(10 * time.Second)
		for {
			if _, err := VnetSyncNow(); err != nil {
				log.Printf("[VNET] 自动同步: %v", err)
			}
			time.Sleep(60 * time.Second)
		}
	}()
	log.Printf("[VNET] 后台同步已启动（60秒/次）")
}

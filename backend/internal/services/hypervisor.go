package services

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"ops-center/internal/models"
)

// hvClient 共享客户端：厂商自签证书跳过校验，兼容旧主机低版本 TLS
var hvClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS10},
		Proxy:           http.ProxyFromEnvironment,
	},
}

// 单次清单采集的 VM 数量上限（避免大集群把接口拖死）
const hvMaxVMs = 60

// HvnServer 主机摘要
type HvnServer struct {
	Name       string   `json:"name"`
	Version    string   `json:"version"`
	State      string   `json:"state"`
	IP         string   `json:"ip"`
	CPUCount   int      `json:"cpu_count"`
	CPUUsage   float64  `json:"cpu_usage"`
	MemTotalGB float64  `json:"mem_total_gb"`
	MemUsedGB  float64  `json:"mem_used_gb"`
	MemUsage   float64  `json:"mem_usage"`
	DiskInfo   string   `json:"disk_info,omitempty"`
	UptimeSec  int64    `json:"uptime_sec"`
	Notes      []string `json:"notes,omitempty"`
}

// HvnVM 虚拟机 / 容器条目
type HvnVM struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Type  string  `json:"type"` // vm | lxc
	Node  string  `json:"node,omitempty"`
	State string  `json:"state"`
	IP    string  `json:"ip,omitempty"`
	MemGB float64 `json:"mem_gb"`
	CPUs  int     `json:"cpus"`
	OS    string  `json:"os,omitempty"`
}

// HvnNode 节点（PVE 多节点场景）
type HvnNode struct {
	Name   string    `json:"name"`
	Server HvnServer `json:"server"`
	VMs    []HvnVM   `json:"vms"`
}

// HvnResult 探测 / 清单结果
type HvnResult struct {
	HostID    int        `json:"host_id"`
	Reachable bool       `json:"reachable"`
	AuthOK    bool       `json:"auth_ok"`
	Message   string     `json:"message"`
	Server    *HvnServer `json:"server,omitempty"`
	Nodes     []HvnNode  `json:"nodes,omitempty"`
}

// HypervisorEndpoint https://ip[:port]
func HypervisorEndpoint(h *models.HypervisorHost) string {
	return "https://" + h.IP + ":" + strconv.Itoa(hvPort(h))
}

func hvPort(h *models.HypervisorHost) int {
	if h.Port > 0 {
		return h.Port
	}
	if h.Type == "pve" {
		return 8006
	}
	return 443
}

func hvBase(h *models.HypervisorHost) string {
	return "https://" + h.IP + ":" + strconv.Itoa(hvPort(h))
}

// ProbeHypervisor 连通性 + 认证 + 清单采集
func ProbeHypervisor(h *models.HypervisorHost) *HvnResult {
	res := &HvnResult{HostID: h.ID}

	port := hvPort(h)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(h.IP, strconv.Itoa(port)), 6*time.Second)
	if conn != nil {
		conn.Close()
	}
	res.Reachable = err == nil
	if err != nil {
		res.Message = fmt.Sprintf("无法连接到 %s:%d（%s）", h.IP, port, shortErr(err))
		return res
	}

	pwd := models.DecryptPassword(h.Password)

	if h.Type == "pve" {
		return probePVE(h, pwd)
	}
	return probeESXi(h, pwd)
}

// ---------- 通用工具 ----------

func shortErr(err error) string {
	s := err.Error()
	if i := strings.Index(s, "Client.Timeout"); i > 0 {
		return "连接超时"
	}
	return s
}

func hvTruncate(b []byte) string {
	s := strings.TrimSpace(string(b))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 220 {
		s = s[:220]
	}
	return s
}

func hvRound(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}

func hvPct(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return hvRound(used / total * 100)
}

func hvGB(bytes int) float64 {
	if bytes <= 0 {
		return 0
	}
	return hvRound(float64(bytes) / (1 << 30))
}

func hvHuman(bytes int) string {
	if bytes <= 0 {
		return ""
	}
	v := float64(bytes)
	for _, u := range []string{"B", "KB", "MB", "GB", "TB", "PB"} {
		if v < 1024 || u == "PB" {
			return fmt.Sprintf("%.1f %s", v, u)
		}
		v /= 1024
	}
	return ""
}

// ---------- ESXi ----------

func probeESXi(h *models.HypervisorHost, pwd string) *HvnResult {
	res := &HvnResult{HostID: h.ID, Reachable: true}
	base := hvBase(h)

	// 登录：POST /api/vcenter/session
	body, _ := json.Marshal(map[string]string{"username": h.Username, "password": pwd})
	rb, code, err := doHvRequest(http.MethodPost, base+"/api/vcenter/session", nil, body, nil)
	if err != nil {
		res.Message = "请求失败：" + shortErr(err)
		return res
	}
	if code != http.StatusOK {
		res.Message = fmt.Sprintf("认证失败（HTTP %d）：用户 %s 密码错误或该版本不支持 ESXi REST API", code, h.Username)
		return res
	}
	var sess struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(rb, &sess); err != nil || sess.Value == "" {
		res.Message = "认证响应解析失败"
		return res
	}
	res.AuthOK = true
	hdr := map[string]string{"X-Vmware-Vcenter-Session": sess.Value}

	// 主机信息
	rb, code, err = doHvRequest(http.MethodGet, base+"/api/vcenter/host", hdr, nil, nil)
	if err != nil || code != http.StatusOK {
		res.Server = &HvnServer{Name: h.Name, IP: h.IP}
		res.Message = "已认证，但读取主机信息失败"
		return res
	}
	var hv struct {
		Value struct {
			Name   string `json:"name"`
			State  string `json:"state"`
			Config struct {
				ProductVersion string `json:"product_version"`
			} `json:"config"`
			Hardware struct {
				CPUCount int `json:"cpu_count"`
				MemSize  int `json:"mem_size"`
			} `json:"hardware"`
		} `json:"value"`
	}
	if json.Unmarshal(rb, &hv) != nil {
		res.Server = &HvnServer{Name: h.Name, IP: h.IP}
		res.Message = "主机信息解析失败"
		return res
	}

	srv := &HvnServer{
		Name:       hv.Value.Name,
		Version:    hv.Value.Config.ProductVersion,
		State:      hv.Value.State,
		IP:         h.IP,
		CPUCount:   hv.Value.Hardware.CPUCount,
		MemTotalGB: hvGB(hv.Value.Hardware.MemSize),
	}

	// 资源统计：cpu.usage.usage(%) 与 memory.usage.usage(KB)
	rb, code, err = doHvRequest(http.MethodGet, base+"/api/vcenter/host/stats", hdr, nil, nil)
	if err == nil && code == http.StatusOK {
		var stats struct {
			Value []struct {
				ID    string  `json:"id"`
				Value float64 `json:"value"`
			} `json:"value"`
		}
		if json.Unmarshal(rb, &stats) == nil {
			for _, s := range stats.Value {
				switch s.ID {
				case "cpu.usage.usage":
					srv.CPUUsage = hvRound(s.Value)
				case "memory.usage.usage":
					srv.MemUsedGB = hvRound(s.Value / (1024 * 1024))
				}
			}
		}
		srv.MemUsage = hvPct(srv.MemUsedGB, srv.MemTotalGB)
	}
	res.Server = srv

	// 虚拟机清单
	vms, verr := esxiVMList(base, hdr)
	if verr != "" {
		srv.Notes = append(srv.Notes, "虚拟机清单读取失败："+verr)
	}
	res.Nodes = []HvnNode{{Name: srv.Name, Server: *srv, VMs: vms}}
	return res
}

func esxiVMList(base string, hdr map[string]string) ([]HvnVM, string) {
	rb, code, err := doHvRequest(http.MethodGet, base+"/api/vcenter/vm", hdr, nil, nil)
	if err != nil {
		return nil, shortErr(err)
	}
	if code != http.StatusOK {
		return nil, fmt.Sprintf("HTTP %d", code)
	}
	var ids struct {
		Value []string `json:"value"`
	}
	if json.Unmarshal(rb, &ids) != nil || len(ids.Value) == 0 {
		return []HvnVM{}, ""
	}
	if len(ids.Value) > hvMaxVMs {
		ids.Value = ids.Value[:hvMaxVMs]
	}

	vms := make([]HvnVM, 0, len(ids.Value))
	type item struct {
		vm  HvnVM
		err string
	}
	resChan := make(chan item, len(ids.Value))
	var wg sync.WaitGroup
	for _, id := range ids.Value {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			resChan <- item{vm: esxiVMDetail(base, hdr, id)}
		}(id)
	}
	wg.Wait()
	close(resChan)
	for it := range resChan {
		vms = append(vms, it.vm)
	}
	return vms, ""
}

func esxiVMDetail(base string, hdr map[string]string, id string) HvnVM {
	vm := HvnVM{ID: id, Name: id, Type: "vm"}

	rb, code, err := doHvRequest(http.MethodGet, base+"/api/vcenter/vm/"+id+"/guest", hdr, nil, nil)
	if err == nil && code == http.StatusOK {
		var g struct {
			Value struct {
				GuestName   string `json:"guest_name"`
				GuestState  string `json:"guest_state"`
				GuestID     string `json:"guest_id"`
				IPAddresses []struct {
					IPAddress   string `json:"ip_address"`
					AddressType string `json:"address_type"`
				} `json:"ip_addresses"`
			} `json:"value"`
		}
		if json.Unmarshal(rb, &g) == nil {
			if g.Value.GuestName != "" {
				vm.Name = g.Value.GuestName
			}
			vm.OS = g.Value.GuestID
			for _, ip := range g.Value.IPAddresses {
				if ip.IPAddress != "" {
					vm.IP = ip.IPAddress
					break
				}
			}
			switch g.Value.GuestState {
			case "GUEST_STATE_RUNNING":
				vm.State = "running"
			case "GUEST_STATE_STOPPED":
				vm.State = "stopped"
			default:
				vm.State = strings.TrimPrefix(g.Value.GuestState, "GUEST_STATE_")
			}
		}
	}

	rb, code, err = doHvRequest(http.MethodGet, base+"/api/vcenter/vm/"+id+"/power", hdr, nil, nil)
	if err == nil && code == http.StatusOK {
		var p struct {
			Value string `json:"value"`
		}
		if json.Unmarshal(rb, &p) == nil && p.Value != "" {
			switch p.Value {
			case "POWERED_ON":
				vm.State = "running"
			case "POWERED_OFF":
				vm.State = "stopped"
			default:
				vm.State = strings.TrimPrefix(p.Value, "POWER_")
			}
		}
	}

	rb, code, err = doHvRequest(http.MethodGet, base+"/api/vcenter/vm/"+id, hdr, nil, nil)
	if err == nil && code == http.StatusOK {
		var d struct {
			Value struct {
				Hardware struct {
					CPUCount int `json:"cpu_count"`
					MemSize  int `json:"mem_size"`
				} `json:"hardware"`
			} `json:"value"`
		}
		if json.Unmarshal(rb, &d) == nil {
			vm.CPUs = d.Value.Hardware.CPUCount
			vm.MemGB = hvGB(d.Value.Hardware.MemSize)
		}
	}
	return vm
}

// ---------- Proxmox VE ----------

func probePVE(h *models.HypervisorHost, pwd string) *HvnResult {
	res := &HvnResult{HostID: h.ID, Reachable: true}
	base := hvBase(h)

	// 节点列表（无需认证）
	rb, code, _, err := doPveRequest(http.MethodGet, base+"/api2/json", "", nil)
	if err != nil {
		res.Reachable = false
		res.Message = "请求失败：" + shortErr(err)
		return res
	}
	if code != http.StatusOK {
		res.Reachable = false
		res.Message = fmt.Sprintf("PVE 无响应（HTTP %d）：确认端口与地址是否正确", code)
		return res
	}
	var nodeNames []string
	if json.Unmarshal(rb, &nodeNames) != nil {
		res.Message = "PVE 响应解析失败"
		return res
	}
	if len(nodeNames) == 0 {
		res.Message = "未获取到节点信息"
		return res
	}

	// 登录：POST /api2/json/access/ticket
	user := h.Username
	realm := h.Realm
	if realm == "" {
		realm = "pam"
	}
	if !strings.Contains(user, "@") {
		user = user + "@" + realm
	}
	form := url.Values{}
	form.Set("username", user)
	form.Set("password", pwd)
	rb, code, hdr, err := doPveRequest(http.MethodPost, base+"/api2/json/access/ticket", "", form)
	if err != nil {
		res.Message = "登录请求失败：" + shortErr(err)
		return res
	}
	if code != http.StatusCreated {
		res.Message = fmt.Sprintf("认证失败（HTTP %d）：%s", code, hvTruncate(rb))
		return res
	}
	var login struct {
		Data struct {
			Ticket string `json:"ticket"`
		} `json:"data"`
	}
	if json.Unmarshal(rb, &login) != nil || login.Data.Ticket == "" {
		res.Message = "登录响应解析失败"
		return res
	}
	cookie := "PVEAuthCookie=" + login.Data.Ticket
	for _, sc := range hdr.Values("Set-Cookie") {
		if strings.Contains(sc, "PVEAuthCookie") {
			continue
		}
		cookie += "; " + strings.Split(sc, ";")[0]
	}
	res.AuthOK = true

	for _, n := range nodeNames {
		res.Nodes = append(res.Nodes, pveNodeDetail(base, cookie, n))
	}
	return res
}

func pveNodeDetail(base, cookie, node string) HvnNode {
	out := HvnNode{Name: node}

	rb, code, _, err := doPveRequest(http.MethodGet, base+"/api2/json/nodes/"+url.PathEscape(node)+"/status", cookie, nil)
	if err == nil && code == http.StatusOK {
		var s struct {
			Data struct {
				Status       string  `json:"status"`
				Version      string  `json:"version"`
				PveVersion   string  `json:"pveversion"`
				Release      string  `json:"release"`
				Type         string  `json:"type"`
				Arch         string  `json:"arch"`
				Manufacturer string  `json:"manufacturer"`
				Product      string  `json:"product"`
				Hostname     string  `json:"hostname"`
				ID           string  `json:"id"`
				Uptime       int64   `json:"uptime"`
				Cores        int     `json:"cores"`
				Cpus         int     `json:"cpus"`
				CPUUnits     float64 `json:"cpuunits"`
				Memory       struct {
					Total float64 `json:"total"`
					Used  float64 `json:"used"`
				} `json:"memory"`
				Swap struct {
					Total float64 `json:"total"`
					Used  float64 `json:"used"`
				} `json:"swap"`
				Disk struct {
					Total float64 `json:"total"`
					Used  float64 `json:"used"`
					Free  float64 `json:"free"`
				} `json:"disk"`
				CPU []struct {
					ModelName string  `json:"model_name"`
					SpeedGHz  float64 `json:"speed_GHz"`
				} `json:"cpu"`
			} `json:"data"`
		}
		if json.Unmarshal(rb, &s) == nil {
			d := s.Data
			cpucount := d.Cores
			if cpucount == 0 {
				cpucount = d.Cpus
			}
			cpuusage := 0.0
			cpuinfo := ""
			if len(d.CPU) > 0 {
				cpuinfo = fmt.Sprintf("%s @ %.1f GHz", d.CPU[0].ModelName, d.CPU[0].SpeedGHz)
			}
			notes := []string{}
			if cpuinfo != "" {
				notes = append(notes, cpuinfo)
			}
			if d.Manufacturer != "" || d.Product != "" {
				notes = append(notes, strings.TrimSpace(d.Manufacturer+" "+d.Product))
			}
			if d.Swap.Total > 0 {
				notes = append(notes, fmt.Sprintf("交换空间 %s/%s", hvHuman(int(d.Swap.Used)), hvHuman(int(d.Swap.Total))))
			}
			pveVer := d.PveVersion
			if pveVer == "" {
				pveVer = d.Version
			}
			if pveVer == "" && d.Release != "" {
				pveVer = d.Release
			}
			out.Server = HvnServer{
				Name:       node,
				Version:    pveVer,
				State:      d.Status,
				CPUCount:   cpucount,
				CPUUsage:   cpuusage,
				MemTotalGB: hvGB(int(d.Memory.Total)),
				MemUsedGB:  hvGB(int(d.Memory.Used)),
				MemUsage:   hvPct(d.Memory.Used, d.Memory.Total),
				DiskInfo: fmt.Sprintf("磁盘 %s/%s（可用 %s）",
					hvHuman(int(d.Disk.Used)), hvHuman(int(d.Disk.Total)), hvHuman(int(d.Disk.Free))),
				UptimeSec: d.Uptime,
				Notes:     notes,
			}
		}
	}

	out.VMs = append(out.VMs, pveVMList(base, cookie, node, "qemu", "vm")...)
	out.VMs = append(out.VMs, pveVMList(base, cookie, node, "lxc", "lxc")...)
	return out
}

func pveVMList(base, cookie, node, kind, vtype string) []HvnVM {
	rb, code, _, err := doPveRequest(http.MethodGet, base+"/api2/json/nodes/"+url.PathEscape(node)+"/"+kind, cookie, nil)
	if err != nil || code != http.StatusOK {
		return nil
	}
	var r struct {
		Data []struct {
			Name     string  `json:"name"`
			Status   string  `json:"status"`
			VMID     int     `json:"vmid"`
			CID      int     `json:"cid"`
			MaxMem   float64 `json:"maxmem"`
			Mem      float64 `json:"mem"`
			Cores    int     `json:"cores"`
			Sockets  int     `json:"sockets"`
			CPU      float64 `json:"cpu"`
			CPUUnits int     `json:"cpuunits"`
			OSType   string  `json:"ostype"`
			Pool     string  `json:"pool"`
		} `json:"data"`
	}
	if json.Unmarshal(rb, &r) != nil {
		return nil
	}
	if len(r.Data) > hvMaxVMs {
		r.Data = r.Data[:hvMaxVMs]
	}
	vms := make([]HvnVM, 0, len(r.Data))
	for _, v := range r.Data {
		id := strconv.Itoa(v.VMID)
		if id == "0" {
			id = strconv.Itoa(v.CID)
		}
		state := v.Status
		if state == "" {
			state = "unknown"
		}
		vms = append(vms, HvnVM{
			ID:    id,
			Name:  v.Name,
			Type:  vtype,
			Node:  node,
			State: state,
			MemGB: hvGB(int(v.MaxMem)),
			CPUs:  v.Cores,
			OS:    v.OSType,
		})
	}
	return vms
}

// ---------- 底层请求 ----------

func doHvRequest(method, rawURL string, hdr map[string]string, body []byte, form url.Values) ([]byte, int, error) {
	rb, code, _, err := doHvRequestFull(method, rawURL, hdr, body, form)
	return rb, code, err
}

func doHvRequestFull(method, rawURL string, hdr map[string]string, body []byte, form url.Values) ([]byte, int, http.Header, error) {
	var reader io.Reader
	var ct string
	if form != nil {
		reader = strings.NewReader(form.Encode())
		ct = "application/x-www-form-urlencoded"
	} else if len(body) > 0 {
		reader = bytes.NewReader(body)
		ct = "application/json"
	}
	req, err := http.NewRequest(method, rawURL, reader)
	if err != nil {
		return nil, 0, nil, err
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "ops-center-hypervisor")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := hvClient.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	return rb, resp.StatusCode, resp.Header, nil
}

func doPveRequest(method, rawURL string, cookie string, form url.Values) ([]byte, int, http.Header, error) {
	hdr := map[string]string{}
	if cookie != "" {
		hdr["Cookie"] = cookie
	}
	rb, code, rh, err := doHvRequestFull(method, rawURL, hdr, nil, form)
	if err != nil {
		return nil, 0, nil, err
	}
	return rb, code, rh, nil
}

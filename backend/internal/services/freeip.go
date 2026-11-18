package services

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"net"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"
)

// FreeIP 网段内一个候选 IP 的探测结果
type FreeIP struct {
	IP         string `json:"ip"`
	Subnet     string `json:"subnet"`
	Online     bool   `json:"online"`
	MAC        string `json:"mac,omitempty"`
	Registered bool   `json:"registered"` // 已登记在服务器管理中
	ServerID   int    `json:"server_id,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	Status     string `json:"status,omitempty"`
	Services   int    `json:"services"` // 已添加的服务数
}

// FreeIPScan 一次网段扫描的结果
type FreeIPScan struct {
	Subnets    []string `json:"subnets"`
	Candidates int      `json:"candidates"` // 实际探测的 IP 数（不含已登记）
	Skipped    int      `json:"skipped"`    // 因规模上限未探测的 IP 数
	Free       []FreeIP `json:"free"`       // 在线且未登记
	Known      []FreeIP `json:"known"`      // 已登记（来自数据库，供对照）
	DurationMs int64    `json:"duration_ms"`
	ScannedAt  string   `json:"scanned_at"`
	Cached     bool     `json:"cached"`
}

// 单个网段最多探测多少个地址，避免误填 /8 把服务器打满
const freeIPMaxPerSubnet = 4096

// 扫描结果缓存时长（秒）。同一页面反复打开不必重复发包
const freeIPCacheSeconds = 90

var (
	freeIPMu    sync.Mutex
	freeIPCache *FreeIPScan
	freeIPAt    time.Time
)

// ListFreeIPSubnets 可扫描的网段：服务器管理中已有 IP 所在的 /24，
// 加上虚拟网络里显式声明的子网。
func ListFreeIPSubnets(db *sql.DB) ([]string, error) {
	set := map[string]bool{}

	// servers.ip 与 server_virtual.ip 各自归到 /24
	for _, q := range []string{"SELECT ip FROM servers", "SELECT ip FROM server_virtual"} {
		rows, err := db.Query(q)
		if err != nil {
			continue
		}
		for rows.Next() {
			var ip string
			if err := rows.Scan(&ip); err == nil {
				if s := slash24(ip); s != "" {
					set[s] = true
				}
			}
		}
		rows.Close()
	}

	// vnet_networks.subnet 原样收录（可能是任意前缀）
	rows, err := db.Query("SELECT subnet FROM vnet_networks WHERE enabled = 1 AND subnet != ''")
	if err == nil {
		for rows.Next() {
			var sub string
			if err := rows.Scan(&sub); err == nil && sub != "" {
				if _, _, e := net.ParseCIDR(sub); e == nil {
					set[sub] = true
				}
			}
		}
		rows.Close()
	}

	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out, nil
}

// GetFreeIPs 扫描网段，列出在线但尚未加入服务器管理的 IP。
// subnets 为空时扫描 ListFreeIPSubnets 给出的全部网段。
func GetFreeIPs(db *sql.DB, subnets []string, force bool) (*FreeIPScan, error) {
	freeIPMu.Lock()
	defer freeIPMu.Unlock()

	if len(subnets) == 0 {
		auto, err := ListFreeIPSubnets(db)
		if err != nil {
			return nil, err
		}
		subnets = auto
	}
	if len(subnets) == 0 {
		return &FreeIPScan{Subnets: []string{}, Free: []FreeIP{}, Known: []FreeIP{}}, nil
	}
	sort.Strings(subnets)
	key := strings.Join(subnets, ",")

	// 命中缓存
	if !force && freeIPCache != nil && time.Since(freeIPAt) < freeIPCacheSeconds*time.Second &&
		strings.Join(freeIPCache.Subnets, ",") == key {
		c := *freeIPCache
		c.Cached = true
		return &c, nil
	}

	start := time.Now()

	// 已登记的 IP：servers 表 + 该 IP 的服务数
	registered := map[string]regInfo{}
	rows, err := db.Query("SELECT id, ip, name, status FROM servers")
	if err != nil {
		return nil, fmt.Errorf("读取服务器列表失败: %w", err)
	}
	for rows.Next() {
		var id int
		var ip, name, status string
		if err := rows.Scan(&id, &ip, &name, &status); err == nil {
			registered[ip] = regInfo{id: id, name: name, status: status}
		}
	}
	rows.Close()

	// 每台机器已登记的服务数
	svcCount := map[int]int{}
	if srows, e := db.Query("SELECT server_id, COUNT(*) FROM server_services GROUP BY server_id"); e == nil {
		for srows.Next() {
			var sid, cnt int
			if e := srows.Scan(&sid, &cnt); e == nil {
				svcCount[sid] = cnt
			}
		}
		srows.Close()
	}

	// 枚举候选：网段内所有排除已登记后的地址
	type job struct {
		ip     string
		subnet string
	}
	var jobs []job
	skipped := 0
	for _, sub := range subnets {
		ips, skip, err := enumCIDR(sub)
		if err != nil {
			continue // 无效网段直接跳过，不让整个扫描失败
		}
		skipped += skip
		for _, ip := range ips {
			if _, done := registered[ip]; done {
				continue
			}
			jobs = append(jobs, job{ip: ip, subnet: sub})
		}
	}

	// 并发 ping
	online := make([]bool, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 128)
	for i := range jobs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			online[i] = pingIPFast(jobs[i].ip)
		}(i)
	}
	wg.Wait()

	macs := neighTable()

	res := &FreeIPScan{
		Subnets:    subnets,
		Candidates: len(jobs),
		Skipped:    skipped,
		Free:       []FreeIP{},
		Known:      []FreeIP{},
	}
	for i, j := range jobs {
		if !online[i] {
			continue
		}
		res.Free = append(res.Free, FreeIP{
			IP:      j.ip,
			Subnet:  j.subnet,
			Online:  true,
			MAC:     macs[j.ip],
			Services: 0,
		})
	}
	sort.Slice(res.Free, func(a, b int) bool {
		return ipLess(res.Free[a].IP, res.Free[b].IP)
	})

	// 已登记的部分从数据库给出，不重复探测
	for ip, r := range registered {
		if !inSubnets(ip, subnets) {
			continue
		}
		res.Known = append(res.Known, FreeIP{
			IP:         ip,
			Registered: true,
			ServerID:   r.id,
			ServerName: r.name,
			Status:     r.status,
			Services:   svcCount[r.id],
			Online:     r.status != "offline",
		})
	}
	sort.Slice(res.Known, func(a, b int) bool {
		return ipLess(res.Known[a].IP, res.Known[b].IP)
	})

	res.DurationMs = time.Since(start).Milliseconds()
	res.ScannedAt = time.Now().Format("2006-01-02 15:04:05")

	freeIPCache = res
	freeIPAt = time.Now()
	return res, nil
}

type regInfo struct {
	id     int
	name   string
	status string
}

// AddFreeIPsAsServers 把空闲 IP 加入服务器管理。凭据留空由用户后续补，
// 状态标为 pending 以便和已纳管的机器区分。
func AddFreeIPsAsServers(db *sql.DB, ips []string, group string, userID int) (added int, skipped int, err error) {
	tx, e := db.Begin()
	if e != nil {
		return 0, 0, e
	}
	defer tx.Rollback()

	existing := map[string]bool{}
	rows, e := db.Query("SELECT ip FROM servers")
	if e != nil {
		return 0, 0, e
	}
	for rows.Next() {
		var ip string
		if e := rows.Scan(&ip); e == nil {
			existing[ip] = true
		}
	}
	rows.Close()

	stmt, e := tx.Prepare(`
		INSERT INTO servers (name, ip, port, username, password, os, status, auth_type, tags, group_name, created_by)
		VALUES (?, ?, 22, '', '', '', 'pending', 'password', '', ?, ?)`)
	if e != nil {
		return 0, 0, e
	}
	defer stmt.Close()

	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		if ip == "" || net.ParseIP(ip) == nil {
			skipped++
			continue
		}
		if existing[ip] {
			skipped++
			continue
		}
		if _, e := stmt.Exec(ip, ip, group, userID); e != nil {
			return 0, 0, fmt.Errorf("添加 %s 失败: %w", ip, e)
		}
		existing[ip] = true
		added++
	}

	if e := tx.Commit(); e != nil {
		return 0, 0, e
	}
	// 列表变了，缓存作废
	freeIPCache = nil
	return added, skipped, nil
}

// slash24 把一个 IPv4 地址归到它所在的 /24，非法输入返回空串
func slash24(ip string) string {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return ""
	}
	v4 := parsed.To4()
	if v4 == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.0/24", v4[0], v4[1], v4[2])
}

func inSubnets(ip string, subnets []string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, sub := range subnets {
		_, n, err := net.ParseCIDR(sub)
		if err != nil {
			continue
		}
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

// enumCIDR 展开网段为可探测地址列表（点对网点跳过网络/广播地址）。
// 返回第二个值是因规模上限被截掉的数量。
func enumCIDR(cidr string) ([]string, int, error) {
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, 0, err
	}
	v4 := n.IP.To4()
	if v4 == nil {
		return nil, 0, fmt.Errorf("仅支持 IPv4 网段: %s", cidr)
	}
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return nil, 0, fmt.Errorf("非法子网掩码: %s", cidr)
	}
	total := 1 << uint(bits-ones)
	limit := total
	skipped := 0
	if limit > freeIPMaxPerSubnet {
		skipped = limit - freeIPMaxPerSubnet
		limit = freeIPMaxPerSubnet
	}
	base := binary.BigEndian.Uint32(v4)
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		if ones < 31 && (i == 0 || i == total-1) {
			continue // 网络地址和广播地址
		}
		out = append(out, u32ToIP(base+uint32(i)))
	}
	return out, skipped, nil
}

func u32ToIP(v uint32) string {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return net.IP(b).String()
}

// ipLess 按数值比较两个 IPv4，让列表按 IP 顺序而不是字典序排列
func ipLess(a, b string) bool {
	ia, ib := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	return binary.BigEndian.Uint32(ia) < binary.BigEndian.Uint32(ib)
}

// pingIPFast 单次快速 ping，1 秒超时。扫整个网段时这个值决定总耗时
func pingIPFast(ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "1", ip)
	return cmd.Run() == nil
}

// neighTable 读一次内核邻居表，给在线 IP 补上 MAC
func neighTable() map[string]string {
	m := map[string]string{}
	out, err := exec.Command("ip", "neigh", "show").Output()
	if err != nil {
		return m
	}
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || fields[2] != "lladdr" {
			continue
		}
		if ip := net.ParseIP(fields[0]); ip != nil && ip.To4() != nil {
			m[fields[0]] = fields[3]
		}
	}
	return m
}

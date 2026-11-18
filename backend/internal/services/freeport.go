package services

import (
	"database/sql"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"ops-center/internal/models"

	"golang.org/x/crypto/ssh"
)

// OpenPort 目标机上的一个 TCP 监听端口
type OpenPort struct {
	Port        int    `json:"port"`
	Bind        string `json:"bind"`
	Loopback    bool   `json:"loopback"`
	Process     string `json:"process,omitempty"`
	PID         int    `json:"pid,omitempty"`
	Method      string `json:"method"`
	ProfileName string `json:"profile_name,omitempty"`
	Added       bool   `json:"added"`
	ServiceID   int    `json:"service_id,omitempty"`
	ServiceName string `json:"service_name,omitempty"`
}

// PortScan 一台服务器的端口扫描结果
type PortScan struct {
	ServerID   int        `json:"server_id"`
	IP         string     `json:"ip"`
	Source     string     `json:"source"`
	Ports      []OpenPort `json:"ports"`
	DurationMs int64      `json:"duration_ms"`
	ScannedAt  string     `json:"scanned_at"`
	Message    string     `json:"message,omitempty"`
}

var portMethodTable = map[int]string{
	20: "FTP", 21: "FTP", 22: "SSH", 23: "TELNET",
	25: "SMTP", 53: "API", 80: "WEB", 110: "POP3", 143: "IMAP",
	443: "HTTPS", 465: "SMTP", 587: "SMTP", 993: "IMAPS", 995: "POP3S",
	1433: "MSSQL", 1521: "Oracle", 2049: "NFS", 3000: "API", 3128: "API",
	3306: "MySQL", 3389: "RDP", 4317: "FLUENTD", 5000: "API", 5001: "HTTPS",
	5432: "PostgreSQL", 5601: "KIBANA", 5900: "VNC", 5901: "VNC", 5902: "VNC",
	6379: "Redis", 6443: "HTTPS", 8000: "WEB", 8006: "HTTPS", 8080: "WEB",
	8081: "WEB", 8443: "HTTPS", 8888: "WEB", 9000: "API", 9090: "API",
	9100: "PRINTER", 9200: "API", 9300: "API", 11211: "Memcached",
	15672: "RABBITMQ", 27017: "MongoDB", 50000: "API",
}

func ScanServerPorts(db *sql.DB, serverID int) (*PortScan, error) {
	
	// Use a custom query to avoid the NULL private_key issue
	var srv models.Server
	err := db.QueryRow(`
		SELECT id, name, ip, port, username, password, COALESCE(private_key, ''), os, status, auth_type, tags, 
		       COALESCE(group_name, ''), created_by, created_at, updated_at 
		FROM servers WHERE id = ?`, serverID).Scan(
		&srv.ID, &srv.Name, &srv.IP, &srv.Port, &srv.Username, &srv.Password, &srv.PrivateKey, 
		&srv.OS, &srv.Status, &srv.AuthType, &srv.Tags, &srv.GroupName, &srv.CreatedBy, &srv.CreatedAt, &srv.UpdatedAt)
	srv.Password = models.DecryptPassword(srv.Password)
	
	if err != nil {
		return nil, fmt.Errorf("服务器不存在: %d", serverID)
	}

	start := time.Now()
	client, err := GetSSHClient(&srv)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接 %s 失败: %w", srv.IP, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer session.Close()

	cmd := `echo '@@SS'; command -v ss >/dev/null 2>&1 && ss -H -lntp 2>/dev/null;` +
		` echo '@@NETSTAT'; command -v netstat >/dev/null 2>&1 && netstat -anp 2>/dev/null | grep -i '^tcp';` +
		` echo '@@PROC'; [ -r /proc/net/tcp ] && cat /proc/net/tcp`
	out, err := session.CombinedOutput(cmd)
	if err != nil {
		if _, ok := err.(*ssh.ExitError); !ok {
			return nil, fmt.Errorf("执行端口查询失败: %w", err)
		}
	}

	raw := string(out)
	ports, source := parseListenPorts(raw)

	res := &PortScan{
		ServerID:   serverID,
		IP:         srv.IP,
		Source:     source,
		Ports:      ports,
		DurationMs: time.Since(start).Milliseconds(),
		ScannedAt:  time.Now().Format("2006-01-02 15:04:05"),
	}
	if len(ports) == 0 {
		res.Source = "none"
		res.Message = "没有取到任何监听端口。可能这台机器上没有 ss / netstat，" +
			"或当前用户无权读取进程信息。原始输出：" + hvTruncate([]byte(raw))
	}

	enrichPorts(db, serverID, res.Ports)
	return res, nil
}

func parseListenPorts(raw string) ([]OpenPort, string) {
	byPort := map[int]*OpenPort{}

	addLine := func(local, proto, process string, pid int) {
		port, bind, ok := splitPortBind(local)
		if !ok || proto != "tcp" {
			return
		}
		cur, exists := byPort[port]
		if !exists {
			byPort[port] = &OpenPort{Port: port, Bind: bind, Loopback: isLoopback(bind), Process: process, PID: pid}
			return
		}
		if cur.Loopback && !isLoopback(bind) {
			cur.Bind, cur.Loopback = bind, isLoopback(bind)
		}
		if cur.Process == "" && process != "" {
			cur.Process, cur.PID = process, pid
		}
	}

	section := ""
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		switch t {
		case "@@SS":
			section = "ss"
			continue
		case "@@NETSTAT":
			section = "netstat"
			continue
		case "@@PROC":
			section = "proc"
			continue
		}

		switch section {
		case "ss", "netstat":
			f := strings.Fields(t)
			if len(f) < 4 {
				continue
			}
			proto := "tcp"
			if section == "netstat" {
				if !strings.HasPrefix(f[0], "tcp") {
					continue
				}
				if len(f) < 6 || !strings.EqualFold(f[5], "LISTEN") {
					continue
				}
			} else if !strings.EqualFold(f[0], "LISTEN") {
				continue
			}
			name, pid := parseProcessInfo(f, section)
			addLine(f[3], proto, name, pid)
		case "proc":
			if strings.HasPrefix(t, "sl") {
				continue
			}
			f := strings.Fields(t)
			if len(f) < 4 || strings.ToUpper(f[3]) != "0A" {
				continue
			}
			port, bind, ok := splitHexAddrPort(f[1])
			if !ok {
				continue
			}
			cur, exists := byPort[port]
			if !exists {
				byPort[port] = &OpenPort{Port: port, Bind: bind, Loopback: isLoopback(bind)}
			} else if cur.Loopback && !isLoopback(bind) {
				cur.Bind, cur.Loopback = bind, isLoopback(bind)
			}
		}
	}

	source := "none"
	if strings.Contains(raw, "@@SS") {
		if _, ok := byPort[0]; !ok {
		}
	}
	ports := make([]OpenPort, 0, len(byPort))
	for _, p := range byPort {
		if source == "none" {
			source = "detected"
		}
		p.Method = methodForPort(p.Port)
		ports = append(ports, *p)
	}
	if len(ports) > 0 {
		switch {
		case strings.Contains(raw, "@@SS"):
			if firstSectionHasData(raw, "@@SS", "@@NETSTAT") {
				source = "ss"
			} else if firstSectionHasData(raw, "@@NETSTAT", "@@PROC") {
				source = "netstat"
			} else {
				source = "proc"
			}
		}
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
	return ports, source
}

func firstSectionHasData(raw, start, end string) bool {
	i := strings.Index(raw, start)
	if i < 0 {
		return false
	}
	j := strings.Index(raw[i:], end)
	if j < 0 {
		j = len(raw) - i
	}
	block := raw[i+len(start) : i+j]
	for _, l := range strings.Split(block, "\n") {
		if strings.TrimSpace(l) != "" {
			return true
		}
	}
	return false
}

func splitPortBind(s string) (int, string, bool) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return 0, "", false
	}
	port, err := strconv.Atoi(s[idx+1:])
	if err != nil || port <= 0 || port > 65535 {
		return 0, "", false
	}
	bind := s[:idx]
	if bind == "::" || bind == "*" {
		bind = "[::]"
	}
	return port, bind, true
}

func splitHexAddrPort(s string) (int, string, bool) {
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return 0, "", false
	}
	portVal, err := strconv.ParseUint(s[idx+1:], 16, 16)
	if err != nil {
		return 0, "", false
	}
	hex := s[:idx]
	if len(hex) != 8 {
		return 0, "", false
	}
	var b [4]byte
	for i := 0; i < 4; i++ {
		v, e := strconv.ParseUint(hex[i*2:i*2+2], 16, 8)
		if e != nil {
			return 0, "", false
		}
		b[3-i] = byte(v)
	}
	return int(portVal), net.IP(b[:]).String(), true
}

func parseProcessInfo(f []string, section string) (string, int) {
	if section == "ss" {
		for _, tok := range f[4:] {
			if !strings.HasPrefix(tok, "users:") {
				continue
			}
			name := ""
			if q := strings.Index(tok, "\""); q >= 0 {
				if e := strings.Index(tok[q+1:], "\""); e > 0 {
					name = tok[q+1 : q+1+e]
				}
			}
			pid := 0
			if q := strings.Index(tok, "pid="); q >= 0 {
				rest := tok[q+4:]
				if e := strings.IndexAny(rest, ",)"); e > 0 {
					pid, _ = strconv.Atoi(rest[:e])
				}
			}
			return name, pid
		}
		return "", 0
	}
	if len(f) >= 7 {
		last := f[6]
		if slash := strings.Index(last, "/"); slash > 0 {
				pid, _ := strconv.Atoi(last[:slash])
				prog := last[slash+1:]
				if prog == "-" {
					prog = ""
				}
				return prog, pid
			}
		}
		return "", 0
	}

	func isLoopback(bind string) bool {
		if bind == "[::1]" {
			return true
		}
		ip := net.ParseIP(bind)
		return ip != nil && ip.IsLoopback()
	}

	func methodForPort(port int) string {
		if m, ok := portMethodTable[port]; ok {
			return m
		}
		return "其他"
	}

	func enrichPorts(db *sql.DB, serverID int, ports []OpenPort) {
		profiles := map[int]string{}
		if rows, e := db.Query("SELECT port, name FROM port_profiles"); e == nil {
			for rows.Next() {
				var p int
				var n string
				if e := rows.Scan(&p, &n); e == nil {
					profiles[p] = n
				}
			}
			rows.Close()
		}

		existing := map[int]struct {
			id   int
			name string
		}{}
		if rows, e := db.Query("SELECT id, COALESCE(port, 0), name FROM server_services WHERE server_id = ?", serverID); e == nil {
			for rows.Next() {
				var id, port int
				var name string
				if e := rows.Scan(&id, &port, &name); e == nil && port > 0 {
					existing[port] = struct {
						id   int
						name string
					}{id, name}
				}
			}
			rows.Close()
		}

		for i := range ports {
			ports[i].ProfileName = profiles[ports[i].Port]
			if ex, ok := existing[ports[i].Port]; ok {
				ports[i].Added = true
				ports[i].ServiceID = ex.id
				ports[i].ServiceName = ex.name
			}
		}
	}

	func AddServerPorts(db *sql.DB, serverID int, ports []int) (added int, skipped int, err error) {
		var ip string
		if e := db.QueryRow("SELECT ip FROM servers WHERE id = ?", serverID).Scan(&ip); e != nil {
			return 0, 0, fmt.Errorf("服务器不存在: %d", serverID)
		}

		existing := map[int]bool{}
		rows, e := db.Query("SELECT COALESCE(port, 0) FROM server_services WHERE server_id = ?", serverID)
		if e != nil {
			return 0, 0, e
		}
		for rows.Next() {
			var p int
			if e := rows.Scan(&p); e == nil {
				existing[p] = true
			}
		}
		rows.Close()

		profiles := map[int]string{}
		if prow, e := db.Query("SELECT port, name FROM port_profiles"); e == nil {
			for prow.Next() {
				var p int
				var n string
				if e := prow.Scan(&p, &n); e == nil {
					profiles[p] = n
				}
			}
			prow.Close()
		}

		tx, e := db.Begin()
		if e != nil {
			return 0, 0, e
		}
		defer tx.Rollback()

		stmt, e := tx.Prepare(`
			INSERT INTO server_services (server_id, name, access_method, address, port, username, password, status, description)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'running', '端口扫描自动添加')`)
		if e != nil {
			return 0, 0, e
		}
		defer stmt.Close()

		seen := map[int]bool{}
		for _, p := range ports {
			if p <= 0 || p > 65535 || seen[p] {
				skipped++
				continue
			}
			seen[p] = true
			if existing[p] {
				skipped++
				continue
			}
			method := methodForPort(p)
			name := profiles[p]
			if name == "" {
				name = fmt.Sprintf("端口 %d", p)
			}
			if _, e := stmt.Exec(serverID, name, method, ip, p, "", ""); e != nil {
				return 0, 0, e
			}
			existing[p] = true
			added++
		}

		if e := tx.Commit(); e != nil {
			return 0, 0, e
		}
		return added, skipped, nil
	}



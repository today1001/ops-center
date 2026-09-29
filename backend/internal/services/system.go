package services

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Component 系统组件
type Component struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"` // core / service
	Description string `json:"description"`
	Status      string `json:"status"` // running / stopped / failed / unknown
	Unit        string `json:"unit"`
	PID         int    `json:"pid"`
	Port        int    `json:"port"`
	Manageable  bool   `json:"manageable"`
	LogSource   string `json:"log_source"` // file / journal / none
	LogPath     string `json:"log_path"`
}

const opsPort = 7779

var unitNameRegex = regexp.MustCompile(`^[a-zA-Z0-9@._-]+$`)
var hostNameRegex = regexp.MustCompile(`^[a-zA-Z0-9.\-]+$`)

// runCmd 执行命令（带超时）
func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runSudoSystemctl 通过sudo执行systemctl
func runSudoSystemctl(timeout time.Duration, args ...string) (string, error) {
	full := append([]string{"-n", "systemctl"}, args...)
	return runCmd(timeout, "sudo", full...)
}

// 当前进程信息（核心组件）
func coreProcessInfo() (int, string) {
	pid := os.Getpid()
	exe, _ := os.Executable()
	return pid, filepath.Dir(exe)
}

// GetSystemComponents 获取系统支撑组件列表
func GetSystemComponents() ([]Component, error) {
	var components []Component

	// 核心组件：Ops-Center 主服务
	pid, exeDir := coreProcessInfo()
	components = append(components, Component{
		Name:        "ops-center",
		DisplayName: "Ops-Center 主服务",
		Type:        "core",
		Description: "运维中心后端服务，提供全部API与前端页面托管",
		Status:      "running",
		PID:         pid,
		Port:        opsPort,
		Manageable:  true,
		LogSource:   "file",
		LogPath:     filepath.Join(exeDir, "backend.log"),
	})

	// 核心组件：SQLite 数据库
	dbPath := filepath.Join(exeDir, "data", "ops-center.db")
	dbStatus := "stopped"
	if _, err := os.Stat(dbPath); err == nil {
		if fi, err := os.Stat(dbPath); err == nil && fi.Size() > 0 {
			dbStatus = "running"
		}
	}
	components = append(components, Component{
		Name:        "sqlite",
		DisplayName: "SQLite 数据库",
		Type:        "core",
		Description: "服务器数据存储，位于 data/ops-center.db",
		Status:      dbStatus,
		Manageable:  false,
		LogSource:   "file",
		LogPath:     filepath.Join(exeDir, "backend.log"),
	})

	// 系统服务（systemd）
	units, err := getServiceUnits()
	if err == nil {
		components = append(components, units...)
	}

	return components, nil
}

// getServiceUnits 获取systemd服务列表
func getServiceUnits() ([]Component, error) {
	out, err := runCmd(10*time.Second, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--output=json")
	if err != nil {
		return nil, err
	}

	var raw []struct {
		Unit        string `json:"unit"`
		Active      string `json:"active"`
		Sub         string `json:"sub"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, err
	}

	// 重要服务即使停止也展示
	important := map[string]bool{
		"docker.service": true, "containerd.service": true,
		"postgresql": true, "redis-server.service": true,
		"nginx.service": true, "mysql.service": true,
		"mariadb.service": true, "mongod.service": true,
		"ssh.service": true, "cron.service": true,
		"dnsmasq.service": true,
	}

	var components []Component
	for _, u := range raw {
		key := u.Unit
		if !important[key] && u.Active != "active" && u.Active != "failed" {
			continue
		}

		status := "unknown"
		switch u.Active {
		case "active":
			status = "running"
		case "inactive", "dead":
			status = "stopped"
		case "failed":
			status = "failed"
		default:
			status = u.Active
		}

		components = append(components, Component{
			Name:        u.Unit,
			DisplayName: strings.TrimSuffix(u.Unit, ".service"),
			Type:        "service",
			Description: u.Description,
			Status:      status,
			Unit:        u.Unit,
			Manageable:  true,
			LogSource:   "journal",
		})
	}
	return components, nil
}

// isAllowedComponent 校验组件名是否可管理
func isAllowedComponent(name string) bool {
	if name == "ops-center" || name == "sqlite" {
		return true
	}
	if !unitNameRegex.MatchString(name) {
		return false
	}
	// 校验该unit真实存在
	out, err := runCmd(5*time.Second, "systemctl", "list-units", "--type=service", "--all", "--no-pager", "--output=json")
	if err != nil {
		return false
	}
	return strings.Contains(out, `"unit":"`+name+`"`)
}

// RestartComponent 重启组件
func RestartComponent(name string) (string, error) {
	switch name {
	case "ops-center":
		_, exeDir := coreProcessInfo()
		script := fmt.Sprintf(`#!/bin/bash
sleep 2
OLDPID=$(pgrep -x ops-center | head -1)
if [ -n "$OLDPID" ]; then kill "$OLDPID"; fi
sleep 1
cd %s
setsid ./ops-center >> backend.log 2>&1 &
disown
`, exeDir)
		if err := launchDetached(script); err != nil {
			return "", err
		}
		return "主服务将在2秒后自动重启", nil
	case "sqlite":
		return "数据库组件随主服务运行，无需单独重启", nil
	default:
		if !isAllowedComponent(name) {
			return "", fmt.Errorf("组件不存在或不可管理")
		}
		if _, err := runSudoSystemctl(60*time.Second, "restart", name); err != nil {
			return "", fmt.Errorf("重启失败: %v", err)
		}
		return "重启成功", nil
	}
}

// StopComponent 停止组件
func StopComponent(name string) (string, error) {
	switch name {
	case "ops-center":
		script := `#!/bin/bash
sleep 1
OLDPID=$(pgrep -x ops-center | head -1)
if [ -n "$OLDPID" ]; then kill "$OLDPID"; fi
`
		if err := launchDetached(script); err != nil {
			return "", err
		}
		return "主服务将在1秒后停止", nil
	case "sqlite":
		return "数据库组件无法单独停止（随主服务运行）", nil
	default:
		if !isAllowedComponent(name) {
			return "", fmt.Errorf("组件不存在或不可管理")
		}
		if _, err := runSudoSystemctl(60*time.Second, "stop", name); err != nil {
			return "", fmt.Errorf("停止失败: %v", err)
		}
		return "已停止", nil
	}
}

// launchDetached 以独立会话方式运行脚本
func launchDetached(script string) error {
	cmd := exec.Command("bash", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}

// GetComponentLogs 获取组件最新日志
func GetComponentLogs(name string, lines int) (string, string, error) {
	if lines <= 0 || lines > 1000 {
		lines = 100
	}

	switch name {
	case "ops-center":
		_, exeDir := coreProcessInfo()
		logPath := filepath.Join(exeDir, "backend.log")
		out, err := runCmd(10*time.Second, "tail", "-n", strconv.Itoa(lines), logPath)
		if err != nil {
			return "", "", fmt.Errorf("读取日志失败: %v", err)
		}
		return "file", out, nil
	case "sqlite":
		_, exeDir := coreProcessInfo()
		logPath := filepath.Join(exeDir, "backend.log")
		out, err := runCmd(10*time.Second, "tail", "-n", strconv.Itoa(lines), logPath)
		if err != nil {
			return "", "", fmt.Errorf("读取日志失败: %v", err)
		}
		return "file", out, nil
	default:
		if !isAllowedComponent(name) {
			return "", "", fmt.Errorf("组件不存在")
		}
		out, err := runCmd(15*time.Second, "sudo", "-n", "journalctl", "-u", name, "-n", strconv.Itoa(lines), "--no-pager")
		if err != nil && out == "" {
			return "", "", fmt.Errorf("读取日志失败: %v", err)
		}
		return "journal", out, nil
	}
}

// ---------- 网络测试工具 ----------

// RunPing ping测试
func RunPing(host string, count int) (string, error) {
	if !hostNameRegex.MatchString(host) {
		return "", fmt.Errorf("非法的主机名或IP")
	}
	if count <= 0 || count > 20 {
		count = 4
	}
	return runCmd(30*time.Second, "ping", "-c", strconv.Itoa(count), host)
}

// TestTcpPort TCP端口测试
func TestTcpPort(host string, port int) (string, error) {
	if !hostNameRegex.MatchString(host) {
		return "", fmt.Errorf("非法的主机名或IP")
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("非法端口号")
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Sprintf("端口 %d 连接失败: %v", port, err), nil
	}
	conn.Close()
	return fmt.Sprintf("端口 %d 连接成功: %s 可访问", port, addr), nil
}

// ResolveDNS DNS解析测试
func ResolveDNS(hostname string) (string, error) {
	if !hostNameRegex.MatchString(hostname) {
		return "", fmt.Errorf("非法的主机名")
	}
	out, err := runCmd(15*time.Second, "dig", "+short", hostname)
	if err != nil && out == "" {
		return "", fmt.Errorf("DNS解析失败: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		return fmt.Sprintf("%s 无解析结果", hostname), nil
	}
	return fmt.Sprintf("%s 解析结果:\n%s", hostname, out), nil
}

// CheckHTTP HTTP/HTTPS检查
func CheckHTTP(url, method string) (string, int64, error) {
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", 0, fmt.Errorf("URL必须以 http:// 或 https:// 开头")
	}
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)
	if method != "GET" && method != "HEAD" && method != "POST" {
		return "", 0, fmt.Errorf("仅支持GET/HEAD/POST")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	start := time.Now()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return "", 0, fmt.Errorf("请求构建失败: %v", err)
	}
	resp, err := client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if err != nil {
		return fmt.Sprintf("请求失败: %v", err), elapsed, nil
	}
	defer resp.Body.Close()
	return fmt.Sprintf("HTTP %s %s → 状态码 %d，耗时 %dms", method, url, resp.StatusCode, elapsed), elapsed, nil
}

// ServiceCheckResult 服务检测结果
type ServiceCheckResult struct {
	OK           bool   `json:"ok"`
	Output       string `json:"output"`
	ResponseTime int64  `json:"response_time_ms"`
	Version      string `json:"version"`
}

var titleRegex = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
var sshBannerRegex = regexp.MustCompile(`(?m)^SSH-[0-9.]+-[^\r\n]+`)
var versionWordRegex = regexp.MustCompile(`(?i)[A-Za-z][A-Za-z0-9_./-]*\s?[0-9]+\.[0-9]+[0-9.a-z-]*`)
var pgVersionRegex = regexp.MustCompile(`(?s)server_version\x00([^\x00]+)`)
var redisVersionRegex = regexp.MustCompile(`redis_version:([0-9.]+)`)
var rdpVersionRegex = regexp.MustCompile(`(?i)Windows\s+(?:Server\s+)?(\d{4}(?:\s+R2)?|\d+\.\d+)`)

// 内部工具访问HTTPS自签名证书站点
var insecureHTTPClient = &http.Client{
	Timeout: 15 * time.Second,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	},
}

// CheckService 检测服务运行状态（根据访问方式），返回耗时与版本信息
func CheckService(accessMethod, host string, port int) (*ServiceCheckResult, error) {
	res := &ServiceCheckResult{}
	method := strings.ToUpper(accessMethod)

	switch method {
	case "WEB", "HTTP", "HTTPS":
		return checkWebService(method, host, port, res)
	case "SSH", "FTP", "SFTP", "MySQL", "PostgreSQL", "Redis", "RDP", "VNC":
		return checkBannerService(method, host, port, res)
	default:
		return checkTcpService(host, port, res)
	}
}

// checkWebService 检测WEB类服务（HTTP/HTTPS）
func checkWebService(method, host string, port int, res *ServiceCheckResult) (*ServiceCheckResult, error) {
	u := host
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		scheme := "http"
		if method == "HTTPS" {
			scheme = "https"
		}
		u = fmt.Sprintf("%s://%s", scheme, host)
		if port > 0 && port != 80 && port != 443 {
			u = fmt.Sprintf("%s://%s:%d", scheme, host, port)
		}
	}

	client := insecureHTTPClient
	start := time.Now()
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("请求构建失败: %v", err)
	}
	req.Header.Set("User-Agent", "Ops-Center-Monitor/1.0")
	resp, err := client.Do(req)
	res.ResponseTime = time.Since(start).Milliseconds()
	if err != nil {
		res.Output = fmt.Sprintf("HTTP GET %s 失败: %v", u, err)
		return res, nil
	}
	defer resp.Body.Close()

	res.OK = true
	var versions []string
	for _, h := range []string{"Server", "X-Powered-By"} {
		if v := resp.Header.Get(h); v != "" {
			versions = append(versions, v)
		}
	}
	res.Version = strings.Join(versions, " / ")
	res.Output = fmt.Sprintf("HTTP GET %s → 状态码 %d，耗时 %dms", u, resp.StatusCode, res.ResponseTime)

	buf := make([]byte, 4096)
	n, _ := resp.Body.Read(buf)
	if m := titleRegex.FindSubmatch(buf[:n]); len(m) > 1 {
		t := strings.TrimSpace(string(m[1]))
		if t != "" {
			res.Output += "，标题: " + t
		}
	}
	return res, nil
}

// checkBannerService 检测带横幅(banner)的协议服务
func checkBannerService(method, host string, port int, res *ServiceCheckResult) (*ServiceCheckResult, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		res.Output = fmt.Sprintf("端口 %d 连接失败: %v", port, err)
		return res, nil
	}
	defer conn.Close()

	start := time.Now()
	conn.SetDeadline(time.Now().Add(6 * time.Second))

	var buf []byte
	switch method {
	case "Redis":
		if _, err := conn.Write([]byte("INFO server\r\n")); err == nil {
			buf = make([]byte, 4096)
			n, _ := conn.Read(buf)
			buf = buf[:n]
		}
	case "RDP":
		// RDP: 发送 X.224 Connection Request 以获取服务器响应中的版本信息
		x224ConnReq := buildRDPNegotiationRequest()
		if _, err := conn.Write(x224ConnReq); err == nil {
			buf = make([]byte, 4096)
			n, _ := conn.Read(buf)
			buf = buf[:n]
		}
	case "VNC":
		// VNC: 服务器会在连接后主动发送版本字符串 "RFB xxx.yyy\n"
		buf = make([]byte, 4096)
		n, _ := conn.Read(buf)
		buf = buf[:n]
	default:
		buf = make([]byte, 4096)
		n, _ := conn.Read(buf)
		buf = buf[:n]
	}
	res.ResponseTime = time.Since(start).Milliseconds()

	res.OK = true
	raw := string(buf)
	res.Version = detectServiceVersion(method, raw)
	res.Output = fmt.Sprintf("端口 %d 连接成功（%s），耗时 %dms", port, method, res.ResponseTime)
	if res.Version != "" {
		res.Output += "，版本: " + res.Version
	}
	return res, nil
}

// buildRDPNegotiationRequest 构造最小化 X.224 Connection Request 以触发服务器响应
func buildRDPNegotiationRequest() []byte {
	// X.224 Connection Request (TPKT header + X.224 CR + RDP Negotiation Request)
	// 这是一个最小化的协商请求，让服务器返回 Connection Confirm（含版本信息）
	return []byte{
		// TPKT Header
		0x03, 0x00, 0x00, 0x13, // Version(3.0) + Length(19)
		// X.224 Connection Request
		0x0e, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
		0x00, 0x08, 0x00, 0x03, 0x00, 0x00, 0x00,
		// RDP Negotiation Request: PROTOCOL_HYBRID | PROTOCOL_SSL
		0x02, 0x00, 0x00, 0x00,
	}
}

// checkTcpService 纯TCP端口检测
func checkTcpService(host string, port int, res *ServiceCheckResult) (*ServiceCheckResult, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	res.ResponseTime = time.Since(start).Milliseconds()
	if err != nil {
		res.Output = fmt.Sprintf("端口 %d 连接失败: %v", port, err)
		return res, nil
	}
	conn.Close()
	res.OK = true
	res.Output = fmt.Sprintf("端口 %d 连接成功: %s 可访问", port, addr)
	return res, nil
}

// detectServiceVersion 从banner中提取服务版本（尽力而为）
func detectServiceVersion(method, raw string) string {
	if raw == "" {
		return ""
	}
	switch method {
	case "SSH":
		if m := sshBannerRegex.FindString(raw); m != "" {
			return m
		}
	case "FTP", "SFTP":
		// 形如 "220 (vsFTPd 3.0.3)" 或 "220 ProFTPD 1.3.5e Server"
		if m := versionWordRegex.FindString(raw); m != "" {
			return m
		}
	case "MySQL":
		// 握手包: [协议版本(0x0a)][版本字符串以0x00结尾]
		if len(raw) > 1 {
			if i := strings.IndexByte(raw[1:], 0x00); i >= 0 {
				v := raw[1 : 1+i]
				if v != "" {
					return "MySQL " + v
				}
			}
		}
	case "PostgreSQL":
		if m := pgVersionRegex.FindStringSubmatch(raw); len(m) > 1 {
			return "PostgreSQL " + m[1]
		}
		if m := versionWordRegex.FindString(raw); m != "" {
			return m
		}
	case "Redis":
		if m := redisVersionRegex.FindStringSubmatch(raw); len(m) > 1 {
			return "Redis " + m[1]
		}
	case "RDP":
		// 从 X.224 Connection Confirm 中提取 RDP 版本
		if m := rdpVersionRegex.FindStringSubmatch(raw); len(m) > 1 {
			return "RDP " + m[1]
		}
		// 从 rdpNegData (type=0x01) 中提取 selectedProto
		if len(raw) > 11 {
			// 查找 rdpNegData cookie: 0x02 0x00 0x00 0x00 (TYPE_RDP_NEG)
			for i := 0; i < len(raw)-7; i++ {
				if raw[i] == 0x02 && raw[i+1] == 0x00 && raw[i+2] == 0x00 && raw[i+3] == 0x00 {
					// msgType at offset 4 (1 byte), flags at 5, length at 6-7
					// selectedProto at offset 8-11
					if i+11 < len(raw) {
						proto := uint32(raw[i+8]) | uint32(raw[i+9])<<8 | uint32(raw[i+10])<<16 | uint32(raw[i+11])<<24
						switch {
						case proto&0x01 != 0:
							return "RDP (Standard RDP Security)"
						case proto&0x02 != 0:
							return "RDP (TLS)"
						case proto&0x04 != 0:
							return "RDP (NLA/CredSSP)"
						default:
							return "RDP"
						}
					}
				}
			}
			return "RDP"
		}
		return "RDP"
	case "VNC":
		// VNC 版本格式: "RFB 003.008\n"
		if m := regexp.MustCompile(`RFB\s+(\d+\.\d+)`).FindStringSubmatch(raw); len(m) > 1 {
			return "VNC " + m[1]
		}
	}
	return ""
}

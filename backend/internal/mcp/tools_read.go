package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// registerReadTools 只读工具：SQL 只查非敏感列，凭据列根本不进入内存
func (s *Server) registerReadTools() {
	s.tool("list_servers",
		"列出所有受管服务器（ID/名称/IP/端口/系统/状态/分组，不含任何凭据）",
		s.listServers)

	s.tool("get_server_detail",
		"查询单台服务器详情（基本信息 + 已登记服务数）",
		s.getServerDetail,
		intParam("server_id", "服务器 ID"))

	s.tool("list_services",
		"列出某服务器已登记的服务（名称/访问方式/地址/端口/状态，不含凭据）",
		s.listServices,
		intParam("server_id", "服务器 ID"))

	s.tool("scan_free_ips",
		"扫描已有网段中在线但未登记为服务器的 IP（默认 90 秒缓存，force=true 强制重扫）",
		s.scanFreeIPs,
		boolParam("force", "强制重新扫描"))

	s.tool("scan_server_ports",
		"SSH 到目标服务器扫描 TCP 监听端口（需该服务器已配置 SSH 凭据；返回端口/绑定/进程，不含凭据）",
		s.scanServerPorts,
		intParam("server_id", "服务器 ID"))

	s.tool("network_test",
		"网络探测：type=ping(测连通)/port(测TCP端口,需port参数)/dns(域名解析)/http(HTTP状态码)",
		s.networkTest,
		strParam("type", "探测类型: ping|port|dns|http"),
		strParam("host", "目标主机、主机:端口 或 URL"),
		intParam("port", "type=port 时的目标端口"))
}

// ---- 工具实现 ----

func (s *Server) listServers(ctx context.Context, args map[string]any) (string, error) {
	rows, err := models.GetDB().QueryContext(ctx,
		`SELECT id, name, ip, port, os, status, COALESCE(group_name,''), created_at
		 FROM servers ORDER BY id`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	type srv struct {
		ID        int    `json:"id"`
		Name      string `json:"name"`
		IP        string `json:"ip"`
		Port      int    `json:"port"`
		OS        string `json:"os"`
		Status    string `json:"status"`
		Group     string `json:"group,omitempty"`
		CreatedAt string `json:"created_at"`
	}
	list := []srv{}
	for rows.Next() {
		var v srv
		var os, created sql.NullString
		if err := rows.Scan(&v.ID, &v.Name, &v.IP, &v.Port, &os, &v.Status, &v.Group, &created); err != nil {
			return "", err
		}
		v.OS, v.CreatedAt = os.String, created.String
		list = append(list, v)
	}
	return marshal(map[string]any{"count": len(list), "servers": list})
}

func (s *Server) getServerDetail(ctx context.Context, args map[string]any) (string, error) {
	id, ok := argInt(args, "server_id")
	if !ok {
		return "", fmt.Errorf("缺少 server_id")
	}
	var name, ip string
	var port int
	var osV, status, group, created sql.NullString
	err := models.GetDB().QueryRowContext(ctx,
		`SELECT name, ip, port, os, status, COALESCE(group_name,''), created_at
		 FROM servers WHERE id = ?`, id).Scan(&name, &ip, &port, &osV, &status, &group, &created)
	if err != nil {
		return "", fmt.Errorf("服务器 %d 不存在: %w", id, err)
	}
	var svcCount int
	models.GetDB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM server_services WHERE server_id = ?`, id).Scan(&svcCount)
	return marshal(map[string]any{
		"id": id, "name": name, "ip": ip, "port": port,
		"os": osV.String, "status": status.String, "group": group.String,
		"created_at": created.String, "service_count": svcCount,
	})
}

func (s *Server) listServices(ctx context.Context, args map[string]any) (string, error) {
	id, ok := argInt(args, "server_id")
	if !ok {
		return "", fmt.Errorf("缺少 server_id")
	}
	rows, err := models.GetDB().QueryContext(ctx,
		`SELECT id, name, access_method, COALESCE(address,''), COALESCE(port,0), status
		 FROM server_services WHERE server_id = ? ORDER BY id`, id)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	type svc struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		Method string `json:"access_method"`
		Address string `json:"address,omitempty"`
		Port   int    `json:"port"`
		Status string `json:"status"`
	}
	list := []svc{}
	for rows.Next() {
		var v svc
		if err := rows.Scan(&v.ID, &v.Name, &v.Method, &v.Address, &v.Port, &v.Status); err != nil {
			return "", err
		}
		list = append(list, v)
	}
	return marshal(map[string]any{"server_id": id, "count": len(list), "services": list})
}

func (s *Server) scanFreeIPs(ctx context.Context, args map[string]any) (string, error) {
	force := false
	if v, ok := args["force"].(bool); ok {
		force = v
	}
	scan, err := services.GetFreeIPs(models.GetDB(), nil, force)
	if err != nil {
		return "", err
	}
	return marshal(scan)
}

func (s *Server) scanServerPorts(ctx context.Context, args map[string]any) (string, error) {
	id, ok := argInt(args, "server_id")
	if !ok {
		return "", fmt.Errorf("缺少 server_id")
	}
	res, err := services.ScanServerPorts(models.GetDB(), id)
	if err != nil {
		return "", err
	}
	return marshal(res)
}

// networkTest 网络四类探测（在 MCP 进程内实现，不依赖平台 handler）
func (s *Server) networkTest(ctx context.Context, args map[string]any) (string, error) {
	typ := argString(args, "type")
	host := argString(args, "host")
	if typ == "" || host == "" {
		return "", fmt.Errorf("需要 type 和 host 参数")
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	switch typ {
	case "ping":
		c2, cancel2 := context.WithTimeout(cctx, 8*time.Second)
		defer cancel2()
		out, err := exec.CommandContext(c2, "ping", "-c", "2", "-W", "2", host).CombinedOutput()
		if len(out) > 800 {
			out = out[:800]
		}
		ok := err == nil
		return marshal(map[string]any{"type": "ping", "host": host, "alive": ok, "output": string(out)})

	case "port":
		port, _ := argInt(args, "port")
		if port <= 0 {
			return "", fmt.Errorf("type=port 需要 port 参数")
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", host, port), 3*time.Second)
		if err != nil {
			return marshal(map[string]any{"type": "port", "host": host, "port": port, "open": false, "error": err.Error()})
		}
		conn.Close()
		return marshal(map[string]any{"type": "port", "host": host, "port": port, "open": true})

	case "dns":
		ips, err := net.LookupHost(host)
		if err != nil {
			return marshal(map[string]any{"type": "dns", "host": host, "error": err.Error()})
		}
		return marshal(map[string]any{"type": "dns", "host": host, "ips": ips})

	case "http":
		url := host
		if len(url) > 7 && url[:7] != "http://" && (len(url) < 8 || url[:8] != "https://") {
			url = "http://" + url
		}
		req, err := http.NewRequestWithContext(cctx, "GET", url, nil)
		if err != nil {
			return "", err
		}
		resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
		if err != nil {
			return marshal(map[string]any{"type": "http", "url": url, "error": err.Error()})
		}
		defer resp.Body.Close()
		return marshal(map[string]any{"type": "http", "url": url, "status": resp.StatusCode, "final_url": resp.Request.URL.String()})

	default:
		return "", fmt.Errorf("不支持的探测类型: %s（可选 ping|port|dns|http）", typ)
	}
}

// ---- 序列化辅助 ----

func marshal(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

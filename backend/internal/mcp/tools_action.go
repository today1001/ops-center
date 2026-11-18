package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// registerActionTools 动作类工具：受 actions_enabled 总开关 + 各级开关 + token 级别限制。
// 这些动作不接触任何凭据：登记的端口/服务器均写空凭据，由管理员后续在平台 UI 补充。
func (s *Server) registerActionTools() {
	s.tool("add_ports_as_services",
		"把某服务器上扫描到的端口号批量登记为服务（幂等：已登记的自动跳过）",
		s.addPortsAsServices,
		intParam("server_id", "服务器 ID"),
		strParam("ports", "逗号分隔的端口号列表，如 \"22,443,8080\""))

	s.tool("add_free_ips_as_servers",
		"把 IP 批量登记为受管服务器（幂等：已存在的自动跳过；登记后可在平台 UI 补充 SSH 凭据）",
		s.addIPsAsServers,
		strParam("ips", "逗号分隔的 IP 列表，如 \"10.10.10.50,10.10.10.218\""),
		strParam("group", "目标分组名，可空"))

	s.tool("delete_service",
		"删除一条服务登记记录（L3 高危：默认禁用，需管理员在系统管理开启）",
		s.deleteService,
		intParam("service_id", "服务记录 ID"))
}

// addPortsAsServices 复用 services.AddServerPorts（事务 + 幂等去重）
func (s *Server) addPortsAsServices(ctx context.Context, args map[string]any) (string, error) {
	id, ok := argInt(args, "server_id")
	if !ok {
		return "", fmt.Errorf("缺少 server_id")
	}
	ports, err := parsePorts(argString(args, "ports"))
	if err != nil {
		return "", err
	}
	if len(ports) == 0 {
		return "", fmt.Errorf("端口列表为空")
	}
	added, skipped, err := services.AddServerPorts(models.GetDB(), id, ports)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已登记 %d 个端口为服务，跳过 %d 个（已存在或非法）。可用 list_services 工具查看结果。", added, skipped), nil
}

// addIPsAsServers 复用 services.AddFreeIPsAsServers（事务 + 幂等去重），归属 admin
func (s *Server) addIPsAsServers(ctx context.Context, args map[string]any) (string, error) {
	ips := parseIPs(argString(args, "ips"))
	if len(ips) == 0 {
		return "", fmt.Errorf("IP 列表为空")
	}
	group := argString(args, "group")
	added, skipped, err := services.AddFreeIPsAsServers(models.GetDB(), ips, group, 1)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("已登记 %d 台服务器，跳过 %d 个（已存在或非法）。可用 list_servers 工具查看结果。", added, skipped), nil
}

// deleteService L3 高危：删除一条服务登记记录
func (s *Server) deleteService(ctx context.Context, args map[string]any) (string, error) {
	svcID, ok := argInt(args, "service_id")
	if !ok {
		return "", fmt.Errorf("缺少 service_id")
	}
	if err := models.DeleteService(svcID); err != nil {
		return "", err
	}
	return fmt.Sprintf("已删除服务记录 %d", svcID), nil
}

// parsePorts 解析逗号分隔端口，去重 + 合法性校验
func parsePorts(s string) ([]int, error) {
	var out []int
	seen := map[int]bool{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return nil, fmt.Errorf("非法端口: %q", p)
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, nil
}

// parseIPs 解析逗号分隔 IP，去重
func parseIPs(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

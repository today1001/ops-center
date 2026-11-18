package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// Run 启动API服务器
func Run() {
	r := gin.Default()

	// CORS中间件
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// 全局访问日志（audit_logs）
	r.Use(AccessLogMiddleware())

	// 公开路由
	public := r.Group("/api")
	{
		public.POST("/auth/login", LoginHandler)
	}

	// 需要认证的路由
	auth := r.Group("/api")
	auth.Use(AuthMiddleware())
	{
		// 用户相关
		auth.GET("/auth/me", GetCurrentUserHandler)

		// 服务器相关
		auth.GET("/servers", GetServersHandler)
		auth.POST("/servers", CreateServerHandler)
		auth.POST("/servers/batch", CreateServersBatchHandler)
		auth.GET("/servers/:id", GetServerHandler)
		auth.GET("/servers/:id/password", GetAdminPassword)
		auth.PUT("/servers/:id", UpdateServerHandler)
		auth.DELETE("/servers/:id", DeleteServerHandler)
		auth.POST("/servers/:id/test", TestServerConnectionHandler)
		auth.GET("/servers/:id/detail", GetServerDetailHandler)
		auth.POST("/servers/:id/refresh", RefreshServerHandler)

		// 分组管理（创建/修改/删除仅管理员）
		auth.GET("/groups", GetGroupsHandler)
		auth.POST("/groups", RequireRolesMW(models.RoleAdmin), CreateGroupHandler)
		auth.PUT("/groups/:id", RequireRolesMW(models.RoleAdmin), UpdateGroupHandler)
		auth.DELETE("/groups/:id", RequireRolesMW(models.RoleAdmin), DeleteGroupHandler)

		// 服务管理相关
		auth.GET("/servers/:id/services", GetServerServicesHandler)
		auth.POST("/servers/:id/services", CreateServiceHandler)
		auth.PUT("/servers/:id/services/:serviceId", UpdateServiceHandler)
		auth.DELETE("/servers/:id/services/:serviceId", DeleteServiceHandler)
		auth.POST("/servers/:id/services/:serviceId/test", TestServiceHandler)
		auth.GET("/servers/:id/services/:serviceId/detail", GetServiceDetailHandler)
		auth.POST("/servers/refresh-all", RefreshAllServersHandler)
		auth.POST("/servers/collect-missing", CollectMissingDetailsHandler)
		auth.GET("/servers/stats/resource", GetResourceStatsHandler)
		auth.GET("/servers/stats/rankings", GetResourceRankingsHandler)

		// 系统组件相关（重启/停止仅管理员）
		auth.GET("/system/components", GetSystemComponentsHandler)
		auth.POST("/system/components/:name/restart", RequireRolesMW(models.RoleAdmin), RestartComponentHandler)
		auth.POST("/system/components/:name/stop", RequireRolesMW(models.RoleAdmin), StopComponentHandler)
		auth.GET("/system/components/:name/logs", GetComponentLogsHandler)

		// 系统测试（网络工具）
		auth.POST("/system/test/ping", TestPingHandler)
		auth.POST("/system/test/port", TestPortHandler)
		auth.POST("/system/test/dns", TestDNSHandler)
		auth.POST("/system/test/http", TestHTTPHandler)

		// 系统设置（端口列表，增删改仅管理员/分组管理员）
		auth.GET("/settings/ports", GetPortProfilesHandler)
		auth.POST("/settings/ports", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), CreatePortProfileHandler)
		auth.PUT("/settings/ports/:id", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), UpdatePortProfileHandler)
		auth.DELETE("/settings/ports/:id", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), DeletePortProfileHandler)

		// 工具模块
		auth.GET("/tools/web-services", GetWebServicesHandler)
		auth.POST("/tools/web/session", CreateWebSessionHandler)
		auth.GET("/tools/web/sessions", ListWebSessionsHandler)

		// SSH终端
		auth.POST("/ssh/session", CreateSSHSessionHandler)
		auth.POST("/ssh/shadow", CreateSSHShadowHandler)

		// 虚拟网络
		auth.GET("/vnet/networks", GetVnetNetworksHandler)
		auth.POST("/vnet/networks", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), CreateVnetNetworkHandler)
		auth.PUT("/vnet/networks/:id", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), UpdateVnetNetworkHandler)
		auth.DELETE("/vnet/networks/:id", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), DeleteVnetNetworkHandler)
		auth.GET("/vnet/peers", GetVnetPeersHandler)
		auth.POST("/vnet/sync", VnetSyncHandler)
		auth.POST("/vnet/join", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), VnetJoinOrCreateHandler)
		auth.POST("/vnet/instance/stop", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), VnetStopInstanceHandler)
		auth.POST("/vnet/instance/delete", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), VnetDeleteInstanceHandler)
		auth.GET("/vnet/last-sync", GetLastVnetSyncHandler)
		auth.POST("/vnet/bind", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), VnetBindHandler)
		auth.POST("/vnet/unbind", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), VnetUnbindHandler)
		auth.GET("/servers/:id/virtual", VnetServerVirtualHandler)
		// 虚拟化主机（ESXi / PVE）
		auth.GET("/hypervisors", ListHypervisorHandler)
		auth.POST("/hypervisors", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), CreateHypervisorHandler)
		auth.PUT("/hypervisors/:id", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), UpdateHypervisorHandler)
		auth.DELETE("/hypervisors/:id", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), DeleteHypervisorHandler)
		auth.POST("/hypervisors/:id/test", TestHypervisorHandler)
		auth.GET("/hypervisors/:id/inventory", HostInventoryHandler)
		auth.POST("/hypervisors/:id/password", GetHypervisorPasswordHandler)

		// 救援控制台 API（高危操作仅管理员）
		auth.GET("/rescue/status", RescueStatusHandler)
		auth.POST("/rescue/backup-db", RequireRolesMW(models.RoleAdmin), RescueBackupDBHandler)
		auth.GET("/rescue/log", RescueLogHandler)
		auth.POST("/rescue/restart", RequireRolesMW(models.RoleAdmin), RescueRestartHandler)
		auth.POST("/rescue/fix-assets", RequireRolesMW(models.RoleAdmin), RescueFixAssetsHandler)

		// RDP远程桌面
		auth.POST("/tools/rdp/session", CreateRDPSessionHandler)

		// 空闲 IP 管理
		auth.GET("/free/ips", GetFreeIPsHandler)
		auth.POST("/free/ips/add", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin), AddFreeIPsHandler)
		// 空闲 IP 定时扫描（结果落库）
		auth.GET("/free/scan-config", GetFreeScanConfigHandler)
		auth.POST("/free/scan-config", RequireRolesMW(models.RoleAdmin), SetFreeScanConfigHandler)
		auth.GET("/free/records", GetFreeRecordsHandler)

		// 空闲端口管理
		auth.GET("/servers/:id/free-ports", ScanServerPortsHandler)
		auth.POST("/servers/:id/free-ports/add", RequireRolesMW(models.RoleAdmin, models.RoleGroupAdmin, models.RoleServerAdmin), AddServerPortsHandler)

		// MCP 管理（系统管理，仅管理员）
		auth.GET("/mcp/status", RequireRolesMW(models.RoleAdmin), McpStatusHandler)
		auth.GET("/mcp/config", RequireRolesMW(models.RoleAdmin), McpGetConfigHandler)
		auth.POST("/mcp/levels", RequireRolesMW(models.RoleAdmin), McpSetLevelsHandler)
		auth.POST("/mcp/actions", RequireRolesMW(models.RoleAdmin), McpSetActionsHandler)
		auth.POST("/mcp/tokens", RequireRolesMW(models.RoleAdmin), McpAddTokenHandler)
		auth.POST("/mcp/tokens/revoke", RequireRolesMW(models.RoleAdmin), McpRevokeTokenHandler)
		auth.GET("/mcp/audit", RequireRolesMW(models.RoleAdmin), McpAuditHandler)

		// 用户与权限管理（仅管理员）
		admin := auth.Group("/admin", RequireRoleAdmin())
		{
			admin.GET("/users", ListAdminUsersHandler)
			admin.POST("/users", CreateAdminUserHandler)
			admin.PUT("/users/:id", UpdateAdminUserHandler)
			admin.DELETE("/users/:id", DeleteAdminUserHandler)
			admin.GET("/users/:id/permissions", GetUserPermissionsHandler)
			admin.POST("/users/:id/groups", GrantUserGroupsHandler)
			admin.DELETE("/users/:id/groups/:gid", RevokeUserGroupHandler)
			admin.POST("/users/:id/servers", GrantUserServersHandler)
			admin.DELETE("/users/:id/servers/:sid", RevokeUserServerHandler)
			admin.GET("/audit", GetAuditLogsHandler)
		}
	}

	// 救援控制台页面（不依赖前端构建产物，主界面白屏时也可用）
	r.GET("/rescue.html", RescuePageHandler)

	// 虚拟网络后台同步
	go services.StartVnetSyncLoop()

	// Guacamole 反向代理（含 WebSocket 隧道）
	r.Any("/guac/*path", GuacProxyHandler)
	r.Any("/guac", GuacProxyHandler)

	// 网页代理（受会话令牌保护，无需登录令牌，供iframe使用）
	r.GET("/api/tools/web", ProxyWebHandler)
	r.POST("/api/tools/web", ProxyWebPostHandler)
	// 网页会话单控制器锁（供被代理页面调用，无需登录令牌）
	r.GET("/api/tools/web/lock", WebLockStatusHandler)
	r.POST("/api/tools/web/lock", WebLockHandler)
	// ExtJS API 代理（转发 /api2/json 请求到目标服务器）
	r.GET("/api/tools/webproxy-api", ProxyAPIHandler)
	// 全路径反向代理：/px/{tk}/path → target/path（所有请求统一走代理）
	r.Any("/px/:tk/*path", ReverseProxyHandler)
	r.GET("/px/:tk", ReverseProxyHandler)

	// SSH WebSocket终端
	r.GET("/ws/ssh/:tk", SSHWebSocketHandler)

	// 托管前端静态文件
	frontendDir := filepath.Join("frontend-dist")
	if _, err := os.Stat(frontendDir); err == nil {
		r.Static("/assets", filepath.Join(frontendDir, "assets"))
		// Guacamole 客户端库等第三方静态资源
		r.Static("/vendor", filepath.Join(frontendDir, "vendor"))
		r.StaticFile("/rdpguac.html", filepath.Join(frontendDir, "rdpguac.html"))
		r.StaticFile("/rdptest.html", filepath.Join(frontendDir, "rdptest.html"))
		r.StaticFile("/rdptest2.html", filepath.Join(frontendDir, "rdptest2.html"))
		r.StaticFile("/jsload.html", filepath.Join(frontendDir, "jsload.html"))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path

			// API请求返回404
			if strings.HasPrefix(path, "/api") {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}

			// 尝试从 cookie 获取 PVE 会话，只代理 PVE 资源路径
			if tk, err := c.Cookie("px_tk"); err == nil && tk != "" {
				if status, ct, body := services.ProxyPVEResource(tk, path, c.Request); status > 0 && status != 401 {
					c.Header("Content-Length", fmt.Sprintf("%d", len(body)))
					c.Header("Cache-Control", "no-cache")
					c.Data(status, ct, body)
					return
				}
			}

			// 返回 index.html，禁止缓存
			data, _ := os.ReadFile(filepath.Join(frontendDir, "index.html"))
			c.Header("Content-Type", "text/html; charset=utf-8")
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
			c.Data(http.StatusOK, "text/html; charset=utf-8", data)
		})
	}

	// 启动服务器
	r.Run(":7779")
}

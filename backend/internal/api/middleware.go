package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "未提供认证信息"})
			c.Abort()
			return
		}

		// 提取Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "认证格式错误"})
			c.Abort()
			return
		}

		tokenString := parts[1]
		claims, err := services.ParseToken(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的token"})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文中
		c.Set("userID", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}

// RequireRoleAdmin 仅管理员可访问
func RequireRoleAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if currentRole(c) != models.RoleAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "需要管理员权限"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireRolesMW 路由层角色中间件（白名单角色）
func RequireRolesMW(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireRoles(c, roles...) {
			c.Abort()
			return
		}
		c.Next()
	}
}

// requireRoles handler 内用：校验当前角色是否在允许列表中
// 不通过时已写入 403 响应，返回 false
func requireRoles(c *gin.Context, roles ...string) bool {
	role := currentRole(c)
	for _, r := range roles {
		if role == r {
			return true
		}
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "权限不足"})
	return false
}

// AccessLogMiddleware 全局访问日志：记录所有 API 请求到 audit_logs
// 静态资源与健康检查不记录；登录成功/失败由 LoginHandler 单独记录
func AccessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/assets") || strings.HasPrefix(p, "/vendor") ||
			strings.HasPrefix(p, "/px/") || p == "/health" || p == "/auth/login" ||
			p == "/api/auth/login" {
			return
		}

		// 用户信息安全取值（部分代理路由无需登录）
		uname := ""
		if v, ok := c.Get("username"); ok {
			if s, ok2 := v.(string); ok2 {
				uname = s
			}
		}

		models.LogAudit(models.AuditLog{
			UserID:     currentUserID(c),
			Username:   uname,
			Action:     accessAction(c),
			Method:     c.Request.Method,
			Path:       p,
			IP:         c.ClientIP(),
			Status:     c.Writer.Status(),
			DurationMs: time.Since(start).Milliseconds(),
		})
	}
}

// accessAction 简单分类：写操作 / 读操作
func accessAction(c *gin.Context) string {
	switch c.Request.Method {
	case "GET", "HEAD", "OPTIONS":
		return "access"
	default:
		return "write"
	}
}

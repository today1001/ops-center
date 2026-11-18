package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
)

// serverAccessGuard 校验当前用户对指定服务器的访问权限（write=true 时 viewer 拒绝）
// 通过返回 true；不通过时已写入 403 响应，返回 false
func serverAccessGuard(c *gin.Context, serverID int, needWrite bool) bool {
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	userID, _ := c.Get("userID")
	uid, _ := userID.(int)
	if !models.CanAccessServer(roleStr, uid, serverID, needWrite) {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问该服务器"})
		return false
	}
	return true
}

// currentRole 取当前请求角色
func currentRole(c *gin.Context) string {
	role, _ := c.Get("role")
	s, _ := role.(string)
	return s
}

// currentUserID 取当前请求用户ID
func currentUserID(c *gin.Context) int {
	uid, _ := c.Get("userID")
	i, _ := uid.(int)
	return i
}

// ============ 用户管理 ============

// ListAdminUsersHandler 用户列表（含授权摘要）
func ListAdminUsersHandler(c *gin.Context) {
	users, err := models.ListAdminUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取用户列表失败"})
		return
	}
	if users == nil {
		users = []models.AdminUserInfo{}
	}
	c.JSON(http.StatusOK, users)
}

// CreateAdminUserHandler 创建用户
func CreateAdminUserHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		Email    string `json:"email"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if req.Role == "" {
		req.Role = models.RoleViewer // 最小授权原则：默认只读
	}
	if err := models.CreateAdminUser(req.Username, req.Password, req.Email, req.Role); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "grant", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "创建用户 " + req.Username + " 角色 " + req.Role,
	})
	c.JSON(http.StatusCreated, gin.H{"message": "创建成功"})
}

// UpdateAdminUserHandler 修改用户角色/密码/邮箱
func UpdateAdminUserHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	var req struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
		Email    *string `json:"email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	// 防止把自己降级导致失去管理能力
	if req.Role != nil && id == currentUserID(c) && *req.Role != models.RoleAdmin {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不能修改自己的角色"})
		return
	}
	// 降级最后一个 admin 的保护
	if req.Role != nil && *req.Role != models.RoleAdmin {
		if role, err := models.GetUserRoleByID(id); err == nil && role == models.RoleAdmin {
			if cnt, _ := models.CountAdminUsers(); cnt <= 1 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "系统至少需要保留一个管理员"})
				return
			}
		}
	}
	if err := models.UpdateAdminUser(id, req.Role, req.Password, req.Email); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "grant", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "更新用户配置",
	})
	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteAdminUserHandler 删除用户
func DeleteAdminUserHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	if err := models.DeleteAdminUser(id, currentUserID(c)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "revoke", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "删除用户",
	})
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetUserPermissionsHandler 查询用户授权详情 + 可授权的候选列表
func GetUserPermissionsHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	groups, err := models.GetUserGrantedGroups(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取分组授权失败"})
		return
	}
	if groups == nil {
		groups = []models.ServerGroup{}
	}
	servers, err := models.GetUserGrantedServers(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器授权失败"})
		return
	}
	if servers == nil {
		servers = []models.Server{}
	}
	allGroups, _ := models.GetServerGroups()
	allServers, _ := models.GetServersAll()

	c.JSON(http.StatusOK, gin.H{
		"groups":      groups,
		"servers":     servers,
		"all_groups":  allGroups,
		"all_servers": allServers,
	})
}

// GrantUserGroupsHandler 批量授予分组
func GrantUserGroupsHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	var req struct {
		GroupIDs []int `json:"group_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	added, err := models.GrantUserGroups(id, req.GroupIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "授权失败"})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "grant", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "授予分组授权",
	})
	c.JSON(http.StatusOK, gin.H{"message": "授权成功", "added": added})
}

// RevokeUserGroupHandler 移除分组授权
func RevokeUserGroupHandler(c *gin.Context) {
	id, err1 := strconv.Atoi(c.Param("id"))
	gid, err2 := strconv.Atoi(c.Param("gid"))
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}
	if err := models.RevokeUserGroup(id, gid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除授权失败"})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "revoke", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "移除分组授权",
	})
	c.JSON(http.StatusOK, gin.H{"message": "移除成功"})
}

// GrantUserServersHandler 批量授予服务器
func GrantUserServersHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	var req struct {
		ServerIDs []int `json:"server_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	added, err := models.GrantUserServers(id, req.ServerIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "授权失败"})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "grant", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "授予服务器授权",
	})
	c.JSON(http.StatusOK, gin.H{"message": "授权成功", "added": added})
}

// RevokeUserServerHandler 移除服务器授权
func RevokeUserServerHandler(c *gin.Context) {
	id, err1 := strconv.Atoi(c.Param("id"))
	sid, err2 := strconv.Atoi(c.Param("sid"))
	if err1 != nil || err2 != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}
	if err := models.RevokeUserServer(id, sid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "移除授权失败"})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "revoke", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "移除服务器授权",
	})
	c.JSON(http.StatusOK, gin.H{"message": "移除成功"})
}

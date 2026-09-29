package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// GetServersHandler 获取服务器列表
func GetServersHandler(c *gin.Context) {
	userID, _ := c.Get("userID")

	servers, err := models.GetServers(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	if servers == nil {
		servers = []models.Server{}
	}

	c.JSON(http.StatusOK, servers)
}

// GetResourceStatsHandler 获取所有服务器资源汇总统计
func GetResourceStatsHandler(c *gin.Context) {
	stats, err := models.GetResourceStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取资源统计失败"})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// GetServerHandler 获取服务器详情（基础信息）
func GetServerHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	c.JSON(http.StatusOK, server)
}

// GetServerDetailHandler 获取服务器详细配置信息
func GetServerDetailHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	detail, err := models.GetServerDetail(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器详情不存在"})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// CreateServerHandler 创建服务器
func CreateServerHandler(c *gin.Context) {
	var server models.Server
	if err := c.ShouldBindJSON(&server); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	userID, _ := c.Get("userID")
	server.CreatedBy = userID.(int)

	if server.Port == 0 {
		server.Port = 22
	}
	if server.Status == "" {
		server.Status = "active"
	}

	if err := models.CreateServer(&server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务器失败"})
		return
	}

	// 创建后立即测试连接并采集信息
	go func() {
		services.TestAndCollectServerInfo(&server)
	}()

	c.JSON(http.StatusCreated, server)
}

// CreateServersBatchHandler 批量创建服务器
func CreateServersBatchHandler(c *gin.Context) {
	var request struct {
		Servers []models.Server `json:"servers" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	userID, _ := c.Get("userID")
	
	var created []models.Server
	var errors []string
	
	for _, server := range request.Servers {
		server.CreatedBy = userID.(int)
		
		if server.Port == 0 {
			server.Port = 22
		}
		if server.Status == "" {
			server.Status = "active"
		}
		
		if err := models.CreateServer(&server); err != nil {
			errors = append(errors, server.IP+": 创建失败")
			continue
		}
		
		created = append(created, server)
		
		// 异步采集信息
		go func(s models.Server) {
			services.TestAndCollectServerInfo(&s)
		}(server)
	}
	
	c.JSON(http.StatusCreated, gin.H{
		"message": "批量创建完成",
		"total":   len(created),
		"errors":  errors,
	})
}

// UpdateServerHandler 更新服务器
func UpdateServerHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	existing, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 备份原有凭据
	oldPassword := existing.Password
	oldPrivateKey := existing.PrivateKey

	var server models.Server
	if err := c.ShouldBindJSON(&server); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	server.ID = id
	server.CreatedBy = existing.CreatedBy

	// 如果未提供新密码/私钥，保留原有值，避免清空
	if server.Password == "" {
		server.Password = oldPassword
	}
	if server.PrivateKey == "" {
		server.PrivateKey = oldPrivateKey
	}

	if err := models.UpdateServer(&server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务器失败"})
		return
	}

	c.JSON(http.StatusOK, server)
}

// DeleteServerHandler 删除服务器
func DeleteServerHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	if err := models.DeleteServer(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除服务器失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// TestServerConnectionHandler 测试服务器连接
func TestServerConnectionHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	if err := services.TestSSHConnection(server); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "连接测试失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "连接成功"})
}

// RefreshServerHandler 刷新单个服务器信息
func RefreshServerHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	server, err := models.GetServerByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	// 异步刷新
	go func() {
		services.TestAndCollectServerInfo(server)
	}()

	c.JSON(http.StatusOK, gin.H{"message": "开始刷新"})
}

// CollectMissingDetailsHandler 采集缺失详情的服务器
func CollectMissingDetailsHandler(c *gin.Context) {
	userID, _ := c.Get("userID")

	servers, err := models.GetServersWithCreds(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	// 找出没有详情的服务器
	var missing []models.Server
	for _, s := range servers {
		_, err := models.GetServerDetail(s.ID)
		if err != nil {
			missing = append(missing, s)
		}
	}

	if len(missing) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "所有服务器已有详情数据", "total": 0})
		return
	}

	// 异步采集
	go func() {
		for i := range missing {
			services.TestAndCollectServerInfo(&missing[i])
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "开始采集缺失详情", "total": len(missing)})
}

// RefreshAllServersHandler 刷新所有服务器信息
func RefreshAllServersHandler(c *gin.Context) {
	userID, _ := c.Get("userID")

	servers, err := models.GetServersWithCreds(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务器列表失败"})
		return
	}

	// 异步刷新所有服务器
	go func() {
		for i := range servers {
			services.TestAndCollectServerInfo(&servers[i])
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "开始刷新所有服务器", "total": len(servers)})
}

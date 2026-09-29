package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// GetServerServicesHandler 获取指定服务器的服务列表
func GetServerServicesHandler(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	services, err := models.GetServicesByServer(serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取服务列表失败"})
		return
	}

	if services == nil {
		services = []models.ServerService{}
	}
	c.JSON(http.StatusOK, services)
}

// CreateServiceHandler 创建服务
func CreateServiceHandler(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器ID"})
		return
	}

	// 校验服务器存在
	if _, err := models.GetServerByID(serverID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}

	var s models.ServerService
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if s.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "服务名称不能为空"})
		return
	}
	s.ServerID = serverID
	if s.AccessMethod == "" {
		s.AccessMethod = "SSH"
	}
	if s.Status == "" {
		s.Status = "running"
	}

	if err := models.CreateService(&s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建服务失败"})
		return
	}
	c.JSON(http.StatusCreated, s)
}

// UpdateServiceHandler 更新服务
func UpdateServiceHandler(c *gin.Context) {
	serviceID, err := strconv.Atoi(c.Param("serviceId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务ID"})
		return
	}

	existing, err := models.GetServiceByID(serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务不存在"})
		return
	}

	var s models.ServerService
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	s.ID = serviceID
	s.ServerID = existing.ServerID

	// 未提供新密码则保留原值
	if s.Password == "" {
		s.Password = existing.Password
	}

	if err := models.UpdateService(&s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新服务失败"})
		return
	}
	c.JSON(http.StatusOK, s)
}

// DeleteServiceHandler 删除服务
func DeleteServiceHandler(c *gin.Context) {
	serviceID, err := strconv.Atoi(c.Param("serviceId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务ID"})
		return
	}

	if err := models.DeleteService(serviceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除服务失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// TestServiceHandler 检测服务运行状态
func TestServiceHandler(c *gin.Context) {
	serviceID, err := strconv.Atoi(c.Param("serviceId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务ID"})
		return
	}

	s, err := models.GetServiceByID(serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务不存在"})
		return
	}

	// 获取所属服务器IP作为默认检测目标
	server, err := models.GetServerByID(s.ServerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "所属服务器不存在"})
		return
	}

	host := s.Address
	if host == "" {
		host = server.IP
	}

	result, err := services.CheckService(s.AccessMethod, host, s.Port)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检验成功则更新为running，失败为stopped
	newStatus := "stopped"
	if result.OK {
		newStatus = "running"
	}
	models.UpdateService(&models.ServerService{
		ID:           s.ID,
		ServerID:     s.ServerID,
		Name:         s.Name,
		AccessMethod: s.AccessMethod,
		Address:      s.Address,
		Port:         s.Port,
		Username:     s.Username,
		Password:     s.Password,
		Status:       newStatus,
		Description:  s.Description,
	})

	// 记录检测历史
	success := 0
	if result.OK {
		success = 1
	}
	models.CreateServiceCheck(&models.ServiceCheck{
		ServiceID:      s.ID,
		Success:        success,
		ResponseTimeMs: result.ResponseTime,
		Version:        result.Version,
		Message:        result.Output,
	})

	total, okCount, rate, _ := models.GetServiceConnectivity(s.ID)

	c.JSON(http.StatusOK, gin.H{
		"ok":               result.OK,
		"output":           result.Output,
		"status":           newStatus,
		"response_time_ms": result.ResponseTime,
		"version":          result.Version,
		"connectivity_rate": rate,
		"total_checks":     total,
		"success_checks":   okCount,
	})
}

// GetServiceDetailHandler 获取服务详情（含连通率、最近检测与历史）
func GetServiceDetailHandler(c *gin.Context) {
	serviceID, err := strconv.Atoi(c.Param("serviceId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务ID"})
		return
	}

	s, err := models.GetServiceByID(serviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务不存在"})
		return
	}

	// 补全所属服务器信息
	if server, err := models.GetServerByID(s.ServerID); err == nil {
		s.ServerName = server.Name
		s.ServerIP = server.IP
	}

	checks, err := models.GetServiceChecks(serviceID, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取检测记录失败"})
		return
	}
	if checks == nil {
		checks = []models.ServiceCheck{}
	}

	total, success, rate, _ := models.GetServiceConnectivity(serviceID)

	c.JSON(http.StatusOK, gin.H{
		"service":          s,
		"connectivity_rate": rate,
		"total_checks":     total,
		"success_checks":   success,
		"checks":           checks,
	})
}

// GetWebServicesHandler 获取所有WEB类服务（网页访问工具使用）
func GetWebServicesHandler(c *gin.Context) {
	services, err := models.GetWebServices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取WEB服务列表失败"})
		return
	}
	if services == nil {
		services = []models.ServerService{}
	}
	c.JSON(http.StatusOK, services)
}
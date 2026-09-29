package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
)

// GetPortProfilesHandler 获取端口访问方式列表
func GetPortProfilesHandler(c *gin.Context) {
	profiles, err := models.GetPortProfiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取端口列表失败"})
		return
	}
	if profiles == nil {
		profiles = []models.PortProfile{}
	}
	c.JSON(http.StatusOK, profiles)
}

// CreatePortProfileHandler 创建端口配置
func CreatePortProfileHandler(c *gin.Context) {
	var p models.PortProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if p.Port <= 0 || p.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "端口号无效"})
		return
	}
	if p.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "服务名称不能为空"})
		return
	}

	// 校验端口是否已存在
	existing, err := models.GetPortProfiles()
	if err == nil {
		for _, ep := range existing {
			if ep.Port == p.Port {
				c.JSON(http.StatusBadRequest, gin.H{"error": "该端口已存在配置"})
				return
			}
		}
	}

	if err := models.CreatePortProfile(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建端口配置失败"})
		return
	}
	c.JSON(http.StatusCreated, p)
}

// UpdatePortProfileHandler 更新端口配置
func UpdatePortProfileHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的配置ID"})
		return
	}

	var p models.PortProfile
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	p.ID = id
	if p.Port <= 0 || p.Port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "端口号无效"})
		return
	}
	if p.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "服务名称不能为空"})
		return
	}

	if err := models.UpdatePortProfile(&p); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新端口配置失败"})
		return
	}
	c.JSON(http.StatusOK, p)
}

// DeletePortProfileHandler 删除端口配置
func DeletePortProfileHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的配置ID"})
		return
	}
	if err := models.DeletePortProfile(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除端口配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

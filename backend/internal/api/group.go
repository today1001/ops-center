package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
)

// GetGroupsHandler 获取分组列表
func GetGroupsHandler(c *gin.Context) {
	groups, err := models.GetServerGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取分组列表失败"})
		return
	}
	if groups == nil {
		groups = []models.ServerGroup{}
	}
	c.JSON(http.StatusOK, groups)
}

// CreateGroupHandler 创建分组
func CreateGroupHandler(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组名称不能为空"})
		return
	}

	if err := models.CreateServerGroup(req.Name, req.Description); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			c.JSON(http.StatusConflict, gin.H{"error": "分组名称已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建分组失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "创建成功"})
}

// UpdateGroupHandler 更新分组
func UpdateGroupHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组ID"})
		return
	}

	existing, err := models.GetServerGroupByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "分组不存在"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "分组名称不能为空"})
		return
	}

	if err := models.UpdateServerGroup(id, req.Name, req.Description, existing.Name); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			c.JSON(http.StatusConflict, gin.H{"error": "分组名称已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新分组失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// DeleteGroupHandler 删除分组，组下服务器归入未分组
func DeleteGroupHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的分组ID"})
		return
	}

	if err := models.DeleteServerGroup(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除分组失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

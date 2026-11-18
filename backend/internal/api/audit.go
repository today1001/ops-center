package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
)

// GetAuditLogsHandler 审计/访问日志查询
// GET /api/admin/audit?limit=100&offset=0&action=&username=
func GetAuditLogsHandler(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	action := c.Query("action")
	username := c.Query("username")

	logs, total, err := models.GetAuditLogs(limit, offset, action, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询审计日志失败"})
		return
	}
	if logs == nil {
		logs = []models.AuditLog{}
	}
	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": total,
		"limit": limit,
		"offset": offset,
	})
}

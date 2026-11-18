package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// LoginHandler 登录处理
func LoginHandler(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		models.LogAudit(models.AuditLog{
			Username: req.Username, Action: "login_failed",
			IP: c.ClientIP(), Detail: "请求参数错误",
		})
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	resp, err := services.Login(req.Username, req.Password)
	if err != nil {
		models.LogAudit(models.AuditLog{
			Username: req.Username, Action: "login_failed",
			IP: c.ClientIP(), Detail: err.Error(),
		})
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	models.LogAudit(models.AuditLog{
		UserID: resp.User.ID, Username: resp.User.Username, Action: "login",
		IP: c.ClientIP(), Status: 200, Detail: "登录成功",
	})

	c.JSON(http.StatusOK, resp)
}

// GetCurrentUserHandler 获取当前用户信息
func GetCurrentUserHandler(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
		return
	}

	user, err := models.GetUserByID(userID.(int))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	c.JSON(http.StatusOK, user)
}

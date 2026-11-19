package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// GetUpgradeStatusHandler 升级中心总览：三模块当前/最新版本 + 配置
func GetUpgradeStatusHandler(c *gin.Context) {
	db := models.GetDB()
	statuses, err := services.CheckAllUpdates(db)
	if err != nil {
		// 单模块错误已在 statuses 里，不阻塞整体
		_ = err
	}
	if statuses == nil {
		statuses = []services.UpgradeStatus{}
	}
	c.JSON(http.StatusOK, gin.H{
		"source":            services.GetUpgradeSource(db),
		"interval_minutes":  services.GetUpgradeInterval(db),
		"modules":           statuses,
	})
}

// SetUpgradeConfigHandler 保存升级配置
func SetUpgradeConfigHandler(c *gin.Context) {
	db := models.GetDB()
	var req struct {
		Source          *string           `json:"source"`
		IntervalMinutes *int              `json:"interval_minutes"`
		Policies        map[string]string `json:"policies"` // {backend: auto|manual|off, ...}
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}

	if req.Source != nil {
		s := *req.Source
		if s != "" && (len(s) > 6 && s[:6] != "local:" && s[:7] != "http://" && s[:8] != "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "升级源须以 http:// / https:// / local: 开头"})
			return
		}
		services.SetSource(db, s)
	}
	if req.IntervalMinutes != nil {
		n := *req.IntervalMinutes
		if n < 10 || n > 10080 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "检查间隔需在 10 ~ 10080 分钟"})
			return
		}
		services.SetInterval(db, n)
	}
	for mod, pol := range req.Policies {
		if pol != "auto" && pol != "manual" && pol != "off" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "策略仅支持 auto/manual/off"})
			return
		}
		switch mod {
		case "backend", "frontend", "mcp":
			services.SetPolicy(db, mod, pol)
		}
	}

	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "write", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "更新升级配置",
	})
	c.JSON(http.StatusOK, gin.H{"message": "配置已保存"})
}

// RunUpgradeHandler 手动触发升级（module: backend/frontend/mcp）
func RunUpgradeHandler(c *gin.Context) {
	var req struct {
		Module string `json:"module" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	switch req.Module {
	case "backend", "frontend", "mcp":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效模块: " + req.Module})
		return
	}

	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "write", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "手动升级模块 " + req.Module,
	})

	err := services.UpgradeModule(models.GetDB(), req.Module, "manual")
	if err != nil {
		// backend 升级成功时会 exec 重启，响应可能中断——客户端需轮询状态确认
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "升级完成", "module": req.Module, "version": services.GetInstalledVersion(models.GetDB(), req.Module)})
}

// GetUpgradeLogsHandler 升级历史
func GetUpgradeLogsHandler(c *gin.Context) {
	logs, err := services.GetUpgradeLogs(models.GetDB(), 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询升级日志失败"})
		return
	}
	if logs == nil {
		logs = []services.UpgradeLog{}
	}
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

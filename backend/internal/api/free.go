package api

import (
	"database/sql"
	"net/http"
	"strconv"

	"ops-center/internal/models"
	"ops-center/internal/services"

	"github.com/gin-gonic/gin"
)

var freeDB *sql.DB

// SetFreeDB 注入数据库连接
func SetFreeDB(db *sql.DB) {
	freeDB = db
}

// GetFreeIPsHandler 获取空闲 IP 列表（实时扫描）
// force=1 时强制重新扫描，并把结果落库（trigger=manual）
func GetFreeIPsHandler(c *gin.Context) {
	force := c.Query("force") == "1" || c.Query("force") == "true"
	ips, err := services.GetFreeIPs(freeDB, nil, force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if force {
		if _, err := services.SaveScanToDB(freeDB, ips, "manual"); err != nil {
			println("手动扫描落库失败:", err.Error())
		}
	}
	c.JSON(http.StatusOK, gin.H{"ips": ips})
}

// AddFreeIPsHandler 批量添加空闲 IP 为服务器
func AddFreeIPsHandler(c *gin.Context) {
	var req struct {
		IPs   []string `json:"ips"`
		Group string   `json:"group"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.IPs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP 列表不能为空"})
		return
	}
	// 当前登录用户（登记的服务器归属其名下，自动获得访问权限）
	userID := currentUserID(c)
	added, skipped, err := services.AddFreeIPsAsServers(freeDB, req.IPs, req.Group, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"added": added, "skipped": skipped})
}

// ScanServerPortsHandler 扫描服务器开放端口
func ScanServerPortsHandler(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器 ID"})
		return
	}
	if !serverAccessGuard(c, serverID, false) {
		return
	}
	result, err := services.ScanServerPorts(freeDB, serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// AddServerPortsHandler 批量添加端口为服务
func AddServerPortsHandler(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的服务器 ID"})
		return
	}
	if !serverAccessGuard(c, serverID, true) {
		return
	}
	var req struct {
		Ports []int `json:"ports"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Ports) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "端口列表不能为空"})
		return
	}
	added, skipped, err := services.AddServerPorts(freeDB, serverID, req.Ports)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"added": added, "skipped": skipped})
}

// GetFreeScanConfigHandler 查询定时扫描配置
func GetFreeScanConfigHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"interval_minutes": services.GetScanInterval(freeDB),
		"min":              5,
		"max":              1440,
		"last_scan_at":     services.GetLastScanTime(freeDB),
	})
}

// SetFreeScanConfigHandler 修改定时扫描间隔（下轮循环生效，无需重启）
func SetFreeScanConfigHandler(c *gin.Context) {
	var req struct {
		IntervalMinutes int `json:"interval_minutes" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if err := services.SetScanInterval(freeDB, req.IntervalMinutes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	models.LogAudit(models.AuditLog{
		UserID: currentUserID(c), Username: c.MustGet("username").(string),
		Action: "write", Method: c.Request.Method, Path: c.Request.URL.Path,
		IP: c.ClientIP(), Detail: "空闲IP扫描间隔改为 " + strconv.Itoa(req.IntervalMinutes) + " 分钟",
	})
	c.JSON(http.StatusOK, gin.H{"message": "已保存，下一轮扫描按新间隔执行"})
}

// GetFreeRecordsHandler 查询落库的扫描记录
// 可选参数 scan_id：取指定一次扫描的明细；不传取最新一次
func GetFreeRecordsHandler(c *gin.Context) {
	scanID, _ := strconv.ParseInt(c.Query("scan_id"), 10, 64)

	records, err := services.GetScanRecords(freeDB, scanID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询扫描记录失败"})
		return
	}
	if records == nil {
		records = []services.FreeIPRecord{}
	}
	history, err := services.GetScanHistory(freeDB, 10)
	if err != nil {
		history = []services.ScanHistoryItem{}
	}

	// 当前落库明细对应的扫描信息
	latest := services.ScanHistoryItem{}
	if len(history) > 0 {
		if scanID > 0 {
			for _, h := range history {
				if h.ID == scanID {
					latest = h
					break
				}
			}
		} else {
			latest = history[0]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"latest":  latest,
		"records": records,
		"history": history,
	})
}

package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// GetResourceRankingsHandler 获取资源利用率排行（最高/最低5台）
func GetResourceRankingsHandler(c *gin.Context) {
	rankings, err := models.GetResourceRankings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取利用率排行失败"})
		return
	}
	c.JSON(http.StatusOK, rankings)
}

// GetSystemComponentsHandler 获取系统组件列表
func GetSystemComponentsHandler(c *gin.Context) {
	components, err := services.GetSystemComponents()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取系统组件失败"})
		return
	}
	c.JSON(http.StatusOK, components)
}

// RestartComponentHandler 重启组件
func RestartComponentHandler(c *gin.Context) {
	name := c.Param("name")
	message, err := services.RestartComponent(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "name": name})
}

// StopComponentHandler 停止组件
func StopComponentHandler(c *gin.Context) {
	name := c.Param("name")
	message, err := services.StopComponent(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": message, "name": name})
}

// GetComponentLogsHandler 获取组件日志
func GetComponentLogsHandler(c *gin.Context) {
	name := c.Param("name")
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	source, logContent, err := services.GetComponentLogs(name, lines)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name, "source": source, "log": logContent})
}

// TestPingHandler ping测试
func TestPingHandler(c *gin.Context) {
	var req struct {
		Host  string `json:"host" binding:"required"`
		Count int    `json:"count"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少主机参数"})
		return
	}
	output, err := services.RunPing(req.Host, req.Count)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": output})
}

// TestPortHandler TCP端口测试
func TestPortHandler(c *gin.Context) {
	var req struct {
		Host string `json:"host" binding:"required"`
		Port int    `json:"port" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少主机或端口参数"})
		return
	}
	output, err := services.TestTcpPort(req.Host, req.Port)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": output})
}

// TestDNSHandler DNS解析测试
func TestDNSHandler(c *gin.Context) {
	var req struct {
		Hostname string `json:"hostname" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少主机名参数"})
		return
	}
	output, err := services.ResolveDNS(req.Hostname)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": output})
}

// TestHTTPHandler HTTP检查
func TestHTTPHandler(c *gin.Context) {
	var req struct {
		URL    string `json:"url" binding:"required"`
		Method string `json:"method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少URL参数"})
		return
	}
	output, elapsed, err := services.CheckHTTP(req.URL, req.Method)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "output": output, "elapsed_ms": elapsed})
}

package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// ===================== 虚拟网络 =====================

// GetVnetNetworksHandler 网络源列表
func GetVnetNetworksHandler(c *gin.Context) {
	list, err := models.GetVnetNetworks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取网络源失败"})
		return
	}
	if list == nil {
		list = []models.VnetNetwork{}
	}
	c.JSON(http.StatusOK, list)
}

// CreateVnetNetworkHandler 新增网络源
func CreateVnetNetworkHandler(c *gin.Context) {
	var n models.VnetNetwork
	if err := c.ShouldBindJSON(&n); err != nil || n.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if err := models.CreateVnetNetwork(&n); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, n)
}

// UpdateVnetNetworkHandler 更新网络源
func UpdateVnetNetworkHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效ID"})
		return
	}
	var n models.VnetNetwork
	if err := c.ShouldBindJSON(&n); err != nil || n.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	n.ID = id
	if err := models.UpdateVnetNetwork(&n); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, n)
}

// DeleteVnetNetworkHandler 删除网络源
func DeleteVnetNetworkHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效ID"})
		return
	}
	if err := models.DeleteVnetNetwork(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

// GetVnetPeersHandler 在线节点列表（实时查询 + 绑定状态）
func GetVnetPeersHandler(c *gin.Context) {
	networks, err := models.GetVnetNetworks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取网络源失败"})
		return
	}
	svMap, _ := models.GetAllServerVirtual()
	servers, _ := models.GetAllServersBasic()
	nameOf := make(map[int]string)
	virtualOf := make(map[int]models.ServerVirtual)
	for _, s := range servers {
		nameOf[s.ID] = s.Name
	}
	for id, sv := range svMap {
		virtualOf[id] = sv
	}

	type boundInfo struct {
		ServerID   int    `json:"server_id"`
		ServerName string `json:"server_name"`
	}
	out := []map[string]any{}
	for _, net := range networks {
		if net.Enabled != 1 {
			continue
		}
		peers, err := services.ListPeersByNetwork(net)
		if err != nil {
			out = append(out, map[string]any{"network": net.Name, "error": err.Error()})
			continue
		}
		for _, p := range peers {
			entry := map[string]any{
				"network":  net.Name,
				"hostname": p.Hostname,
				"ip":       p.IP,
				"version":  p.Version,
				"bound":    false,
			}
			// 绑定状态：identifier 大小写不敏感匹配
			for id, sv := range virtualOf {
				if sv.Network == net.Name && sv.Identifier != "" && equalFold(sv.Identifier, p.Hostname) {
					entry["bound"] = true
					entry["binding"] = boundInfo{ServerID: id, ServerName: nameOf[id]}
					break
				}
			}
			out = append(out, entry)
		}
	}
	c.JSON(http.StatusOK, out)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// VnetSyncHandler 立即同步
func VnetSyncHandler(c *gin.Context) {
	res, err := services.VnetSyncNow()
	if err != nil && res == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetLastVnetSyncHandler 最近同步结果
func GetLastVnetSyncHandler(c *gin.Context) {
	res := services.GetLastVnetSync()
	if res == nil {
		c.JSON(http.StatusOK, gin.H{"time": nil})
		return
	}
	c.JSON(http.StatusOK, res)
}

// VnetBindHandler 绑定节点到服务器 {network, hostname, ip, server_id}
func VnetBindHandler(c *gin.Context) {
	var req struct {
		Network  string `json:"network"`
		Hostname string `json:"hostname"`
		IP       string `json:"ip"`
		ServerID int    `json:"server_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Hostname == "" || req.ServerID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	// 服务器需存在
	if _, err := models.GetServerByID(req.ServerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "服务器不存在"})
		return
	}
	sv := &models.ServerVirtual{
		ServerID:   req.ServerID,
		Network:    req.Network,
		Identifier: req.Hostname,
		IP:         req.IP,
		Online:     1,
	}
	if err := models.UpsertServerVirtual(sv); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "绑定失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "绑定成功"})
}

// VnetUnbindHandler 解绑服务器 {server_id}
func VnetUnbindHandler(c *gin.Context) {
	var req struct {
		ServerID int `json:"server_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ServerID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if err := models.DeleteServerVirtual(req.ServerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解绑失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "解绑成功"})
}

// GetLastVnetSyncTimeHandler 给服务器列表页用的轻量同步状态
func VnetServerVirtualHandler(c *gin.Context) {
	serverID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效ID"})
		return
	}
	sv, err := models.GetServerVirtual(serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	c.JSON(http.StatusOK, sv)
}


// VnetJoinOrCreateHandler 加入或创建虚拟网络实例
// 先查本机是否已有该网络（按网络名），有则接管管理；无则新建 tun + easytier 连接
func VnetJoinOrCreateHandler(c *gin.Context) {
	var req services.JoinRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	res, err := services.VnetJoinOrCreate(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// VnetStopInstanceHandler 停止托管实例
func VnetStopInstanceHandler(c *gin.Context) {
	var req struct {
		ID int `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	networks, err := models.GetVnetNetworks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	for _, net := range networks {
		if net.ID == req.ID {
			if err := services.VnetStopInstance(net); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "实例已停止"})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "网络源不存在"})
}

// VnetDeleteInstanceHandler 删除托管实例（停止+清理配置）
func VnetDeleteInstanceHandler(c *gin.Context) {
	var req struct {
		ID int `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	networks, err := models.GetVnetNetworks()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败"})
		return
	}
	for _, net := range networks {
		if net.ID == req.ID {
			if err := services.VnetDeleteInstance(net); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "实例已删除"})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "网络源不存在"})
}

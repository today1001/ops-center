package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"ops-center/internal/models"
	"ops-center/internal/services"
)

// hvRequest 写入请求体。HypervisorHost.Password 带 json:"-"，只用于隐藏输出，
// 同时也无法接收输入，所以创建 / 更新必须走这个结构体。
type hvRequest struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Realm    string `json:"realm"`
	Password string `json:"password"`
}

// toHost 转换为存储模型，password 留空表示不改密码
func (r hvRequest) toHost(id int) models.HypervisorHost {
	return models.HypervisorHost{
		ID:       id,
		Name:     r.Name,
		Type:     r.Type,
		IP:       r.IP,
		Port:     r.Port,
		Username: r.Username,
		Realm:    r.Realm,
		Password: r.Password,
	}
}

// ListHypervisorHandler GET /api/hypervisors?type=esxi|pve
func ListHypervisorHandler(c *gin.Context) {
	hvType := c.Query("type")
	list, err := models.GetHypervisorHosts(hvType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取虚拟化主机失败"})
		return
	}
	c.JSON(http.StatusOK, list)
}

// CreateHypervisorHandler POST /api/hypervisors
func CreateHypervisorHandler(c *gin.Context) {
	var req hvRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if req.Name == "" || req.IP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和 IP 地址不能为空"})
		return
	}
	if req.Type != "esxi" && req.Type != "pve" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "类型必须是 esxi 或 pve"})
		return
	}
	h := req.toHost(0)
	h.Password = models.EncryptPassword(h.Password)
	if err := models.CreateHypervisorHost(&h); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": h.ID})
}

// UpdateHypervisorHandler PUT /api/hypervisors/:id
func UpdateHypervisorHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 ID"})
		return
	}
	old, err := models.GetHypervisorHost(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "主机不存在"})
		return
	}
	var req hvRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	if req.Name == "" || req.IP == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称和 IP 地址不能为空"})
		return
	}
	if req.Type != "esxi" && req.Type != "pve" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "类型必须是 esxi 或 pve"})
		return
	}
	h := req.toHost(id)
	if h.Password == "" {
		h.Password = old.Password // 密码留空则保留原值
	} else {
		h.Password = models.EncryptPassword(h.Password)
	}
	if err := models.UpdateHypervisorHost(&h); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteHypervisorHandler DELETE /api/hypervisors/:id
func DeleteHypervisorHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 ID"})
		return
	}
	if err := models.DeleteHypervisorHost(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// TestHypervisorHandler POST /api/hypervisors/:id/test 连通性 + 认证探测，并回写状态
func TestHypervisorHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 ID"})
		return
	}
	h, err := models.GetHypervisorHost(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "主机不存在"})
		return
	}
	res := services.ProbeHypervisor(h)

	status := "unknown"
	switch {
	case res.Reachable && res.AuthOK:
		status = "online"
	case res.Reachable:
		status = "auth_failed"
	default:
		status = "offline"
	}
	models.UpdateHypervisorStatus(id, status, time.Now().Format("2006-01-02 15:04:05"))

	c.JSON(http.StatusOK, res)
}

// HostInventoryHandler GET /api/hypervisors/:id/inventory 仅采集清单（不区分可达/认证）
func HostInventoryHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 ID"})
		return
	}
	h, err := models.GetHypervisorHost(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "主机不存在"})
		return
	}
	res := services.ProbeHypervisor(h)
	if res.AuthOK {
		models.UpdateHypervisorStatus(id, "online", time.Now().Format("2006-01-02 15:04:05"))
	}
	c.JSON(http.StatusOK, res)
}

// GetHypervisorPasswordHandler 管理员验证本人密码后返回主机明文密码（用于打开厂商 Web 控制台）
func GetHypervisorPasswordHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效 ID"})
		return
	}

	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可查看明文密码"})
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入您的登录密码进行验证"})
		return
	}

	userID, _ := c.Get("userID")
	user, err := models.GetUserByID(userID.(int))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "用户不存在"})
		return
	}
	// 这里必须用 403 而不是 401：前端的响应拦截器把 401 当成登录过期，
	// 会清掉 token 并跳回登录页，用户只是输错验证密码就会被登出。
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "验证密码错误"})
		return
	}

	h, err := models.GetHypervisorHost(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "主机不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"password": models.DecryptPassword(h.Password)})
}

package api

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ops-center/internal/services"
)

// ===================== 网页会话共享（一人操作，多人观看） =====================
// 操作者通过 /api/tools/web/session 创建会话并自动注册；
// 其他人通过 /api/tools/web/join 加入同一会话获得"观看"别名；
// 观看别名在代理层只放行 GET，并注入只读遮罩；
// 通过 /api/tools/web/control 可以接管成为操作者，原操作者自动降级为观看者。

type webShareMeta struct {
	BaseTK       string               // 真实代理会话令牌
	ServiceKey   string               // 服务标识（svc-<id>）
	OperatorTK   string               // 当前操作者令牌（初始=BaseTK）
	OperatorUser string               // 当前操作者用户名
	LastSeen     map[string]time.Time // 各别名最近活跃时间
	CreatedAt    time.Time
}

var (
	webShareMu      sync.Mutex
	webShareBySvc   map[string]*webShareMeta // serviceKey -> meta
	webShareByAlias map[string]*webShareMeta // 别名/原始tk -> meta
	webAliasRole    map[string]string        // 别名/原始tk -> operator | viewer
)

func init() {
	webShareBySvc = make(map[string]*webShareMeta)
	webShareByAlias = make(map[string]*webShareMeta)
	webAliasRole = make(map[string]string)
}

const webShareIdleTimeout = 2 * time.Hour

// webShareCleanupLocked 清理长时间无活动的共享会话（需持有锁）
func webShareCleanupLocked() {
	now := time.Now()
	for key, meta := range webShareBySvc {
		newest := meta.CreatedAt
		for _, t := range meta.LastSeen {
			if t.After(newest) {
				newest = t
			}
		}
		if now.Sub(newest) > webShareIdleTimeout {
			for alias, m := range webShareByAlias {
				if m == meta {
					delete(webShareByAlias, alias)
					delete(webAliasRole, alias)
				}
			}
			delete(webShareBySvc, key)
		}
	}
}

// currentUsername 从上下文取当前登录用户名
func currentUsername(c *gin.Context) string {
	if v, ok := c.Get("username"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// registerWebOperator 将新建的网页会话注册为操作者会话
func registerWebOperator(tk, serviceKey, username string) {
	webShareMu.Lock()
	defer webShareMu.Unlock()
	webShareCleanupLocked()
	meta := &webShareMeta{
		BaseTK:       tk,
		ServiceKey:   serviceKey,
		OperatorTK:   tk,
		OperatorUser: username,
		LastSeen:     map[string]time.Time{tk: time.Now()},
		CreatedAt:    time.Now(),
	}
	webShareBySvc[serviceKey] = meta
	webShareByAlias[tk] = meta
	webAliasRole[tk] = "operator"
}

// JoinWebSessionHandler 加入指定服务的进行中会话（观看模式）
func JoinWebSessionHandler(c *gin.Context) {
	var req struct {
		ServiceID int `json:"service_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ServiceID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 service_id"})
		return
	}
	key := fmt.Sprintf("svc-%d", req.ServiceID)
	webShareMu.Lock()
	defer webShareMu.Unlock()
	webShareCleanupLocked()
	meta := webShareBySvc[key]
	if meta == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "暂无进行中的操作会话"})
		return
	}
	// 校验操作会话仍有效，失效则清理
	if _, err := services.GetWebSessionFull(meta.BaseTK); err != nil {
		for alias, m := range webShareByAlias {
			if m == meta {
				delete(webShareByAlias, alias)
				delete(webAliasRole, alias)
			}
		}
		delete(webShareBySvc, key)
		c.JSON(http.StatusNotFound, gin.H{"error": "暂无进行中的操作会话"})
		return
	}
	alias := "v" + generateToken()
	webShareByAlias[alias] = meta
	webAliasRole[alias] = "viewer"
	meta.LastSeen[alias] = time.Now()
	c.JSON(http.StatusOK, gin.H{
		"tk":       alias,
		"role":     "viewer",
		"operator": meta.OperatorUser,
	})
}

// TakeWebControlHandler 接管控制：观看者升级为操作者，原操作者降级为观看者
func TakeWebControlHandler(c *gin.Context) {
	var req struct {
		TK string `json:"tk"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.TK == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 tk"})
		return
	}
	webShareMu.Lock()
	defer webShareMu.Unlock()
	meta := webShareByAlias[req.TK]
	if meta == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在或已结束"})
		return
	}
	old := meta.OperatorTK
	if old != req.TK {
		webAliasRole[old] = "viewer"
	}
	webAliasRole[req.TK] = "operator"
	meta.OperatorTK = req.TK
	meta.OperatorUser = currentUsername(c)
	meta.LastSeen[req.TK] = time.Now()
	c.JSON(http.StatusOK, gin.H{"ok": true, "role": "operator"})
}

// injectViewerOverlay 在观看模式的HTML页面注入只读遮罩与提示条
func injectViewerOverlay(html string) string {
	overlay := `<style>html,body{pointer-events:none!important;user-select:none!important}</style>` +
		`<div style="position:fixed;top:0;left:0;right:0;z-index:2147483647;background:rgba(230,162,60,.95);` +
		`color:#fff;padding:6px;text-align:center;font-size:13px;font-family:sans-serif;">` +
		`&#128065; 观看模式（只读）· 如需操作请在工具栏点击「申请控制」</div>`
	if strings.Contains(html, "<head>") {
		return strings.Replace(html, "<head>", "<head>"+overlay, 1)
	}
	return overlay + html
}

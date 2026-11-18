package api

import (
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"ops-center/internal/mcp"

	"github.com/gin-gonic/gin"
)

// MCP 管理接口（系统管理页用，仅管理员）
const (
	mcpConfigPath = mcp.DefaultConfigPath
	mcpAuditPath  = "/home/lt/ops-center/mcp-audit.log"
	mcpAddr       = "127.0.0.1:7780"
)

func mcpAdminGuard(c *gin.Context) bool {
	if c.GetString("role") != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员可用"})
		return false
	}
	return true
}

func mcpLoad() *mcp.Config {
	cfg, err := mcp.LoadConfig(mcpConfigPath)
	if err != nil {
		return &mcp.Config{
			Listen: ":7780", ActionsEnabled: true,
			Levels: map[string]bool{"L0": true, "L1": true, "L2": true, "L3": false},
		}
	}
	return cfg
}

// mcpRunning 探测 mcp-server 进程是否存活（端口可达即视为运行）
func mcpRunning() (bool, string) {
	conn, err := net.DialTimeout("tcp", mcpAddr, 500*time.Millisecond)
	if err != nil {
		return false, ""
	}
	conn.Close()
	out, _ := exec.Command("sh", "-c", "pgrep -x mcp-server | head -1").Output()
	return true, strings.TrimSpace(string(out))
}

// McpStatusHandler GET /api/mcp/status
func McpStatusHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	cfg := mcpLoad()
	running, pid := mcpRunning()
	auditLines := 0
	if b, err := os.ReadFile(mcpAuditPath); err == nil {
		auditLines = strings.Count(string(b), "\n")
	}
	toolCount := 0
	for _, lv := range cfg.Levels {
		if lv {
			toolCount++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"running": running, "pid": pid, "listen": cfg.Listen,
		"actions_enabled": cfg.ActionsEnabled, "levels": cfg.Levels,
		"token_count": len(cfg.Tokens), "audit_lines": auditLines,
		"open_levels": toolCount,
	})
}

// McpGetConfigHandler GET /api/mcp/config（token 只回掩码）
func McpGetConfigHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	cfg := mcpLoad()
	toks := []gin.H{}
	for _, t := range cfg.Tokens {
		masked := t.Key
		if len(masked) > 10 {
		}
		toks = append(toks, gin.H{
			"key": masked, "remark": t.Remark,
			"max_level": t.MaxLevel, "created_at": t.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"listen": cfg.Listen, "actions_enabled": cfg.ActionsEnabled,
		"levels": cfg.Levels, "tokens": toks,
	})
}

// McpSetLevelsHandler POST /api/mcp/levels {"L0":true,...}
func McpSetLevelsHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	var req map[string]bool
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	cfg := mcpLoad()
	for _, lv := range []string{"L0", "L1", "L2", "L3"} {
		if v, ok := req[lv]; ok {
			cfg.Levels[lv] = v
		}
	}
	if err := mcp.SaveConfig(mcpConfigPath, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "levels": cfg.Levels})
}

// McpSetActionsHandler POST /api/mcp/actions {"enabled":bool}
func McpSetActionsHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	cfg := mcpLoad()
	cfg.ActionsEnabled = *req.Enabled
	if err := mcp.SaveConfig(mcpConfigPath, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "actions_enabled": cfg.ActionsEnabled})
}

// McpAddTokenHandler POST /api/mcp/tokens {"remark","max_level"} → 明文 key 只返回一次
func McpAddTokenHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	var req struct {
		Remark   string `json:"remark"`
		MaxLevel string `json:"max_level"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	if mcp.LevelRank[req.MaxLevel] == 0 && req.MaxLevel != "L0" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "max_level 需为 L0-L3"})
		return
	}
	cfg := mcpLoad()
	t := mcp.NewToken(req.Remark, req.MaxLevel)
	cfg.Tokens = append(cfg.Tokens, t)
	if err := mcp.SaveConfig(mcpConfigPath, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "key": t.Key, "max_level": t.MaxLevel})
}

// McpRevokeTokenHandler POST /api/mcp/tokens/revoke {"key"}
func McpRevokeTokenHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误"})
		return
	}
	cfg := mcpLoad()
	if len(cfg.Tokens) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "至少保留一个 token"})
		return
	}
	left := cfg.Tokens[:0]
	removed := false
	for _, t := range cfg.Tokens {
		if t.Key == req.Key {
			removed = true
			continue
		}
		left = append(left, t)
	}
	if !removed {
		c.JSON(http.StatusNotFound, gin.H{"error": "token 不存在"})
		return
	}
	cfg.Tokens = left
	if err := mcp.SaveConfig(mcpConfigPath, cfg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "remaining": len(cfg.Tokens)})
}

// McpAuditHandler GET /api/mcp/audit?lines=100
func McpAuditHandler(c *gin.Context) {
	if !mcpAdminGuard(c) {
		return
	}
	n, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))
	if n <= 0 || n > 1000 {
		n = 100
	}
	b, err := os.ReadFile(mcpAuditPath)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"lines": []string{}})
		return
	}
	all := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	if len(all) > n {
		all = all[len(all)-n:]
	}
	c.JSON(http.StatusOK, gin.H{"lines": all, "total": len(all)})
}

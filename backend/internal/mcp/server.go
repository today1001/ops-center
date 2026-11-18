package mcp

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	gomcp "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"ops-center/internal/models"
)

// toolLevel 每个工具所属危险等级；未登记的工具按 L3 处理
var toolLevel = map[string]string{
	"list_servers":           "L0",
	"get_server_detail":      "L0",
	"list_services":          "L0",
	"network_test":           "L0",
	"scan_free_ips":          "L1",
	"scan_server_ports":      "L1",
	"add_ports_as_services":  "L2",
	"add_free_ips_as_servers": "L2",
	"delete_service":         "L3",
}

// Server MCP 服务器：复用主平台 models/services 层，
// 凭据解密只发生在 services 内部，任何工具返回值都不含明文密码。
type Server struct {
	mcp       *mcpserver.MCPServer
	auditFile *os.File

	cfgPath string
	cfgMod  time.Time
	mu      sync.RWMutex
	cfg     *Config
}

func (s *Server) config() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// reloadIfChanged 配置文件 mtime 变化则热加载（管理页改级别/令牌无需重启）
func (s *Server) reloadIfChanged() {
	st, err := os.Stat(s.cfgPath)
	if err != nil || st.ModTime().Equal(s.cfgMod) {
		return
	}
	cfg, err := LoadConfig(s.cfgPath)
	if err != nil {
		log.Printf("[MCP] 配置热加载失败(沿用旧配置): %v", err)
		return
	}
	s.mu.Lock()
	s.cfg, s.cfgMod = cfg, st.ModTime()
	s.mu.Unlock()
	log.Printf("[MCP] 配置已热加载（levels=%v actions=%v tokens=%d）",
		cfg.Levels, cfg.ActionsEnabled, len(cfg.Tokens))
}

// Run 初始化 DB、注册工具、启动 HTTP 服务（阻塞）
func Run(cfgPath string) error {
	if err := models.InitDB(); err != nil {
		return fmt.Errorf("InitDB: %w", err)
	}
	db := models.GetDB()
	// 双进程并发：启用 WAL + busy_timeout（WAL 持久化到库文件，主平台同受益）
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		log.Printf("[MCP] WAL 启用失败(不致命): %v", err)
	}
	db.Exec("PRAGMA busy_timeout = 5000")

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("加载配置: %w", err)
	}
	st, _ := os.Stat(cfgPath)
	s := &Server{cfg: cfg, cfgPath: cfgPath}
	if st != nil {
		s.cfgMod = st.ModTime()
	}

	s.mcp = mcpserver.NewMCPServer("ops-center-mcp", "1.0.0")
	s.registerReadTools()
	if cfg.ActionsEnabled {
		s.registerActionTools()
	}

	f, err := os.OpenFile(cfg.AuditLog, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		s.auditFile = f
		defer f.Close()
	}

	handler := s.authMiddleware(mcpserver.NewStreamableHTTPServer(s.mcp, mcpserver.WithStateLess(true)))
	log.Printf("[MCP] 运维 MCP 服务器已启动，监听 %s（动作工具: %v，级别: L0=%v L1=%v L2=%v L3=%v）",
		cfg.Listen, cfg.ActionsEnabled, cfg.Levels["L0"], cfg.Levels["L1"], cfg.Levels["L2"], cfg.Levels["L3"])
	return http.ListenAndServe(cfg.Listen, handler)
}

type mcpReqInfo struct {
	Method string `json:"method"`
	Params struct {
		Name string `json:"name"`
	} `json:"params"`
}

// authMiddleware 认证 + 危险等级双重限制 + 配置热加载
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.reloadIfChanged()
		cfg := s.config()

		// 1. Bearer Token 校验（常数时间比较）
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		key := strings.TrimPrefix(auth, "Bearer ")
		valid := false
		for _, t := range cfg.Tokens {
			if subtle.ConstantTimeCompare([]byte(key), []byte(t.Key)) == 1 {
				valid = true
				break
			}
		}
		if !valid {
			s.auditLog("AUTH", nil, "denied", 0)
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		maxLevel := cfg.TokenMaxLevel(key)

		// 2. 解析请求体，判断 tools/call 的目标工具级别
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		r.Body = io.NopCloser(bytes.NewReader(body))
		var req mcpReqInfo
		json.Unmarshal(body, &req)

		if req.Method == "tools/call" {
			level := toolLevel[req.Params.Name]
			if level == "" {
				level = "L3" // 未知工具按最高危处理
			}
			if level >= "L2" && !cfg.ActionsEnabled {
				s.auditLog(req.Params.Name, nil, "denied(actions_off)", 0)
				http.Error(w, `{"error":"写入类工具已被管理员禁用(actions_enabled=false)"}`, http.StatusForbidden)
				return
			}
			if !cfg.Levels[level] {
				s.auditLog(req.Params.Name, nil, "denied(level_off)", 0)
				http.Error(w, fmt.Sprintf(`{"error":"%s 级工具已被管理员禁用"}`, level), http.StatusForbidden)
				return
			}
			if LevelRank[maxLevel] < LevelRank[level] {
				s.auditLog(req.Params.Name, nil, "denied(token_level)", 0)
				http.Error(w, fmt.Sprintf(`{"error":"该 token 最高允许 %s，不足以调用 %s 级工具"}`, maxLevel, level), http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// toolHandler 统一的工具处理函数签名：入参为参数表，出参为纯文本结果
type toolHandler func(ctx context.Context, args map[string]any) (string, error)

// tool 注册工具并包装审计；工具内部错误以 ToolResultError 返回（不打断 MCP 会话）
func (s *Server) tool(name, desc string, h toolHandler, params ...gomcp.ToolOption) {
	opts := append([]gomcp.ToolOption{gomcp.WithDescription(desc)}, params...)
	s.mcp.AddTool(gomcp.NewTool(name, opts...), func(ctx context.Context, req gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		start := time.Now()
		args := req.GetArguments()
		out, err := h(ctx, args)
		status := "ok"
		if err != nil {
			status = "error"
		}
		s.auditLog(name, args, status, time.Since(start).Milliseconds())
		if err != nil {
			return gomcp.NewToolResultError(err.Error()), nil
		}
		return gomcp.NewToolResultText(out), nil
	})
}

// auditLog 追加一条 JSON Lines 审计记录
func (s *Server) auditLog(tool string, args map[string]any, status string, ms int64) {
	if s.auditFile == nil {
		return
	}
	entry := map[string]any{
		"time":   time.Now().Format("2006-01-02 15:04:05"),
		"tool":   tool,
		"args":   args,
		"status": status,
		"ms":     ms,
	}
	b, _ := json.Marshal(entry)
	s.auditFile.Write(append(b, '\n'))
}

// ---- 参数取值辅助（兼容数字以 float64 传入的 JSON 惯例）----

func argInt(args map[string]any, key string) (int, bool) {
	v, ok := args[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case string:
		var i int
		_, err := fmt.Sscanf(n, "%d", &i)
		return i, err == nil
	}
	return 0, false
}

func argString(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return ""
}

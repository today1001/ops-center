package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	guacBase      = "http://127.0.0.1:8085/guacamole"
	guacAdminUser = "guacadmin"
	guacAdminPass = "guacadmin"
)

// GuacProxyHandler /guac/* 反向代理到本地 Guacamole
// 普通 HTTP 用 http.Client 转发；WebSocket 升级用 gorilla 双向透传
func GuacProxyHandler(c *gin.Context) {
	path := strings.TrimPrefix(c.Request.URL.Path, "/guac")
	if path == "" {
		path = "/"
	}
	targetURL := "http://127.0.0.1:8085/guacamole" + path
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// WebSocket 升级请求：双向透传
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		proxyWebSocket(c, targetURL)
		return
	}

	// 普通 HTTP
	var reqBody []byte
	if c.Request.Body != nil {
		reqBody, _ = io.ReadAll(io.LimitReader(c.Request.Body, 50<<20))
	}
	proxyReq, _ := http.NewRequest(c.Request.Method, targetURL, bytes.NewReader(reqBody))
	proxyReq.Header.Set("Guacamole-Token", c.GetHeader("Guacamole-Token"))
	proxyReq.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	proxyReq.Header.Set("Accept", c.GetHeader("Accept"))
	if cookie := c.GetHeader("Cookie"); cookie != "" {
		proxyReq.Header.Set("Cookie", cookie)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "guacamole proxy: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50<<20))
	for k, vs := range resp.Header {
		if strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") {
			continue
		}
		for _, v := range vs {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(body)
}

// proxyWebSocket WebSocket 双向代理
// proxyWebSocket WebSocket 双向代理
// 观看模式（GUAC_TYPE=s 或 GUAC_SHADOW=1）时，客户端→上游方向的交互指令会被过滤
func proxyWebSocket(c *gin.Context, targetURL string) {
	shadow := c.Query("GUAC_TYPE") == "s" || c.Query("GUAC_SHADOW") == "1"
	if shadow {
		log.Printf("[GUAC] 观看模式隧道（输入过滤已启用）")
	}

	// 升级客户端连接
	up := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
		Subprotocols: []string{"guacamole"},
	}
	clientWS, err := up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[GUAC] ws upgrade failed: %v", err)
		return
	}
	defer clientWS.Close()

	// 连接上游 guacamole
	httpURL := "http://127.0.0.1:8085" + strings.TrimPrefix(targetURL, "http://127.0.0.1:8085")
	wsURL := "ws" + strings.TrimPrefix(httpURL, "http")
	header := http.Header{}
	header.Set("Sec-WebSocket-Protocol", "guacamole")
	// 转发必要的头，防止上游拒绝握手
	if origin := c.GetHeader("Origin"); origin != "" {
		header.Set("Origin", origin)
	}
	if cookie := c.GetHeader("Cookie"); cookie != "" {
		header.Set("Cookie", cookie)
	}
	if ua := c.GetHeader("User-Agent"); ua != "" {
		header.Set("User-Agent", ua)
	}
	serverWS, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		log.Printf("[GUAC] ws dial upstream failed: %v", err)
		return
	}
	defer serverWS.Close()

	// 双向透传（Text 和 Binary 帧原样转发）
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		first := true
		for {
			mt, data, err := serverWS.ReadMessage()
			if err != nil {
				return
			}
			if first && mt == websocket.TextMessage {
				first = false
				recordGuacTunnel(c, data)
			}
			if err := clientWS.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			mt, data, err := clientWS.ReadMessage()
			if err != nil {
				return
			}
			if shadow && mt == websocket.TextMessage {
				data = filterGuacInput(data)
			}
			if len(data) == 0 {
				continue
			}
			if err := serverWS.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}()
	<-done
	<-done
}

// guacLogin 登录 Guacamole 获取认证 token
func guacLogin() (string, error) {
	resp, err := http.PostForm(guacBase+"/api/tokens", url.Values{
		"username": {guacAdminUser},
		"password": {guacAdminPass},
	})
	if err != nil {
		return "", fmt.Errorf("连接 Guacamole 失败: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Guacamole 登录失败 (%d): %s", resp.StatusCode, string(body))
	}
	var out struct {
		AuthToken string `json:"authToken"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.AuthToken == "" {
		return "", fmt.Errorf("Guacamole 登录响应无效")
	}
	return out.AuthToken, nil
}

// guacAPI 带 token 调用 Guacamole REST API
func guacAPI(method, path, token string, payload any) ([]byte, int, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, err
		}
		body = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, guacBase+path, body)
	req.Header.Set("Guacamole-Token", token)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return data, resp.StatusCode, nil
}

// guacFindConnection 查找已有连接
// guacFindConnection 按名称查找已有连接（rdp-<hostname>）
func guacFindConnection(token, hostname string) (string, bool, error) {
	data, status, err := guacAPI("GET", "/api/session/data/postgresql/connections", token, nil)
	if err != nil {
		return "", false, err
	}
	if status != 200 {
		return "", false, fmt.Errorf("获取连接列表失败 (%d)", status)
	}
	var conns map[string]struct {
		Identifier string `json:"identifier"`
		Name       string `json:"name"`
	}
	if err := json.Unmarshal(data, &conns); err != nil {
		return "", false, err
	}
	for _, c := range conns {
		if c.Name == "rdp-"+hostname {
			return c.Identifier, true, nil
		}
	}
	return "", false, nil
}

// guacCreateConnection 创建 RDP 连接
func guacCreateConnection(token, name, hostname string, port int, username, password string) (string, error) {
	if port == 0 {
		port = 3389
	}
	payload := map[string]any{
		"name":             name,
		"parentIdentifier": "ROOT",
		"protocol":         "rdp",
		"parameters": map[string]string{
			"hostname":    hostname,
			"port":        fmt.Sprintf("%d", port),
			"username":    username,
			"password":    password,
			"security":    "any",
			"ignore-cert": "true",
		},
		"attributes": map[string]string{
			"max-connections":          "",
			"max-connections-per-user": "",
			"weight":                   "",
			"failover-only":            "false",
			"guacd-encryption":         "false",
			"guacd-hostname":           "",
			"guacd-port":               "",
		},
	}
	data, status, err := guacAPI("POST", "/api/session/data/postgresql/connections", token, payload)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("创建连接失败 (%d): %s", status, string(data))
	}
	var out struct {
		Identifier string `json:"identifier"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	return out.Identifier, nil
}

// shadow=true 时为影子模式：返回共享令牌，观看者以只读方式加入当前活动会话
func CreateRDPSessionHandler(c *gin.Context) {
	var req struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
		Shadow   bool   `json:"shadow"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Host == "" || req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要参数"})
		return
	}

	if req.Port == 0 {
		req.Port = 3389
	}

	token, err := guacLogin()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	connID, found, err := guacFindConnection(token, req.Host)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if !found {
		name := "rdp-" + req.Host
		connID, err = guacCreateConnection(token, name, req.Host, req.Port, req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
	} else {
		// 复用已有连接：确保不被连接数限制卡住
		guacClearConnLimits(connID)
	}

	// 影子模式：为当前活动会话生成只读共享令牌
	if req.Shadow {
		shareToken, err := guacCreateShadowShare(token, connID)
		if err != nil {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"token":          token,
			"connection_id":  connID,
			"share_token":    shareToken,
			"guac_type":      "s",
			"data_source":    "postgresql",
			"guac_base":      "/guac",
			"websocket_path": "/guac/websocket-tunnel",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":          token,
		"connection_id":  connID,
		"data_source":    "postgresql",
		"guac_base":      "/guac",
		"websocket_path": "/guac/websocket-tunnel",
	})
}

// guacClearConnLimits 清除连接的最大连接数限制，避免 "already in use" 错误
func guacClearConnLimits(connID string) {
	token, err := guacLogin()
	if err != nil {
		return
	}
	_, _, _ = guacAPI("PATCH", "/api/session/data/postgresql/connections/"+connID, token, map[string]any{
		"attributes": map[string]string{
			"max-connections":          "",
			"max-connections-per-user": "",
			"weight":                   "",
		},
	})
}

// filterGuacInput 过滤客户端发送的交互指令（观看模式）
// 只保留心跳/同步/断开等非交互指令，丢弃键盘、鼠标、剪贴板、缩放、文件等
func filterGuacInput(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		// 解析 opcode 长度：若干数字 + '.'
		j := i
		for j < len(data) && data[j] >= '0' && data[j] <= '9' {
			j++
		}
		if j >= len(data) || data[j] != '.' {
			return out
		}
		opLen := 0
		for k := i; k < j; k++ {
			opLen = opLen*10 + int(data[k]-'0')
		}
		opStart := j + 1
		opEnd := opStart + opLen
		if opEnd > len(data) {
			return out
		}
		opcode := string(data[opStart:opEnd])
		// 找指令结束 ';'
		rel := bytes.IndexByte(data[opEnd:], ';')
		if rel < 0 {
			return out
		}
		end := opEnd + rel + 1
		switch opcode {
		case "key", "mouse", "clipboard", "size", "file", "pipe":
			// 交互类指令：观看模式丢弃
		default:
			out = append(out, data[i:end]...)
		}
		i = end
	}
	return out
}

// ===================== 操作员隧道登记（影子模式用） =====================

type guacTunnel struct {
	UUID  string
	Token string // 操作员连接时使用的 guacamole token（共享凭据必须用同一会话请求）
	Seen  time.Time
}

var (
	guacTunnelMu     sync.Mutex
	guacTunnelByConn = make(map[string]guacTunnel) // 连接ID -> 最近操作员隧道
)

// recordGuacTunnel 从上游首帧提取隧道UUID并登记
// 首帧格式: "0.,36.<uuid>;"（仅操作员连接 GUAC_TYPE=c 时记录）
func recordGuacTunnel(c *gin.Context, data []byte) {
	if c.Query("GUAC_TYPE") != "c" {
		return
	}
	connID := c.Query("GUAC_ID")
	if connID == "" {
		return
	}
	str := string(data)
	if len(data) < 42 || !strings.HasPrefix(str, "0.,36.") {
		return
	}
	uuid := str[6:42]
	guacTunnelMu.Lock()
	guacTunnelByConn[connID] = guacTunnel{UUID: uuid, Token: c.Query("token"), Seen: time.Now()}
	guacTunnelMu.Unlock()
	log.Printf("[GUAC] 记录连接 %s 的操作员隧道 %s", connID, uuid)
}

// guacCreateShadowShare 为指定连接的操作员当前会话生成只读共享密钥
func guacCreateShadowShare(token, connID string) (string, error) {
	profID, err := guacEnsureSharingProfile(token, connID)
	if err != nil {
		return "", err
	}
	// 定位操作员隧道
	guacTunnelMu.Lock()
	t, ok := guacTunnelByConn[connID]
	guacTunnelMu.Unlock()
	if !ok || time.Since(t.Seen) > time.Hour {
		return "", fmt.Errorf("未找到进行中的远程桌面会话，请先由操作员通过平台连接后再使用影子模式")
	}
	// 通过隧道生成共享凭据（必须使用操作员的 guacamole 会话令牌）
	data, status, err := guacAPI("GET", "/api/session/tunnels/"+t.UUID+"/activeConnection/sharingCredentials/"+profID, t.Token, nil)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("生成共享凭据失败 (%d): %s", status, string(data))
	}
	var out struct {
		Values struct {
			Key string `json:"key"`
		} `json:"values"`
	}
	if err := json.Unmarshal(data, &out); err != nil || out.Values.Key == "" {
		return "", fmt.Errorf("共享凭据响应无效: %s", string(data))
	}
	return out.Values.Key, nil
}

// guacActiveConnectionID 查找指定连接的活动会话ID，无活动返回空串
func guacActiveConnectionID(token, connID string) (string, error) {
	data, status, err := guacAPI("GET", "/api/session/data/postgresql/activeConnections", token, nil)
	if err != nil {
		return "", err
	}
	if status != 200 {
		return "", fmt.Errorf("获取活动会话失败 (%d)", status)
	}
	var acts map[string]struct {
		ConnectionIdentifier string `json:"connectionIdentifier"`
	}
	if err := json.Unmarshal(data, &acts); err != nil {
		return "", err
	}
	for id, a := range acts {
		if a.ConnectionIdentifier == connID {
			return id, nil
		}
	}
	return "", nil
}

// guacEnsureSharingProfile 确保连接存在名为 shadow-view 的共享配置（只读参数），返回其ID
// REST 创建失败（部分版本目录只读）时回退为直接写数据库
func guacEnsureSharingProfile(token, connID string) (string, error) {
	// 1. 已存在则直接返回
	data, status, err := guacAPI("GET", "/api/session/data/postgresql/connections/"+connID+"/sharingProfiles", token, nil)
	if err == nil && status == 200 {
		var profiles map[string]struct {
			Identifier string `json:"identifier"`
			Name       string `json:"name"`
		}
		if json.Unmarshal(data, &profiles) == nil {
			for _, p := range profiles {
				if p.Name == "shadow-view" {
					return p.Identifier, nil
				}
			}
		}
	}
	// 2. 尝试 REST 创建
	payload := map[string]any{
		"name":             "shadow-view",
		"parentIdentifier": connID,
		"parameters":       map[string]string{"read-only": "true"},
		"attributes":       map[string]string{},
	}
	data, status, err = guacAPI("POST", "/api/session/data/postgresql/connections/"+connID+"/sharingProfiles", token, payload)
	if err == nil && status == 200 {
		var out struct {
			Identifier string `json:"identifier"`
		}
		if json.Unmarshal(data, &out) == nil && out.Identifier != "" {
			return out.Identifier, nil
		}
	}
	// 3. 回退：直接写 Guacamole 数据库
	return guacDBEnsureSharingProfile(connID)
}

// guacDBEnsureSharingProfile 通过 psql 直接在 Guacamole 库中创建只读共享配置
func guacDBEnsureSharingProfile(connID string) (string, error) {
	id, err := strconv.Atoi(connID)
	if err != nil {
		return "", fmt.Errorf("无效的连接ID: %s", connID)
	}
	run := func(sql string) (string, error) {
		cmd := exec.Command("psql", "-h", "127.0.0.1", "-U", "guacamole", "-d", "guacamole_db", "-t", "-A", "-c", sql)
		cmd.Env = append(os.Environ(), "PGPASSWORD=guacamole123")
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	// 插入共享配置（若不存在）
	_, err = run(fmt.Sprintf(
		"insert into guacamole_sharing_profile (sharing_profile_name, primary_connection_id) select 'shadow-view', %d where not exists (select 1 from guacamole_sharing_profile where sharing_profile_name='shadow-view' and primary_connection_id=%d)",
		id, id))
	if err != nil {
		return "", fmt.Errorf("创建影子共享配置失败: %v", err)
	}
	// 只读参数（guacd 端忽略输入）
	_, _ = run(fmt.Sprintf(
		"insert into guacamole_sharing_profile_parameter (sharing_profile_id, parameter_name, parameter_value) select sharing_profile_id, 'read-only', 'true' from guacamole_sharing_profile where sharing_profile_name='shadow-view' and primary_connection_id=%d",
		id))
	// 查询ID
	out, err := run(fmt.Sprintf(
		"select sharing_profile_id from guacamole_sharing_profile where sharing_profile_name='shadow-view' and primary_connection_id=%d",
		id))
	if err != nil || out == "" {
		return "", fmt.Errorf("查询影子共享配置失败: %v (%s)", err, out)
	}
	return out, nil
}

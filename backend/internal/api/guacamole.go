package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
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
func proxyWebSocket(c *gin.Context, targetURL string) {
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
	serverWS, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		log.Printf("[GUAC] ws dial upstream failed: %v", err)
		return
	}
	defer serverWS.Close()

	// 双向透传（Text 和 Binary 帧原样转发）
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		var frames, textF, binF, bytes int64
		start := time.Now()
		for {
			mt, data, err := serverWS.ReadMessage()
			if err != nil {
				log.Printf("[GUAC] upstream->client end %ds: frames=%d text=%d bin=%d bytes=%d err=%v", int(time.Since(start).Seconds()), frames, textF, binF, bytes, err)
				return
			}
			frames++
			bytes += int64(len(data))
			if mt == websocket.TextMessage { textF++ } else { binF++ }
			if mt == websocket.TextMessage {
				preview := string(data[:min(len(data),200)])
				log.Printf("[GUAC] upstream frame#%d TEXT %dB: %s", frames, len(data), preview)
			} else {
				log.Printf("[GUAC] upstream frame#%d BIN %dB head: %x", frames, len(data), data[:min(len(data),30)])
			}
			if err := clientWS.WriteMessage(mt, data); err != nil {
				log.Printf("[GUAC] upstream->client write FAIL at frame#%d: %v", frames, err)
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		var frames int64
		start := time.Now()
		for {
			mt, data, err := clientWS.ReadMessage()
			if err != nil {
				log.Printf("[GUAC] client->upstream end %ds: frames=%d err=%v", int(time.Since(start).Seconds()), frames, err)
				return
			}
			frames++
			if frames <= 5 {
				if mt == websocket.TextMessage {
					log.Printf("[GUAC] client frame#%d TEXT %dB: %s", frames, len(data), string(data[:min(len(data),120)]))
				} else {
					log.Printf("[GUAC] client frame#%d BIN %dB head: %x", frames, len(data), data[:min(len(data),20)])
				}
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

// guacFindConnection 按 hostname 查找已有连接，返回 identifier
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

// CreateRDPSessionHandler 创建 RDP 会话：复用或创建 Guacamole 连接，返回 token + 连接ID
func CreateRDPSessionHandler(c *gin.Context) {
	var req struct {
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Host == "" || req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要参数"})
		return
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

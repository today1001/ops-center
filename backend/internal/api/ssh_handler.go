package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type sshSession struct {
	token    string
	host     string
	port     int
	user     string
	pass     string
	ws       *websocket.Conn
	sshConn  *ssh.Client
	session  *ssh.Session
	stdin    chan []byte
	done     chan struct{}
	once     sync.Once
}

var (
	sshSessions   = make(map[string]*sshSession)
	sshSessionsMu sync.Mutex
)

func createSSHSession(token, host string, port int, user, pass string) *sshSession {
	s := &sshSession{
		token: token,
		host:  host,
		port:  port,
		user:  user,
		pass:  pass,
		stdin: make(chan []byte, 64),
		done:  make(chan struct{}),
	}
	sshSessionsMu.Lock()
	sshSessions[token] = s
	sshSessionsMu.Unlock()
	return s
}

func getSSHSession(token string) *sshSession {
	sshSessionsMu.Lock()
	defer sshSessionsMu.Unlock()
	return sshSessions[token]
}

func removeSSHSession(token string) {
	sshSessionsMu.Lock()
	delete(sshSessions, token)
	sshSessionsMu.Unlock()
}

// CreateSSHSessionHandler 创建SSH WebSocket会话
func CreateSSHSessionHandler(c *gin.Context) {
	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Host == "" || req.User == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少必要参数"})
		return
	}
	if req.Port == 0 {
		req.Port = 22
	}

	token := generateToken()
	createSSHSession(token, req.Host, req.Port, req.User, req.Pass)

	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"host":     req.Host,
		"port":     req.Port,
		"user":     req.User,
		"timeout":  30,
	})
}

// SSHWebSocketHandler WebSocket SSH终端
func SSHWebSocketHandler(c *gin.Context) {
	tk := c.Param("tk")
	sess := getSSHSession(tk)
	if sess == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的SSH会话"})
		return
	}

	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[SSH-WS] WebSocket升级失败: %v", err)
		return
	}
	sess.ws = ws

	// 设置WebSocket心跳处理
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	ws.SetReadDeadline(time.Now().Add(60 * time.Second))

	// 解析终端尺寸
	cols, _ := strconv.Atoi(c.Query("cols"))
	rows, _ := strconv.Atoi(c.Query("rows"))
	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 40
	}

	// SSH连接
	addr := sess.host + ":" + strconv.Itoa(sess.port)
	sshConfig := &ssh.ClientConfig{
		User:            sess.user,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
		Auth: []ssh.AuthMethod{
			ssh.Password(sess.pass),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				ans := make([]string, len(questions))
				for i := range questions {
					ans[i] = sess.pass
				}
				return ans, nil
			}),
		},
	}

	log.Printf("[SSH-WS] 连接 %s@%s", sess.user, addr)
	sshClient, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31mSSH连接失败: "+err.Error()+"\x1b[0m\r\n"))
		ws.Close()
		removeSSHSession(tk)
		return
	}
	sess.sshConn = sshClient

	sshSession, err := sshClient.NewSession()
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m创建会话失败: "+err.Error()+"\x1b[0m\r\n"))
		ws.Close()
		sshClient.Close()
		removeSSHSession(tk)
		return
	}
	sess.session = sshSession

	// 设置终端
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sshSession.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m请求PTY失败: "+err.Error()+"\x1b[0m\r\n"))
		ws.Close()
		sshSession.Close()
		sshClient.Close()
		removeSSHSession(tk)
		return
	}

	stdinPipe, err := sshSession.StdinPipe()
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m获取stdin失败: "+err.Error()+"\x1b[0m\r\n"))
		ws.Close()
		sshSession.Close()
		sshClient.Close()
		removeSSHSession(tk)
		return
	}

	stdoutPipe, err := sshSession.StdoutPipe()
	if err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m获取stdout失败: "+err.Error()+"\x1b[0m\r\n"))
		ws.Close()
		stdinPipe.Close()
		sshSession.Close()
		sshClient.Close()
		removeSSHSession(tk)
		return
	}

	// 启动shell
	if err := sshSession.Shell(); err != nil {
		ws.WriteMessage(websocket.TextMessage, []byte("\r\n\x1b[31m启动shell失败: "+err.Error()+"\x1b[0m\r\n"))
		ws.Close()
		stdinPipe.Close()
		sshSession.Close()
		sshClient.Close()
		removeSSHSession(tk)
		return
	}

	log.Printf("[SSH-WS] SSH连接成功 %s@%s", sess.user, addr)

	// 服务端心跳：定期发送ping保持连接
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-sess.done:
				return
			case <-ticker.C:
				if err := ws.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// SSH stdout → WebSocket
	go func() {
		defer func() {
			sess.once.Do(func() { close(sess.done) })
		}()
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				ws.WriteMessage(websocket.BinaryMessage, buf[:n])
			}
			if err != nil {
				log.Printf("[SSH-WS] stdout读取结束: %v", err)
				return
			}
		}
	}()

	// WebSocket → SSH stdin
	go func() {
		defer func() {
			sess.once.Do(func() { close(sess.done) })
		}()
		for {
			msgType, msg, err := ws.ReadMessage()
			if err != nil {
				log.Printf("[SSH-WS] WebSocket读取结束: %v", err)
				return
			}
			switch msgType {
			case websocket.BinaryMessage:
				select {
				case sess.stdin <- msg:
				default:
				}
			case websocket.TextMessage:
				// 处理控制消息（窗口大小调整等）
				var ctrl struct {
					Type string `json:"type"`
					Cols int    `json:"cols"`
					Rows int    `json:"rows"`
				}
				if json.Unmarshal(msg, &ctrl) == nil {
					switch ctrl.Type {
					case "resize":
						sshSession.WindowChange(ctrl.Rows, ctrl.Cols)
					case "ping":
						if ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"pong"}`)) != nil {
							return
						}
					}
				}
			}
		}
	}()

	// stdin写入协程
	go func() {
		for {
			select {
			case data := <-sess.stdin:
				stdinPipe.Write(data)
			case <-sess.done:
				return
			}
		}
	}()

	// 等待会话结束
	sshSession.Wait()
	ws.Close()
	stdinPipe.Close()
	sshClient.Close()
	removeSSHSession(tk)
	log.Printf("[SSH-WS] 会话结束 %s@%s", sess.user, addr)
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

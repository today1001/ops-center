package api

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var grdpUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// GrdpProxyHandler handles WebSocket connections for grdpwasm RDP proxy
func GrdpProxyHandler(c *gin.Context) {
	target := c.Query("target")
	if target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing target query parameter"})
		return
	}

	// 清理 target 中嵌套的 /ws?target= 部分（grdpwasm 客户端会重复拼接）
	for strings.Contains(target, "/ws?target=") {
		if i := strings.LastIndex(target, "/ws?target="); i >= 0 {
			target = target[i+len("/ws?target="):]
		} else {
			break
		}
	}

	wsConn, err := grdpUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.Error("upgrade", "err", err)
		return
	}
	defer wsConn.Close()

	dialer := &net.Dialer{
		KeepAlive: 30 * time.Second,
	}
	tcpConn, err := dialer.Dial("tcp", target)
	if err != nil {
		slog.Error("dial target", "target", target, "err", err)
		return
	}
	defer tcpConn.Close()

	slog.Info("proxying", "target", target, "remote", c.ClientIP())

	errc := make(chan error, 2)

	// WebSocket → TCP
	go func() {
		for {
			mt, data, err := wsConn.ReadMessage()
			if err != nil {
				errc <- err
				return
			}
			if mt == websocket.BinaryMessage || mt == websocket.TextMessage {
				if _, err := tcpConn.Write(data); err != nil {
					errc <- err
					return
				}
			}
		}
	}()

	// TCP → WebSocket
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := tcpConn.Read(buf)
			if n > 0 {
				if werr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					errc <- werr
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					errc <- err
				} else {
					errc <- nil
				}
				return
			}
		}
	}()

	if err := <-errc; err != nil {
		slog.Debug("proxy done", "err", err)
	}
}
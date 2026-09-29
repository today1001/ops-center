package services

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/exec"
	"time"

	"golang.org/x/crypto/ssh"

	"ops-center/internal/models"
)

// TestSSHConnection 测试SSH连接
func TestSSHConnection(server *models.Server) error {
	log.Printf("[SSH] 测试连接 %s:%d 用户: %s", server.IP, server.Port, server.Username)
	
	config, err := getSSHConfig(server)
	if err != nil {
		return fmt.Errorf("SSH配置错误: %v", err)
	}

	addr := fmt.Sprintf("%s:%d", server.IP, server.Port)
	log.Printf("[SSH] 连接到 %s", addr)
	
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("连接失败: %v", err)
	}
	defer conn.Close()

	log.Printf("[SSH] TCP连接成功，开始SSH握手")
	
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		return fmt.Errorf("SSH握手失败: %v", err)
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)
	go func() {
		for newChan := range chans {
			newChan.Reject(ssh.Prohibited, "连接已关闭")
		}
	}()

	log.Printf("[SSH] 连接测试成功")
	return nil
}

// getSSHConfig 获取SSH配置
func getSSHConfig(server *models.Server) (*ssh.ClientConfig, error) {
	config := &ssh.ClientConfig{
		User:            server.Username,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	if server.Password != "" {
		// 密码认证 + keyboard-interactive 兜底
		// 部分SSH服务器（如VMware ESXi的dropbear）不接受password方式，
		// 但接受keyboard-interactive方式，两者使用同一密码
		config.Auth = []ssh.AuthMethod{
			ssh.Password(server.Password),
			ssh.KeyboardInteractive(func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				ans := make([]string, len(questions))
				for i := range questions {
					ans[i] = server.Password
				}
				return ans, nil
			}),
		}
	} else if server.PrivateKey != "" {
		// 密钥认证
		signer, err := ssh.ParsePrivateKey([]byte(server.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("解析私钥失败: %v", err)
		}
		config.Auth = []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		}
	} else {
		return nil, fmt.Errorf("未配置认证方式")
	}

	return config, nil
}

// GetSSHClient 获取SSH客户端
func GetSSHClient(server *models.Server) (*ssh.Client, error) {
	config, err := getSSHConfig(server)
	if err != nil {
		return nil, err
	}

	addr := fmt.Sprintf("%s:%d", server.IP, server.Port)
	return ssh.Dial("tcp", addr, config)
}

// pingHost ping测试主机是否可达
func pingHost(ip string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", ip)
	return cmd.Run() == nil
}

// TestAndCollectServerInfo 测试连接并采集服务器信息
func TestAndCollectServerInfo(server *models.Server) error {
	log.Printf("[SSH] 开始测试服务器 %s (%s) 的连接", server.Name, server.IP)
	
	// 测试SSH连接
	if err := TestSSHConnection(server); err != nil {
		log.Printf("[SSH] 连接失败: %v", err)
		// 能ping通但SSH连不上 → unreachable（蓝色）；ping不通 → offline（红色）
		if pingHost(server.IP) {
			log.Printf("[SSH] %s 可ping通但SSH无法连接，标记为 unreachable", server.IP)
			models.UpdateServerStatus(server.ID, "unreachable")
		} else {
			models.UpdateServerStatus(server.ID, "offline")
		}
		return err
	}

	log.Printf("[SSH] 连接成功，开始采集系统信息")
	
	// 连接成功，采集系统信息
	detail, err := CollectSystemInfo(server)
	if err != nil {
		log.Printf("[SSH] 采集失败: %v", err)
		models.UpdateServerStatus(server.ID, "busy")
		return err
	}

	// 保存详情
	if err := models.CreateOrUpdateServerDetail(detail); err != nil {
		log.Printf("[SSH] 保存详情失败: %v", err)
		return err
	}

	log.Printf("[SSH] 采集完成: %s", detail.Hostname)
	
	// 更新服务器状态为active
	models.UpdateServerStatus(server.ID, "active")
	return nil
}

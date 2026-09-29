package main

import (
	"log"
	"ops-center/internal/api"
	"ops-center/internal/models"
)

func main() {
	// 初始化数据库
	if err := models.InitDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer models.CloseDB()

	// 创建默认管理员用户
	if err := models.CreateDefaultAdmin(); err != nil {
		log.Printf("Warning: Failed to create default admin: %v", err)
	}

	// 启动API服务器
	api.Run()
}

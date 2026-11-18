package main

import (
	"log"

	"ops-center/internal/api"
	"ops-center/internal/models"
	"ops-center/internal/services"
)

func main() {
	// 初始化数据库
	if err := models.InitDB(); err != nil {
		panic(err)
	}
	defer models.CloseDB()

	// 注入数据库连接到 api 包（供空闲 IP/端口功能使用）
	api.SetFreeDB(models.GetDB())

	// 空闲 IP 定时扫描：建表 + 后台循环（进程由 crontab 保活 = 开机自启，间隔可在页面配置）
	if err := services.EnsureFreeIPTables(models.GetDB()); err != nil {
		log.Printf("Warning: 初始化空闲IP扫描表失败: %v", err)
	}
	go services.StartFreeIPScanLoop(models.GetDB())

	// 启动API服务器
	api.Run()
}

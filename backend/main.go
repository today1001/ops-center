package main

import (
	"log"

	"ops-center/internal/api"
	"ops-center/internal/models"
	"ops-center/internal/services"
)

// BuildVersion 发布时 -ldflags 注入，如 -X main.BuildVersion=1.5.0
var BuildVersion = "1.5.0"

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

	// 自动升级：建表/目录 + 处理上次升级的挂起标记（确认成功或超时回滚）+ 检查循环
	if err := services.EnsureUpgradeTables(models.GetDB()); err != nil {
		log.Printf("Warning: 初始化升级日志表失败: %v", err)
	}
	services.EnsureUpgradeDirs()
	services.BackendVersion = BuildVersion
	services.HandlePendingUpgrade(models.GetDB())
	go services.RunAutoUpgrade(models.GetDB())

	// 启动API服务器
	api.Run()
}

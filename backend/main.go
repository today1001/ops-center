package main

import (
	"ops-center/internal/api"
	"ops-center/internal/models"
)

func main() {
	// 初始化数据库
	if err := models.InitDB(); err != nil {
		panic(err)
	}
	defer models.CloseDB()

	// 启动API服务器
	api.Run()
}

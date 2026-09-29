package main

import (
	"fmt"
	"ops-center/internal/models"
	"ops-center/internal/services"
)

func main() {
	models.InitDB()
	defer models.CloseDB()
	server, err := models.GetServerByID(1)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Server: %s (%s)\n", server.Name, server.IP)
	detail, err := services.CollectSystemInfo(server)
	if err != nil {
		fmt.Printf("Collect failed: %v\n", err)
		return
	}
	fmt.Printf("Hostname: %s\n", detail.Hostname)
	fmt.Printf("OS: %s\n", detail.OS)
	err = models.CreateOrUpdateServerDetail(detail)
	if err != nil {
		fmt.Printf("Save failed: %v\n", err)
		return
	}
	fmt.Println("Saved OK!")
}

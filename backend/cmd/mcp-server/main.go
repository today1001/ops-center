package main

import (
	"flag"
	"log"

	"ops-center/internal/mcp"
)

func main() {
	cfgPath := flag.String("config", mcp.DefaultConfigPath, "MCP 配置文件路径")
	flag.Parse()
	if err := mcp.Run(*cfgPath); err != nil {
		log.Fatal(err)
	}
}

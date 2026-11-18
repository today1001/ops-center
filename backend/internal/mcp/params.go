package mcp

import gomcp "github.com/mark3labs/mcp-go/mcp"

// 参数声明辅助
func strParam(name, desc string) gomcp.ToolOption {
	return gomcp.WithString(name, gomcp.Required(), gomcp.Description(desc))
}

func intParam(name, desc string) gomcp.ToolOption {
	return gomcp.WithNumber(name, gomcp.Required(), gomcp.Description(desc))
}

func boolParam(name, desc string) gomcp.ToolOption {
	return gomcp.WithBoolean(name, gomcp.Description(desc))
}

package models

import "time"

// User 用户模型
type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // 不序列化密码
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Server 服务器模型
type Server struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	IP         string    `json:"ip"`
	Port       int       `json:"port"`
	Username   string    `json:"username"`
	Password   string    `json:"password,omitempty"`
	PrivateKey string    `json:"private_key,omitempty"`
	OS         string    `json:"os"`
	Status     string    `json:"status"`     // active, busy, offline
	AuthType   string    `json:"auth_type"`  // password, key
	Tags       string    `json:"tags"`
	GroupName  string    `json:"group_name"` // 分组名称
	CreatedBy  int       `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ServerGroup 服务器分组模型
type ServerGroup struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	ServerCount int       `json:"server_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应
type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// MessageResponse 通用消息响应
type MessageResponse struct {
	Message string `json:"message"`
}

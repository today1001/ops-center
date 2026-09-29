package models

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sort"

	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

// InitDB 初始化数据库
func InitDB() error {
	// 获取可执行文件所在目录
	execPath, err := os.Executable()
	if err != nil {
		// 如果获取失败，使用当前目录
		execPath = "."
	}
	
	// 数据库路径：可执行文件同级目录下的data/ops-center.db
	dbDir := filepath.Join(filepath.Dir(execPath), "data")
	dbPath := filepath.Join(dbDir, "ops-center.db")
	
	// 确保data目录存在
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return err
	}

	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	// 创建表
	if err := createTables(); err != nil {
		return err
	}

	return nil
}

// CloseDB 关闭数据库连接
func CloseDB() {
	if db != nil {
		db.Close()
	}
}

// createTables 创建数据库表
func createTables() error {
	// 创建用户表
	userTable := `
	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		email TEXT,
		role TEXT DEFAULT 'user',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	
	if _, err := db.Exec(userTable); err != nil {
		return err
	}

	// 创建服务器表
	serverTable := `
	CREATE TABLE IF NOT EXISTS servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		ip TEXT NOT NULL,
		port INTEGER DEFAULT 22,
		username TEXT NOT NULL,
		password TEXT,
		private_key TEXT,
		os TEXT,
		status TEXT DEFAULT 'active',
		auth_type TEXT DEFAULT 'password',
		tags TEXT,
		group_name TEXT,
		created_by INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (created_by) REFERENCES users(id)
	);`
	
	if _, err := db.Exec(serverTable); err != nil {
		return err
	}

	// 添加auth_type字段（如果不存在）
	_, _ = db.Exec("ALTER TABLE servers ADD COLUMN auth_type TEXT DEFAULT 'password'")
	// 添加分组字段（如果不存在）
	_, _ = db.Exec("ALTER TABLE servers ADD COLUMN group_name TEXT")

	// 创建服务器分组表
	groupTable := `
	CREATE TABLE IF NOT EXISTS server_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(groupTable); err != nil {
		return err
	}

	// 将 servers 表中已有的 group_name 同步到 server_groups（兼容旧数据）
	syncExistingGroups()

	// 创建服务器详情表
	serverDetailTable := `
	CREATE TABLE IF NOT EXISTS server_details (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id INTEGER UNIQUE NOT NULL,
		hostname TEXT,
		os TEXT,
		os_version TEXT,
		kernel TEXT,
		uptime TEXT,
		cpu_model TEXT,
		cpu_cores INTEGER,
		cpu_usage REAL,
		memory_total INTEGER,
		memory_used INTEGER,
		memory_usage REAL,
		disk_total INTEGER,
		disk_used INTEGER,
		disk_usage REAL,
		network_ip TEXT,
		network_mac TEXT,
		network_gateway TEXT,
		network_dns TEXT,
		network_speed INTEGER,
		network_rx INTEGER,
		network_tx INTEGER,
		network_usage REAL,
		ports TEXT,
		services TEXT,
		last_checked DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
	);`
	
	if _, err := db.Exec(serverDetailTable); err != nil {
		return err
	}

	// 迁移：添加新字段（如果不存在）
	migrations := []string{
		"ALTER TABLE server_details ADD COLUMN network_gateway TEXT",
		"ALTER TABLE server_details ADD COLUMN network_dns TEXT",
		"ALTER TABLE server_details ADD COLUMN network_speed INTEGER",
		"ALTER TABLE server_details ADD COLUMN network_rx INTEGER",
		"ALTER TABLE server_details ADD COLUMN network_tx INTEGER",
		"ALTER TABLE server_details ADD COLUMN network_usage REAL",
	}
	for _, m := range migrations {
		db.Exec(m) // 忽略错误（字段已存在时会报错）
	}

	// 创建服务器服务表（服务管理模块）
	serviceTable := `
	CREATE TABLE IF NOT EXISTS server_services (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		server_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		access_method TEXT DEFAULT 'SSH',
		address TEXT,
		port INTEGER,
		username TEXT,
		password TEXT,
		status TEXT DEFAULT 'running',
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
	);`
	if _, err := db.Exec(serviceTable); err != nil {
		return err
	}

	// 创建服务检测记录表（连通率统计）
	serviceCheckTable := `
	CREATE TABLE IF NOT EXISTS service_checks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		service_id INTEGER NOT NULL,
		success INTEGER DEFAULT 0,
		response_time_ms INTEGER DEFAULT 0,
		version TEXT,
		message TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (service_id) REFERENCES server_services(id) ON DELETE CASCADE
	);`
	if _, err := db.Exec(serviceCheckTable); err != nil {
		return err
	}

	// 创建端口列表表（系统设置-服务访问方式）
	portProfileTable := `
	CREATE TABLE IF NOT EXISTS port_profiles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		port INTEGER UNIQUE NOT NULL,
		name TEXT NOT NULL,
		description TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(portProfileTable); err != nil {
		return err
	}
	// 填充默认端口列表
	seedDefaultPortProfiles()

	return nil
}

// syncExistingGroups 将 servers 表中已有的 group_name 同步到 server_groups 表
func syncExistingGroups() {
	rows, err := db.Query("SELECT DISTINCT group_name FROM servers WHERE group_name IS NOT NULL AND group_name != ''")
	if err != nil {
		return
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if rows.Scan(&n) == nil && n != "" {
			names = append(names, n)
		}
	}
	for _, n := range names {
		db.Exec("INSERT OR IGNORE INTO server_groups (name) VALUES (?)", n)
	}
}

// GetServerGroups 获取所有分组及服务器数量
func GetServerGroups() ([]ServerGroup, error) {
	rows, err := db.Query(`
		SELECT g.id, g.name, COALESCE(g.description, ''),
		       (SELECT COUNT(*) FROM servers s WHERE s.group_name = g.name) AS server_count,
		       g.created_at, g.updated_at
		FROM server_groups g ORDER BY g.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []ServerGroup
	for rows.Next() {
		var g ServerGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.ServerCount, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GetServerGroupByID 根据ID获取分组
func GetServerGroupByID(id int) (*ServerGroup, error) {
	g := &ServerGroup{}
	err := db.QueryRow(`
		SELECT g.id, g.name, COALESCE(g.description, ''),
		       (SELECT COUNT(*) FROM servers s WHERE s.group_name = g.name) AS server_count,
		       g.created_at, g.updated_at
		FROM server_groups g WHERE g.id = ?`, id).
		Scan(&g.ID, &g.Name, &g.Description, &g.ServerCount, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return g, nil
}

// CreateServerGroup 创建分组
func CreateServerGroup(name, description string) error {
	_, err := db.Exec("INSERT INTO server_groups (name, description) VALUES (?, ?)", name, description)
	return err
}

// UpdateServerGroup 更新分组，同时同步 servers 表中的 group_name
func UpdateServerGroup(id int, newName, description string, oldName string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("UPDATE server_groups SET name = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newName, description, id); err != nil {
		return err
	}
	// 同步更新所有该组下的服务器
	if oldName != newName {
		if _, err := tx.Exec("UPDATE servers SET group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE group_name = ?", newName, oldName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteServerGroup 删除分组，并将该组下服务器置为未分组
func DeleteServerGroup(id int) error {
	g, err := GetServerGroupByID(id)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM server_groups WHERE id = ?", id); err != nil {
		return err
	}
	// 该组下服务器归入未分组
	if _, err := tx.Exec("UPDATE servers SET group_name = '', updated_at = CURRENT_TIMESTAMP WHERE group_name = ?", g.Name); err != nil {
		return err
	}
	return tx.Commit()
}

// seedDefaultPortProfiles 填充默认端口访问方式列表
func seedDefaultPortProfiles() {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM port_profiles").Scan(&count); err != nil || count > 0 {
		return
	}
	defaults := []struct {
		Port int
		Name string
		Desc string
	}{
		{80, "WEB", "HTTP Web服务"},
		{443, "HTTPS", "HTTPS加密Web服务"},
		{22, "SSH", "SSH远程登录"},
		{21, "FTP", "FTP文件传输"},
		{3389, "RDP", "Windows远程桌面"},
		{3306, "MySQL", "MySQL数据库"},
		{5432, "PostgreSQL", "PostgreSQL数据库"},
		{6379, "Redis", "Redis缓存服务"},
	}
	for _, d := range defaults {
		db.Exec("INSERT INTO port_profiles (port, name, description) VALUES (?, ?, ?)", d.Port, d.Name, d.Desc)
	}
}

// CreateDefaultAdmin 创建默认管理员用户
func CreateDefaultAdmin() error {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		if err != nil {
			return err
		}

		_, err = db.Exec(
			"INSERT INTO users (username, password, email, role) VALUES (?, ?, ?, ?)",
			"admin", string(hashedPassword), "admin@ops-center.local", "admin",
		)
		if err != nil {
			return err
		}

		log.Println("Default admin user created: admin/admin123")
	}

	return nil
}

// GetUserByUsername 根据用户名获取用户
func GetUserByUsername(username string) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		"SELECT id, username, password, email, role, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Password, &user.Email, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID 根据ID获取用户
func GetUserByID(id int) (*User, error) {
	user := &User{}
	err := db.QueryRow(
		"SELECT id, username, password, email, role, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Username, &user.Password, &user.Email, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetServers 获取服务器列表
func GetServers(userID int) ([]Server, error) {
	rows, err := db.Query(
		"SELECT id, name, ip, port, username, os, status, auth_type, tags, COALESCE(group_name, ''), created_by, created_at, updated_at FROM servers WHERE created_by = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var s Server
		err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.OS, &s.Status, &s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}

	return servers, nil
}

// GetServersWithCreds 获取服务器列表（含密码/私钥，用于后台采集）
func GetServersWithCreds(userID int) ([]Server, error) {
	rows, err := db.Query(
		"SELECT id, name, ip, port, username, password, private_key, os, status, auth_type, tags, COALESCE(group_name, ''), created_by, created_at, updated_at FROM servers WHERE created_by = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var s Server
		err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.Password, &s.PrivateKey, &s.OS, &s.Status, &s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}

	return servers, nil
}

// UpdateServerStatus 仅更新服务器状态，不覆盖其他字段
func UpdateServerStatus(id int, status string) error {
	_, err := db.Exec(
		"UPDATE servers SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, id,
	)
	return err
}

// GetServerByID 根据ID获取服务器
func GetServerByID(id int) (*Server, error) {
	s := &Server{}
	err := db.QueryRow(
		"SELECT id, name, ip, port, username, password, private_key, os, status, auth_type, tags, COALESCE(group_name, ''), created_by, created_at, updated_at FROM servers WHERE id = ?",
		id,
	).Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.Password, &s.PrivateKey, &s.OS, &s.Status, &s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// CreateServer 创建服务器
func CreateServer(server *Server) error {
	result, err := db.Exec(
		"INSERT INTO servers (name, ip, port, username, password, private_key, os, status, auth_type, tags, group_name, created_by) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		server.Name, server.IP, server.Port, server.Username, server.Password, server.PrivateKey, server.OS, server.Status, server.AuthType, server.Tags, server.GroupName, server.CreatedBy,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	server.ID = int(id)
	return nil
}

// UpdateServer 更新服务器
func UpdateServer(server *Server) error {
	_, err := db.Exec(
		"UPDATE servers SET name = ?, ip = ?, port = ?, username = ?, password = ?, private_key = ?, os = ?, status = ?, auth_type = ?, tags = ?, group_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		server.Name, server.IP, server.Port, server.Username, server.Password, server.PrivateKey, server.OS, server.Status, server.AuthType, server.Tags, server.GroupName, server.ID,
	)
	return err
}

// DeleteServer 删除服务器
func DeleteServer(id int) error {
	_, err := db.Exec("DELETE FROM servers WHERE id = ?", id)
	return err
}

// GetServerDetail 获取服务器详情
func GetServerDetail(serverID int) (*ServerDetail, error) {
	detail := &ServerDetail{}
	err := db.QueryRow(`
		SELECT id, server_id, hostname, os, os_version, kernel, uptime, 
			   cpu_model, cpu_cores, cpu_usage, memory_total, memory_used, memory_usage,
			   disk_total, disk_used, disk_usage, network_ip, network_mac, 
			   network_gateway, network_dns, network_speed, network_rx, network_tx, network_usage,
			   ports, services, last_checked, created_at, updated_at 
		FROM server_details WHERE server_id = ?`, serverID).Scan(
		&detail.ID, &detail.ServerID, &detail.Hostname, &detail.OS, &detail.OSVersion,
		&detail.Kernel, &detail.Uptime, &detail.CPUModel, &detail.CPUCores, &detail.CPUUsage,
		&detail.MemoryTotal, &detail.MemoryUsed, &detail.MemoryUsage,
		&detail.DiskTotal, &detail.DiskUsed, &detail.DiskUsage,
		&detail.NetworkIP, &detail.NetworkMAC,
		&detail.NetworkGateway, &detail.NetworkDNS, &detail.NetworkSpeed, &detail.NetworkRX, &detail.NetworkTX, &detail.NetworkUsage,
		&detail.Ports, &detail.Services,
		&detail.LastChecked, &detail.CreatedAt, &detail.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return detail, nil
}

// CreateOrUpdateServerDetail 创建或更新服务器详情
func CreateOrUpdateServerDetail(detail *ServerDetail) error {
	_, err := db.Exec(`
		INSERT INTO server_details (server_id, hostname, os, os_version, kernel, uptime, 
									cpu_model, cpu_cores, cpu_usage, memory_total, memory_used, memory_usage,
									disk_total, disk_used, disk_usage, network_ip, network_mac, 
									network_gateway, network_dns, network_speed, network_rx, network_tx, network_usage,
									ports, services, last_checked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(server_id) DO UPDATE SET
			hostname = excluded.hostname,
			os = excluded.os,
			os_version = excluded.os_version,
			kernel = excluded.kernel,
			uptime = excluded.uptime,
			cpu_model = excluded.cpu_model,
			cpu_cores = excluded.cpu_cores,
			cpu_usage = excluded.cpu_usage,
			memory_total = excluded.memory_total,
			memory_used = excluded.memory_used,
			memory_usage = excluded.memory_usage,
			disk_total = excluded.disk_total,
			disk_used = excluded.disk_used,
			disk_usage = excluded.disk_usage,
			network_ip = excluded.network_ip,
			network_mac = excluded.network_mac,
			network_gateway = excluded.network_gateway,
			network_dns = excluded.network_dns,
			network_speed = excluded.network_speed,
			network_rx = excluded.network_rx,
			network_tx = excluded.network_tx,
			network_usage = excluded.network_usage,
			ports = excluded.ports,
			services = excluded.services,
			last_checked = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP`,
		detail.ServerID, detail.Hostname, detail.OS, detail.OSVersion,
		detail.Kernel, detail.Uptime, detail.CPUModel, detail.CPUCores, detail.CPUUsage,
		detail.MemoryTotal, detail.MemoryUsed, detail.MemoryUsage,
		detail.DiskTotal, detail.DiskUsed, detail.DiskUsage,
		detail.NetworkIP, detail.NetworkMAC,
		detail.NetworkGateway, detail.NetworkDNS,
		detail.NetworkSpeed, detail.NetworkRX, detail.NetworkTX, detail.NetworkUsage,
		detail.Ports, detail.Services)
	return err
}

// GetResourceStats 汇总所有服务器资源总量和利用率
func GetResourceStats() (*ResourceStats, error) {
	stats := &ResourceStats{}

	// 服务器总数
	if err := db.QueryRow("SELECT COUNT(*) FROM servers").Scan(&stats.ServerCount); err != nil {
		return nil, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM server_details").Scan(&stats.DetailCount); err != nil {
		return nil, err
	}

	// 汇总CPU、内存、磁盘、网络（只统计已有详情的服务器）
	rows, err := db.Query("SELECT cpu_cores, cpu_usage, memory_total, memory_used, memory_usage, disk_total, disk_used, disk_usage, network_speed, network_rx, network_tx FROM server_details")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cpuSum int
	var cpuUsageSum float64
	var cpuCount int
	var netUsageSum float64
	var netCount int
	for rows.Next() {
		var cores int
		var cpuUsage, memUsage, diskUsage float64
		var memTotal, memUsed, diskTotal, diskUsed int64
		var netSpeed, netRX, netTX int
		if err := rows.Scan(&cores, &cpuUsage, &memTotal, &memUsed, &memUsage, &diskTotal, &diskUsed, &diskUsage, &netSpeed, &netRX, &netTX); err != nil {
			continue
		}
		cpuSum += cores
		if cpuUsage > 0 {
			cpuUsageSum += cpuUsage
			cpuCount++
		}
		stats.TotalMemory += memTotal
		stats.UsedMemory += memUsed
		stats.TotalDisk += diskTotal
		stats.UsedDisk += diskUsed
		stats.TotalNetworkSpeed += netSpeed
		stats.TotalNetworkRX += netRX
		stats.TotalNetworkTX += netTX
		if netSpeed > 0 {
			usage := float64(netRX)
			if netTX > netRX {
				usage = float64(netTX)
			}
			netUsageSum += usage / float64(netSpeed) * 100
			netCount++
		}
	}

	stats.TotalCPUCores = cpuSum
	if cpuCount > 0 {
		stats.AvgCPUUsage = round1(cpuUsageSum / float64(cpuCount))
	}
	if stats.TotalMemory > 0 {
		stats.MemoryUsage = round1(float64(stats.UsedMemory) / float64(stats.TotalMemory) * 100)
	}
	if stats.TotalDisk > 0 {
		stats.DiskUsage = round1(float64(stats.UsedDisk) / float64(stats.TotalDisk) * 100)
	}
	if netCount > 0 {
		stats.NetworkUsage = round1(netUsageSum / float64(netCount))
	}

	return stats, nil
}

// round1 保留一位小数
func round1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}

// ServerUsage 服务器单项资源利用率
type ServerUsage struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	IP           string  `json:"ip"`
	CPUUsage     float64 `json:"cpu_usage"`
	MemoryUsage  float64 `json:"memory_usage"`
	DiskUsage    float64 `json:"disk_usage"`
	NetworkUsage float64 `json:"network_usage"`
}

// TopBottom 最高/最低排行
type TopBottom struct {
	Top    []ServerUsage `json:"top"`
	Bottom []ServerUsage `json:"bottom"`
}

// ResourceRankings 资源利用率排行
type ResourceRankings struct {
	CPU    TopBottom `json:"cpu"`
	Memory TopBottom `json:"memory"`
	Disk   TopBottom `json:"disk"`
	Network TopBottom `json:"network"`
}

// GetResourceRankings 获取CPU/内存/磁盘/网络利用率最高的5台和最低的5台
func GetResourceRankings() (*ResourceRankings, error) {
	rows, err := db.Query(`
		SELECT s.id, s.name, s.ip, d.cpu_usage, d.memory_usage, d.disk_usage, d.network_usage
		FROM server_details d JOIN servers s ON s.id = d.server_id
		WHERE s.status != 'offline'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []ServerUsage
	for rows.Next() {
		var u ServerUsage
		if err := rows.Scan(&u.ID, &u.Name, &u.IP, &u.CPUUsage, &u.MemoryUsage, &u.DiskUsage, &u.NetworkUsage); err != nil {
			continue
		}
		u.CPUUsage = round1(u.CPUUsage)
		u.MemoryUsage = round1(u.MemoryUsage)
		u.DiskUsage = round1(u.DiskUsage)
		u.NetworkUsage = round1(u.NetworkUsage)
		all = append(all, u)
	}

	rankings := &ResourceRankings{}

	// CPU
	cpuSorted := make([]ServerUsage, len(all))
	copy(cpuSorted, all)
	sort.Slice(cpuSorted, func(i, j int) bool { return cpuSorted[i].CPUUsage > cpuSorted[j].CPUUsage })
	rankings.CPU.Top = top(cpuSorted, cpuSorted, func(u ServerUsage) bool { return u.CPUUsage > 0 })
	rankings.CPU.Bottom = bottom(cpuSorted, func(u ServerUsage) bool { return u.CPUUsage > 0 })

	// 内存
	memSorted := make([]ServerUsage, len(all))
	copy(memSorted, all)
	sort.Slice(memSorted, func(i, j int) bool { return memSorted[i].MemoryUsage > memSorted[j].MemoryUsage })
	rankings.Memory.Top = top(memSorted, memSorted, func(u ServerUsage) bool { return u.MemoryUsage > 0 })
	rankings.Memory.Bottom = bottom(memSorted, func(u ServerUsage) bool { return u.MemoryUsage > 0 })

	// 磁盘
	diskSorted := make([]ServerUsage, len(all))
	copy(diskSorted, all)
	sort.Slice(diskSorted, func(i, j int) bool { return diskSorted[i].DiskUsage > diskSorted[j].DiskUsage })
	rankings.Disk.Top = top(diskSorted, diskSorted, func(u ServerUsage) bool { return u.DiskUsage > 0 })
	rankings.Disk.Bottom = bottom(diskSorted, func(u ServerUsage) bool { return u.DiskUsage > 0 })

	// 网络
	netSorted := make([]ServerUsage, len(all))
	copy(netSorted, all)
	sort.Slice(netSorted, func(i, j int) bool { return netSorted[i].NetworkUsage > netSorted[j].NetworkUsage })
	rankings.Network.Top = top(netSorted, netSorted, func(u ServerUsage) bool { return u.NetworkUsage > 0 })
	rankings.Network.Bottom = bottom(netSorted, func(u ServerUsage) bool { return u.NetworkUsage > 0 })

	return rankings, nil
}

// top 取利用率最高的前5台（排除0利用率的）
func top(sorted, all []ServerUsage, hasData func(ServerUsage) bool) []ServerUsage {
	var result []ServerUsage
	for _, u := range sorted {
		if !hasData(u) {
			continue
		}
		result = append(result, u)
		if len(result) >= 5 {
			break
		}
	}
	if len(result) == 0 && len(all) > 0 {
		result = []ServerUsage{}
	}
	return result
}

// bottom 取利用率最低的5台（排除0利用率的）
func bottom(sorted []ServerUsage, hasData func(ServerUsage) bool) []ServerUsage {
	var result []ServerUsage
	for i := len(sorted) - 1; i >= 0; i-- {
		u := sorted[i]
		if !hasData(u) {
			continue
		}
		result = append(result, u)
		if len(result) >= 5 {
			break
		}
	}
	return result
}

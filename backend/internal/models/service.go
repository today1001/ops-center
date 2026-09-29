package models

import (
	"database/sql"
	"time"
)

// ServerService 服务器上的服务模型
type ServerService struct {
	ID              int        `json:"id"`
	ServerID        int        `json:"server_id"`
	ServerName      string     `json:"server_name,omitempty"` // 联查时返回
	ServerIP        string     `json:"server_ip,omitempty"`   // 联查时返回
	Name            string     `json:"name"`
	AccessMethod    string     `json:"access_method"` // SSH/HTTP/HTTPS/FTP/SFTP/RDP/VNC/MySQL/PostgreSQL/Redis/API/其他
	Address         string     `json:"address"`
	Port            int        `json:"port"`
	Username        string     `json:"username"`
	Password        string     `json:"password,omitempty"`
	Status          string     `json:"status"` // running/stopped/unknown
	Description     string     `json:"description"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	TotalChecks     int        `json:"total_checks"`      // 检测次数
	SuccessChecks   int        `json:"success_checks"`    // 成功次数
	Connectivity    float64    `json:"connectivity_rate"` // 连通率 0-100
	LastResponseMs  int64      `json:"last_response_time"`// 最近检测耗时
	LastVersion     string     `json:"last_version"`      // 最近检测到的版本
	LastChecked     *time.Time `json:"last_checked"`      // 最近检测时间
}

// ServiceCheck 服务检测记录
type ServiceCheck struct {
	ID             int       `json:"id"`
	ServiceID      int       `json:"service_id"`
	Success        int       `json:"success"` // 1成功 0失败
	ResponseTimeMs int64     `json:"response_time_ms"`
	Version        string    `json:"version"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"created_at"`
}

// 服务列表通用统计字段SQL片段（通过相关子查询计算）
const serviceStatsSelect = `
	COALESCE((SELECT COUNT(*) FROM service_checks sc WHERE sc.service_id = sv.id), 0) AS total_checks,
	COALESCE((SELECT COUNT(*) FROM service_checks sc WHERE sc.service_id = sv.id AND sc.success = 1), 0) AS success_checks,
	COALESCE((SELECT sc.response_time_ms FROM service_checks sc WHERE sc.service_id = sv.id ORDER BY sc.id DESC LIMIT 1), 0) AS last_response_time,
	COALESCE((SELECT sc.version FROM service_checks sc WHERE sc.service_id = sv.id ORDER BY sc.id DESC LIMIT 1), '') AS last_version,
	(SELECT sc.created_at FROM service_checks sc WHERE sc.service_id = sv.id ORDER BY sc.id DESC LIMIT 1) AS last_checked`

// scanServiceStats 扫描统计字段并计算连通率
func scanServiceStats(s *ServerService, totalChecks, successChecks int, lastResponse int64, lastVersion string, lastChecked sql.NullTime) {
	s.TotalChecks = totalChecks
	s.SuccessChecks = successChecks
	s.LastResponseMs = lastResponse
	s.LastVersion = lastVersion
	if lastChecked.Valid {
		t := lastChecked.Time
		s.LastChecked = &t
	}
	if s.TotalChecks > 0 {
		s.Connectivity = float64(s.SuccessChecks) / float64(s.TotalChecks) * 100
	}
}

// CreateService 创建服务
func CreateService(s *ServerService) error {
	result, err := db.Exec(
		"INSERT INTO server_services (server_id, name, access_method, address, port, username, password, status, description) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		s.ServerID, s.Name, s.AccessMethod, s.Address, s.Port, s.Username, EncryptPassword(s.Password), s.Status, s.Description,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	s.ID = int(id)
	return nil
}

// GetServicesByServer 获取指定服务器的服务列表
func GetServicesByServer(serverID int) ([]ServerService, error) {
	rows, err := db.Query(
		`SELECT sv.id, sv.server_id, sv.name, sv.access_method, sv.address, sv.port, sv.username, sv.password, sv.status, sv.description, sv.created_at, sv.updated_at,
			`+serviceStatsSelect+`
		FROM server_services sv WHERE sv.server_id = ? ORDER BY sv.id`,
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []ServerService
	for rows.Next() {
		var s ServerService
		var lastChecked sql.NullTime
		var totalChecks, successChecks int
		var lastResponse int64
		var lastVersion string
		if err := rows.Scan(&s.ID, &s.ServerID, &s.Name, &s.AccessMethod, &s.Address, &s.Port, &s.Username, &s.Password, &s.Status, &s.Description, &s.CreatedAt, &s.UpdatedAt, &totalChecks, &successChecks, &lastResponse, &lastVersion, &lastChecked); err != nil {
			return nil, err
		}
		s.Password = DecryptPassword(s.Password)
		scanServiceStats(&s, totalChecks, successChecks, lastResponse, lastVersion, lastChecked)
		services = append(services, s)
	}
	return services, nil
}

// GetWebServices 获取所有WEB类服务（工具模块使用）
func GetWebServices() ([]ServerService, error) {
	rows, err := db.Query(
		`SELECT sv.id, sv.server_id, sv.name, sv.access_method, sv.address, sv.port, sv.username, sv.password, sv.status, sv.description, sv.created_at, sv.updated_at,
			s.name AS server_name, s.ip AS server_ip,
			`+serviceStatsSelect+`
		FROM server_services sv
		JOIN servers s ON sv.server_id = s.id
		WHERE sv.access_method IN ('WEB', 'HTTP', 'HTTPS')
		ORDER BY sv.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []ServerService
	for rows.Next() {
		var s ServerService
		var lastChecked sql.NullTime
		var totalChecks, successChecks int
		var lastResponse int64
		var lastVersion string
		if err := rows.Scan(&s.ID, &s.ServerID, &s.Name, &s.AccessMethod, &s.Address, &s.Port, &s.Username, &s.Password, &s.Status, &s.Description, &s.CreatedAt, &s.UpdatedAt, &s.ServerName, &s.ServerIP, &totalChecks, &successChecks, &lastResponse, &lastVersion, &lastChecked); err != nil {
			return nil, err
		}
		s.Password = DecryptPassword(s.Password)
		scanServiceStats(&s, totalChecks, successChecks, lastResponse, lastVersion, lastChecked)
		services = append(services, s)
	}
	return services, nil
}

// GetServiceByID 获取单个服务
func GetServiceByID(id int) (*ServerService, error) {
	s := &ServerService{}
	err := db.QueryRow(
		`SELECT id, server_id, name, access_method, address, port, username, password, status, description, created_at, updated_at FROM server_services WHERE id = ?`,
		id,
	).Scan(&s.ID, &s.ServerID, &s.Name, &s.AccessMethod, &s.Address, &s.Port, &s.Username, &s.Password, &s.Status, &s.Description, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	s.Password = DecryptPassword(s.Password)
	return s, nil
}

// UpdateService 更新服务
func UpdateService(s *ServerService) error {
	_, err := db.Exec(
		"UPDATE server_services SET name = ?, access_method = ?, address = ?, port = ?, username = ?, password = ?, status = ?, description = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		s.Name, s.AccessMethod, s.Address, s.Port, s.Username, EncryptPassword(s.Password), s.Status, s.Description, s.ID,
	)
	return err
}

// DeleteService 删除服务
func DeleteService(id int) error {
	_, err := db.Exec("DELETE FROM server_services WHERE id = ?", id)
	return err
}

// CreateServiceCheck 记录一次检测
func CreateServiceCheck(c *ServiceCheck) error {
	_, err := db.Exec(
		"INSERT INTO service_checks (service_id, success, response_time_ms, version, message) VALUES (?, ?, ?, ?, ?)",
		c.ServiceID, c.Success, c.ResponseTimeMs, c.Version, c.Message,
	)
	return err
}

// GetServiceChecks 获取最近检测记录
func GetServiceChecks(serviceID, limit int) ([]ServiceCheck, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := db.Query(
		"SELECT id, service_id, success, response_time_ms, version, message, created_at FROM service_checks WHERE service_id = ? ORDER BY id DESC LIMIT ?",
		serviceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []ServiceCheck
	for rows.Next() {
		var c ServiceCheck
		if err := rows.Scan(&c.ID, &c.ServiceID, &c.Success, &c.ResponseTimeMs, &c.Version, &c.Message, &c.CreatedAt); err != nil {
			return nil, err
		}
		checks = append(checks, c)
	}
	return checks, nil
}

// GetServiceConnectivity 获取服务连通率统计
func GetServiceConnectivity(serviceID int) (total, success int, rate float64, err error) {
	err = db.QueryRow(
		"SELECT COUNT(*), COALESCE(SUM(success), 0) FROM service_checks WHERE service_id = ?",
		serviceID,
	).Scan(&total, &success)
	if err != nil {
		return
	}
	if total > 0 {
		rate = float64(success) / float64(total) * 100
	}
	return
}

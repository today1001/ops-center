package models

import "time"

// PortProfile 端口访问方式配置
type PortProfile struct {
	ID          int       `json:"id"`
	Port        int       `json:"port"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

// GetPortProfiles 获取所有端口配置
func GetPortProfiles() ([]PortProfile, error) {
	rows, err := db.Query("SELECT id, port, name, description, created_at FROM port_profiles ORDER BY port")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []PortProfile
	for rows.Next() {
		var p PortProfile
		if err := rows.Scan(&p.ID, &p.Port, &p.Name, &p.Description, &p.CreatedAt); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, nil
}

// CreatePortProfile 创建端口配置
func CreatePortProfile(p *PortProfile) error {
	result, err := db.Exec(
		"INSERT INTO port_profiles (port, name, description) VALUES (?, ?, ?)",
		p.Port, p.Name, p.Description,
	)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = int(id)
	return nil
}

// UpdatePortProfile 更新端口配置
func UpdatePortProfile(p *PortProfile) error {
	_, err := db.Exec(
		"UPDATE port_profiles SET port = ?, name = ?, description = ? WHERE id = ?",
		p.Port, p.Name, p.Description, p.ID,
	)
	return err
}

// DeletePortProfile 删除端口配置
func DeletePortProfile(id int) error {
	_, err := db.Exec("DELETE FROM port_profiles WHERE id = ?", id)
	return err
}

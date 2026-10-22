package models

import "database/sql"

// HypervisorHost 虚拟化主机（VMware ESXi / Proxmox VE）
type HypervisorHost struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // esxi | pve
	IP        string `json:"ip"`
	Port      int    `json:"port"` // 0 = 按类型取默认端口（ESXi 443 / PVE 8006）
	Username  string `json:"username"`
	Password  string `json:"-"`      // 加密存储，接口不返回
	Realm     string `json:"realm"`  // PVE 认证域：pam / pve / ldap；ESXi 忽略
	Status    string `json:"status"` // online | offline | unknown
	LastCheck string `json:"last_check"`
	CreatedAt string `json:"created_at"`
}

// CreateHypervisorTables 建表
func CreateHypervisorTables() error {
	t := `
CREATE TABLE IF NOT EXISTS hypervisor_hosts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'esxi',
	ip TEXT NOT NULL,
	port INTEGER DEFAULT 0,
	username TEXT DEFAULT '',
	password TEXT DEFAULT '',
	realm TEXT DEFAULT '',
	status TEXT DEFAULT 'unknown',
	last_check TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`
	if _, err := db.Exec(t); err != nil {
		return err
	}
	return nil
}

// GetHypervisorHosts 按类型列出（type 为空则全部）
func GetHypervisorHosts(hvType string) ([]HypervisorHost, error) {
	q := `SELECT id, name, type, ip, COALESCE(port,0), COALESCE(username,''),
	              COALESCE(password,''), COALESCE(realm,''), COALESCE(status,'unknown'),
	              COALESCE(last_check,''), COALESCE(created_at,'')
	         FROM hypervisor_hosts`
	var rows *sql.Rows
	var err error
	if hvType != "" {
		q += " WHERE type = ?"
		rows, err = db.Query(q+" ORDER BY name", hvType)
	} else {
		rows, err = db.Query(q + " ORDER BY type, name")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]HypervisorHost, 0)
	for rows.Next() {
		var h HypervisorHost
		if err := rows.Scan(&h.ID, &h.Name, &h.Type, &h.IP, &h.Port, &h.Username,
			&h.Password, &h.Realm, &h.Status, &h.LastCheck, &h.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return list, rows.Err()
}

// GetHypervisorHost 按 ID 取
func GetHypervisorHost(id int) (*HypervisorHost, error) {
	var h HypervisorHost
	err := db.QueryRow(`SELECT id, name, type, ip, COALESCE(port,0), COALESCE(username,''),
		COALESCE(password,''), COALESCE(realm,''), COALESCE(status,'unknown'),
		COALESCE(last_check,''), COALESCE(created_at,'')
		FROM hypervisor_hosts WHERE id = ?`, id).Scan(
		&h.ID, &h.Name, &h.Type, &h.IP, &h.Port, &h.Username,
		&h.Password, &h.Realm, &h.Status, &h.LastCheck, &h.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// CreateHypervisorHost 新建
func CreateHypervisorHost(h *HypervisorHost) error {
	if h.Type == "" {
		h.Type = "esxi"
	}
	if h.Status == "" {
		h.Status = "unknown"
	}
	res, err := db.Exec(`INSERT INTO hypervisor_hosts
		(name, type, ip, port, username, password, realm, status, last_check)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, '')`,
		h.Name, h.Type, h.IP, h.Port, h.Username, h.Password, h.Realm, h.Status)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	h.ID = int(id)
	return nil
}

// UpdateHypervisorHost 更新；password 为空则保留原值
func UpdateHypervisorHost(h *HypervisorHost) error {
	if h.Password == "" {
		_, err := db.Exec(`UPDATE hypervisor_hosts
			SET name=?, type=?, ip=?, port=?, username=?, realm=?, status=?
			WHERE id=?`,
			h.Name, h.Type, h.IP, h.Port, h.Username, h.Realm, h.Status, h.ID)
		return err
	}
	_, err := db.Exec(`UPDATE hypervisor_hosts
		SET name=?, type=?, ip=?, port=?, username=?, password=?, realm=?, status=?
		WHERE id=?`,
		h.Name, h.Type, h.IP, h.Port, h.Username, h.Password, h.Realm, h.Status, h.ID)
	return err
}

// UpdateHypervisorStatus 仅回写探测结果
func UpdateHypervisorStatus(id int, status, checkedAt string) error {
	_, err := db.Exec(`UPDATE hypervisor_hosts SET status=?, last_check=? WHERE id=?`,
		status, checkedAt, id)
	return err
}

// DeleteHypervisorHost 删除
func DeleteHypervisorHost(id int) error {
	_, err := db.Exec(`DELETE FROM hypervisor_hosts WHERE id = ?`, id)
	return err
}

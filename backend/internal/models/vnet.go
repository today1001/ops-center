package models

import (
	"database/sql"
)

// VnetNetwork 虚拟网络源
type VnetNetwork struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`      // easytier-local / easytier-rpc
	RPCAddr   string `json:"rpc_addr"`  // RPC 地址，如 127.0.0.1:15888
	Subnet    string `json:"subnet"`    // 虚拟网段，如 10.10.10.0/24
	Enabled   int    `json:"enabled"`
	Managed   int    `json:"managed"` // 1=本平台创建的实例(可停止/删除)
	CreatedAt string `json:"created_at"`
}

// ServerVirtual 服务器虚拟网络信息（与 servers 一对一）
type ServerVirtual struct {
	ServerID   int    `json:"server_id"`
	Network    string `json:"virtual_network"`
	Identifier string `json:"virtual_identifier"`
	IP         string `json:"virtual_ip"`
	Online     int    `json:"virtual_online"`
}

// CreateVnetTables 建表 + 预置默认网络源
func CreateVnetTables() error {
	t1 := `
	CREATE TABLE IF NOT EXISTS vnet_networks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT UNIQUE NOT NULL,
		type TEXT DEFAULT 'easytier-local',
		rpc_addr TEXT DEFAULT '127.0.0.1:15888',
		subnet TEXT DEFAULT '',
		enabled INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(t1); err != nil {
		return err
	}
	t2 := `
	CREATE TABLE IF NOT EXISTS server_virtual (
		server_id INTEGER PRIMARY KEY,
		network TEXT DEFAULT '',
		identifier TEXT DEFAULT '',
		ip TEXT DEFAULT '',
		online INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(t2); err != nil {
		return err
	}
	// managed 列（已存在时忽略报错）
	_, _ = db.Exec("ALTER TABLE vnet_networks ADD COLUMN managed INTEGER DEFAULT 0")
	// 预置默认源
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM vnet_networks").Scan(&count); err == nil && count == 0 {
		db.Exec("INSERT INTO vnet_networks (name, type, rpc_addr, subnet, enabled) VALUES ('LtNet', 'easytier-local', '127.0.0.1:15888', '10.10.10.0/24', 1)")
	}
	return nil
}

// GetVnetNetworks 网络源列表
func GetVnetNetworks() ([]VnetNetwork, error) {
	rows, err := db.Query("SELECT id, name, type, rpc_addr, subnet, enabled, managed, created_at FROM vnet_networks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []VnetNetwork
	for rows.Next() {
		var n VnetNetwork
		if err := rows.Scan(&n.ID, &n.Name, &n.Type, &n.RPCAddr, &n.Subnet, &n.Enabled, &n.Managed, &n.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, nil
}

// CreateVnetNetwork 新增网络源
func CreateVnetNetwork(n *VnetNetwork) error {
	result, err := db.Exec("INSERT INTO vnet_networks (name, type, rpc_addr, subnet, enabled, managed) VALUES (?, ?, ?, ?, ?, ?)",
		n.Name, n.Type, n.RPCAddr, n.Subnet, n.Enabled, n.Managed)
	if err != nil {
		return err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	n.ID = int(id)
	return nil
}

// UpdateVnetNetwork 更新网络源
func UpdateVnetNetwork(n *VnetNetwork) error {
	_, err := db.Exec("UPDATE vnet_networks SET name = ?, type = ?, rpc_addr = ?, subnet = ?, enabled = ? WHERE id = ?",
		n.Name, n.Type, n.RPCAddr, n.Subnet, n.Enabled, n.ID)
	return err
}

// DeleteVnetNetwork 删除网络源
func DeleteVnetNetwork(id int) error {
	_, err := db.Exec("DELETE FROM vnet_networks WHERE id = ?", id)
	return err
}

// GetServerVirtual 单台服务器的虚拟信息
func GetServerVirtual(serverID int) (*ServerVirtual, error) {
	sv := &ServerVirtual{ServerID: serverID}
	err := db.QueryRow("SELECT network, identifier, ip, online FROM server_virtual WHERE server_id = ?", serverID).
		Scan(&sv.Network, &sv.Identifier, &sv.IP, &sv.Online)
	if err == sql.ErrNoRows {
		return sv, nil
	}
	if err != nil {
		return nil, err
	}
	return sv, nil
}

// GetAllServerVirtual 全部服务器的虚拟信息（按 server_id 索引）
func GetAllServerVirtual() (map[int]ServerVirtual, error) {
	rows, err := db.Query("SELECT server_id, network, identifier, ip, online FROM server_virtual")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := make(map[int]ServerVirtual)
	for rows.Next() {
		var sv ServerVirtual
		if err := rows.Scan(&sv.ServerID, &sv.Network, &sv.Identifier, &sv.IP, &sv.Online); err != nil {
			return nil, err
		}
		m[sv.ServerID] = sv
	}
	return m, nil
}

// UpsertServerVirtual 写入/更新虚拟信息（ip 为空时保留原 IP）
func UpsertServerVirtual(sv *ServerVirtual) error {
	var oldIP string
	db.QueryRow("SELECT ip FROM server_virtual WHERE server_id = ?", sv.ServerID).Scan(&oldIP)
	if sv.IP == "" {
		sv.IP = oldIP
	}
	_, err := db.Exec(`INSERT INTO server_virtual (server_id, network, identifier, ip, online, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(server_id) DO UPDATE SET network = ?, identifier = ?, ip = ?, online = ?, updated_at = CURRENT_TIMESTAMP`,
		sv.ServerID, sv.Network, sv.Identifier, sv.IP, sv.Online,
		sv.Network, sv.Identifier, sv.IP, sv.Online)
	return err
}

// DeleteServerVirtual 删除虚拟信息
func DeleteServerVirtual(serverID int) error {
	_, err := db.Exec("DELETE FROM server_virtual WHERE server_id = ?", serverID)
	return err
}

// ServerBasic 服务器基本信息（同步匹配用）
type ServerBasic struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// GetAllServersBasic 全部服务器 id+name
func GetAllServersBasic() ([]ServerBasic, error) {
	rows, err := db.Query("SELECT id, name FROM servers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ServerBasic
	for rows.Next() {
		var b ServerBasic
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
			return nil, err
		}
		list = append(list, b)
	}
	return list, nil
}

// FollowServiceAddress 虚拟 IP 变化时联动更新服务地址
func FollowServiceAddress(serverID int, oldIP, newIP string) (int64, error) {
	if oldIP == "" || oldIP == newIP {
		return 0, nil
	}
	result, err := db.Exec("UPDATE server_services SET address = ? WHERE server_id = ? AND address = ?", newIP, serverID, oldIP)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return n, nil
}

// CheckDB 数据库健康检查
func CheckDB() error {
	var one int
	return db.QueryRow("SELECT 1").Scan(&one)
}

package services

import (
	"database/sql"
	"fmt"
	"strings"
)

// 定时扫描的持久化：配置（app_settings）+ 扫描历史（free_ip_scans）+ 明细（free_ip_records）

// 默认扫描间隔（分钟）
const freeIPDefaultInterval = 30

// 扫描间隔上下限（分钟）：最短 5 分钟避免发包过密，最长 1 天
const (
	freeIPMinInterval = 5
	freeIPMaxInterval = 1440
)

// 保留最近多少次扫描记录，超过的连同明细一起清理
const freeIPKeepScans = 20

// EnsureFreeIPTables 建定时扫描相关表（幂等）
func EnsureFreeIPTables(db *sql.DB) error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS free_ip_scans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subnets TEXT,
			candidates INTEGER DEFAULT 0,
			skipped INTEGER DEFAULT 0,
			free_count INTEGER DEFAULT 0,
			known_count INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			trigger_type TEXT DEFAULT 'auto',
			scanned_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS free_ip_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			scan_id INTEGER NOT NULL,
			ip TEXT NOT NULL,
			subnet TEXT,
			mac TEXT,
			scanned_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_free_ip_records_scan ON free_ip_records(scan_id);`,
		`CREATE INDEX IF NOT EXISTS idx_free_ip_records_ip ON free_ip_records(ip);`,
		`CREATE INDEX IF NOT EXISTS idx_free_ip_scans_time ON free_ip_scans(scanned_at DESC);`,
	}
	for _, t := range tables {
		if _, err := db.Exec(t); err != nil {
			return err
		}
	}
	return nil
}

// GetScanInterval 读取扫描间隔（分钟），无配置或非法值回退默认
func GetScanInterval(db *sql.DB) int {
	var v string
	if err := db.QueryRow("SELECT value FROM app_settings WHERE key = 'freeip_scan_interval'").Scan(&v); err != nil {
		return freeIPDefaultInterval
	}
	var n int
	if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &n); err != nil {
		return freeIPDefaultInterval
	}
	if n < freeIPMinInterval || n > freeIPMaxInterval {
		return freeIPDefaultInterval
	}
	return n
}

// SetScanInterval 保存扫描间隔（分钟）
func SetScanInterval(db *sql.DB, minutes int) error {
	if minutes < freeIPMinInterval || minutes > freeIPMaxInterval {
		return fmt.Errorf("间隔需在 %d ~ %d 分钟之间", freeIPMinInterval, freeIPMaxInterval)
	}
	_, err := db.Exec(`
		INSERT INTO app_settings (key, value) VALUES ('freeip_scan_interval', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprintf("%d", minutes))
	return err
}

// SaveScanToDB 把一次扫描结果落库，并清理超出保留数量的历史
func SaveScanToDB(db *sql.DB, scan *FreeIPScan, trigger string) (int64, error) {
	if scan == nil {
		return 0, fmt.Errorf("扫描结果为空")
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	r, err := tx.Exec(`INSERT INTO free_ip_scans
		(subnets, candidates, skipped, free_count, known_count, duration_ms, trigger_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		strings.Join(scan.Subnets, ","),
		scan.Candidates, scan.Skipped, len(scan.Free), len(scan.Known),
		scan.DurationMs, trigger)
	if err != nil {
		return 0, err
	}
	scanID, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}

	stmt, err := tx.Prepare(`INSERT INTO free_ip_records (scan_id, ip, subnet, mac) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, f := range scan.Free {
		if _, err := stmt.Exec(scanID, f.IP, f.Subnet, f.MAC); err != nil {
			return 0, err
		}
	}

	// 清理旧扫描：只保留最近 freeIPKeepScans 次
	if _, err := tx.Exec(`DELETE FROM free_ip_records WHERE scan_id IN (
		SELECT id FROM free_ip_scans ORDER BY id DESC LIMIT -1 OFFSET ?)`, freeIPKeepScans); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM free_ip_scans WHERE id NOT IN (
		SELECT id FROM free_ip_scans ORDER BY id DESC LIMIT ?)`, freeIPKeepScans); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return scanID, nil
}

// ScanHistoryItem 扫描历史汇总行
type ScanHistoryItem struct {
	ID         int64  `json:"id"`
	Subnets    string `json:"subnets"`
	Candidates int    `json:"candidates"`
	Skipped    int    `json:"skipped"`
	FreeCount  int    `json:"free_count"`
	KnownCount int    `json:"known_count"`
	DurationMs int64  `json:"duration_ms"`
	Trigger    string `json:"trigger"`
	ScannedAt  string `json:"scanned_at"`
}

// GetScanHistory 最近 N 次扫描历史（新→旧）
func GetScanHistory(db *sql.DB, limit int) ([]ScanHistoryItem, error) {
	if limit <= 0 || limit > freeIPKeepScans {
		limit = 10
	}
	rows, err := db.Query(`SELECT id, COALESCE(subnets,''), candidates, skipped, free_count, known_count,
		duration_ms, COALESCE(trigger_type,'auto'), scanned_at
		FROM free_ip_scans ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScanHistoryItem
	for rows.Next() {
		var it ScanHistoryItem
		if err := rows.Scan(&it.ID, &it.Subnets, &it.Candidates, &it.Skipped, &it.FreeCount,
			&it.KnownCount, &it.DurationMs, &it.Trigger, &it.ScannedAt); err != nil {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

// FreeIPRecord 落库的一条空闲 IP 明细
type FreeIPRecord struct {
	ID        int64  `json:"id"`
	ScanID    int64  `json:"scan_id"`
	IP        string `json:"ip"`
	Subnet    string `json:"subnet"`
	MAC       string `json:"mac"`
	ScannedAt string `json:"scanned_at"`
}

// GetScanRecords 取某次扫描的空闲 IP 明细；scanID=0 取最新一次
func GetScanRecords(db *sql.DB, scanID int64) ([]FreeIPRecord, error) {
	if scanID <= 0 {
		err := db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM free_ip_scans").Scan(&scanID)
		if err != nil || scanID == 0 {
			return []FreeIPRecord{}, nil
		}
	}
	rows, err := db.Query(`SELECT id, scan_id, ip, COALESCE(subnet,''), COALESCE(mac,''), scanned_at
		FROM free_ip_records WHERE scan_id = ? ORDER BY ip`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FreeIPRecord
	for rows.Next() {
		var r FreeIPRecord
		if err := rows.Scan(&r.ID, &r.ScanID, &r.IP, &r.Subnet, &r.MAC, &r.ScannedAt); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// GetLastScanTime 最近一次扫描时间（含手动与定时）
func GetLastScanTime(db *sql.DB) string {
	var t string
	if err := db.QueryRow("SELECT scanned_at FROM free_ip_scans ORDER BY id DESC LIMIT 1").Scan(&t); err != nil {
		return ""
	}
	return t
}

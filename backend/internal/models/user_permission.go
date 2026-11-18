package models

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// 角色常量：分级权限体系
// admin       超级管理员：全部资源 + 用户管理
// group_admin 分组管理员：授权分组内全部服务器可管理
// server_admin 服务器管理员：授权服务器可管理
// viewer      只读观察者：授权服务器仅可查看
const (
	RoleAdmin       = "admin"
	RoleGroupAdmin  = "group_admin"
	RoleServerAdmin = "server_admin"
	RoleViewer      = "viewer"
)

// ValidRoles 合法角色集合
var ValidRoles = map[string]bool{
	RoleAdmin: true, RoleGroupAdmin: true, RoleServerAdmin: true, RoleViewer: true,
}

// AuditLog 审计/访问日志
type AuditLog struct {
	ID         int64  `json:"id"`
	UserID     int    `json:"user_id"`
	Username   string `json:"username"`
	Action     string `json:"action"` // login / login_failed / write / access / grant / revoke
	Method     string `json:"method"`
	Path       string `json:"path"`
	IP         string `json:"ip"`
	Status     int    `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	Detail     string `json:"detail"`
	CreatedAt  string `json:"created_at"`
}

// auditCh 异步写库队列：避免多 goroutine 并发写 SQLite 触发锁冲突
var auditCh chan AuditLog

func initAuditWorker() {
	auditCh = make(chan AuditLog, 1024)
	go func() {
		for a := range auditCh {
			insertAuditLog(&a)
		}
	}()
}

// LogAudit 异步写一条审计日志（队列满则丢弃，绝不阻塞主流程）
func LogAudit(a AuditLog) {
	if auditCh == nil {
		return
	}
	select {
	case auditCh <- a:
	default:
	}
}

func insertAuditLog(a *AuditLog) {
	_, err := db.Exec(`INSERT INTO audit_logs
		(user_id, username, action, method, path, ip, status, duration_ms, detail)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, a.Username, a.Action, a.Method, a.Path, a.IP, a.Status, a.DurationMs, a.Detail)
	if err != nil {
		println("audit log write failed:", err.Error())
	}
}

// GetAuditLogs 分页查询审计日志，支持 action/username 过滤
func GetAuditLogs(limit, offset int, action, username string) ([]AuditLog, int, error) {
	where := " WHERE 1=1"
	var args []interface{}
	if action != "" {
		where += " AND action = ?"
		args = append(args, action)
	}
	if username != "" {
		where += " AND username = ?"
		args = append(args, username)
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM audit_logs"+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT id, COALESCE(user_id,0), COALESCE(username,''), action, method, path, ip, status, duration_ms, COALESCE(detail,''), created_at
		FROM audit_logs` + where + " ORDER BY id DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []AuditLog
	for rows.Next() {
		var a AuditLog
		if err := rows.Scan(&a.ID, &a.UserID, &a.Username, &a.Action, &a.Method, &a.Path,
			&a.IP, &a.Status, &a.DurationMs, &a.Detail, &a.CreatedAt); err != nil {
			continue
		}
		logs = append(logs, a)
	}
	return logs, total, nil
}

// ============ 授权关系 CRUD ============

// GrantUserGroups 批量把用户加入授权分组（幂等）
func GrantUserGroups(userID int, groupIDs []int) (added int, err error) {
	tx, e := db.Begin()
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	for _, gid := range groupIDs {
		r, e := tx.Exec("INSERT OR IGNORE INTO user_groups (user_id, group_id) VALUES (?, ?)", userID, gid)
		if e != nil {
			return added, e
		}
		if n, _ := r.RowsAffected(); n > 0 {
			added++
		}
	}
	return added, tx.Commit()
}

// RevokeUserGroup 移除用户的分组授权
func RevokeUserGroup(userID, groupID int) error {
	_, err := db.Exec("DELETE FROM user_groups WHERE user_id = ? AND group_id = ?", userID, groupID)
	return err
}

// GetUserGrantedGroups 获取用户被授权的分组列表
func GetUserGrantedGroups(userID int) ([]ServerGroup, error) {
	rows, err := db.Query(`
		SELECT g.id, g.name, COALESCE(g.description, ''),
		       (SELECT COUNT(*) FROM servers s WHERE s.group_name = g.name) AS server_count,
		       g.created_at, g.updated_at
		FROM user_groups ug JOIN server_groups g ON g.id = ug.group_id
		WHERE ug.user_id = ? ORDER BY g.name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []ServerGroup
	for rows.Next() {
		var g ServerGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.Description, &g.ServerCount, &g.CreatedAt, &g.UpdatedAt); err != nil {
			continue
		}
		groups = append(groups, g)
	}
	return groups, nil
}

// GrantUserServers 批量授权用户访问服务器（幂等）
func GrantUserServers(userID int, serverIDs []int) (added int, err error) {
	tx, e := db.Begin()
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	for _, sid := range serverIDs {
		r, e := tx.Exec("INSERT OR IGNORE INTO user_servers (user_id, server_id) VALUES (?, ?)", userID, sid)
		if e != nil {
			return added, e
		}
		if n, _ := r.RowsAffected(); n > 0 {
			added++
		}
	}
	return added, tx.Commit()
}

// RevokeUserServer 取消用户对某服务器的授权
func RevokeUserServer(userID, serverID int) error {
	_, err := db.Exec("DELETE FROM user_servers WHERE user_id = ? AND server_id = ?", userID, serverID)
	return err
}

// GetUserGrantedServers 获取用户被授权的服务器列表（不含凭据）
func GetUserGrantedServers(userID int) ([]Server, error) {
	rows, err := db.Query(`
		SELECT DISTINCT s.id, s.name, s.ip, s.port, s.username, s.os, s.status, s.auth_type, s.tags,
		       COALESCE(s.group_name, ''), s.created_by, s.created_at, s.updated_at
		FROM user_servers us JOIN servers s ON s.id = us.server_id
		WHERE us.user_id = ? ORDER BY s.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.OS, &s.Status,
			&s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		servers = append(servers, s)
	}
	return servers, nil
}

// ============ 权限判定 ============

// serverInScope 判断服务器是否在用户的授权范围内
// （自己创建的 ∪ 直接授权的 ∪ 授权分组下的）
func serverInScope(userID, serverID int) bool {
	var cnt int
	err := db.QueryRow(`
		SELECT COUNT(*) FROM servers s WHERE s.id = ? AND (
			s.created_by = ?
			OR s.id IN (SELECT server_id FROM user_servers WHERE user_id = ?)
			OR s.group_name IN (
				SELECT g.name FROM user_groups ug
				JOIN server_groups g ON g.id = ug.group_id
				WHERE ug.user_id = ?
			)
		)`, serverID, userID, userID, userID).Scan(&cnt)
	return err == nil && cnt > 0
}

// CanAccessServer 判断用户可否访问某服务器
// needWrite=true 时 viewer 一律拒绝，其余角色需在授权范围内
func CanAccessServer(role string, userID, serverID int, needWrite bool) bool {
	if role == RoleAdmin {
		return true
	}
	if needWrite && role == RoleViewer {
		return false
	}
	return serverInScope(userID, serverID)
}

// GetServersForUser 按角色过滤服务器列表（不含凭据）
func GetServersForUser(role string, userID int) ([]Server, error) {
	if role == RoleAdmin {
		return GetServersAll()
	}
	rows, err := db.Query(`
		SELECT DISTINCT s.id, s.name, s.ip, s.port, s.username, s.os, s.status, s.auth_type, s.tags,
		       COALESCE(s.group_name, ''), s.created_by, s.created_at, s.updated_at
		FROM servers s
		WHERE s.created_by = ?
		   OR s.id IN (SELECT server_id FROM user_servers WHERE user_id = ?)
		   OR s.group_name IN (
				SELECT g.name FROM user_groups ug
				JOIN server_groups g ON g.id = ug.group_id
				WHERE ug.user_id = ?
		   )
		ORDER BY s.id`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.OS, &s.Status,
			&s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		servers = append(servers, s)
	}
	return servers, nil
}

// GetServersWithCredsForUser 按角色过滤服务器列表（含解密凭据，供采集用）
func GetServersWithCredsForUser(role string, userID int) ([]Server, error) {
	if role == RoleAdmin {
		return GetServersWithCredsAll()
	}
	rows, err := db.Query(`
		SELECT DISTINCT s.id, s.name, s.ip, s.port, s.username, s.password, s.private_key, s.os, s.status,
		       s.auth_type, s.tags, COALESCE(s.group_name, ''), s.created_by, s.created_at, s.updated_at
		FROM servers s
		WHERE s.created_by = ?
		   OR s.id IN (SELECT server_id FROM user_servers WHERE user_id = ?)
		   OR s.group_name IN (
				SELECT g.name FROM user_groups ug
				JOIN server_groups g ON g.id = ug.group_id
				WHERE ug.user_id = ?
		   )
		ORDER BY s.id`, userID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.Password, &s.PrivateKey,
			&s.OS, &s.Status, &s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			continue
		}
		s.Password = DecryptPassword(s.Password)
		servers = append(servers, s)
	}
	return servers, nil
}

// GetServersAll 全量服务器列表（admin 用）
func GetServersAll() ([]Server, error) {
	rows, err := db.Query(`SELECT id, name, ip, port, username, os, status, auth_type, tags, COALESCE(group_name, ''), created_by, created_at, updated_at FROM servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.OS, &s.Status, &s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}

// GetServersWithCredsAll 全量服务器列表含凭据（admin 采集用）
func GetServersWithCredsAll() ([]Server, error) {
	rows, err := db.Query(`SELECT id, name, ip, port, username, password, private_key, os, status, auth_type, tags, COALESCE(group_name, ''), created_by, created_at, updated_at FROM servers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var servers []Server
	for rows.Next() {
		var s Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IP, &s.Port, &s.Username, &s.Password, &s.PrivateKey, &s.OS, &s.Status, &s.AuthType, &s.Tags, &s.GroupName, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		s.Password = DecryptPassword(s.Password)
		servers = append(servers, s)
	}
	return servers, nil
}

// ============ 用户管理（admin API 用） ============

// AdminUserInfo 管理界面的用户信息（含授权摘要）
type AdminUserInfo struct {
	ID          int       `json:"id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	ServerCount int       `json:"server_count"`
	GroupCount  int       `json:"group_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListAdminUsers 全部用户及授权摘要
func ListAdminUsers() ([]AdminUserInfo, error) {
	rows, err := db.Query(`
		SELECT u.id, u.username, COALESCE(u.email,''), u.role, u.created_at, u.updated_at,
		       (SELECT COUNT(*) FROM user_servers us WHERE us.user_id = u.id),
		       (SELECT COUNT(*) FROM user_groups ug WHERE ug.user_id = u.id)
		FROM users u ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []AdminUserInfo
	for rows.Next() {
		var u AdminUserInfo
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.CreatedAt, &u.UpdatedAt, &u.ServerCount, &u.GroupCount); err != nil {
			continue
		}
		users = append(users, u)
	}
	return users, nil
}

// CreateAdminUser 创建用户（密码 bcrypt 加密）
func CreateAdminUser(username, password, email, role string) error {
	if !ValidRoles[role] {
		return errors.New("非法角色: " + role)
	}
	username = strings.TrimSpace(username)
	if username == "" || len(password) < 6 {
		return errors.New("用户名不能为空且密码至少 6 位")
	}
	var cnt int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&cnt); err != nil {
		return err
	}
	if cnt > 0 {
		return errors.New("用户名已存在")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if email == "" {
		// 老 users 表的 email 列无默认值，省略列会写入 NULL，
		// 导致 GetUserByUsername 扫描 NULL→string 失败（表现为"用户不存在"）
		email = ""
	}
	_, err = db.Exec("INSERT INTO users (username, password, email, role) VALUES (?, ?, ?, ?)", username, string(hashed), email, role)
	return err
}

// UpdateAdminUser 修改角色/密码/邮箱（传 nil 表示不修改该项）
func UpdateAdminUser(id int, role, password, email *string) error {
	// 不能降级自己（保证系统至少有一个可用管理员由 handler 层配合校验）
	if role != nil {
		if !ValidRoles[*role] {
			return errors.New("非法角色: " + *role)
		}
		if _, err := GetUserByID(id); err != nil {
			return errors.New("用户不存在")
		}
	}
	if password != nil && len(*password) < 6 {
		return errors.New("密码至少 6 位")
	}

	if role != nil {
		if _, err := db.Exec("UPDATE users SET role = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *role, id); err != nil {
			return err
		}
	}
	if password != nil {
		hashed, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if _, err := db.Exec("UPDATE users SET password = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", string(hashed), id); err != nil {
			return err
		}
	}
	if email != nil {
		if _, err := db.Exec("UPDATE users SET email = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *email, id); err != nil {
			return err
		}
	}
	return nil
}

// CountAdminUsers 统计 admin 角色用户数
func CountAdminUsers() (int, error) {
	var cnt int
	err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = ?", RoleAdmin).Scan(&cnt)
	return cnt, err
}

// GetUserRoleByID 查询指定用户的角色
func GetUserRoleByID(id int) (string, error) {
	var role string
	err := db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&role)
	return role, err
}

// DeleteAdminUser 删除用户（级联清理授权），禁止删除自己与最后一个 admin
func DeleteAdminUser(id int, operatorID int) error {
	if id == operatorID {
		return errors.New("不能删除当前登录的账号")
	}
	var role string
	if err := db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&role); err != nil {
		if err == sql.ErrNoRows {
			return errors.New("用户不存在")
		}
		return err
	}
	if role == RoleAdmin {
		var adminCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE role = ?", RoleAdmin).Scan(&adminCount); err != nil {
			return err
		}
		if adminCount <= 1 {
			return errors.New("系统至少需要保留一个管理员")
		}
	}
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, q := range []string{
		"DELETE FROM user_servers WHERE user_id = ?",
		"DELETE FROM user_groups WHERE user_id = ?",
		"DELETE FROM users WHERE id = ?",
	} {
		if _, e := tx.Exec(q, id); e != nil {
			return e
		}
	}
	return tx.Commit()
}

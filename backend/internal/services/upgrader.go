package services

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ============ 版本与配置 ============

// BackendVersion 后端构建版本（发布时 -ldflags 注入）
var BackendVersion = "dev"

// 升级源三个模块对应 update-server 的工作区（可配置，app_settings: upgrade_ws_<module>）
// 默认适配实际部署：backend 独占 oc-backend；frontend/mcp 共用 ops-center
// （latest 接口按工作区取最新文件，多模块共用时改用 history+文件名前缀过滤）
var upgradeModules = []string{"backend", "frontend", "mcp"}

var defaultWorkspace = map[string]string{
	"backend":  "oc-backend",
	"frontend": "ops-center",
	"mcp":      "ops-center",
}

// modulePrefix 各模块发布文件名前缀（history 过滤用）
var modulePrefix = map[string]string{
	"backend":  "ops-center-",
	"frontend": "frontend-",
	"mcp":      "mcp-server-",
}

// GetUpgradeWorkspace 模块对应的工作区 code
func GetUpgradeWorkspace(db *sql.DB, module string) string {
	ws := settingGet(db, "upgrade_ws_" + module)
	if ws == "" {
		ws = defaultWorkspace[module]
	}
	return ws
}

// SetWorkspace 保存模块工作区映射
func SetWorkspace(db *sql.DB, module, ws string) {
	settingSet(db, "upgrade_ws_"+module, ws)
}

// 安装路径（相对 backend 工作目录）
const (
	upgradeStagingDir = "../upgrade/staging"
	upgradeBackupDir  = "../upgrade/backup"
	upgradeRepoDir    = "../upgrade/repo" // local 源模式的包目录
	backendBinary     = "ops-center"
	mcpBinary         = "mcp-server"
	frontendDistDir   = "../frontend-dist"
)

// UpgradeStatus 一次检查/升级的模块状态
type UpgradeStatus struct {
	Module    string `json:"module"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Size      int64  `json:"size"`
	Changelog string `json:"changelog"`
	Updated   string `json:"updated"`
	HasUpdate bool   `json:"has_update"`
	Policy    string `json:"policy"` // auto / manual / off
	Err       string `json:"error,omitempty"`
}

// ============ 配置读写（app_settings） ============

func settingGet(db *sql.DB, key string) string {
	var v string
	_ = db.QueryRow("SELECT value FROM app_settings WHERE key = ?", key).Scan(&v)
	return v
}

func settingSet(db *sql.DB, key, value string) {
	db.Exec("INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
}

// GetUpgradeSource 升级源：update-server 基地址（http://10.172.66.61:15800）或 local:
func GetUpgradeSource(db *sql.DB) string {
	s := settingGet(db, "upgrade_source")
	if s == "" {
		s = "http://10.172.66.61:15800"
	}
	return s
}

func GetUpgradeInterval(db *sql.DB) int {
	n := 0
	fmt.Sscanf(settingGet(db, "upgrade_interval_minutes"), "%d", &n)
	if n < 10 || n > 10080 {
		n = 60 // 默认每小时检查一次
	}
	return n
}

// GetUpgradePolicy 模块策略：auto（自动升级）/ manual（提示+手动）/ off（忽略）
func GetUpgradePolicy(db *sql.DB, module string) string {
	p := settingGet(db, "upgrade_policy_" + module)
	if p == "" {
		p = "manual" // 保守默认
	}
	return p
}

// SetSource 保存升级源
func SetSource(db *sql.DB, source string) {
	if source == "" {
		settingSet(db, "upgrade_source", "http://10.172.66.61:15800")
		return
	}
	settingSet(db, "upgrade_source", source)
}

// SetInterval 保存检查间隔（分钟）
func SetInterval(db *sql.DB, minutes int) {
	settingSet(db, "upgrade_interval_minutes", fmt.Sprintf("%d", minutes))
}

// SetPolicy 保存模块升级策略
func SetPolicy(db *sql.DB, module, policy string) {
	settingSet(db, "upgrade_policy_"+module, policy)
}

// GetInstalledVersion 模块当前安装版本
func GetInstalledVersion(db *sql.DB, module string) string {
	if module == "backend" {
		if BackendVersion != "" && BackendVersion != "dev" {
			return BackendVersion
		}
		// ldflags 没注入时以 app_settings 记录为准
		v := settingGet(db, "installed_version_backend")
		if v == "" {
			return "dev"
		}
		return v
	}
	v := settingGet(db, "installed_version_" + module)
	if v == "" {
		return "unknown"
	}
	return v
}

// ============ 升级日志 ============

// UpgradeLog 升级历史
type UpgradeLog struct {
	ID      int64  `json:"id"`
	Module  string `json:"module"`
	From    string `json:"from_ver"`
	To      string `json:"to_ver"`
	Status  string `json:"status"` // checking / success / failed / rolled_back
	Message string `json:"message"`
	Mode    string `json:"mode"` // auto / manual
	Created string `json:"created_at"`
}

func logUpgrade(db *sql.DB, module, from, to, status, message, mode string) {
	db.Exec("INSERT INTO upgrade_logs (module, from_ver, to_ver, status, message, mode) VALUES (?, ?, ?, ?, ?, ?)",
		module, from, to, status, message, mode)
}

// GetUpgradeLogs 最近升级记录
func GetUpgradeLogs(db *sql.DB, limit int) ([]UpgradeLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := db.Query(`SELECT id, module, COALESCE(from_ver,''), COALESCE(to_ver,''), status,
		COALESCE(message,''), COALESCE(mode,''), created_at FROM upgrade_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UpgradeLog
	for rows.Next() {
		var l UpgradeLog
		if err := rows.Scan(&l.ID, &l.Module, &l.From, &l.To, &l.Status, &l.Message, &l.Mode, &l.Created); err != nil {
			continue
		}
		out = append(out, l)
	}
	return out, nil
}

// EnsureUpgradeTables 升级相关表
func EnsureUpgradeTables(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS upgrade_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		module TEXT,
		from_ver TEXT,
		to_ver TEXT,
		status TEXT,
		message TEXT,
		mode TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

// ============ 版本清单获取 ============

// releaseMeta 统一的版本元数据（update-server latest 或 local latest.json）
type releaseMeta struct {
	Version   string `json:"version"`
	Filename  string `json:"filename"`
	URL       string `json:"url"`   // http 模式的下载路径（相对源基地址）
	LocalFile string `json:"local"` // local 模式的文件绝对路径
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Updated   string `json:"updated"`
	Changelog string `json:"changelog"`
}

// fetchLatest 获取模块最新版本元数据
func fetchLatest(db *sql.DB, module string) (*releaseMeta, error) {
	source := GetUpgradeSource(db)
	ws := GetUpgradeWorkspace(db, module)

	if len(source) > 6 && source[:6] == "local:" {
		// local 源：<dir>/<module>/latest.json + 文件
		dir := filepath.Join(source[6:], ws)
		data, err := os.ReadFile(filepath.Join(dir, "latest.json"))
		if err != nil {
			return nil, fmt.Errorf("本地源无版本清单: %w", err)
		}
		var m releaseMeta
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		m.LocalFile = filepath.Join(dir, m.Filename)
		return &m, nil
	}

	// update-server 模式：走公开 history 接口 + 模块文件名前缀过滤。
	// （latest 接口按工作区取最新文件，多模块共用一个工作区会互相覆盖）
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(fmt.Sprintf("%s/%s/app/history", trimSlash(source), ws))
	if err != nil {
		return nil, fmt.Errorf("连接升级源失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("升级源返回 %d（工作区 %s 可能未发布）", resp.StatusCode, ws)
	}
	var hr struct {
		History []struct {
			Filename  string `json:"filename"`
			Version   string `json:"version"`
			Download  string `json:"download"`
			SHA256    string `json:"sha256"`
			Size      int64  `json:"size"`
			Updated   string `json:"updated"`
			Changelog string `json:"changelog"`
		} `json:"history"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&hr); err != nil {
		return nil, err
	}
	// history 倒序，第一条匹配模块前缀的就是最新版
	for _, it := range hr.History {
		if !strings.HasPrefix(it.Filename, modulePrefix[module]) {
			continue
		}
		return &releaseMeta{
			Version:   it.Version,
			Filename:  it.Filename,
			URL:       it.Download,
			SHA256:    it.SHA256,
			Size:      it.Size,
			Updated:   it.Updated,
			Changelog: it.Changelog,
		}, nil
	}
	return nil, fmt.Errorf("工作区 %s 中没有 %s 前缀的发布文件", ws, modulePrefix[module])
}

// checkModule 检查单模块更新
func checkModule(db *sql.DB, module string) (*UpgradeStatus, error) {
	cur := GetInstalledVersion(db, module)
	m, err := fetchLatest(db, module)
	if err != nil {
		return &UpgradeStatus{Module: module, Current: cur, Policy: GetUpgradePolicy(db, module), Err: err.Error()}, err
	}
	st := &UpgradeStatus{
		Module:    module,
		Current:   cur,
		Latest:    m.Version,
		Size:      m.Size,
		Changelog: m.Changelog,
		Updated:   m.Updated,
		Policy:    GetUpgradePolicy(db, module),
		HasUpdate: m.Version != cur && m.Version != "",
	}
	return st, nil
}

// CheckAllUpdates 检查全部模块
func CheckAllUpdates(db *sql.DB) ([]UpgradeStatus, error) {
	var out []UpgradeStatus
	for _, m := range upgradeModules {
		st, _ := checkModule(db, m)
		out = append(out, *st)
	}
	return out, nil
}

// ============ 下载与校验 ============

// downloadPackage 下载/复制升级包到暂存目录，做 sha256+大小+格式校验
func downloadPackage(db *sql.DB, module string, m *releaseMeta) (string, error) {
	os.MkdirAll(upgradeStagingDir, 0755)
	dst := filepath.Join(upgradeStagingDir, fmt.Sprintf("%s-%s", module, m.Version))

	var src io.ReadCloser
	if m.LocalFile != "" {
		f, err := os.Open(m.LocalFile)
		if err != nil {
			return "", fmt.Errorf("本地包打开失败: %w", err)
		}
		src = f
	} else {
		url := m.URL
		if len(url) > 0 && url[0] == '/' {
			url = trimSlash(GetUpgradeSource(db)) + url
		}
		client := &http.Client{Timeout: 5 * time.Minute}
		resp, err := client.Get(url)
		if err != nil {
			return "", fmt.Errorf("下载失败: %w", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return "", fmt.Errorf("下载返回 %d", resp.StatusCode)
		}
		src = resp.Body
	}
	defer src.Close()

	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, src)
	f.Close()
	if err != nil {
		os.Remove(dst)
		return "", fmt.Errorf("写入暂存失败: %w", err)
	}

	// 大小校验
	if m.Size > 0 && n != m.Size {
		os.Remove(dst)
		return "", fmt.Errorf("大小不符: 期望 %d 实际 %d", m.Size, n)
	}

	// sha256 校验（元数据提供时）
	if m.SHA256 != "" {
		sum := sha256Sum(dst)
		if sum != m.SHA256 {
			os.Remove(dst)
			return "", fmt.Errorf("sha256 校验失败")
		}
	}

	// 格式校验：二进制模块必须是 ELF，前端必须是 tar.gz
	if module == "backend" || module == "mcp" {
		if err := verifyELF(dst); err != nil {
			os.Remove(dst)
			return "", err
		}
	} else {
		if err := verifyTarGz(dst); err != nil {
			os.Remove(dst)
			return "", err
		}
	}
	return dst, nil
}

func sha256Sum(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func verifyELF(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return fmt.Errorf("文件过小")
	}
	if head[0] != 0x7f || head[1] != 'E' || head[2] != 'L' || head[3] != 'F' {
		return fmt.Errorf("不是有效的 Linux 可执行文件")
	}
	return nil
}

func verifyTarGz(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("不是有效的 tar.gz")
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar 读取失败")
		}
		if hdr.Name == "index.html" || hdr.Name == "./index.html" || hdr.Name == "index.html/" {
			return nil // 包含 index.html 即有效前端包
		}
	}
	return fmt.Errorf("前端包缺少 index.html")
}

// ============ 安装与生效 ============

var upgradeMu = make(chan struct{}, 1) // 简单升级锁

// UpgradeModule 执行单模块升级（校验→备份→安装→生效）
func UpgradeModule(db *sql.DB, module, mode string) error {
	select {
	case upgradeMu <- struct{}{}:
		defer func() { <-upgradeMu }()
	default:
		return fmt.Errorf("已有升级任务在进行中")
	}

	from := GetInstalledVersion(db, module)
	m, err := fetchLatest(db, module)
	if err != nil {
		logUpgrade(db, module, from, "", "failed", err.Error(), mode)
		return err
	}
	if m.Version == from {
		return fmt.Errorf("已是最新版本 %s", from)
	}

	pkg, err := downloadPackage(db, module, m)
	if err != nil {
		logUpgrade(db, module, from, m.Version, "failed", err.Error(), mode)
		return err
	}

	os.MkdirAll(upgradeBackupDir, 0755)
	stamp := time.Now().Format("20060102-150405")

	switch module {
	case "backend":
		err = installBackend(db, pkg, m.Version, stamp, mode, from)
	case "mcp":
		err = installMCP(db, pkg, m.Version, stamp, mode, from)
	case "frontend":
		err = installFrontend(db, pkg, m.Version, stamp, mode, from)
	}
	return err
}

// installBackend 备份→替换→打挂起标记→自替换进程镜像
func installBackend(db *sql.DB, pkg, toVer, stamp, mode, fromVer string) error {
	binPath := filepath.Join(".", backendBinary)
	bakPath := filepath.Join(upgradeBackupDir, backendBinary+"."+stamp)

	// 备份
	if err := copyFile(binPath, bakPath); err != nil {
		return fmt.Errorf("备份失败: %w", err)
	}
	// 替换
	if err := copyFile(pkg, binPath+".new"); err != nil {
		return fmt.Errorf("写入新二进制失败: %w", err)
	}
	if err := os.Chmod(binPath+".new", 0755); err != nil {
		return err
	}
	// 安装成功标记（新进程启动后确认；5 分钟未确认由下次启动回滚）
	settingSet(db, "upgrade_pending_backend", fmt.Sprintf(`{"from":"%s","to":"%s","deadline":%d,"bak":%q,"mode":%q}`,
		fromVer, toVer, time.Now().Add(5*time.Minute).Unix(), bakPath, mode))
	settingSet(db, "installed_version_backend", toVer)

	// 自替换：rename 新二进制到位，然后 exec 替换进程镜像（PID 不变）
	if err := os.Rename(binPath+".new", binPath); err != nil {
		settingSet(db, "upgrade_pending_backend", "")
		return fmt.Errorf("替换二进制失败: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		self = binPath
	}
	logUpgrade(db, "backend", fromVer, toVer, "restarting", "二进制已替换，正在重启进程", mode)
	// flush DB（WAL checkpoint 一次，确保新进程读到最新数据）
	db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	go func() {
		time.Sleep(200 * time.Millisecond)
		syscall.Exec(self, os.Args, os.Environ())
	}()
	return nil
}

// HandlePendingUpgrade 进程启动时调用：确认升级成功或超时回滚
func HandlePendingUpgrade(db *sql.DB) {
	pending := settingGet(db, "upgrade_pending_backend")
	if pending == "" {
		return
	}
	var p struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Deadline int64  `json:"deadline"`
		Bak      string `json:"bak"`
		Mode     string `json:"mode"`
	}
	if err := json.Unmarshal([]byte(pending), &p); err != nil {
		settingSet(db, "upgrade_pending_backend", "")
		return
	}
	mode := p.Mode
	if mode == "" {
		mode = "manual"
	}
	now := time.Now().Unix()
	if now <= p.Deadline {
		// 新版本正常启动 = 升级成功
		logUpgrade(db, "backend", p.From, p.To, "success", "新版本启动成功", mode)
		settingSet(db, "upgrade_pending_backend", "")
		return
	}
	// 超时：新版本 5 分钟内没能正常完成启动 → 回滚
	if p.Bak != "" {
		binPath := filepath.Join(".", backendBinary)
		if _, err := os.Stat(p.Bak); err == nil {
			copyFile(p.Bak, binPath+".rollback")
			os.Rename(binPath+".rollback", binPath)
			logUpgrade(db, "backend", p.From, p.To, "rolled_back", "升级超时未确认，已回滚到 "+p.From, mode)
			settingSet(db, "upgrade_pending_backend", "")
			settingSet(db, "installed_version_backend", p.From)
			// 回滚后重启进程
			self, _ := os.Executable()
			go func() {
				time.Sleep(200 * time.Millisecond)
				syscall.Exec(self, os.Args, os.Environ())
			}()
			return
		}
	}
	settingSet(db, "upgrade_pending_backend", "")
}

// installMCP 备份→替换→重启 mcp-server 进程
func installMCP(db *sql.DB, pkg, toVer, stamp, mode, fromVer string) error {
	binPath := filepath.Join(".", mcpBinary)
	bakPath := filepath.Join(upgradeBackupDir, mcpBinary+"."+stamp)

	if err := copyFile(binPath, bakPath); err != nil {
		// mcp-server 可能尚未存在
		if !os.IsNotExist(err) {
			return fmt.Errorf("备份失败: %w", err)
		}
	}
	if err := copyFile(pkg, binPath+".new"); err != nil {
		return fmt.Errorf("写入新二进制失败: %w", err)
	}
	os.Chmod(binPath+".new", 0755)
	if err := os.Rename(binPath+".new", binPath); err != nil {
		return fmt.Errorf("替换失败: %w", err)
	}
	settingSet(db, "installed_version_mcp", toVer)

	// 重启 mcp-server（独立进程，不影响主平台）
	exec.Command("pkill", "-x", "mcp-server").Run()
	time.Sleep(500 * time.Millisecond)
	cmd := exec.Command("setsid", "nohup", "./"+mcpBinary)
	cmd.Dir = "."
	cmd.Stdout, _ = os.OpenFile("/tmp/mcp-server.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err := cmd.Start(); err != nil {
		logUpgrade(db, "mcp", fromVer, toVer, "failed", "重启失败: "+err.Error(), mode)
		return fmt.Errorf("mcp-server 重启失败: %w", err)
	}
	logUpgrade(db, "mcp", fromVer, toVer, "success", "已安装并重启", mode)
	return nil
}

// installFrontend 备份→解包→覆盖 frontend-dist（免重启）
func installFrontend(db *sql.DB, pkg, toVer, stamp, mode, fromVer string) error {
	tmpDir := filepath.Join(upgradeStagingDir, "fe-"+stamp)
	os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)

	if err := extractTarGz(pkg, tmpDir); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("解包失败: %w", err)
	}
	// 校验 index.html 存在（可能在根或 ./ 前缀下）
	if _, err := os.Stat(filepath.Join(tmpDir, "index.html")); err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("前端包缺少 index.html")
	}

	// 备份当前
	distAbs, _ := filepath.Abs(frontendDistDir)
	bakPath := filepath.Join(upgradeBackupDir, "frontend-dist."+stamp)
	if _, err := os.Stat(distAbs); err == nil {
		if out, err := exec.Command("cp", "-r", distAbs, bakPath).CombinedOutput(); err != nil {
			os.RemoveAll(tmpDir)
			return fmt.Errorf("备份失败: %s %v", out, err)
		}
	}

	// 覆盖内容（保留目录本身与符号链接位置）
	entries, _ := os.ReadDir(tmpDir)
	for _, e := range entries {
		name := e.Name()
		if name == "vendor" {
			// 保留既有 vendor（Guacamole 库等非 vite 产物），除非包里明确带了
			continue
		}
		target := filepath.Join(distAbs, name)
		if e.IsDir() {
			os.RemoveAll(target)
		} else {
			os.Remove(target)
		}
		if err := exec.Command("cp", "-r", filepath.Join(tmpDir, name), target).Run(); err != nil {
			return fmt.Errorf("覆盖 %s 失败: %w", name, err)
		}
	}
	os.RemoveAll(tmpDir)
	settingSet(db, "installed_version_frontend", toVer)
	logUpgrade(db, "frontend", fromVer, toVer, "success", "已安装（免重启）", mode)
	return nil
}

// RunAutoUpgrade 循环：按模块策略自动升级（auto 才动，manual 只记检查日志）
func RunAutoUpgrade(db *sql.DB) {
	go func() {
		for {
			interval := GetUpgradeInterval(db)
			time.Sleep(time.Duration(interval) * time.Minute)
			for _, module := range upgradeModules {
				if GetUpgradePolicy(db, module) != "auto" {
					continue
				}
				st, err := checkModule(db, module)
				if err != nil || st == nil || !st.HasUpdate {
					continue
				}
				if err := UpgradeModule(db, module, "auto"); err != nil {
					// 失败已记日志，继续下一模块
					continue
				}
				// backend 升级会 exec 重启本进程，不会走到这里
			}
		}
	}()
}

// ============ 工具函数 ============

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// extractTarGz 解包 tar.gz 到目录
func extractTarGz(path, dest string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(hdr.Name)
		if name == "." || name == ".." || filepath.IsAbs(name) {
			continue
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			io.Copy(out, tr)
			out.Close()
		}
	}
	return nil
}

// EnsureUpgradeDirs 建升级工作目录
func EnsureUpgradeDirs() {
	os.MkdirAll(upgradeStagingDir, 0755)
	os.MkdirAll(upgradeBackupDir, 0755)
	os.MkdirAll(upgradeRepoDir, 0755)
}

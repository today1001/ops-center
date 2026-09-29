package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"ops-center/internal/models"
)

// ===================== 救援控制台（主界面不可用时的诊断修复页） =====================
// 页面由后端二进制直接提供，不依赖前端构建产物。
// 访问 /rescue.html，页面内的 API 需管理员令牌。

var rescueStart = time.Now()
var rescueRestarting sync.Mutex

const rescueHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<title>Ops-Center 救援控制台</title>
<style>
body{margin:0;background:#0f172a;color:#e2e8f0;font-family:system-ui,sans-serif;padding:20px}
h1{font-size:20px;margin:0 0 4px} .sub{color:#94a3b8;font-size:12px;margin-bottom:16px}
.card{background:#1e293b;border:1px solid #334155;border-radius:8px;padding:14px;margin-bottom:14px}
.card h3{margin:0 0 10px;font-size:14px;color:#7dd3fc}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(220px,1fr));gap:10px}
.item{background:#0f172a;border-radius:6px;padding:8px 10px;font-size:13px}
.item b{display:block;font-size:11px;color:#94a3b8;margin-bottom:3px;font-weight:normal}
.ok{color:#4ade80}.bad{color:#f87171}.warn{color:#facc15}
button{background:#334155;color:#e2e8f0;border:1px solid #475569;border-radius:6px;padding:6px 14px;cursor:pointer;margin-right:8px}
button.danger{background:#7f1d1d;border-color:#b91c1c}
button:hover{filter:brightness(1.2)}
#log{background:#020617;border:1px solid #334155;border-radius:6px;padding:10px;font-family:monospace;font-size:12px;white-space:pre-wrap;max-height:300px;overflow:auto;color:#94a3b8}
input{background:#0f172a;border:1px solid #475569;color:#e2e8f0;border-radius:6px;padding:6px 10px;margin-right:6px}
.row{display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-bottom:10px}
#msg{color:#facc15;font-size:13px;min-height:18px}
</style>
</head>
<body>
<h1>🛠 Ops-Center 救援控制台</h1>
<div class="sub">独立于主界面的诊断修复页 · 当主界面白屏/无法访问时使用</div>
<div id="loginBox" class="card">
  <h3>管理员登录</h3>
  <div class="row"><input id="u" placeholder="用户名" value="admin"><input id="p" type="password" placeholder="密码"><button onclick="doLogin()">登录</button></div>
  <div id="msg"></div>
</div>
<div id="main" style="display:none">
  <div class="card">
    <h3>系统体检</h3>
    <div class="grid" id="status"></div>
    <div style="margin-top:10px">
      <button onclick="loadStatus()">刷新体检</button>
      <button onclick="backupDB()">备份数据库</button>
      <button onclick="loadLog()">查看日志</button>
      <button class="danger" onclick="restart()">重启后端服务</button>
    </div>
  </div>
  <div class="card"><h3>运行日志（server.log 尾部）</h3><div id="log">点击"查看日志"加载</div></div>
</div>
<script>
var TOKEN = localStorage.getItem('rescue_token') || '';
function h(t,c){return '<div class="item"><b>'+t+'</b><span class="'+(c||'')+'">'}
function doLogin(){
  fetch('/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},
    body:JSON.stringify({username:document.getElementById('u').value,password:document.getElementById('p').value})})
  .then(r=>r.json()).then(d=>{
    if(d.token){TOKEN=d.token;localStorage.setItem('rescue_token',TOKEN);document.getElementById('loginBox').style.display='none';document.getElementById('main').style.display='';loadStatus();}
    else{document.getElementById('msg').textContent=d.error||'登录失败';}
  }).catch(e=>{document.getElementById('msg').textContent='网络错误: '+e;});
}
function authReq(url,opts){opts=opts||{};opts.headers=Object.assign({'Authorization':'Bearer '+TOKEN},opts.headers||{});return fetch(url,opts);}
function loadStatus(){
  authReq('/api/rescue/status').then(r=>r.json()).then(d=>{
    if(d.error){document.getElementById('msg').textContent=d.error;return;}
    var s='';
    s+=h('后端进程')+'运行中 (PID '+d.pid+')，已运行 '+d.uptime+'</span></div>';
    s+=h('数据库')+(d.db_ok?'<span class="ok">正常</span>':'<span class="bad">异常</span>')+'</span></div>';
    s+=h('磁盘 (/home)')+'已用 '+d.disk_used_pct+'%</span></div>';
    s+=h('前端资产 index.html')+(d.fe_index_ok?'<span class="ok">存在</span>':'<span class="bad">丢失 → 主界面会白屏！</span>');
    if(!d.fe_index_ok){s+=' <button onclick="fixAssets()">从源码重建</button>';}
    s+='</span></div>';
    s+=h('前端资产 主JS('+d.fe_js_ref+')')+(d.fe_js_ok?'<span class="ok">存在</span>':'<span class="bad">丢失</span>')+'</span></div>';
    s+=h('EasyTier')+(d.et_ok?'<span class="ok">在线 ('+d.et_peers+' 节点)</span>':'<span class="warn">不可用</span>')+'</span></div>';
    s+=h('数据库备份')+'最近: '+(d.last_backup||'无')+'</span></div>';
    s+=h('Go 版本')+d.go_version+'</span></div>';
    document.getElementById('status').innerHTML=s;
  }).catch(e=>{document.getElementById('msg').textContent='请求失败: '+e;});
}
function fixAssets(){authReq('/api/rescue/fix-assets',{method:'POST'}).then(r=>r.json()).then(d=>{document.getElementById('msg').textContent=d.message||d.error;loadStatus();});}
function backupDB(){authReq('/api/rescue/backup-db',{method:'POST'}).then(r=>r.json()).then(d=>{document.getElementById('msg').textContent=d.message||d.error;loadStatus();});}
function loadLog(){authReq('/api/rescue/log?lines=60').then(r=>r.json()).then(d=>{document.getElementById('log').textContent=d.log||d.error;});}
function restart(){if(!confirm('确定重启后端服务？'))return;authReq('/api/rescue/restart',{method:'POST'}).then(r=>r.json()).then(d=>{document.getElementById('msg').textContent=d.message||d.error;});}
if(TOKEN){document.getElementById('loginBox').style.display='none';document.getElementById('main').style.display='';loadStatus();}
</script>
</body>
</html>`

// RescuePageHandler 救援控制台页面（无需登录）
func RescuePageHandler(c *gin.Context) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, rescueHTML)
}

func rescueAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	return role == "admin"
}

// RescueStatusHandler 系统体检
func RescueStatusHandler(c *gin.Context) {
	if !rescueAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员"})
		return
	}
	out := gin.H{
		"pid":        os.Getpid(),
		"go_version": runtime.Version(),
		"uptime":     fmt.Sprintf("%.0f分钟", time.Since(rescueStart).Minutes()),
	}

	// 数据库
	out["db_ok"] = models.CheckDB() == nil

	// 磁盘
	var st struct{ All, Free uint64 }
	if err := statfsHome(&st); err == nil && st.All > 0 {
		out["disk_used_pct"] = fmt.Sprintf("%.0f%%", float64(st.All-st.Free)/float64(st.All)*100)
	} else {
		out["disk_used_pct"] = "未知"
	}

	// 前端资产完整性
	feDir := "frontend-dist"
	idx := filepath.Join(feDir, "index.html")
	out["fe_index_ok"] = false
	out["fe_js_ok"] = false
	out["fe_js_ref"] = "-"
	if data, err := os.ReadFile(idx); err == nil {
		out["fe_index_ok"] = true
		// 提取引用的主 JS
		s := string(data)
		if i := indexOf(s, "assets/index-"); i >= 0 {
			ref := s[i+len("assets/") : i+len("assets/")+60]
			end := indexOfAny(ref, "\"'")
			if end > 0 {
				ref = ref[:end]
			}
			out["fe_js_ref"] = ref
			if _, err := os.Stat(filepath.Join(feDir, "assets", ref)); err == nil {
				out["fe_js_ok"] = true
			}
		}
	}

	// EasyTier
	etOK, etPeers := checkEasyTier()
	out["et_ok"] = etOK
	out["et_peers"] = etPeers

	// 最近备份
	out["last_backup"] = lastBackupInfo()

	c.JSON(http.StatusOK, out)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func indexOfAny(s, chars string) int {
	for i := 0; i < len(s); i++ {
		for _, ch := range []byte(chars) {
			if s[i] == ch {
				return i
			}
		}
	}
	return -1
}

// RescueBackupDBHandler 手动备份数据库
func RescueBackupDBHandler(c *gin.Context) {
	if !rescueAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员"})
		return
	}
	src := "data/ops-center.db"
	dst := fmt.Sprintf("/home/lt/ops-backups/ops-center-%s.db", time.Now().Format("20060102-150405"))
	if err := os.MkdirAll("/home/lt/ops-backups", 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	data, err := os.ReadFile(src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取数据库失败: " + err.Error()})
		return
	}
	if err := os.WriteFile(dst, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "写入备份失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "备份成功: " + dst})
}

// RescueLogHandler 查看运行日志
func RescueLogHandler(c *gin.Context) {
	if !rescueAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员"})
		return
	}
	lines := 60
	data, err := os.ReadFile("server.log")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"log": "无法读取 server.log: " + err.Error()})
		return
	}
	all := splitLines(string(data))
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	c.JSON(http.StatusOK, gin.H{"log": joinLines(all)})
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func joinLines(ls []string) string {
	out := ""
	for _, l := range ls {
		out += l + "\n"
	}
	return out
}

// RescueRestartHandler 重启后端服务（派生独立 shell 完成停启）
func RescueRestartHandler(c *gin.Context) {
	if !rescueAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员"})
		return
	}
	if !rescueRestarting.TryLock() {
		c.JSON(http.StatusOK, gin.H{"message": "重启已在进行中"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "后端服务将在 2 秒内重启，请稍候刷新"})
	go func() {
		time.Sleep(2 * time.Second)
		cmd := execCommand("sh", "-c",
			"pkill -x ops-center; sleep 1; cd /home/lt/ops-center/backend && setsid nohup ./ops-center > server.log 2>&1 < /dev/null &")
		_ = cmd
	}()
}

// RescueFixAssetsHandler 从源码重建前端产物（npm run build + 部署）
func RescueFixAssetsHandler(c *gin.Context) {
	if !rescueAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "仅管理员"})
		return
	}
	if _, err := os.Stat("/home/lt/ops-center/repo/frontend/node_modules"); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "源码或依赖缺失，请手动执行: cd /home/lt/ops-center/repo/frontend && npm install && npm run build"})
		return
	}
	go func() {
		cmd := execCommand("sh", "-c",
			"cd /home/lt/ops-center/repo/frontend && npm run build > /tmp/rescue_build.log 2>&1 && rm -rf /home/lt/ops-center/backend/frontend-dist && mkdir -p /home/lt/ops-center/backend/frontend-dist && cp -r dist/* /home/lt/ops-center/backend/frontend-dist/ >> /tmp/rescue_build.log 2>&1 &")
		_ = cmd
	}()
	c.JSON(http.StatusOK, gin.H{"message": "前端重建已启动（约1分钟），稍后刷新体检确认"})
}

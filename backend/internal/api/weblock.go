package api

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ===================== 网页会话单控制器（演示/观看模式） =====================
// 同一网页代理会话（tk）同一时间只允许一人操作：
// - 第一个打开页面的人自动获得控制权
// - 其他人进入观看模式（页面只读 + 顶部提示条），可点"申请控制"接管
// - 控制者页面定期续约；超过 webLockTTL 未续约则自动释放锁

type webLock struct {
	holder string
	since  time.Time
}

const webLockTTL = 10 * time.Second

var (
	webLockMu sync.Mutex
	webLocks  = make(map[string]webLock)
)

func getWebLock(tk string) (webLock, bool) {
	webLockMu.Lock()
	defer webLockMu.Unlock()
	l, ok := webLocks[tk]
	if ok && time.Since(l.since) > webLockTTL {
		delete(webLocks, tk)
		return webLock{}, false
	}
	return l, ok
}

// WebLockStatusHandler GET /api/tools/web/lock?tk= 查询会话控制状态
func WebLockStatusHandler(c *gin.Context) {
	tk := c.Query("tk")
	if tk == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少tk参数"})
		return
	}
	l, ok := getWebLock(tk)
	c.JSON(http.StatusOK, gin.H{"locked": ok, "holder": l.holder})
}

// WebLockHandler POST /api/tools/web/lock {tk, user, action}
// action: acquire(获取/抢占空闲锁) / renew(续约) / release(释放)
func WebLockHandler(c *gin.Context) {
	var req struct {
		TK     string `json:"tk"`
		User   string `json:"user"`
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.TK == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误"})
		return
	}
	webLockMu.Lock()
	defer webLockMu.Unlock()
	switch req.Action {
	case "acquire", "renew":
		l, ok := webLocks[req.TK]
		if ok && time.Since(l.since) > webLockTTL {
			delete(webLocks, req.TK)
			ok = false
		}
		if ok && l.holder != req.User {
			c.JSON(http.StatusOK, gin.H{"ok": false, "holder": l.holder})
			return
		}
		webLocks[req.TK] = webLock{holder: req.User, since: time.Now()}
		c.JSON(http.StatusOK, gin.H{"ok": true, "holder": req.User})
	case "release":
		l, ok := webLocks[req.TK]
		if ok && (l.holder == req.User || req.User == "") {
			delete(webLocks, req.TK)
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "未知操作"})
	}
}

// webControlScript 注入到被代理页面的单控制器脚本
// 观看者：页面禁止输入并显示"观看模式"提示条 + 申请控制按钮
// 控制者：定期续约锁；失去锁时自动切换为观看模式
func webControlScript(tk string) string {
	return fmt.Sprintf(`<script>(function(){
  var TK=%q;var MYID;
  try{MYID=sessionStorage.getItem('wcid_'+TK);if(!MYID){MYID='u'+Math.random().toString(36).slice(2);sessionStorage.setItem('wcid_'+TK,MYID);}}catch(e){MYID='u'+Math.random().toString(36).slice(2);}
  var overlay=null;var holding=false;
  function lockAPI(a){return fetch('/api/tools/web/lock',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({tk:TK,user:MYID,action:a})}).then(function(r){return r.json();}).catch(function(){return{};});}
  function blockInput(on){
    document.documentElement.style.pointerEvents=on?'none':'';
    if(on){document.addEventListener('keydown',stopKey,true);document.addEventListener('keyup',stopKey,true);document.addEventListener('keypress',stopKey,true);}
    else{document.removeEventListener('keydown',stopKey,true);document.removeEventListener('keyup',stopKey,true);document.removeEventListener('keypress',stopKey,true);}
  }
  function stopKey(e){e.preventDefault();e.stopPropagation();}
  function showOverlay(holder){
    if(!overlay){
      overlay=document.createElement('div');
      overlay.style.cssText='position:fixed;top:0;left:0;right:0;z-index:2147483647;background:rgba(0,0,0,.78);color:#fff;font:13px/1.4 sans-serif;padding:8px 12px;display:flex;gap:12px;align-items:center;';
      var s=document.createElement('span');s.textContent='\ud83d\udc41 观看模式：'+(holder||'其他用户')+' 正在操作';
      var b=document.createElement('button');b.textContent='申请控制';b.style.cssText='padding:3px 10px;cursor:pointer;';
      b.onclick=function(){lockAPI('acquire');};
      overlay.appendChild(s);overlay.appendChild(b);
      document.body.appendChild(overlay);
    }
    holding=false;blockInput(true);overlay.style.pointerEvents='auto';
  }
  function hideOverlay(){if(overlay){overlay.parentNode.removeChild(overlay);overlay=null;}blockInput(false);}
  function tick(){
    fetch('/api/tools/web/lock?tk='+encodeURIComponent(TK)).then(function(r){return r.json();}).then(function(s){
      if(s.holder&&s.holder!==MYID){showOverlay(s.holder);return;}
      if(!s.holder){
        lockAPI('acquire').then(function(r){
          if(r.ok){hideOverlay();holding=true;lockAPI('renew');}
          else if(r.holder){showOverlay(r.holder);}
        });
        return;
      }
      hideOverlay();holding=true;lockAPI('renew');
    }).catch(function(){});
  }
  window.addEventListener('beforeunload',function(){if(holding)lockAPI('release');});
  function start(){tick();setInterval(tick,2500);}
  if(document.readyState==='loading'){document.addEventListener('DOMContentLoaded',start);}else{start();}
})();</script>`, tk)
}

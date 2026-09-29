package services

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// 内部工具访问内部HTTPS站点（多为自签名证书），跳过证书校验
var proxyTransport = &http.Transport{
	TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
	MaxIdleConns:          20,
	IdleConnTimeout:       60 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 20 * time.Second,
}

// webSession 网页代理会话（保存目标主机与Cookie，避免跨请求丢失登录态）
type webSession struct {
	User    string
	Pass    string
	Host    string
	BaseURL string
	Lang    string // PVE 语言偏好 (zh_CN, en, etc.)
	Jar     http.CookieJar
	Client  *http.Client
	// PVE 登录后服务器端存储的认证信息（浏览器端无法存储 Secure cookie）
	PVECookie   string // PVEAuthCookie=...
	CSRFToken   string // CSRFPreventionToken
}

var (
	webSessionsMu sync.RWMutex
	webSessions   = map[string]*webSession{}
)

const proxyBase = "/api/tools/web"

var headRegex = regexp.MustCompile(`(?i)<head[^>]*>`)
var formActionRegex = regexp.MustCompile(`(?i)(<form[^>]*\baction=)(["'])([^"']*)`)

// CreateWebSession 创建网页代理会话，返回会话令牌
func CreateWebSession(user, pass, target, lang string) (string, error) {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("目标地址必须是有效的 http/https 地址")
	}
	if lang == "" {
		lang = "zh_CN" // 默认中文
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 20 * time.Second, Transport: proxyTransport}
	tk := genToken()
	webSessionsMu.Lock()
	webSessions[tk] = &webSession{
		User: user, Pass: pass, Host: u.Host, Lang: lang,
		BaseURL: fmt.Sprintf("%s://%s", u.Scheme, u.Host),
		Jar: jar, Client: client,
	}
	webSessionsMu.Unlock()
	return tk, nil
}

// GetWebSessionFull 获取会话完整信息（供API代理使用）
func GetWebSessionFull(tk string) (*webSession, error) {
	webSessionsMu.RLock()
	defer webSessionsMu.RUnlock()
	s, ok := webSessions[tk]
	if !ok {
		return nil, fmt.Errorf("无效会话")
	}
	return s, nil
}

// StorePVEAuth 从 PVE 登录响应中提取并存储认证信息
func StorePVEAuth(tk string, respBody []byte, respHeader http.Header) {
	webSessionsMu.Lock()
	defer webSessionsMu.Unlock()
	s, ok := webSessions[tk]
	if !ok {
		return
	}

	// 方式一：从 Set-Cookie 头提取 PVEAuthCookie
	for _, cookie := range respHeader.Values("Set-Cookie") {
		if strings.Contains(cookie, "PVEAuthCookie") {
			// 截取 cookie 值（去掉 Path/Secure/HttpOnly 等属性）
			parts := strings.SplitN(cookie, ";", 2)
			s.PVECookie = parts[0] // "PVEAuthCookie=..."
		}
	}

	// 方式二：从 JSON 响应体提取 ticket（备用）
	if s.PVECookie == "" {
		var resp struct {
			Data struct {
				Ticket string `json:"ticket"`
			} `json:"data"`
		}
		if json.Unmarshal(respBody, &resp) == nil && resp.Data.Ticket != "" {
			s.PVECookie = "PVEAuthCookie=" + resp.Data.Ticket
		}
	}

	// 提取 CSRFPreventionToken
	var csrfResp struct {
		Data struct {
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
		} `json:"data"`
	}
	if json.Unmarshal(respBody, &csrfResp) == nil && csrfResp.Data.CSRFPreventionToken != "" {
		s.CSRFToken = csrfResp.Data.CSRFPreventionToken
	}
}

// GetPVEAuthCookie 获取存储的 PVE auth cookie
func GetPVEAuthCookie(tk string) (cookie string, csrf string) {
	webSessionsMu.RLock()
	defer webSessionsMu.RUnlock()
	s, ok := webSessions[tk]
	if !ok {
		return "", ""
	}
	return s.PVECookie, s.CSRFToken
}

// ProxyPVEResource 代理 PVE 资源请求（CSS/JS/图片等），供 SPA NoRoute 使用
func ProxyPVEResource(tk, path string, req *http.Request) (int, string, []byte) {
	webSessionsMu.RLock()
	s, ok := webSessions[tk]
	webSessionsMu.RUnlock()
	if !ok {
		return 401, "application/json", []byte(`{"error":"无效会话"}`)
	}

	// 只代理 PVE 静态资源路径
	if !strings.HasPrefix(path, "/pve2") && !strings.HasPrefix(path, "/pwt") &&
		!strings.HasPrefix(path, "/fa") &&
		path != "/qrcode.min.js" && path != "/proxmoxlib.js" &&
		!strings.HasPrefix(path, "/api2") {
		return 0, "", nil
	}

	targetURL := s.BaseURL + path
	if req.URL.RawQuery != "" {
		targetURL += "?" + req.URL.RawQuery
	}

	// 先读取 body，用 bytes.NewReader 确保有明确的 Content-Length（PVE 不支持 chunked）
	var reqBody []byte
	if req.Body != nil {
		reqBody, _ = io.ReadAll(io.LimitReader(req.Body, 50<<20))
	}

	proxyReq, err := http.NewRequest(req.Method, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		return 502, "application/json", []byte(`{"error":"` + err.Error() + `"}`)
	}
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	proxyReq.Header.Set("Referer", s.BaseURL)
	proxyReq.Header.Set("Accept", "*/*")
	proxyReq.Header.Set("Accept-Encoding", "identity")
	// 转发原始 Content-Type（POST 表单/JSON 等需要）
	if ct := req.Header.Get("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}
	for key, values := range req.Header {
		for _, v := range values {
			if !strings.EqualFold(key, "Cookie") && !strings.EqualFold(key, "Accept-Encoding") {
				proxyReq.Header.Add(key, v)
			}
		}
	}
	if s.User != "" {
		proxyReq.SetBasicAuth(s.User, s.Pass)
	}

	resp, err := s.Client.Do(proxyReq)
	if err != nil {
		return 502, "application/json", []byte(`{"error":"代理失败: ` + err.Error() + `"}`)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	return resp.StatusCode, ct, body
}

// CleanupWebSessions 清理过期的代理会话（每次清理掉所有旧会话）
func CleanupWebSessions() {
	webSessionsMu.Lock()
	webSessions = map[string]*webSession{}
	webSessionsMu.Unlock()
}

func genToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])[:32]
}

// ProxyWeb 代理访问目标网页（保留Cookie、注入base href + 自动填充脚本；仅重写表单action）
func ProxyWeb(tk, target string, autoSubmit bool) (int, string, []byte) {
	webSessionsMu.RLock()
	s, ok := webSessions[tk]
	webSessionsMu.RUnlock()
	if !ok {
		return http.StatusUnauthorized, "application/json", []byte(`{"error":"无效的代理会话，请重新连接"}`)
	}

	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return http.StatusBadRequest, "application/json", []byte(`{"error":"无效的目标地址"}`)
	}
	if u.Host != s.Host {
		return http.StatusForbidden, "application/json", []byte(`{"error":"目标地址超出会话允许范围"}`)
	}

	client := s.Client
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return http.StatusBadRequest, "application/json", []byte(`{"error":"请求构建失败"}`)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	if s.User != "" {
		req.SetBasicAuth(s.User, s.Pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		msg, _ := json.Marshal(fmt.Sprintf("代理请求失败: %v", err))
		return http.StatusBadGateway, "application/json", []byte(`{"error":` + string(msg) + `}`)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		msg, _ := json.Marshal(fmt.Sprintf("读取目标内容失败: %v", err))
		return http.StatusBadGateway, "application/json", []byte(`{"error":` + string(msg) + `}`)
	}

	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)
	if mt == "text/html" || strings.Contains(strings.ToLower(ct), "html") {
		body = injectHtmlHelpers(body, target, tk, autoSubmit, s)
		ct = "text/html; charset=utf-8"
	}

	return http.StatusOK, ct, body
}

// ProxyWebWithBody 处理 POST 表单提交（登录等）
func ProxyWebWithBody(c *gin.Context, tk, target string, autoSubmit bool) {
	webSessionsMu.RLock()
	s, ok := webSessions[tk]
	webSessionsMu.RUnlock()
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效的代理会话"})
		return
	}

	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != s.Host {
		c.JSON(http.StatusForbidden, gin.H{"error": "目标地址超出会话允许范围"})
		return
	}

	// 读取原始请求体
	reqBody, _ := io.ReadAll(io.LimitReader(c.Request.Body, 10<<20))

	// 构建转发请求
	proxyReq, err := http.NewRequest(c.Request.Method, target, bytes.NewReader(reqBody))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "构建请求失败"})
		return
	}
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	proxyReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	// 转发原始 Content-Type（表单 POST 需要）
	if ct := c.GetHeader("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}
	// 转发 Referer
	if ref := c.GetHeader("Referer"); ref != "" {
		proxyReq.Header.Set("Referer", ref)
	}

	// 登录请求：不加 Basic Auth（PVE 优先用 Basic Auth 会忽略 POST body 中的凭据）
	isLoginReq := strings.Contains(target, "access/ticket")
	if !isLoginReq && s.User != "" {
		proxyReq.SetBasicAuth(s.User, s.Pass)
	}

	resp, err := s.Client.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "代理请求失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	ct := resp.Header.Get("Content-Type")
	mt, _, _ := mime.ParseMediaType(ct)

	// 如果是 HTML 响应，注入代理助手（重写 action 等）
	if mt == "text/html" || strings.Contains(strings.ToLower(ct), "html") {
		// 用响应的 Location 头（如果有重定向）作为当前 URL
		newTarget := target
		if loc := resp.Header.Get("Location"); loc != "" {
			if locURL, e := url.Parse(loc); e == nil {
				newTarget = u.ResolveReference(locURL).String()
			}
		}
		body = injectHtmlHelpers(body, newTarget, tk, autoSubmit, s)
		ct = "text/html; charset=utf-8"
	}

	// 设置 cookie（PVE 登录成功后会 Set-Cookie）
	for _, cookie := range resp.Cookies() {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     cookie.Name,
			Value:    cookie.Value,
			Path:     cookie.Path,
			Domain:   cookie.Domain,
			Expires:  cookie.Expires,
			MaxAge:   cookie.MaxAge,
			HttpOnly: cookie.HttpOnly,
			SameSite: cookie.SameSite,
		})
	}

	c.Data(resp.StatusCode, ct, body)
}

// injectHtmlHelpers 注入 base href（让浏览器原生正确加载所有静态资源）
// 仅重写 <form action> 确保表单提交走代理，其余资源由浏览器直连目标加载
func injectHtmlHelpers(body []byte, currentURL, tk string, autoSubmit bool, s *webSession) []byte {
	html := string(body)

	// 1. 注入 <base href> 指向目标源，让所有相对路径在浏览器端正确解析（CSS/JS/图片均可正常加载）
	cur, err := url.Parse(currentURL)
	if err == nil {
		baseTag := fmt.Sprintf(`<base href="%s://%s/">`, cur.Scheme, cur.Host)
		html = headRegex.ReplaceAllStringFunc(html, func(m string) string {
			return m + baseTag
		})
	}

	// 2. 仅重写 <form action> 为空或同主机相对路径 → 走代理（保持登录态Cookie）
	html = formActionRegex.ReplaceAllStringFunc(html, func(m string) string {
		mm := formActionRegex.FindStringSubmatch(m)
		val := mm[3]
		// 保留锚点、空action、javascript: 等不动；相对路径表单提交走代理
		if val == "" || strings.HasPrefix(val, "#") || strings.HasPrefix(val, "javascript:") {
			return m
		}
		ref, err := url.Parse(val)
		if err != nil {
			return m
		}
		abs := cur.ResolveReference(ref)
		if (abs.Scheme == "http" || abs.Scheme == "https") && abs.Host == s.Host {
			return mm[1] + mm[2] + fmt.Sprintf("%s?url=%s&tk=%s", proxyBase, url.QueryEscape(abs.String()), tk) + mm[2]
		}
		return m
	})

	// 空action表单提交到代理（POST 当前页面）
	html = strings.ReplaceAll(html, `action=""`, fmt.Sprintf(`action="%s?url=%s&tk=%s"`, proxyBase, url.QueryEscape(currentURL), tk))
	html = strings.ReplaceAll(html, `action=''`, fmt.Sprintf(`action="%s?url=%s&tk=%s"`, proxyBase, url.QueryEscape(currentURL), tk))

	// 3a. 早期拦截（</head>）：XHR + Ext.Ajax 双重覆盖
	earlyIntercept := `<script>
(function(){
  var _RD=[{"realm":"pam","type":"pam","comment":"Linux PAM standard authentication"},{"realm":"pve","type":"pve","comment":"Proxmox VE authentication server"}];
  var _RJ=JSON.stringify({data:_RD});
  var _TK=` + "`" + tk + "`" + `;
  // === 1. 拦截 XMLHttpRequest（domains + login） ===
  var _oOpen=XMLHttpRequest.prototype.open;
  var _oSend=XMLHttpRequest.prototype.send;
  XMLHttpRequest.prototype.open=function(m,u){
    this._isDomains=(typeof u==='string'&&u.indexOf('access/domains')>=0);
    this._isLogin=(typeof u==='string'&&u.indexOf('access/ticket')>=0);
    this._myUrl=u;
    return _oOpen.apply(this,arguments);};
  XMLHttpRequest.prototype.send=function(b){
    if(this._isDomains){
      // realms 请求：直接返回假数据
      var s=this;
      try{Object.defineProperty(s,'readyState',{writable:true,value:4});
      Object.defineProperty(s,'status',{writable:true,value:200});
      Object.defineProperty(s,'responseText',{writable:true,value:_RJ});
      Object.defineProperty(s,'response',{writable:true,value:_RJ});}catch(e){}
      setTimeout(function(){try{s.onreadystatechange&&s.onreadystatechange();}catch(e){}try{s.onload&&s.onload();}catch(e){}try{s.dispatchEvent(new Event('readystatechange'));}catch(e){}try{s.dispatchEvent(new Event('load'));}catch(e){}try{s.dispatchEvent(new Event('loadend'));}catch(e){}},10);
      return;
    }
    if(this._isLogin){
      // 登录请求：通过反向代理 POST（/px/{tk}/path）
      var self=this;var realUrl=self._myUrl;
      var pxUrl='/px/'+_TK+realUrl;
      // 如果 username 已包含 @realm，去掉 body 中的 realm 参数（PVE 不允许重复）
      var bodyStr=typeof b==='string'?b:'';
      if(bodyStr.indexOf('username=')>=0 && bodyStr.indexOf('@')>=0){
        bodyStr=bodyStr.replace(/&?realm=[^&]*/g,'').replace(/^&/,'');
      }
      var px=new XMLHttpRequest();
      px.open('POST',pxUrl,true);
      px.setRequestHeader('Content-Type','application/x-www-form-urlencoded');
      px.responseType='text';
      px.onload=function(){
        try{Object.defineProperty(self,'status',{writable:true,value:px.status});}catch(e){}
        try{Object.defineProperty(self,'readyState',{writable:true,value:4});}catch(e){}
        try{Object.defineProperty(self,'responseText',{writable:true,value:px.responseText});}catch(e){}
        try{Object.defineProperty(self,'response',{writable:true,value:px.responseText});}catch(e){}
        try{self.onreadystatechange&&self.onreadystatechange();}catch(e){}
        try{self.onload&&self.onload();}catch(e){}
        try{self.dispatchEvent(new Event('readystatechange'));}catch(e){}
        try{self.dispatchEvent(new Event('load'));}catch(e){}
        try{self.dispatchEvent(new Event('loadend'));}catch(e){}
      };
      px.onerror=function(){try{self.status=0;self.onerror&&self.onerror();}catch(e){}};
      px.send(b);
      return;
    }
    return _oSend.apply(this,arguments);};
  // === 2. 等 ExtJS 加载后覆盖 Ext.Ajax.request ===
  function _patchAjax(){
    if(typeof Ext==='undefined'||!Ext.Ajax||!Ext.Ajax.request)return;
    var orig=Ext.Ajax.request;
    Ext.Ajax.request=function(opts){
      var url=(opts&&opts.url)?opts.url:'';
      if(typeof url==='string'&&url.indexOf('access/domains')>=0){
        var resp={responseText:_RJ,status:200,statusText:'OK',getResponseHeader:function(h){return h==='content-type'?'application/json':null;}};
        setTimeout(function(){if(opts.success)opts.success.call(opts.scope||this,resp);if(opts.callback)opts.callback.call(opts.scope||this,true,resp);},30);
        return null;
      }
      if(typeof url==='string'&&url.indexOf('access/ticket')>=0){
        var body='';
        if(opts.params){body=Object.keys(opts.params).map(function(k){return encodeURIComponent(k)+'='+encodeURIComponent(opts.params[k]);}).join('&');}
        // 如果 username 已包含 @realm，去掉 body 中的 realm 参数
        if(body.indexOf('username=')>=0 && body.indexOf('@')>=0){
          body=body.replace(/&?realm=[^&]*/g,'').replace(/^&/,'');
        }
        var pxUrl='/px/'+_TK+url;
        var px=new XMLHttpRequest();
        px.open('POST',pxUrl,true);
        px.setRequestHeader('Content-Type','application/x-www-form-urlencoded');
        px.onload=function(){
          var r={responseText:px.responseText,status:px.status,statusText:px.statusText,getResponseHeader:function(h){return h==='content-type'?'application/json':null;}};
          if(px.status>=200&&px.status<300){if(opts.success)opts.success.call(opts.scope||this,r);}
          else{if(opts.failure)opts.failure.call(opts.scope||this,r);}
          if(opts.callback)opts.callback.call(opts.scope||this,px.status>=200&&px.status<300,r);
        };
        px.onerror=function(){if(opts.failure)opts.failure.call(opts.scope||this,{status:0});if(opts.callback)opts.callback.call(opts.scope||this,false,{status:0});};
        px.send(body);
        return{xhr:px};
      }
      return orig.apply(Ext.Ajax,arguments);
    };
  }
  function _waitExt(){if(typeof Ext!=='undefined'&&Ext.Ajax&&Ext.Ajax.request){_patchAjax();return;}setTimeout(_waitExt,300);}
  _waitExt();
})();
</script>`
	html = strings.Replace(html, "</head>", earlyIntercept+"</head>", 1)

	// 3b. 后期填充（</body>）：直接注入 <select> 覆盖 ExtJS Realm 下拉框
	realmScript := `<script>
(function(){
  var _REALMS=[{v:'pam',t:'Linux PAM standard authentication'},{v:'pve',t:'Proxmox VE authentication server'}];
  function _inject(){
    // 找到 "Realm" 文本所在的单元格
    var all=document.querySelectorAll('td,label,span');
    for(var i=0;i<all.length;i++){
      var txt=(all[i].textContent||'').trim();
      if(txt==='Realm:'||txt==='Realm'||txt==='领域:'||txt==='领域'){
        var cell=all[i];
        // 找到同行的下一个 td（input 区域）
        var row=cell.closest('tr')||cell.parentElement;
        if(!row) continue;
        var tds=row.querySelectorAll('td');
        var inputCell=null;
        for(var j=0;j<tds.length;j++){
          if(tds[j]!==cell && tds[j].querySelector('input,.x-form-combo,.x-form-field')){
            inputCell=tds[j];
            break;
          }
        }
        if(!inputCell) inputCell=cell.nextElementSibling;
        if(!inputCell) continue;
        // 检查是否已注入
        if(inputCell.querySelector('#_pv_realm_sel')) return true;
        // 创建 <select>
        var sel=document.createElement('select');
        sel.id='_pv_realm_sel';
        sel.name='realm';
        sel.style.cssText='width:180px;padding:3px;font-size:12px;';
        for(var k=0;k<_REALMS.length;k++){
          var o=document.createElement('option');
          o.value=_REALMS[k].v;
          o.textContent=_REALMS[k].t;
          sel.appendChild(o);
        }
        sel.value='pam';
        inputCell.appendChild(sel);
        // 隐藏原有 ExtJS combo
        var extEl=inputCell.querySelector('.x-form-combo,.x-form-field-wrap,.x-form-item');
        if(extEl) extEl.style.display='none';
        return true;
      }
    }
    return false;
  }
  // 高频率重试
  var tries=0;
  var timer=setInterval(function(){
    tries++;
    if(_inject()||tries>40) clearInterval(timer);
  },250);
})();
</script>`
	html = strings.Replace(html, "</body>", realmScript+"</body>", 1)

	// 4. 注入自动填充脚本
	script := AutofillScript(s.User, s.Pass)
	if strings.Contains(strings.ToLower(html), "</head>") {
		html = strings.Replace(html, "</head>", script+"</head>", 1)
	} else {
		html = script + html
	}

	return []byte(html)
}

// autofillScript 生成自动填充用户名/密码的脚本（兼容 ExtJS / 动态表单）
func AutofillScript(user, pass string) string {
	u, _ := json.Marshal(user)
	p, _ := json.Marshal(pass)
	return fmt.Sprintf(`<script>
(function(){
  var ue = %s;
  var pe = %s;
  var done = false;

  function tryFill(){
    if (done) return false;

    // 1. 精准定位：优先用 autocomplete 属性（PVE / 标准表单）
    var pw = document.querySelector('input[type="password"]')
           || document.querySelector('input[autocomplete="current-password"]');
    if (!pw) return false;

    var uf = document.querySelector('input[autocomplete="username"]')
          || document.querySelector('input[autocomplete="email"]');

    // 2. 备选：在密码框所在容器中找密码框之前的第一个 text 类输入
    if (!uf) {
      var container = pw.form || pw.closest('form') || pw.closest('[class*="form"]') || pw.parentElement.parentElement || document.body;
      var inputs = container.querySelectorAll('input');
      for (var i = 0; i < inputs.length; i++) {
        var el = inputs[i];
        if (el === pw) break;
        if (el.type === 'password' || el.type === 'hidden' || el.type === 'submit' ||
            el.type === 'button' || el.type === 'checkbox' || el.type === 'radio' ||
            el.type === 'file' || el.disabled) continue;
        uf = el;
      }
    }

    // 3. 全局兜底：找第一个可用的 text 类输入
    if (!uf) {
      var all = document.querySelectorAll('input[type="text"], input[type="email"], input:not([type])');
      for (var j = 0; j < all.length; j++) {
        if (all[j] !== pw && !all[j].disabled && all[j].offsetParent !== null) {
          uf = all[j];
          break;
        }
      }
    }

    // 4. 填入（触发 input/change 事件让 ExtJS / 框架感知）
    var filled = false;
    if (uf) {
      try { uf.focus(); } catch(e) {}
      uf.value = ue;
      uf.dispatchEvent(new Event('input', {bubbles:true}));
      uf.dispatchEvent(new Event('change', {bubbles:true}));
      filled = true;
    }
    try { pw.focus(); } catch(e) {}
    pw.value = pe;
    pw.dispatchEvent(new Event('input', {bubbles:true}));
    pw.dispatchEvent(new Event('change', {bubbles:true}));

    // 5. 两个都填了才标记完成
    if (filled) {
      done = true;
      if (false) {
        var f = pw.form || pw.closest('form');
        if (f) setTimeout(function(){ if (typeof f.submit === 'function') f.submit(); }, 400);
      }
      return true;
    }
    return false;
  }

  // 初始尝试
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function(){ setTimeout(tryFill, 100); });
  } else {
    setTimeout(tryFill, 100);
  }
  // 多次重试（ExtJS 动态渲染需要时间）
  [300, 600, 1000, 1800, 3000, 5000, 8000].forEach(function(ms){ setTimeout(tryFill, ms); });

  // 监听动态插入的 DOM（ExtJS 弹窗/表单异步创建）
  if (document.body) {
    var obs = new MutationObserver(function(){
      if (!done) tryFill();
    });
    obs.observe(document.body, { childList: true, subtree: true });
    setTimeout(function(){ obs.disconnect(); }, 12000);
  }
})();
</script>`, string(u), string(p))
}

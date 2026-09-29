package api

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"ops-center/internal/services"
)

var reRealmParam = regexp.MustCompile(`&?realm=[^&]*`)
var reRealmValue = regexp.MustCompile(`(?:^|&)realm=([^&]*)`)
var reUsername = regexp.MustCompile(`(?:^|&)username=[^&]*`)
var reSaveUsername = regexp.MustCompile(`&?saveusername=[^&]*`)

// CreateWebSessionHandler 创建网页代理会话
func CreateWebSessionHandler(c *gin.Context) {
	var req struct {
		URL  string `json:"url"`
		User string `json:"user"`
		Pass string `json:"pass"`
		Lang string `json:"lang"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少目标地址"})
		return
	}
	tk, err := services.CreateWebSession(req.User, req.Pass, req.URL, req.Lang)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tk": tk})
}

// ProxyWebHandler 旧版网页代理
func ProxyWebHandler(c *gin.Context) {
	target := c.Query("url")
	tk := c.Query("tk")
	autoSubmit := c.Query("auto_submit") == "1"
	if target == "" || tk == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 url 或 tk 参数"})
		return
	}
	status, contentType, body := services.ProxyWeb(tk, target, autoSubmit)
	c.Data(status, contentType, body)
}

// ProxyWebPostHandler POST 表单提交
func ProxyWebPostHandler(c *gin.Context) {
	target := c.Query("url")
	tk := c.Query("tk")
	autoSubmit := c.Query("auto_submit") == "1"
	if target == "" || tk == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 url 或 tk 参数"})
		return
	}
	services.ProxyWebWithBody(c, tk, target, autoSubmit)
}

// ProxyAPIHandler 转发 ExtJS API 请求
func ProxyAPIHandler(c *gin.Context) {
	tk := c.Query("tk")
	path := c.Query("path")
	if tk == "" || path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少参数"})
		return
	}
	sess, err := services.GetWebSessionFull(tk)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效会话"})
		return
	}
	baseURL, _ := url.Parse(sess.BaseURL)
	apiURL, _ := url.Parse(path)
	fullURL := baseURL.ResolveReference(apiURL)
	proxyReq, _ := http.NewRequest(c.Request.Method, fullURL.String(), c.Request.Body)
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Ops-Center WebProxy)")
	proxyReq.Header.Set("Accept", "application/json, text/plain, */*")
	proxyReq.Header.Set("Referer", sess.BaseURL)
	if sess.User != "" {
		proxyReq.SetBasicAuth(sess.User, sess.Pass)
	}
	resp, err := sess.Client.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "转发失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/json"
	}
	c.Data(resp.StatusCode, ct, body)
}

// ReverseProxyHandler 全路径反向代理：/px/{tk}/path → target/path
// 不使用 httputil.ReverseProxy，直接用 HTTP Client 以避免 Transfer-Encoding 问题
func ReverseProxyHandler(c *gin.Context) {
	tk := c.Param("tk")
	if tk == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "缺少 tk 参数"})
		return
	}
	sess, err := services.GetWebSessionFull(tk)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "无效会话"})
		return
	}

	reqPath := c.Param("path")
	if reqPath == "" {
		reqPath = "/"
	}

	targetURL := sess.BaseURL + reqPath
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// 先读取 body，用 bytes.NewReader 确保有明确的 Content-Length（PVE 不支持 chunked）
	var reqBody []byte
	if c.Request.Body != nil {
		reqBody, _ = io.ReadAll(io.LimitReader(c.Request.Body, 50<<20))
	}

	isLoginReq := strings.Contains(targetURL, "access/ticket")

	// 登录请求：去掉客户端参数，合并 realm 到 username
	if isLoginReq && len(reqBody) > 0 {
		body := string(reqBody)

		// 去掉 PVE 不认识的客户端参数
		body = reSaveUsername.ReplaceAllString(body, "")
		body = strings.TrimPrefix(body, "&")

		if strings.Contains(body, "username=") && strings.Contains(body, "realm=") {
			realmMatch := reRealmValue.FindStringSubmatch(body)
			body = reRealmParam.ReplaceAllString(body, "")
			body = strings.TrimPrefix(body, "&")
			if len(realmMatch) > 1 && realmMatch[1] != "" {
				realm := realmMatch[1]
				body = reUsername.ReplaceAllStringFunc(body, func(match string) string {
					prefix := ""
					if strings.HasPrefix(match, "&") {
						prefix = "&"
						match = strings.TrimPrefix(match, "&")
					}
					parts := strings.SplitN(match, "=", 2)
					if len(parts) == 2 {
						val, _ := url.QueryUnescape(parts[1])
						if !strings.Contains(val, "@") {
							val = val + "@" + realm
							return prefix + "username=" + url.QueryEscape(val)
						}
					}
					return prefix + match
				})
			}
			reqBody = []byte(body)
			log.Printf("[PROXY] Login: merged realm, body=%s", body)
		}
	}

	// 构建代理请求
	proxyReq, _ := http.NewRequest(c.Request.Method, targetURL, bytes.NewReader(reqBody))
	proxyReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	proxyReq.Header.Set("Referer", sess.BaseURL)
	proxyReq.Header.Set("Accept", "*/*")
	proxyReq.Header.Set("Accept-Encoding", "identity")
	// 转发原始 Content-Type（POST 表单/JSON 等需要）
	if ct := c.GetHeader("Content-Type"); ct != "" {
		proxyReq.Header.Set("Content-Type", ct)
	}

	if isLoginReq {
		// 登录请求：不加 Basic Auth（PVE 优先用 Basic Auth 会忽略 POST body 中的凭据）
		// 同时转发原始 Cookie（浏览器可能有 PVEAuthCookie）
		if cookie := c.GetHeader("Cookie"); cookie != "" {
			proxyReq.Header.Set("Cookie", cookie)
		}
	} else {
		// 非登录请求：注入服务器端存储的 PVE auth cookie
		pveCookie, csrf := services.GetPVEAuthCookie(tk)
		if pveCookie != "" {
			cookieHeader := pveCookie
			if csrf != "" {
				cookieHeader += "; CSRFPreventionToken=" + csrf
			}
			proxyReq.Header.Set("Cookie", cookieHeader)
		} else if sess.User != "" {
			// 无 cookie 时回退到 Basic Auth
			proxyReq.SetBasicAuth(sess.User, sess.Pass)
		}
	}

	resp, err := sess.Client.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "代理失败: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")

	// 只对 HTML 响应注入脚本
	if strings.Contains(ct, "text/html") {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "读取失败"})
			return
		}

		html := string(body)
		proxyPrefix := fmt.Sprintf("/px/%s", tk)

		// 注入完整的 PVE ExtJS 拦截脚本（realm伪装 + 登录拦截 + Ext.Ajax补丁 + realm下拉注入）
		interceptScript := fmt.Sprintf(`<script>
(function(){
  var _PX="%s";
  var _origOpen=XMLHttpRequest.prototype.open;
  var _origSend=XMLHttpRequest.prototype.send;
  function _fixUrl(u){return typeof u==='string'&&u.charAt(0)==='/'&&u.indexOf(_PX)!==0?_PX+u:u;}
  function _fixLoginBody(b){
    if(typeof b!=='string'||b.indexOf('username=')<0) return b;
    b=b.replace(/&?saveusername=[^&]*/g,'').replace(/^&/,'');
    var rm=b.match(/(?:^|&)realm=([^&]*)/);
    if(rm){
      var rv=rm[1];
      b=b.replace(/&?realm=[^&]*/g,'').replace(/^&/,'');
      if(rv&&b.indexOf('username=')>=0){
        b=b.replace(/(?:^|&)username=([^&]*)/,function(m,v){
          try{
            var prefix=m.charAt(0)==='&'?'&':'';
            var u=decodeURIComponent(v);
            if(u.indexOf('@')<0)return prefix+'username='+encodeURIComponent(u+'@'+rv);
            return prefix+m.substring(prefix.length);
          }catch(e){}
          return m;
        });
      }
    }
    return b;
  }
  XMLHttpRequest.prototype.open=function(m,u){
    this._isLogin=(typeof u==='string'&&u.indexOf('access/ticket')>=0);
    this._myUrl=u;
    return _origOpen.call(this,m,_fixUrl(u));};
  XMLHttpRequest.prototype.send=function(b){
    if(this._isLogin){
      var self=this;
      var bodyStr=typeof b==='string'?b:'';
      bodyStr=_fixLoginBody(bodyStr);
      var px=new XMLHttpRequest();
      _origOpen.call(px,'POST',_fixUrl(this._myUrl),true);
      px.setRequestHeader('Content-Type','application/x-www-form-urlencoded');
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
      _origSend.call(px,bodyStr||b);
      return;
    }
    return _origSend.apply(this,arguments);};
  var _origFetch=window.fetch;
  window.fetch=function(u,o){return _origFetch.call(this,_fixUrl(u),o);};
  function _patchAjax(){
    if(typeof Ext==='undefined'||!Ext.Ajax||!Ext.Ajax.request)return;
    var origReq=Ext.Ajax.request;
    Ext.Ajax.request=function(opts){
      var url=(opts&&opts.url)?opts.url:'';
      if(typeof url==='string'&&url.indexOf('access/ticket')>=0){
        var body='';
        if(opts.params){body=Object.keys(opts.params).map(function(k){return encodeURIComponent(k)+'='+encodeURIComponent(opts.params[k]);}).join('&');}
        body=_fixLoginBody(body);
        var pxUrl=_fixUrl(url);
        var px=new XMLHttpRequest();
        _origOpen.call(px,'POST',pxUrl,true);
        px.setRequestHeader('Content-Type','application/x-www-form-urlencoded');
        px.onload=function(){
          var r={responseText:px.responseText,status:px.status,statusText:px.statusText,getResponseHeader:function(h){return h==='content-type'?'application/json':null;}};
          if(px.status>=200&&px.status<300){if(opts.success)opts.success.call(opts.scope||this,r,opts);}
          else{if(opts.failure)opts.failure.call(opts.scope||this,r,opts);}
          if(opts.callback)opts.callback.call(opts.scope||this,px.status>=200&&px.status<300,r,opts);
        };
        px.onerror=function(){if(opts.failure)opts.failure.call(opts.scope||this,{status:0},opts);if(opts.callback)opts.callback.call(opts.scope||this,false,{status:0},opts);};
        _origSend.call(px,body);
        return{xhr:px};
      }
      return origReq.apply(Ext.Ajax,arguments);
    };
  }
  function _waitExt(){if(typeof Ext!=='undefined'&&Ext.Ajax&&Ext.Ajax.request){_patchAjax();return;}setTimeout(_waitExt,300);}
  _waitExt();
})();
</script>`, proxyPrefix)
		html = strings.Replace(html, "<head>", "<head>"+interceptScript, 1)

		// 替换 ExtJS locale 文件为用户选择的语言
		if sess.Lang != "" && sess.Lang != "en" {
			localeFile := fmt.Sprintf("locale-%s.js", sess.Lang)
			html = strings.Replace(html, "locale-en.js", localeFile, -1)
			html = strings.Replace(html, "locale/en/", fmt.Sprintf("locale/%s/", sess.Lang), -1)
		}

		// 注入自动填充脚本
		autofillScript := services.AutofillScript(sess.User, sess.Pass)
		html = strings.Replace(html, "</head>", autofillScript+"</head>", 1)

		c.Header("Content-Type", "text/html; charset=utf-8")
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.SetCookie("px_tk", tk, 3600, "/", "", false, false)
		// 设置 PVELang cookie，让 SPA 加载对应语言的 locale 文件
		if sess.Lang != "" {
			c.SetCookie("PVELang", sess.Lang, 86400*365, "/", "", false, false)
		}
		// 注入 PVEAuthCookie 到浏览器，让 SPA ExtJS 正常初始化
		pveCookie, csrf := services.GetPVEAuthCookie(tk)
		if pveCookie != "" {
			// pveCookie 格式: "PVEAuthCookie=xxx;..."
			parts := strings.SplitN(pveCookie, "=", 2)
			if len(parts) == 2 {
				c.SetCookie("PVEAuthCookie", parts[1], 3600, "/", "", false, true)
			}
		}
		if csrf != "" {
			c.SetCookie("CSRFPreventionToken", csrf, 3600, "/", "", false, true)
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
		return
	}

	// 非 HTML：直接透传
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50<<20))

	// 登录响应：提取并存储 PVE 认证信息
	if isLoginReq {
		services.StorePVEAuth(tk, body, resp.Header)
		// 登录成功后注入 PVEAuthCookie 到浏览器，让 SPA 正常工作
		pveCookie, csrf := services.GetPVEAuthCookie(tk)
		if pveCookie != "" {
			parts := strings.SplitN(pveCookie, "=", 2)
			if len(parts) == 2 {
				c.SetCookie("PVEAuthCookie", parts[1], 3600, "/", "", false, true)
			}
		}
		if csrf != "" {
			c.SetCookie("CSRFPreventionToken", csrf, 3600, "/", "", false, true)
		}
	}

	// 清除可能干扰的头
	c.Writer.Header().Del("Transfer-Encoding")
	c.Writer.Header().Del("Content-Encoding")
	c.Writer.Header().Del("Content-Length")
	// 不转发 PVE 的 Set-Cookie 到浏览器（认证信息由服务器端管理）
	for key, values := range resp.Header {
		for _, v := range values {
			if !strings.EqualFold(key, "Transfer-Encoding") &&
				!strings.EqualFold(key, "Content-Encoding") &&
				!strings.EqualFold(key, "Content-Length") &&
				!strings.EqualFold(key, "Set-Cookie") {
				c.Writer.Header().Add(key, v)
			}
		}
	}
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(body)
}

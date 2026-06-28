package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	defaultPort       = "18788"
	defaultConfigPath = "data/config.json"
	adminCookieName   = "nvw_admin"
)

type App struct {
	store      *Store
	dispatcher *Dispatcher
	stats      *Stats
}

func main() {
	store, err := NewStore(env("CONFIG_PATH", defaultConfigPath))
	if err != nil {
		log.Fatal(err)
	}
	cfg := store.Get()
	stats := &Stats{StartedAt: time.Now()}
	dispatcher := NewDispatcher(store, stats, cfg.Workers, cfg.QueueSize)
	app := &App{store: store, dispatcher: dispatcher, stats: stats}

	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleHome)
	mux.HandleFunc("/admin", app.handleAdmin)
	mux.HandleFunc("/healthz", app.handleHealth)
	mux.HandleFunc("/api/admin/login", app.handleAdminLogin)
	mux.HandleFunc("/api/config", app.handleConfig)
	mux.HandleFunc("/api/status", app.handleStatus)
	mux.HandleFunc("/api/catalog", app.handleCatalog)
	mux.HandleFunc("/api/parse", app.handleParse)
	mux.HandleFunc("/v1/models", app.handleModels)
	mux.HandleFunc("/v1/chat/completions", app.handleChatCompletions)

	port := env("PORT", defaultPort)
	log.Printf("newapi-video-wrapper listening on http://127.0.0.1:%s", port)
	log.Printf("admin: http://127.0.0.1:%s/admin", port)
	log.Fatal(http.ListenAndServe(":"+port, logRequest(mux)))
}

func (a *App) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
}

func (a *App) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		_ = loginTpl.Execute(w, nil)
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = adminTpl.Execute(w, map[string]any{"Config": a.store.Get()})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{"ok": true, "status": "up"})
}

func (a *App) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct{ Password string `json:"password"` }
	_ = json.NewDecoder(r.Body).Decode(&body)
	if !safeEqual(body.Password, a.store.Get().AdminPassword) {
		jsonError(w, http.StatusUnauthorized, "admin password is wrong")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    a.adminToken(),
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !a.isAdmin(r) {
		jsonError(w, http.StatusUnauthorized, "admin auth required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, a.store.Get())
	case http.MethodPost:
		var cfg Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.store.Save(cfg); err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonOK(w, a.store.Get())
	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, a.statusSnapshot())
}

func (a *App) handleCatalog(w http.ResponseWriter, r *http.Request) {
	cfg := a.store.Get()
	jsonOK(w, map[string]any{"siteName": cfg.SiteName, "apis": publicAPIs(cfg)})
}

func (a *App) isWrapperAuthed(r *http.Request) bool {
	secret := a.store.Get().WrapperSecret
	if secret == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return safeEqual(strings.TrimSpace(auth[7:]), secret)
	}
	return false
}

func (a *App) isAdmin(r *http.Request) bool {
	c, err := r.Cookie(adminCookieName)
	return err == nil && safeEqual(c.Value, a.adminToken())
}

func (a *App) adminToken() string {
	return hmacHex(a.store.Get().AdminPassword, "newapi-video-wrapper-admin")
}

func (a *App) statusSnapshot() map[string]any {
	total := atomic.LoadUint64(&a.stats.TotalRequests)
	lat := atomic.LoadUint64(&a.stats.TotalLatencyMs)
	var avg uint64
	if total > 0 {
		avg = lat / total
	}
	cfg := a.store.Get()
	return map[string]any{
		"startedAt":        a.stats.StartedAt,
		"uptimeSeconds":    int64(time.Since(a.stats.StartedAt).Seconds()),
		"workers":          cfg.Workers,
		"queueCapacity":    a.dispatcher.QueueCap(),
		"queueLength":      a.dispatcher.QueueLen(),
		"inFlight":         atomic.LoadInt64(&a.stats.InFlight),
		"totalRequests":    total,
		"successRequests":  atomic.LoadUint64(&a.stats.SuccessRequests),
		"failedRequests":   atomic.LoadUint64(&a.stats.FailedRequests),
		"upstreamRequests": atomic.LoadUint64(&a.stats.UpstreamRequests),
		"queueRejected":    atomic.LoadUint64(&a.stats.QueueRejected),
		"avgLatencyMs":     avg,
		"lastError":        a.stats.LastError,
	}
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

var loginTpl = template.Must(template.New("login").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Wrapper 后台登录</title><style>
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:#101418;color:#e8edf2;font-family:Arial,"Microsoft YaHei",sans-serif}.card{width:min(420px,calc(100% - 32px));padding:28px;border:1px solid #26313d;border-radius:8px;background:#151b22}h1{margin:0 0 8px;font-size:24px}p{color:#9aa6b2;line-height:1.6}input,button{width:100%;height:44px;border-radius:6px;font:inherit}input{border:1px solid #2e3a47;background:#0f141a;color:#fff;padding:0 12px}button{margin-top:12px;border:0;background:#2f81f7;color:white;font-weight:700;cursor:pointer}
</style></head><body><form class="card" id="login"><h1>Wrapper 后台</h1><p>输入管理员密码后配置上游 key、worker 并发和平台接口。</p><input id="password" type="password" placeholder="管理员密码" autofocus><button type="submit">进入后台</button></form>
<script>document.querySelector('#login').addEventListener('submit',async e=>{e.preventDefault();const res=await fetch('/api/admin/login',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify({password:document.querySelector('#password').value})});if(res.ok)location.reload();else alert('密码错误')})</script></body></html>`))

var adminTpl = template.Must(template.New("admin").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Config.SiteName}}</title><style>
*{box-sizing:border-box}body{margin:0;background:#f6f8fa;color:#1f2328;font-family:Arial,"Microsoft YaHei",sans-serif}.top{height:56px;display:flex;align-items:center;justify-content:space-between;padding:0 20px;background:#111827;color:white}.wrap{max-width:1280px;margin:0 auto;padding:18px}.grid{display:grid;grid-template-columns:360px 1fr;gap:16px}.panel{background:white;border:1px solid #d8dee4;border-radius:8px;padding:16px}h2{font-size:18px;margin:0 0 12px}.stats{display:grid;grid-template-columns:repeat(4,1fr);gap:10px}.stat{border:1px solid #d8dee4;border-radius:6px;padding:10px;background:#f6f8fa}.stat b{display:block;font-size:22px}label{display:block;font-size:13px;font-weight:700;margin:10px 0 5px}input{width:100%;border:1px solid #d0d7de;border-radius:6px;padding:8px;font:inherit}button{border:0;border-radius:6px;background:#2f81f7;color:white;padding:9px 13px;font-weight:700;cursor:pointer}.api{display:grid;grid-template-columns:92px 1fr 1fr 120px;gap:8px;align-items:center;border-top:1px solid #eaeef2;padding:10px 0}.muted{color:#667085;font-size:12px}@media(max-width:900px){.grid{grid-template-columns:1fr}.stats{grid-template-columns:repeat(2,1fr)}.api{grid-template-columns:1fr}}
</style></head><body><div class="top"><strong>{{.Config.SiteName}}</strong><span>NewAPI Base URL: <code>/v1</code></span></div><div class="wrap">
<div class="panel"><h2>系统状态</h2><div class="stats" id="stats"></div></div>
<div class="grid" style="margin-top:16px"><div class="panel"><h2>基础配置</h2><form id="configForm">
<label>站点名</label><input name="siteName"><label>后台密码</label><input name="adminPassword" type="password"><label>Wrapper Secret（填到 newapi 渠道密钥）</label><input name="wrapperSecret"><label>上游全局 apikey</label><input name="globalApiKey"><label>请求超时 ms</label><input name="requestTimeoutMs" type="number"><label>Worker 数（修改后重启生效）</label><input name="workers" type="number"><label>队列长度（修改后重启生效）</label><input name="queueSize" type="number"><label>失败重试次数</label><input name="retryTimes" type="number"><button style="margin-top:14px">保存配置</button></form><p class="muted">newapi 配置：Base URL 填 http://服务器IP:8788/v1，模型填 video-parse，密钥填上面的 Wrapper Secret。</p></div>
<div class="panel"><h2>平台接口</h2><div id="apis"></div></div></div></div>
<script>
let cfg=null;
async function loadConfig(){cfg=await (await fetch('/api/config')).json();for(const [k,v] of Object.entries(cfg)){const el=document.querySelector('[name="'+k+'"]');if(el&&typeof v!=='object')el.value=v??''}renderApis()}
function renderApis(){const box=document.querySelector('#apis');box.innerHTML='';cfg.apis.forEach((api,i)=>{const row=document.createElement('div');row.className='api';row.innerHTML='<label><input type="checkbox" data-i="'+i+'" data-k="enabled" '+(api.enabled?'checked':'')+'> '+api.id+'</label><div><input data-i="'+i+'" data-k="name" value="'+esc(api.name||'')+'"><div class="muted">'+esc(api.group||'')+'</div></div><div><input data-i="'+i+'" data-k="endpointUrl" value="'+esc(api.endpointUrl||'')+'"><div class="muted">模型：video-parse-'+api.id+'</div></div><div><input data-i="'+i+'" data-k="apiKey" value="'+esc(api.apiKey||'')+'" placeholder="单独key"></div>';box.appendChild(row)});box.querySelectorAll('input').forEach(el=>el.addEventListener('change',()=>{const i=Number(el.dataset.i),k=el.dataset.k;cfg.apis[i][k]=el.type==='checkbox'?el.checked:el.value}))}
document.querySelector('#configForm').addEventListener('submit',async e=>{e.preventDefault();new FormData(e.target).forEach((v,k)=>{cfg[k]=['requestTimeoutMs','workers','queueSize','retryTimes'].includes(k)?Number(v):String(v)});const res=await fetch('/api/config',{method:'POST',headers:{'content-type':'application/json'},body:JSON.stringify(cfg)});if(!res.ok)return alert(await res.text());cfg=await res.json();alert('已保存。worker 数和队列长度需要重启服务后完全生效。');renderApis()});
async function refreshStatus(){const s=await (await fetch('/api/status')).json();document.querySelector('#stats').innerHTML=[['队列',s.queueLength+'/'+s.queueCapacity],['处理中',s.inFlight],['总请求',s.totalRequests],['成功',s.successRequests],['失败',s.failedRequests],['上游请求',s.upstreamRequests],['平均耗时',s.avgLatencyMs+'ms'],['运行',s.uptimeSeconds+'s']].map(x=>'<div class="stat"><span>'+x[0]+'</span><b>'+x[1]+'</b></div>').join('')}
function esc(s){return String(s).replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]))}
loadConfig();refreshStatus();setInterval(refreshStatus,3000);
</script></body></html>`))

package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
)

var pageTpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="{{.Lang}}"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LE-EZVIZ-UI — {{.Title}}</title>
<style>
  :root { color-scheme: dark; }
  body { font-family: -apple-system, system-ui, sans-serif; background:#0c0f14; color:#e8eaed; margin:0; padding:0; }
  .card { max-width:720px; margin:0 auto; padding:8px 12px 24px; }
  .tabs { display:flex; align-items:center; gap:2px; border-bottom:1px solid #2a3038; margin:0 0 12px; }
  .tabs a { color:#9aa4b2; text-decoration:none; padding:10px 9px 8px; font-size:14px; border-bottom:2px solid transparent; white-space:nowrap; }
  .tabs a.active { color:#e8eaed; border-bottom-color:#3b82f6; }
  .tabs a.logout { margin-left:auto; color:#f87171; }
  .langbar { display:flex; align-items:center; gap:8px; margin:0 0 16px; }
  .langbar label { margin:0; }
  .langbar select { width:auto; min-width:160px; margin:0; }
  .rbtn { margin-top:0; background:#2a3038; }
  .rbtn.on { background:#3b82f6; }
  h1 { font-size:18px; margin:0 0 12px; }
  label { display:block; font-size:13px; color:#9aa4b2; margin:12px 0 4px; }
  input, select { width:100%; box-sizing:border-box; background:#0c0f14; border:1px solid #2a3038; border-radius:8px; color:#e8eaed; padding:10px; font-size:15px; }
  button, .btn { display:inline-block; margin-top:16px; background:#3b82f6; color:#fff; border:0; border-radius:8px; padding:10px 18px; font-size:15px; cursor:pointer; text-decoration:none; }
  .btn.gray { background:#2a3038; }
  .err { color:#f87171; font-size:13px; margin-top:10px; }
  .ok { color:#34d399; font-size:13px; margin-top:10px; }
  .muted { color:#9aa4b2; font-size:13px; }
  .statusline { color:#9aa4b2; font-size:13px; font-weight:400; text-align:center; margin:10px 0 0; min-height:1.2em; }
  .toolbar { display:grid; grid-template-columns:1fr auto 1fr; align-items:center; gap:8px; margin:10px 0 0; min-height:28px; }
  .toolbar .statusline { grid-column:2; margin:0; }
  .toolbar .actions { grid-column:3; justify-self:end; display:flex; gap:4px; }
  button.textbtn { -webkit-appearance:none; appearance:none; display:inline; margin:0; padding:0 6px; background:transparent; border:0; border-radius:0; color:#9aa4b2; font-family:inherit; font-size:13px; font-weight:400; line-height:1.2; cursor:pointer; white-space:nowrap; }
  button.textbtn:hover { color:#e8eaed; }
  button.textbtn.copied { color:#34d399; }
  .player { position:relative; width:100%; aspect-ratio:16/9; border-radius:10px; background:#000; overflow:hidden; margin-top:8px; }
  .player video { position:absolute; inset:0; width:100%; height:100%; object-fit:contain; background:#000; }
  .player video::-webkit-media-controls-volume-slider { display:none; }
  code { background:#0c0f14; padding:2px 6px; border-radius:6px; font-size:12px; word-break:break-all; }
  .row { display:flex; gap:10px; align-items:center; flex-wrap:wrap; }
  .updaterow { display:flex; align-items:center; gap:12px; margin:0; }
  .updaterow .muted { margin:0; flex:1; min-width:0; }
  .updaterow form { margin:0; margin-left:auto; }
  .updaterow button { margin:0; white-space:nowrap; }
  .updaterow button:disabled { opacity:0.7; cursor:wait; }
  .updatebox {
    display:flex; flex-direction:column; gap:8px;
    margin:0 0 8px; padding:12px 14px;
    background:#12161c; border:1px solid #2a3038; border-radius:10px;
  }
  .updatebox-top { display:flex; align-items:center; gap:10px; min-height:36px; }
  .updatebox-top code {
    flex-shrink:0; background:#0c0f14; border:1px solid #2a3038;
    padding:2px 8px; border-radius:6px; font-size:13px; font-weight:500;
    color:#e8eaed; line-height:1.3;
  }
  .updatebox #updStatus {
    flex:1; min-width:0; margin:0;
    font-size:12px; font-weight:500; color:#9aa4b2; line-height:1.25;
  }
  .updatebox #updStatus:empty { display:none; }
  .updatebox #updStatus.ok { color:#34d399; }
  .updatebox #updStatus.err { color:#f87171; }
  .updatebox #updBtn {
    margin:0 0 0 auto; flex-shrink:0; width:36px; height:36px; padding:0;
    display:inline-flex; align-items:center; justify-content:center;
    background:#2a3038; color:#e8eaed; border-radius:8px;
  }
  .updatebox #updBtn svg { width:18px; height:18px; display:block; }
  .updatebox #updBtn:hover { background:#343b46; }
  .updatebox #updBtn:disabled { opacity:0.75; cursor:wait; background:#2a3038; }
  .updatebox #updBtn.busy { background:#1e3a5f; color:#93c5fd; }
  .updatebox #updBtn.busy svg { animation: updspin 0.9s linear infinite; }
  .updatebox .updprog {
    display:none; height:2px; border-radius:1px; overflow:hidden; background:#2a3038;
  }
  .updatebox.working .updprog { display:block; }
  .updatebox .updprog > i {
    display:block; height:100%; width:40%; border-radius:1px; background:#3b82f6;
    animation: updslide 1.1s ease-in-out infinite;
  }
  @keyframes updslide {
    0% { transform: translateX(-120%); }
    100% { transform: translateX(280%); }
  }
  @keyframes updspin { to { transform: rotate(360deg); } }
  .seg { display:flex; width:100%; max-width:100%; padding:3px; gap:2px; background:#12161c; border:1px solid #2a3038; border-radius:10px; box-sizing:border-box; }
  .seg form, .seg > button { margin:0; flex:1 1 0; min-width:0; }
  .seg button { margin:0; width:100%; border:0; border-radius:8px; background:transparent; color:#9aa4b2; font:inherit; font-size:clamp(12px, 3.4vw, 14px); font-weight:500; padding:8px 6px; cursor:pointer; white-space:normal; line-height:1.25; text-align:center; overflow-wrap:break-word; }
  .seg button.on { background:#3b82f6; color:#fff; }
  .seg button.on:disabled { opacity:1; cursor:default; }
  .seg + p { margin-top: 10px; }
  .anchor { scroll-margin-top: 8px; }
  details.fold { margin-top: 26px; }
  details.fold > summary {
    display: flex; align-items: center; gap: 10px;
    font-size: 18px; font-weight: 600; line-height: 1.2;
    color: #e8eaed; cursor: pointer; user-select: none; -webkit-user-select: none;
    list-style: none; margin: 0 0 8px;
  }
  details.fold > summary::-webkit-details-marker,
  details.fold > summary::marker { display: none; content: none; }
  details.fold > summary::before {
    content: ""; width: 0.42em; height: 0.42em; flex-shrink: 0;
    border-right: 2px solid #9aa4b2; border-bottom: 2px solid #9aa4b2;
    transform: rotate(-45deg); margin-top: -1px;
  }
  details.fold[open] > summary::before { transform: rotate(45deg); margin-top: -4px; }
  #bat:not(:empty) { margin-right: 8px; }
  .stats-head { display:flex; align-items:baseline; gap:10px; margin:0 0 8px; }
  .stats-cur { font-size:20px; font-weight:600; }
  .stats-head .muted { margin:0; }
  pre.log { background:#0c0f14; border:1px solid #2a3038; border-radius:8px; padding:12px; font-size:11px; line-height:1.45; overflow:auto; max-height:70vh; white-space:pre-wrap; word-break:break-all; margin:12px 0 0; }
</style></head><body><div class="card">{{.Body}}</div></body></html>`))

func render(w http.ResponseWriter, r *http.Request, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	pageTpl.Execute(w, map[string]any{"Title": title, "Body": template.HTML(body), "Lang": langOf(r)})
}

func isLivePath(p string) bool {
	return p == *basePath || p == *basePath+"/"
}

func tabs(r *http.Request, active string) string {
	lang := langOf(r)
	item := func(name, href, key string) string {
		cls := ""
		if name == active {
			cls = ` class="active"`
		}
		return `<a href="` + *basePath + href + `"` + cls + `>` + template.HTMLEscapeString(T(lang, key)) + `</a>`
	}
	return `<div class="tabs">` +
		item("live", "/", "nav.live") +
		item("recordings", "/recordings", "nav.recordings") +
		item("stats", "/stats", "nav.stats") +
		item("logs", "/logs", "nav.logs") +
		item("setup", "/setup", "nav.setup") +
		`<a class="logout" href="` + *basePath + `/logout">` + template.HTMLEscapeString(T(lang, "nav.logout")) + `</a></div>`
}

func regionOptions(current string) string {
	regions := []string{"Russia", "Europe", "Africa", "India", "Oceania", "NorthAmerica", "SouthAmerica"}
	var b strings.Builder
	for _, r := range regions {
		sel := ""
		if r == current {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s"%s>%s</option>`, r, sel, r)
	}
	return b.String()
}

func handleInit(w http.ResponseWriter, r *http.Request) {
	if cfgCopy().PasswordHash != "" {
		http.Redirect(w, r, *basePath+"/login", http.StatusSeeOther)
		return
	}
	lang := langOf(r)
	esc := template.HTMLEscapeString
	form := `<form method="post"><label>` + esc(T(lang, "init.password")) + `</label><input type="password" name="password" minlength="8" required autofocus><button>` + esc(T(lang, "init.save")) + `</button></form>`
	if r.Method == http.MethodPost {
		pw := r.FormValue("password")
		if len(pw) < 8 {
			render(w, r, T(lang, "init.title"), langBar(r)+`<h1>`+esc(T(lang, "init.title"))+`</h1><p class="err">`+esc(T(lang, "init.min8"))+`</p>`+form)
			return
		}
		err := updateCfg(func(c *Config) error {
			hash, err := bcryptHash(pw)
			if err != nil {
				return err
			}
			c.PasswordHash = hash
			c.SessionSecret = randomHex(32)
			return nil
		})
		if err != nil {
			render(w, r, T(lang, "error"), `<p class="err">`+esc(err.Error())+`</p>`)
			return
		}
		setSession(w)
		http.Redirect(w, r, *basePath+"/setup", http.StatusSeeOther)
		return
	}
	render(w, r, T(lang, "init.title"), langBar(r)+`<h1>`+esc(T(lang, "init.title"))+`</h1><p class="muted">`+esc(T(lang, "init.hint"))+`</p>`+form)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	if cfgCopy().PasswordHash == "" {
		http.Redirect(w, r, *basePath+"/init", http.StatusSeeOther)
		return
	}
	lang := langOf(r)
	esc := template.HTMLEscapeString
	form := `<form method="post"><label>` + esc(T(lang, "login.password")) + `</label><input type="password" name="password" required autofocus><button>` + esc(T(lang, "login.submit")) + `</button></form>`
	if r.Method == http.MethodPost {
		ip := clientIP(r)
		if !loginAllowed(ip) {
			render(w, r, T(lang, "login.title"), langBar(r)+`<h1>`+esc(T(lang, "login.title"))+`</h1><p class="err">`+esc(T(lang, "login.ratelimit"))+`</p>`)
			return
		}
		if passwordOK(r.FormValue("password")) {
			setSession(w)
			http.Redirect(w, r, *basePath+"/", http.StatusSeeOther)
			return
		}
		loginFailed(ip)
		render(w, r, T(lang, "login.title"), langBar(r)+`<h1>`+esc(T(lang, "login.title"))+`</h1><p class="err">`+esc(T(lang, "login.wrong"))+`</p>`+form)
		return
	}
	render(w, r, T(lang, "login.title"), langBar(r)+`<h1>`+esc(T(lang, "login.title"))+`</h1>`+form)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSession(w)
	http.Redirect(w, r, *basePath+"/login", http.StatusSeeOther)
}

func setupFold(id, title, inner string, open bool) string {
	attr := ""
	if open {
		attr = " open"
	}
	return `<details class="fold anchor" id="` + id + `"` + attr + `><summary>` + title + `</summary>` + inner + `</details>`
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	esc := template.HTMLEscapeString
	ezvizMsg, streamMsg, pwMsg := "", "", ""
	if r.Method == http.MethodPost {
		switch r.FormValue("form") {
		case "sitepw":
			pwMsg = changeSitePassword(r, lang)
		case "stream":
			streamMsg = changeStreamMode(r, lang)
		case "update":
			_ = runUpdateCheck(lang)
		default:
			logOn := bridgeLogEnabled()
			err := updateCfg(func(c *Config) error {
				c.Email = strings.TrimSpace(r.FormValue("email"))
				c.Password = r.FormValue("password")
				c.Region = r.FormValue("region")
				if c.Region == "" {
					c.Region = "Russia"
				}
				if activeSerial(*c) == "" {
					if s := pickDefaultSerial(c.Email, c.Password, c.Region, logOn); s != "" {
						c.ActiveSerial = s
						c.Serial = s
					}
				}
				return nil
			})
			if err != nil {
				ezvizMsg = `<p class="err">` + esc(err.Error()) + `</p>`
			} else {
				streamer.Kick()
				ezvizMsg = `<p class="ok">` + esc(T(lang, "setup.saved")) + `</p>`
			}
		}
	}
	c := cfgCopy()
	ver := installedVersion()
	refreshDevStatusOnVisit()
	updJS, _ := json.Marshal(map[string]string{
		"updating":    T(lang, "setup.updating"),
		"downloading": T(lang, "setup.downloading"),
		"installing":  T(lang, "setup.installing"),
		"upToDate":    T(lang, "setup.upToDate"),
		"updated":     T(lang, "setup.updated"),
		"fail":        T(lang, "setup.updateFail"),
		"restarting":  T(lang, "setup.restarting"),
	})
	render(w, r, T(lang, "setup.title"), tabs(r, "setup")+langBar(r)+`
<div class="anchor" id="update">
<h1>`+esc(T(lang, "setup.update"))+`</h1>
<div class="updatebox" id="updatebox">
<div class="updatebox-top">
<code id="verLabel">`+esc(ver)+`</code>
<span id="updStatus"></span>
<button type="button" id="updBtn" title="`+esc(T(lang, "setup.updateCheck"))+`" aria-label="`+esc(T(lang, "setup.updateCheck"))+`"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36"/><polyline points="21 3 21 9 15 9"/></svg></button>
</div>
<div class="updprog" aria-hidden="true"><i></i></div>
</div>
</div>
<div class="anchor" id="stream-mode">
<h1 style="margin-top:26px">`+esc(T(lang, "setup.stream"))+`</h1>
`+streamModeSeg(lang, c.StreamMode)+streamMsg+`
<p class="muted">`+esc(T(lang, "setup.streamHint"))+`</p>
</div>
`+setupFold("ezviz", esc(T(lang, "setup.ezviz")), `<form method="post" autocomplete="off">
<input type="hidden" name="form" value="ezviz">
<label>`+esc(T(lang, "setup.email"))+`</label><input name="email" type="email" value="`+esc(c.Email)+`" required autocomplete="off">
<label>`+esc(T(lang, "setup.ezvizpw"))+`</label><input name="password" type="password" value="`+esc(c.Password)+`" required autocomplete="new-password">
<label>`+esc(T(lang, "setup.region"))+`</label><select name="region" autocomplete="off">`+regionOptions(c.Region)+`</select>
<p class="muted">`+esc(T(lang, "setup.devicesHint"))+`</p>
<button>`+esc(T(lang, "setup.save"))+`</button>
</form>`+ezvizMsg, ezvizMsg != "" || strings.TrimSpace(c.Email) == "")+`
`+setupFold("sitepw", esc(T(lang, "setup.sitepw")), `<form method="post" autocomplete="off">
<input type="hidden" name="form" value="sitepw">
<label>`+esc(T(lang, "setup.current"))+`</label><input name="current" type="password" required autocomplete="new-password">
<label>`+esc(T(lang, "setup.new"))+`</label><input name="new" type="password" minlength="8" required autocomplete="new-password">
<label>`+esc(T(lang, "setup.repeat"))+`</label><input name="repeat" type="password" minlength="8" required autocomplete="new-password">
<button>`+esc(T(lang, "setup.change"))+`</button>
</form>`+pwMsg, pwMsg != "")+`
<script>
document.querySelectorAll("details.fold").forEach((d) => {
  const sync = () => {
    d.querySelectorAll("input:not([type=hidden]), select, button").forEach((el) => { el.disabled = !d.open; });
  };
  sync();
  d.addEventListener("toggle", sync);
});
const base = "`+*basePath+`";
const ut = `+string(updJS)+`;
const box = document.getElementById("updatebox");
const btn = document.getElementById("updBtn");
const verLabel = document.getElementById("verLabel");
const statusEl = document.getElementById("updStatus");
function setStatus(text, cls) {
  statusEl.className = cls || "";
  statusEl.textContent = text || "";
}
function setBusy(on) {
  box.classList.toggle("working", !!on);
  btn.disabled = !!on;
  btn.classList.toggle("busy", !!on);
}
function reloadSoon() {
  setTimeout(() => { location.reload(); }, 800);
}
function norm(v) { return String(v || "").replace(/^v/i, ""); }
async function pollUpdate(expect, was) {
  const deadline = Date.now() + 180000;
  let sawDown = false;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 1500));
    try {
      const s = await (await fetch(base + "/api/version", {cache:"no-store"})).json();
      if (s.version) verLabel.textContent = s.version;
      if (s.phase === "error") {
        setBusy(false);
        setStatus(ut.fail + (s.error ? (" " + s.error) : ""), "err");
        return { ok: false, version: s.version || was };
      }
      if (s.phase === "download") {
        setBusy(true);
        setStatus(ut.downloading);
      } else if (s.phase === "install") {
        setBusy(true);
        setStatus(ut.installing);
      }
      if (expect && s.version && norm(s.version) === norm(expect)) {
        setStatus(ut.updated, "ok");
        return { ok: true, version: s.version };
      }
      if (sawDown && s.version && norm(s.version) !== norm(was)) {
        setStatus(ut.updated, "ok");
        return { ok: true, version: s.version };
      }
    } catch (e) {
      sawDown = true;
      setBusy(true);
      setStatus(ut.restarting);
    }
  }
  try {
    const s = await (await fetch(base + "/api/version", {cache:"no-store"})).json();
    if (s.version) verLabel.textContent = s.version;
    if (expect && norm(s.version) === norm(expect)) {
      setStatus(ut.updated, "ok");
      return { ok: true, version: s.version };
    }
    if (norm(s.version) !== norm(was)) {
      setStatus(ut.updated, "ok");
      return { ok: true, version: s.version };
    }
  } catch (_) {}
  setBusy(false);
  setStatus(ut.fail, "err");
  return { ok: false, version: was };
}
btn.addEventListener("click", async () => {
  setBusy(true);
  setStatus(ut.updating);
  const was = verLabel.textContent;
  try {
    const r = await fetch(base + "/api/update", {method:"POST", headers:{"Accept":"application/json"}});
    const j = await r.json();
    if (j.status === "up_to_date") {
      if (j.version) verLabel.textContent = j.version;
      setBusy(false);
      setStatus(ut.upToDate, "ok");
      return;
    }
    if (j.status === "error") {
      setBusy(false);
      setStatus(ut.fail + (j.error ? (" " + j.error) : ""), "err");
      return;
    }
    setBusy(true);
    setStatus(ut.downloading);
    const res = await pollUpdate(j.latest || "", was);
    if (res.version) verLabel.textContent = res.version;
    if (res.ok) { setBusy(false); reloadSoon(); return; }
  } catch (e) {
    setBusy(true);
    setStatus(ut.restarting);
    const res = await pollUpdate("", was);
    if (res.version) verLabel.textContent = res.version;
    if (res.ok) { setBusy(false); reloadSoon(); return; }
  }
  setBusy(false);
});
</script>`)
}

func changeSitePassword(r *http.Request, lang string) string {
	esc := template.HTMLEscapeString
	cur, nw, repeat := r.FormValue("current"), r.FormValue("new"), r.FormValue("repeat")
	if !passwordOK(cur) {
		return `<p class="err">` + esc(T(lang, "setup.pwBad")) + `</p>`
	}
	if len(nw) < 8 {
		return `<p class="err">` + esc(T(lang, "setup.pwShort")) + `</p>`
	}
	if nw != repeat {
		return `<p class="err">` + esc(T(lang, "setup.pwMismatch")) + `</p>`
	}
	hash, err := bcryptHash(nw)
	if err != nil {
		return `<p class="err">` + esc(err.Error()) + `</p>`
	}
	if err := updateCfg(func(c *Config) error {
		c.PasswordHash = hash
		return nil
	}); err != nil {
		return `<p class="err">` + esc(err.Error()) + `</p>`
	}
	return `<p class="ok">` + esc(T(lang, "setup.pwOk")) + `</p>`
}

func segForm(action, formName, field, value, label string, on bool) string {
	cls, dis := "", ""
	if on {
		cls = ` class="on"`
		dis = " disabled"
	}
	return `<form method="post" action="` + action + `"><input type="hidden" name="form" value="` + formName + `"><input type="hidden" name="` + field + `" value="` + value + `"><button type="submit"` + cls + dis + `>` + label + `</button></form>`
}

func streamModeSeg(lang, current string) string {
	if current != "always" {
		current = "on_demand"
	}
	esc := template.HTMLEscapeString
	return `<div class="seg">` +
		segForm("#stream-mode", "stream", "mode", "on_demand", esc(T(lang, "setup.streamOnDemand")), current == "on_demand") +
		segForm("#stream-mode", "stream", "mode", "always", esc(T(lang, "setup.streamAlways")), current == "always") +
		`</div>`
}

func changeStreamMode(r *http.Request, lang string) string {
	esc := template.HTMLEscapeString
	mode := r.FormValue("mode")
	if mode != "always" && mode != "on_demand" {
		return `<p class="err">` + esc(T(lang, "error")) + `</p>`
	}
	prev := streamMode()
	if err := updateCfg(func(c *Config) error {
		c.StreamMode = mode
		return nil
	}); err != nil {
		return `<p class="err">` + esc(err.Error()) + `</p>`
	}
	if mode == "always" {
		log.Printf("streamer: always-on, keeping the camera awake")
		streamer.maybeStart()
		return `<p class="ok">` + esc(T(lang, "setup.streamSavedAlways")) + `</p>`
	}
	if prev != "on_demand" {
		log.Printf("streamer: on-demand")
	}
	return `<p class="ok">` + esc(T(lang, "setup.streamSavedOnDemand")) + `</p>`
}

func handlePlayer(w http.ResponseWriter, r *http.Request) {
	if !isLivePath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	streamer.Touch()
	lang := langOf(r)
	jsT, _ := json.Marshal(map[string]string{
		"saving":        T(lang, "live.saving"),
		"downloaded":    T(lang, "live.downloaded"),
		"savedServer":   T(lang, "live.savedServer"),
		"saveFail":      T(lang, "live.saveFail"),
		"saveErr":       T(lang, "live.saveErr"),
		"switchFail":    T(lang, "live.switchFail"),
		"waiting":       T(lang, "live.waiting"),
		"reconnecting":  T(lang, "live.reconnecting"),
		"noRtc":         T(lang, "live.noRtc"),
		"relogin":       T(lang, "live.relogin"),
		"notConfigured": T(lang, "live.notConfigured"),
		"lastError":     T(lang, "live.lastError"),
		"battery":       T(lang, "live.battery"),
		"online":        T(lang, "live.online"),
		"offline":       T(lang, "live.offline"),
		"upgrade":       T(lang, "live.upgrade"),
		"alwaysOn":      T(lang, "live.alwaysOn"),
	})
	render(w, r, T(lang, "live.title"), tabs(r, "live")+`
<div class="seg" id="devSeg" style="margin-bottom:10px;display:none"></div>
<div class="player">
  <video id="v" controls autoplay muted playsinline webkit-playsinline></video>
</div>
`+liveToolbarHTML(lang)+`
<script src="`+*basePath+`/static/live-player.js"></script>
<script>
const t = `+string(jsT)+`;
`+liveSaveJS()+`
async function loadDevices() {
  const seg = document.getElementById("devSeg");
  if (!seg) return;
  try {
    const j = await (await fetch("`+*basePath+`/api/devices")).json();
    if (!j.devices || j.devices.length <= 1) {
      seg.style.display = "none";
      return;
    }
    seg.style.display = "";
    seg.innerHTML = "";
    j.devices.forEach((d) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.textContent = d.name || d.serial;
      if (d.serial === j.active) btn.classList.add("on");
      btn.onclick = async () => {
        if (d.serial === j.active) return;
        btn.disabled = true;
        try {
          const r = await fetch("`+*basePath+`/api/live/device", {
            method: "POST",
            headers: {"Content-Type": "application/json"},
            body: JSON.stringify({serial: d.serial})
          });
          if (!r.ok) throw new Error(String(r.status));
          if (window.__live && window.__live.notifyDeviceSwitch) window.__live.notifyDeviceSwitch();
          await loadDevices();
        } catch (_) {
          const st = document.getElementById("st");
          if (st) st.textContent = t.switchFail || "Switch failed";
        } finally {
          btn.disabled = false;
        }
      };
      seg.appendChild(btn);
    });
  } catch (_) {
    seg.style.display = "none";
  }
}
loadDevices();
window.__live = new LivePlayer().start({
  base: "`+*basePath+`",
  strings: t
});
</script>`)
}

func liveToolbarHTML(lang string) string {
	esc := template.HTMLEscapeString
	return `<div class="toolbar">
  <p class="statusline"><span id="bat"></span><span id="st"></span></p>
  <div class="actions">
    <button id="save" type="button" class="textbtn">` + esc(T(lang, "live.save")) + `</button>
  </div>
</div>
<p class="statusline" id="mode"></p>`
}

func liveSaveJS() string {
	return `
document.getElementById("save").onclick = async () => {
  const st = document.getElementById("st");
  st.textContent = t.saving;
  try {
    const r = await fetch("` + *basePath + `/save", {method: "POST"});
    const j = await r.json().catch(() => ({}));
    if (!r.ok) {
      st.textContent = t.saveFail + (j.error || "");
      return;
    }
    st.textContent = t.savedServer + (j.name || "");
  } catch (e) {
    st.textContent = t.saveErr;
  }
};
`
}

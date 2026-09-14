package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
  .player { position:relative; width:100%; aspect-ratio:16/9; border-radius:10px; background:#000; overflow:hidden; margin-top:8px; }
  .player img, .player video { position:absolute; inset:0; width:100%; height:100%; object-fit:cover; }
  .player video { z-index:1; background:transparent; }
  code { background:#0c0f14; padding:2px 6px; border-radius:6px; font-size:12px; word-break:break-all; }
  .row { display:flex; gap:10px; align-items:center; flex-wrap:wrap; }
  .seg { display:flex; width:100%; max-width:100%; padding:3px; gap:2px; background:#12161c; border:1px solid #2a3038; border-radius:10px; box-sizing:border-box; }
  .seg form, .seg > button { margin:0; flex:1 1 0; min-width:0; }
  .seg button { margin:0; width:100%; border:0; border-radius:8px; background:transparent; color:#9aa4b2; font:inherit; font-size:clamp(12px, 3.4vw, 14px); font-weight:500; padding:8px 6px; cursor:pointer; white-space:normal; line-height:1.25; text-align:center; overflow-wrap:break-word; }
  .seg button.on { background:#3b82f6; color:#fff; }
  .seg button.on:disabled { opacity:1; cursor:default; }
  .seg + p { margin-top: 10px; }
  .anchor { scroll-margin-top: 8px; }
  #bat:not(:empty) { margin-right: 8px; }
  pre.log { background:#0c0f14; border:1px solid #2a3038; border-radius:8px; padding:12px; font-size:11px; line-height:1.45; overflow:auto; max-height:70vh; white-space:pre-wrap; word-break:break-all; margin:12px 0 0; }
</style></head><body><div class="card">{{.Body}}</div></body></html>`))

func render(w http.ResponseWriter, r *http.Request, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	pageTpl.Execute(w, map[string]any{"Title": title, "Body": template.HTML(body), "Lang": langOf(r)})
}

func publicOrigin(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "https"
		}
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
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
		item("rec", "/recordings", "nav.rec") +
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
			c.DeviceToken = randomHex(24)
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
		default:
			err := updateCfg(func(c *Config) error {
				c.Email = strings.TrimSpace(r.FormValue("email"))
				c.Password = r.FormValue("password")
				c.Serial = strings.TrimSpace(r.FormValue("serial"))
				c.Region = r.FormValue("region")
				if c.Region == "" {
					c.Region = "Russia"
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
	render(w, r, T(lang, "setup.title"), tabs(r, "setup")+langBar(r)+`
<form method="post">
<input type="hidden" name="form" value="ezviz">
<label>`+esc(T(lang, "setup.email"))+`</label><input name="email" type="email" value="`+esc(c.Email)+`" required>
<label>`+esc(T(lang, "setup.ezvizpw"))+`</label><input name="password" type="password" value="`+esc(c.Password)+`" required>
<label>`+esc(T(lang, "setup.serial"))+`</label><input name="serial" value="`+esc(c.Serial)+`" required>
<label>`+esc(T(lang, "setup.region"))+`</label><select name="region">`+regionOptions(c.Region)+`</select>
<button>`+esc(T(lang, "setup.save"))+`</button>
</form>`+ezvizMsg+`
<div class="anchor" id="stream-mode">
<h1 style="margin-top:26px">`+esc(T(lang, "setup.stream"))+`</h1>
`+streamModeSeg(lang, c.StreamMode)+streamMsg+`
<p class="muted">`+esc(T(lang, "setup.streamHint"))+`</p>
</div>
<h1 style="margin-top:26px">`+esc(T(lang, "setup.sitepw"))+`</h1>
<form method="post">
<input type="hidden" name="form" value="sitepw">
<label>`+esc(T(lang, "setup.current"))+`</label><input name="current" type="password" required>
<label>`+esc(T(lang, "setup.new"))+`</label><input name="new" type="password" minlength="8" required>
<label>`+esc(T(lang, "setup.repeat"))+`</label><input name="repeat" type="password" minlength="8" required>
<button>`+esc(T(lang, "setup.change"))+`</button>
</form>`+pwMsg+`
<p class="muted" style="margin-top:18px">`+esc(T(lang, "setup.token"))+`<br><code>`+esc(c.DeviceToken)+`</code></p>`)
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

func streamModeSeg(lang, current string) string {
	if current != "always" {
		current = "on_demand"
	}
	return `<div class="seg">` + streamModeSegItem(lang, "on_demand", current) + streamModeSegItem(lang, "always", current) + `</div>`
}

func streamModeSegItem(lang, mode, current string) string {
	on := mode == current
	cls, dis := "", ""
	if on {
		cls = ` class="on"`
		dis = " disabled"
	}
	key := "setup.streamOnDemand"
	if mode == "always" {
		key = "setup.streamAlways"
	}
	esc := template.HTMLEscapeString
	return `<form method="post" action="#stream-mode"><input type="hidden" name="form" value="stream"><input type="hidden" name="mode" value="` + mode + `"><button type="submit"` + cls + dis + `>` + esc(T(lang, key)) + `</button></form>`
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
	lang := langOf(r)
	esc := template.HTMLEscapeString
	shareURL := publicOrigin(r) + *basePath + "/hls/" + playlistName + "?token=" + cfgCopy().DeviceToken
	jsT, _ := json.Marshal(map[string]string{
		"noHls":         T(lang, "live.noHls"),
		"relogin":       T(lang, "live.relogin"),
		"notConfigured": T(lang, "live.notConfigured"),
		"lastError":     T(lang, "live.lastError"),
		"saving":        T(lang, "live.saving"),
		"saved":         T(lang, "live.saved"),
		"seeArchive":    T(lang, "live.seeArchive"),
		"saveFail":      T(lang, "live.saveFail"),
		"saveErr":       T(lang, "live.saveErr"),
		"copied":        T(lang, "live.copied"),
		"battery":       T(lang, "live.battery"),
		"online":        T(lang, "live.online"),
		"offline":       T(lang, "live.offline"),
		"upgrade":       T(lang, "live.upgrade"),
		"waking":        T(lang, "live.waking"),
		"alwaysOn":      T(lang, "live.alwaysOn"),
	})
	render(w, r, T(lang, "live.title"), tabs(r, "live")+`
<div class="player">
  <img id="prev" alt="" src="`+*basePath+`/preview.jpg">
  <video id="v" controls autoplay muted playsinline poster="`+*basePath+`/preview.jpg"></video>
</div>
<p class="muted"><span id="bat"></span><span id="st"></span></p>
<p class="muted" id="mode"></p>
<div class="row">
  <button id="save" type="button">`+esc(T(lang, "live.save"))+`</button>
  <button id="share" type="button" class="btn gray" style="margin-top:16px">`+esc(T(lang, "live.share"))+`</button>
</div>
<script>
const base = "`+*basePath+`";
const shareURL = "`+template.JSEscapeString(shareURL)+`";
const t = `+string(jsT)+`;
const prev = document.getElementById("prev");
function showPreview() {
  prev.src = base + "/preview.jpg?t=" + Date.now();
}
prev.onerror = () => { prev.style.display = "none"; };
prev.onload = () => { prev.style.display = "block"; };
const st = document.getElementById("st");
const bat = document.getElementById("bat");
const modeEl = document.getElementById("mode");
let attached = false, hasPlayed = false, lastT = -1, stuckSince = 0, seenRestarts = 0, attachAt = 0, cooldownUntil = 0, readyHits = 0, goneHits = 0;
let pollTimer = 0;

function vid() { return document.getElementById("v"); }
function bindVideo(el) {
  el.addEventListener("playing", () => { hasPlayed = true; prev.style.display = "none"; if (st.textContent === t.waking) st.textContent = ""; });
  el.addEventListener("error", () => { if (attached) detach(); });
}
bindVideo(vid());

function attach() {
  if (Date.now() < cooldownUntil) return;
  const v = vid();
  attached = true;
  hasPlayed = false;
  attachAt = Date.now();
  goneHits = 0;
  if (!v.canPlayType("application/vnd.apple.mpegurl")) {
    st.textContent = t.noHls;
    attached = false;
    return;
  }
  v.src = base + "/hls/`+playlistName+`?t=" + Date.now();
  v.play().catch(()=>{});
}

function detach() {
  attached = false;
  hasPlayed = false;
  attachAt = 0;
  cooldownUntil = Date.now() + 1500;
  const old = vid();
  old.removeAttribute("src");
  const neu = old.cloneNode(false);
  neu.removeAttribute("src");
  old.replaceWith(neu);
  bindVideo(neu);
  showPreview();
}

setInterval(() => {
  const v = vid();
  if (attached && !hasPlayed && attachAt && Date.now() - attachAt > 25000) detach();
  if (!attached || v.paused || !hasPlayed) { lastT = -1; stuckSince = 0; return; }
  if (v.currentTime === lastT) {
    if (!stuckSince) stuckSince = Date.now();
    if (Date.now() - stuckSince > 15000) { stuckSince = 0; detach(); }
  } else stuckSince = 0;
  lastT = v.currentTime;
}, 4000);

function tabActive() { return document.visibilityState === "visible"; }

function schedulePoll(ms) {
  if (pollTimer) clearTimeout(pollTimer);
  pollTimer = setTimeout(poll, ms);
}

async function poll() {
  pollTimer = 0;
  if (!tabActive()) return;
  try {
    const r = await fetch(base + "/start", {method: "POST"});
    if (!tabActive()) return;
    if (!r.ok) { st.textContent = t.relogin; schedulePoll(4000); return; }
    const s = await (await fetch(base + "/api/status")).json();
    if (!tabActive()) return;
    if (s.device && s.device.battery) {
      const d = s.device;
      let line = t.battery + ": " + d.battery + "%";
      if (d.wifi_signal) line += " · Wi-Fi: " + d.wifi_signal + "%";
      line += d.online ? " · " + t.online : " · " + t.offline;
      if (d.upgrade_available === 1) line += " · " + t.upgrade;
      bat.textContent = line;
    }
    if (modeEl) modeEl.textContent = s.stream_mode === "always" ? t.alwaysOn : "";
    if (s.restarts && s.restarts !== seenRestarts) {
      if (seenRestarts) detach();
      seenRestarts = s.restarts;
    }
    const ready = !!(s.running && s.manifest);
    if (ready) { readyHits++; goneHits = 0; } else { readyHits = 0; goneHits++; }
    const dead = !s.running && !s.starting;
    if (attached && dead && goneHits >= 2) detach();
    if (ready && !attached && readyHits >= 1) attach();
    if (!s.configured) st.textContent = t.notConfigured;
    else if (s.last_error && dead) st.textContent = t.lastError + s.last_error;
    else if (!hasPlayed && (s.starting || s.running) && !attached) st.textContent = t.waking;
    else if (hasPlayed && st.textContent === t.waking) st.textContent = "";
    schedulePoll((ready && attached) ? 2000 : 1000);
  } catch(e) { if (tabActive()) schedulePoll(3000); }
}

function onTab() {
  if (tabActive()) poll();
  else {
    if (pollTimer) { clearTimeout(pollTimer); pollTimer = 0; }
    detach();
  }
}
document.addEventListener("visibilitychange", onTab);
window.addEventListener("pageshow", onTab);

document.getElementById("save").onclick = async () => {
  st.textContent = t.saving;
  try {
    const r = await fetch(base + "/save", {method: "POST"});
    st.textContent = r.ok ? t.saved + (await r.json()).name + t.seeArchive : t.saveFail + await r.text();
  } catch(e) { st.textContent = t.saveErr; }
};

document.getElementById("share").onclick = async () => {
  try {
    if (navigator.share) {
      await navigator.share({title: "LE-EZVIZ-UI live", url: shareURL});
      return;
    }
  } catch(e) {
    if (e && e.name === "AbortError") return;
  }
  try {
    await navigator.clipboard.writeText(shareURL);
    st.textContent = t.copied;
  } catch(e) {
    st.textContent = shareURL;
  }
};
</script>`)
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	streamer.Touch()
	w.WriteHeader(http.StatusNoContent)
}

func handlePreview(w http.ResponseWriter, r *http.Request) {
	p := previewPath()
	if _, err := os.Stat(p); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "image/jpeg")
	http.ServeFile(w, r, p)
}

func handleHLS(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	if name != playlistName && !strings.HasSuffix(name, ".m4s") && name != "init.mp4" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	// Cookie HLS from the live tab must not keep the camera awake — only the
	// JS heartbeat (/start) does. A share-token player has no JS, so token
	// requests still count as a viewer.
	if validToken(r) {
		streamer.Touch()
	}
	w.Header().Set("Cache-Control", "no-store")
	path := filepath.Join(hlsDir(), name)
	if _, err := os.Stat(path); err != nil {
		running, starting, _, _, _ := streamer.Status()
		wait := time.Duration(0)
		if running || starting {
			if name == playlistName {
				wait = 20 * time.Second
			} else {
				wait = 5 * time.Second
			}
		}
		if wait == 0 || !waitForFile(path, wait) {
			running, starting, _, _, _ = streamer.Status()
			if running || starting {
				w.Header().Set("Retry-After", "2")
				http.Error(w, "starting", http.StatusServiceUnavailable)
				return
			}
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	if strings.HasSuffix(name, ".m3u8") {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		if t := r.URL.Query().Get("token"); t != "" && validToken(r) {
			data, err := os.ReadFile(path)
			if err != nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Write(rewritePlaylist(data, t))
			return
		}
	}
	http.ServeFile(w, r, path)
}

func rewritePlaylist(data []byte, token string) []byte {
	lines := strings.Split(string(data), "\n")
	for i, ln := range lines {
		if idx := strings.Index(ln, `URI="`); idx >= 0 {
			rest := ln[idx+5:]
			if end := strings.Index(rest, `"`); end >= 0 {
				uri := rest[:end]
				if !strings.Contains(uri, "?") {
					lines[i] = ln[:idx+5] + uri + "?token=" + token + rest[end:]
				}
			}
			continue
		}
		if ln != "" && !strings.HasPrefix(ln, "#") && !strings.Contains(ln, "?") {
			lines[i] = ln + "?token=" + token
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	maybeRefreshDevStatus()
	running, starting, lastError, restarts, startedAt := streamer.Status()
	ds, dsAt := devStatusGet()
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"running":     running,
		"starting":    starting,
		"manifest":    running && hlsPlayable(),
		"last_error":  lastError,
		"restarts":    restarts,
		"configured":  streamer.configured(),
		"stream_mode": streamMode(),
		"device":      ds,
	}
	if !startedAt.IsZero() {
		resp["started_at"] = startedAt
	}
	if !dsAt.IsZero() {
		resp["device_at"] = dsAt
	}
	json.NewEncoder(w).Encode(resp)
}

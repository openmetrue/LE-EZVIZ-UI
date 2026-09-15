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
  .player { position:relative; width:100%; aspect-ratio:16/9; border-radius:10px; background:#000; overflow:hidden; margin-top:8px; }
  .player video { position:absolute; inset:0; width:100%; height:100%; object-fit:cover; background:#000; }
  .player video::-webkit-media-controls-volume-slider { display:none; }
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
	w.Header().Set("Cache-Control", "no-store")
	pageTpl.Execute(w, map[string]any{"Title": title, "Body": template.HTML(body), "Lang": langOf(r)})
}

func isLivePath(p string) bool {
	return p == *basePath || p == *basePath+"/"
}

func publicOrigin(r *http.Request) string {
	proto := r.Header.Get("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
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
	if !isLivePath(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	streamer.Touch()
	lang := langOf(r)
	esc := template.HTMLEscapeString
	shareURL := publicOrigin(r) + *basePath + "/share?token=" + cfgCopy().DeviceToken
	jsT, _ := json.Marshal(map[string]string{
		"noRtc":         T(lang, "live.noRtc"),
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
		"alwaysOn":      T(lang, "live.alwaysOn"),
	})
	render(w, r, T(lang, "live.title"), tabs(r, "live")+`
<div class="player">
  <video id="v" controls autoplay muted playsinline></video>
</div>
<div class="toolbar">
  <p class="statusline"><span id="bat"></span><span id="st"></span></p>
  <div class="actions">
    <button id="save" type="button" class="textbtn">`+esc(T(lang, "live.save"))+`</button>
    <button id="share" type="button" class="textbtn">`+esc(T(lang, "live.share"))+`</button>
  </div>
</div>
<p class="statusline" id="mode"></p>
<script>
`+webrtcPlayerJS("", true)+`
const shareURL = "`+template.JSEscapeString(shareURL)+`";
const t = `+string(jsT)+`;
const bat = document.getElementById("bat");
const modeEl = document.getElementById("mode");

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

func handleShare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := ""
	if validToken(r) {
		token = r.URL.Query().Get("token")
		streamer.Touch()
	}
	lang := langOf(r)
	jsT, _ := json.Marshal(map[string]string{
		"noRtc":         T(lang, "live.noRtc"),
		"relogin":       T(lang, "live.relogin"),
		"lastError":     T(lang, "live.lastError"),
		"notConfigured": T(lang, "live.notConfigured"),
		"battery":       T(lang, "live.battery"),
		"online":        T(lang, "live.online"),
		"offline":       T(lang, "live.offline"),
		"upgrade":       T(lang, "live.upgrade"),
		"alwaysOn":      T(lang, "live.alwaysOn"),
	})
	body := `<div class="player">
  <video id="v" controls autoplay muted playsinline></video>
</div>
<p class="statusline"><span id="bat"></span><span id="st"></span></p>
<script>
` + webrtcPlayerJS(token, true) + `
const t = ` + string(jsT) + `;
const bat = document.getElementById("bat");
</script>`
	if validSession(r) {
		render(w, r, T(lang, "live.title"), tabs(r, "live")+body)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html>
<html lang="%s"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LE-EZVIZ-UI — Live</title>
<style>
  :root { color-scheme: dark; }
  body { font-family: -apple-system, system-ui, sans-serif; background:#0c0f14; color:#e8eaed; margin:0; padding:12px; }
  .player { background:#000; border-radius:8px; overflow:hidden; aspect-ratio:16/9; }
  video { width:100%%; height:100%%; display:block; background:#000; object-fit:contain; }
  .statusline { color:#9aa4b2; font-size:13px; text-align:center; margin:10px 0 0; }
  #bat:not(:empty) { margin-right: 8px; }
</style></head><body>
%s
</body></html>`, lang, body)
}

// webrtcPlayerJS is the shared Live/Share WebRTC client. tokenQ is appended to API URLs.
// withChrome enables battery/mode status fields used on the authenticated Live page.
func webrtcPlayerJS(token string, withChrome bool) string {
	tokJS := template.JSEscapeString(token)
	rtcJS := "false"
	if rtcEnabled() {
		rtcJS = "true"
	}
	chromePoll := ""
	if withChrome {
		chromePoll = `
    if (typeof bat !== "undefined" && s.device && s.device.battery) {
      const d = s.device;
      let line = t.battery + ": " + d.battery + "%";
      if (d.wifi_signal) line += " · Wi-Fi: " + d.wifi_signal + "%";
      line += d.online ? " · " + t.online : " · " + t.offline;
      if (d.upgrade_available === 1) line += " · " + t.upgrade;
      bat.textContent = line;
    }
    if (typeof modeEl !== "undefined" && modeEl) modeEl.textContent = s.stream_mode === "always" ? t.alwaysOn : "";
`
	}
	return `const base = "` + *basePath + `";
const token = "` + tokJS + `";
const tokQ = token ? ("?token=" + encodeURIComponent(token)) : "";
const st = document.getElementById("st");
let attached = false, hasPlayed = false, lastT = -1, stuckSince = 0, seenRestarts = 0, attachAt = 0, cooldownUntil = 0, pc = null, rtcOK = ` + rtcJS + `;
let lastPkts = 0, lastDecoded = 0, decodeStuckSince = 0, streamReady = false;

function vid() { return document.getElementById("v"); }
function bindVideo(el) {
  el.muted = true;
  el.playbackRate = 1;
  el.addEventListener("playing", () => { hasPlayed = true; });
  el.addEventListener("error", () => { if (attached) detach(); });
}
bindVideo(vid());

function waitIce(conn) {
  return new Promise((res) => {
    if (conn.iceGatheringState === "complete") { res(); return; }
    const tmr = setTimeout(() => res(), 50);
    const onCand = () => { clearTimeout(tmr); conn.removeEventListener("icecandidate", onCand); res(); };
    conn.addEventListener("icecandidate", onCand);
    conn.addEventListener("icegatheringstatechange", () => {
      if (conn.iceGatheringState === "complete") { clearTimeout(tmr); conn.removeEventListener("icecandidate", onCand); res(); }
    });
  });
}

async function attachRTC() {
  const v = vid();
  const RTC = window.RTCPeerConnection || window.webkitRTCPeerConnection;
  // Host candidates only — server advertises a public ICE IP; STUN only adds gather delay.
  pc = new RTC({iceServers: []});
  pc.addEventListener("connectionstatechange", () => {
    // iOS often flickers through "disconnected" on brief UDP loss; tearing down
    // the PC/video there causes a gray flash. Only hard-fail kills the session.
    if (pc && pc.connectionState === "failed" && attached) detach();
  });
  pc.addEventListener("track", (ev) => {
    v.srcObject = ev.streams[0] || new MediaStream([ev.track]);
    v.muted = true;
    v.setAttribute("playsinline", "");
    v.setAttribute("webkit-playsinline", "");
    v.play().catch(()=>{});
  });
  const tr = pc.addTransceiver("video", {direction: "recvonly"});
  try {
    const caps = RTCRtpReceiver.getCapabilities && RTCRtpReceiver.getCapabilities("video");
    if (caps && tr.setCodecPreferences) {
      const pref = caps.codecs.filter((c) => /H264/i.test(c.mimeType));
      if (pref.length) tr.setCodecPreferences(pref);
    }
  } catch (_) {}
  const offer = await pc.createOffer();
  if (!/H264/i.test(offer.sdp || "")) throw new Error("no h264 in offer");
  await pc.setLocalDescription(offer);
  await waitIce(pc);
  const r = await fetch(base + "/webrtc" + tokQ, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(pc.localDescription)});
  if (!r.ok) throw new Error("webrtc " + r.status);
  await pc.setRemoteDescription(await r.json());
}

function waitPlaying(ms) {
  const v = vid();
  if (v.videoWidth > 0) return Promise.resolve(true);
  return new Promise((res) => {
    const tmr = setTimeout(() => res(v.videoWidth > 0), ms);
    const done = () => { if (v.videoWidth > 0) { clearTimeout(tmr); res(true); } };
    v.addEventListener("playing", done);
    v.addEventListener("loadeddata", done);
    v.addEventListener("resize", done);
  });
}

function rtcSupported() {
  return !!(window.RTCPeerConnection || window.webkitRTCPeerConnection);
}

async function attach() {
  if (attached || Date.now() < cooldownUntil) return;
  if (!rtcSupported()) {
    if (st) st.textContent = t.noRtc;
    return;
  }
  if (!rtcOK || !streamReady) return;
  attached = true;
  hasPlayed = false;
  attachAt = Date.now();
  lastPkts = 0; lastDecoded = 0; decodeStuckSince = 0;
  try {
    await attachRTC();
    if (await waitPlaying(20000)) {
      if (st && st.textContent === t.noRtc) st.textContent = "";
      return;
    }
  } catch (e) {}
  if (pc) { try { pc.close(); } catch (_) {} pc = null; }
  attached = false;
  hasPlayed = false;
  cooldownUntil = Date.now() + 800;
}

function detach() {
  attached = false;
  hasPlayed = false;
  attachAt = 0;
  lastPkts = 0; lastDecoded = 0; decodeStuckSince = 0;
  cooldownUntil = Date.now() + 800;
  if (pc) { try { pc.close(); } catch (_) {} pc = null; }
  const old = vid();
  old.removeAttribute("src");
  old.srcObject = null;
  const neu = old.cloneNode(false);
  neu.removeAttribute("src");
  neu.srcObject = null;
  neu.setAttribute("playsinline", "");
  neu.setAttribute("webkit-playsinline", "");
  old.replaceWith(neu);
  bindVideo(neu);
}

async function checkDecodeHealth() {
  if (!attached || !pc) return;
  const v = vid();
  // No dimensions after ICE + keyframe budget → gray/black stuck; full renegotiate.
  if (attachAt && Date.now() - attachAt > 8000 && v.videoWidth === 0) {
    detach();
    return;
  }
  try {
    const stats = await pc.getStats();
    let pkts = 0, decoded = 0;
    stats.forEach((r) => {
      if (r.type === "inbound-rtp" && (r.kind === "video" || r.mediaType === "video")) {
        pkts += r.packetsReceived || 0;
        decoded += r.framesDecoded || 0;
      }
    });
    if (pkts > lastPkts + 8 && decoded <= lastDecoded) {
      if (!decodeStuckSince) decodeStuckSince = Date.now();
      // RTP flowing but decoder not producing frames → classic gray lockup.
      if (Date.now() - decodeStuckSince > 4000) { decodeStuckSince = 0; detach(); return; }
    } else decodeStuckSince = 0;
    lastPkts = pkts;
    lastDecoded = decoded;
  } catch (_) {}
}

setInterval(() => {
  const v = vid();
  if (attached && !hasPlayed && attachAt && Date.now() - attachAt > 45000) detach();
  checkDecodeHealth();
  if (!attached || v.paused || !hasPlayed) { lastT = -1; stuckSince = 0; return; }
  if (v.currentTime === lastT) {
    if (!stuckSince) stuckSince = Date.now();
    if (Date.now() - stuckSince > 15000) { stuckSince = 0; detach(); }
  } else stuckSince = 0;
  lastT = v.currentTime;
}, 2000);

async function poll() {
  if (document.visibilityState !== "visible") {
    if (attached && !vid().paused) vid().pause();
  } else try {
    const r = await fetch(base + "/start" + tokQ, {method: "POST"});
    if (document.visibilityState !== "visible") { setTimeout(poll, 1000); return; }
    if (!r.ok) { if (st) st.textContent = t.relogin; setTimeout(poll, hasPlayed ? 1000 : 300); return; }
    if (attached && hasPlayed && vid().paused) vid().play().catch(()=>{});
    const s = await (await fetch(base + "/api/status" + tokQ)).json();
` + chromePoll + `
    rtcOK = !!s.webrtc;
    streamReady = !!s.ready;
    if (s.restarts && s.restarts !== seenRestarts) {
      if (seenRestarts && attached) detach();
      seenRestarts = s.restarts;
    }
    const dead = !s.running && !s.starting;
    if (attached && dead) detach();
    // Wait for an encoder IDR (ready) before joining — mid-GOP join stays gray forever.
    if (!attached && streamReady) attach();
    if (!s.configured && st) st.textContent = t.notConfigured;
    else if (s.last_error && dead && st) st.textContent = t.lastError + s.last_error;
  } catch (e) {}
  const wait = (document.visibilityState === "visible" && !hasPlayed) ? 250 : 1000;
  setTimeout(poll, wait);
}

(async () => {
  try {
    const r = await fetch(base + "/start" + tokQ, {method: "POST"});
    if (!r.ok && st) st.textContent = t.relogin;
  } catch (_) {}
  poll();
})();
`
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	streamer.Touch()
	w.WriteHeader(http.StatusNoContent)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	maybeRefreshDevStatus()
	st := streamer.Status()
	ds, dsAt := devStatusGet()
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"running":     st.Running,
		"starting":    st.Starting,
		"ready":       st.Running && rtcPlayable(),
		"last_error":  st.LastError,
		"restarts":    st.Restarts,
		"configured":  streamer.configured(),
		"stream_mode": streamMode(),
		"webrtc":      rtcEnabled(),
		"device":      ds,
	}
	if !st.StartedAt.IsZero() {
		resp["started_at"] = st.StartedAt
	}
	if !dsAt.IsZero() {
		resp["device_at"] = dsAt
	}
	json.NewEncoder(w).Encode(resp)
}

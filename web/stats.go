package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type statPoint struct {
	TS      int64 `json:"ts"`
	Battery int   `json:"b"`
	Online  bool  `json:"on"`
}

const statusPollEvery = 15 * time.Minute

var (
	statsMu        sync.Mutex
	stats          []statPoint
	statusPollMu   sync.Mutex
	statusPollAt   time.Time
	statusPollBusy bool
)

func statsPath() string { return filepath.Join(*workDir, "stats.json") }

func statsLoad() {
	data, err := os.ReadFile(statsPath())
	if err != nil {
		return
	}
	statsMu.Lock()
	json.Unmarshal(data, &stats)
	if n := len(stats); n > 0 {
		statusPollMu.Lock()
		statusPollAt = time.Unix(stats[n-1].TS, 0)
		statusPollMu.Unlock()
	}
	statsMu.Unlock()
}

func statsSaveLocked() {
	cutoff := time.Now().Add(-45 * 24 * time.Hour).Unix()
	kept := stats[:0]
	for _, p := range stats {
		if p.TS >= cutoff {
			kept = append(kept, p)
		}
	}
	stats = kept
	if data, err := json.Marshal(stats); err == nil {
		os.WriteFile(statsPath(), data, 0o600)
	}
}

func statsClear() {
	statsMu.Lock()
	stats = nil
	statsSaveLocked()
	statsMu.Unlock()
}

func statsCount() int {
	statsMu.Lock()
	defer statsMu.Unlock()
	return len(stats)
}

func pollDeviceStatus() {
	if !streamer.configured() {
		return
	}
	statusPollMu.Lock()
	if statusPollBusy || (!statusPollAt.IsZero() && time.Since(statusPollAt) < statusPollEvery) {
		statusPollMu.Unlock()
		return
	}
	statusPollBusy = true
	statusPollAt = time.Now()
	statusPollMu.Unlock()

	ds, err := fetchDevStatus()
	if err != nil {
		log.Printf("stats: poll failed: %v", err)
		statusPollMu.Lock()
		statusPollAt = time.Now().Add(-statusPollEvery + 3*time.Minute)
		statusPollBusy = false
		statusPollMu.Unlock()
		return
	}
	applyDevStatus(ds)
	bat, _ := strconv.Atoi(strings.TrimSuffix(ds.Battery, "%"))
	statsMu.Lock()
	stats = append(stats, statPoint{TS: time.Now().Unix(), Battery: bat, Online: ds.Online})
	statsSaveLocked()
	statsMu.Unlock()
	log.Printf("devstatus: battery %s%%, wifi %d%%, online=%v", ds.Battery, ds.WifiSignal, ds.Online)

	statusPollMu.Lock()
	statusPollBusy = false
	statusPollMu.Unlock()
}

func statsCollector() {
	for {
		pollDeviceStatus()
		time.Sleep(statusPollEvery)
	}
}

func handleStatsAPI(w http.ResponseWriter, r *http.Request) {
	dur := 24 * time.Hour
	switch r.URL.Query().Get("range") {
	case "week":
		dur = 7 * 24 * time.Hour
	case "month":
		dur = 30 * 24 * time.Hour
	}
	cutoff := time.Now().Add(-dur).Unix()
	statsMu.Lock()
	out := make([]statPoint, 0)
	for _, p := range stats {
		if p.TS >= cutoff {
			out = append(out, p)
		}
	}
	statsMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	esc := template.HTMLEscapeString
	jsT, _ := json.Marshal(map[string]string{
		"points":  T(lang, "stats.points"),
		"empty":   T(lang, "stats.empty"),
		"fail":    T(lang, "stats.fail"),
		"online":  T(lang, "live.online"),
		"offline": T(lang, "live.offline"),
	})
	render(w, r, T(lang, "stats.title"), tabs(r, "stats")+`
<div class="row" style="margin-bottom:10px">
  <button class="rbtn on" data-r="day">`+esc(T(lang, "stats.day"))+`</button>
  <button class="rbtn" data-r="week">`+esc(T(lang, "stats.week"))+`</button>
  <button class="rbtn" data-r="month">`+esc(T(lang, "stats.month"))+`</button>
</div>
<div id="chartwrap" style="position:relative">
  <canvas id="chart" style="width:100%;height:240px;cursor:crosshair;display:block"></canvas>
  <div id="ctip"></div>
</div>
<style>
  #ctip { display:none; position:absolute; pointer-events:none; z-index:2;
    background:#1a1f27; border:1px solid #2a3038; color:#e8eaed; font-size:13px;
    padding:6px 10px; border-radius:8px; white-space:nowrap; box-shadow:0 8px 24px #0008; }
  #ctip b { color:#34d399; font-size:16px; }
</style>
<p class="muted" id="meta"></p>
<script>
const base = "`+*basePath+`";
const loc = "`+localeFor(lang)+`";
const t = `+string(jsT)+`;
const c = document.getElementById("chart");
const wrap = document.getElementById("chartwrap");
const tip = document.getElementById("ctip");
const meta = document.getElementById("meta");
let range = "day";
let pts = [];
let hoverIdx = null;
document.querySelectorAll(".rbtn").forEach(b => b.onclick = () => {
  range = b.dataset.r;
  hoverIdx = null;
  tip.style.display = "none";
  document.querySelectorAll(".rbtn").forEach(x => x.classList.toggle("on", x === b));
  load();
});
async function load() {
  try {
    pts = await (await fetch(base + "/api/stats?range=" + range)).json() || [];
    hoverIdx = null;
    tip.style.display = "none";
    draw();
    meta.textContent = pts.length
      ? t.points.replace("%s", String(pts.length))
      : t.empty;
  } catch(e) { meta.textContent = t.fail; }
}
function geom() {
  const W = c.clientWidth, H = 240;
  const padL = 32, padR = 8, padT = 10, padB = 20;
  const iw = W - padL - padR, ih = H - padT - padB;
  const t0 = pts.length ? pts[0].ts : 0;
  const t1 = pts.length ? pts[pts.length - 1].ts : 1;
  const span = Math.max(1, t1 - t0);
  return {
    W, H, padL, padR, padT, padB, iw, ih,
    px: ts => padL + iw * (ts - t0) / span,
    py: v => padT + ih * (1 - Math.max(0, Math.min(100, v)) / 100)
  };
}
function fmtAxis(ts) {
  return range === "day"
    ? new Date(ts * 1000).toLocaleTimeString(loc, {hour:"2-digit", minute:"2-digit"})
    : new Date(ts * 1000).toLocaleDateString(loc, {day:"2-digit", month:"2-digit"});
}
function fmtHover(ts) {
  return new Date(ts * 1000).toLocaleString(loc, {
    day:"2-digit", month:"2-digit", hour:"2-digit", minute:"2-digit"
  });
}
function draw() {
  const dpr = window.devicePixelRatio || 1;
  const g = geom();
  c.width = g.W * dpr; c.height = g.H * dpr;
  const ctx = c.getContext("2d");
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, g.W, g.H);
  ctx.strokeStyle = "#2a3038"; ctx.fillStyle = "#9aa4b2"; ctx.font = "11px sans-serif"; ctx.lineWidth = 1;
  for (let p = 0; p <= 100; p += 25) {
    const yy = g.padT + g.ih * (1 - p / 100);
    ctx.beginPath(); ctx.moveTo(g.padL, yy); ctx.lineTo(g.W - g.padR, yy); ctx.stroke();
    ctx.fillText(p + "%", 2, yy + 4);
  }
  if (!pts.length) return;
  ctx.fillStyle = "#9aa4b2";
  ctx.fillText(fmtAxis(pts[0].ts), g.padL, g.H - 6);
  const endLbl = fmtAxis(pts[pts.length - 1].ts);
  ctx.fillText(endLbl, g.W - g.padR - ctx.measureText(endLbl).width, g.H - 6);

  if (hoverIdx != null) {
    const hp = pts[hoverIdx];
    const x = g.px(hp.ts);
    ctx.strokeStyle = "rgba(59,130,246,0.55)";
    ctx.lineWidth = 1;
    ctx.setLineDash([4, 4]);
    ctx.beginPath(); ctx.moveTo(x, g.padT); ctx.lineTo(x, g.padT + g.ih); ctx.stroke();
    ctx.setLineDash([]);
  }

  ctx.strokeStyle = "#34d399"; ctx.lineWidth = 2; ctx.beginPath();
  let pen = false, prev = 0;
  for (const p of pts) {
    if (pen && p.ts - prev > 45 * 60) pen = false;
    if (pen) ctx.lineTo(g.px(p.ts), g.py(p.b)); else ctx.moveTo(g.px(p.ts), g.py(p.b));
    pen = true; prev = p.ts;
  }
  ctx.stroke();
  for (let i = 0; i < pts.length; i++) {
    const p = pts[i];
    const active = i === hoverIdx;
    ctx.beginPath();
    ctx.arc(g.px(p.ts), g.py(p.b), active ? 6 : 3.5, 0, Math.PI * 2);
    ctx.fillStyle = "#34d399";
    ctx.fill();
    if (active) {
      ctx.lineWidth = 2;
      ctx.strokeStyle = "#fff";
      ctx.stroke();
    }
  }
}
function nearest(mx) {
  if (!pts.length) return null;
  const g = geom();
  if (mx < g.padL - 4 || mx > g.W - g.padR + 4) return null;
  let best = 0, bestD = Infinity;
  for (let i = 0; i < pts.length; i++) {
    const d = Math.abs(g.px(pts[i].ts) - mx);
    if (d < bestD) { bestD = d; best = i; }
  }
  return best;
}
function showTip(i, mx, my) {
  const p = pts[i];
  const st = p.on ? t.online : t.offline;
  tip.innerHTML = "<b>" + p.b + "%</b> · " + fmtHover(p.ts) + " · " + st;
  tip.style.display = "block";
  const tw = tip.offsetWidth, th = tip.offsetHeight, W = wrap.clientWidth;
  let left = mx + 14;
  if (left + tw > W - 4) left = mx - tw - 10;
  if (left < 4) left = 4;
  let top = my - th - 10;
  if (top < 4) top = my + 14;
  tip.style.left = left + "px";
  tip.style.top = top + "px";
}
c.addEventListener("pointermove", e => {
  const r = c.getBoundingClientRect();
  const mx = e.clientX - r.left, my = e.clientY - r.top;
  const i = nearest(mx);
  if (i == null) {
    if (hoverIdx != null) { hoverIdx = null; draw(); }
    tip.style.display = "none";
    return;
  }
  const changed = hoverIdx !== i;
  hoverIdx = i;
  if (changed) draw();
  showTip(i, mx, my);
});
c.addEventListener("pointerleave", () => {
  hoverIdx = null;
  tip.style.display = "none";
  draw();
});
window.addEventListener("resize", () => { if (pts.length) draw(); });
load();
</script>`)
}

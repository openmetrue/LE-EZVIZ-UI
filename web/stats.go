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

const defaultBatteryPollMin = 15

var (
	statsMu        sync.Mutex
	stats          []statPoint
	statusPollMu   sync.Mutex
	statusPollAt   time.Time
	statusPollBusy bool
)

func batteryPollMin() int {
	m := cfgCopy().BatteryPollMin
	if m < 1 || m > 1440 {
		return defaultBatteryPollMin
	}
	return m
}

func batteryPollEvery() time.Duration {
	return time.Duration(batteryPollMin()) * time.Minute
}

// chartGapSec is how far apart samples may be before the chart breaks the line.
// One missed poll is still a line; two missed polls become a gap.
func chartGapSec(pollMin int) int {
	g := pollMin * 150
	if g < 45*60 {
		return 45 * 60
	}
	return g
}

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

func pollDeviceStatus() {
	tryPollDeviceStatus(false)
}

// tryPollDeviceStatus fetches cloud STATUS/WIFI (does not wake the camera).
// force=true is for a page visit: ignore the configured poll interval, but still
// debounce rapid reloads (~20s) so Live's 1s status loop cannot spam the API.
func tryPollDeviceStatus(force bool) {
	if !streamer.configured() {
		return
	}
	statusPollMu.Lock()
	if statusPollBusy {
		statusPollMu.Unlock()
		return
	}
	minGap := batteryPollEvery()
	if force {
		minGap = 20 * time.Second
	}
	if !statusPollAt.IsZero() && time.Since(statusPollAt) < minGap {
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
		statusPollAt = time.Now().Add(-batteryPollEvery() + 3*time.Minute)
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
	log.Printf("devstatus: battery %s%%, wifi %d%%, online=%v, upgrade=%d",
		ds.Battery, ds.WifiSignal, ds.Online, ds.UpgradeAvailable)

	statusPollMu.Lock()
	statusPollBusy = false
	statusPollMu.Unlock()
}

func statsCollector() {
	for {
		pollDeviceStatus()
		time.Sleep(30 * time.Second)
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

func batteryPollForm(lang string, current int) string {
	esc := template.HTMLEscapeString
	return `<form method="post" action="#battery-poll" class="row" style="margin-top:10px;gap:8px;align-items:end">` +
		`<input type="hidden" name="form" value="poll">` +
		`<div style="flex:1;min-width:120px"><label>` + esc(T(lang, "stats.pollCustom")) + `</label>` +
		`<input name="min" type="number" min="1" max="1440" step="1" value="` + strconv.Itoa(current) + `" required></div>` +
		`<button type="submit" style="margin-top:0">` + esc(T(lang, "stats.pollSave")) + `</button></form>`
}

func saveBatteryPoll(r *http.Request, lang string) string {
	esc := template.HTMLEscapeString
	if r.FormValue("form") != "poll" {
		return ""
	}
	n, err := strconv.Atoi(r.FormValue("min"))
	if err != nil || n < 1 || n > 1440 {
		return `<p class="err">` + esc(T(lang, "stats.pollBad")) + `</p>`
	}
	if err := updateCfg(func(c *Config) error {
		c.BatteryPollMin = n
		return nil
	}); err != nil {
		return `<p class="err">` + esc(err.Error()) + `</p>`
	}
	log.Printf("stats: battery poll every %d min", n)
	return `<p class="ok">` + esc(T(lang, "stats.pollSaved")) + `</p>`
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	esc := template.HTMLEscapeString
	msg := ""
	if r.Method == http.MethodPost {
		msg = saveBatteryPoll(r, lang)
	}
	refreshDevStatusOnVisit()
	pollMin := batteryPollMin()
	jsT, _ := json.Marshal(map[string]any{
		"points":  T(lang, "stats.points"),
		"empty":   T(lang, "stats.empty"),
		"fail":    T(lang, "stats.fail"),
		"battery": T(lang, "live.battery"),
		"online":  T(lang, "live.online"),
		"offline": T(lang, "live.offline"),
	})
	render(w, r, T(lang, "stats.title"), tabs(r, "stats")+`
<div class="seg" id="rangeSeg" style="margin-bottom:10px">
  <button type="button" class="on" data-r="day">`+esc(T(lang, "stats.day"))+`</button>
  <button type="button" data-r="week">`+esc(T(lang, "stats.week"))+`</button>
  <button type="button" data-r="month">`+esc(T(lang, "stats.month"))+`</button>
</div>
<div class="stats-head">
  <span class="stats-cur" id="current">—</span>
  <span class="muted" id="meta"></span>
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
<div class="anchor" id="battery-poll">
<p class="muted" style="margin-top:18px">`+esc(T(lang, "stats.poll"))+`</p>
<p class="muted" style="margin-top:4px">`+esc(T(lang, "stats.pollHint"))+`</p>
`+batteryPollForm(lang, pollMin)+msg+`
<form method="post" action="`+*basePath+`/maint?what=stats#battery-poll"><button class="btn gray">`+esc(T(lang, "maint.clearStats"))+`</button></form>
</div>
<script src="`+*basePath+`/static/stats.js"></script>
<script>
StatsChart.init({
  base: "`+*basePath+`",
  locale: "`+localeFor(lang)+`",
  pollMin: `+strconv.Itoa(pollMin)+`,
  gapSec: `+strconv.Itoa(chartGapSec(pollMin))+`,
  strings: `+string(jsT)+`
});
</script>`)
}

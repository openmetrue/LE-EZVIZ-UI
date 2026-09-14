package main

import (
	"fmt"
	"html"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type logFile struct {
	ID   string
	Path string // empty = journalctl
	Max  int64
}

func logFiles() []logFile {
	return []logFile{
		{"lez", filepath.Join(*workDir, "lez.log"), 80 << 20},
		{"ffmpeg", filepath.Join(*workDir, "ffmpeg.log"), 18 << 20},
		{"bridge", filepath.Join(*workDir, "bridge.err"), 2 << 20},
		{"ezvizd", "", 0},
	}
}

func handleMaint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch r.URL.Query().Get("what") {
	case "logs":
		clearLogFiles()
		http.Redirect(w, r, *basePath+"/logs", http.StatusSeeOther)
		return
	case "stats":
		statsClear()
		log.Printf("maint: battery history reset")
		http.Redirect(w, r, *basePath+"/stats", http.StatusSeeOther)
		return
	case "recordings":
		deleteAllRecordings()
		log.Printf("maint: video archive cleared")
		http.Redirect(w, r, *basePath+"/recordings", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, *basePath+"/setup", http.StatusSeeOther)
}

func clearLogFiles() {
	for _, f := range logFiles() {
		if f.Path != "" {
			os.WriteFile(f.Path, nil, 0o644)
		}
	}
	log.Printf("maint: logs cleared")
}

func logRotator() {
	for {
		time.Sleep(15 * time.Minute)
		for _, f := range logFiles() {
			if f.Path != "" && f.Max > 0 {
				trimLog(f.Path, f.Max)
			}
		}
	}
}

func trimLog(path string, max int64) {
	st, err := os.Stat(path)
	if err != nil || st.Size() <= max {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	tail := make([]byte, max)
	if _, err := f.ReadAt(tail, st.Size()-max); err != nil {
		return
	}
	if err := os.WriteFile(path, tail, 0o644); err == nil {
		log.Printf("maint: trimmed %s (%d MB → %d MB)", filepath.Base(path), st.Size()>>20, max>>20)
	}
}

func maintStats() (logsMB float64, statCount int) {
	var total int64
	for _, f := range logFiles() {
		if f.Path == "" {
			continue
		}
		if st, err := os.Stat(f.Path); err == nil {
			total += st.Size()
		}
	}
	return float64(total) / 1048576, statsCount()
}

func maintSection(lang string) string {
	esc := html.EscapeString
	logsMB, statCount := maintStats()
	return `
<h1 style="margin-top:26px">` + esc(T(lang, "maint.title")) + `</h1>
<p class="muted">` + esc(fmt.Sprintf(T(lang, "maint.summary"), logsMB, fmt.Sprint(statCount))) + `<a href="` + *basePath + `/logs">` + esc(T(lang, "maint.read")) + `</a></p>
<p class="muted">` + esc(T(lang, "maint.statsHint")) + `</p>
<div class="row">
<form method="post" action="` + *basePath + `/maint?what=stats"><button class="btn gray" style="margin-top:0">` + esc(T(lang, "maint.clearStats")) + `</button></form>
</div>`
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	cur := r.URL.Query().Get("f")
	if cur == "" {
		cur = r.FormValue("f")
	}
	if cur == "" {
		cur = "lez"
	}
	okID := false
	for _, f := range logFiles() {
		if f.ID == cur {
			okID = true
			break
		}
	}
	if !okID {
		cur = "lez"
	}
	if r.Method == http.MethodPost {
		loc := *basePath + "/logs?f=" + cur
		switch r.FormValue("action") {
		case "level":
			lvl := "info"
			if r.FormValue("level") == "debug" {
				lvl = "debug"
			}
			_ = updateCfg(func(c *Config) error {
				c.LogLevel = lvl
				return nil
			})
			streamer.Kick()
			loc += "&saved=1"
		case "clear":
			clearLogFiles()
		}
		http.Redirect(w, r, loc, http.StatusSeeOther)
		return
	}
	var chosen *logFile
	var nav string
	for _, f := range logFiles() {
		cls := ""
		if f.ID == cur {
			cls = " on"
			c := f
			chosen = &c
		}
		nav += `<a class="btn gray rbtn` + cls + `" href="` + *basePath + `/logs?f=` + f.ID + `" style="margin-top:0">` + html.EscapeString(T(lang, "logs."+f.ID)) + `</a>`
	}
	if chosen == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("dl") == "1" && chosen.Path != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(chosen.Path)+`"`)
		http.ServeFile(w, r, chosen.Path)
		return
	}

	body, size := readLogTail(chosen, 256<<10, lang)
	meta := T(lang, "logs."+chosen.ID)
	if size > 0 {
		meta += fmt.Sprintf(T(lang, "logs.shown"), float64(len(body))/1024, float64(size)/1048576)
	}
	dl := ""
	if chosen.Path != "" {
		dl = ` <a href="` + *basePath + `/logs?f=` + chosen.ID + `&dl=1">` + html.EscapeString(T(lang, "logs.download")) + `</a>`
	}
	lvl := bridgeLogLevel()
	esc := html.EscapeString
	levelBtn := func(id, key string) string {
		cls := "btn gray rbtn"
		if lvl == id {
			cls += " on"
		}
		return `<form method="post" style="margin:0"><input type="hidden" name="action" value="level"><input type="hidden" name="level" value="` + id + `"><input type="hidden" name="f" value="` + esc(cur) + `"><button class="` + cls + `" style="margin-top:0">` + esc(T(lang, key)) + `</button></form>`
	}
	msg := ""
	if r.URL.Query().Get("saved") == "1" {
		msg = `<p class="ok">` + esc(T(lang, "logs.saved")) + `</p>`
	}
	render(w, r, T(lang, "logs.title"), tabs(r, "logs")+
		`<div class="row" style="margin-bottom:8px">`+nav+`</div>`+
		`<p class="muted">`+esc(T(lang, "logs.level"))+`</p>`+
		`<div class="row" style="margin-bottom:8px">`+levelBtn("info", "logs.levelInfo")+levelBtn("debug", "logs.levelDebug")+`</div>`+
		`<p class="muted">`+esc(T(lang, "logs.levelHint"))+`</p>`+msg+
		`<div class="row" style="margin:12px 0">
<form method="post" onsubmit="return confirm('`+template.JSEscapeString(T(lang, "logs.clearConfirm"))+`')"><input type="hidden" name="action" value="clear"><input type="hidden" name="f" value="`+esc(cur)+`"><button class="btn gray" style="margin-top:0">`+esc(T(lang, "logs.clear"))+`</button></form>
</div>`+
		`<p class="muted">`+esc(T(lang, "logs.clearHint"))+`</p>`+
		`<p class="muted">`+html.EscapeString(meta)+dl+` · `+html.EscapeString(T(lang, "logs.limit"))+`</p>`+
		`<pre class="log">`+html.EscapeString(string(body))+`</pre>`)
}

func readLogTail(f *logFile, max int, lang string) (data []byte, size int64) {
	if f.Path == "" {
		ctxOut, err := exec.Command("journalctl", "-u", serviceUnit, "-n", "400", "--no-pager").Output()
		if err != nil {
			return []byte(err.Error()), 0
		}
		return ctxOut, int64(len(ctxOut))
	}
	st, err := os.Stat(f.Path)
	if err != nil {
		return []byte(T(lang, "logs.empty")), 0
	}
	size = st.Size()
	fh, err := os.Open(f.Path)
	if err != nil {
		return []byte(err.Error()), size
	}
	defer fh.Close()
	if size > int64(max) {
		fh.Seek(size-int64(max), io.SeekStart)
	}
	data, _ = io.ReadAll(fh)
	return data, size
}

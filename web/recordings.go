package main

import (
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var recNameRe = regexp.MustCompile(`^(hp2|rec)-[\d-]+_[\d-]+\.mp4$`)

func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	st := streamer.Status()
	if !st.Active() {
		http.Error(w, T(langOf(r), "save.inactive"), http.StatusServiceUnavailable)
		return
	}
	manifest, err := os.ReadFile(playlistPath())
	if err != nil {
		http.Error(w, T(langOf(r), "save.inactive"), http.StatusServiceUnavailable)
		return
	}
	segs := playlistSegments(manifest)
	if len(segs) == 0 {
		http.Error(w, T(langOf(r), "save.empty"), http.StatusServiceUnavailable)
		return
	}
	if err := os.MkdirAll(recDir(), 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	name := "rec-" + time.Now().Format("2006-01-02_15-04-05") + ".mp4"
	out := filepath.Join(recDir(), name)

	cmd := exec.Command(*ffmpegPath, "-y", "-hide_banner", "-loglevel", "error",
		"-fflags", "+genpts", "-f", "mp4", "-i", "pipe:0",
		"-map", "0:v:0", "-c:v", "copy",
		"-bsf:v", "setts=pts=N/(15*TB):dts=N/(15*TB)",
		"-movflags", "+faststart", out)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := cmd.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go func() {
		defer stdin.Close()
		if init, err := os.ReadFile(hlsFile("init.mp4")); err == nil {
			stdin.Write(init)
		}
		for _, s := range segs {
			if data, err := os.ReadFile(hlsFile(s)); err == nil {
				stdin.Write(data)
			}
		}
	}()
	if err := cmd.Wait(); err != nil {
		os.Remove(out)
		http.Error(w, "ffmpeg: "+err.Error(), http.StatusInternalServerError)
		return
	}
	pruneRecordings()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": name})
}

func pruneRecordings() {
	entries, err := os.ReadDir(recDir())
	if err != nil {
		return
	}
	type rec struct {
		name  string
		size  int64
		mtime time.Time
	}
	var recs []rec
	var total int64
	for _, e := range entries {
		if !recNameRe.MatchString(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			recs = append(recs, rec{e.Name(), info.Size(), info.ModTime()})
			total += info.Size()
		}
	}
	const maxTotal = 1 << 30
	for total > maxTotal && len(recs) > 0 {
		oldest := 0
		for i := range recs {
			if recs[i].mtime.Before(recs[oldest].mtime) {
				oldest = i
			}
		}
		os.Remove(filepath.Join(recDir(), recs[oldest].name))
		total -= recs[oldest].size
		log.Printf("recordings: 1 GB cap exceeded, deleted %s", recs[oldest].name)
		recs = append(recs[:oldest], recs[oldest+1:]...)
	}
}

func handleRecordings(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	entries, _ := os.ReadDir(recDir())
	var rows strings.Builder
	var total int64
	for i := len(entries) - 1; i >= 0; i-- {
		info, err := entries[i].Info()
		if err != nil || !recNameRe.MatchString(entries[i].Name()) {
			continue
		}
		total += info.Size()
		name := entries[i].Name()
		fmt.Fprintf(&rows, `<tr><td><code>%s</code></td><td>%.1f MB</td><td>%s</td>
<td><a href="%s/rec/%s">▶</a> <a href="%s/rec/%s?dl=1">⬇</a>
<a href="#" onclick="fetch('%s/rec/%s/delete',{method:'POST'}).then(()=>location.reload());return false">✕</a></td></tr>`,
			name, float64(info.Size())/1048576, info.ModTime().Format("02.01 15:04"),
			*basePath, name, *basePath, name, *basePath, name)
	}
	render(w, r, T(lang, "rec.title"), tabs(r, "rec")+
		`<p class="muted">`+fmt.Sprintf(T(lang, "rec.total"), fmt.Sprintf("%.1f", float64(total)/1048576))+`</p>`+
		`<form method="post" action="`+*basePath+`/maint?what=recordings" onsubmit="return confirm('`+template.JSEscapeString(T(lang, "rec.confirm"))+`')"><button class="btn gray">`+html.EscapeString(T(lang, "rec.clearAll"))+`</button></form>`+
		`<table style="width:100%;font-size:14px;border-spacing:0 8px">`+rows.String()+`</table>`)
}

func handleRecFile(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, *basePath+"/rec/")
	if strings.HasSuffix(rest, "/delete") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := filepath.Base(strings.TrimSuffix(rest, "/delete"))
		if recNameRe.MatchString(name) {
			os.Remove(filepath.Join(recDir(), name))
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	name := filepath.Base(rest)
	if !recNameRe.MatchString(name) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if r.URL.Query().Get("dl") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	http.ServeFile(w, r, filepath.Join(recDir(), name))
}

func deleteAllRecordings() {
	entries, _ := os.ReadDir(recDir())
	for _, e := range entries {
		if recNameRe.MatchString(e.Name()) {
			os.Remove(filepath.Join(recDir(), e.Name()))
		}
	}
}

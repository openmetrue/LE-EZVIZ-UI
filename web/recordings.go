package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	clipPrefix   = "rec-"
	clipSuffix   = ".mp4"
	maxClips     = 200
	maxClipsSize = 2 << 30 // 2 GiB; oldest clips are pruned past this
)

// saveMu serializes saves so two clips made in the same second cannot pick the
// same name.
var saveMu sync.Mutex

func recordingsDir() string { return workFile("recordings") }

func clipPath(name string) string { return filepath.Join(recordingsDir(), name) }

// validClipName accepts only server-generated names, so a request can never
// address a file outside recordingsDir.
func validClipName(name string) bool {
	if name == "" || filepath.Base(name) != name {
		return false
	}
	if !strings.HasPrefix(name, clipPrefix) || !strings.HasSuffix(name, clipSuffix) {
		return false
	}
	stem := strings.TrimSuffix(strings.TrimPrefix(name, clipPrefix), clipSuffix)
	if stem == "" {
		return false
	}
	for _, r := range stem {
		if (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func newClipName() string {
	base := clipPrefix + time.Now().Format("2006-01-02_15-04-05")
	name := base + clipSuffix
	for i := 1; ; i++ {
		if _, err := os.Stat(clipPath(name)); os.IsNotExist(err) {
			return name
		}
		name = fmt.Sprintf("%s-%d%s", base, i, clipSuffix)
	}
}

type clipInfo struct {
	Name string    `json:"name"`
	Size int64     `json:"size"`
	At   time.Time `json:"at"`
}

func listClips() []clipInfo {
	entries, err := os.ReadDir(recordingsDir())
	if err != nil {
		return nil
	}
	clips := make([]clipInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !validClipName(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		clips = append(clips, clipInfo{Name: e.Name(), Size: info.Size(), At: info.ModTime()})
	}
	sort.Slice(clips, func(i, j int) bool { return clips[i].At.After(clips[j].At) })
	return clips
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// remuxAppleMP4 remuxes without re-encoding. Prefers hvc1 for QuickTime/iOS;
// falls back without the tag if the stream is not HEVC.
func remuxAppleMP4(src, dst string) error {
	try := func(tag string) error {
		args := []string{
			"-y", "-hide_banner", "-loglevel", "error",
			"-fflags", "+genpts+discardcorrupt",
			"-probesize", "8M", "-analyzeduration", "8M",
			"-i", src,
			"-c", "copy", "-an",
			"-avoid_negative_ts", "make_zero",
			"-movflags", "+faststart",
		}
		if tag != "" {
			args = append(args, "-tag:v", tag)
		}
		// Explicit muxer: temp paths like *.mp4.part are not sniffed as mp4.
		args = append(args, "-f", "mp4", dst)
		cmd := exec.Command(*ffmpegPath, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			detail := strings.TrimSpace(string(out))
			if detail == "" {
				return err
			}
			return fmt.Errorf("%w: %s", err, detail)
		}
		return nil
	}
	if err := try("hvc1"); err != nil {
		_ = os.Remove(dst)
		if err2 := try(""); err2 != nil {
			return err
		}
	}
	return nil
}

// remuxLiveClip snapshots the MPEG-PS ring and remuxes to a temp MP4.
// Caller must remove tmpPath when done.
func remuxLiveClip(r *http.Request) (name, tmpPath string, status int, errMsg string) {
	st := streamer.Status()
	if !st.Active() {
		return "", "", http.StatusServiceUnavailable, T(langOf(r), "save.inactive")
	}
	raw, ok := rawRingSnapshot()
	if !ok || len(raw) == 0 {
		return "", "", http.StatusServiceUnavailable, T(langOf(r), "save.empty")
	}
	name = newClipName()
	psFile, err := os.CreateTemp("", "rec-*.ps")
	if err != nil {
		return "", "", http.StatusInternalServerError, err.Error()
	}
	psPath := psFile.Name()
	if _, err := psFile.Write(raw); err != nil {
		psFile.Close()
		os.Remove(psPath)
		return "", "", http.StatusInternalServerError, err.Error()
	}
	psFile.Close()
	defer os.Remove(psPath)

	tmpFile, err := os.CreateTemp("", "rec-*.mp4")
	if err != nil {
		return "", "", http.StatusInternalServerError, err.Error()
	}
	tmpPath = tmpFile.Name()
	tmpFile.Close()

	// Write PS to disk first so ffmpeg can probe the finite buffer; pipe +
	// -map 0:v:0 often failed with exit 234 when no video stream was detected yet.
	if err := remuxAppleMP4(psPath, tmpPath); err != nil {
		os.Remove(tmpPath)
		log.Printf("save: remux: %v", err)
		return "", "", http.StatusInternalServerError, "ffmpeg: " + err.Error()
	}
	return name, tmpPath, 0, ""
}

// storeClip moves a freshly remuxed temp file into the recordings directory.
func storeClip(tmp, name string) (clipInfo, error) {
	if !validClipName(name) {
		return clipInfo{}, fmt.Errorf("invalid clip name %q", name)
	}
	if err := os.MkdirAll(recordingsDir(), 0o755); err != nil {
		return clipInfo{}, err
	}
	dst := clipPath(name)
	if err := os.Rename(tmp, dst); err != nil {
		// Temp files may live on another filesystem than the workdir.
		if err := copyFile(tmp, dst); err != nil {
			return clipInfo{}, err
		}
		os.Remove(tmp)
	}
	pruneClips()
	st, err := os.Stat(dst)
	if err != nil {
		return clipInfo{}, err
	}
	return clipInfo{Name: name, Size: st.Size(), At: st.ModTime()}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// pruneClips removes the oldest clips once the count or total size limit is
// exceeded, so server-side storage cannot fill the disk.
func pruneClips() {
	clips := listClips()
	var total int64
	for i, c := range clips {
		total += c.Size
		if i+1 > maxClips || total > maxClipsSize {
			for _, old := range clips[i:] {
				if err := os.Remove(clipPath(old.Name)); err == nil {
					log.Printf("recordings: pruned %s (limit reached)", old.Name)
				}
			}
			return
		}
	}
}

func writeJSONErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// handleSave remuxes the live HEVC buffer and stores it on the server.
func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	name, tmp, code, msg := remuxLiveClip(r)
	if code != 0 {
		writeJSONErr(w, code, msg)
		return
	}
	defer os.Remove(tmp)
	info, err := storeClip(tmp, name)
	if err != nil {
		log.Printf("save: store: %v", err)
		writeJSONErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("save: stored %s (%s)", info.Name, humanSize(info.Size))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

func handleRecordingsAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	clips := listClips()
	if clips == nil {
		clips = []clipInfo{}
	}
	json.NewEncoder(w).Encode(map[string]any{"clips": clips})
}

// handleRecordingFile serves one clip inline for viewing, or as an attachment
// with ?dl=1. http.ServeFile handles Range requests so the player can seek.
func handleRecordingFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, *basePath+"/rec/")
	if !validClipName(name) {
		http.NotFound(w, r)
		return
	}
	path := clipPath(name)
	if _, err := os.Stat(path); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	if r.URL.Query().Get("dl") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	http.ServeFile(w, r, path)
}

func handleRecordingDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.FormValue("name")
	if validClipName(name) {
		if err := os.Remove(clipPath(name)); err != nil && !os.IsNotExist(err) {
			log.Printf("recordings: delete %s: %v", name, err)
		} else {
			log.Printf("recordings: deleted %s", name)
		}
	}
	http.Redirect(w, r, *basePath+"/recordings?deleted=1", http.StatusSeeOther)
}

func handleRecordings(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	esc := template.HTMLEscapeString
	clips := listClips()

	var list strings.Builder
	if len(clips) == 0 {
		list.WriteString(`<p class="muted">` + esc(T(lang, "rec.empty")) + `</p>`)
	} else {
		var total int64
		for _, c := range clips {
			total += c.Size
			src := *basePath + "/rec/" + c.Name
			list.WriteString(`<div class="row recrow">`)
			list.WriteString(`<button type="button" class="textbtn recplay" data-src="` + esc(src) + `">` + esc(T(lang, "rec.play")) + `</button>`)
			list.WriteString(`<span class="recname">` + esc(c.Name) + `</span>`)
			list.WriteString(`<span class="muted">` + esc(humanSize(c.Size)) + ` · ` + esc(c.At.Format("2006-01-02 15:04")) + `</span>`)
			list.WriteString(`<a class="btn gray" href="` + esc(src+"?dl=1") + `" style="margin-top:0">` + esc(T(lang, "rec.download")) + `</a>`)
			list.WriteString(`<form method="post" action="` + *basePath + `/recordings/delete" onsubmit="return confirm('` + template.JSEscapeString(T(lang, "rec.deleteConfirm")) + `')"><input type="hidden" name="name" value="` + esc(c.Name) + `"><button class="btn gray" style="margin-top:0">` + esc(T(lang, "rec.delete")) + `</button></form>`)
			list.WriteString(`</div>`)
		}
		list.WriteString(`<p class="muted">` + esc(fmt.Sprintf(T(lang, "rec.total"), humanSize(total), len(clips))) + `</p>`)
	}

	msg := ""
	if r.URL.Query().Get("deleted") == "1" {
		msg = `<p class="ok">` + esc(T(lang, "rec.deleted")) + `</p>`
	}

	render(w, r, T(lang, "rec.title"), tabs(r, "recordings")+`
<h1>`+esc(T(lang, "rec.title"))+`</h1>
<div class="player">
  <video id="rv" controls playsinline preload="metadata"></video>
</div>
<p class="muted">`+esc(T(lang, "rec.hint"))+`</p>`+msg+`
<div class="reclist">`+list.String()+`</div>
<style>
  .recrow { border-top:1px solid #2a3038; padding:8px 0; }
  .recrow form { margin:0; }
  .recname { font-size:13px; }
  .reclist { margin-top:12px; }
</style>
<script>
document.querySelectorAll(".recplay").forEach((el) => {
  el.onclick = () => {
    const v = document.getElementById("rv");
    v.src = el.dataset.src;
    v.play().catch(() => {});
    v.scrollIntoView({behavior:"smooth", block:"center"});
  };
});
</script>`)
}

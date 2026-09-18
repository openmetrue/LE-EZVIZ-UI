package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yapingcat/gomedia/go-codec"
	gomp4 "github.com/yapingcat/gomedia/go-mp4"
	"github.com/yapingcat/gomedia/go-mpeg2"
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

var errNoVideo = errors.New("no HEVC video in the ring buffer")

type psAU struct {
	data []byte
	pts  uint64
	dts  uint64
}

func hevcHasParams(au []byte) bool {
	return hevcHasTypeRange(au, codec.H265_NAL_VPS, codec.H265_NAL_PPS)
}

// remuxPSToMP4 demuxes the camera MPEG-PS with gomedia — the same demuxer Live
// uses — and muxes the HEVC access units into a progressive MP4 (hvc1). No
// ffmpeg. Samples start at the first IRAP so the clip opens on a keyframe; any
// parameter-set access units just before it are kept so hvcC stays complete.
func remuxPSToMP4(ps []byte, w io.WriteSeeker) error {
	muxer, err := gomp4.CreateMp4Muxer(w)
	if err != nil {
		return err
	}
	tid := muxer.AddVideoTrack(gomp4.MP4_CODEC_H265)

	var (
		au      []byte
		auPTS   uint64
		auDTS   uint64
		started bool
		pre     []psAU
		samples int
		werr    error
	)

	write := func(a psAU) error {
		if !started {
			if !hevcHasIRAP(a.data) {
				if hevcHasParams(a.data) {
					pre = append(pre, a)
				} else {
					pre = pre[:0]
				}
				return nil
			}
			started = true
			for _, p := range pre {
				if err := muxer.Write(tid, p.data, p.pts, p.dts); err != nil {
					return err
				}
			}
			pre = nil
		}
		samples++
		return muxer.Write(tid, a.data, a.pts, a.dts)
	}

	flush := func() error {
		if len(au) == 0 {
			return nil
		}
		a := psAU{data: au, pts: auPTS, dts: auDTS}
		au = nil
		return write(a)
	}

	newDemuxer := func() *mpeg2.PSDemuxer {
		d := mpeg2.NewPSDemuxer()
		d.OnFrame = func(frame []byte, cid mpeg2.PS_STREAM_TYPE, pts, dts uint64) {
			if werr != nil || cid != mpeg2.PS_STREAM_H265 || len(frame) == 0 {
				return
			}
			if len(au) > 0 && pts != auPTS {
				if err := flush(); err != nil {
					werr = err
					return
				}
			}
			if dts == 0 || dts > pts {
				dts = pts
			}
			au = append(au, frame...)
			auPTS, auDTS = pts, dts
		}
		return d
	}

	demuxer := newDemuxer()
	const chunk = 64 << 10
	for off := 0; off < len(ps) && werr == nil; off += chunk {
		end := off + chunk
		if end > len(ps) {
			end = len(ps)
		}
		func() {
			defer func() {
				if recover() != nil {
					demuxer = newDemuxer()
				}
			}()
			_ = demuxer.Input(ps[off:end])
		}()
	}
	if werr != nil {
		return werr
	}
	if err := flush(); err != nil {
		return err
	}
	if samples == 0 {
		return errNoVideo
	}
	return muxer.WriteTrailer()
}

// remuxLiveClip snapshots the MPEG-PS ring and muxes it to a temp MP4.
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
	tmpFile, err := os.CreateTemp("", "rec-*.mp4")
	if err != nil {
		return "", "", http.StatusInternalServerError, err.Error()
	}
	tmpPath = tmpFile.Name()
	if err := remuxPSToMP4(raw, tmpFile); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		log.Printf("save: remux: %v", err)
		return "", "", http.StatusInternalServerError, "remux: " + err.Error()
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return "", "", http.StatusInternalServerError, err.Error()
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

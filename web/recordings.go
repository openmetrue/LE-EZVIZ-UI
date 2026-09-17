package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func clipFileName() string {
	return "rec-" + time.Now().Format("2006-01-02_15-04-05") + ".mp4"
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
	name = clipFileName()
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

// handleSave remuxes the live HEVC buffer and streams it as an attachment.
// Nothing is kept on the server.
func handleSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name, tmp, code, msg := remuxLiveClip(r)
	if code != 0 {
		http.Error(w, msg, code)
		return
	}
	defer os.Remove(tmp)
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, tmp)
}

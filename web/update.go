package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"time"
)

func handleVersionAPI(w http.ResponseWriter, r *http.Request) {
	st := readUpdateStatus()
	updateMu.Lock()
	busy := updateBusy
	updateMu.Unlock()
	if st.Phase == "download" || st.Phase == "install" {
		busy = true
	}
	// After a successful detached install, clear stale "download" if version already matches.
	if st.Phase != "" && st.Phase != "done" && st.Phase != "error" &&
		st.Target != "" && versionsEqual(installedVersion(), st.Target) {
		st.Phase = "done"
		writeUpdateStatus("done", st.Target, "")
		busy = false
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"version": installedVersion(),
		"busy":    busy,
		"phase":   st.Phase,
		"target":  st.Target,
		"error":   st.Error,
		"at":      st.At,
	})
}

func handleUpdateAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	cur := installedVersion()
	latest, err := latestGitHubRelease(defaultUpdateRepo)
	if err != nil {
		log.Printf("update: latest: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "error",
			"version": cur,
			"error":   err.Error(),
		})
		return
	}
	if versionsEqual(cur, latest) {
		writeUpdateStatus("done", latest, "")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "up_to_date",
			"version": cur,
			"latest":  latest,
		})
		return
	}

	st := readUpdateStatus()
	updateMu.Lock()
	busy := updateBusy || st.Phase == "download" || st.Phase == "install"
	if busy {
		updateMu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "busy",
			"version": cur,
			"latest":  latest,
			"phase":   st.Phase,
			"target":  st.Target,
		})
		return
	}
	updateBusy = true
	updateMu.Unlock()

	writeUpdateStatus("download", latest, "")

	json.NewEncoder(w).Encode(map[string]any{
		"status":  "updating",
		"version": cur,
		"latest":  latest,
		"phase":   "download",
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go scheduleInstall(cur, latest)
}

func runUpdateCheck(lang string) string {
	esc := template.HTMLEscapeString
	cur := installedVersion()
	latest, err := latestGitHubRelease(defaultUpdateRepo)
	if err != nil {
		return `<p class="err">` + esc(T(lang, "setup.updateFail")) + ` ` + esc(err.Error()) + `</p>`
	}
	if versionsEqual(cur, latest) {
		return `<p class="ok">` + esc(T(lang, "setup.upToDate")) + `</p>`
	}
	st := readUpdateStatus()
	updateMu.Lock()
	busy := updateBusy || st.Phase == "download" || st.Phase == "install"
	if !busy {
		updateBusy = true
	}
	updateMu.Unlock()
	if busy {
		return `<p class="ok">` + esc(T(lang, "setup.updating")) + `</p>`
	}
	writeUpdateStatus("download", latest, "")
	go scheduleInstall(cur, latest)
	return `<p class="ok">` + esc(T(lang, "setup.updating")) + `</p>`
}

// reconcileUpdateStatusOnBoot clears a stuck "download/install" if we already
// landed on the target version (e.g. after self-restart mid-update).
func reconcileUpdateStatusOnBoot() {
	st := readUpdateStatus()
	if st.Target == "" {
		return
	}
	if versionsEqual(installedVersion(), st.Target) && st.Phase != "error" {
		writeUpdateStatus("done", st.Target, "")
		return
	}
	// Stale in-progress older than 10 minutes → mark error so UI can recover.
	if (st.Phase == "download" || st.Phase == "install") && st.At != "" {
		if t, err := time.Parse(time.RFC3339, st.At); err == nil && time.Since(t) > 10*time.Minute {
			writeUpdateStatus("error", st.Target, "update timed out")
		}
	}
}

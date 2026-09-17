package main

import (
	"encoding/json"
	"net/http"
)

// Live status JSON contract for a browser Live client:
//
//	ready            — gomedia has emitted an HEVC IRAP (safe to play)
//	running/starting — pipeline lifecycle
//	restarts         — increments each pipeline start (client reconnects)
//	configured       — EZVIZ creds + active serial present
//	last_error       — last pipeline failure string
//	stream_mode      — on_demand | always
//	device           — cloud STATUS/WIFI snapshot for active serial
//	active_serial    — camera currently streamed
//	viewers          — current /live HTTP subscribers
//	restart_reason   — last watchdog/idle/error reason (may be empty)
//	codec            — RFC 6381 hvc1 string once HEVC is ready (may be empty)

func handleStatus(w http.ResponseWriter, r *http.Request) {
	streamer.Touch()
	if r.URL.Query().Get("fresh") == "1" {
		refreshDevStatusOnVisit()
	} else {
		maybeRefreshDevStatus()
	}
	st := streamer.Status()
	ds, dsAt := devStatusGet()
	ready := liveReady()
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"running":        st.Running,
		"starting":       st.Starting,
		"ready":          ready,
		"last_error":     st.LastError,
		"restarts":       st.Restarts,
		"configured":     streamer.configured(),
		"stream_mode":    streamMode(),
		"device":         ds,
		"active_serial":  activeSerial(cfgCopy()),
		"viewers":        liveViewers(),
		"restart_reason": st.RestartReason,
		"codec":          liveCodec(),
	}
	if !st.StartedAt.IsZero() {
		resp["started_at"] = st.StartedAt
	}
	if !dsAt.IsZero() {
		resp["device_at"] = dsAt
	}
	json.NewEncoder(w).Encode(resp)
}

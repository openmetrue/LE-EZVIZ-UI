package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type devStatus struct {
	Battery          string `json:"battery"`
	Online           bool   `json:"online"`
	WifiSignal       int    `json:"wifi_signal"`
	WifiSSID         string `json:"wifi_ssid"`
	Pir              int    `json:"pir"`
	UpgradeAvailable int    `json:"upgrade_available"`
	KeepAliveSec     int    `json:"keep_alive_sec"`
	Cover            string `json:"cover,omitempty"`
}

var (
	devStatusMu  sync.Mutex
	devStatusVal devStatus
	devStatusAt  time.Time
)

func fetchDevStatus() (devStatus, error) {
	email, password, serial, region := streamer.creds()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, *bridgePath,
		"-region", region, "-deviceSerial", serial,
		"-statusOnly", "-stdout=false", "-logFile=true",
		"-logLevel", bridgeLogLevel())
	cmd.Dir = *workDir
	cmd.Env = append(os.Environ(), "EZVIZ_EMAIL="+email, "EZVIZ_PASSWORD="+password)
	out, err := cmd.Output()
	if err != nil {
		return devStatus{}, err
	}
	var ds devStatus
	if err := json.Unmarshal(out, &ds); err != nil {
		return devStatus{}, err
	}
	if ds.Battery == "" {
		return devStatus{}, fmt.Errorf("empty status response")
	}
	return ds, nil
}

func applyDevStatus(ds devStatus) {
	devStatusMu.Lock()
	devStatusVal = ds
	devStatusAt = time.Now()
	devStatusMu.Unlock()
	devStatusSave()
	if ds.Cover != "" {
		go maybeDownloadCover(ds.Cover)
	}
}

func devStatusSave() {
	devStatusMu.Lock()
	defer devStatusMu.Unlock()
	data, err := json.Marshal(map[string]any{"status": devStatusVal, "at": devStatusAt})
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(*workDir, "devstatus.json"), data, 0o600)
}

func devStatusLoad() {
	data, err := os.ReadFile(filepath.Join(*workDir, "devstatus.json"))
	if err != nil {
		return
	}
	var v struct {
		Status devStatus `json:"status"`
		At     time.Time `json:"at"`
	}
	if json.Unmarshal(data, &v) == nil && v.Status.Battery != "" {
		devStatusMu.Lock()
		devStatusVal = v.Status
		devStatusAt = v.At
		devStatusMu.Unlock()
	}
}

func maybeRefreshDevStatus() {
	statusPollMu.Lock()
	due := !statusPollBusy && (statusPollAt.IsZero() || time.Since(statusPollAt) >= statusPollEvery)
	statusPollMu.Unlock()
	if due {
		go pollDeviceStatus()
	}
}

func devStatusGet() (devStatus, time.Time) {
	devStatusMu.Lock()
	defer devStatusMu.Unlock()
	return devStatusVal, devStatusAt
}

func previewPath() string { return filepath.Join(*workDir, "preview.jpg") }

// maybeDownloadCover fetches the EZVIZ cloud cover (resourceCover / devpic).
func maybeDownloadCover(url string) {
	if url == "" || (!strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://")) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("preview: cover download failed: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil || len(data) < 100 {
		return
	}
	os.WriteFile(previewPath(), data, 0o644)
}

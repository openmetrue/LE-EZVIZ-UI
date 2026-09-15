package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
	cmd := exec.CommandContext(ctx, *bridgePath, bridgeArgv(region, serial,
		"-statusOnly", "-stdout=false", "-logFile=true")...)
	applyBridgeEnv(cmd, email, password)
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
}

func devStatusSave() {
	devStatusMu.Lock()
	defer devStatusMu.Unlock()
	data, err := json.Marshal(map[string]any{"status": devStatusVal, "at": devStatusAt})
	if err != nil {
		return
	}
	os.WriteFile(workFile("devstatus.json"), data, 0o600)
}

func devStatusLoad() {
	data, err := os.ReadFile(workFile("devstatus.json"))
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
	due := !statusPollBusy && (statusPollAt.IsZero() || time.Since(statusPollAt) >= batteryPollEvery())
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

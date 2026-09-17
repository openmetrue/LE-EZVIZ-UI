package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type cloudDevice struct {
	Serial  string `json:"serial"`
	Name    string `json:"name"`
	Channel int    `json:"channel"`
}

func fetchDeviceList(email, password, region string) ([]cloudDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	args := []string{"-region", region, "-listDevices"}
	if bridgeLogEnabled() {
		args = append(args, "-logFile")
	}
	cmd := exec.CommandContext(ctx, *bridgePath, args...)
	applyBridgeEnv(cmd, email, password)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	var list []cloudDevice
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("parse device list: %w", err)
	}
	return list, nil
}

func pickDefaultSerial(email, password, region string) string {
	list, err := fetchDeviceList(email, password, region)
	if err != nil || len(list) == 0 {
		return ""
	}
	return list[0].Serial
}

func serialInList(serial string, list []cloudDevice) bool {
	for _, d := range list {
		if d.Serial == serial {
			return true
		}
	}
	return false
}

func handleDevicesAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c := cfgCopy()
	if !hasEzvizCreds(c) {
		http.Error(w, "not configured", http.StatusServiceUnavailable)
		return
	}
	list, err := fetchDeviceList(c.Email, c.Password, c.Region)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	active := activeSerial(c)
	if active == "" && len(list) > 0 {
		active = list[0].Serial
		_ = updateCfg(func(c *Config) error {
			c.ActiveSerial = active
			c.Serial = active
			return nil
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"active":  active,
		"devices": list,
	})
}

func handleLiveDevice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Serial string `json:"serial"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	serial := strings.TrimSpace(req.Serial)
	if serial == "" {
		http.Error(w, "serial required", http.StatusBadRequest)
		return
	}
	c := cfgCopy()
	if !hasEzvizCreds(c) {
		http.Error(w, "not configured", http.StatusServiceUnavailable)
		return
	}
	list, err := fetchDeviceList(c.Email, c.Password, c.Region)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !serialInList(serial, list) {
		http.Error(w, "unknown device", http.StatusBadRequest)
		return
	}
	if serial == activeSerial(c) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"active": serial})
		return
	}
	if err := updateCfg(func(c *Config) error {
		c.ActiveSerial = serial
		c.Serial = serial
		return nil
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	streamer.Kick()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"active": serial})
}

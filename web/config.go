package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	PasswordHash   string `json:"password_hash"`  // bcrypt of the site password; empty = first run
	SessionSecret  string `json:"session_secret"` // hex, HMAC key for session cookies
	Email          string `json:"email"`
	Password       string `json:"password"`
	Serial         string `json:"serial"`        // legacy; kept in sync with active_serial
	ActiveSerial   string `json:"active_serial"` // camera streamed on Live
	Region         string `json:"region"`
	BridgeLog      bool   `json:"bridge_log"`       // write verbose bridge logs to lez.log
	StreamMode     string `json:"stream_mode"`      // on_demand (default) | always
	BatteryPollMin int    `json:"battery_poll_min"` // background cloud status poll, minutes 1–1440; visits also refresh
}

var (
	cfg   Config
	cfgMu sync.Mutex
)

func loadConfig() error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	data, err := os.ReadFile(*configPath)
	if os.IsNotExist(err) {
		cfg = Config{Region: "Russia"}
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	normalizeConfig(&cfg)
	return nil
}

func normalizeConfig(c *Config) {
	if c.ActiveSerial == "" {
		c.ActiveSerial = c.Serial
	}
	if c.ActiveSerial != "" {
		c.Serial = c.ActiveSerial
	}
}

func activeSerial(c Config) string {
	if c.ActiveSerial != "" {
		return c.ActiveSerial
	}
	return c.Serial
}

func hasEzvizCreds(c Config) bool {
	return c.Email != "" && c.Password != "" && c.Region != ""
}

func cfgCopy() Config {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return cfg
}

func updateCfg(fn func(*Config) error) error {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	if err := fn(&cfg); err != nil {
		return err
	}
	return saveConfigLocked()
}

func saveConfigLocked() error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*configPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(*configPath, data, 0o600)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

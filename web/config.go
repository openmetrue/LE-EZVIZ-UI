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
	PasswordHash  string `json:"password_hash"`  // bcrypt of the site password; empty = first run
	SessionSecret string `json:"session_secret"` // hex, HMAC key for session cookies
	DeviceToken   string `json:"device_token"`   // token for machine / share URLs
	Email         string `json:"email"`
	Password      string `json:"password"`
	Serial        string `json:"serial"`
	Region        string `json:"region"`
	LogLevel      string `json:"log_level"` // debug|info; empty means info
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
	return json.Unmarshal(data, &cfg)
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

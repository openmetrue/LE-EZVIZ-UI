package main

import "testing"

func TestBatteryPollMinDefault(t *testing.T) {
	orig := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.BatteryPollMin = 0
	cfgMu.Unlock()
	if batteryPollMin() != 15 {
		t.Fatalf("empty config should default to 15, got %d", batteryPollMin())
	}
	cfgMu.Lock()
	cfg.BatteryPollMin = 7
	cfgMu.Unlock()
	if batteryPollMin() != 15 {
		t.Fatalf("invalid interval should default to 15, got %d", batteryPollMin())
	}
	cfgMu.Lock()
	cfg.BatteryPollMin = 5
	cfgMu.Unlock()
	if batteryPollMin() != 5 {
		t.Fatalf("got %d", batteryPollMin())
	}
}

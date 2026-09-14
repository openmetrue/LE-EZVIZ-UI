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

func TestChartGapAllowsHourlyPoll(t *testing.T) {
	if chartGapSec(5) != 45*60 {
		t.Fatalf("5 min: %d", chartGapSec(5))
	}
	if chartGapSec(15) != 45*60 {
		t.Fatalf("15 min: %d", chartGapSec(15))
	}
	if chartGapSec(30) != 75*60 {
		t.Fatalf("30 min: %d", chartGapSec(30))
	}
	if g := chartGapSec(60); g <= 60*60 {
		t.Fatalf("1 h poll must connect hourly samples, gap %d", g)
	}
}

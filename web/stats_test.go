package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
	if batteryPollMin() != 7 {
		t.Fatalf("custom 7 min should be allowed, got %d", batteryPollMin())
	}
	cfgMu.Lock()
	cfg.BatteryPollMin = 1441
	cfgMu.Unlock()
	if batteryPollMin() != 15 {
		t.Fatalf("out of range should default to 15, got %d", batteryPollMin())
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

func TestStatsPageUsesExternalChart(t *testing.T) {
	orig := streamer
	streamer = NewStreamer()
	t.Cleanup(func() { streamer = orig })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ezviz/stats", nil)
	handleStats(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"StatsChart.init", `id="rangeSeg"`, `data-r="week"`, `data-r="month"`,
		`/static/stats.js`, `id="current"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stats page missing %q", want)
		}
	}
	if strings.Contains(body, "function geom()") {
		t.Fatal("stats page still inlines the old chart")
	}
}

func TestStaticStatsJS(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/ezviz/static/", http.StripPrefix("/ezviz/static/", staticHandler()))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ezviz/static/stats.js", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"StatsChart", "RANGES", "rangeSeg", "/api/stats?range=", "domainStart",
		"p.ts * 1000", "gapSec * 1000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("stats.js missing %q", want)
		}
	}
}

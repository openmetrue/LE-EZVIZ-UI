package main

import (
	"testing"
	"time"
)

func TestWantedAlwaysOn(t *testing.T) {
	orig := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.Email, cfg.Password, cfg.Serial = "a", "b", "c"
	cfg.StreamMode = "always"
	cfgMu.Unlock()
	s := NewStreamer()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wantedLocked() {
		t.Fatal("always-on should be wanted without lastTouch")
	}
	cfgMu.Lock()
	cfg.StreamMode = "on_demand"
	cfgMu.Unlock()
	if s.wantedLocked() {
		t.Fatal("on-demand without lastTouch should not be wanted")
	}
	s.lastTouch = time.Now()
	if !s.wantedLocked() {
		t.Fatal("on-demand with fresh lastTouch should be wanted")
	}
}

func TestOnDemandExpiresAfterIdle(t *testing.T) {
	orig := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.Email, cfg.Password, cfg.Serial = "a", "b", "c"
	cfg.StreamMode = "on_demand"
	cfgMu.Unlock()
	s := NewStreamer()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastTouch = time.Now().Add(-idleTimeout - time.Second)
	if s.wantedLocked() {
		t.Fatal("on-demand should not be wanted after idle timeout")
	}
}

func TestKickDoesNotCountAsViewer(t *testing.T) {
	s := NewStreamer()
	s.Kick()
	if !s.lastTouch.IsZero() {
		t.Fatal("Kick should not create a viewer")
	}
}

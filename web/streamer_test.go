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
	cfg.Email, cfg.Password, cfg.Region, cfg.ActiveSerial, cfg.Serial = "a", "b", "Russia", "c", "c"
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
	cfg.Email, cfg.Password, cfg.Region, cfg.ActiveSerial, cfg.Serial = "a", "b", "Russia", "c", "c"
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

func TestReholdBackoffBlocksWanted(t *testing.T) {
	orig := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.Email, cfg.Password, cfg.Region, cfg.ActiveSerial, cfg.Serial = "a", "b", "Russia", "c", "c"
	cfg.StreamMode = "always"
	cfgMu.Unlock()
	s := NewStreamer()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reholdUntil = time.Now().Add(time.Minute)
	if s.wantedLocked() {
		t.Fatal("wanted must be false during rehold backoff")
	}
	s.reholdUntil = time.Now().Add(-time.Second)
	if !s.wantedLocked() {
		t.Fatal("wanted should resume after rehold backoff")
	}
}

func TestKickSetsRestartReason(t *testing.T) {
	s := NewStreamer()
	s.Kick()
	st := s.Status()
	if st.RestartReason != "kick" {
		t.Fatalf("restart reason=%q", st.RestartReason)
	}
}

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

func TestStreamStatusActive(t *testing.T) {
	if (StreamStatus{}).Active() {
		t.Fatal("zero status should be inactive")
	}
	if !(StreamStatus{Running: true}).Active() || !(StreamStatus{Starting: true}).Active() {
		t.Fatal("running or starting should be active")
	}
}

func TestIsLivePath(t *testing.T) {
	orig := *basePath
	t.Cleanup(func() { *basePath = orig })
	*basePath = "/ezviz"
	if !isLivePath("/ezviz") || !isLivePath("/ezviz/") {
		t.Fatal("live paths")
	}
	if isLivePath("/ezviz/preview.jpg") || isLivePath("/ezviz/share") {
		t.Fatal("non-live paths")
	}
}

func TestRingSnapshotNeedsIDR(t *testing.T) {
	ringClear()
	t.Cleanup(ringClear)
	ringPush([]byte{0x00, 0x00, 0x00, 0x01, 0x01}, false)
	if _, ok := ringSnapshot(); ok {
		t.Fatal("P-frame only must not snapshot")
	}
	if rtcPlayable() {
		t.Fatal("not ready without IDR")
	}
	ringPush([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, true)
	ringPush([]byte{0x00, 0x00, 0x00, 0x01, 0x01}, false)
	data, ok := ringSnapshot()
	if !ok || len(data) < 10 {
		t.Fatalf("snapshot ok=%v len=%d", ok, len(data))
	}
	if !rtcPlayable() || !rtcFresh(time.Second) {
		t.Fatal("ready after IDR")
	}
}

package main

import (
	"errors"
	"testing"
	"time"
)

func TestWaitBothAfterOneReceive(t *testing.T) {
	done := make(chan error, 2)
	done <- errors.New("remaining")
	waitBoth(done, 1)
	select {
	case <-done:
		t.Fatal("waitBoth drained more than the remaining Wait")
	default:
	}
}

func TestWaitBothFromScratch(t *testing.T) {
	done := make(chan error, 2)
	done <- nil
	done <- nil
	waitBoth(done, 0)
}

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

func TestWaitBothDoesNotHang(t *testing.T) {
	done := make(chan error, 2)
	go func() {
		time.Sleep(20 * time.Millisecond)
		done <- errors.New("a")
		done <- errors.New("b")
	}()
	finished := make(chan struct{})
	go func() {
		waitBoth(done, 0)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("waitBoth hung")
	}
}

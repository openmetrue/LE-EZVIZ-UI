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

func TestStopClearsViewer(t *testing.T) {
	s := NewStreamer()
	s.Touch()
	if s.lastTouch.IsZero() {
		t.Fatal("Touch should set lastTouch")
	}
	s.Stop()
	if !s.lastTouch.IsZero() {
		t.Fatal("Stop should forget the viewer so idle restart cannot happen")
	}
	if s.cancel != nil {
		t.Fatal("Stop should not leave a live cancel when nothing was running")
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

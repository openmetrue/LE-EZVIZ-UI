package main

import (
	"context"
	"io"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var idleTimeout = 10 * time.Second

const (
	restartBackoff      = 15 * time.Second
	noMediaWatchdog     = 20 * time.Second
	staleMediaWatchdog  = 12 * time.Second
	maxNoMediaWatchdogs = 3
	reholdBackoff       = 25 * time.Second
)

type Streamer struct {
	mu             sync.Mutex
	running        bool
	starting       bool
	lastError      string
	restartReason  string
	restarts       int
	startedAt      time.Time
	lastTouch      time.Time
	cancel         context.CancelFunc
	hold           *exec.Cmd
	holdIn         io.WriteCloser
	holdSerial     string
	noMediaStrikes int
	reholdUntil    time.Time
}

func NewStreamer() *Streamer { return &Streamer{} }

func (s *Streamer) Touch() {
	s.mu.Lock()
	s.lastTouch = time.Now()
	s.mu.Unlock()
	s.maybeStart()
}

func (s *Streamer) Kick() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.killHoldLocked()
	s.noMediaStrikes = 0
	s.reholdUntil = time.Time{}
	s.restartReason = "kick"
	s.mu.Unlock()
}

func alwaysOn() bool {
	return strings.EqualFold(strings.TrimSpace(cfgCopy().StreamMode), "always")
}

func streamMode() string {
	if alwaysOn() {
		return "always"
	}
	return "on_demand"
}

// wantedLocked reports whether the pipeline should run. Caller holds s.mu.
func (s *Streamer) wantedLocked() bool {
	if !s.configured() {
		return false
	}
	if !s.reholdUntil.IsZero() && time.Now().Before(s.reholdUntil) {
		return false
	}
	if alwaysOn() {
		return true
	}
	if liveViewers() > 0 {
		return true
	}
	return !s.lastTouch.IsZero() && time.Since(s.lastTouch) < idleTimeout
}

func (s *Streamer) maybeStart() {
	if !s.configured() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running || s.starting || !s.wantedLocked() {
		return
	}
	s.starting = true
	s.lastError = ""
	email, password, serial, region := s.creds()
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go s.runWithBackoff(ctx, email, password, serial, region)
}

func (s *Streamer) configured() bool {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return hasEzvizCreds(cfg) && activeSerial(cfg) != ""
}

func (s *Streamer) creds() (email, password, serial, region string) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return cfg.Email, cfg.Password, activeSerial(cfg), cfg.Region
}

func bridgeLogEnabled() bool {
	return cfgCopy().BridgeLog
}

type StreamStatus struct {
	Running       bool
	Starting      bool
	LastError     string
	RestartReason string
	Restarts      int
	StartedAt     time.Time
}

func (st StreamStatus) Active() bool { return st.Running || st.Starting }

func (s *Streamer) Status() StreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StreamStatus{
		Running:       s.running,
		Starting:      s.starting,
		LastError:     s.lastError,
		RestartReason: s.restartReason,
		Restarts:      s.restarts,
		StartedAt:     s.startedAt,
	}
}

func (s *Streamer) supervise() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if s.configured() {
			email, password, serial, region := s.creds()
			if err := s.ensureHold(email, password, serial, region); err != nil {
				log.Printf("streamer: session: %v", err)
			}
		}
		s.maybeStart()
		s.mu.Lock()
		desired := s.wantedLocked()
		switch {
		case !desired && (s.running || s.starting) && s.cancel != nil:
			if !s.reholdUntil.IsZero() && time.Now().Before(s.reholdUntil) {
				log.Printf("streamer: rehold backoff until %s", s.reholdUntil.Format(time.RFC3339))
			} else {
				log.Printf("streamer: idle %s, stopping so the camera can sleep", idleTimeout)
				s.restartReason = "idle"
			}
			s.cancel()
		case desired && s.running && s.cancel != nil:
			age := time.Since(s.startedAt)
			if age >= noMediaWatchdog && !rawRingFresh(noMediaWatchdog) {
				s.noMediaStrikes++
				log.Printf("streamer: watchdog — no media after 20s (strike %d/%d), restarting pipeline",
					s.noMediaStrikes, maxNoMediaWatchdogs)
				s.restartReason = "no_media"
				if s.noMediaStrikes >= maxNoMediaWatchdogs {
					log.Printf("streamer: too many empty pipelines — rehold backoff %s", reholdBackoff)
					s.killHoldLocked()
					s.reholdUntil = time.Now().Add(reholdBackoff)
					s.noMediaStrikes = 0
					s.restartReason = "rehold_backoff"
				}
				s.cancel()
			} else if age >= 60*time.Second && !rawRingFresh(staleMediaWatchdog) {
				log.Printf("streamer: watchdog — media stale for 12s, restarting pipeline")
				s.restartReason = "stale_media"
				s.cancel()
			} else if rawRingFresh(staleMediaWatchdog) {
				s.noMediaStrikes = 0
			}
		}
		s.mu.Unlock()
	}
}

func (s *Streamer) runWithBackoff(ctx context.Context, email, password, serial, region string) {
	err := s.runOnce(ctx, email, password, serial, region)
	if ctx.Err() == nil {
		s.mu.Lock()
		if err != nil {
			s.lastError = err.Error()
			if s.restartReason == "" {
				s.restartReason = "pipeline_error"
			}
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-time.After(restartBackoff):
		}
	}
	s.mu.Lock()
	s.running = false
	s.starting = false
	s.mu.Unlock()
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var idleTimeout = 30 * time.Second

const restartBackoff = 15 * time.Second

type Streamer struct {
	mu        sync.Mutex
	running   bool
	starting  bool
	lastError string
	restarts  int
	startedAt time.Time
	lastTouch time.Time
	cancel    context.CancelFunc
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
	s.lastTouch = time.Now()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
}

// Stop kills the pipeline and clears lastTouch so supervise will not
// restart until the next heartbeat (/start) or a share-token HLS request.
func (s *Streamer) Stop() {
	s.mu.Lock()
	s.lastTouch = time.Time{}
	if s.cancel != nil {
		log.Printf("streamer: viewer gone, stopping so the camera can sleep")
		s.cancel()
	}
	s.mu.Unlock()
}

func (s *Streamer) maybeStart() {
	if !s.configured() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running || s.starting || time.Since(s.lastTouch) >= idleTimeout {
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
	return cfg.Email != "" && cfg.Password != "" && cfg.Serial != ""
}

func (s *Streamer) creds() (email, password, serial, region string) {
	cfgMu.Lock()
	defer cfgMu.Unlock()
	return cfg.Email, cfg.Password, cfg.Serial, cfg.Region
}

func bridgeLogLevel() string {
	if strings.ToLower(cfgCopy().LogLevel) == "debug" {
		return "debug"
	}
	return "info"
}

func (s *Streamer) Status() (running, starting bool, lastError string, restarts int, startedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running, s.starting, s.lastError, s.restarts, s.startedAt
}

func (s *Streamer) supervise() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.maybeStart()
		s.mu.Lock()
		desired := s.configured() && time.Since(s.lastTouch) < idleTimeout
		switch {
		case !desired && (s.running || s.starting) && s.cancel != nil:
			log.Printf("streamer: idle %s, stopping so the camera can sleep", idleTimeout)
			s.cancel()
		case desired && s.running && s.cancel != nil:
			if time.Since(s.startedAt) >= 60*time.Second {
				if st, err := os.Stat(filepath.Join(*workDir, "hls", playlistName)); err == nil &&
					time.Since(st.ModTime()) > 12*time.Second {
					log.Printf("streamer: watchdog — segments stale for 12s, restarting pipeline")
					s.cancel()
				}
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

func (s *Streamer) runOnce(ctx context.Context, email, password, serial, region string) error {
	hlsDir := filepath.Join(*workDir, "hls")
	if err := os.MkdirAll(hlsDir, 0o755); err != nil {
		return err
	}
	clearHLS()
	defer clearHLS()

	bridge := exec.CommandContext(ctx, *bridgePath,
		"-region", region,
		"-deviceSerial", serial,
		"-maxStreamTime", "170",
		"-logLevel", bridgeLogLevel(),
		"-out=-", "-stdout=false", "-logFile=true")
	bridge.Dir = *workDir
	bridge.Env = append(os.Environ(),
		"EZVIZ_EMAIL="+email,
		"EZVIZ_PASSWORD="+password,
	)

	ff := exec.CommandContext(ctx, *ffmpegPath,
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "+genpts",
		"-f", "mpeg", "-probesize", "1000000",
		"-i", "pipe:0",
		"-map", "0:v:0", "-c:v", "copy", "-tag:v", "hvc1",
		"-bsf:v", "setts=pts=N/(15*TB):dts=N/(15*TB)",
		"-f", "hls", "-hls_time", "2", "-hls_list_size", "90",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_flags", "delete_segments+independent_segments+temp_file",
		filepath.Join(hlsDir, playlistName))
	ff.Dir = *workDir

	pipe, err := bridge.StdoutPipe()
	if err != nil {
		return err
	}
	ff.Stdin = pipe
	if f, err := os.OpenFile(filepath.Join(*workDir, "ffmpeg.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		defer f.Close()
		ff.Stderr = f
	}
	if f, err := os.OpenFile(filepath.Join(*workDir, "bridge.err"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		defer f.Close()
		bridge.Stderr = f
	}

	if err := bridge.Start(); err != nil {
		return fmt.Errorf("bridge start: %w", err)
	}
	if err := ff.Start(); err != nil {
		bridge.Process.Kill()
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	s.mu.Lock()
	s.running = true
	s.restarts++
	s.startedAt = time.Now()
	if ctx.Err() == nil {
		s.lastError = ""
	}
	s.mu.Unlock()
	log.Printf("streamer: started (run #%d)", s.restarts)

	done := make(chan error, 2)
	go func() { done <- bridge.Wait() }()
	go func() { done <- ff.Wait() }()

	var firstErr error
	got := 0
	select {
	case <-ctx.Done():
	case firstErr = <-done:
		got = 1
	}
	bridge.Process.Kill()
	ff.Process.Kill()
	waitBoth(done, got)
	if ctx.Err() != nil {
		return nil
	}
	if firstErr != nil {
		return fmt.Errorf("pipeline exited: %v", firstErr)
	}
	return fmt.Errorf("pipeline exited cleanly")
}

// waitBoth drains remaining Wait() results. already is how many were already
// received (0 if we bailed on ctx, 1 if one process exited first). Receiving
// twice after a single receive deadlocks: only two Wait goroutines send.
func waitBoth(done <-chan error, already int) {
	for already < 2 {
		<-done
		already++
	}
}

func hlsPlayable() bool {
	dir := hlsDir()
	st, err := os.Stat(filepath.Join(dir, playlistName))
	if err != nil || time.Since(st.ModTime()) > 15*time.Second {
		return false
	}
	if inf, err := os.Stat(filepath.Join(dir, "init.mp4")); err != nil || inf.Size() < 100 {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, playlistName))
	if err != nil {
		return false
	}
	n := 0
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if i := strings.IndexByte(ln, '?'); i >= 0 {
			ln = ln[:i]
		}
		if !strings.HasSuffix(ln, ".m4s") {
			continue
		}
		inf, err := os.Stat(filepath.Join(dir, filepath.Base(ln)))
		if err != nil || inf.Size() < 512 {
			continue
		}
		n++
	}
	return n >= 3
}

func waitForFile(path string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		running, starting, _, _, _ := streamer.Status()
		if !running && !starting {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func hlsDir() string { return filepath.Join(*workDir, "hls") }

func clearHLS() {
	dir := hlsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		os.Remove(filepath.Join(dir, e.Name()))
	}
}

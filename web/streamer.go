package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	hold      *exec.Cmd
	holdIn    io.WriteCloser
}

func NewStreamer() *Streamer { return &Streamer{} }

func fifoPath() string { return filepath.Join(*workDir, "stream.ps") }

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
	s.mu.Unlock()
}

func (s *Streamer) killHoldLocked() {
	if s.holdIn != nil {
		s.holdIn.Close()
		s.holdIn = nil
	}
	if s.hold != nil && s.hold.Process != nil {
		s.hold.Process.Kill()
	}
	s.hold = nil
}

func (s *Streamer) ensureHold(email, password, serial, region string) error {
	s.mu.Lock()
	if s.hold != nil && s.hold.Process != nil {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	fifo := fifoPath()
	_ = os.Remove(fifo)
	if err := syscall.Mkfifo(fifo, 0o600); err != nil && !os.IsExist(err) {
		return err
	}
	cmd := exec.Command(*bridgePath,
		"-region", region,
		"-deviceSerial", serial,
		"-maxStreamTime", "170",
		"-logLevel", bridgeLogLevel(),
		"-out", fifo,
		"-idleWait",
		"-stdout=false", "-logFile=true")
	cmd.Dir = *workDir
	cmd.Env = append(os.Environ(),
		"EZVIZ_EMAIL="+email,
		"EZVIZ_PASSWORD="+password,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var errFile *os.File
	if f, err := os.OpenFile(filepath.Join(*workDir, "bridge.err"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		cmd.Stderr = f
		errFile = f
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		if errFile != nil {
			errFile.Close()
		}
		return err
	}
	s.mu.Lock()
	if s.hold != nil {
		s.mu.Unlock()
		stdin.Close()
		cmd.Process.Kill()
		if errFile != nil {
			errFile.Close()
		}
		return nil
	}
	s.hold = cmd
	s.holdIn = stdin
	s.mu.Unlock()
	log.Printf("streamer: holding EZVIZ session")
	go func() {
		err := cmd.Wait()
		if errFile != nil {
			errFile.Close()
		}
		s.mu.Lock()
		if s.hold == cmd {
			s.hold = nil
			s.holdIn = nil
		}
		s.mu.Unlock()
		if err != nil {
			log.Printf("streamer: session process exited: %v", err)
		}
	}()
	return nil
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
	if alwaysOn() {
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
			log.Printf("streamer: idle %s, stopping so the camera can sleep", idleTimeout)
			s.cancel()
		case desired && s.running && s.cancel != nil:
			age := time.Since(s.startedAt)
			if age >= 20*time.Second && !hlsPlayable() {
				log.Printf("streamer: watchdog — no playable HLS after 20s, restarting pipeline")
				s.cancel()
			} else if age >= 60*time.Second {
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
	if err := s.ensureHold(email, password, serial, region); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	clearHLS()
	defer clearHLS()

	ff := exec.CommandContext(ctx, *ffmpegPath,
		"-hide_banner", "-loglevel", "warning",
		"-fflags", "+genpts+nobuffer", "-flags", "low_delay",
		"-probesize", "16384", "-analyzeduration", "0",
		"-f", "mpeg", "-i", fifoPath(),
		"-flush_packets", "1",
		"-map", "0:v:0", "-c:v", "copy", "-tag:v", "hvc1",
		"-bsf:v", "setts=pts=N/(15*TB):dts=N/(15*TB)",
		"-f", "hls", "-hls_time", "1", "-hls_init_time", "0.4", "-hls_list_size", "180",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_flags", "delete_segments+temp_file+split_by_time+omit_endlist",
		filepath.Join(hlsDir, playlistName))
	ff.Dir = *workDir
	if f, err := os.OpenFile(filepath.Join(*workDir, "ffmpeg.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		defer f.Close()
		ff.Stderr = f
	}
	if err := ff.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	s.mu.Lock()
	hold := s.hold
	holdIn := s.holdIn
	s.running = true
	s.restarts++
	s.startedAt = time.Now()
	if ctx.Err() == nil {
		s.lastError = ""
	}
	s.mu.Unlock()
	if holdIn != nil {
		if _, err := holdIn.Write([]byte("\n")); err != nil {
			ff.Process.Kill()
			return fmt.Errorf("session kick: %w", err)
		}
	}
	log.Printf("streamer: started (run #%d)", s.restarts)

	done := make(chan error, 1)
	go func() { done <- ff.Wait() }()
	select {
	case <-ctx.Done():
		if hold != nil && hold.Process != nil {
			_ = hold.Process.Signal(syscall.SIGUSR1)
		}
		ff.Process.Kill()
		<-done
		return nil
	case err := <-done:
		if hold != nil && hold.Process != nil {
			_ = hold.Process.Signal(syscall.SIGUSR1)
		}
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return fmt.Errorf("pipeline exited: %v", err)
		}
		return fmt.Errorf("pipeline exited cleanly")
	}
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
		if err != nil || inf.Size() < 100 {
			continue
		}
		n++
	}
	return n >= 1
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
		time.Sleep(50 * time.Millisecond)
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

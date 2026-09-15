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
	cmd := exec.Command(*bridgePath, bridgeArgv(region, serial,
		"-maxStreamTime", "170",
		"-out", fifo,
		"-idleWait",
		"-stdout=false", "-logFile=true")...)
	applyBridgeEnv(cmd, email, password)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var errFile *os.File
	if f, err := os.OpenFile(bridgeErrPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
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

type StreamStatus struct {
	Running   bool
	Starting  bool
	LastError string
	Restarts  int
	StartedAt time.Time
}

func (st StreamStatus) Active() bool { return st.Running || st.Starting }

func (s *Streamer) Status() StreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return StreamStatus{
		Running:   s.running,
		Starting:  s.starting,
		LastError: s.lastError,
		Restarts:  s.restarts,
		StartedAt: s.startedAt,
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
			log.Printf("streamer: idle %s, stopping so the camera can sleep", idleTimeout)
			s.cancel()
		case desired && s.running && s.cancel != nil:
			age := time.Since(s.startedAt)
			if age >= 20*time.Second && !hlsPlayable() {
				log.Printf("streamer: watchdog — no playable HLS after 20s, restarting pipeline")
				s.cancel()
			} else if age >= 60*time.Second {
				if st, err := os.Stat(playlistPath()); err == nil &&
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
	dir := hlsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
		"-probesize", "32768", "-analyzeduration", "200000",
		"-f", "mpeg", "-i", fifoPath(),
		"-flush_packets", "1",
		"-map", "0:v:0", "-c:v", "copy", "-an", "-tag:v", "hvc1",
		"-bsf:v", "setts=pts=N/(15*TB):dts=N/(15*TB)",
		"-f", "hls", "-hls_time", "1", "-hls_init_time", "0.5", "-hls_list_size", "180",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_flags", "delete_segments+temp_file+split_by_time",
		filepath.Join(dir, playlistName))
	ff.Dir = *workDir
	if f, err := os.OpenFile(ffmpegLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
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
	go func(run int, t0 time.Time) {
		if waitForPlayable(20 * time.Second) {
			log.Printf("streamer: first HLS after %s (run #%d)", time.Since(t0).Round(10*time.Millisecond), run)
		}
	}(s.restarts, s.startedAt)

	done := make(chan error, 1)
	go func() { done <- ff.Wait() }()
	select {
	case <-ctx.Done():
		signalHold(hold)
		ff.Process.Kill()
		<-done
		return nil
	case err := <-done:
		signalHold(hold)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return fmt.Errorf("pipeline exited: %v", err)
		}
		return fmt.Errorf("pipeline exited cleanly")
	}
}

func signalHold(hold *exec.Cmd) {
	if hold != nil && hold.Process != nil {
		_ = hold.Process.Signal(syscall.SIGUSR1)
	}
}

func playlistSegments(data []byte) []string {
	var segs []string
	for _, ln := range strings.Split(string(data), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		if i := strings.IndexByte(ln, '?'); i >= 0 {
			ln = ln[:i]
		}
		name := filepath.Base(ln)
		if strings.HasSuffix(name, ".m4s") {
			segs = append(segs, name)
		}
	}
	return segs
}

const hlsMinFile = 100

func hlsFileReady(path string) bool {
	inf, err := os.Stat(path)
	return err == nil && inf.Size() >= hlsMinFile
}

func hlsPlayable() bool {
	st, err := os.Stat(playlistPath())
	if err != nil || time.Since(st.ModTime()) > 15*time.Second {
		return false
	}
	if !hlsFileReady(hlsFile("init.mp4")) {
		return false
	}
	data, err := os.ReadFile(playlistPath())
	if err != nil {
		return false
	}
	for _, name := range playlistSegments(data) {
		if hlsFileReady(hlsFile(name)) {
			return true
		}
	}
	return false
}

func waitWhileActive(d time.Duration, ready func() bool) bool {
	deadline := time.Now().Add(d)
	for {
		if ready() {
			return true
		}
		if !time.Now().Before(deadline) || !streamer.Status().Active() {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitForPlayable(d time.Duration) bool {
	return waitWhileActive(d, hlsPlayable)
}

func waitForFile(path string, d time.Duration) bool {
	return waitWhileActive(d, func() bool {
		return hlsFileReady(path)
	})
}

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

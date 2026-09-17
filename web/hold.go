package main

import (
	"io"
	"log"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ensureHold keeps a logged-in le-ezviz-vs process with -idleWait and a FIFO.
// Kick() kills the hold so the next ensureHold uses the current active serial.
func (s *Streamer) ensureHold(email, password, serial, region string) error {
	s.mu.Lock()
	if s.hold != nil && s.hold.Process != nil && s.holdSerial == serial {
		s.mu.Unlock()
		return nil
	}
	if s.hold != nil && s.holdSerial != serial {
		log.Printf("streamer: hold serial changed %s → %s, recycling session", s.holdSerial, serial)
		s.killHoldLocked()
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
		"-idleWait")...)
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
	s.holdSerial = serial
	s.mu.Unlock()
	log.Printf("streamer: holding EZVIZ session (%s)", serial)
	go func() {
		err := cmd.Wait()
		if errFile != nil {
			errFile.Close()
		}
		s.mu.Lock()
		if s.hold == cmd {
			s.hold = nil
			s.holdIn = nil
			s.holdSerial = ""
		}
		s.mu.Unlock()
		if err != nil {
			log.Printf("streamer: session process exited: %v", err)
		}
	}()
	return nil
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
	s.holdSerial = ""
}

func signalHold(hold *exec.Cmd) {
	if hold != nil && hold.Process != nil {
		_ = hold.Process.Signal(syscall.SIGUSR1)
	}
}

// kickHoldStream writes a newline to start VTDU after hold is ready.
func kickHoldStream(holdIn io.WriteCloser) error {
	if holdIn == nil {
		return io.ErrClosedPipe
	}
	_, err := holdIn.Write([]byte("\n"))
	return err
}

func waitForMedia(d time.Duration) bool {
	deadline := time.Now().Add(d)
	for {
		if rawRingFresh(2 * time.Second) {
			return true
		}
		if !time.Now().Before(deadline) || !streamer.Status().Active() {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

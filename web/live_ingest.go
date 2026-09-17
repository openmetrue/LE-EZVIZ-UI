package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
)

// ingestFIFO copies the camera MPEG-PS into the Save ring and demuxes HEVC
// with gomedia for Live fMP4. No ffmpeg on this path.

func ingestFIFO(ctx context.Context) {
	live.reset()
	f, err := os.OpenFile(fifoPath(), os.O_RDONLY, 0)
	if err != nil {
		log.Printf("streamer: fifo: %v", err)
		return
	}
	defer f.Close()

	demuxer := newLiveDemuxer()
	buf := make([]byte, 32<<10)
	for {
		if ctx.Err() != nil {
			live.flush()
			return
		}
		n, err := f.Read(buf)
		if n > 0 {
			rawRingPush(buf[:n])
			feedPS(&demuxer, buf[:n])
		}
		if err != nil {
			live.flush()
			return
		}
	}
}

func (s *Streamer) runOnce(ctx context.Context, email, password, serial, region string) error {
	if err := s.ensureHold(email, password, serial, region); err != nil {
		return fmt.Errorf("session: %w", err)
	}
	rawRingClear()

	s.mu.Lock()
	hold := s.hold
	holdIn := s.holdIn
	holdOK := holdIn != nil && s.holdSerial == serial
	if !holdOK {
		s.mu.Unlock()
		return fmt.Errorf("session: hold not ready for %s", serial)
	}
	s.running = true
	s.restarts++
	s.startedAt = time.Now()
	if ctx.Err() == nil {
		s.lastError = ""
	}
	run := s.restarts
	s.mu.Unlock()

	if err := kickHoldStream(holdIn); err != nil {
		return fmt.Errorf("session kick: %w", err)
	}
	log.Printf("streamer: started (run #%d)", run)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ingestFIFO(ctx)
	}()
	go func(run int, t0 time.Time) {
		if waitForMedia(20 * time.Second) {
			log.Printf("streamer: first media after %s (run #%d)", time.Since(t0).Round(10*time.Millisecond), run)
		}
		deadline := time.Now().Add(25 * time.Second)
		for {
			if liveReady() {
				log.Printf("streamer: hevc ready after %s (run #%d)",
					time.Since(t0).Round(10*time.Millisecond), run)
				return
			}
			if !time.Now().Before(deadline) || !streamer.Status().Active() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}(run, time.Now())

	select {
	case <-ctx.Done():
		signalHold(hold)
		<-done
		return nil
	case <-done:
		signalHold(hold)
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("pipeline exited")
	}
}

package main

import (
	"errors"
	"io"
	"log"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
)

// LivePub is the single H.264 source shared by all WebRTC viewers.
// Inter frames are gated until an encoder IDR so late joiners never
// start mid-GOP (that produces a permanent gray picture in Safari).
type LivePub struct {
	mu sync.Mutex

	track *webrtc.TrackLocalStaticSample
	sps   []byte
	pps   []byte

	// needKey drops non-IDR samples until the next encoder keyframe.
	needKey bool
	lastKey []byte

	lastPLI     time.Time
	lastSample  time.Time
	ready       bool
	lastWriteAt time.Time
}

var live = &LivePub{needKey: true}

func rtcEnabled() bool { return rtcAPI != nil }

func rtcPlayable() bool { return live.Playable() }

func rtcFresh(maxAge time.Duration) bool { return live.Fresh(maxAge) }

func rtcEnsureTrack() *webrtc.TrackLocalStaticSample { return live.EnsureTrack() }

func rtcDropTrack() { live.Reset() }

func rtcRequestIDR() { live.RequestKey() }

func (p *LivePub) Playable() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ready
}

func (p *LivePub) Fresh(maxAge time.Duration) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ready && !p.lastSample.IsZero() && time.Since(p.lastSample) <= maxAge
}

func (p *LivePub) RequestKey() {
	p.mu.Lock()
	p.needKey = true
	p.mu.Unlock()
}

func (p *LivePub) Reset() {
	p.mu.Lock()
	p.track = nil
	p.sps, p.pps = nil, nil
	p.needKey = true
	p.lastKey = nil
	p.ready = false
	p.lastSample = time.Time{}
	p.lastWriteAt = time.Time{}
	p.mu.Unlock()
	rawRingClear()
}

func (p *LivePub) EnsureTrack() *webrtc.TrackLocalStaticSample {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ensureTrackLocked()
}

func (p *LivePub) ensureTrackLocked() *webrtc.TrackLocalStaticSample {
	if p.track != nil {
		return p.track
	}
	t, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeH264,
			ClockRate:    90000,
			SDPFmtpLine:  h264Fmtp,
			RTCPFeedback: videoRTCPFeedback(),
		},
		"video", "ezvizd",
	)
	if err != nil {
		log.Printf("webrtc: track: %v", err)
		return nil
	}
	p.track = t
	p.needKey = true
	log.Printf("webrtc: H264 track ready")
	return t
}

// BeginViewer must run before AddTrack. It arms keyframe gating and
// returns the shared track plus a keyframe to seed the new decoder.
func (p *LivePub) BeginViewer() (track *webrtc.TrackLocalStaticSample, seed []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.needKey = true
	track = p.ensureTrackLocked()
	if len(p.lastKey) > 0 {
		seed = append([]byte(nil), p.lastKey...)
	}
	return track, seed
}

// SeedViewer writes a prior IDR so the browser has a picture immediately,
// while keeping needKey set so P-frames cannot follow until a fresh IDR.
func (p *LivePub) SeedViewer(track *webrtc.TrackLocalStaticSample, seed []byte) {
	if track == nil || len(seed) == 0 {
		return
	}
	p.mu.Lock()
	p.needKey = true
	p.mu.Unlock()
	_ = track.WriteSample(media.Sample{Data: seed, Duration: time.Second / 15})
}

// OnPLI handles Picture Loss / FIR: re-seed the last key and wait for a
// fresh encoder IDR. Unlike the old path, a pending needKey does not
// suppress the seed — that left Safari stuck on gray after a late join.
func (p *LivePub) OnPLI(track *webrtc.TrackLocalStaticSample) {
	if track == nil {
		return
	}
	p.mu.Lock()
	if time.Since(p.lastPLI) < 150*time.Millisecond {
		p.mu.Unlock()
		return
	}
	p.lastPLI = time.Now()
	p.needKey = true
	seed := append([]byte(nil), p.lastKey...)
	p.mu.Unlock()
	log.Printf("webrtc: PLI/FIR — seed IDR, wait fresh keyframe")
	if len(seed) > 0 {
		_ = track.WriteSample(media.Sample{Data: seed, Duration: time.Second / 15})
	}
}

func (p *LivePub) setParamSet(kind h264reader.NalUnitType, data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := append([]byte(nil), data...)
	switch kind {
	case h264reader.NalUnitTypeSPS:
		p.sps = cp
	case h264reader.NalUnitTypePPS:
		p.pps = cp
	}
}

func (p *LivePub) writeAU(au []byte, idr bool) error {
	p.mu.Lock()
	if p.needKey && !idr {
		p.mu.Unlock()
		return nil
	}
	track := p.track
	now := time.Now()
	dur := 66 * time.Millisecond
	if !p.lastWriteAt.IsZero() {
		if d := now.Sub(p.lastWriteAt); d >= 33*time.Millisecond && d <= 120*time.Millisecond {
			dur = d
		}
	}
	p.lastWriteAt = now
	p.lastSample = now
	if idr {
		p.needKey = false
		p.ready = true
		p.lastKey = append([]byte(nil), au...)
	}
	p.mu.Unlock()

	if track == nil || len(au) == 0 {
		return nil
	}
	if err := track.WriteSample(media.Sample{Data: au, Duration: dur}); err != nil {
		if errors.Is(err, io.ErrClosedPipe) {
			return nil
		}
		return err
	}
	return nil
}

func pumpH264(r io.Reader) {
	hr, err := h264reader.NewReader(r)
	if err != nil {
		log.Printf("webrtc: h264 reader: %v", err)
		return
	}
	logged := false
	for {
		nal, err := hr.NextNAL()
		if err != nil {
			return
		}
		switch nal.UnitType {
		case h264reader.NalUnitTypeSPS, h264reader.NalUnitTypePPS:
			live.setParamSet(nal.UnitType, nal.Data)
			live.EnsureTrack()
			continue
		case h264reader.NalUnitTypeSEI, h264reader.NalUnitTypeAUD,
			h264reader.NalUnitTypeFiller, h264reader.NalUnitTypeEndOfSequence,
			h264reader.NalUnitTypeEndOfStream:
			continue
		}
		if nal.UnitType != h264reader.NalUnitTypeCodedSliceIdr &&
			nal.UnitType != h264reader.NalUnitTypeCodedSliceNonIdr &&
			nal.UnitType != h264reader.NalUnitTypeCodedSliceAux {
			continue
		}
		live.EnsureTrack()
		idr := nal.UnitType == h264reader.NalUnitTypeCodedSliceIdr
		var au []byte
		if idr {
			live.mu.Lock()
			sps, pps := live.sps, live.pps
			live.mu.Unlock()
			au = annexBNAL(sps, pps, nal.Data)
		} else {
			au = annexBNAL(nal.Data)
		}
		if err := live.writeAU(au, idr); err != nil {
			return
		}
		if !logged {
			logged = true
			log.Printf("webrtc: first H264 sample type=%s bytes=%d", nal.UnitType.String(), len(au))
		}
	}
}

func annexBNAL(nals ...[]byte) []byte {
	var out []byte
	start := []byte{0x00, 0x00, 0x00, 0x01}
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		out = append(out, start...)
		out = append(out, n...)
	}
	return out
}

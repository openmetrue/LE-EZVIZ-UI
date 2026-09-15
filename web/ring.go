package main

import (
	"sync"
	"time"
)

const ringKeep = 180 * time.Second

type h264Chunk struct {
	t    time.Time
	data []byte
	idr  bool
}

var (
	ringMu       sync.Mutex
	ringChunks   []h264Chunk
	rtcReady     bool
	lastSampleAt time.Time
)

func ringPush(au []byte, idr bool) {
	if len(au) == 0 {
		return
	}
	ringMu.Lock()
	defer ringMu.Unlock()
	now := time.Now()
	ringChunks = append(ringChunks, h264Chunk{
		t:    now,
		data: append([]byte(nil), au...),
		idr:  idr,
	})
	lastSampleAt = now
	if idr {
		rtcReady = true
	}
	cutoff := now.Add(-ringKeep)
	i := 0
	for i < len(ringChunks) && ringChunks[i].t.Before(cutoff) {
		i++
	}
	for i > 0 && i < len(ringChunks) && !ringChunks[i].idr {
		i--
	}
	if i > 0 {
		ringChunks = append([]h264Chunk(nil), ringChunks[i:]...)
	}
}

func ringClear() {
	ringMu.Lock()
	ringChunks = nil
	rtcReady = false
	lastSampleAt = time.Time{}
	ringMu.Unlock()
}

func rtcPlayable() bool {
	ringMu.Lock()
	defer ringMu.Unlock()
	return rtcReady
}

func rtcFresh(maxAge time.Duration) bool {
	ringMu.Lock()
	defer ringMu.Unlock()
	return rtcReady && !lastSampleAt.IsZero() && time.Since(lastSampleAt) <= maxAge
}

// ringSnapshot returns annex-B from the oldest retained IDR through now.
// ok is false when there is no IDR yet.
func ringSnapshot() (data []byte, ok bool) {
	ringMu.Lock()
	defer ringMu.Unlock()
	start := -1
	for i, c := range ringChunks {
		if c.idr {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	var out []byte
	for _, c := range ringChunks[start:] {
		out = append(out, c.data...)
	}
	return out, len(out) > 0
}

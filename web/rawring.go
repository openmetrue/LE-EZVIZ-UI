package main

import (
	"sync"
	"time"
)

// rawRing keeps ~ringKeep of the camera MPEG-PS (HEVC 1080p) for Save,
// separate from the downscaled H.264 used for WebRTC.
type rawChunk struct {
	t    time.Time
	data []byte
}

var (
	rawMu     sync.Mutex
	rawChunks []rawChunk
)

func rawRingPush(p []byte) {
	if len(p) == 0 {
		return
	}
	rawMu.Lock()
	defer rawMu.Unlock()
	now := time.Now()
	rawChunks = append(rawChunks, rawChunk{
		t:    now,
		data: append([]byte(nil), p...),
	})
	cutoff := now.Add(-ringKeep)
	i := 0
	for i < len(rawChunks) && rawChunks[i].t.Before(cutoff) {
		i++
	}
	if i > 0 {
		rawChunks = append([]rawChunk(nil), rawChunks[i:]...)
	}
}

func rawRingClear() {
	rawMu.Lock()
	rawChunks = nil
	rawMu.Unlock()
}

// rawRingSnapshot returns MPEG-PS bytes aligned to a pack header when possible.
func rawRingSnapshot() (data []byte, ok bool) {
	rawMu.Lock()
	defer rawMu.Unlock()
	if len(rawChunks) == 0 {
		return nil, false
	}
	var out []byte
	for _, c := range rawChunks {
		out = append(out, c.data...)
	}
	if i := indexMPEGPack(out); i >= 0 {
		out = out[i:]
	}
	return out, len(out) > 0
}

func indexMPEGPack(b []byte) int {
	for i := 0; i+3 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 1 && b[i+3] == 0xba {
			return i
		}
	}
	return -1
}

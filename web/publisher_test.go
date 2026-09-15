package main

import (
	"testing"
	"time"
)

func TestLivePubGatingDropsPUntilIDR(t *testing.T) {
	p := &LivePub{needKey: true}
	if p.Playable() {
		t.Fatal("not playable before IDR")
	}
	if err := p.writeAU([]byte{0x00, 0x00, 0x00, 0x01, 0x01}, false); err != nil {
		t.Fatal(err)
	}
	if p.Playable() {
		t.Fatal("P-frame must not mark ready")
	}
	if len(p.lastKey) != 0 {
		t.Fatal("P-frame must not become lastKey")
	}
	idr := []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0xaa}
	if err := p.writeAU(idr, true); err != nil {
		t.Fatal(err)
	}
	if !p.Playable() || !p.Fresh(time.Second) {
		t.Fatal("IDR should mark ready")
	}
	if len(p.lastKey) != len(idr) {
		t.Fatalf("lastKey len=%d", len(p.lastKey))
	}
	p.RequestKey()
	p.mu.Lock()
	need := p.needKey
	p.mu.Unlock()
	if !need {
		t.Fatal("RequestKey should arm gating")
	}
	if err := p.writeAU([]byte{0x00, 0x00, 0x00, 0x01, 0x01}, false); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	still := p.needKey
	p.mu.Unlock()
	if !still {
		t.Fatal("P-frame must not clear needKey")
	}
	if err := p.writeAU([]byte{0x00, 0x00, 0x00, 0x01, 0x65, 0xbb}, true); err != nil {
		t.Fatal(err)
	}
	p.mu.Lock()
	cleared := !p.needKey
	p.mu.Unlock()
	if !cleared {
		t.Fatal("encoder IDR must clear needKey")
	}
}

func TestBeginViewerArmsBeforeSeed(t *testing.T) {
	p := &LivePub{needKey: false}
	p.lastKey = []byte{0x00, 0x00, 0x00, 0x01, 0x65}
	_, seed := p.BeginViewer()
	if len(seed) == 0 {
		t.Fatal("expected seed from lastKey")
	}
	p.mu.Lock()
	need := p.needKey
	p.mu.Unlock()
	if !need {
		t.Fatal("BeginViewer must arm needKey before AddTrack")
	}
}

func TestResetClearsReady(t *testing.T) {
	p := &LivePub{needKey: true}
	_ = p.writeAU([]byte{0x00, 0x00, 0x00, 0x01, 0x65}, true)
	if !p.Playable() {
		t.Fatal("expected ready")
	}
	p.Reset()
	if p.Playable() || len(p.lastKey) != 0 {
		t.Fatal("Reset must clear playhead")
	}
}

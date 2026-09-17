package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Eyevinn/mp4ff/hevc"
	"github.com/Eyevinn/mp4ff/mp4"
	"github.com/yapingcat/gomedia/go-codec"
	"github.com/yapingcat/gomedia/go-mpeg2"
)

const (
	liveDefaultDur = 20 * time.Millisecond
	liveTimescale  = 90000
	liveFragHold   = 60 * time.Millisecond
	livePTSMax     = 250 * time.Millisecond
	liveGOPMaxAU   = 120
	livePeerBuf    = 256
)

type liveSample struct {
	data []byte
	dur  uint32
	sync bool
}

type liveAU struct {
	data []byte
	pts  uint64 // gomedia OnFrame: milliseconds (PES PTS / 90)
}

type livePeer struct {
	ch   chan []byte
	once sync.Once
}

type liveHub struct {
	mu           sync.Mutex
	ready        bool
	peers        map[int]*livePeer
	next         int
	initSeg      []byte
	codec        string
	gop          [][]byte
	seq          uint32
	dts          uint64
	lastSend     time.Time
	lastPTS      uint64
	pending      []liveSample
	pendingSince time.Time
	au           []byte
	auPTS        uint64
	vps          []byte
	sps          []byte
	pps          []byte
}

var live = &liveHub{peers: map[int]*livePeer{}}

// liveEdgeMarker follows the pre-attach GOP a new peer receives. It lets the
// player jump to the true live edge as soon as the backlog is fully buffered
// instead of guessing from buffer growth. Exactly four bytes: a valid fMP4 box
// header is never that short, so it cannot collide with a fragment.
var liveEdgeMarker = []byte("EDGE")

func liveReady() bool   { return live.isReady() }
func liveViewers() int  { return live.viewers() }
func liveCodec() string { return live.codecString() }

func (h *liveHub) isReady() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.ready
}

func (h *liveHub) viewers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.peers)
}

func (h *liveHub) codecString() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.codec
}

func (h *liveHub) reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = false
	h.initSeg = nil
	h.codec = ""
	h.gop = nil
	h.seq = 0
	h.dts = 0
	h.lastSend = time.Time{}
	h.lastPTS = 0
	h.pending = nil
	h.pendingSince = time.Time{}
	h.au = nil
	h.auPTS = 0
	h.vps, h.sps, h.pps = nil, nil, nil
}

func (p *livePeer) close() {
	if p == nil {
		return
	}
	p.once.Do(func() { close(p.ch) })
}

func (h *liveHub) addPeer() (id int, ch <-chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	id = h.next
	p := &livePeer{ch: make(chan []byte, livePeerBuf)}
	h.peers[id] = p
	if len(h.initSeg) > 0 {
		h.sendLocked(p, h.initSeg)
		for _, frag := range h.gop {
			h.sendLocked(p, frag)
		}
		h.sendLocked(p, liveEdgeMarker)
	}
	return id, p.ch
}

// sendLocked enqueues a copy to one peer. It returns false when the peer is too
// slow, so the caller can drop it.
func (h *liveHub) sendLocked(p *livePeer, msg []byte) bool {
	if p == nil || len(msg) == 0 {
		return true
	}
	select {
	case p.ch <- append([]byte(nil), msg...):
		return true
	default:
		return false
	}
}

func (h *liveHub) removePeer(id int) {
	h.mu.Lock()
	p, ok := h.peers[id]
	if ok {
		delete(h.peers, id)
	}
	h.mu.Unlock()
	if ok {
		p.close()
	}
}

func (h *liveHub) pushNAL(nal []byte, pts uint64) {
	if len(nal) == 0 {
		return
	}
	h.mu.Lock()
	h.noteParamsLocked(nal)
	if len(h.au) > 0 && pts != h.auPTS {
		au := h.takeAULocked()
		h.mu.Unlock()
		h.emitAU(au)
		h.mu.Lock()
	}
	h.au = append(h.au, nal...)
	h.auPTS = pts
	h.mu.Unlock()
}

func (h *liveHub) flush() {
	h.mu.Lock()
	au := h.takeAULocked()
	h.mu.Unlock()
	h.emitAU(au)
	h.mu.Lock()
	h.flushPendingLocked()
	h.mu.Unlock()
}

func (h *liveHub) takeAULocked() liveAU {
	if len(h.au) == 0 {
		return liveAU{}
	}
	out := liveAU{data: append([]byte(nil), h.au...), pts: h.auPTS}
	h.au = h.au[:0]
	return out
}

func (h *liveHub) emitAU(au liveAU) {
	if len(au.data) == 0 || !hevcHasVCL(au.data) {
		return
	}
	irap := hevcHasIRAP(au.data)
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.ready && !irap {
		return
	}
	now := time.Now()
	if irap && !h.ready {
		initSeg, codec, err := encodeLiveInit(h.vps, h.sps, h.pps)
		if err != nil {
			log.Printf("live: init: %v", err)
			return
		}
		h.initSeg = initSeg
		h.codec = codec
		h.ready = true
		log.Printf("streamer: hevc ready")
		h.broadcastLocked(initSeg)
	}
	if !h.ready {
		return
	}
	sample := liveSampleBytes(au.data)
	if len(sample) == 0 {
		return
	}
	dur := liveTicks(liveSampleDuration(h.lastPTS, au.pts, h.lastSend, now))
	h.lastPTS = au.pts
	h.lastSend = now
	if irap {
		h.flushPendingLocked()
	}
	h.pending = append(h.pending, liveSample{data: sample, dur: dur, sync: irap})
	if h.pendingSince.IsZero() {
		h.pendingSince = now
	}
	if shouldFlushLive(h.pending, h.pendingSince, now) {
		h.flushPendingLocked()
	}
}

func shouldFlushLive(pending []liveSample, since time.Time, now time.Time) bool {
	if len(pending) == 0 || since.IsZero() {
		return false
	}
	wait := now.Sub(since)
	if len(pending) >= 2 && wait >= 25*time.Millisecond {
		return true
	}
	return wait >= liveFragHold
}

func (h *liveHub) flushPendingLocked() {
	if len(h.pending) == 0 {
		return
	}
	h.seq++
	frag, err := encodeLiveFrag(h.seq, h.dts, h.pending)
	if err != nil {
		log.Printf("live: fragment: %v", err)
		h.pending = nil
		h.pendingSince = time.Time{}
		return
	}
	for _, s := range h.pending {
		h.dts += uint64(s.dur)
	}
	irap := h.pending[0].sync
	h.pending = nil
	h.pendingSince = time.Time{}
	h.gop = appendLiveGOP(h.gop, frag, irap)
	h.broadcastLocked(frag)
}

func (h *liveHub) broadcastLocked(msg []byte) {
	if len(msg) == 0 {
		return
	}
	var slow []int
	for id, p := range h.peers {
		if !h.sendLocked(p, msg) {
			slow = append(slow, id)
		}
	}
	for _, id := range slow {
		log.Printf("live: subscriber overflow, disconnect")
		if p := h.peers[id]; p != nil {
			delete(h.peers, id)
			p.close()
		}
	}
}

func (h *liveHub) noteParamsLocked(au []byte) {
	codec.SplitFrame(au, func(nal []byte) bool {
		switch hevcNALType(nal) {
		case codec.H265_NAL_VPS:
			h.vps = withStartCode(nal)
		case codec.H265_NAL_SPS:
			h.sps = withStartCode(nal)
		case codec.H265_NAL_PPS:
			h.pps = withStartCode(nal)
		}
		return true
	})
}

func liveFrameDuration(last, now time.Time) time.Duration {
	if last.IsZero() || now.Before(last) {
		return liveDefaultDur
	}
	d := now.Sub(last)
	if d < time.Millisecond {
		return time.Millisecond
	}
	return d
}

// liveSampleDuration uses gomedia millisecond PTS (PES 90 kHz / 90).
func liveSampleDuration(prevPTS, pts uint64, last, now time.Time) time.Duration {
	if prevPTS != 0 && pts > prevPTS {
		d := time.Duration(pts-prevPTS) * time.Millisecond
		if d >= time.Millisecond && d <= livePTSMax {
			return d
		}
	}
	return liveFrameDuration(last, now)
}

func liveTicks(d time.Duration) uint32 {
	t := uint32((d * liveTimescale) / time.Second)
	if t < 90 {
		return 90
	}
	return t
}

func encodeLiveInit(vps, sps, pps []byte) ([]byte, string, error) {
	vpsN := nalWithoutStartCode(vps)
	spsN := nalWithoutStartCode(sps)
	ppsN := nalWithoutStartCode(pps)
	if len(vpsN) == 0 || len(spsN) == 0 || len(ppsN) == 0 {
		return nil, "", errLive("missing VPS/SPS/PPS")
	}
	init := mp4.CreateEmptyInit()
	init.AddEmptyTrack(liveTimescale, "video", "und")
	if err := init.Moov.Trak.SetHEVCDescriptor("hvc1", [][]byte{vpsN}, [][]byte{spsN}, [][]byte{ppsN}, nil, true); err != nil {
		return nil, "", err
	}
	if err := init.TweakSingleTrakLive(); err != nil {
		return nil, "", err
	}
	buf := &bytes.Buffer{}
	if err := init.Encode(buf); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), hevcRFC6381(spsN), nil
}

func encodeLiveFrag(seq uint32, dts uint64, samples []liveSample) ([]byte, error) {
	frag, err := mp4.CreateFragment(seq, mp4.DefaultTrakID)
	if err != nil {
		return nil, err
	}
	t := dts
	for _, s := range samples {
		flags := mp4.NonSyncSampleFlags
		if s.sync {
			flags = mp4.SyncSampleFlags
		}
		frag.AddFullSample(mp4.FullSample{
			Sample: mp4.Sample{
				Flags: flags,
				Dur:   s.dur,
				Size:  uint32(len(s.data)),
			},
			DecodeTime: t,
			Data:       s.data,
		})
		t += uint64(s.dur)
	}
	buf := &bytes.Buffer{}
	if err := frag.Encode(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func hevcRFC6381(spsN []byte) string {
	sps, err := hevc.ParseSPSNALUnit(spsN)
	if err != nil {
		return "hvc1.1.6.L120.B0"
	}
	p := sps.ProfileTierLevel
	profile := fmt.Sprintf("%d", p.GeneralProfileIDC)
	if p.GeneralProfileSpace > 0 {
		profile = fmt.Sprintf("%c%d", 'A'+p.GeneralProfileSpace-1, p.GeneralProfileIDC)
	}
	tier := "L"
	if p.GeneralTierFlag {
		tier = "H"
	}
	compat := reverseBits32(p.GeneralProfileCompatibilityFlags)
	ci0 := byte(p.GeneralConstraintIndicatorFlags >> 40)
	return fmt.Sprintf("hvc1.%s.%X.%s%d.%02X", profile, compat, tier, p.GeneralLevelIDC, ci0)
}

func reverseBits32(x uint32) uint32 {
	var y uint32
	for i := 0; i < 32; i++ {
		y <<= 1
		y |= x & 1
		x >>= 1
	}
	return y
}

type errLive string

func (e errLive) Error() string { return string(e) }

func withStartCode(nal []byte) []byte {
	raw := nalWithoutStartCode(nal)
	out := make([]byte, 0, 4+len(raw))
	out = append(out, 0, 0, 0, 1)
	return append(out, raw...)
}

func nalWithoutStartCode(nal []byte) []byte {
	if len(nal) == 0 {
		return nil
	}
	start, sc := codec.FindStartCode(nal, 0)
	if start < 0 {
		return append([]byte(nil), nal...)
	}
	return append([]byte(nil), nal[start+int(sc):]...)
}

func liveSampleBytes(au []byte) []byte {
	return lengthPrefixed(au, true)
}

func lengthPrefixed(au []byte, vclOnly bool) []byte {
	var out []byte
	codec.SplitFrame(au, func(nal []byte) bool {
		if len(nal) == 0 {
			return true
		}
		if vclOnly && !keepLiveNAL(hevcNALType(nal)) {
			return true
		}
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(nal)))
		out = append(out, hdr[:]...)
		out = append(out, nal...)
		return true
	})
	return out
}

func keepLiveNAL(t codec.H265_NAL_TYPE) bool {
	return t <= codec.H265_NAL_SLICE_CRA || t == 39 || t == 40
}

func appendLiveGOP(gop [][]byte, frag []byte, irap bool) [][]byte {
	cp := append([]byte(nil), frag...)
	if irap {
		return [][]byte{cp}
	}
	if len(gop) == 0 || len(gop) >= liveGOPMaxAU {
		return gop
	}
	return append(gop, cp)
}

func hevcHasIRAP(au []byte) bool {
	return hevcHasTypeRange(au, codec.H265_NAL_SLICE_BLA_W_LP, codec.H265_NAL_SLICE_CRA)
}

func hevcHasVCL(au []byte) bool {
	return hevcHasTypeRange(au, 0, codec.H265_NAL_SLICE_CRA)
}

func hevcNALType(nal []byte) codec.H265_NAL_TYPE {
	if len(nal) == 0 {
		return 0
	}
	start, sc := codec.FindStartCode(nal, 0)
	if start >= 0 {
		i := start + int(sc)
		if i < len(nal) {
			return codec.H265NaluTypeWithoutStartCode(nal[i:])
		}
		return 0
	}
	return codec.H265NaluTypeWithoutStartCode(nal)
}

func hevcHasTypeRange(au []byte, lo, hi codec.H265_NAL_TYPE) bool {
	found := false
	n := 0
	codec.SplitFrame(au, func(nal []byte) bool {
		n++
		t := hevcNALType(nal)
		if t >= lo && t <= hi {
			found = true
			return false
		}
		return true
	})
	if n == 0 && len(au) > 0 {
		t := hevcNALType(au)
		return t >= lo && t <= hi
	}
	return found
}

func newLiveDemuxer() *mpeg2.PSDemuxer {
	d := mpeg2.NewPSDemuxer()
	d.OnFrame = func(frame []byte, cid mpeg2.PS_STREAM_TYPE, pts, dts uint64) {
		if cid != mpeg2.PS_STREAM_H265 || len(frame) == 0 {
			return
		}
		live.pushNAL(append([]byte(nil), frame...), pts)
	}
	return d
}

func feedPS(d **mpeg2.PSDemuxer, data []byte) {
	defer func() {
		if recover() != nil {
			*d = newLiveDemuxer()
		}
	}()
	_ = (*d).Input(data)
}

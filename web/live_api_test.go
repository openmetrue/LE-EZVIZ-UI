package main

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yapingcat/gomedia/go-codec"
)

func TestStatusContractKeys(t *testing.T) {
	orig := streamer
	streamer = NewStreamer()
	t.Cleanup(func() { streamer = orig })
	statusPollMu.Lock()
	statusPollBusy = true
	statusPollMu.Unlock()
	t.Cleanup(func() {
		statusPollMu.Lock()
		statusPollBusy = false
		statusPollMu.Unlock()
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ezviz/api/status", nil)
	handleStatus(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{
		"ready", "running", "starting", "restarts", "configured",
		"last_error", "stream_mode", "device", "active_serial", "viewers", "restart_reason",
	} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing key %s", k)
		}
	}
	if m["ready"] != false {
		t.Errorf("ready=%v want false without HEVC IRAP", m["ready"])
	}
}

func TestLiveRejectsPOST(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ezviz/live", strings.NewReader("{}"))
	handleLive(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestStaticLivePlayerJS(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/ezviz/static/", http.StripPrefix("/ezviz/static/", staticHandler()))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ezviz/static/live-player.js", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"LivePlayer", "notifyDeviceSwitch", "/live", "ManagedMediaSource",
		"MediaSource", "appendBuffer", "addSourceBuffer", "fresh=1",
		"waiting", "stalled", "playsinline", "hvc1", "Infinity", "sequence", "ftyp",
		"goLive", "govern", "seekToEdge", "playbackRate", "LIVE_SEEK_TARGET",
		"currentTime", "isLiveEdge", "fetchWithTimeout",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("player js missing %q", want)
		}
	}
	if strings.Contains(body, "playRaw") || strings.Contains(body, "live.mp4") {
		t.Fatal("Live is MediaSource only; the raw /live.mp4 fallback was removed")
	}
	if strings.Contains(body, "preferH264") {
		t.Fatal("Live is HEVC; player must not prefer H.264")
	}
	if strings.Contains(body, "86400") {
		t.Fatal("fake 24h MediaSource duration turns Live into a recording")
	}
	if strings.Contains(body, "RTCPeerConnection") || strings.Contains(body, "/webrtc") {
		t.Fatal("Live must not use WebRTC")
	}
	if strings.Contains(body, "attachClock") || strings.Contains(body, "/share") {
		t.Fatal("share-only player hacks must not be in Live")
	}
	if strings.Contains(body, "armClock") {
		t.Fatal("adding the clock after the first frame freezes Safari on that picture")
	}
	if strings.Contains(body, "startstreaming") {
		t.Fatal("waiting for Managed Media Source startstreaming queues seconds of video")
	}
}

func TestLiveSampleDurationPrefersPTS(t *testing.T) {
	t0 := time.Unix(1000, 0)
	// gomedia OnFrame is milliseconds (PES 90 kHz / 90).
	d := liveSampleDuration(1000, 1066, t0, t0.Add(time.Millisecond))
	if d != 66*time.Millisecond {
		t.Fatalf("pts 66ms got %s", d)
	}
	d = liveSampleDuration(1000, 2000, t0, t0.Add(time.Second))
	if d != time.Second {
		t.Fatalf("1s hole got %s", d)
	}
}

func TestLiveSampleBytesSkipsParamSets(t *testing.T) {
	vps := []byte{0, 0, 0, 1, byte(codec.H265_NAL_VPS << 1), 1}
	idr := []byte{0, 0, 0, 1, byte(codec.H265_NAL_SLICE_IDR_W_RADL << 1), 1, 0xaa}
	out := liveSampleBytes(append(append([]byte{}, vps...), idr...))
	if binary.BigEndian.Uint32(out[:4]) != 3 || out[4] != byte(codec.H265_NAL_SLICE_IDR_W_RADL<<1) {
		t.Fatalf("%x", out)
	}
}

func TestLiveSaveStoresOnServer(t *testing.T) {
	js := liveSaveJS()
	if !strings.Contains(js, "/save") || !strings.Contains(js, `method: "POST"`) {
		t.Fatal("save should POST the clip to the server")
	}
	if strings.Contains(js, "createObjectURL") || strings.Contains(js, ".download") {
		t.Fatal("save must not download the clip to the browser any more")
	}
	if strings.Contains(js, "iframe") {
		t.Fatal("hidden iframe downloads break Save on Safari")
	}
	if strings.Contains(js, "/share") || strings.Contains(js, "clipboard.writeText") {
		t.Fatal("Save must not wire a share URL")
	}
}

func TestLiveFrameDurationWallClock(t *testing.T) {
	if d := liveFrameDuration(time.Time{}, time.Now()); d != liveDefaultDur {
		t.Fatalf("first frame duration=%s", d)
	}
	t0 := time.Unix(1000, 0)
	if d := liveFrameDuration(t0, t0.Add(66*time.Millisecond)); d != 66*time.Millisecond {
		t.Fatalf("66ms wall delta=%s", d)
	}
	if d := liveFrameDuration(t0, t0.Add(time.Millisecond)); d != time.Millisecond {
		t.Fatalf("1ms burst duration=%s", d)
	}
	if d := liveFrameDuration(t0, t0.Add(time.Second)); d != time.Second {
		t.Fatalf("1s camera hole must stay 1s, got %s", d)
	}
}

func TestShouldFlushLive(t *testing.T) {
	t0 := time.Unix(1, 0)
	if shouldFlushLive(nil, t0, t0.Add(time.Second)) {
		t.Fatal("empty pending must not flush")
	}
	one := []liveSample{{}}
	if shouldFlushLive(one, t0, t0.Add(40*time.Millisecond)) {
		t.Fatal("a lone IDR at 40ms must wait so P-frames can join the fragment")
	}
	if !shouldFlushLive(one, t0, t0.Add(liveFragHold)) {
		t.Fatal("lone sample must flush at hold")
	}
	two := []liveSample{{}, {}}
	if !shouldFlushLive(two, t0, t0.Add(40*time.Millisecond)) {
		t.Fatal("IDR+P must flush after 40ms")
	}
}

func TestLiveTicksFromWallClock(t *testing.T) {
	if liveTicks(20*time.Millisecond) != 1800 {
		t.Fatalf("20ms ticks=%d", liveTicks(20*time.Millisecond))
	}
	if liveTicks(time.Second) != liveTimescale {
		t.Fatalf("1s ticks=%d", liveTicks(time.Second))
	}
}

func TestAppendLiveGOP(t *testing.T) {
	idr := []byte{1, 2, 3}
	p := []byte{4, 5}
	gop := appendLiveGOP(nil, p, false)
	if len(gop) != 0 {
		t.Fatal("P-frame before IDR must not start a GOP")
	}
	gop = appendLiveGOP(gop, idr, true)
	gop = appendLiveGOP(gop, p, false)
	if len(gop) != 2 || string(gop[0]) != string(idr) || string(gop[1]) != string(p) {
		t.Fatalf("gop=%v", gop)
	}
	gop = appendLiveGOP(gop, []byte{9}, true)
	if len(gop) != 1 || gop[0][0] != 9 {
		t.Fatal("next IDR must replace the GOP so reconnects are live, not from session start")
	}
}

func TestHevcIRAPDetectsIDR(t *testing.T) {
	idr := []byte{0, 0, 0, 1, byte(codec.H265_NAL_SLICE_IDR_W_RADL << 1), 1}
	if !hevcHasIRAP(idr) {
		t.Fatal("IDR_W_RADL should count as IRAP")
	}
	trail := []byte{0, 0, 0, 1, byte(codec.H265_NAL_LICE_TRAIL_R << 1), 1}
	if hevcHasIRAP(trail) {
		t.Fatal("TRAIL_R should not count as IRAP")
	}
}

func TestLiveEdgeMarkerFollowsBacklog(t *testing.T) {
	h := &liveHub{peers: map[int]*livePeer{}}
	h.initSeg = []byte("init")
	h.gop = [][]byte{[]byte("idr"), []byte("p")}
	id, ch := h.addPeer()
	defer h.removePeer(id)
	for _, want := range []string{"init", "idr", "p", string(liveEdgeMarker)} {
		select {
		case got := <-ch:
			if string(got) != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing %q", want)
		}
	}
}

func TestNoEdgeMarkerBeforeReady(t *testing.T) {
	h := &liveHub{peers: map[int]*livePeer{}}
	id, ch := h.addPeer()
	defer h.removePeer(id)
	select {
	case got := <-ch:
		t.Fatalf("unexpected message %q for a not-ready peer", got)
	case <-time.After(50 * time.Millisecond):
	}
}

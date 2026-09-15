package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWantedAlwaysOn(t *testing.T) {
	orig := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.Email, cfg.Password, cfg.Serial = "a", "b", "c"
	cfg.StreamMode = "always"
	cfgMu.Unlock()
	s := NewStreamer()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.wantedLocked() {
		t.Fatal("always-on should be wanted without lastTouch")
	}
	cfgMu.Lock()
	cfg.StreamMode = "on_demand"
	cfgMu.Unlock()
	if s.wantedLocked() {
		t.Fatal("on-demand without lastTouch should not be wanted")
	}
	s.lastTouch = time.Now()
	if !s.wantedLocked() {
		t.Fatal("on-demand with fresh lastTouch should be wanted")
	}
}

func TestOnDemandExpiresAfterIdle(t *testing.T) {
	orig := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.Email, cfg.Password, cfg.Serial = "a", "b", "c"
	cfg.StreamMode = "on_demand"
	cfgMu.Unlock()
	s := NewStreamer()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastTouch = time.Now().Add(-idleTimeout - time.Second)
	if s.wantedLocked() {
		t.Fatal("on-demand should not be wanted after idle timeout")
	}
}

func TestKickDoesNotCountAsViewer(t *testing.T) {
	s := NewStreamer()
	s.Kick()
	if !s.lastTouch.IsZero() {
		t.Fatal("Kick should not create a viewer")
	}
}

func TestLivePlaylistColdStartPlaysImmediately(t *testing.T) {
	in := []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:1\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1,\nlive0.m4s\n")
	out := string(livePlaylist(in, ""))
	if !strings.Contains(out, "#EXT-X-START:TIME-OFFSET=0,PRECISE=YES") {
		t.Fatal(out)
	}
	if strings.Contains(out, "#EXT-X-PLAYLIST-TYPE:EVENT") {
		t.Fatal("EVENT playlists were rolled back")
	}
	long := append([]byte(nil), in...)
	long = append(long, []byte("#EXTINF:1,\nlive1.m4s\n#EXTINF:1,\nlive2.m4s\n#EXTINF:1,\nlive3.m4s\n#EXTINF:1,\nlive4.m4s\n")...)
	if n := len(playlistSegments(long)); n != 5 {
		t.Fatalf("segments=%d", n)
	}
	if strings.Contains(string(livePlaylist(long, "")), "#EXT-X-START:") {
		t.Fatal(string(livePlaylist(long, "")))
	}
}

func TestLivePlaylistFixesZeroDuration(t *testing.T) {
	in := []byte("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:0\n#EXTINF:1,\nlive0.m4s\n")
	out := string(livePlaylist(in, ""))
	if strings.Contains(out, "#EXT-X-TARGETDURATION:0") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "#EXT-X-TARGETDURATION:1") {
		t.Fatal(out)
	}
}

func TestPlaylistSegments(t *testing.T) {
	in := []byte("#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:1,\nlive0.m4s\n#EXTINF:1,\nlive1.m4s?token=x\n")
	got := playlistSegments(in)
	if len(got) != 2 || got[0] != "live0.m4s" || got[1] != "live1.m4s" {
		t.Fatalf("%v", got)
	}
}

func TestStreamStatusActive(t *testing.T) {
	if (StreamStatus{}).Active() {
		t.Fatal("zero status should be inactive")
	}
	if !(StreamStatus{Running: true}).Active() || !(StreamStatus{Starting: true}).Active() {
		t.Fatal("running or starting should be active")
	}
}

func TestIsLivePath(t *testing.T) {
	orig := *basePath
	t.Cleanup(func() { *basePath = orig })
	*basePath = "/ezviz"
	if !isLivePath("/ezviz") || !isLivePath("/ezviz/") {
		t.Fatal("live paths")
	}
	if isLivePath("/ezviz/preview.jpg") || isLivePath("/ezviz/hls/live.m3u8") {
		t.Fatal("leftover preview must not be the live page")
	}
}

func TestHlsFileReady(t *testing.T) {
	p := filepath.Join(t.TempDir(), "init.mp4")
	if hlsFileReady(p) {
		t.Fatal("missing file")
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hlsFileReady(p) {
		t.Fatal("empty-ish file must wait")
	}
	if err := os.WriteFile(p, make([]byte, hlsMinFile), 0o644); err != nil {
		t.Fatal(err)
	}
	if !hlsFileReady(p) {
		t.Fatal("complete file")
	}
}

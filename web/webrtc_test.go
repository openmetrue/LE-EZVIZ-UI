package main

import (
	"strings"
	"testing"
)

func TestOfferH264LinesIgnoresVP9(t *testing.T) {
	sdp := strings.Join([]string{
		"a=rtpmap:109 VP9/90000",
		"a=fmtp:109 profile-id=0",
		"a=rtpmap:102 H264/90000",
		"a=fmtp:102 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}, "\n")
	got := offerH264Lines(sdp)
	if strings.Contains(got, "109") || strings.Contains(got, "VP9") {
		t.Fatalf("should ignore VP9: %s", got)
	}
	if !strings.Contains(got, "H264") || !strings.Contains(got, "42e01f") {
		t.Fatalf("should keep H264: %s", got)
	}
}

func TestAnnexBNAL(t *testing.T) {
	out := annexBNAL([]byte{0x67, 0x42}, []byte{0x68, 0xce})
	if len(out) != 4+2+4+2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0] != 0 || out[3] != 1 || out[4] != 0x67 {
		t.Fatalf("bad annex-b: %v", out)
	}
}

package main

import "testing"

func TestAnnexBNAL(t *testing.T) {
	out := annexBNAL([]byte{0x67, 0x42}, []byte{0x68, 0xce})
	if len(out) != 4+2+4+2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0] != 0 || out[3] != 1 || out[4] != 0x67 {
		t.Fatalf("bad annex-b: %v", out)
	}
}

package main

import "testing"

func TestRawRingSnapshotAlignsPack(t *testing.T) {
	rawRingClear()
	t.Cleanup(rawRingClear)
	rawRingPush([]byte{0xff, 0xfe})
	rawRingPush([]byte{0x00, 0x00, 0x01, 0xba, 0x11, 0x22})
	data, ok := rawRingSnapshot()
	if !ok || len(data) < 4 || data[3] != 0xba {
		t.Fatalf("want pack-aligned snapshot, got ok=%v len=%d data=%x", ok, len(data), data)
	}
}

package main

import "testing"

func TestNormalizeConfigLegacySerial(t *testing.T) {
	c := Config{Serial: "BC1234567"}
	normalizeConfig(&c)
	if c.ActiveSerial != "BC1234567" {
		t.Fatalf("ActiveSerial = %q", c.ActiveSerial)
	}
}

func TestActiveSerialPrefersActiveSerial(t *testing.T) {
	c := Config{Serial: "OLD", ActiveSerial: "NEW"}
	if got := activeSerial(c); got != "NEW" {
		t.Fatalf("activeSerial = %q", got)
	}
}

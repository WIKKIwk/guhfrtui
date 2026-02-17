package reader

import (
	"strings"
	"testing"

	"new_era_go/internal/gobot/config"
)

func TestSetLongRangeMode(t *testing.T) {
	m := New(config.Config{}, nil, nil)

	initial := m.Status()
	if initial.ScanProfile != "balanced" {
		t.Fatalf("unexpected default profile: %q", initial.ScanProfile)
	}
	if initial.OutputPower != 0x1E {
		t.Fatalf("unexpected default power: 0x%02X", initial.OutputPower)
	}

	summaryOn := m.SetLongRangeMode(true)
	if !strings.Contains(summaryOn, "long-range yoqildi") {
		t.Fatalf("unexpected enable summary: %q", summaryOn)
	}
	stOn := m.Status()
	if stOn.ScanProfile != "long_range" {
		t.Fatalf("unexpected profile after enable: %q", stOn.ScanProfile)
	}
	if stOn.ScanTime != 0x0A {
		t.Fatalf("unexpected scan time after enable: 0x%02X", stOn.ScanTime)
	}
	if stOn.RegionCode != "US" {
		t.Fatalf("unexpected region after enable: %q", stOn.RegionCode)
	}
	if stOn.PerAntenna != 8 {
		t.Fatalf("unexpected per-antenna count after enable: %d", stOn.PerAntenna)
	}

	summaryOff := m.SetLongRangeMode(false)
	if !strings.Contains(summaryOff, "o'chirildi") {
		t.Fatalf("unexpected disable summary: %q", summaryOff)
	}
	stOff := m.Status()
	if stOff.ScanProfile != "balanced" {
		t.Fatalf("unexpected profile after disable: %q", stOff.ScanProfile)
	}
	if stOff.ScanTime != 0x01 {
		t.Fatalf("unexpected scan time after disable: 0x%02X", stOff.ScanTime)
	}
	if stOff.RegionCode != "-" {
		t.Fatalf("unexpected region after disable: %q", stOff.RegionCode)
	}
}

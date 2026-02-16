package sdk

import (
	"testing"
	"time"
)

func TestEffectiveIntervalUsesScanTimeFloor(t *testing.T) {
	cfg := InventoryConfig{
		ScanTime:     2,
		PollInterval: 40 * time.Millisecond,
	}
	got := cfg.EffectiveInterval()
	want := 200 * time.Millisecond
	if got != want {
		t.Fatalf("effective interval mismatch: got %s want %s", got, want)
	}
}

func TestEffectiveIntervalUsesPollWhenLarger(t *testing.T) {
	cfg := InventoryConfig{
		ScanTime:     1,
		PollInterval: 350 * time.Millisecond,
	}
	got := cfg.EffectiveInterval()
	want := 350 * time.Millisecond
	if got != want {
		t.Fatalf("effective interval mismatch: got %s want %s", got, want)
	}
}

func TestNormalizeConfigSetsSafeDefaults(t *testing.T) {
	cfg := normalizeConfig(InventoryConfig{
		AntennaMask:        0,
		ScanTime:           0,
		PollInterval:       0,
		OutputPower:        0x40,
		SingleFallbackEach: -5,
		NoTagABSwitch:      -1,
	})
	if cfg.AntennaMask != 0x01 {
		t.Fatalf("antenna mask not normalized: %02X", cfg.AntennaMask)
	}
	if cfg.ScanTime != 0x01 {
		t.Fatalf("scan time not normalized: %02X", cfg.ScanTime)
	}
	if cfg.OutputPower != 0x1E {
		t.Fatalf("output power not clamped: %02X", cfg.OutputPower)
	}
	if cfg.PollInterval <= 0 {
		t.Fatalf("poll interval not set")
	}
	if cfg.SingleFallbackEach != 0 {
		t.Fatalf("single fallback not clamped: %d", cfg.SingleFallbackEach)
	}
	if cfg.NoTagABSwitch != 0 {
		t.Fatalf("no-tag switch not clamped: %d", cfg.NoTagABSwitch)
	}
}

func TestNextInventoryAntennaCyclesMask(t *testing.T) {
	mask := byte(0x05) // ant1 and ant3
	a1, next := nextInventoryAntenna(mask, 0)
	if a1 != 0x80 {
		t.Fatalf("unexpected first antenna byte: 0x%02X", a1)
	}
	a2, _ := nextInventoryAntenna(mask, next)
	if a2 != 0x82 {
		t.Fatalf("unexpected second antenna byte: 0x%02X", a2)
	}
}

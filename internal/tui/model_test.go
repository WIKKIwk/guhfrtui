package tui

import (
	"bytes"
	"testing"
)

func TestParseHexInput(t *testing.T) {
	got, err := parseHexInput("A0 03 01 00")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []byte{0xA0, 0x03, 0x01, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected bytes: got %x want %x", got, want)
	}
}

func TestParseHexInputSingleToken(t *testing.T) {
	got, err := parseHexInput("0xA0030100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []byte{0xA0, 0x03, 0x01, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected bytes: got %x want %x", got, want)
	}
}

func TestParseHexInputInvalid(t *testing.T) {
	if _, err := parseHexInput("A0 GG"); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

package reader18

import (
	"bytes"
	"testing"
)

func buildResponseFrame(addr, cmd, status byte, data []byte) []byte {
	length := byte(len(data) + 5)
	packet := make([]byte, 0, int(length)+1)
	packet = append(packet, length, addr, cmd, status)
	packet = append(packet, data...)
	crc := crc16MCRF4XX(packet)
	packet = append(packet, byte(crc&0xFF), byte(crc>>8))
	return packet
}

func TestBuildInventoryCommandAddr00(t *testing.T) {
	got := BuildCommand(0x00, CmdInventory, nil)
	want := []byte{0x04, 0x00, 0x01, 0xDB, 0x4B}
	if !bytes.Equal(got, want) {
		t.Fatalf("inventory command mismatch: got %X want %X", got, want)
	}
}

func TestBuildGetReaderInfoAddr00(t *testing.T) {
	got := BuildCommand(0x00, CmdGetReaderInfo, nil)
	want := []byte{0x04, 0x00, 0x21, 0xD9, 0x6A}
	if !bytes.Equal(got, want) {
		t.Fatalf("get-info command mismatch: got %X want %X", got, want)
	}
}

func TestBuildInventoryCommandAddrFF(t *testing.T) {
	got := BuildCommand(0xFF, CmdInventory, nil)
	want := []byte{0x04, 0xFF, 0x01, 0x1B, 0xB4}
	if !bytes.Equal(got, want) {
		t.Fatalf("inventory broadcast command mismatch: got %X want %X", got, want)
	}
}

func TestBuildInventorySingleTagCommandAddr00(t *testing.T) {
	got := InventorySingleTagCommand(0x00)
	want := []byte{0x04, 0x00, 0x0F, 0xA5, 0xA2}
	if !bytes.Equal(got, want) {
		t.Fatalf("single-tag inventory command mismatch: got %X want %X", got, want)
	}
}

func TestBuildInventoryCommandWithPayload(t *testing.T) {
	got := InventoryCommand(0x00, 0x00, 0x01)
	want := []byte{0x06, 0x00, 0x01, 0x00, 0x01, 0x45, 0x40}
	if !bytes.Equal(got, want) {
		t.Fatalf("inventory command payload mismatch: got %X want %X", got, want)
	}
}

func TestParseFrames(t *testing.T) {
	frame1 := buildResponseFrame(0x00, CmdInventory, StatusSuccess, []byte{0x01, 0xAA})
	frame2 := buildResponseFrame(0x00, CmdGetReaderInfo, StatusSuccess, []byte{0x10})
	stream := append(append([]byte{}, frame1...), frame2...)

	frames, remaining := ParseFrames(stream)
	if len(remaining) != 0 {
		t.Fatalf("expected no remaining bytes, got %d", len(remaining))
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames, got %d", len(frames))
	}
	if frames[0].Command != CmdInventory {
		t.Fatalf("frame[0] command mismatch: %02X", frames[0].Command)
	}
	if frames[1].Command != CmdGetReaderInfo {
		t.Fatalf("frame[1] command mismatch: %02X", frames[1].Command)
	}
}

func TestParseFramesWithGarbagePrefix(t *testing.T) {
	frame := buildResponseFrame(0x00, CmdInventory, StatusNoTag, nil)
	stream := append([]byte{0x00, 0x00}, frame...)
	frames, remaining := ParseFrames(stream)
	if len(remaining) != 0 {
		t.Fatalf("expected no remaining bytes, got %d", len(remaining))
	}
	if len(frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(frames))
	}
}

func TestInventoryTagCount(t *testing.T) {
	f := Frame{Command: CmdInventory, Status: StatusSuccess, Data: []byte{0x03}}
	count, err := InventoryTagCount(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Fatalf("unexpected tag count: got %d want 3", count)
	}
}

func TestParseSingleInventoryResult(t *testing.T) {
	f := Frame{
		Command: CmdInventorySingle,
		Status:  StatusNoTag,
		Data:    []byte{0x01, 0x01, 0x0C, 0x30, 0x34, 0x25, 0x7B, 0xF7, 0x19, 0x4E, 0x40, 0x00, 0x00, 0x00, 0x42},
	}
	result, err := ParseSingleInventoryResult(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Antenna != 0x01 {
		t.Fatalf("unexpected antenna: got %d", result.Antenna)
	}
	if result.TagCount != 1 {
		t.Fatalf("unexpected tag count: got %d", result.TagCount)
	}
	if len(result.EPC) != 12 {
		t.Fatalf("unexpected epc len: got %d", len(result.EPC))
	}
	if result.EPC[0] != 0x30 {
		t.Fatalf("unexpected epc first byte: got 0x%02X", result.EPC[0])
	}
}

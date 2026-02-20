package testmode

import "testing"

func TestParseEPCFileStats(t *testing.T) {
	content := []byte("\uFEFF# sample\n\nE20000112233\nE20000112233\nxx-yy\n  e20000aa  \n")

	epcs, stats := parseEPCFile(content)

	if stats.TotalLines != 4 {
		t.Fatalf("TotalLines mismatch: got=%d want=4", stats.TotalLines)
	}
	if stats.ValidLines != 3 {
		t.Fatalf("ValidLines mismatch: got=%d want=3", stats.ValidLines)
	}
	if stats.UniqueEPCs != 2 {
		t.Fatalf("UniqueEPCs mismatch: got=%d want=2", stats.UniqueEPCs)
	}
	if stats.DuplicateLines != 1 {
		t.Fatalf("DuplicateLines mismatch: got=%d want=1", stats.DuplicateLines)
	}
	if stats.InvalidLines != 1 {
		t.Fatalf("InvalidLines mismatch: got=%d want=1", stats.InvalidLines)
	}
	if len(epcs) != 2 || epcs[0] != "E20000112233" || epcs[1] != "E20000AA" {
		t.Fatalf("unexpected epcs: %#v", epcs)
	}
}

func TestManagerLoadFileReplacesPreviousState(t *testing.T) {
	m := New()

	first, err := m.LoadFile(11, "first.txt", []byte("E200AA\nE200BB\n"))
	if err != nil {
		t.Fatalf("first LoadFile failed: %v", err)
	}

	match1 := m.RecordRead("E200AA")
	if !match1.Matched || !match1.NewlyRead || match1.SessionID != first.SessionID {
		t.Fatalf("first match invalid: %#v", match1)
	}

	second, err := m.LoadFile(11, "second.txt", []byte("E200CC\nE200CC\n"))
	if err != nil {
		t.Fatalf("second LoadFile failed: %v", err)
	}
	if second.SessionID == first.SessionID {
		t.Fatalf("session id did not increment: %d", second.SessionID)
	}

	oldMatch := m.RecordRead("E200AA")
	if oldMatch.Matched {
		t.Fatalf("old EPC should not match after new file: %#v", oldMatch)
	}

	newMatch := m.RecordRead("E200CC")
	if !newMatch.Matched || !newMatch.NewlyRead || newMatch.SessionID != second.SessionID {
		t.Fatalf("new EPC should match new session: %#v", newMatch)
	}
}

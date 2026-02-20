package telegram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteCacheDumpFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 2, 19, 12, 0, 0, 0, time.UTC)

	draftPath, seenPath, err := writeCacheDumpFiles(dir, now, []string{"E100CC", "", "E100DD"}, []string{"E200AA", "", "E200BB"})
	if err != nil {
		t.Fatalf("writeCacheDumpFiles failed: %v", err)
	}

	if draftPath != filepath.Join(dir, "cache_draft_epcs.txt") {
		t.Fatalf("unexpected draft path: %s", draftPath)
	}
	if seenPath != filepath.Join(dir, "cache_seen_epcs.txt") {
		t.Fatalf("unexpected seen path: %s", seenPath)
	}

	draftBody, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatalf("read draft file failed: %v", err)
	}
	draftText := string(draftBody)
	if !strings.Contains(draftText, "# draft_epc_count=2") {
		t.Fatalf("draft epc count missing: %q", draftText)
	}
	if !strings.Contains(draftText, "\nE100CC\n") || !strings.Contains(draftText, "\nE100DD\n") {
		t.Fatalf("draft EPC entries missing: %q", draftText)
	}

	seenBody, err := os.ReadFile(seenPath)
	if err != nil {
		t.Fatalf("read seen file failed: %v", err)
	}
	seenText := string(seenBody)
	if !strings.Contains(seenText, "# seen_epc_count=2") {
		t.Fatalf("seen count header missing: %q", seenText)
	}
	if !strings.Contains(seenText, "\nE200AA\n") || !strings.Contains(seenText, "\nE200BB\n") {
		t.Fatalf("seen EPC entries missing: %q", seenText)
	}
}

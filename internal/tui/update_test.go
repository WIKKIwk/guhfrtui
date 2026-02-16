package tui

import (
	"testing"

	"new_era_go/internal/discovery"
	"new_era_go/internal/reader"
)

func TestStartReadingQueuesScanWhenDisconnectedAndNoCandidates(t *testing.T) {
	m := NewModel()
	m.scanning = false
	m.candidates = nil

	next, cmd := m.runControlAction(0)
	if cmd == nil {
		t.Fatal("expected scan/connect command, got nil")
	}

	nm, ok := next.(Model)
	if !ok {
		t.Fatal("unexpected model type")
	}
	if nm.pendingAction != 0 {
		t.Fatalf("expected pendingAction=0, got %d", nm.pendingAction)
	}
	if !nm.scanning {
		t.Fatal("expected scanning=true")
	}
	if !nm.pendingConnect {
		t.Fatal("expected pendingConnect=true")
	}
}

func TestStartReadingQueuesConnectWhenCandidatesExist(t *testing.T) {
	m := NewModel()
	m.scanning = false
	m.pendingConnect = false
	m.candidates = []discovery.Candidate{
		{Host: "192.168.1.200", Port: 6000, Verified: true, Score: 999},
	}

	next, cmd := m.runControlAction(0)
	if cmd == nil {
		t.Fatal("expected connect command, got nil")
	}

	nm := next.(Model)
	if nm.pendingAction != 0 {
		t.Fatalf("expected pendingAction=0, got %d", nm.pendingAction)
	}
	if nm.scanning {
		t.Fatal("expected scanning=false")
	}
	if nm.status == "" {
		t.Fatal("expected status message")
	}
}

func TestOnConnectFinishedRunsPendingStartAction(t *testing.T) {
	m := NewModel()
	m.pendingAction = 0

	next, cmd := m.onConnectFinished(connectFinishedMsg{Endpoint: reader.Endpoint{Host: "10.0.0.5", Port: 6000}, Err: nil})
	if cmd == nil {
		t.Fatal("expected command batch")
	}

	nm := next.(Model)
	if !nm.inventoryRunning {
		t.Fatal("expected inventoryRunning=true")
	}
	if nm.pendingAction != noPendingAction {
		t.Fatalf("expected pendingAction cleared, got %d", nm.pendingAction)
	}
}

func TestOnScanFinishedClearsPendingActionWhenNoCandidates(t *testing.T) {
	m := NewModel()
	m.pendingAction = 2
	m.pendingConnect = true

	next, _ := m.onScanFinished(scanFinishedMsg{Candidates: nil})
	nm := next.(Model)
	if nm.pendingAction != noPendingAction {
		t.Fatalf("expected pendingAction cleared, got %d", nm.pendingAction)
	}
	if nm.pendingConnect {
		t.Fatal("expected pendingConnect=false")
	}
}

package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"new_era_go/internal/gobot/cache"
	"new_era_go/internal/gobot/config"
	"new_era_go/internal/gobot/erp"
)

func testConfig() config.Config {
	return config.Config{
		RequestTimeout:   2 * time.Second,
		RefreshInterval:  60 * time.Second,
		SubmitRetry:      0,
		SubmitRetryDelay: 10 * time.Millisecond,
		WorkerCount:      1,
		QueueSize:        128,
		RecentSeenTTL:    10 * time.Minute,
	}
}

func TestHandleEPCRequiresActiveScan(t *testing.T) {
	c := cache.New()
	c.Add([]string{"E200001122334455"})
	svc := New(testConfig(), nil, c)

	res := svc.HandleEPC(context.Background(), "E200001122334455", "test")
	if res.Action != "scan_inactive" {
		t.Fatalf("expected scan_inactive action, got %q", res.Action)
	}

	st := svc.Status()
	if st.ScanInactive != 1 {
		t.Fatalf("expected scan_inactive=1, got %d", st.ScanInactive)
	}
}

func TestSetScanActiveReplaysSeenEPCs(t *testing.T) {
	c := cache.New()
	c.Add([]string{"E200001122334455"})
	svc := New(testConfig(), nil, c)

	_ = svc.HandleEPC(context.Background(), "E200001122334455", "test")
	replay := svc.SetScanActive(true, "unit_test")
	if replay < 1 {
		t.Fatalf("expected replay >= 1, got %d", replay)
	}

	res := svc.HandleEPC(context.Background(), "E200001122334455", "test")
	if res.Action != "queued" && res.Action != "queued_or_dropped" {
		t.Fatalf("expected queued* action after active scan, got %q", res.Action)
	}
}

func TestSubmittedEPCCanReplayWhenNewDraftArrives(t *testing.T) {
	const epcValue = "E200001122334455"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "submit_open_stock_entry_by_epc"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":{"ok":true,"status":"submitted"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.RequestTimeout = time.Second
	c := cache.New()
	c.Add([]string{epcValue})
	erpClient := erp.New(srv.URL, "k", "s", cfg.RequestTimeout)
	svc := New(cfg, erpClient, c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Run(ctx)
	svc.SetScanActive(true, "unit_test")

	res := svc.HandleEPC(ctx, epcValue, "unit_test")
	if res.Action != "queued" && res.Action != "queued_or_dropped" {
		t.Fatalf("expected queued action, got %q", res.Action)
	}

	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if svc.Status().SubmittedOK >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if svc.Status().SubmittedOK < 1 {
		t.Fatalf("expected submitted_ok >= 1, got %d", svc.Status().SubmittedOK)
	}
	if svc.cache.Has(epcValue) {
		t.Fatal("expected EPC removed from cache after submit")
	}

	svc.mu.Lock()
	_, seenKept := svc.recentSeen[epcValue]
	svc.mu.Unlock()
	if !seenKept {
		t.Fatal("expected recentSeen to keep EPC after submit for replay")
	}

	added, replay := svc.AddDraftEPCs(ctx, []string{epcValue})
	if added != 1 {
		t.Fatalf("expected added=1, got %d", added)
	}
	if replay < 1 {
		t.Fatalf("expected replay>=1 for same EPC, got %d", replay)
	}
}

type captureNotifier struct {
	messages []string
}

func (n *captureNotifier) Notify(text string) {
	n.messages = append(n.messages, text)
}

func TestRefreshCacheNotifiesWhenDraftCountIncreasesWithoutNewEPC(t *testing.T) {
	const epcValue = "E200001122334455"
	call := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "get_open_stock_entry_drafts_fast") {
			http.NotFound(w, r)
			return
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = w.Write([]byte(`{"message":{"ok":true,"epc_only":true,"epcs":["` + epcValue + `"],"count_drafts":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"message":{"ok":true,"epc_only":true,"epcs":["` + epcValue + `"],"count_drafts":2}}`))
	}))
	defer srv.Close()

	cfg := testConfig()
	cfg.RequestTimeout = time.Second
	erpClient := erp.New(srv.URL, "k", "s", cfg.RequestTimeout)
	svc := New(cfg, erpClient, cache.New())
	notifier := &captureNotifier{}
	svc.SetNotifier(notifier)

	ctx := context.Background()
	if err := svc.RefreshCache(ctx, "startup", false); err != nil {
		t.Fatalf("startup refresh failed: %v", err)
	}
	if len(notifier.messages) != 0 {
		t.Fatalf("expected no startup notify, got %v", notifier.messages)
	}

	if err := svc.RefreshCache(ctx, "periodic", false); err != nil {
		t.Fatalf("periodic refresh failed: %v", err)
	}
	if len(notifier.messages) == 0 {
		t.Fatal("expected draft increase notification")
	}
	last := notifier.messages[len(notifier.messages)-1]
	if !strings.Contains(last, "draft +1") {
		t.Fatalf("expected draft +1 message, got %q", last)
	}
}

func TestRecentSeenEPCsReturnsSortedSnapshot(t *testing.T) {
	svc := New(testConfig(), nil, cache.New())

	_ = svc.HandleEPC(context.Background(), "e200001122334455", "test")
	_ = svc.HandleEPC(context.Background(), "  E200001122334450  ", "test")

	got := svc.RecentSeenEPCs()
	want := []string{"E200001122334450", "E200001122334455"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("seen snapshot mismatch: got %v want %v", got, want)
	}
}

func TestDraftEPCsReturnsSortedSnapshot(t *testing.T) {
	c := cache.New()
	c.Add([]string{"E300002", "E300001", "E300003"})
	svc := New(testConfig(), nil, c)

	got := svc.DraftEPCs()
	want := []string{"E300001", "E300002", "E300003"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("draft snapshot mismatch: got %v want %v", got, want)
	}
}

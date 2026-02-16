package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"new_era_go/internal/gobot/cache"
	"new_era_go/internal/gobot/config"
	"new_era_go/internal/gobot/erp"
)

type Notifier interface {
	Notify(text string)
}

type IngestResult struct {
	EPC    string `json:"epc"`
	Action string `json:"action"`
	Error  string `json:"error,omitempty"`
}

type Stats struct {
	CacheSize     int       `json:"cache_size"`
	DraftCount    int       `json:"draft_count"`
	LastRefreshAt time.Time `json:"last_refresh_at"`
	LastRefreshOK bool      `json:"last_refresh_ok"`
	LastError     string    `json:"last_error,omitempty"`
	ScanActive    bool      `json:"scan_active"`
	ScanSince     time.Time `json:"scan_since"`

	SeenTotal      uint64 `json:"seen_total"`
	CacheHits      uint64 `json:"cache_hits"`
	CacheMisses    uint64 `json:"cache_misses"`
	SubmittedOK    uint64 `json:"submitted_ok"`
	SubmitNotFound uint64 `json:"submit_not_found"`
	SubmitErrors   uint64 `json:"submit_errors"`
	QueueDropped   uint64 `json:"queue_dropped"`
	ScanInactive   uint64 `json:"scan_inactive"`
}

type Service struct {
	cfg   config.Config
	erp   *erp.Client
	cache *cache.Store
	queue chan string

	mu          sync.Mutex
	inflight    map[string]struct{}
	queued      map[string]struct{}
	recentSeen  map[string]time.Time
	draftCount  int
	lastRefresh time.Time
	lastErr     string
	scanActive  bool
	scanSince   time.Time
	stats       Stats
	notifier    Notifier
}

func New(cfg config.Config, erpClient *erp.Client, c *cache.Store) *Service {
	now := time.Now()
	scanSince := time.Time{}
	if cfg.ScanDefaultActive {
		scanSince = now
	}
	return &Service{
		cfg:        cfg,
		erp:        erpClient,
		cache:      c,
		queue:      make(chan string, cfg.QueueSize),
		inflight:   make(map[string]struct{}),
		queued:     make(map[string]struct{}),
		recentSeen: make(map[string]time.Time),
		scanActive: cfg.ScanDefaultActive,
		scanSince:  scanSince,
		stats: Stats{
			ScanActive: cfg.ScanDefaultActive,
			ScanSince:  scanSince,
		},
	}
}

func (s *Service) SetNotifier(n Notifier) {
	s.mu.Lock()
	s.notifier = n
	s.mu.Unlock()
}

func (s *Service) Bootstrap(ctx context.Context) error {
	return s.RefreshCache(ctx, "startup", true)
}

func (s *Service) Run(ctx context.Context) {
	for i := 0; i < s.cfg.WorkerCount; i++ {
		go s.worker(ctx, i+1)
	}
	go s.refreshLoop(ctx)
}

func (s *Service) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.RefreshCache(ctx, "periodic", false); err != nil {
				log.Printf("[bot] periodic refresh failed: %v", err)
			}
		}
	}
}

func (s *Service) RefreshCache(parent context.Context, reason string, notify bool) error {
	ctx, cancel := context.WithTimeout(parent, s.cfg.RequestTimeout)
	defer cancel()

	res, err := s.erp.FetchDraftEPCs(ctx)
	if err != nil {
		s.mu.Lock()
		s.lastErr = err.Error()
		s.lastRefresh = time.Now()
		s.stats.LastRefreshAt = s.lastRefresh
		s.stats.LastRefreshOK = false
		s.stats.LastError = s.lastErr
		s.mu.Unlock()
		return err
	}

	newEPCs := s.diffNewEPCs(res.EPCs)
	s.cache.Replace(res.EPCs)
	now := time.Now()
	replay := s.collectReplayCandidates(now, res.EPCs)

	s.mu.Lock()
	prevDraftCount := s.draftCount
	s.draftCount = res.DraftCount
	s.lastRefresh = now
	s.lastErr = ""
	s.stats.CacheSize = s.cache.Size()
	s.stats.DraftCount = s.draftCount
	s.stats.LastRefreshAt = now
	s.stats.LastRefreshOK = true
	s.stats.LastError = ""
	s.mu.Unlock()

	if s.ScanActive() {
		for _, epc := range replay {
			_ = s.enqueue(epc)
		}
	}

	if reason != "startup" {
		cacheSize := s.cache.Size()
		if len(newEPCs) > 0 {
			s.notify(fmt.Sprintf("Yangi draft ERP'dan keldi: +%d EPC (cache=%d). Namuna: %s",
				len(newEPCs), cacheSize, summarizeEPCs(newEPCs, 3)))
		} else if res.DraftCount > prevDraftCount {
			s.notify(fmt.Sprintf("Yangi draft ERP'dan keldi: draft +%d (cache=%d, EPC diff=0)",
				res.DraftCount-prevDraftCount, cacheSize))
		}
	}

	if notify {
		s.notify(fmt.Sprintf("Turbo tayyor: %d ta draft, %d ta EPC cache ga yangilandi.", res.DraftCount, len(res.EPCs)))
	} else {
		log.Printf("[bot] cache refresh (%s): drafts=%d epcs=%d replay=%d", reason, res.DraftCount, len(res.EPCs), len(replay))
	}

	return nil
}

func (s *Service) AddDraftEPCs(_ context.Context, epcs []string) (int, int) {
	now := time.Now()
	clean := normalizeEPCList(epcs)
	newEPCs := s.diffNewEPCs(clean)
	added := s.cache.Add(clean)
	replay := s.collectReplayCandidates(now, clean)
	if s.ScanActive() {
		for _, epc := range replay {
			_ = s.enqueue(epc)
		}
	}
	if added > 0 {
		cacheSize := s.cache.Size()
		s.notify(fmt.Sprintf("Yangi draft webhook: +%d EPC (cache=%d). Namuna: %s",
			added, cacheSize, summarizeEPCs(newEPCs, 3)))
	}
	return added, len(replay)
}

func (s *Service) HandleEPC(_ context.Context, rawEPC, _ string) IngestResult {
	epc := erp.NormalizeEPC(rawEPC)
	if epc == "" {
		return IngestResult{Action: "invalid", Error: "epc is empty"}
	}

	now := time.Now()
	s.mu.Lock()
	s.recentSeen[epc] = now
	s.gcRecentSeenLocked(now)
	s.stats.SeenTotal++
	scanActive := s.scanActive
	if !scanActive {
		s.stats.ScanInactive++
	}
	s.mu.Unlock()
	if !scanActive {
		return IngestResult{EPC: epc, Action: "scan_inactive"}
	}

	if !s.cache.Has(epc) {
		s.mu.Lock()
		s.stats.CacheMisses++
		s.mu.Unlock()
		return IngestResult{EPC: epc, Action: "miss"}
	}

	s.mu.Lock()
	s.stats.CacheHits++
	s.mu.Unlock()

	if !s.enqueue(epc) {
		return IngestResult{EPC: epc, Action: "queued_or_dropped"}
	}
	return IngestResult{EPC: epc, Action: "queued"}
}

func (s *Service) SetScanActive(active bool, reason string) int {
	now := time.Now()
	var becameActive bool

	s.mu.Lock()
	if s.scanActive != active {
		s.scanActive = active
		s.stats.ScanActive = active
		if active {
			s.scanSince = now
			s.stats.ScanSince = now
			becameActive = true
		} else {
			s.scanSince = time.Time{}
			s.stats.ScanSince = time.Time{}
		}
	}
	s.mu.Unlock()

	if !becameActive {
		return 0
	}

	replay := s.collectReplayCandidates(now, nil)
	for _, epc := range replay {
		_ = s.enqueue(epc)
	}
	log.Printf("[bot] scan active (%s): replay=%d", reason, len(replay))
	return len(replay)
}

func (s *Service) ScanActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanActive
}

func (s *Service) Status() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.CacheSize = s.cache.Size()
	s.stats.DraftCount = s.draftCount
	s.stats.ScanActive = s.scanActive
	s.stats.ScanSince = s.scanSince
	return s.stats
}

func (s *Service) StatusText() string {
	st := s.Status()
	return fmt.Sprintf(
		"Scan: active=%v since=%s\nCache: %d EPC (draft=%d)\nSeen: %d | hit=%d miss=%d inactive=%d\nSubmit: ok=%d not_found=%d err=%d\nLast refresh: %s (ok=%v)",
		st.ScanActive,
		formatTime(st.ScanSince),
		st.CacheSize,
		st.DraftCount,
		st.SeenTotal,
		st.CacheHits,
		st.CacheMisses,
		st.ScanInactive,
		st.SubmittedOK,
		st.SubmitNotFound,
		st.SubmitErrors,
		formatTime(st.LastRefreshAt),
		st.LastRefreshOK,
	)
}

func (s *Service) worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case epc := <-s.queue:
			s.mu.Lock()
			delete(s.queued, epc)
			s.mu.Unlock()

			if err := s.processSubmit(ctx, epc); err != nil {
				log.Printf("[bot] worker=%d submit failed epc=%s err=%v", workerID, epc, err)
			}
		}
	}
}

func (s *Service) processSubmit(parent context.Context, epc string) error {
	if epc == "" {
		return nil
	}
	if !s.cache.Has(epc) {
		return nil
	}

	if !s.lockInflight(epc) {
		return nil
	}
	defer s.unlockInflight(epc)

	var lastErr error
	retries := s.cfg.SubmitRetry
	for attempt := 0; attempt <= retries; attempt++ {
		ctx, cancel := context.WithTimeout(parent, s.cfg.RequestTimeout)
		status, err := s.erp.SubmitByEPC(ctx, epc)
		cancel()

		if err == nil {
			switch status {
			case erp.SubmitStatusSubmitted:
				s.cache.Remove(epc)
				s.mu.Lock()
				s.stats.SubmittedOK++
				s.stats.CacheSize = s.cache.Size()
				s.mu.Unlock()
				s.notify("Submit OK: " + trimEPC(epc))
				return nil
			case erp.SubmitStatusNotFound:
				s.cache.Remove(epc)
				s.mu.Lock()
				s.stats.SubmitNotFound++
				s.stats.CacheSize = s.cache.Size()
				s.mu.Unlock()
				return nil
			default:
				lastErr = fmt.Errorf("unexpected submit status: %s", status)
			}
		} else {
			lastErr = err
		}

		if attempt < retries {
			time.Sleep(s.cfg.SubmitRetryDelay)
		}
	}

	s.mu.Lock()
	s.stats.SubmitErrors++
	s.mu.Unlock()
	s.notify("Submit xato: " + trimEPC(epc))
	return lastErr
}

func (s *Service) enqueue(epc string) bool {
	if epc == "" {
		return false
	}

	s.mu.Lock()
	if _, ok := s.queued[epc]; ok {
		s.mu.Unlock()
		return false
	}
	if _, ok := s.inflight[epc]; ok {
		s.mu.Unlock()
		return false
	}
	s.queued[epc] = struct{}{}
	s.mu.Unlock()

	select {
	case s.queue <- epc:
		return true
	default:
		s.mu.Lock()
		delete(s.queued, epc)
		s.stats.QueueDropped++
		s.mu.Unlock()
		return false
	}
}

func (s *Service) lockInflight(epc string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.inflight[epc]; ok {
		return false
	}
	s.inflight[epc] = struct{}{}
	return true
}

func (s *Service) unlockInflight(epc string) {
	s.mu.Lock()
	delete(s.inflight, epc)
	s.mu.Unlock()
}

func (s *Service) removeSeen(epc string) {
	s.mu.Lock()
	delete(s.recentSeen, epc)
	s.mu.Unlock()
}

func (s *Service) collectReplayCandidates(now time.Time, focus []string) []string {
	focusSet := make(map[string]struct{}, len(focus))
	for _, epc := range focus {
		if epc != "" {
			focusSet[epc] = struct{}{}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.gcRecentSeenLocked(now)
	out := make([]string, 0, len(s.recentSeen))
	for epc := range s.recentSeen {
		if len(focusSet) > 0 {
			if _, ok := focusSet[epc]; !ok {
				continue
			}
		}
		if s.cache.Has(epc) {
			out = append(out, epc)
		}
	}
	return out
}

func (s *Service) gcRecentSeenLocked(now time.Time) {
	for epc, ts := range s.recentSeen {
		if now.Sub(ts) > s.cfg.RecentSeenTTL {
			delete(s.recentSeen, epc)
		}
	}
}

func (s *Service) notify(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	s.mu.Lock()
	n := s.notifier
	s.mu.Unlock()
	if n != nil {
		n.Notify(text)
	}
}

func normalizeEPCList(values []string) []string {
	uniq := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, raw := range values {
		epc := erp.NormalizeEPC(raw)
		if epc == "" {
			continue
		}
		if _, ok := uniq[epc]; ok {
			continue
		}
		uniq[epc] = struct{}{}
		out = append(out, epc)
	}
	return out
}

func (s *Service) diffNewEPCs(epcs []string) []string {
	if len(epcs) == 0 {
		return nil
	}
	out := make([]string, 0, len(epcs))
	for _, epc := range epcs {
		if epc == "" {
			continue
		}
		if s.cache.Has(epc) {
			continue
		}
		out = append(out, epc)
	}
	return out
}

func summarizeEPCs(epcs []string, max int) string {
	if len(epcs) == 0 {
		return "-"
	}
	if max <= 0 {
		max = 1
	}
	if max > len(epcs) {
		max = len(epcs)
	}
	parts := make([]string, 0, max)
	for i := 0; i < max; i++ {
		parts = append(parts, trimEPC(epcs[i]))
	}
	if len(epcs) > max {
		return strings.Join(parts, ", ") + fmt.Sprintf(" (+%d)", len(epcs)-max)
	}
	return strings.Join(parts, ", ")
}

func trimEPC(epc string) string {
	if len(epc) <= 16 {
		return epc
	}
	return epc[:16] + "..."
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format(time.RFC3339)
}

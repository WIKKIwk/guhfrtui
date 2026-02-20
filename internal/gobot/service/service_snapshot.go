package service

import (
	"sort"
	"time"
)

// RecentSeenEPCs returns normalized EPCs observed in the recent-seen window.
func (s *Service) RecentSeenEPCs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Keep export aligned with runtime dedup window semantics.
	s.gcRecentSeenLocked(time.Now())

	out := make([]string, 0, len(s.recentSeen))
	for epc := range s.recentSeen {
		out = append(out, epc)
	}
	sort.Strings(out)
	return out
}

// DraftEPCs returns current cache EPC snapshot (ERP draft EPC cache).
func (s *Service) DraftEPCs() []string {
	return s.cache.SnapshotSorted()
}

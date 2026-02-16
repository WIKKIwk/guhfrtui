package cache

import "sync"

type Store struct {
	mu   sync.RWMutex
	epcs map[string]struct{}
}

func New() *Store {
	return &Store{epcs: make(map[string]struct{})}
}

func (s *Store) Replace(epcs []string) {
	next := make(map[string]struct{}, len(epcs))
	for _, epc := range epcs {
		if epc == "" {
			continue
		}
		next[epc] = struct{}{}
	}

	s.mu.Lock()
	s.epcs = next
	s.mu.Unlock()
}

func (s *Store) Add(epcs []string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	added := 0
	for _, epc := range epcs {
		if epc == "" {
			continue
		}
		if _, exists := s.epcs[epc]; exists {
			continue
		}
		s.epcs[epc] = struct{}{}
		added++
	}
	return added
}

func (s *Store) Remove(epc string) {
	if epc == "" {
		return
	}
	s.mu.Lock()
	delete(s.epcs, epc)
	s.mu.Unlock()
}

func (s *Store) Has(epc string) bool {
	if epc == "" {
		return false
	}
	s.mu.RLock()
	_, ok := s.epcs[epc]
	s.mu.RUnlock()
	return ok
}

func (s *Store) Size() int {
	s.mu.RLock()
	size := len(s.epcs)
	s.mu.RUnlock()
	return size
}

package testmode

import (
	"fmt"
	"sync"
	"time"

	"new_era_go/internal/gobot/erp"
)

type LoadStats struct {
	FileName       string
	TotalLines     int
	ValidLines     int
	UniqueEPCs     int
	DuplicateLines int
	InvalidLines   int
	SessionID      uint64
}

type MatchResult struct {
	Matched   bool
	NewlyRead bool
	EPC       string
	ChatID    int64
	ReadCount int
	Total     int
	SessionID uint64
}

type StopResult struct {
	SessionID uint64
	FileName  string
	Total     int
	Read      int
	Unread    int
	ChatID    int64
}

type Manager struct {
	mu sync.Mutex

	awaitingFile bool
	ownerChatID  int64

	active    bool
	sessionID uint64
	fileName  string
	startedAt time.Time

	expected map[string]struct{}
	read     map[string]struct{}
}

func New() *Manager {
	return &Manager{
		expected: make(map[string]struct{}),
		read:     make(map[string]struct{}),
	}
}

func (m *Manager) RequestFile(chatID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.awaitingFile = true
	m.ownerChatID = chatID
	m.active = false
	m.fileName = ""
	m.startedAt = time.Time{}
	m.expected = make(map[string]struct{})
	m.read = make(map[string]struct{})
}

func (m *Manager) IsAwaitingFile(chatID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.awaitingFile {
		return false
	}
	return m.ownerChatID == 0 || m.ownerChatID == chatID
}

func (m *Manager) LoadFile(chatID int64, fileName string, content []byte) (LoadStats, error) {
	epcs, stats := parseEPCFile(content)
	stats.FileName = fileName
	if stats.UniqueEPCs == 0 {
		return stats, fmt.Errorf("faylda yaroqli EPC topilmadi")
	}

	expected := make(map[string]struct{}, len(epcs))
	for _, epc := range epcs {
		normalized := erp.NormalizeEPC(epc)
		if normalized == "" {
			continue
		}
		expected[normalized] = struct{}{}
	}
	if len(expected) == 0 {
		return stats, fmt.Errorf("faylda yaroqli EPC topilmadi")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.expected = expected
	m.read = make(map[string]struct{}, len(expected))
	m.active = true
	m.awaitingFile = false
	m.ownerChatID = chatID
	m.fileName = fileName
	m.startedAt = time.Now()
	m.sessionID++

	stats.SessionID = m.sessionID
	return stats, nil
}

func (m *Manager) RecordRead(epc string) MatchResult {
	normalized := erp.NormalizeEPC(epc)
	if normalized == "" {
		return MatchResult{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.active || len(m.expected) == 0 {
		return MatchResult{}
	}

	if _, ok := m.expected[normalized]; !ok {
		return MatchResult{}
	}

	result := MatchResult{
		Matched:   true,
		EPC:       normalized,
		ChatID:    m.ownerChatID,
		Total:     len(m.expected),
		SessionID: m.sessionID,
	}

	if _, already := m.read[normalized]; already {
		result.ReadCount = len(m.read)
		return result
	}

	m.read[normalized] = struct{}{}
	result.NewlyRead = true
	result.ReadCount = len(m.read)
	return result
}

func (m *Manager) Stop(chatID int64) (StopResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.active {
		return StopResult{}, fmt.Errorf("aktiv test yo'q")
	}
	if m.ownerChatID != 0 && chatID != m.ownerChatID {
		return StopResult{}, fmt.Errorf("testni boshlagan chatgina to'xtata oladi")
	}

	total := len(m.expected)
	read := len(m.read)
	result := StopResult{
		SessionID: m.sessionID,
		FileName:  m.fileName,
		Total:     total,
		Read:      read,
		Unread:    total - read,
		ChatID:    m.ownerChatID,
	}

	m.active = false
	m.awaitingFile = false
	m.fileName = ""
	m.startedAt = time.Time{}
	m.expected = make(map[string]struct{})
	m.read = make(map[string]struct{})

	return result, nil
}

func (m *Manager) IsSessionActive(sessionID uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active && m.sessionID == sessionID
}

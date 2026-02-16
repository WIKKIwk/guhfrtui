package reader

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"new_era_go/internal/gobot/config"
	"new_era_go/sdk"
)

type EPCHandler func(epc string)
type Notifier func(text string)

type Status struct {
	Running      bool
	Connected    bool
	Endpoint     string
	LastError    string
	UniqueSeen   uint64
	LastTagAt    time.Time
	LastTagEPC   string
	LastStartAt  time.Time
	RestartCount uint64
}

type Manager struct {
	cfg      config.Config
	onEPC    EPCHandler
	notifyFn Notifier

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	status  Status
}

func New(cfg config.Config, onEPC EPCHandler, notify Notifier) *Manager {
	return &Manager{
		cfg:      cfg,
		onEPC:    onEPC,
		notifyFn: notify,
	}
}

func (m *Manager) SetNotifier(notify Notifier) {
	m.mu.Lock()
	m.notifyFn = notify
	m.mu.Unlock()
}

func (m *Manager) Start(parent context.Context) error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	m.running = true
	m.cancel = cancel
	m.done = make(chan struct{})
	m.status.Running = true
	m.status.LastError = ""
	m.status.LastStartAt = time.Now()
	done := m.done
	m.mu.Unlock()

	go m.scanLoop(ctx, done)
	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	done := m.done
	m.cancel = nil
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) StatusText() string {
	st := m.Status()
	return fmt.Sprintf(
		"running=%v connected=%v endpoint=%s\nseen=%d last_tag=%s at=%s\nrestarts=%d last_error=%s",
		st.Running,
		st.Connected,
		fallback(st.Endpoint, "-"),
		st.UniqueSeen,
		fallback(trimEPC(st.LastTagEPC), "-"),
		formatTime(st.LastTagAt),
		st.RestartCount,
		fallback(st.LastError, "-"),
	)
}

func (m *Manager) scanLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	defer m.finishStopped()

	retry := m.cfg.ReaderRetryDelay
	if retry < 500*time.Millisecond {
		retry = 2 * time.Second
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		client := sdk.NewClient()
		connected, err := m.connectAndStart(ctx, client)
		if err != nil {
			m.setError(err)
			log.Printf("[reader] start failed: %v", err)
			if !sleepWithContext(ctx, retry) {
				return
			}
			continue
		}

		if connected {
			m.notify("RFID scan boshlandi: " + m.Status().Endpoint)
		}

		shouldReconnect := m.consumeTags(ctx, client)
		_ = client.StopInventory()
		_ = client.Close()

		m.mu.Lock()
		m.status.Connected = false
		m.status.Endpoint = ""
		m.mu.Unlock()

		if !shouldReconnect {
			return
		}
		if !sleepWithContext(ctx, retry) {
			return
		}

		m.mu.Lock()
		m.status.RestartCount++
		m.mu.Unlock()
	}
}

func (m *Manager) connectAndStart(ctx context.Context, client *sdk.Client) (bool, error) {
	timeout := m.cfg.ReaderConnectTimeout
	if timeout <= 0 {
		timeout = 25 * time.Second
	}

	var endpoint string
	if m.cfg.ReaderHost != "" && m.cfg.ReaderPort > 0 {
		dialCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		target := sdk.Endpoint{Host: m.cfg.ReaderHost, Port: m.cfg.ReaderPort}
		if err := client.Reconnect(dialCtx, target, timeout); err != nil {
			return false, fmt.Errorf("direct connect %s:%d: %w", m.cfg.ReaderHost, m.cfg.ReaderPort, err)
		}
		endpoint = target.Address()
	} else {
		scanCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		candidates, err := client.Discover(scanCtx, sdk.DefaultScanOptions())
		if err != nil && len(candidates) == 0 {
			return false, fmt.Errorf("discover: %w", err)
		}
		index := -1
		for i, candidate := range candidates {
			if candidate.Verified {
				index = i
				break
			}
		}
		if index < 0 {
			if len(candidates) == 0 {
				return false, fmt.Errorf("discover: no reader endpoint found")
			}
			index = 0
			log.Printf("[reader] warning: verified endpoint topilmadi, fallback=%s:%d", candidates[index].Host, candidates[index].Port)
		}

		chosen := candidates[index]
		dialCtx, cancelDial := context.WithTimeout(ctx, timeout)
		defer cancelDial()
		target := sdk.Endpoint{Host: chosen.Host, Port: chosen.Port}
		if err := client.Reconnect(dialCtx, target, timeout); err != nil {
			return false, fmt.Errorf("connect %s:%d: %w", chosen.Host, chosen.Port, err)
		}
		endpoint = target.Address()
	}

	cfg := client.InventoryConfig()
	client.SetInventoryConfig(cfg)

	if err := client.StartInventory(ctx); err != nil {
		return false, fmt.Errorf("start inventory: %w", err)
	}

	m.mu.Lock()
	m.status.Connected = true
	m.status.Endpoint = endpoint
	m.status.LastError = ""
	m.mu.Unlock()
	return true, nil
}

func (m *Manager) consumeTags(ctx context.Context, client *sdk.Client) bool {
	tags := client.Tags()
	errs := client.Errors()

	for {
		select {
		case <-ctx.Done():
			return false
		case tag, ok := <-tags:
			if !ok {
				m.setError(fmt.Errorf("tag channel closed"))
				return true
			}
			if !tag.IsNew {
				continue
			}
			epc := strings.TrimSpace(tag.EPC)
			if epc == "" {
				continue
			}

			m.mu.Lock()
			m.status.UniqueSeen++
			m.status.LastTagAt = time.Now()
			m.status.LastTagEPC = epc
			m.mu.Unlock()

			if m.onEPC != nil {
				m.onEPC(epc)
			}
		case err, ok := <-errs:
			if !ok {
				m.setError(fmt.Errorf("error channel closed"))
				return true
			}
			if err != nil {
				m.setError(err)
				return true
			}
		}
	}
}

func (m *Manager) setError(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	m.status.LastError = err.Error()
	m.mu.Unlock()
}

func (m *Manager) finishStopped() {
	m.mu.Lock()
	m.running = false
	m.cancel = nil
	m.done = nil
	m.status.Running = false
	m.status.Connected = false
	m.mu.Unlock()
}

func (m *Manager) notify(text string) {
	text = strings.TrimSpace(text)
	if text == "" || m.notifyFn == nil {
		return
	}
	m.notifyFn(text)
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
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

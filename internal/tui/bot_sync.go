package tui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type botSyncClient struct {
	enabled    bool
	source     string
	socketPath string
	timeout    time.Duration
	queue      chan string

	errMu     sync.Mutex
	lastErrAt time.Time
}

var (
	botSyncOnce sync.Once
	botSyncInst *botSyncClient
)

func getBotSyncClient() *botSyncClient {
	botSyncOnce.Do(func() {
		botSyncInst = newBotSyncClient()
	})
	return botSyncInst
}

func newBotSyncClient() *botSyncClient {
	if !envBool("BOT_SYNC_ENABLED", true) {
		return &botSyncClient{}
	}

	timeoutMS := envInt("BOT_SYNC_TIMEOUT_MS", 1200)
	if timeoutMS < 200 {
		timeoutMS = 200
	}
	queueSize := envInt("BOT_SYNC_QUEUE_SIZE", 4096)
	if queueSize < 128 {
		queueSize = 128
	}

	c := &botSyncClient{
		enabled:    true,
		source:     envOr("BOT_SYNC_SOURCE", "st8508-tui"),
		socketPath: envOr("BOT_SYNC_SOCKET", envOr("BOT_IPC_SOCKET", "/tmp/rfid-go-bot.sock")),
		timeout:    time.Duration(timeoutMS) * time.Millisecond,
		queue:      make(chan string, queueSize),
	}
	go c.ingestWorker()
	return c
}

func (c *botSyncClient) ingestWorker() {
	for epc := range c.queue {
		if err := c.sendFrame(syncFrame{
			Type:   "epc",
			Source: c.source,
			EPC:    epc,
		}); err != nil {
			c.logErrorRateLimited("bot ingest failed", err)
		}
	}
}

func (c *botSyncClient) onStartReading() {
	if !c.enabled {
		return
	}
	go func() {
		if err := c.sendFrame(syncFrame{
			Type:   "scan_start",
			Source: c.source,
		}); err != nil {
			c.logErrorRateLimited("bot scan start failed", err)
		}
	}()
}

func (c *botSyncClient) onStopReading() {
	if !c.enabled {
		return
	}
	go func() {
		if err := c.sendFrame(syncFrame{
			Type:   "scan_stop",
			Source: c.source,
		}); err != nil {
			c.logErrorRateLimited("bot scan stop failed", err)
		}
	}()
}

func (c *botSyncClient) onNewEPC(epc string) {
	if !c.enabled {
		return
	}
	epc = strings.TrimSpace(epc)
	if epc == "" {
		return
	}
	select {
	case c.queue <- epc:
	default:
		c.logErrorRateLimited("bot ingest queue full", fmt.Errorf("dropped epc=%s", trimText(epc, 16)))
	}
}

func (c *botSyncClient) sendFrame(frame syncFrame) error {
	_, err := c.roundTrip(frame)
	return err
}

func (c *botSyncClient) status() (botRuntimeStats, error) {
	if !c.enabled {
		return botRuntimeStats{}, errors.New("bot sync disabled")
	}
	resp, err := c.roundTrip(syncFrame{
		Type:   "status",
		Source: c.source,
	})
	if err != nil {
		return botRuntimeStats{}, err
	}
	return resp.Stats, nil
}

func (c *botSyncClient) roundTrip(frame syncFrame) (syncResponse, error) {
	if !c.enabled {
		return syncResponse{}, errors.New("bot sync disabled")
	}

	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return syncResponse{}, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.timeout))

	body, _ := json.Marshal(frame)
	body = append(body, '\n')
	if _, err := conn.Write(body); err != nil {
		return syncResponse{}, err
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return syncResponse{}, err
	}

	var resp syncResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return syncResponse{}, err
	}
	if !resp.OK {
		if strings.TrimSpace(resp.Error) == "" {
			return syncResponse{}, errors.New("ipc response not ok")
		}
		return syncResponse{}, errors.New(resp.Error)
	}
	if strings.TrimSpace(resp.Warning) != "" {
		c.logErrorRateLimited("bot ipc warning", errors.New(resp.Warning))
	}
	return resp, nil
}

func (c *botSyncClient) logErrorRateLimited(prefix string, err error) {
	if err == nil {
		return
	}
	now := time.Now()
	c.errMu.Lock()
	if now.Sub(c.lastErrAt) < 3*time.Second {
		c.errMu.Unlock()
		return
	}
	c.lastErrAt = now
	c.errMu.Unlock()
	_ = prefix
	_ = err
}

type syncFrame struct {
	Type   string   `json:"type"`
	Source string   `json:"source,omitempty"`
	EPC    string   `json:"epc,omitempty"`
	EPCs   []string `json:"epcs,omitempty"`
}

type syncResponse struct {
	OK      bool            `json:"ok"`
	Error   string          `json:"error,omitempty"`
	Warning string          `json:"warning,omitempty"`
	Stats   botRuntimeStats `json:"stats"`
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

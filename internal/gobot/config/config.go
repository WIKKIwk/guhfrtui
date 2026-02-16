package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPEnabled          bool
	BotToken             string
	ERPURL               string
	ERPAPIKey            string
	ERPAPISecret         string
	HTTPAddr             string
	IPCEnabled           bool
	IPCSocket            string
	WebhookSecret        string
	RequestTimeout       time.Duration
	RefreshInterval      time.Duration
	SubmitRetry          int
	SubmitRetryDelay     time.Duration
	WorkerCount          int
	QueueSize            int
	RecentSeenTTL        time.Duration
	PollTimeout          time.Duration
	ScanBackend          string
	ScanDefaultActive    bool
	AutoScan             bool
	ReaderConnectTimeout time.Duration
	ReaderRetryDelay     time.Duration
	ReaderHost           string
	ReaderPort           int
}

func Load() (Config, error) {
	cfg := Config{
		HTTPEnabled:          envBool("BOT_HTTP_ENABLED", true),
		BotToken:             strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		ERPURL:               strings.TrimSpace(os.Getenv("ERP_URL")),
		ERPAPIKey:            strings.TrimSpace(os.Getenv("ERP_API_KEY")),
		ERPAPISecret:         strings.TrimSpace(os.Getenv("ERP_API_SECRET")),
		HTTPAddr:             envOr("BOT_HTTP_ADDR", ":8098"),
		IPCEnabled:           envBool("BOT_IPC_ENABLED", true),
		IPCSocket:            envOr("BOT_IPC_SOCKET", "/tmp/rfid-go-bot.sock"),
		WebhookSecret:        strings.TrimSpace(os.Getenv("BOT_WEBHOOK_SECRET")),
		RequestTimeout:       envDurationMS("BOT_HTTP_TIMEOUT_MS", 12_000),
		RefreshInterval:      envDurationSec("BOT_CACHE_REFRESH_SEC", 5),
		SubmitRetry:          envInt("BOT_SUBMIT_RETRY", 2),
		SubmitRetryDelay:     envDurationMS("BOT_SUBMIT_RETRY_MS", 300),
		WorkerCount:          envInt("BOT_WORKER_COUNT", 4),
		QueueSize:            envInt("BOT_QUEUE_SIZE", 2048),
		RecentSeenTTL:        envDurationSec("BOT_RECENT_SEEN_TTL_SEC", 600),
		PollTimeout:          envDurationSec("BOT_POLL_TIMEOUT_SEC", 25),
		ScanBackend:          strings.ToLower(envOr("BOT_SCAN_BACKEND", "hybrid")),
		ScanDefaultActive:    envBool("BOT_SCAN_DEFAULT_ACTIVE", true),
		AutoScan:             envBool("BOT_AUTO_SCAN", false),
		ReaderConnectTimeout: envDurationSec("BOT_READER_CONNECT_TIMEOUT_SEC", 25),
		ReaderRetryDelay:     envDurationSec("BOT_READER_RETRY_SEC", 2),
		ReaderHost:           strings.TrimSpace(os.Getenv("BOT_READER_HOST")),
		ReaderPort:           envInt("BOT_READER_PORT", 0),
	}

	cfg.ERPURL = strings.TrimRight(cfg.ERPURL, "/")
	if !cfg.HTTPEnabled {
		cfg.HTTPAddr = ""
	}
	if !cfg.IPCEnabled {
		cfg.IPCSocket = ""
	}

	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.ERPURL == "" || cfg.ERPAPIKey == "" || cfg.ERPAPISecret == "" {
		return Config{}, fmt.Errorf("ERP_URL, ERP_API_KEY, ERP_API_SECRET are required")
	}
	if cfg.SubmitRetry < 0 {
		cfg.SubmitRetry = 0
	}
	if cfg.WorkerCount < 1 {
		cfg.WorkerCount = 1
	}
	if cfg.QueueSize < 64 {
		cfg.QueueSize = 64
	}
	if cfg.RequestTimeout < time.Second {
		cfg.RequestTimeout = time.Second
	}
	if cfg.RefreshInterval < 5*time.Second {
		cfg.RefreshInterval = 5 * time.Second
	}
	if cfg.PollTimeout < 5*time.Second {
		cfg.PollTimeout = 5 * time.Second
	}
	if cfg.PollTimeout > 55*time.Second {
		cfg.PollTimeout = 55 * time.Second
	}
	if cfg.RecentSeenTTL < 30*time.Second {
		cfg.RecentSeenTTL = 30 * time.Second
	}
	switch cfg.ScanBackend {
	case "ingest", "sdk", "hybrid":
	default:
		cfg.ScanBackend = "ingest"
	}
	if cfg.ReaderConnectTimeout < 5*time.Second {
		cfg.ReaderConnectTimeout = 5 * time.Second
	}
	if cfg.ReaderRetryDelay < 500*time.Millisecond {
		cfg.ReaderRetryDelay = 2 * time.Second
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return fallback
	}
	return val
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

func envDurationSec(key string, fallbackSec int) time.Duration {
	return time.Duration(envInt(key, fallbackSec)) * time.Second
}

func envDurationMS(key string, fallbackMS int) time.Duration {
	return time.Duration(envInt(key, fallbackMS)) * time.Millisecond
}

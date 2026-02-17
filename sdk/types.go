package sdk

import (
	"net"
	"strconv"
	"time"

	reader18 "new_era_go/internal/protocol/reader18"
)

// Endpoint is a public network address of reader.
type Endpoint struct {
	Host string
	Port int
}

func (e Endpoint) Address() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// ScanOptions controls LAN discovery behavior in SDK API.
type ScanOptions struct {
	Ports                 []int
	Timeout               time.Duration
	Concurrency           int
	HostLimitPerInterface int
}

// Candidate is one discovered endpoint with scoring/verification metadata.
type Candidate struct {
	Host          string
	Port          int
	Score         int
	Banner        string
	Reason        string
	Verified      bool
	ReaderAddress byte
	Protocol      string
}

// InventoryConfig controls how the reader performs inventory polling.
type InventoryConfig struct {
	ReaderAddress      byte
	AutoAddress        bool
	QValue             byte
	Session            byte
	Target             byte
	AntennaMask        byte
	ScanTime           byte
	PollInterval       time.Duration
	OutputPower        byte
	RegionSet          bool
	RegionHigh         byte
	RegionLow          byte
	PerAntennaPower    []byte
	NoTagABSwitch      int
	SingleFallbackEach int
}

// DefaultInventoryConfig returns a balanced low-latency configuration.
func DefaultInventoryConfig() InventoryConfig {
	return InventoryConfig{
		ReaderAddress:      reader18.DefaultReaderAddress,
		AutoAddress:        true,
		QValue:             0x04,
		Session:            0x01,
		Target:             0x00,
		AntennaMask:        0x01,
		ScanTime:           0x01,
		PollInterval:       40 * time.Millisecond,
		OutputPower:        0x1E,
		NoTagABSwitch:      4,
		SingleFallbackEach: 6,
	}
}

// EffectiveInterval is the real inventory cycle; firmware scan-time is a hard lower bound.
func (c InventoryConfig) EffectiveInterval() time.Duration {
	min := time.Duration(c.ScanTime) * 100 * time.Millisecond
	if min < 40*time.Millisecond {
		min = 40 * time.Millisecond
	}
	if c.PollInterval > min {
		return c.PollInterval
	}
	return min
}

func normalizeConfig(cfg InventoryConfig) InventoryConfig {
	cfg = cloneInventoryConfig(cfg)
	if cfg.AntennaMask == 0 {
		cfg.AntennaMask = 0x01
	}
	if cfg.ScanTime == 0 {
		cfg.ScanTime = 0x01
	}
	if cfg.OutputPower > 0x1E {
		cfg.OutputPower = 0x1E
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 40 * time.Millisecond
	}
	if cfg.SingleFallbackEach < 0 {
		cfg.SingleFallbackEach = 0
	}
	if cfg.NoTagABSwitch < 0 {
		cfg.NoTagABSwitch = 0
	}
	for i := range cfg.PerAntennaPower {
		if cfg.PerAntennaPower[i] > 0x1E {
			cfg.PerAntennaPower[i] = 0x1E
		}
	}
	return cfg
}

func cloneInventoryConfig(cfg InventoryConfig) InventoryConfig {
	out := cfg
	if len(cfg.PerAntennaPower) > 0 {
		out.PerAntennaPower = append([]byte(nil), cfg.PerAntennaPower...)
	}
	return out
}

// TagEvent is one decoded EPC read event.
type TagEvent struct {
	When       time.Time
	Source     string
	EPC        string
	Antenna    int
	RSSI       int
	IsNew      bool
	Rounds     int
	UniqueTags int
}

// StatusEvent is a lightweight progress signal from SDK loops.
type StatusEvent struct {
	When    time.Time
	Message string
}

// Stats captures current inventory counters.
type Stats struct {
	Running     bool
	Rounds      int
	UniqueTags  int
	LastTagEPC  string
	ReaderAddr  byte
	TargetValue byte
}

package sdk

import (
	"context"
	"sync"

	"new_era_go/internal/reader"
)

// Client is a high-level ST-8508/Reader18 SDK facade for Go applications.
type Client struct {
	transport *reader.Client

	mu            sync.RWMutex
	cfg           InventoryConfig
	inventoryOn   bool
	inventoryDone chan struct{}
	cancelInv     context.CancelFunc
	seen          map[string]struct{}
	parserBuffer  []byte
	rounds        int
	uniqueTags    int
	noTagHit      int
	antIdx        int
	readerAddr    byte
	targetValue   byte
	lastTagEPC    string

	tags     chan TagEvent
	statuses chan StatusEvent
	errs     chan error
}

func NewClient() *Client {
	cfg := normalizeConfig(DefaultInventoryConfig())
	return &Client{
		transport:   reader.NewClient(),
		cfg:         cfg,
		seen:        make(map[string]struct{}),
		readerAddr:  cfg.ReaderAddress,
		targetValue: cfg.Target,
		tags:        make(chan TagEvent, 256),
		statuses:    make(chan StatusEvent, 256),
		errs:        make(chan error, 64),
	}
}

func (c *Client) Tags() <-chan TagEvent {
	return c.tags
}

func (c *Client) Statuses() <-chan StatusEvent {
	return c.statuses
}

func (c *Client) Errors() <-chan error {
	return c.errs
}

func (c *Client) IsConnected() bool {
	return c.transport.IsConnected()
}

func (c *Client) Endpoint() (Endpoint, bool) {
	endpoint, ok := c.transport.Endpoint()
	if !ok {
		return Endpoint{}, false
	}
	return Endpoint{Host: endpoint.Host, Port: endpoint.Port}, true
}

func (c *Client) InventoryConfig() InventoryConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneInventoryConfig(c.cfg)
}

func (c *Client) SetInventoryConfig(cfg InventoryConfig) {
	cfg = normalizeConfig(cfg)
	c.mu.Lock()
	c.cfg = cloneInventoryConfig(cfg)
	if c.readerAddr == 0 {
		c.readerAddr = cfg.ReaderAddress
	}
	c.mu.Unlock()
	c.emitStatus("inventory config updated")
}

package sdk

import (
	"context"
	"fmt"
	"time"

	reader18 "new_era_go/internal/protocol/reader18"
)

func (c *Client) ApplyInventoryConfig(ctx context.Context) error {
	if !c.transport.IsConnected() {
		return fmt.Errorf("not connected")
	}

	cfg, addr := c.snapshotConfig()
	commands := make([][]byte, 0, 7)
	commands = append(commands, reader18.SetWorkModeCommand(addr, []byte{0x00}))
	if cfg.RegionSet {
		commands = append(commands, reader18.SetFrequencyRangeCommand(addr, cfg.RegionHigh, cfg.RegionLow))
	}
	commands = append(commands,
		reader18.SetScanTimeCommand(addr, cfg.ScanTime),
		reader18.SetAntennaMuxCommand(addr, cfg.AntennaMask),
	)
	if len(cfg.PerAntennaPower) > 0 {
		commands = append(commands, reader18.SetOutputPowerByAntCommand(addr, cfg.PerAntennaPower))
	}
	// Always send global power as fallback after optional per-antenna settings.
	commands = append(commands, reader18.SetOutputPowerCommand(addr, cfg.OutputPower))

	for _, command := range commands {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if err := c.transport.SendRaw(command, 2*time.Second); err != nil {
			return err
		}
	}
	return nil
}

// StartInventory starts continuous inventory loops and streams events via channels.
func (c *Client) StartInventory(ctx context.Context) error {
	if !c.transport.IsConnected() {
		return fmt.Errorf("not connected")
	}

	c.mu.Lock()
	if c.inventoryOn {
		c.mu.Unlock()
		return fmt.Errorf("inventory already running")
	}
	c.inventoryOn = true
	c.inventoryDone = make(chan struct{})
	c.seen = make(map[string]struct{})
	c.parserBuffer = nil
	c.rounds = 0
	c.uniqueTags = 0
	c.noTagHit = 0
	c.antIdx = 0
	c.lastTagEPC = ""
	c.targetValue = c.cfg.Target
	if c.readerAddr == 0 {
		c.readerAddr = c.cfg.ReaderAddress
	}
	invCtx, cancel := context.WithCancel(ctx)
	c.cancelInv = cancel
	done := c.inventoryDone
	c.mu.Unlock()

	if err := c.ApplyInventoryConfig(invCtx); err != nil {
		c.mu.Lock()
		if c.cancelInv != nil {
			c.cancelInv()
		}
		c.cancelInv = nil
		c.inventoryOn = false
		close(done)
		c.mu.Unlock()
		return err
	}

	c.emitStatus("inventory started")

	go c.inventoryRun(invCtx)
	return nil
}

func (c *Client) StopInventory() error {
	c.mu.Lock()
	if !c.inventoryOn {
		c.mu.Unlock()
		return nil
	}
	cancel := c.cancelInv
	done := c.inventoryDone
	c.cancelInv = nil
	c.inventoryOn = false
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	c.emitStatus("inventory stopped")
	return nil
}

func (c *Client) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Stats{
		Running:     c.inventoryOn,
		Rounds:      c.rounds,
		UniqueTags:  c.uniqueTags,
		LastTagEPC:  c.lastTagEPC,
		ReaderAddr:  c.readerAddr,
		TargetValue: c.targetValue,
	}
}

func (c *Client) SendRaw(payload []byte) error {
	if !c.transport.IsConnected() {
		return fmt.Errorf("not connected")
	}
	return c.transport.SendRaw(payload, 2*time.Second)
}

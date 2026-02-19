package sdk

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	reader18 "new_era_go/internal/protocol/reader18"
)

func (c *Client) inventoryRun(ctx context.Context) {
	defer c.finishInventoryRun()

	packets := c.transport.Packets()
	errorsCh := c.transport.Errors()
	if packets == nil {
		c.emitErr(fmt.Errorf("packet channel unavailable"))
		return
	}
	if errorsCh == nil {
		c.emitErr(fmt.Errorf("error channel unavailable"))
		return
	}

	go c.inventoryTxLoop(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case packet, ok := <-packets:
			if !ok {
				c.emitErr(fmt.Errorf("reader packet channel closed"))
				return
			}
			c.consumePacket(packet.Data)
		case err, ok := <-errorsCh:
			if !ok {
				c.emitErr(fmt.Errorf("reader error channel closed"))
				return
			}
			if err != nil {
				c.emitErr(err)
				return
			}
		}
	}
}

func (c *Client) inventoryTxLoop(ctx context.Context) {
	for {
		command, single, interval, ok := c.nextInventoryCommand()
		if !ok {
			return
		}
		if err := c.transport.SendRaw(command, 2*time.Second); err != nil {
			c.emitErr(err)
			c.stopInventoryAsync()
			return
		}
		if single != nil {
			if err := c.transport.SendRaw(single, 2*time.Second); err != nil {
				c.emitErr(err)
				c.stopInventoryAsync()
				return
			}
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (c *Client) consumePacket(data []byte) {
	c.mu.Lock()
	c.parserBuffer = append(c.parserBuffer, data...)
	if len(c.parserBuffer) > 8192 {
		c.parserBuffer = append([]byte{}, c.parserBuffer[len(c.parserBuffer)-4096:]...)
	}
	frames, remaining := reader18.ParseFrames(c.parserBuffer)
	c.parserBuffer = remaining
	c.mu.Unlock()

	for _, frame := range frames {
		c.consumeFrame(frame)
	}
}

func (c *Client) consumeFrame(frame reader18.Frame) {
	c.mu.Lock()
	if c.cfg.AutoAddress {
		c.readerAddr = frame.Address
	}
	c.mu.Unlock()

	switch frame.Command {
	case reader18.CmdInventory:
		c.handleInventoryFrame(frame)
	case reader18.CmdInventorySingle:
		c.handleInventorySingleFrame(frame)
	case reader18.CmdGetReaderInfo:
		c.emitStatus("reader info received")
	}
}

func (c *Client) handleInventoryFrame(frame reader18.Frame) {
	tags, err := reader18.ParseInventoryG2Tags(frame)
	if err != nil {
		if strings.Contains(err.Error(), "truncated") || strings.Contains(err.Error(), "invalid") {
			c.emitErr(err)
		}
		return
	}
	if len(tags) > 0 {
		for _, tag := range tags {
			c.recordTag("inventory-g2", tag.Antenna, tag.RSSI, tag.EPC)
		}
		return
	}

	if frame.Status == reader18.StatusSuccess {
		count, err := reader18.InventoryTagCount(frame)
		if err == nil && count > 0 {
			c.emitStatus(fmt.Sprintf("count-only inventory response: %d", count))
		}
		return
	}
	c.observeNoTag(frame.Status)
}

func (c *Client) handleInventorySingleFrame(frame reader18.Frame) {
	result, err := reader18.ParseSingleInventoryResult(frame)
	if err != nil {
		return
	}
	if result.TagCount > 0 && len(result.EPC) > 0 {
		c.recordTag("inventory-single", int(result.Antenna), 0, result.EPC)
		return
	}
	c.observeNoTag(frame.Status)
}

func (c *Client) observeNoTag(status byte) {
	switch status {
	case reader18.StatusNoTag, reader18.StatusNoTagOrTimeout, 0x02, 0x03, 0x04:
	default:
		return
	}

	var switched bool
	var target byte
	c.mu.Lock()
	c.noTagHit++
	if c.cfg.Session > 1 && c.cfg.NoTagABSwitch > 0 && c.noTagHit >= c.cfg.NoTagABSwitch {
		c.targetValue ^= 0x01
		c.noTagHit = 0
		switched = true
		target = c.targetValue
	}
	c.mu.Unlock()

	if switched {
		c.emitStatus("target switched to " + targetLabel(target))
	}
}

func (c *Client) recordTag(source string, antenna int, rssi int, epc []byte) {
	epcText := strings.ToUpper(hex.EncodeToString(epc))
	if epcText == "" {
		return
	}

	c.mu.Lock()
	c.noTagHit = 0
	_, exists := c.seen[epcText]
	if !exists {
		c.seen[epcText] = struct{}{}
		c.uniqueTags++
	}
	c.lastTagEPC = epcText
	rounds := c.rounds
	unique := c.uniqueTags
	c.mu.Unlock()

	c.emitTag(TagEvent{
		When:       time.Now(),
		Source:     source,
		EPC:        epcText,
		Antenna:    antenna,
		RSSI:       rssi,
		IsNew:      !exists,
		Rounds:     rounds,
		UniqueTags: unique,
	})
}

func (c *Client) nextInventoryCommand() (inventory []byte, single []byte, interval time.Duration, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.inventoryOn {
		return nil, nil, 0, false
	}
	cfg := normalizeConfig(c.cfg)
	c.cfg = cfg
	c.rounds++

	antenna, nextIdx := nextInventoryAntenna(cfg.AntennaMask, c.antIdx)
	c.antIdx = nextIdx

	inventory = reader18.InventoryG2Command(
		c.readerAddr,
		cfg.QValue,
		cfg.Session,
		0x00,
		0x00,
		c.targetValue,
		antenna,
		cfg.ScanTime,
	)
	if cfg.SingleFallbackEach > 0 && c.rounds%cfg.SingleFallbackEach == 0 {
		single = reader18.InventorySingleTagCommand(c.readerAddr)
	}
	return inventory, single, cfg.EffectiveInterval(), true
}

func (c *Client) snapshotConfig() (InventoryConfig, byte) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneInventoryConfig(c.cfg), c.readerAddr
}

func (c *Client) currentReaderAddress() byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.readerAddr
}

func (c *Client) finishInventoryRun() {
	c.mu.Lock()
	done := c.inventoryDone
	c.inventoryDone = nil
	c.cancelInv = nil
	c.inventoryOn = false
	c.mu.Unlock()
	if done != nil {
		close(done)
	}
}

func (c *Client) stopInventoryAsync() {
	c.mu.Lock()
	cancel := c.cancelInv
	c.cancelInv = nil
	c.inventoryOn = false
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func nextInventoryAntenna(mask byte, start int) (byte, int) {
	if mask == 0 {
		mask = 0x01
	}
	start = ((start % 8) + 8) % 8
	for i := 0; i < 8; i++ {
		idx := (start + i) % 8
		if mask&(byte(1)<<idx) != 0 {
			return byte(0x80 | idx), (idx + 1) % 8
		}
	}
	return 0x80, start
}

func targetLabel(target byte) string {
	if target&0x01 == 0 {
		return "A"
	}
	return "B"
}

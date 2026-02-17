package sdk

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"new_era_go/internal/discovery"
	reader18 "new_era_go/internal/protocol/reader18"
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

func DefaultScanOptions() ScanOptions {
	opts := discovery.DefaultOptions()
	return ScanOptions{
		Ports:                 append([]int{}, opts.Ports...),
		Timeout:               opts.Timeout,
		Concurrency:           opts.Concurrency,
		HostLimitPerInterface: opts.HostLimitPerInterface,
	}
}

// Discover scans LAN for probable reader endpoints.
func (c *Client) Discover(ctx context.Context, opts ScanOptions) ([]Candidate, error) {
	internalOpts := toInternalScanOptions(opts)
	internalCandidates, err := discovery.Scan(ctx, internalOpts)
	candidates := make([]Candidate, 0, len(internalCandidates))
	for _, candidate := range internalCandidates {
		candidates = append(candidates, fromInternalCandidate(candidate))
	}
	return candidates, err
}

// QuickConnect scans and connects to best candidate (verified-first).
func (c *Client) QuickConnect(ctx context.Context, opts ScanOptions) (Candidate, error) {
	candidates, err := c.Discover(ctx, opts)
	if err != nil && len(candidates) == 0 {
		return Candidate{}, err
	}
	if len(candidates) == 0 {
		return Candidate{}, fmt.Errorf("no reader candidate found")
	}

	index := 0
	for i, candidate := range candidates {
		if candidate.Verified {
			index = i
			break
		}
	}
	chosen := candidates[index]

	timeout := 3 * time.Second
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	if err := c.Reconnect(ctx, Endpoint{Host: chosen.Host, Port: chosen.Port}, timeout); err != nil {
		return chosen, err
	}

	if chosen.Verified {
		c.mu.Lock()
		c.readerAddr = chosen.ReaderAddress
		c.mu.Unlock()
	}
	c.emitStatus(fmt.Sprintf("connected to %s:%d", chosen.Host, chosen.Port))
	return chosen, nil
}

func (c *Client) Connect(ctx context.Context, endpoint Endpoint, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	internalEndpoint := reader.Endpoint{Host: endpoint.Host, Port: endpoint.Port}
	if err := c.transport.Connect(ctx, internalEndpoint, timeout); err != nil {
		return err
	}
	c.emitStatus("connected: " + endpoint.Address())
	return nil
}

func (c *Client) Reconnect(ctx context.Context, endpoint Endpoint, timeout time.Duration) error {
	_ = c.StopInventory()
	_ = c.transport.Disconnect()
	return c.Connect(ctx, endpoint, timeout)
}

func (c *Client) Disconnect() error {
	_ = c.StopInventory()
	err := c.transport.Disconnect()
	if err == nil {
		c.emitStatus("disconnected")
	}
	return err
}

func (c *Client) Close() error {
	return c.Disconnect()
}

// ProbeInfo sends GetReaderInfo command.
func (c *Client) ProbeInfo() error {
	if !c.transport.IsConnected() {
		return fmt.Errorf("not connected")
	}
	addr := c.currentReaderAddress()
	return c.transport.SendRaw(reader18.GetReaderInfoCommand(addr), 2*time.Second)
}

// ApplyInventoryConfig sends inventory-related configuration commands to reader.
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

func (c *Client) emitTag(event TagEvent) {
	select {
	case c.tags <- event:
	default:
	}
}

func (c *Client) emitStatus(message string) {
	select {
	case c.statuses <- StatusEvent{When: time.Now(), Message: message}:
	default:
	}
}

func (c *Client) emitErr(err error) {
	if err == nil {
		return
	}
	select {
	case c.errs <- err:
	default:
	}
}

func toInternalScanOptions(opts ScanOptions) discovery.ScanOptions {
	defaults := discovery.DefaultOptions()
	if len(opts.Ports) == 0 {
		opts.Ports = defaults.Ports
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaults.Timeout
	}
	if opts.Concurrency <= 0 {
		opts.Concurrency = defaults.Concurrency
	}
	if opts.HostLimitPerInterface <= 0 {
		opts.HostLimitPerInterface = defaults.HostLimitPerInterface
	}
	return discovery.ScanOptions{
		Ports:                 append([]int{}, opts.Ports...),
		Timeout:               opts.Timeout,
		Concurrency:           opts.Concurrency,
		HostLimitPerInterface: opts.HostLimitPerInterface,
	}
}

func fromInternalCandidate(candidate discovery.Candidate) Candidate {
	return Candidate{
		Host:          candidate.Host,
		Port:          candidate.Port,
		Score:         candidate.Score,
		Banner:        candidate.Banner,
		Reason:        candidate.Reason,
		Verified:      candidate.Verified,
		ReaderAddress: candidate.ReaderAddress,
		Protocol:      candidate.Protocol,
	}
}

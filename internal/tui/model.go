package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"new_era_go/internal/discovery"
	reader18 "new_era_go/internal/protocol/reader18"
	"new_era_go/internal/reader"
	"new_era_go/internal/regions"
)

func NewModel() Model {
	in := textinput.New()
	in.Prompt = "HEX> "
	in.Placeholder = "04 00 21 D9 6A"
	in.CharLimit = 4096
	in.Width = 56

	regionIdx := regions.DefaultIndex()

	opts := discovery.DefaultOptions()

	return Model{
		reader:            reader.NewClient(),
		activeScreen:      screenHome,
		homeIndex:         0,
		deviceIndex:       0,
		controlIndex:      0,
		inventoryIndex:    0,
		regionIndex:       regionIdx,
		regionCursor:      regionIdx,
		logScroll:         0,
		pendingConnect:    true,
		pendingAction:     0,
		scanOptions:       opts,
		scanning:          true,
		candidates:        []discovery.Candidate{},
		lastScanTime:      0,
		input:             in,
		inputMode:         inputModeNone,
		status:            "Startup scan running...",
		logs:              []string{"[startup] scan requested"},
		botSocket:         envOr("BOT_SYNC_SOCKET", envOr("BOT_IPC_SOCKET", "/tmp/rfid-go-bot.sock")),
		rxBytes:           0,
		txBytes:           0,
		lastRX:            "",
		inventoryRunning:  false,
		inventoryInterval: 60 * time.Millisecond,
		inventoryAddress:  reader18.DefaultReaderAddress,
		inventoryAutoAddr: true,
		inventoryQValue:   0x04,
		inventorySession:  0x01,
		inventoryTarget:   0x00,
		inventoryAntenna:  0x80,
		inventoryAntMask:  0x01,
		inventoryScanTime: 0x01,
		inventoryNoTagAB:  4,
		inventoryNoTagHit: 0,
		showPhaseFreq:     false,
		lastTagAntenna:    0,
		lastTagRSSI:       0,
		inventoryRounds:   0,
		inventoryTagTotal: 0,
		inventoryFreqIdx:  0,
		inventoryAntIdx:   0,
		lastTagEPC:        "",
		seenTagEPC:        make(map[string]struct{}),
		protocolBuffer:    nil,
		lastRawLogAt:      time.Time{},
		awaitingProbe:     false,
		width:             0,
		height:            0,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		runScanCmd(m.scanOptions),
		botStatusTickCmd(300*time.Millisecond),
	)
}

func (m Model) effectiveInventoryInterval() time.Duration {
	minDelay := time.Duration(m.inventoryScanTime) * 100 * time.Millisecond
	if minDelay < 40*time.Millisecond {
		minDelay = 40 * time.Millisecond
	}
	if m.inventoryInterval > minDelay {
		return m.inventoryInterval
	}
	return minDelay
}

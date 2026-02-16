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
		rxBytes:           0,
		txBytes:           0,
		lastRX:            "",
		inventoryRunning:  false,
		inventoryInterval: 220 * time.Millisecond,
		inventoryAddress:  reader18.DefaultReaderAddress,
		inventoryAutoAddr: true,
		inventoryRounds:   0,
		inventoryTagTotal: 0,
		inventoryFreqIdx:  0,
		lastTagEPC:        "",
		protocolBuffer:    nil,
		lastRawLogAt:      time.Time{},
		awaitingProbe:     false,
		width:             0,
		height:            0,
	}
}

func (m Model) Init() tea.Cmd {
	return runScanCmd(m.scanOptions)
}

package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"

	"new_era_go/internal/discovery"
	"new_era_go/internal/reader"
)

type screen int

const (
	screenHome screen = iota
	screenDevices
	screenControl
	screenInventory
	screenRegions
	screenLogs
	screenHelp
)

type inputMode int

const (
	inputModeNone inputMode = iota
	inputModeRawHex
)

type menuItem struct {
	Label string
	Desc  string
}

type frequencyWindow struct {
	High byte
	Low  byte
}

var inventoryFrequencyWindows = []frequencyWindow{
	{High: 0x3E, Low: 0x28},
	{High: 0x37, Low: 0x05},
	{High: 0x3E, Low: 0x00},
}

var homeMenu = []menuItem{
	{Label: "Quick Connect", Desc: "Scan LAN and connect to best reader"},
	{Label: "Devices", Desc: "Browse discovered reader endpoints"},
	{Label: "Control", Desc: "Start/stop reading and run commands"},
	{Label: "Inventory Tune", Desc: "Q/session/target/antenna performance settings"},
	{Label: "Regions", Desc: "Choose RF region preset"},
	{Label: "Logs", Desc: "Inspect recent events"},
	{Label: "Help", Desc: "Show key guide"},
}

var controlMenu = []menuItem{
	{Label: "Start Reading", Desc: "Inventory loop on"},
	{Label: "Stop Reading", Desc: "Inventory loop off"},
	{Label: "Probe Reader Info", Desc: "Send command 0x21"},
	{Label: "Send Raw Hex", Desc: "Manual packet input"},
	{Label: "Disconnect", Desc: "Close TCP connection"},
	{Label: "Rescan + Quick Connect", Desc: "Find and reconnect"},
	{Label: "Clear Logs", Desc: "Keep only new events"},
	{Label: "Inventory Tune", Desc: "Open inventory parameter page"},
	{Label: "Back To Home", Desc: "Return to home page"},
}

type scanFinishedMsg struct {
	Candidates []discovery.Candidate
	Err        error
	Duration   time.Duration
}

type connectFinishedMsg struct {
	Endpoint reader.Endpoint
	Err      error
}

type disconnectFinishedMsg struct {
	Err error
}

type commandSentMsg struct {
	Name string
	Sent int
	Err  error
}

type inventoryTickMsg struct{}
type probeTimeoutMsg struct{}

type packetMsg struct {
	Packet reader.Packet
}

type packetChannelClosedMsg struct{}

type readerErrMsg struct {
	Err error
}

type readerErrChannelClosedMsg struct{}

type botStatusMsg struct {
	Stats botRuntimeStats
	Err   error
	At    time.Time
}

type botRuntimeStats struct {
	CacheSize      int       `json:"cache_size"`
	DraftCount     int       `json:"draft_count"`
	LastRefreshAt  time.Time `json:"last_refresh_at"`
	LastRefreshOK  bool      `json:"last_refresh_ok"`
	ScanActive     bool      `json:"scan_active"`
	ScanSince      time.Time `json:"scan_since"`
	SeenTotal      uint64    `json:"seen_total"`
	CacheHits      uint64    `json:"cache_hits"`
	CacheMisses    uint64    `json:"cache_misses"`
	SubmittedOK    uint64    `json:"submitted_ok"`
	SubmitNotFound uint64    `json:"submit_not_found"`
	SubmitErrors   uint64    `json:"submit_errors"`
	QueueDropped   uint64    `json:"queue_dropped"`
	ScanInactive   uint64    `json:"scan_inactive"`
}

// Model is the app state.
type Model struct {
	reader *reader.Client

	activeScreen   screen
	homeIndex      int
	deviceIndex    int
	controlIndex   int
	inventoryIndex int
	regionIndex    int
	regionCursor   int
	logScroll      int
	pendingConnect bool
	pendingAction  int

	scanOptions  discovery.ScanOptions
	scanning     bool
	candidates   []discovery.Candidate
	lastScanTime time.Duration

	input     textinput.Model
	inputMode inputMode

	status string
	logs   []string

	botOnline   bool
	botStats    botRuntimeStats
	botLastSync time.Time
	botLastErr  string
	botSocket   string

	connectQueue       []reader.Endpoint
	connectAttempt     int
	connectActionLabel string
	connecting         bool

	rxBytes int
	txBytes int
	lastRX  string

	inventoryRunning  bool
	inventoryInterval time.Duration
	inventoryAddress  byte
	inventoryAutoAddr bool
	inventoryQValue   byte
	inventorySession  byte
	inventoryTarget   byte
	inventoryAntenna  byte
	inventoryAntMask  byte
	inventoryScanTime byte
	inventoryNoTagAB  int
	inventoryNoTagHit int
	showPhaseFreq     bool
	lastTagAntenna    int
	lastTagRSSI       int
	inventoryRounds   int
	inventoryTagTotal int
	inventoryFreqIdx  int
	inventoryAntIdx   int
	lastTagEPC        string
	seenTagEPC        map[string]struct{}
	protocolBuffer    []byte
	lastRawLogAt      time.Time
	awaitingProbe     bool

	width  int
	height int
}

const noPendingAction = -1

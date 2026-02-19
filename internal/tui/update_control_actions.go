package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	reader18 "new_era_go/internal/protocol/reader18"
	tuiupdate "new_era_go/internal/tui/update"
)

func (m Model) updateControlKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.controlIndex = (m.controlIndex - 1 + len(controlMenu)) % len(controlMenu)
		return m, nil
	case "down", "j":
		m.controlIndex = (m.controlIndex + 1) % len(controlMenu)
		return m, nil
	case "/":
		return m.enterRawMode()
	case "enter":
		return m.runControlAction(m.controlIndex)
	}

	if idx, ok := parseDigit(msg.String()); ok && idx < len(controlMenu) {
		m.controlIndex = idx
		return m.runControlAction(idx)
	}

	return m, nil
}
func (m Model) runControlAction(index int) (tea.Model, tea.Cmd) {
	switch index {
	case 0:
		if !m.reader.IsConnected() {
			return m.requestConnectionForAction(0, "Start Reading")
		}
		if m.inventoryRunning {
			m.status = "Reading already running"
			return m, nil
		}
		m.inventoryRunning = true
		m.inventoryRounds = 0
		m.inventoryTagTotal = 0
		m.inventoryFreqIdx = 0
		m.inventoryNoTagHit = 0
		m.inventoryAntIdx = 0
		if m.inventoryAntMask == 0 {
			m.inventoryAntMask = 0x01
		}
		m.lastTagEPC = ""
		m.lastTagAntenna = 0
		m.lastTagRSSI = 0
		m.seenTagEPC = make(map[string]struct{})
		m.inventoryAutoAddr = true
		m.protocolBuffer = nil
		m.status = "Preparing reader + reading started"
		m.pushLog(fmt.Sprintf("reading started (poll=%s effective=%s scan=%d)", m.inventoryInterval, m.effectiveInventoryInterval(), m.inventoryScanTime))
		getBotSyncClient().onStartReading()
		return m, tea.Batch(
			sendNamedCmdSilent(m.reader, "cfg-work-mode", reader18.SetWorkModeCommand(m.inventoryAddress, []byte{0x00})),
			sendNamedCmdSilent(m.reader, "cfg-scan-time", reader18.SetScanTimeCommand(m.inventoryAddress, m.inventoryScanTime)),
			sendNamedCmdSilent(m.reader, "cfg-ant-mask", reader18.SetAntennaMuxCommand(m.inventoryAddress, m.inventoryAntMask)),
			sendNamedCmdSilent(m.reader, "cfg-power", reader18.SetOutputPowerCommand(m.inventoryAddress, 0x1E)),
			inventoryTickCmd(m.effectiveInventoryInterval()),
		)

	case 1:
		if !m.inventoryRunning {
			m.status = "Reading already stopped"
			return m, nil
		}
		m.inventoryRunning = false
		m.status = fmt.Sprintf("Reading stopped. rounds=%d tags=%d", m.inventoryRounds, m.inventoryTagTotal)
		m.pushLog("reading stopped")
		getBotSyncClient().onStopReading()
		return m, nil

	case 2:
		if !m.reader.IsConnected() {
			return m.requestConnectionForAction(2, "Probe Reader Info")
		}
		packet := reader18.GetReaderInfoCommand(m.inventoryAddress)
		m.status = "Sending GetReaderInfo"
		return m, sendNamedCmd(m.reader, "probe-info", packet)

	case 3:
		if !m.reader.IsConnected() {
			return m.requestConnectionForAction(3, "Raw Command")
		}
		return m.enterRawMode()

	case 4:
		if !m.reader.IsConnected() {
			m.status = "Reader already disconnected"
			return m, nil
		}
		m.status = "Disconnecting..."
		return m, disconnectCmd(m.reader)

	case 5:
		m.pendingConnect = true
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		if m.scanning {
			m.status = "Scan already running, quick-connect queued"
			return m, nil
		}
		m.scanning = true
		m.status = "Rescanning..."
		m.pushLog("rescan + quick-connect")
		return m, runScanCmd(m.scanOptions)

	case 6:
		m.logs = nil
		m.logScroll = 0
		m.status = "Logs cleared"
		return m, nil

	case 7:
		m.activeScreen = screenInventory
		m.status = "Inventory Tune"
		return m, nil

	case 8:
		m.activeScreen = screenHome
		m.status = "Home"
		return m, nil
	}

	return m, nil
}

func (m Model) enterRawMode() (tea.Model, tea.Cmd) {
	if !m.reader.IsConnected() {
		m.status = "Reader not connected"
		return m, nil
	}
	m.inputMode = inputModeRawHex
	m.input.Focus()
	m.status = "Raw mode: enter hex and press Enter"
	if strings.TrimSpace(m.input.Value()) == "" {
		m.input.SetValue("")
	}
	return m, nil
}

func (m Model) requestConnectionForAction(action int, actionName string) (tea.Model, tea.Cmd) {
	m.pendingAction = action
	if m.reader.IsConnected() {
		m.status = "Connected. Running action..."
		return m, nil
	}

	if len(m.candidates) > 0 {
		idx := tuiupdate.PreferredCandidateIndex(m.candidates)
		if idx < 0 {
			if m.scanning {
				m.pendingConnect = true
				m.status = fmt.Sprintf("%s requested: waiting for reader...", actionName)
				m.pushLog("pending action waiting for reader")
				return m, nil
			}
			m.pendingConnect = true
			m.scanning = true
			m.status = fmt.Sprintf("%s requested: rescanning for reader...", actionName)
			m.pushLog("pending action triggered scan for reader")
			return m, runScanCmd(m.scanOptions)
		}
		return m.beginConnectPlan(actionName, idx)
	}

	if m.scanning {
		m.pendingConnect = true
		m.status = fmt.Sprintf("%s requested: waiting for scan result...", actionName)
		m.pushLog("pending action queued while scan running")
		return m, nil
	}

	m.pendingConnect = true
	m.scanning = true
	m.status = fmt.Sprintf("%s requested: scanning for reader...", actionName)
	m.pushLog("pending action triggered scan")
	return m, runScanCmd(m.scanOptions)
}

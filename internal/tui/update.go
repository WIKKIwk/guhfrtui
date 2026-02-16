package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"new_era_go/internal/discovery"
	reader18 "new_era_go/internal/protocol/reader18"
	"new_era_go/internal/reader"
	"new_era_go/internal/regions"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.width > 20 {
			m.input.Width = m.width - 14
		}
		return m, nil

	case tea.KeyMsg:
		if m.inputMode == inputModeRawHex {
			return m.updateRawInput(msg)
		}
		return m.updateKey(msg)

	case scanFinishedMsg:
		return m.onScanFinished(msg)

	case connectFinishedMsg:
		return m.onConnectFinished(msg)

	case disconnectFinishedMsg:
		if msg.Err != nil {
			m.status = "Disconnect failed: " + msg.Err.Error()
			m.pushLog("disconnect error: " + msg.Err.Error())
			return m, nil
		}
		m.inventoryRunning = false
		m.awaitingProbe = false
		m.status = "Disconnected"
		m.pushLog("reader disconnected")
		return m, nil

	case commandSentMsg:
		return m.onCommandSent(msg)

	case inventoryTickMsg:
		if !m.inventoryRunning {
			return m, nil
		}
		if !m.reader.IsConnected() {
			m.inventoryRunning = false
			m.status = "Reading stopped: reader disconnected"
			return m, nil
		}

		m.inventoryRounds++
		cmds := make([]tea.Cmd, 0, 4)
		if len(inventoryFrequencyWindows) > 0 && m.inventoryTagTotal == 0 && (m.inventoryRounds == 1 || m.inventoryRounds%30 == 0) {
			window := inventoryFrequencyWindows[m.inventoryFreqIdx%len(inventoryFrequencyWindows)]
			m.inventoryFreqIdx++
			cmds = append(cmds, sendNamedCmd(m.reader, "cfg-freq-cycle", reader18.SetFrequencyRangeCommand(m.inventoryAddress, window.High, window.Low)))
		}

		cmdSingle := reader18.InventorySingleTagCommand(m.inventoryAddress)
		cmdLegacy := reader18.InventoryCommand(m.inventoryAddress, 0x00, 0x01)
		cmds = append(cmds,
			sendNamedCmd(m.reader, "inventory-single", cmdSingle),
			sendNamedCmd(m.reader, "inventory-legacy", cmdLegacy),
			inventoryTickCmd(m.inventoryInterval),
		)
		return m, tea.Batch(cmds...)

	case probeTimeoutMsg:
		if m.awaitingProbe && !m.inventoryRunning {
			m.awaitingProbe = false
			m.status = "Connected endpoint did not answer reader protocol"
			m.pushLog("probe timeout: endpoint is likely not ST-8508")
			return m, disconnectCmd(m.reader)
		}
		return m, nil

	case packetMsg:
		m.rxBytes += len(msg.Packet.Data)
		m.lastRX = formatHex(msg.Packet.Data, 52)
		m.protocolBuffer = append(m.protocolBuffer, msg.Packet.Data...)
		if len(m.protocolBuffer) > 8192 {
			m.protocolBuffer = append([]byte{}, m.protocolBuffer[len(m.protocolBuffer)-4096:]...)
		}

		frames, remaining := reader18.ParseFrames(m.protocolBuffer)
		m.protocolBuffer = remaining
		if len(frames) == 0 {
			if !m.inventoryRunning || time.Since(m.lastRawLogAt) > 2*time.Second {
				m.pushLog("rx raw " + m.lastRX)
				m.lastRawLogAt = time.Now()
			}
		} else {
			for _, frame := range frames {
				m.handleProtocolFrame(frame)
			}
		}
		return m, waitPacketCmd(m.reader.Packets())

	case packetChannelClosedMsg:
		if m.reader.IsConnected() {
			return m, waitPacketCmd(m.reader.Packets())
		}
		return m, nil

	case readerErrMsg:
		if !errors.Is(msg.Err, net.ErrClosed) && !strings.Contains(strings.ToLower(msg.Err.Error()), "use of closed network connection") {
			m.pushLog("reader error: " + msg.Err.Error())
		}
		if !m.reader.IsConnected() {
			m.inventoryRunning = false
			m.awaitingProbe = false
			m.status = "Reader connection closed"
		}
		return m, waitReaderErrCmd(m.reader.Errors())

	case readerErrChannelClosedMsg:
		if m.reader.IsConnected() {
			return m, waitReaderErrCmd(m.reader.Errors())
		}
		return m, nil
	}

	return m, nil
}

func (m Model) onScanFinished(msg scanFinishedMsg) (tea.Model, tea.Cmd) {
	m.scanning = false
	m.lastScanTime = msg.Duration

	if msg.Err != nil && !errors.Is(msg.Err, context.DeadlineExceeded) && !errors.Is(msg.Err, context.Canceled) {
		m.pendingConnect = false
		m.status = "Scan failed: " + msg.Err.Error()
		m.pushLog("scan error: " + msg.Err.Error())
		return m, nil
	}

	m.candidates = msg.Candidates
	if m.deviceIndex >= len(m.candidates) {
		m.deviceIndex = 0
	}

	if len(m.candidates) == 0 {
		m.status = fmt.Sprintf("Scan finished (%s), no reader found", msg.Duration.Round(time.Millisecond))
		m.pushLog("scan done: no candidates")
		m.pendingConnect = false
		m.pendingAction = noPendingAction
		return m, nil
	}

	verified := countVerifiedCandidates(m.candidates)
	m.status = fmt.Sprintf("Scan finished (%s), %d candidate(s), verified=%d", msg.Duration.Round(time.Millisecond), len(m.candidates), verified)
	m.pushLog(fmt.Sprintf("scan done: %d candidates (%d verified)", len(m.candidates), verified))

	if m.pendingConnect {
		m.pendingConnect = false
		idx := preferredVerifiedCandidateIndex(m.candidates)
		if idx < 0 {
			m.status = "No verified reader found. Open Devices page and check network."
			m.pushLog("quick connect skipped: no verified reader")
			m.pendingAction = noPendingAction
			return m, nil
		}
		ep := reader.Endpoint{Host: m.candidates[idx].Host, Port: m.candidates[idx].Port}
		m.status = "Quick connecting to " + ep.Address()
		m.pushLog("quick connect: " + ep.Address())
		return m, reconnectCmd(m.reader, ep)
	}

	return m, nil
}

func (m Model) onConnectFinished(msg connectFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.status = "Connect failed: " + msg.Err.Error()
		m.pushLog("connect error: " + msg.Err.Error())
		m.pendingAction = noPendingAction
		return m, nil
	}

	m.activeScreen = screenControl
	m.inventoryRunning = false
	m.protocolBuffer = nil
	m.awaitingProbe = false
	m.status = "Connected: " + msg.Endpoint.Address()
	m.pushLog("connected: " + msg.Endpoint.Address())
	base := []tea.Cmd{
		waitPacketCmd(m.reader.Packets()),
		waitReaderErrCmd(m.reader.Errors()),
	}

	switch m.pendingAction {
	case 0:
		m.pendingAction = noPendingAction
		m.inventoryRunning = true
		m.inventoryRounds = 0
		m.inventoryTagTotal = 0
		m.inventoryFreqIdx = 0
		m.lastTagEPC = ""
		m.inventoryAutoAddr = true
		m.protocolBuffer = nil
		m.status = "Connected. Preparing reader + reading started"
		m.pushLog(fmt.Sprintf("reading started (interval %s)", m.inventoryInterval))
		base = append(base,
			sendNamedCmd(m.reader, "cfg-work-mode", reader18.SetWorkModeCommand(m.inventoryAddress, []byte{0x00})),
			sendNamedCmd(m.reader, "cfg-scan-time", reader18.SetScanTimeCommand(m.inventoryAddress, 0x03)),
			sendNamedCmd(m.reader, "cfg-power", reader18.SetOutputPowerCommand(m.inventoryAddress, 0x21)),
			sendNamedCmd(m.reader, "cfg-freq", reader18.SetFrequencyRangeCommand(m.inventoryAddress, 0x3E, 0x28)),
			inventoryTickCmd(120*time.Millisecond),
		)
	case 2:
		m.pendingAction = noPendingAction
		packet := reader18.GetReaderInfoCommand(m.inventoryAddress)
		m.status = "Connected. Sending GetReaderInfo"
		base = append(base, sendNamedCmd(m.reader, "probe-info", packet))
	case 3:
		m.pendingAction = noPendingAction
		m.inputMode = inputModeRawHex
		m.input.Focus()
		m.status = "Connected. Raw mode ready"
	default:
		packet := reader18.GetReaderInfoCommand(m.inventoryAddress)
		m.awaitingProbe = true
		base = append(base, sendNamedCmd(m.reader, "probe-info", packet))
		base = append(base, probeTimeoutCmd(2*time.Second))
	}

	return m, tea.Batch(base...)
}

func (m Model) onCommandSent(msg commandSentMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		if strings.HasPrefix(msg.Name, "inventory-") {
			m.inventoryRunning = false
			m.status = "Reading stopped: " + msg.Err.Error()
			m.pushLog("inventory send error: " + msg.Err.Error())
			return m, nil
		}
		m.status = "Command failed: " + msg.Err.Error()
		m.pushLog(msg.Name + " error: " + msg.Err.Error())
		return m, nil
	}

	m.txBytes += msg.Sent
	if strings.HasPrefix(msg.Name, "inventory-") {
		if m.inventoryRounds%8 == 0 {
			m.status = fmt.Sprintf("Reading... rounds=%d tags=%d", m.inventoryRounds, m.inventoryTagTotal)
		}
		return m, nil
	}
	if strings.HasPrefix(msg.Name, "cfg-") {
		return m, nil
	}

	switch msg.Name {
	case "raw":
		m.status = fmt.Sprintf("Raw sent (%d bytes)", msg.Sent)
		m.pushLog(fmt.Sprintf("tx raw %d bytes", msg.Sent))
		m.input.SetValue("")
		m.inputMode = inputModeNone
		m.input.Blur()
	case "probe-info":
		m.status = "GetReaderInfo sent, waiting response"
		m.pushLog("tx probe reader info")
		m.awaitingProbe = true
	default:
		m.status = fmt.Sprintf("Sent %s (%d bytes)", msg.Name, msg.Sent)
	}

	return m, nil
}

func (m *Model) handleProtocolFrame(frame reader18.Frame) {
	m.lastRX = formatHex(frame.Raw, 52)

	switch frame.Command {
	case reader18.CmdInventory:
		m.awaitingProbe = false
		if m.inventoryAutoAddr {
			m.inventoryAddress = frame.Address
			m.inventoryAutoAddr = false
			m.pushLog(fmt.Sprintf("inventory address detected: 0x%02X", frame.Address))
		}

		switch frame.Status {
		case reader18.StatusSuccess:
			count, err := reader18.InventoryTagCount(frame)
			if err != nil {
				m.pushLog("inventory parse error: " + err.Error())
				return
			}
			m.inventoryTagTotal += count
			if count > 0 {
				m.status = fmt.Sprintf("Reading OK: last=%d total=%d", count, m.inventoryTagTotal)
				m.pushLog(fmt.Sprintf("inventory: %d tag(s)", count))
			} else if m.inventoryRunning && m.inventoryRounds%15 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusNoTag:
			count := 0
			if len(frame.Data) >= 2 {
				count = int(frame.Data[1])
			} else if len(frame.Data) >= 1 {
				count = int(frame.Data[0])
			}
			if count > 0 {
				m.inventoryTagTotal += count
				m.status = fmt.Sprintf("Reading OK: last=%d total=%d", count, m.inventoryTagTotal)
				m.pushLog(fmt.Sprintf("inventory: %d tag(s)", count))
			} else if m.inventoryRunning && m.inventoryRounds%15 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusCmdError:
			m.pushLog("inventory status: command error (0xFE)")
		case reader18.StatusCRCError:
			m.pushLog("inventory status: crc error (0xFF)")
		case reader18.StatusNoTagOrTimeout:
			if m.inventoryRunning && m.inventoryRounds%18 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusAntennaError:
			if m.inventoryRunning && m.inventoryRounds%20 == 0 {
				m.status = fmt.Sprintf("Reading... antenna check (rounds=%d)", m.inventoryRounds)
			}
		default:
			if !m.inventoryRunning {
				m.pushLog(fmt.Sprintf("inventory status: 0x%02X", frame.Status))
			}
		}

	case reader18.CmdInventorySingle:
		m.awaitingProbe = false
		if m.inventoryAutoAddr {
			m.inventoryAddress = frame.Address
			m.inventoryAutoAddr = false
			m.pushLog(fmt.Sprintf("inventory address detected: 0x%02X", frame.Address))
		}

		switch frame.Status {
		case reader18.StatusNoTag:
			result, err := reader18.ParseSingleInventoryResult(frame)
			if err != nil {
				if !m.inventoryRunning {
					m.pushLog("single inventory parse error: " + err.Error())
				}
				return
			}
			if result.TagCount > 0 {
				m.inventoryTagTotal += result.TagCount
				epcText := strings.ReplaceAll(formatHex(result.EPC, 96), " ", "")
				m.lastTagEPC = epcText
				m.status = fmt.Sprintf("Tag detected: ant=%d epc=%s", result.Antenna, trimText(epcText, 28))
				m.pushLog(fmt.Sprintf("tag ant=%d epc=%s", result.Antenna, epcText))
			} else if m.inventoryRunning && m.inventoryRounds%14 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusNoTagOrTimeout:
			if m.inventoryRunning && m.inventoryRounds%14 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusAntennaError:
			if m.inventoryRunning && m.inventoryRounds%20 == 0 {
				m.status = fmt.Sprintf("Reading... antenna check (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusCmdError:
			if !m.inventoryRunning {
				m.pushLog("single inventory status: command error (0xFE)")
			}
		case reader18.StatusCRCError:
			if !m.inventoryRunning {
				m.pushLog("single inventory status: parameter error (0xFF)")
			}
		default:
			if !m.inventoryRunning {
				m.pushLog(fmt.Sprintf("single inventory status: 0x%02X", frame.Status))
			}
		}

	case reader18.CmdGetReaderInfo:
		m.awaitingProbe = false
		if frame.Status == reader18.StatusSuccess {
			m.status = "Reader info received"
			m.pushLog("reader info: " + formatHex(frame.Data, 48))
		} else {
			m.pushLog(fmt.Sprintf("reader info status: 0x%02X", frame.Status))
		}

	default:
		if !m.inventoryRunning {
			m.pushLog(fmt.Sprintf("rx cmd=0x%02X status=0x%02X", frame.Command, frame.Status))
		}
	}
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		_ = m.reader.Disconnect()
		return m, tea.Quit
	case "m":
		m.activeScreen = screenHome
		m.status = "Home"
		return m, nil
	case "b", "backspace":
		if m.activeScreen != screenHome {
			m.activeScreen = screenHome
			m.status = "Back to home"
			return m, nil
		}
	}

	switch m.activeScreen {
	case screenHome:
		return m.updateHomeKeys(msg)
	case screenDevices:
		return m.updateDeviceKeys(msg)
	case screenControl:
		return m.updateControlKeys(msg)
	case screenRegions:
		return m.updateRegionKeys(msg)
	case screenLogs:
		return m.updateLogKeys(msg)
	case screenHelp:
		return m.updateHelpKeys(msg)
	default:
		return m, nil
	}
}

func (m Model) updateHomeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.homeIndex = (m.homeIndex - 1 + len(homeMenu)) % len(homeMenu)
		return m, nil
	case "down", "j":
		m.homeIndex = (m.homeIndex + 1) % len(homeMenu)
		return m, nil
	case "enter":
		return m.runHomeAction(m.homeIndex)
	}

	if idx, ok := parseDigit(msg.String()); ok && idx < len(homeMenu) {
		m.homeIndex = idx
		return m.runHomeAction(idx)
	}
	return m, nil
}

func (m Model) runHomeAction(index int) (tea.Model, tea.Cmd) {
	switch index {
	case 0:
		return m.runQuickConnect()
	case 1:
		m.activeScreen = screenDevices
		m.status = "Devices"
	case 2:
		m.activeScreen = screenControl
		m.status = "Control"
	case 3:
		m.activeScreen = screenRegions
		m.regionCursor = m.regionIndex
		m.status = "Regions"
	case 4:
		m.activeScreen = screenLogs
		m.logScroll = 0
		m.status = "Logs"
	case 5:
		m.activeScreen = screenHelp
		m.status = "Help"
	}
	return m, nil
}

func (m Model) runQuickConnect() (tea.Model, tea.Cmd) {
	if m.scanning {
		m.pendingConnect = true
		m.status = "Scan running, quick-connect queued"
		m.pushLog("quick-connect queued")
		return m, nil
	}

	if len(m.candidates) > 0 {
		idx := preferredVerifiedCandidateIndex(m.candidates)
		if idx < 0 {
			m.status = "No verified reader in cache. Rescanning..."
			m.pushLog("quick connect requires verified reader")
			m.scanning = true
			m.pendingConnect = true
			return m, runScanCmd(m.scanOptions)
		}
		ep := reader.Endpoint{Host: m.candidates[idx].Host, Port: m.candidates[idx].Port}
		m.status = "Quick connecting to " + ep.Address()
		m.pushLog("quick connect: " + ep.Address())
		return m, reconnectCmd(m.reader, ep)
	}

	m.scanning = true
	m.pendingConnect = true
	m.status = "No cached devices, starting scan..."
	m.pushLog("quick-connect triggered scan")
	return m, runScanCmd(m.scanOptions)
}

func (m Model) updateDeviceKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if len(m.candidates) > 0 {
			m.deviceIndex = (m.deviceIndex - 1 + len(m.candidates)) % len(m.candidates)
		}
		return m, nil
	case "down", "j":
		if len(m.candidates) > 0 {
			m.deviceIndex = (m.deviceIndex + 1) % len(m.candidates)
		}
		return m, nil
	case "s":
		if m.scanning {
			m.status = "Scan already running"
			return m, nil
		}
		m.scanning = true
		m.pendingConnect = false
		m.status = "Scanning LAN..."
		m.pushLog("manual scan")
		return m, runScanCmd(m.scanOptions)
	case "a":
		return m.runQuickConnect()
	case "enter":
		return m.connectSelectedDevice()
	}

	if idx, ok := parseDigit(msg.String()); ok && idx < len(m.candidates) {
		m.deviceIndex = idx
		return m.connectSelectedDevice()
	}

	return m, nil
}

func (m Model) connectSelectedDevice() (tea.Model, tea.Cmd) {
	if len(m.candidates) == 0 {
		m.status = "No devices, press 's' to scan"
		return m, nil
	}

	selected := m.candidates[m.deviceIndex]
	ep := reader.Endpoint{Host: selected.Host, Port: selected.Port}
	if !selected.Verified {
		m.pushLog("warning: connecting to unverified endpoint " + ep.Address())
	}
	m.status = "Connecting to " + ep.Address()
	m.pushLog("manual connect: " + ep.Address())
	return m, reconnectCmd(m.reader, ep)
}

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
		m.lastTagEPC = ""
		m.inventoryAutoAddr = true
		m.protocolBuffer = nil
		m.status = "Preparing reader + reading started"
		m.pushLog(fmt.Sprintf("reading started (interval %s)", m.inventoryInterval))
		return m, tea.Batch(
			sendNamedCmd(m.reader, "cfg-work-mode", reader18.SetWorkModeCommand(m.inventoryAddress, []byte{0x00})),
			sendNamedCmd(m.reader, "cfg-scan-time", reader18.SetScanTimeCommand(m.inventoryAddress, 0x03)),
			sendNamedCmd(m.reader, "cfg-power", reader18.SetOutputPowerCommand(m.inventoryAddress, 0x21)),
			sendNamedCmd(m.reader, "cfg-freq", reader18.SetFrequencyRangeCommand(m.inventoryAddress, 0x3E, 0x28)),
			inventoryTickCmd(120*time.Millisecond),
		)

	case 1:
		if !m.inventoryRunning {
			m.status = "Reading already stopped"
			return m, nil
		}
		m.inventoryRunning = false
		m.status = fmt.Sprintf("Reading stopped. rounds=%d tags=%d", m.inventoryRounds, m.inventoryTagTotal)
		m.pushLog("reading stopped")
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

func (m Model) updateRegionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := len(regions.Catalog)
	if total == 0 {
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		m.regionCursor = (m.regionCursor - 1 + total) % total
		return m, nil
	case "down", "j":
		m.regionCursor = (m.regionCursor + 1) % total
		return m, nil
	case "enter":
		m.regionIndex = m.regionCursor
		selected := regions.Catalog[m.regionIndex]
		m.status = fmt.Sprintf("Region selected: %s (%s)", selected.Code, selected.Band)
		m.pushLog("region selected: " + selected.Code)
		return m, nil
	}

	if idx, ok := parseDigit(msg.String()); ok && idx < total {
		m.regionCursor = idx
		m.regionIndex = idx
		selected := regions.Catalog[m.regionIndex]
		m.status = fmt.Sprintf("Region selected: %s (%s)", selected.Code, selected.Band)
		m.pushLog("region selected: " + selected.Code)
	}
	return m, nil
}

func (m Model) updateLogKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxVisible := m.logViewSize()
	maxScroll := len(m.logs) - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}

	switch msg.String() {
	case "up", "k":
		if m.logScroll < maxScroll {
			m.logScroll++
		}
	case "down", "j":
		if m.logScroll > 0 {
			m.logScroll--
		}
	case "c":
		m.logs = nil
		m.logScroll = 0
		m.status = "Logs cleared"
	}
	return m, nil
}

func (m Model) updateHelpKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "enter" {
		m.activeScreen = screenHome
		m.status = "Home"
	}
	return m, nil
}

func (m Model) updateRawInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputMode = inputModeNone
		m.input.Blur()
		m.status = "Raw input canceled"
		return m, nil
	case "enter":
		payload, err := parseHexInput(m.input.Value())
		if err != nil {
			m.status = "Hex parse error: " + err.Error()
			return m, nil
		}
		m.status = fmt.Sprintf("Sending %d byte(s)...", len(payload))
		return m, sendNamedCmd(m.reader, "raw", payload)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) requestConnectionForAction(action int, actionName string) (tea.Model, tea.Cmd) {
	m.pendingAction = action
	if m.reader.IsConnected() {
		m.status = "Connected. Running action..."
		return m, nil
	}

	if len(m.candidates) > 0 {
		idx := preferredVerifiedCandidateIndex(m.candidates)
		if idx < 0 {
			if m.scanning {
				m.pendingConnect = true
				m.status = fmt.Sprintf("%s requested: waiting for verified reader...", actionName)
				m.pushLog("pending action waiting for verified reader")
				return m, nil
			}
			m.pendingConnect = true
			m.scanning = true
			m.status = fmt.Sprintf("%s requested: rescanning for verified reader...", actionName)
			m.pushLog("pending action triggered scan for verified reader")
			return m, runScanCmd(m.scanOptions)
		}
		candidate := m.candidates[idx]
		endpoint := reader.Endpoint{Host: candidate.Host, Port: candidate.Port}
		m.status = fmt.Sprintf("%s requested: connecting to %s", actionName, endpoint.Address())
		m.pushLog(fmt.Sprintf("pending action %d -> connect %s", action, endpoint.Address()))
		return m, reconnectCmd(m.reader, endpoint)
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

func preferredVerifiedCandidateIndex(candidates []discovery.Candidate) int {
	if len(candidates) == 0 {
		return -1
	}
	for i, candidate := range candidates {
		if candidate.Verified {
			return i
		}
	}
	return -1
}

func countVerifiedCandidates(candidates []discovery.Candidate) int {
	count := 0
	for _, candidate := range candidates {
		if candidate.Verified {
			count++
		}
	}
	return count
}

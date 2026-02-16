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

const autoFreqCycleEnabled = false

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

	case botStatusMsg:
		if msg.Err != nil {
			m.botOnline = false
			m.botLastErr = msg.Err.Error()
			m.botLastSync = msg.At
		} else {
			m.botOnline = true
			m.botStats = msg.Stats
			m.botLastErr = ""
			m.botLastSync = msg.At
		}
		return m, botStatusTickCmd(time.Second)

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
		if m.inventoryRunning {
			getBotSyncClient().onStopReading()
		}
		m.inventoryRunning = false
		m.awaitingProbe = false
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		m.connecting = false
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
			if m.inventoryRunning {
				getBotSyncClient().onStopReading()
			}
			m.inventoryRunning = false
			m.status = "Reading stopped: reader disconnected"
			return m, nil
		}

		m.inventoryRounds++
		cmds := make([]tea.Cmd, 0, 5)
		if autoFreqCycleEnabled && len(inventoryFrequencyWindows) > 0 && m.inventoryTagTotal == 0 && (m.inventoryRounds == 1 || m.inventoryRounds%80 == 0) {
			window := inventoryFrequencyWindows[m.inventoryFreqIdx%len(inventoryFrequencyWindows)]
			m.inventoryFreqIdx++
			cmds = append(cmds, sendNamedCmdSilent(m.reader, "cfg-freq-cycle", reader18.SetFrequencyRangeCommand(m.inventoryAddress, window.High, window.Low)))
		}

		antenna, nextIdx := nextInventoryAntenna(m.inventoryAntMask, m.inventoryAntIdx)
		m.inventoryAntenna = antenna
		m.inventoryAntIdx = nextIdx

		cmdInventory := reader18.InventoryG2Command(
			m.inventoryAddress,
			m.inventoryQValue,
			m.inventorySession,
			0x00,
			0x00,
			m.inventoryTarget,
			m.inventoryAntenna,
			m.inventoryScanTime,
		)
		cmds = append(cmds,
			sendNamedCmdSilent(m.reader, "inventory-g2", cmdInventory),
			inventoryTickCmd(m.effectiveInventoryInterval()),
		)
		// Some firmwares return only tag count on cmd 0x01; periodic single inventory keeps EPC capture reliable.
		if m.inventoryRounds%6 == 0 {
			cmds = append(cmds, sendNamedCmdSilent(m.reader, "inventory-single", reader18.InventorySingleTagCommand(m.inventoryAddress)))
		}
		return m, tea.Batch(cmds...)

	case probeTimeoutMsg:
		if m.awaitingProbe && !m.inventoryRunning {
			m.awaitingProbe = false
			m.pushLog("probe timeout: endpoint did not answer reader protocol")
			if next, cmd, ok := m.retryNextConnect("probe timeout"); ok {
				return next, cmd
			}
			m.status = "Connected endpoint did not answer reader protocol"
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
			if m.inventoryRunning {
				getBotSyncClient().onStopReading()
			}
			m.inventoryRunning = false
			m.awaitingProbe = false
			if !m.connecting {
				if next, cmd, ok := m.retryNextConnect(msg.Err.Error()); ok {
					return next, cmd
				}
			}
			m.connecting = false
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
		m.connecting = false
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
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		m.connecting = false
		return m, nil
	}

	verified := countVerifiedCandidates(m.candidates)
	m.status = fmt.Sprintf("Scan finished (%s), %d candidate(s), verified=%d", msg.Duration.Round(time.Millisecond), len(m.candidates), verified)
	m.pushLog(fmt.Sprintf("scan done: %d candidates (%d verified)", len(m.candidates), verified))

	if m.pendingConnect {
		m.pendingConnect = false
		idx := preferredCandidateIndex(m.candidates)
		if idx < 0 {
			m.status = "No reader endpoint found."
			m.pushLog("quick connect skipped: no endpoints")
			m.pendingAction = noPendingAction
			m.connectQueue = nil
			m.connectAttempt = 0
			m.connectActionLabel = ""
			return m, nil
		}
		return m.beginConnectPlan("Quick Connect", idx)
	}

	return m, nil
}

func (m Model) onConnectFinished(msg connectFinishedMsg) (tea.Model, tea.Cmd) {
	m.connecting = false
	if msg.Err != nil {
		m.pushLog("connect error: " + msg.Err.Error())
		if next, cmd, ok := m.retryNextConnect(msg.Err.Error()); ok {
			return next, cmd
		}
		m.status = "Connect failed: " + msg.Err.Error()
		m.pendingAction = noPendingAction
		return m, nil
	}

	m.connectQueue = nil
	m.connectAttempt = 0
	m.connectActionLabel = ""

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
		m.status = "Connected. Preparing reader + reading started"
		m.pushLog(fmt.Sprintf("reading started (poll=%s effective=%s scan=%d)", m.inventoryInterval, m.effectiveInventoryInterval(), m.inventoryScanTime))
		getBotSyncClient().onStartReading()
		base = append(base,
			sendNamedCmdSilent(m.reader, "cfg-work-mode", reader18.SetWorkModeCommand(m.inventoryAddress, []byte{0x00})),
			sendNamedCmdSilent(m.reader, "cfg-scan-time", reader18.SetScanTimeCommand(m.inventoryAddress, m.inventoryScanTime)),
			sendNamedCmdSilent(m.reader, "cfg-ant-mask", reader18.SetAntennaMuxCommand(m.inventoryAddress, m.inventoryAntMask)),
			sendNamedCmdSilent(m.reader, "cfg-power", reader18.SetOutputPowerCommand(m.inventoryAddress, 0x1E)),
			inventoryTickCmd(m.effectiveInventoryInterval()),
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
			if m.inventoryRunning {
				getBotSyncClient().onStopReading()
			}
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
		if m.activeScreen == screenControl && m.inventoryRounds%24 == 0 {
			m.status = fmt.Sprintf("Reading... rounds=%d unique=%d", m.inventoryRounds, m.inventoryTagTotal)
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
		tags, err := reader18.ParseInventoryG2Tags(frame)
		if err != nil {
			if !m.inventoryRunning {
				m.pushLog("inventory parse error: " + err.Error())
			}
			return
		}
		if len(tags) > 0 {
			if m.seenTagEPC == nil {
				m.seenTagEPC = make(map[string]struct{})
			}
			m.inventoryNoTagHit = 0
			newCount := 0
			for _, tag := range tags {
				epcText := strings.ReplaceAll(formatHex(tag.EPC, 96), " ", "")
				if epcText == "" {
					continue
				}
				m.lastTagEPC = epcText
				m.lastTagAntenna = tag.Antenna
				m.lastTagRSSI = tag.RSSI
				if _, exists := m.seenTagEPC[epcText]; exists {
					continue
				}
				m.seenTagEPC[epcText] = struct{}{}
				m.inventoryTagTotal++
				newCount++
				getBotSyncClient().onNewEPC(epcText)
				m.pushLog(fmt.Sprintf("new tag ant=%d epc=%s rssi=%d total=%d", tag.Antenna, epcText, tag.RSSI, m.inventoryTagTotal))
			}
			if newCount > 0 {
				if m.activeScreen == screenControl {
					m.status = fmt.Sprintf("New tag(s): +%d total=%d", newCount, m.inventoryTagTotal)
				}
			} else if m.inventoryRunning && m.inventoryRounds%12 == 0 && m.lastTagEPC != "" {
				if m.activeScreen == screenControl {
					m.status = fmt.Sprintf("Tag seen again: %s", trimText(m.lastTagEPC, 28))
				}
			}
			return
		}
		if frame.Status == reader18.StatusSuccess {
			if count, err := reader18.InventoryTagCount(frame); err == nil && count > 0 {
				if m.activeScreen == screenControl && m.inventoryRounds%6 == 0 {
					m.status = fmt.Sprintf("Reader reports %d tag(s), waiting EPC frame...", count)
				}
				if m.inventoryRounds%30 == 0 {
					m.pushLog(fmt.Sprintf("inventory count-only response: %d tag(s)", count))
				}
			}
			return
		}

		switch frame.Status {
		case reader18.StatusNoTag, 0x02, 0x03, 0x04, reader18.StatusNoTagOrTimeout:
			m.onNoTagObserved()
			if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
				m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusAntennaError:
			if m.inventoryRunning && m.inventoryRounds%20 == 0 {
				m.status = fmt.Sprintf("Reading... antenna check (rounds=%d)", m.inventoryRounds)
			}
		case reader18.StatusCmdError:
			if !m.inventoryRunning {
				m.pushLog("inventory status: illegal command (0xFE)")
			}
		case reader18.StatusCRCError:
			if !m.inventoryRunning {
				m.pushLog("inventory status: parameter error (0xFF)")
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
				epcText := strings.ReplaceAll(formatHex(result.EPC, 96), " ", "")
				m.lastTagEPC = epcText
				m.lastTagAntenna = int(result.Antenna)
				m.lastTagRSSI = 0
				m.inventoryNoTagHit = 0
				if m.seenTagEPC == nil {
					m.seenTagEPC = make(map[string]struct{})
				}
				if _, exists := m.seenTagEPC[epcText]; !exists {
					m.seenTagEPC[epcText] = struct{}{}
					m.inventoryTagTotal++
					getBotSyncClient().onNewEPC(epcText)
					if m.activeScreen == screenControl {
						m.status = fmt.Sprintf("New tag: ant=%d epc=%s", result.Antenna, trimText(epcText, 28))
					}
					m.pushLog(fmt.Sprintf("new tag ant=%d epc=%s total=%d", result.Antenna, epcText, m.inventoryTagTotal))
				} else if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
					m.status = fmt.Sprintf("Tag seen again: %s", trimText(epcText, 28))
				}
			} else {
				m.onNoTagObserved()
				if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
					m.status = fmt.Sprintf("Reading... no tag (rounds=%d)", m.inventoryRounds)
				}
			}
		case reader18.StatusNoTagOrTimeout:
			m.onNoTagObserved()
			if m.activeScreen == screenControl && m.inventoryRunning && m.inventoryRounds%24 == 0 {
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
	case "0":
		if m.activeScreen != screenHome {
			m.activeScreen = screenHome
			m.status = "Back to home"
			return m, nil
		}
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
	case screenInventory:
		return m.updateInventoryKeys(msg)
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
		m.activeScreen = screenInventory
		m.status = "Inventory Tune"
	case 4:
		m.activeScreen = screenRegions
		m.regionCursor = m.regionIndex
		m.status = "Regions"
	case 5:
		m.activeScreen = screenLogs
		m.logScroll = 0
		m.status = "Logs"
	case 6:
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
		idx := preferredCandidateIndex(m.candidates)
		if idx < 0 {
			m.status = "No reader in cache. Rescanning..."
			m.pushLog("quick connect requires at least one endpoint")
			m.scanning = true
			m.pendingConnect = true
			return m, runScanCmd(m.scanOptions)
		}
		return m.beginConnectPlan("Quick Connect", idx)
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
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
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

	return m.beginConnectPlan("Manual Connect", m.deviceIndex)
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

const (
	invTuneQValue = iota
	invTuneSession
	invTuneTarget
	invTuneScanTime
	invTuneNoTagAB
	invTunePhaseFreq
	invTuneAntennaMask
	invTunePollInterval
	invTuneApply
	invTuneScanMask
	invTunePresetFast
	invTunePresetBalanced
	invTunePresetLongRange
	invTuneCount
)

func (m Model) updateInventoryKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.inventoryIndex = (m.inventoryIndex - 1 + invTuneCount) % invTuneCount
		return m, nil
	case "down", "j":
		m.inventoryIndex = (m.inventoryIndex + 1) % invTuneCount
		return m, nil
	case "left", "h":
		return m.adjustInventorySetting(-1)
	case "right", "l":
		return m.adjustInventorySetting(1)
	case "enter":
		return m.runInventoryAction()
	}

	if idx, ok := parseDigit(msg.String()); ok {
		if idx < invTuneCount {
			m.inventoryIndex = idx
			if idx >= invTuneApply {
				return m.runInventoryAction()
			}
			return m, nil
		}
	}

	return m, nil
}

func (m Model) adjustInventorySetting(delta int) (tea.Model, tea.Cmd) {
	switch m.inventoryIndex {
	case invTuneQValue:
		m.inventoryQValue = byte(clampInt(int(m.inventoryQValue)+delta, 0, 15))
		m.status = fmt.Sprintf("Q value set to %d", m.inventoryQValue)
	case invTuneSession:
		m.inventorySession = byte(clampInt(int(m.inventorySession)+delta, 0, 3))
		m.status = fmt.Sprintf("Session set to %d", m.inventorySession)
	case invTuneTarget:
		m.inventoryTarget ^= 0x01
		m.status = "Target set to " + targetLabel(m.inventoryTarget)
	case invTuneScanTime:
		m.inventoryScanTime = byte(clampInt(int(m.inventoryScanTime)+delta, 1, 255))
		m.status = fmt.Sprintf("Scan time set to %d (x100ms), effective cycle %s", m.inventoryScanTime, m.effectiveInventoryInterval())
	case invTuneNoTagAB:
		m.inventoryNoTagAB = clampInt(m.inventoryNoTagAB+delta, 0, 255)
		m.status = fmt.Sprintf("No-tag A/B switch count set to %d", m.inventoryNoTagAB)
	case invTunePhaseFreq:
		m.showPhaseFreq = !m.showPhaseFreq
		m.status = "Phase/freq columns " + strings.ToLower(onOff(m.showPhaseFreq))
	case invTuneAntennaMask:
		next := clampInt(int(m.inventoryAntMask)+delta, 1, 255)
		m.inventoryAntMask = byte(next)
		m.status = fmt.Sprintf("Antenna mask set to 0x%02X", m.inventoryAntMask)
	case invTunePollInterval:
		nextMS := clampInt(int(m.inventoryInterval/time.Millisecond)+delta*10, 20, 1000)
		m.inventoryInterval = time.Duration(nextMS) * time.Millisecond
		m.status = fmt.Sprintf("Poll interval set to %s, effective cycle %s", m.inventoryInterval, m.effectiveInventoryInterval())
	default:
		m.status = "Select a parameter row to edit"
	}
	return m, nil
}

func (m Model) runInventoryAction() (tea.Model, tea.Cmd) {
	switch m.inventoryIndex {
	case invTuneTarget, invTunePhaseFreq:
		return m.adjustInventorySetting(1)

	case invTuneApply:
		if !m.reader.IsConnected() {
			m.status = "Parameters saved locally (apply when connected)"
			m.pushLog("inventory tune saved local")
			return m, nil
		}
		m.status = "Applying inventory parameters..."
		m.pushLog(fmt.Sprintf("apply tune q=%d s=%d t=%d scan=%d mask=0x%02X poll=%s effective=%s", m.inventoryQValue, m.inventorySession, m.inventoryTarget, m.inventoryScanTime, m.inventoryAntMask, m.inventoryInterval, m.effectiveInventoryInterval()))
		return m, tea.Batch(
			sendNamedCmdSilent(m.reader, "cfg-scan-time", reader18.SetScanTimeCommand(m.inventoryAddress, m.inventoryScanTime)),
			sendNamedCmdSilent(m.reader, "cfg-ant-mask", reader18.SetAntennaMuxCommand(m.inventoryAddress, m.inventoryAntMask)),
		)

	case invTuneScanMask:
		if m.inventoryAntMask == 0 {
			m.inventoryAntMask = 0x01
		}
		m.inventoryAntIdx = 0
		if m.reader.IsConnected() {
			m.status = fmt.Sprintf("Antenna scan configured: mask=0x%02X", m.inventoryAntMask)
			m.pushLog(fmt.Sprintf("antenna scan mask set: 0x%02X", m.inventoryAntMask))
			return m, sendNamedCmdSilent(m.reader, "cfg-ant-mask", reader18.SetAntennaMuxCommand(m.inventoryAddress, m.inventoryAntMask))
		}
		m.status = fmt.Sprintf("Antenna scan mask saved: 0x%02X", m.inventoryAntMask)
		return m, nil

	case invTunePresetFast:
		m = m.applyInventoryPreset("fast")
		return m, nil
	case invTunePresetBalanced:
		m = m.applyInventoryPreset("balanced")
		return m, nil
	case invTunePresetLongRange:
		m = m.applyInventoryPreset("long-range")
		return m, nil
	}

	return m.adjustInventorySetting(1)
}

func (m Model) applyInventoryPreset(name string) Model {
	switch name {
	case "fast":
		m.inventoryQValue = 4
		m.inventorySession = 1
		m.inventoryTarget = 0
		m.inventoryScanTime = 1
		m.inventoryNoTagAB = 4
		m.inventoryInterval = 40 * time.Millisecond
		m.inventoryAntMask = 0x01
		m.status = "Preset applied: fast"
	case "balanced":
		m.inventoryQValue = 4
		m.inventorySession = 1
		m.inventoryTarget = 0
		m.inventoryScanTime = 2
		m.inventoryNoTagAB = 4
		m.inventoryInterval = 70 * time.Millisecond
		m.inventoryAntMask = 0x01
		m.status = "Preset applied: balanced"
	case "long-range":
		m.inventoryQValue = 4
		m.inventorySession = 2
		m.inventoryTarget = 0
		m.inventoryScanTime = 8
		m.inventoryNoTagAB = 5
		m.inventoryInterval = 120 * time.Millisecond
		m.inventoryAntMask = 0x01
		m.status = "Preset applied: long-range"
	}
	m.pushLog(fmt.Sprintf("preset %s: q=%d s=%d target=%d scan=%d poll=%s effective=%s mask=0x%02X", name, m.inventoryQValue, m.inventorySession, m.inventoryTarget, m.inventoryScanTime, m.inventoryInterval, m.effectiveInventoryInterval(), m.inventoryAntMask))
	return m
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
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
		idx := preferredCandidateIndex(m.candidates)
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

func (m Model) beginConnectPlan(actionLabel string, preferredIndex int) (tea.Model, tea.Cmd) {
	plan := buildConnectPlan(m.candidates, preferredIndex, m.scanOptions.Ports)
	if len(plan) == 0 {
		m.status = actionLabel + ": no reachable endpoints"
		m.pushLog(strings.ToLower(actionLabel) + " failed: no endpoints in plan")
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		m.connecting = false
		return m, nil
	}

	m.connectQueue = plan
	m.connectAttempt = 0
	m.connectActionLabel = actionLabel
	m.connecting = true

	first := plan[0]
	m.status = fmt.Sprintf("%s: connecting %s (1/%d)", actionLabel, first.Address(), len(plan))
	m.pushLog(fmt.Sprintf("%s connect 1/%d -> %s", strings.ToLower(actionLabel), len(plan), first.Address()))
	return m, reconnectCmd(m.reader, first)
}

func (m Model) retryNextConnect(reason string) (tea.Model, tea.Cmd, bool) {
	if len(m.connectQueue) == 0 {
		return m, nil, false
	}

	label := strings.TrimSpace(m.connectActionLabel)
	if label == "" {
		label = "Connect"
	}
	cause := strings.TrimSpace(reason)
	if cause == "" {
		cause = "connection error"
	}

	next := m.connectAttempt + 1
	total := len(m.connectQueue)
	if next >= total {
		current := m.connectQueue[m.connectAttempt]
		m.pushLog(fmt.Sprintf("%s failed at %s: %s", strings.ToLower(label), current.Address(), cause))
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		m.connecting = false
		return m, nil, false
	}

	m.connectAttempt = next
	endpoint := m.connectQueue[next]
	m.connecting = true
	m.status = fmt.Sprintf("%s: retry %d/%d -> %s", label, next+1, total, endpoint.Address())
	m.pushLog(fmt.Sprintf("%s retry %d/%d (%s) -> %s", strings.ToLower(label), next+1, total, cause, endpoint.Address()))
	return m, reconnectCmd(m.reader, endpoint), true
}

func buildConnectPlan(candidates []discovery.Candidate, preferredIndex int, scanPorts []int) []reader.Endpoint {
	if len(candidates) == 0 {
		return nil
	}

	portFallback := []int{2022, 5000, 27011, 6000, 4001, 10001}
	indexOrder := candidateConnectOrder(len(candidates), preferredIndex)
	plan := make([]reader.Endpoint, 0, len(indexOrder)*4)
	seen := make(map[string]struct{}, len(indexOrder)*8)

	addEndpoint := func(host string, port int) {
		if strings.TrimSpace(host) == "" || port <= 0 {
			return
		}
		ep := reader.Endpoint{Host: host, Port: port}
		key := ep.Address()
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		plan = append(plan, ep)
	}

	for _, idx := range indexOrder {
		candidate := candidates[idx]
		host := candidate.Host
		ports := mergePortOrder(append([]int{candidate.Port}, scanPorts...), portFallback)
		for _, port := range ports {
			addEndpoint(host, port)
		}
	}

	return plan
}

func candidateConnectOrder(total int, preferred int) []int {
	if total <= 0 {
		return nil
	}
	order := make([]int, 0, total)
	seen := make(map[int]struct{}, total)

	appendIndex := func(i int) {
		if i < 0 || i >= total {
			return
		}
		if _, ok := seen[i]; ok {
			return
		}
		seen[i] = struct{}{}
		order = append(order, i)
	}

	appendIndex(preferred)
	for i := 0; i < total; i++ {
		appendIndex(i)
	}
	return order
}

func mergePortOrder(primary []int, fallback []int) []int {
	out := make([]int, 0, len(primary)+len(fallback))
	seen := make(map[int]struct{}, len(primary)+len(fallback))

	appendPort := func(port int) {
		if port <= 0 {
			return
		}
		if _, ok := seen[port]; ok {
			return
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}

	for _, port := range primary {
		appendPort(port)
	}
	for _, port := range fallback {
		appendPort(port)
	}

	return out
}

func (m *Model) onNoTagObserved() {
	if !m.inventoryRunning {
		return
	}
	m.inventoryNoTagHit++
	if m.inventorySession <= 1 || m.inventoryNoTagAB <= 0 {
		return
	}
	if m.inventoryNoTagHit >= m.inventoryNoTagAB {
		m.inventoryTarget ^= 0x01
		m.inventoryNoTagHit = 0
		m.pushLog("no-tag threshold reached, target switched to " + targetLabel(m.inventoryTarget))
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

func preferredCandidateIndex(candidates []discovery.Candidate) int {
	if len(candidates) == 0 {
		return -1
	}
	if idx := preferredVerifiedCandidateIndex(candidates); idx >= 0 {
		return idx
	}
	return 0
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

package tui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	reader18 "new_era_go/internal/protocol/reader18"
	tuiupdate "new_era_go/internal/tui/update"
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

		antenna, nextIdx := tuiupdate.NextInventoryAntenna(m.inventoryAntMask, m.inventoryAntIdx)
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

	verified := tuiupdate.CountVerifiedCandidates(m.candidates)
	m.status = fmt.Sprintf("Scan finished (%s), %d candidate(s), verified=%d", msg.Duration.Round(time.Millisecond), len(m.candidates), verified)
	m.pushLog(fmt.Sprintf("scan done: %d candidates (%d verified)", len(m.candidates), verified))

	if m.pendingConnect {
		m.pendingConnect = false
		idx := tuiupdate.PreferredCandidateIndex(m.candidates)
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

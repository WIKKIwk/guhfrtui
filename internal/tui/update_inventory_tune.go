package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	reader18 "new_era_go/internal/protocol/reader18"
	tuiupdate "new_era_go/internal/tui/update"
)

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
		m.inventoryQValue = byte(tuiupdate.ClampInt(int(m.inventoryQValue)+delta, 0, 15))
		m.status = fmt.Sprintf("Q value set to %d", m.inventoryQValue)
	case invTuneSession:
		m.inventorySession = byte(tuiupdate.ClampInt(int(m.inventorySession)+delta, 0, 3))
		m.status = fmt.Sprintf("Session set to %d", m.inventorySession)
	case invTuneTarget:
		m.inventoryTarget ^= 0x01
		m.status = "Target set to " + targetLabel(m.inventoryTarget)
	case invTuneScanTime:
		m.inventoryScanTime = byte(tuiupdate.ClampInt(int(m.inventoryScanTime)+delta, 1, 255))
		m.status = fmt.Sprintf("Scan time set to %d (x100ms), effective cycle %s", m.inventoryScanTime, m.effectiveInventoryInterval())
	case invTuneNoTagAB:
		m.inventoryNoTagAB = tuiupdate.ClampInt(m.inventoryNoTagAB+delta, 0, 255)
		m.status = fmt.Sprintf("No-tag A/B switch count set to %d", m.inventoryNoTagAB)
	case invTunePhaseFreq:
		m.showPhaseFreq = !m.showPhaseFreq
		m.status = "Phase/freq columns " + strings.ToLower(onOff(m.showPhaseFreq))
	case invTuneAntennaMask:
		next := tuiupdate.ClampInt(int(m.inventoryAntMask)+delta, 1, 255)
		m.inventoryAntMask = byte(next)
		m.status = fmt.Sprintf("Antenna mask set to 0x%02X", m.inventoryAntMask)
	case invTunePollInterval:
		nextMS := tuiupdate.ClampInt(int(m.inventoryInterval/time.Millisecond)+delta*10, 20, 1000)
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

package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	tuiupdate "new_era_go/internal/tui/update"
)

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
		idx := tuiupdate.PreferredCandidateIndex(m.candidates)
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

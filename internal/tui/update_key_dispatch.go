package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

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

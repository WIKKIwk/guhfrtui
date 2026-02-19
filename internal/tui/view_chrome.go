package tui

import (
	"fmt"
	"strings"

	"new_era_go/internal/regions"
	tuiupdate "new_era_go/internal/tui/update"
)

func (m Model) tabsLine() string {
	tabs := []struct {
		name   string
		screen screen
	}{
		{name: "Home", screen: screenHome},
		{name: "Devices", screen: screenDevices},
		{name: "Control", screen: screenControl},
		{name: "Tune", screen: screenInventory},
		{name: "Regions", screen: screenRegions},
		{name: "Logs", screen: screenLogs},
		{name: "Help", screen: screenHelp},
	}

	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		if tab.screen == m.activeScreen {
			parts = append(parts, "▣ "+strings.ToUpper(tab.name))
		} else {
			parts = append(parts, "□ "+strings.ToUpper(tab.name))
		}
	}

	return strings.Join(parts, "   ")
}

func (m Model) metaLine() string {
	connection := "OFFLINE"
	if m.reader.IsConnected() {
		connection = "ONLINE"
	}

	regionCode := "N/A"
	if m.regionIndex >= 0 && m.regionIndex < len(regions.Catalog) {
		regionCode = regions.Catalog[m.regionIndex].Code
	}
	scanState := "IDLE"
	if m.scanning {
		scanState = "RUNNING"
	}

	return fmt.Sprintf("Reader %s | Region %s | Scan %s | Verified %d/%d", connection, regionCode, scanState, tuiupdate.CountVerifiedCandidates(m.candidates), len(m.candidates))
}

func (m Model) footerLine() string {
	if m.inputMode == inputModeRawHex {
		return "[Enter] Send  [Esc] Cancel  [0/b] Back  [q] Exit"
	}

	switch m.activeScreen {
	case screenHome:
		return "[1..7] Open  [Enter] Open  [0/b] Back  [q] Exit"
	case screenDevices:
		return "[Enter] Connect  [s] Scan  [a] Quick Connect  [0/b] Back"
	case screenControl:
		return "[Enter] Run  [/] Raw Hex  [0/b] Back"
	case screenInventory:
		return "[h/l] Change  [Enter] Apply/Action  [0/b] Back"
	case screenRegions:
		return "[Enter] Select Region  [0/b] Back"
	case screenLogs:
		return "[Up/Down] Scroll  [c] Clear  [0/b] Back"
	case screenHelp:
		return "[0/b] Back  [m] Home  [q] Exit"
	default:
		return "[0/b] Back  [q] Exit"
	}
}

func (m Model) statusLine() string {
	return statusTag(m.status) + " " + m.status
}

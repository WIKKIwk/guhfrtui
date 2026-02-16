package tui

import (
	"fmt"
	"strings"
	"time"

	"new_era_go/internal/regions"
)

func (m Model) View() string {
	var b strings.Builder

	b.WriteString("ST-8508 Reader TUI\n")
	b.WriteString(m.tabsLine())
	b.WriteString("\n")
	b.WriteString(m.metaLine())
	b.WriteString("\n")
	b.WriteString("Status: ")
	b.WriteString(m.status)
	b.WriteString("\n\n")

	for _, line := range m.pageLines() {
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.footerLine())
	return b.String()
}

func (m Model) pageLines() []string {
	switch m.activeScreen {
	case screenHome:
		return m.homePageLines()
	case screenDevices:
		return m.devicesPageLines()
	case screenControl:
		return m.controlPageLines()
	case screenRegions:
		return m.regionsPageLines()
	case screenLogs:
		return m.logsPageLines()
	case screenHelp:
		return m.helpPageLines()
	default:
		return []string{"Unknown page"}
	}
}

func (m Model) homePageLines() []string {
	lines := []string{"Home"}
	for i, item := range homeMenu {
		prefix := "  "
		if i == m.homeIndex {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s", prefix, i+1, item.Label))
	}
	lines = append(lines, "")
	selected := homeMenu[m.homeIndex]
	lines = append(lines, "Selected: "+selected.Label)
	lines = append(lines, selected.Desc)
	return lines
}

func (m Model) devicesPageLines() []string {
	lines := []string{"Devices"}
	verifiedCount := countVerifiedCandidates(m.candidates)
	if m.scanning {
		lines = append(lines, "Scan: running...")
	} else {
		lines = append(lines, fmt.Sprintf("Scan: idle (last %s)", m.lastScanTime.Round(time.Millisecond)))
	}
	lines = append(lines, fmt.Sprintf("Candidates: %d (verified: %d)", len(m.candidates), verifiedCount))

	if len(m.candidates) == 0 {
		lines = append(lines, "", "No candidates", "Press s to scan")
		return lines
	}

	start, end := listWindow(m.deviceIndex, len(m.candidates), m.deviceViewSize())
	lines = append(lines, "")
	for i := start; i < end; i++ {
		candidate := m.candidates[i]
		prefix := "  "
		if i == m.deviceIndex {
			prefix = "> "
		}
		marker := ""
		if candidate.Verified {
			marker = " [VERIFIED]"
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s:%d (score:%d)%s", prefix, i+1, candidate.Host, candidate.Port, candidate.Score, marker))
	}

	selected := m.candidates[m.deviceIndex]
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Selected: %s:%d", selected.Host, selected.Port))
	lines = append(lines, "Reason: "+selected.Reason)
	if selected.Verified {
		lines = append(lines, fmt.Sprintf("Protocol: %s  addr:0x%02X", selected.Protocol, selected.ReaderAddress))
	}
	if selected.Banner != "" {
		lines = append(lines, "Banner: "+trimText(selected.Banner, 64))
	}
	return lines
}

func (m Model) controlPageLines() []string {
	lines := []string{"Control"}
	if endpoint, ok := m.reader.Endpoint(); ok {
		lines = append(lines, "Connection: "+endpoint.Address())
	} else {
		lines = append(lines, "Connection: disconnected")
	}

	invState := "stopped"
	if m.inventoryRunning {
		invState = "running"
	}
	addr := fmt.Sprintf("0x%02X", m.inventoryAddress)
	if m.inventoryAutoAddr {
		addr = "auto(0x00/0xFF)"
	}
	lines = append(lines, fmt.Sprintf("Inventory: %s | rounds:%d | tags:%d", invState, m.inventoryRounds, m.inventoryTagTotal))
	lines = append(lines, fmt.Sprintf("Protocol: Reader18 | addr:%s | every:%s", addr, m.inventoryInterval))
	if m.lastTagEPC != "" {
		lines = append(lines, "Last Tag: "+trimText(m.lastTagEPC, 36))
	}
	if m.lastRX != "" {
		lines = append(lines, "Last RX: "+trimText(m.lastRX, 64))
	}

	lines = append(lines, "", "Actions:")
	for i, item := range controlMenu {
		prefix := "  "
		if i == m.controlIndex {
			prefix = "> "
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s", prefix, i+1, item.Label))
	}
	lines = append(lines, "")
	lines = append(lines, "Selected: "+controlMenu[m.controlIndex].Desc)

	if m.inputMode == inputModeRawHex {
		lines = append(lines, "", "Raw Input")
		lines = append(lines, m.input.View())
		lines = append(lines, "Enter=send Esc=cancel")
	}

	return lines
}

func (m Model) regionsPageLines() []string {
	lines := []string{"Regions"}
	if len(regions.Catalog) == 0 {
		return append(lines, "No region catalog")
	}

	start, end := listWindow(m.regionCursor, len(regions.Catalog), m.regionViewSize())
	lines = append(lines, "")
	for i := start; i < end; i++ {
		region := regions.Catalog[i]
		prefix := "  "
		if i == m.regionCursor {
			prefix = "> "
		}
		tag := ""
		if i == m.regionIndex {
			tag = " [selected]"
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s %s%s", prefix, i+1, region.Code, region.Band, tag))
	}

	selected := regions.Catalog[m.regionCursor]
	lines = append(lines, "")
	lines = append(lines, "Selected: "+selected.Name)
	lines = append(lines, "Band: "+selected.Band)
	return lines
}

func (m Model) logsPageLines() []string {
	lines := []string{"Logs"}
	if len(m.logs) == 0 {
		return append(lines, "No logs yet")
	}

	visible := m.visibleLogs(m.logViewSize())
	lines = append(lines, "")
	lines = append(lines, visible...)
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Total:%d  Scroll:%d", len(m.logs), m.logScroll))
	return lines
}

func (m Model) helpPageLines() []string {
	return []string{
		"Help",
		"",
		"Recommended flow:",
		"1) Home -> Quick Connect",
		"2) Control -> Start Reading",
		"3) Put tag near antenna",
		"4) Logs -> verify responses",
		"5) Control -> Stop Reading",
		"",
		"Global keys: q quit, m home, b back",
		"Move keys: j/k or up/down",
		"Select key: enter",
	}
}

func (m Model) tabsLine() string {
	tabs := []struct {
		name   string
		screen screen
	}{
		{name: "Home", screen: screenHome},
		{name: "Devices", screen: screenDevices},
		{name: "Control", screen: screenControl},
		{name: "Regions", screen: screenRegions},
		{name: "Logs", screen: screenLogs},
		{name: "Help", screen: screenHelp},
	}

	parts := make([]string, 0, len(tabs))
	for _, tab := range tabs {
		if tab.screen == m.activeScreen {
			parts = append(parts, "["+tab.name+"]")
		} else {
			parts = append(parts, tab.name)
		}
	}

	return strings.Join(parts, " | ")
}

func (m Model) metaLine() string {
	connection := "disconnected"
	if endpoint, ok := m.reader.Endpoint(); ok {
		connection = endpoint.Address()
	}

	region := regions.Catalog[m.regionIndex]
	scanState := "idle"
	if m.scanning {
		scanState = "running"
	}

	return fmt.Sprintf("Conn:%s | Region:%s | Scan:%s | Nodes:%d/%dv | TX/RX:%d/%d", connection, region.Code, scanState, len(m.candidates), countVerifiedCandidates(m.candidates), m.txBytes, m.rxBytes)
}

func (m Model) footerLine() string {
	if m.inputMode == inputModeRawHex {
		return "Enter=Send  Esc=Cancel  q=Exit"
	}

	switch m.activeScreen {
	case screenHome:
		return "Use 1-6 or Enter | q=Exit"
	case screenDevices:
		return "Enter=Connect  s=Scan  a=QuickConnect  b=Back"
	case screenControl:
		return "Enter=Run  /=Raw  b=Back"
	case screenRegions:
		return "Enter=Select region  b=Back"
	case screenLogs:
		return "Up/Down=Scroll  c=Clear  b=Back"
	case screenHelp:
		return "b=Back  m=Home  q=Exit"
	default:
		return "q=Exit"
	}
}

func (m Model) deviceViewSize() int {
	if m.height <= 0 {
		return 8
	}
	size := m.height - 18
	if size < 4 {
		size = 4
	}
	if size > 12 {
		size = 12
	}
	return size
}

func (m Model) regionViewSize() int {
	if m.height <= 0 {
		return 10
	}
	size := m.height - 18
	if size < 6 {
		size = 6
	}
	if size > 14 {
		size = 14
	}
	return size
}

func (m Model) logViewSize() int {
	if m.height <= 0 {
		return 12
	}
	size := m.height - 10
	if size < 6 {
		size = 6
	}
	if size > 24 {
		size = 24
	}
	return size
}

func (m Model) visibleLogs(limit int) []string {
	if len(m.logs) == 0 || limit <= 0 {
		return nil
	}

	end := len(m.logs) - m.logScroll
	if end < 0 {
		end = 0
	}
	if end > len(m.logs) {
		end = len(m.logs)
	}

	start := end - limit
	if start < 0 {
		start = 0
	}
	if start > end {
		start = end
	}

	return m.logs[start:end]
}

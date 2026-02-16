package tui

import (
	"fmt"
	"strings"
	"time"

	"new_era_go/internal/regions"
)

const backHomeLine = "◀ 0. Back to Home"

func (m Model) View() string {
	contentWidth := m.panelContentWidth()

	headerPanel := renderPanel(
		"",
		[]string{
			"ST-8508 Reader TUI",
			m.tabsLine(),
			m.metaLine(),
			m.statusLine(),
		},
		contentWidth,
	)

	page := m.pageLines()
	pageTitle := "Page"
	pageBody := []string{}
	if len(page) > 0 {
		pageTitle = page[0]
		pageBody = page[1:]
	}
	pageBody = m.clampPageBody(pageBody)
	pagePanel := renderPanel(pageTitle, pageBody, contentWidth)

	footerPanel := renderPanel(
		"",
		[]string{"Keys: " + m.footerLine()},
		contentWidth,
	)

	layout := strings.Join([]string{
		headerPanel,
		pagePanel,
		footerPanel,
	}, "\n")
	return paintLayout(layout)
}

func (m Model) clampPageBody(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}

	height := m.height
	if height <= 0 {
		height = 24
	}

	headerLines := panelLineCount("", 4)
	footerLines := panelLineCount("", 1)
	availableForPagePanel := height - headerLines - footerLines
	if availableForPagePanel < 7 {
		availableForPagePanel = 7
	}

	bodyLimit := availableForPagePanel - panelLineCount("page-title", 0)
	if bodyLimit < 1 {
		bodyLimit = 1
	}
	if len(lines) <= bodyLimit {
		return lines
	}
	if bodyLimit == 1 {
		return []string{fmt.Sprintf("... %d more line(s)", len(lines))}
	}
	if len(lines) > 0 && lines[len(lines)-1] == backHomeLine {
		if bodyLimit <= 3 {
			return lines[len(lines)-bodyLimit:]
		}
		headCount := bodyLimit - 3
		hiddenCount := len(lines) - headCount - 2
		if hiddenCount < 0 {
			hiddenCount = 0
		}
		clipped := make([]string, 0, bodyLimit)
		clipped = append(clipped, lines[:headCount]...)
		clipped = append(clipped, fmt.Sprintf("... %d more line(s)", hiddenCount))
		clipped = append(clipped, "")
		clipped = append(clipped, backHomeLine)
		return clipped
	}

	clipped := make([]string, 0, bodyLimit)
	clipped = append(clipped, lines[:bodyLimit-1]...)
	clipped = append(clipped, fmt.Sprintf("... %d more line(s)", len(lines)-bodyLimit+1))
	return clipped
}

func panelLineCount(title string, bodyLines int) int {
	if strings.TrimSpace(title) == "" {
		return bodyLines + 2
	}
	return bodyLines + 4
}

func (m Model) pageLines() []string {
	var lines []string
	switch m.activeScreen {
	case screenHome:
		lines = m.homePageLines()
	case screenDevices:
		lines = m.devicesPageLines()
	case screenControl:
		lines = m.controlPageLines()
	case screenInventory:
		lines = m.inventoryPageLines()
	case screenRegions:
		lines = m.regionsPageLines()
	case screenLogs:
		lines = m.logsPageLines()
	case screenHelp:
		lines = m.helpPageLines()
	default:
		lines = []string{"Unknown page"}
	}
	lines = append(lines, "", backHomeLine)
	return lines
}

func (m Model) homePageLines() []string {
	lines := []string{"Home"}
	lines = append(lines, "Main Menu")
	for i, item := range homeMenu {
		prefix := "  "
		if i == m.homeIndex {
			prefix = "▶ "
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
	lines = append(lines, "Discovered Endpoints")
	for i := start; i < end; i++ {
		candidate := m.candidates[i]
		prefix := "  "
		if i == m.deviceIndex {
			prefix = "▶ "
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
	if m.reader.IsConnected() {
		lines = append(lines, "Connection: connected")
	} else {
		lines = append(lines, "Connection: disconnected")
	}
	if len(m.connectQueue) > 0 {
		lines = append(lines, fmt.Sprintf("Connect plan: %s %d/%d", m.connectActionLabel, m.connectAttempt+1, len(m.connectQueue)))
	}

	botState := "OFFLINE"
	if m.botOnline {
		botState = "ONLINE"
	}
	botScan := "OFF"
	if m.botStats.ScanActive {
		botScan = "ON"
	}
	lines = append(lines, fmt.Sprintf("Bot: %s | scan:%s | socket:%s", botState, botScan, m.botSocket))
	if m.botOnline {
		lines = append(lines, fmt.Sprintf("Bot Cache: %d EPC | draft:%d | refresh:%s", m.botStats.CacheSize, m.botStats.DraftCount, formatShortTime(m.botStats.LastRefreshAt)))
		lines = append(lines, fmt.Sprintf("Bot Submit: ok:%d not_found:%d err:%d", m.botStats.SubmittedOK, m.botStats.SubmitNotFound, m.botStats.SubmitErrors))
		lines = append(lines, fmt.Sprintf("Bot Seen: total:%d hit:%d miss:%d inactive:%d", m.botStats.SeenTotal, m.botStats.CacheHits, m.botStats.CacheMisses, m.botStats.ScanInactive))
	} else {
		lines = append(lines, "Bot status: unavailable")
		if strings.TrimSpace(m.botLastErr) != "" {
			lines = append(lines, "Bot error: "+trimText(m.botLastErr, 72))
		}
	}

	invState := "stopped"
	if m.inventoryRunning {
		invState = "running"
	}
	addr := fmt.Sprintf("0x%02X", m.inventoryAddress)
	if m.inventoryAutoAddr {
		addr = "auto(0x00/0xFF)"
	}
	lines = append(lines, fmt.Sprintf("Inventory: %s | rounds:%d | unique-tags:%d", invState, m.inventoryRounds, m.inventoryTagTotal))
	lines = append(lines, fmt.Sprintf("Protocol: Reader18 | addr:%s | poll:%s | cycle:%s", addr, m.inventoryInterval, m.effectiveInventoryInterval()))
	if m.lastTagEPC != "" {
		lines = append(lines, fmt.Sprintf("Last Tag: %s | Ant:%d | RSSI:%d", trimText(m.lastTagEPC, 28), m.lastTagAntenna, m.lastTagRSSI))
		if m.showPhaseFreq {
			lines = append(lines, "Phase/Freq: n/a (not present in cmd 0x01 frame)")
		}
	}
	if m.lastRX != "" {
		lines = append(lines, "Last RX: "+trimText(m.lastRX, 64))
	}

	lines = append(lines, "", "Actions")
	for i, item := range controlMenu {
		prefix := "  "
		if i == m.controlIndex {
			prefix = "▶ "
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

func (m Model) inventoryPageLines() []string {
	lines := []string{"Inventory Tune"}
	lines = append(lines, "Use h/l or left/right to change values, Enter to run action")
	lines = append(lines, "")

	rows := []string{
		fmt.Sprintf("Q Value: %d", m.inventoryQValue),
		fmt.Sprintf("Session: %d", m.inventorySession),
		fmt.Sprintf("Target: %s", targetLabel(m.inventoryTarget)),
		fmt.Sprintf("Scan Time (x100ms): %d", m.inventoryScanTime),
		fmt.Sprintf("No-tag A/B Switch Count: %d", m.inventoryNoTagAB),
		fmt.Sprintf("Phase/Freq Columns: %s", onOff(m.showPhaseFreq)),
		fmt.Sprintf("Antenna Mask (bit): 0x%02X (%s)", m.inventoryAntMask, maskBits(m.inventoryAntMask)),
		fmt.Sprintf("Poll Interval: %s (effective cycle: %s)", m.inventoryInterval, m.effectiveInventoryInterval()),
		"Apply Parameters To Reader",
		"Antenna Scan (use mask)",
		"Fast Preset",
		"Balanced Preset",
		"Long Range Preset",
	}

	start, end := listWindow(m.inventoryIndex, len(rows), m.inventoryViewSize())
	for i := start; i < end; i++ {
		row := rows[i]
		prefix := "  "
		if i == m.inventoryIndex {
			prefix = "▶ "
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s", prefix, i+1, row))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Rows: %d-%d of %d", start+1, end, len(rows)))
	lines = append(lines, "Tip: Session 2/3 + A/B switch helps for far tags")
	lines = append(lines, "Speed tip: keep Scan Time low (1-3) for realtime reads")
	return lines
}

func (m Model) regionsPageLines() []string {
	lines := []string{"Regions"}
	if len(regions.Catalog) == 0 {
		return append(lines, "No region catalog")
	}

	start, end := listWindow(m.regionCursor, len(regions.Catalog), m.regionViewSize())
	lines = append(lines, "")
	lines = append(lines, "Region Catalog")
	for i := start; i < end; i++ {
		region := regions.Catalog[i]
		prefix := "  "
		if i == m.regionCursor {
			prefix = "▶ "
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

	return fmt.Sprintf("Reader %s | Region %s | Scan %s | Verified %d/%d", connection, regionCode, scanState, countVerifiedCandidates(m.candidates), len(m.candidates))
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

func (m Model) inventoryViewSize() int {
	if m.height <= 0 {
		return 6
	}
	size := m.height - 20
	if size < 4 {
		size = 4
	}
	if size > 8 {
		size = 8
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

func renderPanel(title string, lines []string, contentWidth int) string {
	if contentWidth < 24 {
		contentWidth = 24
	}

	var b strings.Builder
	horizontal := strings.Repeat("─", contentWidth+2)
	top := "┌" + horizontal + "┐"
	mid := "├" + horizontal + "┤"
	bottom := "└" + horizontal + "┘"

	b.WriteString(top)
	if strings.TrimSpace(title) != "" {
		b.WriteString("\n")
		titleText := "[" + strings.ToUpper(strings.TrimSpace(title)) + "]"
		b.WriteString("│ ")
		b.WriteString(padRight(trimText(titleText, contentWidth), contentWidth))
		b.WriteString(" │\n")
		b.WriteString(mid)
	}

	if len(lines) == 0 {
		b.WriteString("\n")
		b.WriteString("│ ")
		b.WriteString(strings.Repeat(" ", contentWidth))
		b.WriteString(" │\n")
		b.WriteString(bottom)
		return b.String()
	}

	b.WriteString("\n")
	for i, line := range lines {
		clipped := trimText(line, contentWidth)
		b.WriteString("│ ")
		b.WriteString(padRight(clipped, contentWidth))
		b.WriteString(" │")
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(bottom)
	return b.String()
}

func (m Model) panelContentWidth() int {
	if m.width <= 0 {
		return 78
	}
	width := m.width - 4
	if width < 36 {
		width = 36
	}
	if width > 120 {
		width = 120
	}
	return width
}

func (m Model) statusLine() string {
	return statusTag(m.status) + " " + m.status
}

func statusTag(status string) string {
	text := strings.ToLower(status)
	switch {
	case strings.Contains(text, "failed"),
		strings.Contains(text, "error"),
		strings.Contains(text, "timeout"),
		strings.Contains(text, "disconnected"),
		strings.Contains(text, "closed"):
		return "[ERROR]"
	case strings.Contains(text, "no tag"),
		strings.Contains(text, "stopped"),
		strings.Contains(text, "idle"),
		strings.Contains(text, "antenna check"):
		return "[WARN ]"
	case strings.Contains(text, "connected"),
		strings.Contains(text, "running"),
		strings.Contains(text, "started"),
		strings.Contains(text, "new tag"),
		strings.Contains(text, "received"):
		return "[ OK  ]"
	default:
		return "[INFO ]"
	}
}

func targetLabel(target byte) string {
	if target&0x01 == 0 {
		return "A"
	}
	return "B"
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

func formatShortTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return t.Format("15:04:05")
}

func maskBits(mask byte) string {
	parts := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		if mask&(byte(1)<<i) != 0 {
			parts = append(parts, fmt.Sprintf("ANT%d", i+1))
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ",")
}

func runeLen(s string) int {
	return len([]rune(s))
}

func padRight(s string, width int) string {
	n := runeLen(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

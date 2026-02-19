package tui

import (
	"fmt"
	"strings"
	"time"

	"new_era_go/internal/regions"
	tuiupdate "new_era_go/internal/tui/update"
)

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
	verifiedCount := tuiupdate.CountVerifiedCandidates(m.candidates)
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

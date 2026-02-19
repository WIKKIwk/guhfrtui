package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"new_era_go/internal/regions"
)

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

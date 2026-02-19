package tui

import (
	"fmt"
	"strings"
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

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	borderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("0")).
			Bold(true)

	tabsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Bold(true)

	metaOnlineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	metaOfflineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	panelTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("0")).
			Bold(true)

	statusOKStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	statusWarnStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	statusErrStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	statusInfoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	selectedLineStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color("255")).
				Bold(true)

	backLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("250")).
			Bold(true)

	keysStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	verifiedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	sectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Bold(true)

	bodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))
)

func paintLayout(layout string) string {
	if layout == "" {
		return layout
	}

	lines := strings.Split(layout, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "┌"), strings.HasPrefix(line, "├"), strings.HasPrefix(line, "└"),
			strings.HasPrefix(line, "╭"), strings.HasPrefix(line, "╰"):
			lines[i] = borderStyle.Render(line)
		case strings.Contains(line, "ST-8508 Reader TUI"):
			lines[i] = headerStyle.Render(line)
		case strings.Contains(line, "▣ ") || strings.Contains(line, "□ "):
			lines[i] = tabsStyle.Render(line)
		case strings.Contains(line, "Reader ONLINE"):
			lines[i] = metaOnlineStyle.Render(line)
		case strings.Contains(line, "Reader OFFLINE"):
			lines[i] = metaOfflineStyle.Render(line)
		case strings.Contains(line, "[OK]"):
			lines[i] = statusOKStyle.Render(line)
		case strings.Contains(line, "[WARN]"):
			lines[i] = statusWarnStyle.Render(line)
		case strings.Contains(line, "[ERR]"):
			lines[i] = statusErrStyle.Render(line)
		case strings.Contains(line, "[INFO ]"):
			lines[i] = statusInfoStyle.Render(line)
		case strings.Contains(line, "│ ▶ "):
			lines[i] = selectedLineStyle.Render(line)
		case strings.Contains(line, "Back to Home"):
			lines[i] = backLineStyle.Render(line)
		case strings.Contains(line, "[VERIFIED]"):
			lines[i] = verifiedStyle.Render(line)
		case strings.Contains(line, "Main Menu"),
			strings.Contains(line, "Discovered Endpoints"),
			strings.Contains(line, "Actions"),
			strings.Contains(line, "Region Catalog"),
			strings.Contains(line, "Recommended flow"):
			lines[i] = sectionStyle.Render(line)
		case strings.Contains(line, "│ ─"):
			lines[i] = borderStyle.Render(line)
		case isPanelTitleLine(line):
			lines[i] = panelTitleStyle.Render(line)
		case strings.Contains(line, "Keys:"):
			lines[i] = keysStyle.Render(line)
		case strings.HasPrefix(line, "│ "):
			lines[i] = bodyStyle.Render(line)
		}
	}

	return strings.Join(lines, "\n")
}

func isPanelTitleLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "│ ") || !strings.HasSuffix(trimmed, " │") {
		return false
	}
	content := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "│ "), " │"))
	if !strings.HasPrefix(content, "[") || !strings.HasSuffix(content, "]") {
		return false
	}
	return !strings.Contains(content, "[OK]") &&
		!strings.Contains(content, "[WARN]") &&
		!strings.Contains(content, "[ERR]") &&
		!strings.Contains(content, "[INFO ]")
}

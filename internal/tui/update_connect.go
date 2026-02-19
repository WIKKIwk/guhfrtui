package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	tuiupdate "new_era_go/internal/tui/update"
)

func (m Model) beginConnectPlan(actionLabel string, preferredIndex int) (tea.Model, tea.Cmd) {
	plan := tuiupdate.BuildConnectPlan(m.candidates, preferredIndex, m.scanOptions.Ports)
	if len(plan) == 0 {
		m.status = actionLabel + ": no reachable endpoints"
		m.pushLog(strings.ToLower(actionLabel) + " failed: no endpoints in plan")
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		m.connecting = false
		return m, nil
	}

	m.connectQueue = plan
	m.connectAttempt = 0
	m.connectActionLabel = actionLabel
	m.connecting = true

	first := plan[0]
	m.status = fmt.Sprintf("%s: connecting %s (1/%d)", actionLabel, first.Address(), len(plan))
	m.pushLog(fmt.Sprintf("%s connect 1/%d -> %s", strings.ToLower(actionLabel), len(plan), first.Address()))
	return m, reconnectCmd(m.reader, first)
}

func (m Model) retryNextConnect(reason string) (tea.Model, tea.Cmd, bool) {
	if len(m.connectQueue) == 0 {
		return m, nil, false
	}

	label := strings.TrimSpace(m.connectActionLabel)
	if label == "" {
		label = "Connect"
	}
	cause := strings.TrimSpace(reason)
	if cause == "" {
		cause = "connection error"
	}

	next := m.connectAttempt + 1
	total := len(m.connectQueue)
	if next >= total {
		current := m.connectQueue[m.connectAttempt]
		m.pushLog(fmt.Sprintf("%s failed at %s: %s", strings.ToLower(label), current.Address(), cause))
		m.connectQueue = nil
		m.connectAttempt = 0
		m.connectActionLabel = ""
		m.connecting = false
		return m, nil, false
	}

	m.connectAttempt = next
	endpoint := m.connectQueue[next]
	m.connecting = true
	m.status = fmt.Sprintf("%s: retry %d/%d -> %s", label, next+1, total, endpoint.Address())
	m.pushLog(fmt.Sprintf("%s retry %d/%d (%s) -> %s", strings.ToLower(label), next+1, total, cause, endpoint.Address()))
	return m, reconnectCmd(m.reader, endpoint), true
}

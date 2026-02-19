package tui

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

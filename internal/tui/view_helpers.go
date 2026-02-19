package tui

import (
	"fmt"
	"strings"
	"time"
)

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

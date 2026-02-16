package tui

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"new_era_go/internal/discovery"
	"new_era_go/internal/reader"
)

func runScanCmd(opts discovery.ScanOptions) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		start := time.Now()
		candidates, err := discovery.Scan(ctx, opts)
		return scanFinishedMsg{
			Candidates: candidates,
			Err:        err,
			Duration:   time.Since(start),
		}
	}
}

func connectCmd(client *reader.Client, endpoint reader.Endpoint) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := client.Connect(ctx, endpoint, 3*time.Second)
		return connectFinishedMsg{Endpoint: endpoint, Err: err}
	}
}

func reconnectCmd(client *reader.Client, endpoint reader.Endpoint) tea.Cmd {
	return func() tea.Msg {
		_ = client.Disconnect()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		err := client.Connect(ctx, endpoint, 3*time.Second)
		return connectFinishedMsg{Endpoint: endpoint, Err: err}
	}
}

func disconnectCmd(client *reader.Client) tea.Cmd {
	return func() tea.Msg {
		err := client.Disconnect()
		return disconnectFinishedMsg{Err: err}
	}
}

func sendNamedCmd(client *reader.Client, name string, payload []byte) tea.Cmd {
	data := make([]byte, len(payload))
	copy(data, payload)
	return func() tea.Msg {
		err := client.SendRaw(data, 2*time.Second)
		return commandSentMsg{Name: name, Sent: len(data), Err: err}
	}
}

func inventoryTickCmd(delay time.Duration) tea.Cmd {
	if delay <= 0 {
		return func() tea.Msg { return inventoryTickMsg{} }
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return inventoryTickMsg{} })
}

func probeTimeoutCmd(delay time.Duration) tea.Cmd {
	if delay <= 0 {
		return func() tea.Msg { return probeTimeoutMsg{} }
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return probeTimeoutMsg{} })
}

func waitPacketCmd(ch <-chan reader.Packet) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return packetChannelClosedMsg{}
		}
		packet, ok := <-ch
		if !ok {
			return packetChannelClosedMsg{}
		}
		return packetMsg{Packet: packet}
	}
}

func waitReaderErrCmd(ch <-chan error) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return readerErrChannelClosedMsg{}
		}
		err, ok := <-ch
		if !ok {
			return readerErrChannelClosedMsg{}
		}
		return readerErrMsg{Err: err}
	}
}

func parseHexInput(in string) ([]byte, error) {
	text := strings.TrimSpace(in)
	if text == "" {
		return nil, fmt.Errorf("empty input")
	}

	replacer := strings.NewReplacer(",", " ", ":", " ", "\n", " ", "\r", " ", "\t", " ")
	text = replacer.Replace(text)

	tokens := strings.Fields(text)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no hex bytes found")
	}

	out := make([]byte, 0, len(tokens))
	for _, token := range tokens {
		norm := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(token)), "0x")
		if norm == "" {
			continue
		}
		if len(norm)%2 != 0 {
			norm = "0" + norm
		}
		decoded, err := hex.DecodeString(norm)
		if err != nil {
			return nil, fmt.Errorf("invalid token %q", token)
		}
		out = append(out, decoded...)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no hex bytes parsed")
	}
	return out, nil
}

func formatHex(data []byte, maxBytes int) string {
	if len(data) == 0 {
		return ""
	}

	truncated := false
	if len(data) > maxBytes {
		data = data[:maxBytes]
		truncated = true
	}

	encoded := strings.ToUpper(hex.EncodeToString(data))
	parts := make([]string, 0, len(encoded)/2)
	for i := 0; i+1 < len(encoded); i += 2 {
		parts = append(parts, encoded[i:i+2])
	}

	out := strings.Join(parts, " ")
	if truncated {
		out += " ..."
	}
	return out
}

func parseDigit(key string) (int, bool) {
	if len(key) != 1 {
		return 0, false
	}
	if key[0] < '1' || key[0] > '9' {
		return 0, false
	}
	value, err := strconv.Atoi(key)
	if err != nil {
		return 0, false
	}
	return value - 1, true
}

func trimText(in string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(in)
	if len(r) <= max {
		return in
	}
	if max <= 3 {
		return string(r[:max])
	}
	return string(r[:max-3]) + "..."
}

func listWindow(cursor, total, window int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	if window <= 0 || window >= total {
		return 0, total
	}

	start := cursor - window/2
	if start < 0 {
		start = 0
	}
	end := start + window
	if end > total {
		end = total
		start = end - window
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func (m *Model) pushLog(line string) {
	stamp := time.Now().Format("15:04:05")
	m.logs = append(m.logs, fmt.Sprintf("[%s] %s", stamp, line))
	if len(m.logs) > 240 {
		m.logs = m.logs[len(m.logs)-240:]
	}
}

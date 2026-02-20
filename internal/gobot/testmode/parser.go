package testmode

import (
	"bufio"
	"bytes"
	"strings"

	"new_era_go/internal/gobot/erp"
)

func parseEPCFile(content []byte) ([]string, LoadStats) {
	stats := LoadStats{}
	if len(content) == 0 {
		return nil, stats
	}

	unique := make(map[string]struct{})
	out := make([]string, 0, 128)

	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if lineNo == 0 {
			raw = strings.TrimPrefix(raw, "\uFEFF")
		}
		lineNo++
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		stats.TotalLines++
		epc := erp.NormalizeEPC(raw)
		if epc == "" {
			stats.InvalidLines++
			continue
		}

		stats.ValidLines++
		if _, exists := unique[epc]; exists {
			stats.DuplicateLines++
			continue
		}
		unique[epc] = struct{}{}
		out = append(out, epc)
	}

	stats.UniqueEPCs = len(out)
	return out, stats
}

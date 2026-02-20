package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (b *Bot) handleCacheDump(ctx context.Context, chatID int64) error {
	b.addChat(chatID)

	draftEPCs := b.svc.DraftEPCs()
	seenEPCs := b.svc.RecentSeenEPCs()
	logDir := resolveCacheDumpDir()

	draftFile, seenFile, err := writeCacheDumpFiles(logDir, time.Now(), draftEPCs, seenEPCs)
	if err != nil {
		return b.sendMessage(ctx, chatID, "Cache dump yozishda xato: "+err.Error())
	}

	if err := b.sendMessage(ctx, chatID, fmt.Sprintf("Cache dump tayyor. 2 ta fayl yuborilyapti.\nDraft EPC: %d\nSeen EPC: %d", len(draftEPCs), len(seenEPCs))); err != nil {
		return err
	}
	if err := b.sendDocument(ctx, chatID, draftFile, "Draft snapshot"); err != nil {
		return b.sendMessage(ctx, chatID, "Draft fayl yuborilmadi: "+err.Error())
	}
	if err := b.sendDocument(ctx, chatID, seenFile, "Seen EPC snapshot"); err != nil {
		return b.sendMessage(ctx, chatID, "Seen EPC fayl yuborilmadi: "+err.Error())
	}
	return nil
}

func resolveCacheDumpDir() string {
	if custom := strings.TrimSpace(os.Getenv("BOT_CACHE_DUMP_DIR")); custom != "" {
		return custom
	}
	if logDir := strings.TrimSpace(os.Getenv("BOT_LOG_DIR")); logDir != "" {
		return logDir
	}
	return "logs"
}

func writeCacheDumpFiles(dir string, now time.Time, draftEPCs, seenEPCs []string) (string, string, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "logs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}

	draftPath := filepath.Join(dir, "cache_draft_epcs.txt")
	seenPath := filepath.Join(dir, "cache_seen_epcs.txt")

	draftBody := buildEPCListFileBody("draft_epc_count", now, draftEPCs)
	if err := writeFileAtomic(draftPath, []byte(draftBody), 0o644); err != nil {
		return "", "", err
	}

	seenBody := buildEPCListFileBody("seen_epc_count", now, seenEPCs)
	if err := writeFileAtomic(seenPath, []byte(seenBody), 0o644); err != nil {
		return "", "", err
	}

	return draftPath, seenPath, nil
}

func buildEPCListFileBody(header string, now time.Time, epcs []string) string {
	clean := make([]string, 0, len(epcs))
	for _, epc := range epcs {
		epc = strings.TrimSpace(epc)
		if epc == "" {
			continue
		}
		clean = append(clean, epc)
	}

	var sb strings.Builder
	sb.WriteString("# generated_at=")
	sb.WriteString(now.Format(time.RFC3339))
	sb.WriteString("\n# ")
	sb.WriteString(header)
	sb.WriteString("=")
	sb.WriteString(fmt.Sprintf("%d", len(clean)))
	sb.WriteString("\n")
	for _, epc := range clean {
		sb.WriteString(epc)
		sb.WriteString("\n")
	}
	return sb.String()
}

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

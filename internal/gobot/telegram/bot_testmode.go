package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	"new_era_go/internal/gobot/testmode"
)

const maxTestFileSize = 2 << 20

func (b *Bot) handleTestFileUpload(ctx context.Context, chatID int64, sourceMessageID int, doc document) error {
	if !b.testMode.IsAwaitingFile(chatID) {
		return b.sendMessage(ctx, chatID, "ℹ️ Avval /test buyrug'ini yuboring, keyin fayl tashlang.")
	}
	if sourceMessageID > 0 {
		_ = b.deleteMessage(ctx, chatID, sourceMessageID)
	}

	fileName := strings.TrimSpace(doc.FileName)
	if fileName == "" {
		fileName = "epc_test.txt"
	}

	content, err := b.fetchTelegramFileContent(ctx, doc.FileID, maxTestFileSize)
	if err != nil {
		return b.sendOrEditTestPrompt(ctx, chatID, "❌ Faylni olishda xato: "+err.Error())
	}

	stats, err := b.testMode.LoadFile(chatID, fileName, content)
	if err != nil {
		return b.sendOrEditTestPrompt(ctx, chatID, "❌ Test boshlanmadi: "+err.Error())
	}

	// New file replaces previous test session in memory for this chat.
	b.clearTestReadRefsForChat(chatID)

	text := fmt.Sprintf(
		"📥 Fayl qabul qilindi: %s\nSatrlar: %d\nYaroqli EPC: %d\nUnikal EPC: %d\nDublikat: %d\nNoto'g'ri: %d\n\n🟢 Test boshlandi.",
		stats.FileName,
		stats.TotalLines,
		stats.ValidLines,
		stats.UniqueEPCs,
		stats.DuplicateLines,
		stats.InvalidLines,
	)
	return b.sendOrEditTestPrompt(ctx, chatID, text)
}

func (b *Bot) handleTestStop(ctx context.Context, chatID int64) error {
	b.addChat(chatID)
	b.takeTestPromptMessage(chatID)

	result, err := b.testMode.Stop(chatID)
	if err != nil {
		return b.sendMessage(ctx, chatID, "⚠️ "+err.Error())
	}

	refs := b.takeTestReadRefs(result.SessionID)
	deleteChatID := refs.chatID
	if deleteChatID == 0 {
		deleteChatID = result.ChatID
	}
	if deleteChatID == 0 {
		deleteChatID = chatID
	}
	for _, messageID := range refs.messageIDs {
		_ = b.deleteMessage(ctx, deleteChatID, messageID)
	}

	text := fmt.Sprintf(
		"📊 Test natijasi\nFayl: %s\nJami EPC: %d\nO'qildi: %d\nO'qilmadi: %d",
		result.FileName,
		result.Total,
		result.Read,
		result.Unread,
	)
	return b.sendMessage(ctx, chatID, text)
}

func (b *Bot) OnReaderEPC(epc string) {
	match := b.testMode.RecordRead(epc)
	if !match.Matched || !match.NewlyRead || match.ChatID == 0 {
		return
	}

	go b.sendTestMatch(match)
}

func (b *Bot) sendTestMatch(match testmode.MatchResult) {
	if !b.testMode.IsSessionActive(match.SessionID) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	text := fmt.Sprintf("✅ O'qildi: %s (%d/%d)", match.EPC, match.ReadCount, match.Total)
	messageID, err := b.sendMessageWithID(ctx, match.ChatID, text)
	if err != nil {
		return
	}

	// Stop command may arrive while message is being sent.
	if !b.testMode.IsSessionActive(match.SessionID) {
		_ = b.deleteMessage(ctx, match.ChatID, messageID)
		return
	}
	b.appendTestReadRef(match.SessionID, match.ChatID, messageID)
}

func (b *Bot) appendTestReadRef(sessionID uint64, chatID int64, messageID int) {
	if sessionID == 0 || chatID == 0 || messageID == 0 {
		return
	}

	b.testMu.Lock()
	defer b.testMu.Unlock()

	ref := b.testReadRefs[sessionID]
	if ref.chatID == 0 {
		ref.chatID = chatID
	}
	ref.messageIDs = append(ref.messageIDs, messageID)
	b.testReadRefs[sessionID] = ref
}

func (b *Bot) takeTestReadRefs(sessionID uint64) testReadRefs {
	if sessionID == 0 {
		return testReadRefs{}
	}

	b.testMu.Lock()
	defer b.testMu.Unlock()

	ref := b.testReadRefs[sessionID]
	delete(b.testReadRefs, sessionID)
	return ref
}

func (b *Bot) clearTestReadRefsForChat(chatID int64) {
	if chatID == 0 {
		return
	}

	b.testMu.Lock()
	defer b.testMu.Unlock()

	for sessionID, ref := range b.testReadRefs {
		if ref.chatID == chatID {
			delete(b.testReadRefs, sessionID)
		}
	}
}

func (b *Bot) setTestPromptMessage(chatID int64, messageID int) {
	if chatID == 0 || messageID == 0 {
		return
	}
	b.testMu.Lock()
	b.testPrompts[chatID] = messageID
	b.testMu.Unlock()
}

func (b *Bot) takeTestPromptMessage(chatID int64) int {
	if chatID == 0 {
		return 0
	}
	b.testMu.Lock()
	defer b.testMu.Unlock()
	messageID := b.testPrompts[chatID]
	delete(b.testPrompts, chatID)
	return messageID
}

func (b *Bot) sendOrEditTestPrompt(ctx context.Context, chatID int64, text string) error {
	promptID := b.takeTestPromptMessage(chatID)
	if promptID > 0 {
		if err := b.editMessage(ctx, chatID, promptID, text); err == nil {
			return nil
		}
	}
	return b.sendMessage(ctx, chatID, text)
}

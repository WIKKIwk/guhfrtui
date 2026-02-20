package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type MessageRef struct {
	ChatID    int64
	MessageID int
}

func (b *Bot) SendStartupNotice(ctx context.Context, text string) []MessageRef {
	chatIDs := b.snapshotChats()
	if len(chatIDs) == 0 {
		return nil
	}

	refs := make([]MessageRef, 0, len(chatIDs))
	for _, chatID := range chatIDs {
		messageID, err := b.sendMessageWithID(ctx, chatID, text)
		if err != nil {
			continue
		}
		refs = append(refs, MessageRef{ChatID: chatID, MessageID: messageID})
	}
	return refs
}

func (b *Bot) EditNotices(ctx context.Context, refs []MessageRef, text string) {
	for _, ref := range refs {
		if ref.ChatID == 0 || ref.MessageID == 0 {
			continue
		}
		if err := b.editMessage(ctx, ref.ChatID, ref.MessageID, text); err != nil {
			// Fall back to a fresh message if edit is rejected (e.g. too old message).
			_ = b.sendMessage(ctx, ref.ChatID, text)
		}
	}
}

func (b *Bot) sendMessageWithID(ctx context.Context, chatID int64, text string) (int, error) {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")

	body, err := b.telegramFormPost(ctx, "/sendMessage", form)
	if err != nil {
		return 0, err
	}

	var resp telegramMessageEnvelope
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("telegram sendMessage decode: %w", err)
	}
	if !resp.OK {
		return 0, fmt.Errorf("telegram sendMessage not ok: %s", strings.TrimSpace(resp.Description))
	}
	if resp.Result.MessageID == 0 {
		return 0, fmt.Errorf("telegram sendMessage missing message_id")
	}
	return resp.Result.MessageID, nil
}

func (b *Bot) editMessage(ctx context.Context, chatID int64, messageID int, text string) error {
	form := url.Values{}
	form.Set("chat_id", strconv.FormatInt(chatID, 10))
	form.Set("message_id", strconv.Itoa(messageID))
	form.Set("text", text)
	form.Set("disable_web_page_preview", "true")

	body, err := b.telegramFormPost(ctx, "/editMessageText", form)
	if err != nil {
		return err
	}

	var resp telegramMessageEnvelope
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("telegram editMessageText decode: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("telegram editMessageText not ok: %s", strings.TrimSpace(resp.Description))
	}
	return nil
}

func (b *Bot) telegramFormPost(ctx context.Context, path string, form url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("telegram %s HTTP %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

type telegramMessageEnvelope struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      struct {
		MessageID int `json:"message_id"`
	} `json:"result"`
}

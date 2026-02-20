package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

func (b *Bot) sendDocument(ctx context.Context, chatID int64, filePath, caption string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("chat_id", strconv.FormatInt(chatID, 10)); err != nil {
		_ = writer.Close()
		return err
	}
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			_ = writer.Close()
			return err
		}
	}

	part, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		_ = writer.Close()
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.baseURL+"/sendDocument", &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := b.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		return fmt.Errorf("telegram sendDocument HTTP %d: %s", resp.StatusCode, string(raw))
	}

	return nil
}

package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (b *Bot) fetchTelegramFileContent(ctx context.Context, fileID string, maxBytes int64) ([]byte, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return nil, fmt.Errorf("file_id bo'sh")
	}
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}

	filePath, err := b.resolveTelegramFilePath(ctx, fileID)
	if err != nil {
		return nil, err
	}

	reqURL := "https://api.telegram.org/file/bot" + b.token + "/" + filePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("telegram file HTTP %d: %s", resp.StatusCode, compactBody(body))
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("fayl juda katta: max %d byte", maxBytes)
	}
	return body, nil
}

func (b *Bot) resolveTelegramFilePath(ctx context.Context, fileID string) (string, error) {
	values := url.Values{}
	values.Set("file_id", fileID)

	reqURL := b.baseURL + "/getFile?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := b.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("telegram getFile HTTP %d: %s", resp.StatusCode, compactBody(body))
	}

	var env telegramFileEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", fmt.Errorf("telegram getFile decode: %w", err)
	}
	if !env.OK {
		return "", fmt.Errorf("telegram getFile not ok: %s", strings.TrimSpace(env.Description))
	}
	path := strings.TrimSpace(env.Result.FilePath)
	if path == "" {
		return "", fmt.Errorf("telegram getFile file_path bo'sh")
	}
	return path, nil
}

type telegramFileEnvelope struct {
	OK          bool   `json:"ok"`
	Description string `json:"description"`
	Result      struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

func compactBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 320 {
		return s[:320] + "..."
	}
	return s
}

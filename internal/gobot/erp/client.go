package erp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	auth    string
	http    *http.Client
}

type FetchResult struct {
	EPCs       []string
	DraftCount int
}

type SubmitStatus string

const (
	SubmitStatusSubmitted SubmitStatus = "submitted"
	SubmitStatusNotFound  SubmitStatus = "not_found"
)

func New(baseURL, apiKey, apiSecret string, timeout time.Duration) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return &Client{
		baseURL: baseURL,
		auth:    fmt.Sprintf("token %s:%s", strings.TrimSpace(apiKey), strings.TrimSpace(apiSecret)),
		http: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) FetchDraftEPCs(ctx context.Context) (FetchResult, error) {
	q := url.Values{}
	q.Set("limit", "5000")
	q.Set("include_items", "0")
	q.Set("only_with_epc", "1")
	q.Set("compact", "1")
	q.Set("epc_only", "1")

	endpoint := c.baseURL + "/api/method/titan_telegram.api.get_open_stock_entry_drafts_fast?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return FetchResult{}, err
	}
	req.Header.Set("Authorization", c.auth)

	resp, err := c.http.Do(req)
	if err != nil {
		return FetchResult{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return FetchResult{}, fmt.Errorf("ERP fast drafts HTTP %d: %s", resp.StatusCode, compactBody(body))
	}

	var payload fastDraftEnvelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return FetchResult{}, fmt.Errorf("ERP fast drafts decode: %w", err)
	}

	msg := payload.Message
	if !msg.OK {
		return FetchResult{}, fmt.Errorf("ERP fast drafts error: %s", msg.Error)
	}
	if !msg.EPCOnly {
		return FetchResult{}, fmt.Errorf("ERP fast drafts response is not epc_only")
	}

	unique := make(map[string]struct{}, len(msg.EPCs))
	out := make([]string, 0, len(msg.EPCs))
	for _, raw := range msg.EPCs {
		epc := NormalizeEPC(raw)
		if epc == "" {
			continue
		}
		if _, exists := unique[epc]; exists {
			continue
		}
		unique[epc] = struct{}{}
		out = append(out, epc)
	}

	draftCount := msg.CountDrafts
	if draftCount == 0 && msg.DraftCountAlt > 0 {
		draftCount = msg.DraftCountAlt
	}

	return FetchResult{
		EPCs:       out,
		DraftCount: draftCount,
	}, nil
}

func (c *Client) SubmitByEPC(ctx context.Context, epc string) (SubmitStatus, error) {
	epc = NormalizeEPC(epc)
	if epc == "" {
		return "", fmt.Errorf("epc is empty")
	}

	body, _ := json.Marshal(map[string]string{"epc": epc})
	endpoint := c.baseURL + "/api/method/titan_telegram.api.submit_open_stock_entry_by_epc"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("ERP submit HTTP %d: %s", resp.StatusCode, compactBody(respBody))
	}

	var payload submitEnvelope
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("ERP submit decode: %w", err)
	}

	if payload.Message.OK && payload.Message.Status == string(SubmitStatusSubmitted) {
		return SubmitStatusSubmitted, nil
	}
	if payload.Message.OK && payload.Message.Status == string(SubmitStatusNotFound) {
		return SubmitStatusNotFound, nil
	}
	if payload.Message.Error != "" {
		return "", fmt.Errorf("ERP submit error: %s", payload.Message.Error)
	}
	return "", fmt.Errorf("ERP submit unexpected payload")
}

func NormalizeEPC(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'F') {
			b.WriteByte(ch)
		}
	}
	return b.String()
}

func compactBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 320 {
		return s[:320] + "..."
	}
	return s
}

type fastDraftEnvelope struct {
	Message struct {
		OK            bool     `json:"ok"`
		Error         string   `json:"error"`
		EPCOnly       bool     `json:"epc_only"`
		EPCs          []string `json:"epcs"`
		CountDrafts   int      `json:"count_drafts"`
		DraftCountAlt int      `json:"draft_count"`
	} `json:"message"`
}

type submitEnvelope struct {
	Message struct {
		OK     bool   `json:"ok"`
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"message"`
}

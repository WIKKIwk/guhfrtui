package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"new_era_go/internal/gobot/service"
	"new_era_go/internal/gobot/testmode"
)

type Bot struct {
	token       string
	baseURL     string
	http        *http.Client
	pollTimeout time.Duration
	svc         *service.Service
	scanner     Scanner
	chatsFile   string
	notifyRetry int

	mu    sync.Mutex
	chats map[int64]struct{}

	testMu       sync.Mutex
	testMode     *testmode.Manager
	testReadRefs map[uint64]testReadRefs
}

type testReadRefs struct {
	chatID     int64
	messageIDs []int
}

type Scanner interface {
	Start(ctx context.Context) error
	Stop()
	StatusText() string
}

type RangeTuner interface {
	SetLongRangeMode(enabled bool) string
	LongRangeMode() bool
}

func New(token string, requestTimeout, pollTimeout time.Duration, svc *service.Service, scanner Scanner) *Bot {
	chatsFile := strings.TrimSpace(os.Getenv("BOT_CHAT_STORE_FILE"))
	if chatsFile == "" {
		chatsFile = "logs/telegram_chats.json"
	}

	b := &Bot{
		token:        strings.TrimSpace(token),
		baseURL:      "https://api.telegram.org/bot" + strings.TrimSpace(token),
		http:         &http.Client{Timeout: requestTimeout},
		pollTimeout:  pollTimeout,
		svc:          svc,
		scanner:      scanner,
		chatsFile:    chatsFile,
		notifyRetry:  2,
		chats:        make(map[int64]struct{}),
		testMode:     testmode.New(),
		testReadRefs: make(map[uint64]testReadRefs),
	}
	b.loadChats()
	return b
}

func (b *Bot) Run(ctx context.Context) {
	if b.token == "" {
		log.Printf("[bot] telegram token empty; telegram loop skipped")
		return
	}

	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			log.Printf("[bot] telegram poll error: %v", err)
			time.Sleep(1200 * time.Millisecond)
			continue
		}

		for _, upd := range updates {
			if upd.UpdateID >= offset {
				offset = upd.UpdateID + 1
			}
			if err := b.handleUpdate(ctx, upd); err != nil {
				log.Printf("[bot] telegram handle error: %v", err)
			}
		}
	}
}

func (b *Bot) Notify(text string) {
	chatIDs := b.snapshotChats()
	if len(chatIDs) == 0 {
		log.Printf("[bot] notify skipped: no registered chats")
		return
	}
	for _, chatID := range chatIDs {
		if err := b.sendMessageWithRetry(chatID, text); err != nil {
			log.Printf("[bot] notify chat=%d failed: %v", chatID, err)
		}
	}
}

func (b *Bot) handleUpdate(ctx context.Context, upd update) error {
	msg := upd.Message
	if msg.Chat.ID == 0 {
		return nil
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	if text == "" {
		if msg.Document != nil && b.testMode.IsAwaitingFile(msg.Chat.ID) {
			return b.handleTestFileUpload(ctx, msg.Chat.ID, *msg.Document)
		}
		return nil
	}

	cmd, args := parseCommand(text)
	switch cmd {
	case "/start", "/help":
		b.addChat(msg.Chat.ID)
		text := "🤖 RFID Go bot tayyor.\n" +
			"📚 Buyruqlar:\n" +
			"/scan - reader scan ni boshlash ▶️\n" +
			"/read - /scan bilan bir xil (start)\n" +
			"/read stop - /stop bilan bir xil\n" +
			"/stop - reader scan ni to'xtatish ⏹️\n" +
			"/status - holat ℹ️\n" +
			"/cache - draft/epc snapshot fayllarini yozish 📁\n" +
			"/range20 on|off|status - long-range profil 📡\n" +
			"/range20_on | /range20_off - tez yoqish/o'chirish ⚡\n" +
			"/turbo - cache ni darrov yangilash 🚀\n" +
			"/test - EPC test uchun txt fayl kutish 🧪\n" +
			"/test_stop - testni yakunlash va natijani olish 🛑"
		return b.sendMessage(ctx, msg.Chat.ID, text)

	case "/scan":
		return b.handleScanStart(ctx, msg.Chat.ID, "telegram_scan")

	case "/read":
		if len(args) > 0 {
			action := strings.ToLower(strings.TrimSpace(args[0]))
			switch action {
			case "stop", "off", "0":
				return b.handleScanStop(ctx, msg.Chat.ID, "telegram_read_stop")
			case "start", "on", "1":
				return b.handleScanStart(ctx, msg.Chat.ID, "telegram_read_start")
			}
		}
		return b.handleScanStart(ctx, msg.Chat.ID, "telegram_read_start")

	case "/stop":
		return b.handleScanStop(ctx, msg.Chat.ID, "telegram_stop")

	case "/status":
		b.addChat(msg.Chat.ID)
		text := b.svc.StatusText()
		if b.scanner != nil {
			text += "\n\nReader:\n" + b.scanner.StatusText()
		}
		return b.sendMessage(ctx, msg.Chat.ID, text)

	case "/cache":
		return b.handleCacheDump(ctx, msg.Chat.ID)

	case "/range20":
		return b.handleRange20(ctx, msg.Chat.ID, args)

	case "/range20_on", "range20_on":
		return b.handleRange20(ctx, msg.Chat.ID, []string{"on"})

	case "/range20_off", "range20_off":
		return b.handleRange20(ctx, msg.Chat.ID, []string{"off"})

	case "/range20_status", "range20_status":
		return b.handleRange20(ctx, msg.Chat.ID, []string{"status"})

	case "/turbo":
		b.addChat(msg.Chat.ID)
		if err := b.sendMessage(ctx, msg.Chat.ID, "🚀 Turbo rejim: ERPNext dan cache yangilanmoqda..."); err != nil {
			return err
		}
		if err := b.svc.RefreshCache(ctx, "telegram_turbo", false); err != nil {
			return b.sendMessage(ctx, msg.Chat.ID, "❌ Turbo xato: "+err.Error())
		}
		return b.sendMessage(ctx, msg.Chat.ID, "✅ Turbo tayyor: cache yangilandi.")

	case "/test":
		b.addChat(msg.Chat.ID)
		b.testMode.RequestFile(msg.Chat.ID)
		if msg.Document != nil {
			return b.handleTestFileUpload(ctx, msg.Chat.ID, *msg.Document)
		}
		return b.sendMessage(ctx, msg.Chat.ID, "🧪 Test rejimi yoqildi. EPC ro'yxati bor .txt fayl yuboring.")

	case "/test_stop", "test_stop":
		return b.handleTestStop(ctx, msg.Chat.ID)
	}

	if msg.Document != nil && b.testMode.IsAwaitingFile(msg.Chat.ID) {
		return b.handleTestFileUpload(ctx, msg.Chat.ID, *msg.Document)
	}

	return nil
}

func (b *Bot) handleScanStart(ctx context.Context, chatID int64, reason string) error {
	b.addChat(chatID)
	if err := b.svc.RefreshCache(ctx, reason, false); err != nil {
		_ = b.sendMessage(ctx, chatID, "⚠️ Ogohlantirish: cache refresh xato: "+err.Error())
	}
	replay := b.svc.SetScanActive(true, reason)
	if b.scanner != nil {
		if err := b.scanner.Start(ctx); err != nil {
			return b.sendMessage(ctx, chatID, fmt.Sprintf("❌ Scan active, lekin reader start xato: %v", err))
		}
	}
	return b.sendMessage(ctx, chatID, fmt.Sprintf("✅ Scan boshlandi. Replay navbati: %d", replay))
}

func (b *Bot) handleScanStop(ctx context.Context, chatID int64, reason string) error {
	b.addChat(chatID)
	if b.scanner != nil {
		b.scanner.Stop()
	}
	b.svc.SetScanActive(false, reason)
	return b.sendMessage(ctx, chatID, "⏹️ Scan to'xtatildi.")
}

func (b *Bot) handleRange20(ctx context.Context, chatID int64, args []string) error {
	b.addChat(chatID)
	if b.scanner == nil {
		return b.sendMessage(ctx, chatID, "⚠️ Reader manager topilmadi. /range20 faqat SDK scanner bilan ishlaydi.")
	}

	tuner, ok := b.scanner.(RangeTuner)
	if !ok {
		return b.sendMessage(ctx, chatID, "⚠️ Reader buyrug'i qo'llab-quvvatlanmaydi: /range20 mavjud emas.")
	}

	action := "status"
	if len(args) > 0 {
		action = strings.ToLower(strings.TrimSpace(args[0]))
	}

	switch action {
	case "status", "st":
		state := "off"
		if tuner.LongRangeMode() {
			state = "on"
		}
		return b.sendMessage(ctx, chatID, "📡 range20 holati: "+state+"\n\nReader:\n"+b.scanner.StatusText())

	case "on", "1", "start", "enable":
		summary := tuner.SetLongRangeMode(true)
		if b.svc.ScanActive() {
			b.scanner.Stop()
			if err := b.scanner.Start(ctx); err != nil {
				return b.sendMessage(ctx, chatID, summary+"\n❌ Reader restart xato: "+err.Error())
			}
			return b.sendMessage(ctx, chatID, "✅ "+summary+"\nQo'llandi va reader qayta ishga tushirildi.")
		}
		return b.sendMessage(ctx, chatID, "✅ "+summary+"\nQo'llandi. Real ta'sir /scan bilan ishga tushganda ko'rinadi.")

	case "off", "0", "stop", "disable":
		summary := tuner.SetLongRangeMode(false)
		if b.svc.ScanActive() {
			b.scanner.Stop()
			if err := b.scanner.Start(ctx); err != nil {
				return b.sendMessage(ctx, chatID, summary+"\n❌ Reader restart xato: "+err.Error())
			}
			return b.sendMessage(ctx, chatID, "✅ "+summary+"\nQo'llandi va reader qayta ishga tushirildi.")
		}
		return b.sendMessage(ctx, chatID, "✅ "+summary)
	}

	return b.sendMessage(ctx, chatID, "ℹ️ Foydalanish: /range20 on | /range20 off | /range20 status")
}

func (b *Bot) getUpdates(ctx context.Context, offset int64) ([]update, error) {
	values := url.Values{}
	values.Set("timeout", strconv.Itoa(int(b.pollTimeout/time.Second)))
	values.Set("offset", strconv.FormatInt(offset, 10))

	reqURL := b.baseURL + "/getUpdates?" + values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("telegram getUpdates HTTP %d", resp.StatusCode)
	}

	var env updatesEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		return nil, fmt.Errorf("telegram getUpdates not ok")
	}
	return env.Result, nil
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string) error {
	_, err := b.sendMessageWithID(ctx, chatID, text)
	return err
}

func (b *Bot) addChat(chatID int64) {
	if chatID == 0 {
		return
	}

	b.mu.Lock()
	if _, ok := b.chats[chatID]; ok {
		b.mu.Unlock()
		return
	}
	b.chats[chatID] = struct{}{}
	snapshot := make([]int64, 0, len(b.chats))
	for id := range b.chats {
		snapshot = append(snapshot, id)
	}
	b.mu.Unlock()

	b.persistChats(snapshot)
}

func (b *Bot) snapshotChats() []int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]int64, 0, len(b.chats))
	for chatID := range b.chats {
		out = append(out, chatID)
	}
	return out
}

func parseCommand(text string) (string, []string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	parts := strings.Fields(text)
	cmd := strings.ToLower(parts[0])
	if idx := strings.IndexByte(cmd, '@'); idx > 0 {
		cmd = cmd[:idx]
	}
	if len(parts) == 1 {
		return cmd, nil
	}
	args := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		args = append(args, part)
	}
	return cmd, args
}

func (b *Bot) sendMessageWithRetry(chatID int64, text string) error {
	var lastErr error
	for attempt := 0; attempt <= b.notifyRetry; attempt++ {
		if err := b.sendMessage(context.Background(), chatID, text); err != nil {
			lastErr = err
			if attempt < b.notifyRetry {
				time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
			}
			continue
		}
		return nil
	}
	return lastErr
}

func (b *Bot) loadChats() {
	if strings.TrimSpace(b.chatsFile) == "" {
		return
	}

	data, err := os.ReadFile(b.chatsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[bot] chat store read failed: %v", err)
		}
		return
	}

	var ids []int64
	if err := json.Unmarshal(data, &ids); err != nil {
		log.Printf("[bot] chat store decode failed: %v", err)
		return
	}

	b.mu.Lock()
	for _, id := range ids {
		if id != 0 {
			b.chats[id] = struct{}{}
		}
	}
	count := len(b.chats)
	b.mu.Unlock()

	if count > 0 {
		log.Printf("[bot] loaded %d chat(s) from store", count)
	}
}

func (b *Bot) persistChats(ids []int64) {
	if strings.TrimSpace(b.chatsFile) == "" {
		return
	}

	dir := filepath.Dir(b.chatsFile)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("[bot] chat store mkdir failed: %v", err)
			return
		}
	}

	data, err := json.Marshal(ids)
	if err != nil {
		log.Printf("[bot] chat store encode failed: %v", err)
		return
	}

	tmp := b.chatsFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("[bot] chat store write failed: %v", err)
		return
	}
	if err := os.Rename(tmp, b.chatsFile); err != nil {
		log.Printf("[bot] chat store rename failed: %v", err)
		return
	}
}

type updatesEnvelope struct {
	OK     bool     `json:"ok"`
	Result []update `json:"result"`
}

type update struct {
	UpdateID int64   `json:"update_id"`
	Message  message `json:"message"`
}

type message struct {
	Text     string    `json:"text"`
	Caption  string    `json:"caption"`
	Chat     chat      `json:"chat"`
	Document *document `json:"document"`
}

type chat struct {
	ID int64 `json:"id"`
}

type document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
}

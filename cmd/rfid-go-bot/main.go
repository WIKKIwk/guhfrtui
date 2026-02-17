package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"new_era_go/internal/gobot/cache"
	"new_era_go/internal/gobot/config"
	"new_era_go/internal/gobot/erp"
	"new_era_go/internal/gobot/httpapi"
	"new_era_go/internal/gobot/ipc"
	"new_era_go/internal/gobot/reader"
	"new_era_go/internal/gobot/service"
	"new_era_go/internal/gobot/telegram"
	"new_era_go/internal/tui"
)

func main() {
	envFile := os.Getenv("BOT_ENV_FILE")
	if envFile == "" {
		envFile = ".env"
	}
	if err := config.LoadDotEnv(envFile); err != nil {
		log.Fatalf("env load failed: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	showTUI := shouldShowTUI()
	closeLog := func() {}
	if showTUI {
		closeLog = redirectBotLogs()
		defer closeLog()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	erpClient := erp.New(cfg.ERPURL, cfg.ERPAPIKey, cfg.ERPAPISecret, cfg.RequestTimeout)
	cacheStore := cache.New()
	svc := service.New(cfg, erpClient, cacheStore)

	backend := strings.ToLower(cfg.ScanBackend)
	useSDKScanner := backend == "sdk" || backend == "hybrid"

	// Keep scanner manager available for Telegram/HTTP commands regardless of ingest backend.
	// In ingest mode we still avoid wiring scanner into IPC start/stop flow to prevent duplicate readers.
	scanner := reader.New(cfg, func(epc string) {
		svc.HandleEPC(context.Background(), epc, "sdk")
	}, nil)

	tg := telegram.New(cfg.BotToken, cfg.RequestTimeout, cfg.PollTimeout, svc, scanner)
	svc.SetNotifier(tg)
	scanner.SetNotifier(tg.Notify)

	if err := svc.Bootstrap(ctx); err != nil {
		log.Printf("[bot] startup cache refresh failed: %v", err)
	} else {
		tg.Notify("Bot ishga tushdi. Cache ERPNext'dan yuklandi.")
	}

	if cfg.ScanDefaultActive {
		replay := svc.SetScanActive(true, "startup_default_active")
		if replay > 0 {
			log.Printf("[bot] startup replay queued: %d", replay)
		}
	}

	svc.Run(ctx)
	go tg.Run(ctx)

	if cfg.AutoScan && useSDKScanner {
		if err := scanner.Start(ctx); err != nil {
			log.Printf("[bot] auto scan start failed: %v", err)
		} else {
			svc.SetScanActive(true, "auto_scan")
			tg.Notify("Auto scan boshlandi.")
		}
	}

	var ipcScanner ipc.Scanner
	if useSDKScanner {
		ipcScanner = scanner
	}
	if cfg.IPCEnabled && cfg.IPCSocket != "" {
		ipcServer := ipc.New(cfg.IPCSocket, svc, ipcScanner)
		go func() {
			if err := ipcServer.Run(ctx); err != nil {
				log.Printf("[bot] ipc server failed: %v", err)
				stop()
			}
		}()
	}

	if cfg.HTTPEnabled && strings.TrimSpace(cfg.HTTPAddr) != "" {
		httpServer := httpapi.New(cfg.HTTPAddr, cfg.WebhookSecret, svc, scanner)
		go func() {
			if err := httpServer.Run(ctx); err != nil {
				log.Printf("[bot] http server failed: %v", err)
				stop()
			}
		}()
	}

	if showTUI {
		_ = os.Setenv("BOT_AUTOSTART", "0")
		if err := tui.Run(); err != nil {
			log.Printf("[bot] tui error: %v", err)
		}
		stop()
	}

	<-ctx.Done()
}

func shouldShowTUI() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("BOT_SHOW_TUI")))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}

	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func redirectBotLogs() func() {
	logDir := strings.TrimSpace(os.Getenv("BOT_LOG_DIR"))
	if logDir == "" {
		logDir = "logs"
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return func() {}
	}

	path := filepath.Join(logDir, "rfid-go-bot.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return func() {}
	}
	log.SetOutput(f)
	return func() {
		_ = f.Close()
	}
}

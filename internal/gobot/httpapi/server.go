package httpapi

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"new_era_go/internal/gobot/service"
)

type Server struct {
	addr          string
	webhookSecret string
	svc           *service.Service
	scanner       Scanner
	http          *http.Server
}

type Scanner interface {
	Start(ctx context.Context) error
	Stop()
	StatusText() string
}

func New(addr, webhookSecret string, svc *service.Service, scanner Scanner) *Server {
	mux := http.NewServeMux()
	s := &Server{
		addr:          addr,
		webhookSecret: strings.TrimSpace(webhookSecret),
		svc:           svc,
		scanner:       scanner,
		http: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/ingest", s.handleIngest)
	mux.HandleFunc("/webhook/draft", s.handleWebhookDraft)
	mux.HandleFunc("/api/webhook/erp", s.handleLegacyERPWebhook)
	mux.HandleFunc("/turbo", s.handleTurbo)
	mux.HandleFunc("/scan/start", s.handleScanStart)
	mux.HandleFunc("/scan/stop", s.handleScanStop)
	return s
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		log.Printf("[bot] http listening on %s", s.addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "rfid-go-bot",
	})
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Status())
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	var payload epcPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}

	epcs := payload.EPCs
	if payload.EPC != "" {
		epcs = append(epcs, payload.EPC)
	}

	results := make([]service.IngestResult, 0, len(epcs))
	for _, epc := range epcs {
		results = append(results, s.svc.HandleEPC(r.Context(), epc, payload.Source))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"results": results,
		"stats":   s.svc.Status(),
	})
}

func (s *Server) handleWebhookDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	if s.webhookSecret != "" && strings.TrimSpace(r.Header.Get("X-Webhook-Secret")) != s.webhookSecret {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "invalid webhook secret"})
		return
	}

	var payload epcPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}

	epcs := payload.EPCs
	if payload.EPC != "" {
		epcs = append(epcs, payload.EPC)
	}
	added, replay := s.svc.AddDraftEPCs(r.Context(), epcs)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"added_to_cache": added,
		"replayed_seen":  replay,
		"stats":          s.svc.Status(),
	})
}

func (s *Server) handleLegacyERPWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	var payload struct {
		Doctype string `json:"doctype"`
		Name    string `json:"name"`
		Event   string `json:"event"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}

	if err := s.svc.RefreshCache(r.Context(), "erp_webhook", false); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"legacy":  true,
		"doctype": payload.Doctype,
		"name":    payload.Name,
		"event":   payload.Event,
		"stats":   s.svc.Status(),
	})
}

func (s *Server) handleTurbo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if err := s.svc.RefreshCache(r.Context(), "http_turbo", false); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stats": s.svc.Status()})
}

func (s *Server) handleScanStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}

	if err := s.svc.RefreshCache(r.Context(), "http_scan_start", false); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	replay := s.svc.SetScanActive(true, "http_scan_start")
	if s.scanner != nil {
		if err := s.scanner.Start(r.Context()); err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":             true,
				"warning":        "scan active, reader start failed: " + err.Error(),
				"replayed_seen":  replay,
				"reader_running": false,
				"stats":          s.svc.Status(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"replayed_seen": replay,
		"stats":         s.svc.Status(),
	})
}

func (s *Server) handleScanStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "method not allowed"})
		return
	}
	if s.scanner != nil {
		s.scanner.Stop()
	}
	s.svc.SetScanActive(false, "http_scan_stop")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "stats": s.svc.Status()})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

type epcPayload struct {
	EPC    string   `json:"epc"`
	EPCs   []string `json:"epcs"`
	Source string   `json:"source"`
}

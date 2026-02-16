package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"new_era_go/internal/gobot/service"
)

type Scanner interface {
	Start(ctx context.Context) error
	Stop()
}

type Server struct {
	socketPath string
	svc        *service.Service
	scanner    Scanner
}

func New(socketPath string, svc *service.Service, scanner Scanner) *Server {
	return &Server{
		socketPath: strings.TrimSpace(socketPath),
		svc:        svc,
		scanner:    scanner,
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s.socketPath == "" || s.svc == nil {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.socketPath), 0o755); err != nil {
		return err
	}
	_ = os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(s.socketPath)
	}()
	_ = os.Chmod(s.socketPath, 0o666)
	log.Printf("[bot] ipc listening on %s", s.socketPath)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return err
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	enc := json.NewEncoder(conn)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{OK: false, Error: "invalid json"})
			continue
		}

		resp := s.handleRequest(ctx, req)
		_ = enc.Encode(resp)
	}
}

func (s *Server) handleRequest(ctx context.Context, req request) response {
	typ := strings.ToLower(strings.TrimSpace(req.Type))
	source := strings.TrimSpace(req.Source)
	if source == "" {
		source = "ipc"
	}

	switch typ {
	case "status":
		return response{OK: true, Action: "status", Stats: s.svc.Status()}

	case "scan_start":
		if err := s.svc.RefreshCache(ctx, "ipc_scan_start", false); err != nil {
			return response{OK: false, Action: "scan_start", Error: err.Error(), Stats: s.svc.Status()}
		}
		replay := s.svc.SetScanActive(true, "ipc_scan_start")
		warn := ""
		if s.scanner != nil {
			if err := s.scanner.Start(ctx); err != nil {
				warn = "scan active, reader start failed: " + err.Error()
			}
		}
		return response{
			OK:      true,
			Action:  "scan_start",
			Replay:  replay,
			Warning: warn,
			Stats:   s.svc.Status(),
		}

	case "scan_stop":
		if s.scanner != nil {
			s.scanner.Stop()
		}
		s.svc.SetScanActive(false, "ipc_scan_stop")
		return response{OK: true, Action: "scan_stop", Stats: s.svc.Status()}

	case "turbo":
		if err := s.svc.RefreshCache(ctx, "ipc_turbo", false); err != nil {
			return response{OK: false, Action: "turbo", Error: err.Error(), Stats: s.svc.Status()}
		}
		return response{OK: true, Action: "turbo", Stats: s.svc.Status()}

	case "epc":
		res := s.svc.HandleEPC(ctx, req.EPC, source)
		return response{OK: true, Action: "epc", Results: []service.IngestResult{res}, Stats: s.svc.Status()}

	case "epcs":
		results := make([]service.IngestResult, 0, len(req.EPCs))
		for _, epc := range req.EPCs {
			results = append(results, s.svc.HandleEPC(ctx, epc, source))
		}
		return response{OK: true, Action: "epcs", Results: results, Stats: s.svc.Status()}

	case "draft_epc":
		added, replay := s.svc.AddDraftEPCs(ctx, []string{req.EPC})
		return response{OK: true, Action: "draft_epc", Added: added, Replay: replay, Stats: s.svc.Status()}

	case "draft_epcs":
		added, replay := s.svc.AddDraftEPCs(ctx, req.EPCs)
		return response{OK: true, Action: "draft_epcs", Added: added, Replay: replay, Stats: s.svc.Status()}
	}

	return response{
		OK:    false,
		Error: fmt.Sprintf("unsupported type: %s", req.Type),
		Stats: s.svc.Status(),
	}
}

type request struct {
	Type   string   `json:"type"`
	Source string   `json:"source,omitempty"`
	EPC    string   `json:"epc,omitempty"`
	EPCs   []string `json:"epcs,omitempty"`
}

type response struct {
	OK      bool                   `json:"ok"`
	Action  string                 `json:"action,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Warning string                 `json:"warning,omitempty"`
	Replay  int                    `json:"replayed_seen,omitempty"`
	Added   int                    `json:"added_to_cache,omitempty"`
	Results []service.IngestResult `json:"results,omitempty"`
	Stats   service.Stats          `json:"stats"`
}

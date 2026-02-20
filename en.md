# New Era Go: ST-8508 RFID Reader Platform (English Edition)

Comprehensive technical documentation for academic and applied-lab use.

## Abstract
This project is a Go-based platform for controlling ST-8508 class LAN UHF RFID readers, processing EPC streams in near real time, and synchronizing with ERPNext workflows. The system is built as a two-layer runtime: a terminal operator interface (`st8508-tui`) and a background service (`rfid-go-bot`). Its architecture integrates reader protocol handling, LAN discovery, EPC deduplication, worker queues, retry logic, Telegram operations, and HTTP/IPC integration.

This document is written in an academic-practical format and covers problem framing, architecture, algorithms, module-level analysis, interface contracts, deployment, diagnostics, and extension directions.

## Keywords
RFID, UHF, ST-8508, Reader18, EPC, ERPNext, Go, Bubble Tea TUI, IPC, webhook, Telegram bot, worker queue.

## 1. Problem Statement and Objectives
### Problem
In warehouse/logistics environments, processing RFID EPC streams reliably requires:
1. Low-latency ingestion.
2. Duplicate handling and control.
3. Strict processing only for ERP draft EPCs.
4. Reliable submit/error behavior.
5. Practical operator-facing control tooling.

### Objectives
1. Auto-discover and connect readers in LAN.
2. Implement reliable Reader18 frame build/parse behavior.
3. Build an ERPNext draft-cache-driven submit pipeline.
4. Provide one operational platform via TUI + Telegram + HTTP + IPC.
5. Run natively on Linux (Ubuntu/Arch) without Docker.

## 2. High-Level Architecture

```text
+-----------------------+      unix socket      +---------------------------+
| st8508-tui            |  <------------------>  | rfid-go-bot               |
| - Bubble Tea UI       |   /tmp/rfid-go-bot.sock| - EPC service core        |
| - discovery/connect   |                        | - ERPNext client          |
| - inventory tuning    |                        | - Telegram bot            |
+-----------+-----------+                        | - HTTP API + IPC server   |
            |                                    +-----------+---------------+
            | TCP (Reader18)                                 |
            v                                                |
+-----------------------+                                    |
| ST-8508 Reader        |                                    |
| ports: 2022/27011/... |                                    |
+-----------------------+                                    |
                                                             v
                                            +-------------------------------+
                                            | ERPNext + Telegram users      |
                                            +-------------------------------+
```

## 3. End-to-End Runtime Flow
### 3.1 TUI startup flow
1. `cmd/st8508-tui/main.go` loads `.env` (`BOT_ENV_FILE`, default `.env`).
2. Sidecar (`cmd/st8508-tui/sidecar.go`) probes IPC socket (`BOT_IPC_SOCKET`).
3. If bot is not running, sidecar auto-starts it (`BOT_AUTOSTART=1`).
4. TUI starts initial LAN scan via `runScanCmd`.
5. TUI and bot exchange events over IPC: `status`, `scan_start`, `scan_stop`, `epc`.

### 3.2 EPC submit flow
1. Reader produces EPC.
2. TUI (or SDK reader manager) passes EPC to service.
3. Service normalizes EPC (`A-F0-9` only).
4. If `scan_active=false`, EPC is counted as `scan_inactive`.
5. Cache hit -> queue (`queued`); cache miss -> `miss`.
6. Worker submits to ERP `submit_open_stock_entry_by_epc`.
7. On `submitted` or `not_found`, EPC is removed from cache.

### 3.3 Replay flow
Service keeps `recentSeen` EPCs inside a TTL window. When scan is re-enabled or new drafts arrive, `recentSeen ∩ cache` EPCs are replay-enqueued.

## 4. Deep Module Analysis
## 4.1 `internal/discovery`
Purpose: discover likely reader endpoints in LAN.

Core behavior:
1. Collect active IPv4 interface prefixes (minimum `/24` scan window).
2. Add `/proc/net/arp` neighbors.
3. Add static seed IP candidates.
4. Probe host/port combinations in parallel.
5. Verify Reader18 handshake via:
   1. `GetReaderInfo` (0x21),
   2. `Inventory G2` (0x01),
   3. legacy inventory probe.
6. Verified endpoints receive higher score and are preferred.

Default scan parameters:
- Ports: `2022, 27011, 6000, 4001, 10001, 5000`
- Timeout: `180ms`
- Concurrency: `96`
- Host limit per interface: `254`

## 4.2 `internal/protocol/reader18`
Purpose: Reader18 frame encoding/decoding.

Frame format:

```text
Len(1) Adr(1) Cmd(1) Status(1) Data(N) CRC_L(1) CRC_H(1)
```

CRC: `CRC16-MCRF4XX`.

Implemented commands include:
- `0x01` Inventory
- `0x0F` Single Inventory
- `0x21` GetReaderInfo
- `0x22` SetRegion
- `0x25` SetScanTime
- `0x2F` SetOutputPower
- `0x33` Acousto-Optic
- `0x35` SetWorkMode
- `0x36` GetWorkMode
- `0x3F` SetAntennaMux

Status codes:
- `0x00` success
- `0x01` no tag
- `0xFB` no tag/timeout
- `0xF8` antenna error
- `0xFE` command error
- `0xFF` crc/parameter error

## 4.3 `internal/reader`
Purpose: low-level TCP session management.

Capabilities:
1. `Connect`/`Disconnect`.
2. Async packet and error channels.
3. `SendRaw` for protocol-level commands.

## 4.4 `sdk`
Public SDK structure:
1. `client_scan.go`: discovery and quick-connect.
2. `client_connection.go`: connect/reconnect/probe.
3. `client_inventory_control.go`: apply config + start/stop inventory.
4. `client_inventory_runtime.go`: rx/tx loops, frame parsing, tag events.

Important cycle formula:

```text
effective_cycle = max(PollInterval, ScanTime*100ms, 40ms)
```

Additional behavior:
1. Optional periodic single-inventory fallback (`SingleFallbackEach`).
2. No-tag A/B target switching support.

## 4.5 `internal/tui`
TUI pages:
1. Home
2. Devices
3. Control
4. Inventory Tune
5. Regions
6. Logs
7. Help

Core logic:
1. Startup scan and candidate management.
2. Connect-plan with fallback ports and retries.
3. Scheduled inventory command flow (`inventory-g2` + periodic `inventory-single`).
4. Frame parsing and unique EPC counting.
5. IPC synchronization with bot service.

## 4.6 `internal/gobot/service`
Primary business-core module.

Data structures:
1. `cache.Store`: draft EPC hash-map.
2. `recentSeen`: TTL-based seen EPC set.
3. `queued` + `inflight`: duplicate submit prevention in concurrent workers.
4. `queue chan string`: EPC worker queue.

Pipeline behavior:
1. `RefreshCache` fetches ERP drafts and performs `cache.Replace`.
2. `HandleEPC` updates hit/miss/inactive stats.
3. `enqueue` increments `queue_dropped` when queue is full.
4. Worker applies retry (`SubmitRetry`, `SubmitRetryDelay`).

## 4.7 `internal/gobot/erp`
Two ERP API endpoints are used:
1. `get_open_stock_entry_drafts_fast` (`epc_only=1`) for draft EPC cache.
2. `submit_open_stock_entry_by_epc` for submit.

Normalization rule:
- EPC is uppercased.
- Non-hex characters are stripped.

## 4.8 `internal/gobot/ipc`
Unix socket JSON-line server.

Supported `type` operations:
1. `status`
2. `scan_start`
3. `scan_stop`
4. `turbo`
5. `epc`
6. `epcs`
7. `draft_epc`
8. `draft_epcs`

## 4.9 `internal/gobot/httpapi`
HTTP endpoints:
1. `GET /health`
2. `GET /stats`
3. `POST /ingest`
4. `POST /webhook/draft`
5. `POST /api/webhook/erp` (legacy)
6. `POST /turbo`
7. `POST /scan/start`
8. `POST /scan/stop`

`/webhook/draft` validates `X-Webhook-Secret` against `BOT_WEBHOOK_SECRET` if configured.

## 4.10 `internal/gobot/telegram`
Long-poll Telegram integration.

Primary commands:
1. `/start`, `/help`
2. `/scan`, `/read`, `/read stop`, `/stop`
3. `/status`
4. `/cache` (sends two txt snapshots)
5. `/range20 on|off|status` and aliases
6. `/turbo`
7. `/test`
8. `/test_stop`

Additional behavior:
1. Startup message send+edit flow (`SendStartupNotice` + `EditNotices`).
2. File delivery via `sendDocument`.
3. Chat cleanup via `deleteMessage`.

## 4.11 `internal/gobot/testmode`
Dedicated module for EPC validation test workflow.

Rules:
1. `/test` clears previous test session state in memory.
2. Bot waits for `.txt` upload.
3. TXT parser:
   1. skips empty and `#` comment lines,
   2. normalizes EPC,
   3. computes unique/duplicate/invalid statistics.
4. Uploaded file message is deleted from chat.
5. `/test` prompt is edited into "File accepted" message.
6. Each matching EPC emits only one `Read` notification.
7. `/test_stop` command message is deleted.
8. Live `Read` messages are removed and final results are produced by editing the prompt message.

## 5. Algorithmic Decisions and Complexity
## 5.1 Discovery
If `H` hosts and `P` ports are probed, discovery complexity is approximately `O(H*P)`. Parallel workers reduce wall-clock latency.

## 5.2 Cache lookup
Draft EPC checks are average `O(1)` via hash-map lookup.

## 5.3 Queue deduplication
`queued` and `inflight` sets block duplicate concurrent processing for the same EPC.

## 5.4 Replay mechanism
The `recentSeen` TTL window recovers missed EPCs when scan state or draft cache changes.

## 6. Full Configuration Reference
Note: `internal/gobot/config.Load()` requires these fields:
1. `BOT_TOKEN`
2. `ERP_URL`
3. `ERP_API_KEY`
4. `ERP_API_SECRET`

## 6.1 Core service (`rfid-go-bot`)
| Variable | Default | Description |
|---|---:|---|
| `BOT_HTTP_ENABLED` | `1` | Enable/disable HTTP server |
| `BOT_HTTP_ADDR` | `:8098` | HTTP listen address |
| `BOT_IPC_ENABLED` | `1` | Enable/disable IPC socket server |
| `BOT_IPC_SOCKET` | `/tmp/rfid-go-bot.sock` | IPC socket path |
| `BOT_HTTP_TIMEOUT_MS` | `12000` | HTTP/ERP timeout |
| `BOT_CACHE_REFRESH_SEC` | `5` | Periodic cache refresh (min 5s) |
| `BOT_SUBMIT_RETRY` | `2` | Submit retry count |
| `BOT_SUBMIT_RETRY_MS` | `300` | Retry delay |
| `BOT_WORKER_COUNT` | `4` | Worker count (min 1) |
| `BOT_QUEUE_SIZE` | `2048` | Queue capacity (min 64) |
| `BOT_RECENT_SEEN_TTL_SEC` | `600` | recentSeen TTL (min 30s) |
| `BOT_POLL_TIMEOUT_SEC` | `25` | Telegram poll timeout (clamped 5..55s) |
| `BOT_SCAN_BACKEND` | `hybrid` | `ingest|sdk|hybrid` |
| `BOT_SCAN_DEFAULT_ACTIVE` | `1` | Startup scan state |
| `BOT_AUTO_SCAN` | `0` | SDK auto-scan loop |
| `BOT_READER_HOST` | `` | Fixed reader host |
| `BOT_READER_PORT` | `0` | Fixed reader port |
| `BOT_READER_CONNECT_TIMEOUT_SEC` | `25` | Reader connect timeout (min 5s) |
| `BOT_READER_RETRY_SEC` | `2` | Reconnect delay (min 500ms) |
| `BOT_WEBHOOK_SECRET` | `` | Secret for `/webhook/draft` |
| `BOT_CHAT_STORE_FILE` | `logs/telegram_chats.json` | Telegram chat registry |
| `BOT_CACHE_DUMP_DIR` | `BOT_LOG_DIR` or `logs` | Output dir for `/cache` files |
| `BOT_LOG_DIR` | `logs` | Log directory |
| `BOT_SHOW_TUI` | `auto` | Show TUI in bot binary (`0/1`) |

## 6.2 TUI sidecar and bot-sync
| Variable | Default | Description |
|---|---:|---|
| `BOT_AUTOSTART` | `1` | Auto-start bot from TUI |
| `BOT_AUTOSTART_CMD` | `` | Custom bot start command |
| `BOT_AUTOSTART_ROOT` | `` | Override bot root search |
| `BOT_ENV_FILE` | `.env` | Path to env file |
| `BOT_SYNC_ENABLED` | `1` | TUI->bot IPC sync toggle |
| `BOT_SYNC_SOCKET` | `BOT_IPC_SOCKET` | Sync socket path |
| `BOT_SYNC_TIMEOUT_MS` | `1200` | Sync timeout (min 200ms) |
| `BOT_SYNC_QUEUE_SIZE` | `4096` | EPC sync buffer (min 128) |
| `BOT_SYNC_SOURCE` | `st8508-tui` | Source label |
| `BOT_SYNC_MODE` | n/a | Not used by current code path |

## 7. Linux-Native Deployment (No Docker)
## 7.1 Requirements
1. Linux (Ubuntu or Arch).
2. Go 1.25+.
3. LAN access to reader device.
4. ERP and Telegram credentials.

## 7.2 Install dependencies
```bash
# Ubuntu
sudo apt update
sudo apt install -y golang-go build-essential ca-certificates git

# Arch
sudo pacman -S --needed go base-devel ca-certificates git
```

## 7.3 Quick start
```bash
git clone <repo_url>
cd new_era_gos
cp .env.bot.example .env
# fill .env values
make run
```

## 7.4 Run options
```bash
# TUI + bot sidecar
make run

# bot only
make bot

# discovery diagnostics
make scan

# build binaries
make build

# formatting + tests
make check
```

## 7.5 Systemd
Service file: `deploy/systemd/rfid-go-bot.service`

Default model:
- WorkingDirectory: `/opt/new_era_gos`
- ExecStart: `/opt/new_era_gos/bin/rfid-go-bot`

## 8. IPC Contract
Socket: `/tmp/rfid-go-bot.sock` (default).

Request examples:
```json
{"type":"status","source":"st8508-tui"}
{"type":"scan_start","source":"st8508-tui"}
{"type":"scan_stop","source":"st8508-tui"}
{"type":"epc","epc":"E200...","source":"st8508-tui"}
{"type":"draft_epcs","epcs":["E200..."],"source":"erp"}
```

Generic response:
```json
{
  "ok": true,
  "action": "scan_start",
  "replayed_seen": 2,
  "warning": "",
  "stats": {"cache_size": 120, "scan_active": true}
}
```

## 9. HTTP API Contract
## 9.1 Health and stats
```bash
curl -s http://127.0.0.1:8098/health
curl -s http://127.0.0.1:8098/stats
```

## 9.2 EPC ingest
```bash
curl -s -X POST http://127.0.0.1:8098/ingest \
  -H 'Content-Type: application/json' \
  -d '{"epcs":["E200001122334455"],"source":"lab"}'
```

## 9.3 Draft webhook
```bash
curl -s -X POST http://127.0.0.1:8098/webhook/draft \
  -H 'Content-Type: application/json' \
  -H 'X-Webhook-Secret: change_me' \
  -d '{"epcs":["E200001122334455"],"source":"erp"}'
```

## 9.4 Scan control
```bash
curl -s -X POST http://127.0.0.1:8098/scan/start
curl -s -X POST http://127.0.0.1:8098/scan/stop
```

## 9.5 Turbo refresh
```bash
curl -s -X POST http://127.0.0.1:8098/turbo
```

## 10. Telegram Command Reference
| Command | Purpose |
|---|---|
| `/start`, `/help` | register chat and show help |
| `/scan` | start scan |
| `/read` | alias of `/scan` |
| `/read stop` | alias of `/stop` |
| `/stop` | stop scan |
| `/status` | service + reader status |
| `/cache` | send `cache_draft_epcs.txt` and `cache_seen_epcs.txt` |
| `/range20 on/off/status` | long-range profile control |
| `/range20_on`, `/range20_off`, `/range20_status` | fast aliases |
| `/turbo` | immediate cache refresh |
| `/test` | start EPC test session (wait for txt file) |
| `/test_stop` | stop test and produce summary |

In `/test` flow, command/file messages are cleaned from chat, and results are shown through prompt message edit operations.

## 11. TUI Key Bindings
## 11.1 Global
- `q`: quit
- `m`: Home
- `b` or `0`: back/Home

## 11.2 Home
- `1..7`: open page
- `enter`: open selected item
- `j/k` or `up/down`: navigate

## 11.3 Devices
- `s`: rescan
- `a`: quick-connect
- `enter`: connect selected endpoint

## 11.4 Control
- `enter`: run selected action
- `/`: raw-hex mode

## 11.5 Inventory Tune
- `h/l` or `left/right`: change setting
- `enter`: apply/run action

## 12. Diagnostics and Observability
## 12.1 Logs
- Sidecar bot log: `logs/rfid-go-bot.log`
- TUI built-in log panel under `Logs` page

## 12.2 Discovery diagnostic utility
```bash
go run ./cmd/scancheck
```
It prints:
1. local interfaces,
2. discovery duration and candidate list,
3. `verified`, `protocol`, `score`, and `reason` fields.

## 12.3 Runtime statistics
`service.Stats` includes:
- `cache_size`, `draft_count`
- `seen_total`, `cache_hits`, `cache_misses`, `scan_inactive`
- `submitted_ok`, `submit_not_found`, `submit_errors`, `queue_dropped`
- `last_refresh_at`, `last_refresh_ok`

## 13. Testing and Quality Assurance
The codebase currently contains `45` unit-test functions (`Test*`).

Main coverage areas:
1. Protocol packet build/parse and CRC behavior.
2. Discovery defaults and host generation.
3. TUI update flow and unique tag counting.
4. Service replay logic and snapshot ordering.
5. Telegram command parsing.
6. Testmode parser and session replacement behavior.

Run all tests:
```bash
go test ./...
```

## 14. Troubleshooting Guide
## 14.1 `BOT_TOKEN is required`
`.env` is missing required fields: `BOT_TOKEN`, `ERP_URL`, `ERP_API_KEY`, `ERP_API_SECRET`.

## 14.2 Reader not discovered
1. Run `make scan` to inspect network results.
2. Set `BOT_READER_HOST` and `BOT_READER_PORT` to force direct endpoint.
3. Verify firewall/VLAN/network segmentation.

## 14.3 IPC connection errors
1. Ensure both processes use the same `BOT_IPC_SOCKET`.
2. Remove stale socket file and restart processes.

## 14.4 Webhook returns 401
`X-Webhook-Secret` does not match `BOT_WEBHOOK_SECRET`.

## 15. Known Limitations
1. Discovery is primarily IPv4/LAN segment oriented.
2. Cache and `recentSeen` are process-memory based by default (no persistent DB).
3. Reader traffic is plain TCP (LAN hardening is recommended).
4. Misconfigured environment values can block startup.

## 16. Extension Roadmap
1. Add persistent queue/cache storage (SQLite/PostgreSQL).
2. Add structured metrics (Prometheus/OpenTelemetry).
3. Add multi-reader orchestration.
4. Add HTTPS + mTLS for stronger external API security.
5. Add CSV/Excel export for `/test` output.

## 17. Repository Layout
```text
new_era_gos/
  cmd/
    st8508-tui/
    rfid-go-bot/
    scancheck/
  internal/
    discovery/
    protocol/reader18/
    reader/
    regions/
    tui/
    gobot/
      cache/
      config/
      erp/
      httpapi/
      ipc/
      reader/
      service/
      telegram/
      testmode/
  sdk/
  docs/
  deploy/systemd/
  Makefile
  .env.bot.example
```

## Conclusion
New Era Go is structured to solve practical ST-8508 operational needs in a production-like environment: discovery, inventory, EPC deduplication, ERP submission, Telegram monitoring, and Linux-native deployment. Because responsibilities are separated by layers, the platform is suitable both for institutional lab work and for gradual industrial hardening.

## License
Proprietary. All rights reserved.

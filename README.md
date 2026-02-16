```
 _   _                 _____               ____
| \ | | _____      __ | ____|_ __ __ _   / ___| ___
|  \| |/ _ \ \ /\ / / |  _| | '__/ _` | | |  _ / _ \
| |\  |  __/\ V  V /  | |___| | | (_| | | |_| | (_) |
|_| \_|\___| \_/\_/   |_____|_|  \__,_|  \____|\___/
```

# New Era Go -- ST-8508 RFID Reader Platform

> Industrial-grade Go platform for ST-8508 LAN RFID readers.
> Terminal UI + Background Service + ERP Integration + Telegram Control.

---

```
                          +---------------------------+
                          |      st8508-tui (TUI)     |
                          |  terminal user interface   |
                          |  discovery, control, view  |
                          +------------+--------------+
                                       |
                               IPC (unix socket)
                               /tmp/rfid-go-bot.sock
                                       |
                          +------------+--------------+
                          |    rfid-go-bot (service)   |
                          |  erp sync, telegram, http  |
                          |  cache, queue, workers     |
                          +------------+--------------+
                                       |
                                   TCP/IP
                                       |
                          +------------+--------------+
                          |   ST-8508 RFID Reader(s)   |
                          |   Reader18 protocol over   |
                          |   LAN (ports 2022, 27011)  |
                          +---------------------------+
```

---

## Table of Contents

```
  01 .... Overview
  02 .... Architecture
  03 .... Project Structure
  04 .... Getting Started
  05 .... Configuration Reference
  06 .... Go SDK
  07 .... Protocol Details
  08 .... Key Bindings
  09 .... Docker Deployment
  10 .... Diagnostics
  11 .... License
```

---

## 01. Overview

New Era Go is a modular, production-ready platform for managing ST-8508 style
LAN-based UHF RFID readers. The system is composed of two primary executables
that communicate over IPC:

| Component        | Role                                                 |
|------------------|------------------------------------------------------|
| `st8508-tui`     | Terminal UI for reader discovery, control, inventory  |
| `rfid-go-bot`    | Background service for ERP, Telegram, HTTP webhooks   |

The platform is built for warehouse and logistics inventory workflows where
real-time tag reading, ERP submission (ERPNext), and operator notification
(Telegram) are critical.

**Core capabilities:**

```
  [x] LAN auto-discovery with protocol handshake verification
  [x] Real-time UHF inventory with live tag counting
  [x] ERPNext draft EPC cache and submission pipeline
  [x] Telegram bot with /scan, /stop, /status commands
  [x] IPC bridge between TUI and background service
  [x] HTTP webhook endpoint for external integrations
  [x] High-level Go SDK for programmatic reader control
  [x] Docker-native deployment with host networking
  [x] Raw hex console for protocol debugging
  [x] Multi-region RF support (US/EU/JP/KR/CN + 10 more)
```

---

## 02. Architecture

```
+-----------------------------------------------------------------------+
|                           st8508-tui                                  |
|                                                                       |
|  +------------------+  +----------------+  +----------------------+   |
|  |  discovery/      |  |  tui/          |  |  reader/             |   |
|  |  LAN scanner     |  |  Bubble Tea    |  |  TCP session mgmt    |   |
|  |  port probing    |  |  7 screen pages|  |  packet read loop    |   |
|  |  protocol verify |  |  responsive UI |  |  connection state    |   |
|  +------------------+  +----------------+  +----------------------+   |
|                                                                       |
|  +------------------+  +----------------+                             |
|  |  protocol/       |  |  regions/      |                             |
|  |  reader18        |  |  15 RF regions |                             |
|  |  CRC16-MCRF4XX   |  |  freq catalogs |                             |
|  |  frame parser    |  +----------------+                             |
|  +------------------+                                                 |
+------------------------------+----------------------------------------+
                               |  IPC (JSON-line over unix socket)
+------------------------------+----------------------------------------+
|                          rfid-go-bot                                  |
|                                                                       |
|  +------------------+  +----------------+  +----------------------+   |
|  |  service/        |  |  erp/          |  |  cache/              |   |
|  |  EPC queue       |  |  ERPNext HTTP  |  |  in-memory map       |   |
|  |  worker pool     |  |  draft fetch   |  |  O(1) EPC lookup     |   |
|  |  statistics      |  |  submit API    |  |  thread-safe         |   |
|  +------------------+  +----------------+  +----------------------+   |
|                                                                       |
|  +------------------+  +----------------+  +----------------------+   |
|  |  telegram/       |  |  ipc/          |  |  httpapi/            |   |
|  |  long-poll bot   |  |  unix socket   |  |  webhook server      |   |
|  |  chat registry   |  |  JSON protocol |  |  POST /webhook/draft |   |
|  |  notifications   |  |  bidirectional |  |  secret validation   |   |
|  +------------------+  +----------------+  +----------------------+   |
|                                                                       |
|  +------------------+                                                 |
|  |  reader/manager  |                                                 |
|  |  SDK scanner     |                                                 |
|  |  auto-retry      |                                                 |
|  +------------------+                                                 |
+-----------------------------------------------------------------------+
```

**Data flow:**

```
  Reader --> TCP --> st8508-tui --> IPC --> rfid-go-bot --> ERPNext
                         |                      |
                         |                      +--> Telegram
                         |                      +--> HTTP webhook
                         |
                         +--> Terminal (live tags, stats, logs)
```

---

## 03. Project Structure

```
new_era_gos/
|
|-- cmd/
|   |-- st8508-tui/           entry point: TUI + bot sidecar launcher
|   |   |-- main.go           loads .env, spawns bot, runs TUI
|   |   +-- sidecar.go        bot process lifecycle management
|   |
|   |-- rfid-go-bot/          entry point: background service
|   |   +-- main.go           ERP, Telegram, IPC, HTTP, SDK scanner
|   |
|   +-- scancheck/            diagnostic: LAN discovery dump
|       +-- main.go           prints interfaces, probes readers
|
|-- internal/
|   |-- discovery/             LAN scanning and endpoint detection
|   |   +-- scanner.go         multi-threaded port/protocol prober
|   |
|   |-- protocol/reader18/    ST-8508 wire protocol
|   |   +-- protocol.go        frame builder, parser, CRC16-MCRF4XX
|   |
|   |-- reader/                TCP transport layer
|   |   +-- client.go          connection, read loop, packet channels
|   |
|   |-- regions/               RF frequency catalogs
|   |   +-- regions.go         15 region presets (US/EU/JP/KR/CN/...)
|   |
|   |-- tui/                   Bubble Tea terminal interface
|   |   |-- types.go           state model, message types, screens
|   |   |-- model.go           model initialization
|   |   |-- view.go            page rendering (7 screens)
|   |   |-- update.go          event handling, state transitions
|   |   |-- commands.go        async commands, scan/connect helpers
|   |   |-- bot_sync.go        IPC client to rfid-go-bot
|   |   |-- style.go           terminal styling definitions
|   |   +-- run.go             TUI entry point
|   |
|   +-- gobot/                 rfid-go-bot internals
|       |-- config/            env-based configuration loader
|       |   |-- config.go      config struct, env parsing
|       |   +-- dotenv.go      .env file reader
|       |
|       |-- service/           core EPC processing engine
|       |   +-- service.go     queue, cache check, ERP submit, stats
|       |
|       |-- erp/               ERPNext API client
|       |   +-- client.go      HTTP methods, token auth, EPC normalize
|       |
|       |-- cache/             in-memory EPC store
|       |   +-- store.go       map-backed cache with mutex
|       |
|       |-- reader/            SDK-based reader manager
|       |   +-- manager.go     auto-scanner, retry loop, status
|       |
|       |-- telegram/          Telegram bot integration
|       |   +-- bot.go         polling, commands, chat persistence
|       |
|       |-- ipc/               unix socket server
|       |   +-- server.go      JSON-line protocol handler
|       |
|       +-- httpapi/           HTTP webhook server
|           +-- server.go      REST endpoints, secret validation
|
|-- sdk/                       high-level Go SDK
|   |-- client.go              facade: discover, connect, inventory
|   +-- types.go               public types: Endpoint, TagEvent, Config
|
|-- Dockerfile.dev             development container (golang:1.25)
|-- docker-compose.yml         service definition (host network)
|-- Makefile                   build/run/deploy commands
|-- go.mod                     module: new_era_go (Go 1.25)
+-- .env.bot.example           configuration template
```

---

## 04. Getting Started

### Prerequisites

```
  Go >= 1.25
  Network access to ST-8508 reader(s) on LAN
  ERPNext instance (for bot ERP features)
  Telegram bot token (for bot notifications)
```

### Quick Start

```bash
# clone and enter project
cd new_era_gos

# copy and edit environment config
cp .env.bot.example .env
# edit .env with your credentials

# run TUI (auto-starts bot sidecar in background)
go run ./cmd/st8508-tui
```

### Run Modes

```
+-----------------------+---------------------------------------------+
| Mode                  | Command                                     |
+-----------------------+---------------------------------------------+
| TUI + Bot (default)   | go run ./cmd/st8508-tui                     |
| TUI only (no bot)     | BOT_AUTOSTART=0 go run ./cmd/st8508-tui     |
| Bot only (headless)   | BOT_SHOW_TUI=0 go run ./cmd/rfid-go-bot     |
| Diagnostic scan       | go run ./cmd/scancheck                       |
+-----------------------+---------------------------------------------+
```

### Startup Sequence

```
  1. st8508-tui loads .env configuration
  2. Sidecar checks /tmp/rfid-go-bot.sock for running bot
  3. If no bot found, spawns rfid-go-bot as child process
  4. Bot logs written to logs/rfid-go-bot.log
  5. TUI launches with Bubble Tea framework
  6. Bot initializes: ERP client, cache, Telegram, IPC, HTTP
  7. Both processes ready for operation
```

---

## 05. Configuration Reference

All configuration is done through environment variables. Use `.env` file or
export directly.

### TUI Configuration

```
+---------------------------+-----------------------------------+-----------+
| Variable                  | Description                       | Default   |
+---------------------------+-----------------------------------+-----------+
| BOT_AUTOSTART             | Auto-spawn bot sidecar            | 1         |
| BOT_AUTOSTART_CMD         | Custom bot start command          | (auto)    |
| BOT_AUTOSTART_ROOT        | Override bot root directory        | (auto)    |
| BOT_LOG_DIR               | Bot log output directory          | logs      |
| BOT_ENV_FILE              | Path to .env file                 | .env      |
| BOT_SYNC_ENABLED          | Enable IPC to bot                 | 1         |
| BOT_SYNC_MODE             | IPC protocol                      | ipc       |
| BOT_SYNC_SOCKET           | Unix socket path                  | /tmp/rfid-go-bot.sock |
| BOT_SYNC_TIMEOUT_MS       | IPC timeout in milliseconds       | 1200      |
| BOT_SYNC_QUEUE_SIZE       | IPC send queue buffer             | 4096      |
| BOT_SYNC_SOURCE           | Source identifier for IPC         | st8508-tui|
+---------------------------+-----------------------------------+-----------+
```

### Bot Service Configuration

```
+---------------------------+-----------------------------------+-----------+
| Variable                  | Description                       | Default   |
+---------------------------+-----------------------------------+-----------+
| BOT_TOKEN                 | Telegram bot token                | (required)|
| ERP_URL                   | ERPNext base URL                  | (required)|
| ERP_API_KEY               | ERPNext API key                   | (required)|
| ERP_API_SECRET            | ERPNext API secret                | (required)|
+---------------------------+-----------------------------------+-----------+
| BOT_HTTP_ENABLED          | Enable HTTP webhook server        | 1         |
| BOT_HTTP_ADDR             | HTTP listen address               | :8098     |
| BOT_HTTP_TIMEOUT_MS       | HTTP request timeout              | 12000     |
+---------------------------+-----------------------------------+-----------+
| BOT_IPC_ENABLED           | Enable IPC unix socket server     | 1         |
| BOT_IPC_SOCKET            | IPC socket path                   | /tmp/rfid-go-bot.sock |
+---------------------------+-----------------------------------+-----------+
| BOT_CACHE_REFRESH_SEC     | ERP cache refresh interval        | 5         |
| BOT_SUBMIT_RETRY          | ERP submission retry attempts     | 2         |
| BOT_SUBMIT_RETRY_MS       | Retry delay in milliseconds       | 300       |
| BOT_WORKER_COUNT          | Concurrent EPC processing workers | 4         |
| BOT_QUEUE_SIZE            | EPC ingestion queue capacity      | 2048      |
| BOT_RECENT_SEEN_TTL_SEC   | Duplicate dedup window            | 600       |
+---------------------------+-----------------------------------+-----------+
| BOT_POLL_TIMEOUT_SEC      | Telegram long-poll timeout        | 25        |
| BOT_CHAT_STORE_FILE       | Telegram chat persistence file    | logs/telegram_chats.json |
| BOT_WEBHOOK_SECRET        | HTTP webhook auth secret          | change_me |
+---------------------------+-----------------------------------+-----------+
| BOT_SCAN_BACKEND          | Reader backend: ingest/sdk/hybrid | hybrid    |
| BOT_SCAN_DEFAULT_ACTIVE   | Auto-start scan on boot           | 1         |
| BOT_AUTO_SCAN             | SDK auto-scanner loop             | 0         |
| BOT_READER_HOST           | Fixed reader host (skip discovery)| (empty)   |
| BOT_READER_PORT           | Fixed reader port                 | (empty)   |
| BOT_READER_CONNECT_TIMEOUT_SEC | Reader connection timeout     | 25        |
| BOT_READER_RETRY_SEC      | Reader reconnect delay            | 2         |
+---------------------------+-----------------------------------+-----------+
```

---

## 06. Go SDK

The `sdk/` package provides a high-level programmatic interface for reader
control, suitable for building custom applications on top of the ST-8508
platform.

### API Surface

```
  sdk.NewClient()                           create client instance
  client.Discover(ctx, opts)                scan LAN for readers
  client.QuickConnect(ctx, opts)            discover + connect best match
  client.Connect(ctx, endpoint)             connect to specific reader
  client.Reconnect(ctx)                     reconnect last endpoint
  client.Close()                            disconnect and cleanup

  client.StartInventory(ctx)                begin tag reading loop
  client.StopInventory()                    stop reading loop
  client.ProbeInfo(ctx)                     query reader capabilities
  client.ApplyInventoryConfig(ctx, cfg)     push config to reader
  client.SendRaw(ctx, data)                 send raw protocol frame

  client.Tags()           <-chan TagEvent   receive decoded tag reads
  client.Statuses()       <-chan StatusEvent receive progress messages
  client.Errors()         <-chan error       receive error events
```

### Usage Example

```go
package main

import (
    "context"
    "fmt"
    "time"

    "new_era_go/sdk"
)

func main() {
    ctx := context.Background()
    client := sdk.NewClient()
    defer client.Close()

    // discover and connect to best reader on LAN
    _, err := client.QuickConnect(ctx, sdk.DefaultScanOptions())
    if err != nil {
        panic(err)
    }

    // configure inventory parameters
    cfg := client.InventoryConfig()
    cfg.ScanTime = 1
    cfg.PollInterval = 40 * time.Millisecond
    client.SetInventoryConfig(cfg)

    // start reading tags
    if err := client.StartInventory(ctx); err != nil {
        panic(err)
    }
    defer client.StopInventory()

    // consume tag events for 10 seconds
    timeout := time.After(10 * time.Second)
    for {
        select {
        case tag := <-client.Tags():
            fmt.Printf("epc=%s ant=%d rssi=%d new=%v total=%d\n",
                tag.EPC, tag.Antenna, tag.RSSI, tag.IsNew, tag.UniqueTags)
        case err := <-client.Errors():
            fmt.Println("error:", err)
        case <-timeout:
            return
        }
    }
}
```

### SDK Types

```
  Endpoint          { Host string, Port int }
  ScanOptions       { Ports, Timeout, Concurrency, HostLimit }
  Candidate         { Host, Port, Score, Banner, Verified, Protocol }
  InventoryConfig   { ReaderAddress, QValue, Session, Target,
                      Antenna, ScanTime, PollInterval, OutputPower }
  TagEvent          { When, Source, EPC, Antenna, RSSI,
                      IsNew, Rounds, UniqueTags }
  StatusEvent       { Message string }
  Stats             { Rounds, UniqueTags, TotalReads }
```

---

## 07. Protocol Details

### Reader18 Frame Format

```
  +-------+-------+-------+--------+------...------+-------+-------+
  | Len   | Adr   | Cmd   | Status | Data          | CRC_L | CRC_H |
  | 1byte | 1byte | 1byte | 1byte  | N bytes       | 1byte | 1byte |
  +-------+-------+-------+--------+------...------+-------+-------+

  Len     total frame length excluding the length byte itself
  Adr     reader address (0x00 unicast, 0xFF broadcast)
  Cmd     command code
  Status  response status (0x00 success)
  Data    command-specific payload
  CRC     CRC16-MCRF4XX over [Len..Data]
```

### Command Table

```
  +------+-------------------------+----------------------------------+
  | Code | Command                 | Description                      |
  +------+-------------------------+----------------------------------+
  | 0x01 | Inventory G2            | Multi-tag inventory with params  |
  | 0x0F | Single Tag Inventory    | Read single tag                  |
  | 0x21 | Get Reader Info         | Query reader capabilities        |
  | 0x22 | Set Region              | Configure RF frequency region    |
  | 0x25 | Set Scan Time           | Configure scan duration          |
  | 0x2F | Set Output Power        | Adjust RF output power           |
  | 0x33 | Acousto-Optic           | Beeper/LED control               |
  | 0x35 | Set Work Mode           | Configure reader operation mode  |
  | 0x36 | Get Work Mode           | Query current work mode          |
  | 0x3F | Set Antenna Mux         | Configure antenna multiplexing   |
  +------+-------------------------+----------------------------------+
```

### Status Codes

```
  0x00    Success
  0x01    No Tag Found
  0xFB    No Tag or Timeout
  0xF8    Antenna Error
  0xFE    Command Error
  0xFF    CRC Error
```

### Discovery Ports

```
  2022, 27011, 6000, 4001, 10001, 5000
```

The scanner probes each port on all detected /24 subnets, verifies Reader18
protocol handshake, and scores candidates by verification status.

---

## 08. Key Bindings

### Global

```
  q ............. quit application
  b ............. go back / previous screen
  m ............. return to home menu
  j / down ...... move selection down
  k / up ........ move selection up
```

### Home Screen

```
  1-7 ........... open menu item directly
  enter ......... open selected item
```

### Device List

```
  enter ......... connect to selected reader
  s ............. rescan LAN for readers
  a ............. quick connect (scan + auto-connect best)
```

### Reader Control

```
  1 ............. start reading
  2 ............. stop reading
  3 ............. probe reader info
  4 ............. raw hex command mode
```

### Raw Hex Mode

```
  enter ......... send hex command
  esc ........... cancel / exit raw mode
```

### Event Logs

```
  up/down ....... scroll log entries
  c ............. clear log buffer
```

---

## 09. Docker Deployment

### Build and Run

```bash
# build image and start container
make up

# run TUI inside container
make run

# interactive shell
make shell

# follow logs
make logs

# stop and remove container
make down
```

### Container Details

```
  Base image ........ golang:1.25-bookworm
  Network mode ...... host (required for LAN reader discovery)
  Working dir ....... /workspace/new_era_go
  Volume mounts ..... source code + build cache
  Go caches ......... persisted to .cache/ directory
```

### docker-compose.yml

The service runs with `network_mode: host` to allow direct LAN access for
reader discovery. Source code is bind-mounted for live development.

```
  Environment defaults:
    BOT_ENV_FILE ........ /workspace/new_era_go/.env
    BOT_SHOW_TUI ........ 0 (headless bot mode)
    BOT_AUTOSTART ....... 1
```

---

## 10. Diagnostics

### Network Scan Check

```bash
go run ./cmd/scancheck
```

Prints all local network interfaces, probes default ports across discovered
subnets, and displays candidate readers with scores and verification status.

### IPC Protocol

The TUI and bot communicate over `/tmp/rfid-go-bot.sock` using JSON-line
protocol:

```
  --> {"type":"epc","epc":"E200...","source":"st8508-tui"}
  <-- {"ok":true}

  --> {"type":"scan_start","source":"st8508-tui"}
  <-- {"ok":true}

  --> {"type":"scan_stop","source":"st8508-tui"}
  <-- {"ok":true}

  --> {"type":"draft_epcs","epcs":["E200..."],"source":"st8508-tui"}
  <-- {"ok":true}
```

### Bot Statistics (visible in TUI)

```
  cache_size ......... draft EPCs loaded in memory
  draft_count ........ total drafts on ERP server
  seen_total ......... EPCs received from scanner
  cache_hits ......... matched against draft cache
  cache_misses ....... not found in cache
  submitted_ok ....... successfully submitted to ERP
  submit_errors ...... failed ERP submissions
  queue_dropped ...... queue overflow drops
```

### Telegram Bot Commands

```
  /start ............ register chat for notifications
  /scan ............. start inventory reading
  /stop ............. stop inventory reading
  /status ........... show runtime statistics
  /turbo ............ toggle turbo scan mode
```

---

## 11. License

Proprietary. All rights reserved.

---

```
  new_era_go v1.0
  Go 1.25 | Reader18 Protocol | Bubble Tea TUI
  ERPNext Integration | Telegram Bot | Docker Ready
```

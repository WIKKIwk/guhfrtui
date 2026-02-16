# ST-8508 Go TUI (MVP)

This project is a modular Go terminal app for ST-8508 style LAN RFID readers.

## What is already implemented

- Nokia-style page flow: `Home -> Devices / Control / Regions / Logs / Help`.
- Each menu opens a dedicated page (compact, not one long mixed screen).
- Lightweight rendering: only current page is drawn, logs are throttled/capped.
- Faster startup feel: scanner profile tuned to reduce LAN probe load.
- LAN auto-discovery of possible reader endpoints (no manual IP typing required).
- Discovery default TCP ports include `2022`, `27011`, `6000`, `4001`, `10001`, `5000`.
- Discovery now verifies reader protocol handshake before prioritizing candidates.
- Post-connect probe timeout detects wrong endpoints and auto-disconnects.
- Startup quick-connect flow (scan + auto-connect to best candidate).
- Startup auto-read flow (scan + auto-connect + start reading loop).
- Manual candidate selection and reconnect from Device List.
- Start/Stop reading from Control menu with SDK-style `Inventory_G2` (`0x01`).
- Adaptive frequency window cycling during read when no tag is detected.
- Auto address detection while reading (`0x00` and `0xFF` fallback).
- Reader protocol parser (`Reader18`) with CRC16-MCRF4XX frame handling.
- Tag read counters (`rounds`, `total-tags`) shown live in TUI.
- Region catalog in TUI (US/EU/JP/KR/CN/etc.) for future hardware apply.
- Raw hex command mode for protocol bring-up and reverse engineering.
- Live RX log stream and byte counters from reader TCP socket.
- Clean architecture for future expansion.

## Project structure

- `cmd/st8508-tui/` app entrypoint
- `internal/discovery/` LAN scanner and endpoint scoring
- `internal/protocol/reader18/` command builder, CRC, and frame parser
- `internal/reader/` connection/session and raw packet I/O
- `internal/regions/` region presets
- `internal/tui/` Bubble Tea terminal UI
  - `types.go` app state and message types
  - `model.go` model initialization
  - `update.go` event/update logic
  - `view.go` page rendering
  - `commands.go` async commands + helpers

## Run

```bash
go run ./cmd/st8508-tui
```

`st8508-tui` now auto-starts `rfid-go-bot` in background (IPC socket mode), writes bot logs to `logs/rfid-go-bot.log`, and keeps terminal focused on TUI.

## Docker (recommended for deploy)

Inside `new_era_go/`:

```bash
make run
```

This command:

- builds/starts container with `docker compose`
- opens TUI inside container
- keeps bot + TUI as one runtime on same machine

Useful commands:

```bash
make logs
make shell
make down
```

## Child app -> bot sync

To sync TUI read/start/stop directly with Go bot:

```bash
export BOT_SYNC_MODE=ipc
export BOT_SYNC_SOCKET=/tmp/rfid-go-bot.sock
export BOT_SYNC_ENABLED=1
go run ./cmd/st8508-tui
```

Optional tuning:

- `BOT_SYNC_MODE` (`ipc`, default `ipc`)
- `BOT_SYNC_SOCKET` (default `/tmp/rfid-go-bot.sock`)
- `BOT_SYNC_TIMEOUT_MS` (default `1200`)
- `BOT_SYNC_QUEUE_SIZE` (default `4096`)
- `BOT_SYNC_SOURCE` (default `st8508-tui`)
- `BOT_AUTOSTART` (`1` default, set `0` to disable bot sidecar autostart)
- `BOT_AUTOSTART_CMD` (optional custom bot start command, e.g. `go run ./cmd/rfid-go-bot`)
- `BOT_LOG_DIR` (default `logs`)

Optional diagnostics:

```bash
go run ./cmd/scancheck
```

`cmd/st8508-tui` now loads `.env` automatically (or `BOT_ENV_FILE` path) before startup.

## Go SDK

`sdk/` package provides a high-level API for discovery, connect, and realtime tag stream.

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

	_, err := client.QuickConnect(ctx, sdk.DefaultScanOptions())
	if err != nil {
		panic(err)
	}

	cfg := client.InventoryConfig()
	cfg.ScanTime = 1
	cfg.PollInterval = 40 * time.Millisecond
	client.SetInventoryConfig(cfg)

	if err := client.StartInventory(ctx); err != nil {
		panic(err)
	}
	defer client.StopInventory()

	timeout := time.After(10 * time.Second)
	for {
		select {
		case tag := <-client.Tags():
			fmt.Printf("epc=%s ant=%d new=%v total=%d\n", tag.EPC, tag.Antenna, tag.IsNew, tag.UniqueTags)
		case err := <-client.Errors():
			fmt.Println("sdk error:", err)
		case <-timeout:
			return
		}
	}
}
```

## Key bindings

- Global: `q` quit, `b` back, `m` home, `j/k` or `up/down` move
- Home page: `1..7` direct open item, `enter` open selected
- Device List: verified readers are marked, `enter` connect selected, `s` rescan LAN, `a` quick connect
- Reader Control: `1` start reading, `2` stop reading, `3` probe info, `4` raw hex
- Raw hex mode: `enter` send, `esc` cancel
- Event Logs: `up/down` scroll, `c` clear logs

## Important note

SDK/protocol command map is not bundled in your PDF. This MVP provides auto-discovery, connection, and a raw frame console so we can integrate real hardware commands quickly with your live device feedback.

Next step is capturing/validating extra command frames for:

- reader beeper on/off
- antenna status
- RF power and region apply

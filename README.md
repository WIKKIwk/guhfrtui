# ST-8508 Go TUI (MVP)

This project is a modular Go terminal app for ST-8508 style LAN RFID readers.

## What is already implemented

- Nokia-style page flow: `Home -> Devices / Control / Regions / Logs / Help`.
- Each menu opens a dedicated page (compact, not one long mixed screen).
- Lightweight rendering: only current page is drawn, logs are throttled/capped.
- Faster startup feel: scanner profile tuned to reduce LAN probe load.
- LAN auto-discovery of possible reader endpoints (no manual IP typing required).
- Discovery default TCP ports include `2022`, `6000`, `4001`, `10001`, `5000`.
- Discovery now verifies reader protocol handshake before prioritizing candidates.
- Post-connect probe timeout detects wrong endpoints and auto-disconnects.
- Startup quick-connect flow (scan + auto-connect to best candidate).
- Manual candidate selection and reconnect from Device List.
- Start/Stop reading from Control menu with inventory polling (`0x01`).
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

Optional diagnostics:

```bash
go run ./cmd/scancheck
```

## Key bindings

- Global: `q` quit, `b` back, `m` home, `j/k` or `up/down` move
- Home page: `1..6` direct open item, `enter` open selected
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

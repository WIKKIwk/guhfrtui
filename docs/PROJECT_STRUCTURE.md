# Project Structure

This project is split by runtime layer and feature ownership.

## Top-level

- `cmd/`
  - app entrypoints only (`main.go`, wiring, flags).
- `internal/`
  - application internals by domain.
- `sdk/`
  - public Go SDK for external callers.
- `docs/`
  - developer documentation.

## `internal/` map

- `internal/discovery/`
  - LAN scanning and candidate detection.
- `internal/protocol/reader18/`
  - Reader18 protocol encode/decode/parsing.
- `internal/reader/`
  - low-level TCP transport client.
- `internal/regions/`
  - RF region presets/catalog.
- `internal/gobot/`
  - bot service layer (`cache`, `erp`, `httpapi`, `ipc`, `reader`, `service`, `telegram`).
- `internal/tui/`
  - BubbleTea terminal UI and interaction logic.

## `internal/tui/` layout

- `model.go`, `types.go`, `run.go`
  - model/state definition and startup.
- `commands.go`
  - async tea commands and message adapters.
- `update_main.go`
  - root `Update` loop and top-level message handling.
- `update_key_dispatch.go`
  - key dispatch to active screens.
- `update_home_devices.go`
  - Home/Devices actions.
- `update_control_actions.go`
  - Control actions and action-triggered connect flow.
- `update_inventory_tune.go`
  - inventory tuning rows/presets.
- `update_misc_keys.go`
  - Regions/Logs/Help/Raw-input key handling.
- `update_connect.go`
  - connection plan and retry sequencing.
- `update_frames.go`
  - protocol frame processing.
- `update/helpers.go`
  - pure helper functions reused by update/view flows.
- `view_layout.go`
  - panel composition and layout clipping.
- `view_pages.go`
  - page body builders per screen.
- `view_chrome.go`
  - tabs/meta/footer/status lines.
- `view_sizes.go`
  - screen-size-dependent list/log windows.
- `view_helpers.go`
  - shared display formatting helpers.

## `sdk/` layout

- `client_core.go`
  - client state, constructor, config getters/setters.
- `client_scan.go`
  - discovery, quick-connect, scan option mapping.
- `client_connection.go`
  - connect/reconnect/disconnect/probe operations.
- `client_inventory_control.go`
  - apply config, start/stop inventory, stats.
- `client_inventory_runtime.go`
  - runtime loops and frame/tag processing.
- `client_events.go`
  - non-blocking event emitters.

## Editing Rules (recommended)

- Keep entrypoints in `cmd/*` thin.
- Put protocol changes under `internal/protocol/reader18/`.
- Put UI behavior changes in `internal/tui/update_*` and UI rendering in `internal/tui/view_*`.
- Prefer small feature files (target: up to ~250 lines where practical).
- Keep helper functions pure and colocated in feature helper files.

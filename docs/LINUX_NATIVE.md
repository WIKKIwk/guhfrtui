# Linux Native Setup

This project runs natively on Linux.

## 1. Install dependencies

### Ubuntu

```bash
sudo apt update
sudo apt install -y golang-go build-essential ca-certificates git
```

### Arch Linux

```bash
sudo pacman -S --needed go base-devel ca-certificates git
```

## 2. Prepare environment

```bash
cp .env.bot.example .env
# edit .env with your ERP/Telegram credentials
```

## 3. Run locally

```bash
# TUI + bot sidecar
make run

# bot only (headless service mode)
make bot

# network scan diagnostics
make scan
```

## 4. Build binaries

```bash
make build
ls -lh bin/
```

Generated binaries:

- `bin/st8508-tui`
- `bin/rfid-go-bot`
- `bin/scancheck`

## 5. Install binaries (optional)

```bash
sudo make install PREFIX=/usr/local
```

## 6. Verify

```bash
make check
```

## Notes

- TUI auto-starts bot sidecar by default (`BOT_AUTOSTART=1`).
- Bot IPC socket default: `/tmp/rfid-go-bot.sock`.

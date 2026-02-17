# RFID Go Bot

Minimal Telegram + ERPNext bot for EPC submit flow.

## Flow

1. Reads config from `.env` (`BOT_TOKEN`, `ERP_URL`, `ERP_API_KEY`, `ERP_API_SECRET`).
2. Loads open draft EPC cache from ERP method:
   - `titan_telegram.api.get_open_stock_entry_drafts_fast` (`epc_only=1`)
3. Accepts EPC events from RFID child app over local IPC socket (`BOT_IPC_SOCKET`).
4. If EPC exists in cache, submits via ERP method:
   - `titan_telegram.api.submit_open_stock_entry_by_epc`
5. On successful submit, removes EPC from cache immediately.
6. Periodically refreshes cache (`BOT_CACHE_REFRESH_SEC`, default `60`).
7. Accepts ERP draft webhook updates (`POST /webhook/draft`) to append EPCs.

## Run

```bash
go run ./cmd/rfid-go-bot
```

## .env example

```env
BOT_TOKEN=123456789:telegram_bot_token
ERP_URL=https://erp.example.com
ERP_API_KEY=your_api_key
ERP_API_SECRET=your_api_secret

BOT_IPC_ENABLED=1
BOT_IPC_SOCKET=/tmp/rfid-go-bot.sock
BOT_CACHE_REFRESH_SEC=5
BOT_SUBMIT_RETRY=2
BOT_SUBMIT_RETRY_MS=300
BOT_WORKER_COUNT=4
BOT_QUEUE_SIZE=2048
BOT_RECENT_SEEN_TTL_SEC=600
BOT_POLL_TIMEOUT_SEC=25
BOT_WEBHOOK_SECRET=change_me
BOT_SCAN_BACKEND=ingest
BOT_SCAN_DEFAULT_ACTIVE=1
BOT_AUTO_SCAN=0
BOT_READER_CONNECT_TIMEOUT_SEC=25
BOT_READER_RETRY_SEC=2
BOT_READER_HOST=
BOT_READER_PORT=
```

## RFID child app -> bot (IPC)

```bash
printf '{"type":"scan_start","source":"child"}\n' | socat - UNIX-CONNECT:/tmp/rfid-go-bot.sock
printf '{"type":"epc","epc":"E2000017221101441890ABCD","source":"child"}\n' | socat - UNIX-CONNECT:/tmp/rfid-go-bot.sock
printf '{"type":"scan_stop","source":"child"}\n' | socat - UNIX-CONNECT:/tmp/rfid-go-bot.sock
```

## ERP -> bot (new draft webhook)

```bash
printf '{"type":"draft_epcs","epcs":["E2000017221101441890ABCD"],"source":"erp"}\n' | socat - UNIX-CONNECT:/tmp/rfid-go-bot.sock
```

## Telegram commands

- `/start`
- `/scan`
- `/stop`
- `/status`
- `/range20 on|off|status` (best-effort long-range profile)
- `/turbo`

`/range20 on` applies built-in strict profile: region + frequency window + per-antenna max power.

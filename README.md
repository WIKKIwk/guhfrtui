# New Era Go: ST-8508 RFID Reader Platform

Akademik-amaliy loyiha uchun chuqur texnik hujjat.

## Annotatsiya
Ushbu loyiha ST-8508 turidagi LAN UHF RFID reader qurilmalarini boshqarish, EPC oqimini real vaqt rejimida qayta ishlash va ERPNext bilan avtomatik sinxronlash uchun yozilgan Go platformasidir. Tizim ikki asosiy ishchi qatlamdan iborat: terminal boshqaruv interfeysi (`st8508-tui`) va fon xizmati (`rfid-go-bot`). Arxitektura reader protokoli, tarmoq discovery, EPC deduplikatsiya, ishchi navbat (queue), retry mexanizmi, Telegram boshqaruvi va HTTP/IPC integratsiyalarini birlashtiradi.

Bu README institut amaliy ishi formatida: muammo qo'yilishi, arxitektura, algoritmlar, modul tahlili, interfeys shartnomalari, ekspluatatsiya, diagnostika va kengaytirish yo'nalishlarini to'liq yoritadi.

## Kalit so'zlar
RFID, UHF, ST-8508, Reader18, EPC, ERPNext, Go, Bubble Tea TUI, IPC, webhook, Telegram bot, worker queue.

## 1. Muammo qo'yilishi va maqsad
### Muammo
Ombor/logistika stsenariylarida RFID readerdan kelayotgan EPC oqimini:
1. past kechikish bilan qabul qilish,
2. dublikatlarni nazorat qilish,
3. faqat ERP draftlari bilan ishlash,
4. submit natijalarini ishonchli qayta ishlash,
5. operator uchun tushunarli boshqaruv interfeysi bilan berish
murakkab amaliy masala hisoblanadi.

### Loyiha maqsadi
1. Readerni LAN ichida avtomatik topish va ulash.
2. Reader18 protokolini ishonchli parse/build qilish.
3. ERPNext draft EPC cache bilan mos submit pipeline yaratish.
4. TUI + Telegram + HTTP + IPC orqali boshqariladigan yagona platforma qurish.
5. Docker'siz, Linux-native (Ubuntu/Arch) ishlash.

## 2. Tizimning umumiy arxitekturasi

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

## 3. End-to-end ish oqimi
### 3.1 TUI ishga tushish oqimi
1. `cmd/st8508-tui/main.go` `.env` ni yuklaydi (`BOT_ENV_FILE`, default `.env`).
2. Sidecar (`cmd/st8508-tui/sidecar.go`) IPC socket (`BOT_IPC_SOCKET`) orqali botni ping qiladi.
3. Bot yo'q bo'lsa, avtomatik start qiladi (`BOT_AUTOSTART=1`).
4. TUI `runScanCmd` orqali startup discovery ni boshlaydi.
5. TUI va bot IPC orqali `status`, `scan_start`, `scan_stop`, `epc` almashadi.

### 3.2 EPC submit oqimi
1. EPC readerdan keladi.
2. TUI yoki SDK reader manager EPCni servicega uzatadi.
3. Service EPCni normalize qiladi (`A-F0-9` qoldiriladi).
4. `scan_active=false` bo'lsa `scan_inactive` sifatida hisoblanadi.
5. Cache hit bo'lsa queuega tushadi (`queued`), miss bo'lsa `miss`.
6. Worker ERP `submit_open_stock_entry_by_epc` ga yuboradi.
7. `submitted` yoki `not_found` bo'lsa EPC cache'dan chiqariladi.

### 3.3 Replay oqimi
`recentSeen` xaritada TTL oynasida ko'rilgan EPClar saqlanadi. Scan qayta yoqilganda yoki yangi draft kirganda, `recentSeen ∩ cache` EPClar qayta enqueue qilinadi.

## 4. Modul bo'yicha chuqur tahlil
## 4.1 `internal/discovery`
Vazifa: LAN ichida ehtimoliy reader endpointlarni topish.

Asosiy g'oyalar:
1. Faol IPv4 interfacelar bo'yicha prefixlar olinadi (`/24` minimum skan oynasi).
2. `/proc/net/arp` qo'shnilar qo'shiladi.
3. Statik seed IPlar ham tekshiriladi.
4. Har host uchun portlar parallel probe qilinadi.
5. Reader18 handshake tekshiruvi:
   1. `GetReaderInfo` (0x21),
   2. `Inventory G2` (0x01),
   3. `Inventory legacy`.
6. `Verified` endpointlar yuqori score oladi va birinchi tanlanadi.

Default scan parametrlari:
- Ports: `2022, 27011, 6000, 4001, 10001, 5000`
- Timeout: `180ms`
- Concurrency: `96`
- HostLimitPerInterface: `254`

## 4.2 `internal/protocol/reader18`
Vazifa: Reader18 frame encode/decode.

Frame formati:

```text
Len(1) Adr(1) Cmd(1) Status(1) Data(N) CRC_L(1) CRC_H(1)
```

CRC: `CRC16-MCRF4XX`.

Qo'llanilgan buyruqlar:
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

Status kodlar:
- `0x00` success
- `0x01` no tag
- `0xFB` no tag/timeout
- `0xF8` antenna error
- `0xFE` command error
- `0xFF` crc/param error

## 4.3 `internal/reader`
Vazifa: TCP transport sessiyasini boshqarish.

Imkoniyatlar:
1. `Connect`/`Disconnect`.
2. Async `packets` va `errors` channel.
3. `SendRaw` bilan raw paket yuborish.

## 4.4 `sdk`
Public SDK arxitekturasi:
1. `client_scan.go`: discovery va quick-connect.
2. `client_connection.go`: connect/reconnect/probe.
3. `client_inventory_control.go`: config apply + inventory start/stop.
4. `client_inventory_runtime.go`: rx/tx loop, frame parse, tag event.

Muhim formula:

```text
effective_cycle = max(PollInterval, ScanTime*100ms, 40ms)
```

Qo'shimcha xulq:
1. `SingleFallbackEach` bo'yicha davriy single inventory yuboriladi.
2. No-tag holatida `A/B target switch` mexanizmi bor.

## 4.5 `internal/tui`
TUI sahifalari:
1. Home
2. Devices
3. Control
4. Inventory Tune
5. Regions
6. Logs
7. Help

TUI funksional yadrosi:
1. Startup discovery.
2. Connect plan (fallback portlar bilan ketma-ket urinish).
3. Inventory command scheduling (`inventory-g2` + periodik `inventory-single`).
4. Reader frame parse va unique EPC hisoblash.
5. IPC orqali bot bilan sinxron ishlash.

## 4.6 `internal/gobot/service`
Bu loyihaning asosiy business-core qatlami.

Ma'lumot tuzilmalari:
1. `cache.Store`: draft EPC hash-map.
2. `recentSeen`: TTL oynadagi EPClar.
3. `queued` va `inflight`: parallel submitda dublikatni to'sish.
4. `queue chan string`: worker navbat.

Pipeline xususiyatlari:
1. `RefreshCache` ERP'dan draft EPClar olib `cache.Replace` qiladi.
2. `HandleEPC` hit/miss/inactive statistikasini yuritadi.
3. `enqueue` queue full bo'lsa `queue_dropped` oshiradi.
4. Worker `SubmitRetry` va `SubmitRetryDelay` bilan retry qiladi.

## 4.7 `internal/gobot/erp`
Ikkita asosiy ERP API:
1. `get_open_stock_entry_drafts_fast` (`epc_only=1`) - draft EPC list.
2. `submit_open_stock_entry_by_epc` - submit.

Normalization:
- EPC uppercase qilinadi.
- faqat hex belgilar qoldiriladi.

## 4.8 `internal/gobot/ipc`
Unix socket JSON-line server.

Qo'llab-quvvatlanadigan `type` lar:
1. `status`
2. `scan_start`
3. `scan_stop`
4. `turbo`
5. `epc`
6. `epcs`
7. `draft_epc`
8. `draft_epcs`

## 4.9 `internal/gobot/httpapi`
HTTP endpointlar:
1. `GET /health`
2. `GET /stats`
3. `POST /ingest`
4. `POST /webhook/draft`
5. `POST /api/webhook/erp` (legacy)
6. `POST /turbo`
7. `POST /scan/start`
8. `POST /scan/stop`

`/webhook/draft` uchun `X-Webhook-Secret` tekshiruvi `BOT_WEBHOOK_SECRET` orqali ishlaydi.

## 4.10 `internal/gobot/telegram`
Telegram bot long-poll asosida ishlaydi.

Asosiy buyruqlar:
1. `/start`, `/help`
2. `/scan`, `/read`, `/read stop`, `/stop`
3. `/status`
4. `/cache` (2 ta txt snapshot yuboradi)
5. `/range20 on|off|status` va aliaslar
6. `/turbo`
7. `/test`
8. `/test_stop`

Qo'shimcha imkoniyatlar:
1. Startup habarini keyin edit qilish (`SendStartupNotice` + `EditNotices`).
2. `sendDocument` bilan txt fayl yuborish.
3. `deleteMessage` orqali command/file chat tozalash.

## 4.11 `internal/gobot/testmode`
`/test` rejimi EPC verifikatsiyasi uchun maxsus modul.

Qoidalar:
1. `/test` berilganda oldingi test state xotiradan tozalanadi.
2. Bot `.txt` kutadi.
3. TXT parse:
   1. bo'sh satrlar va `#` kommentlar skip,
   2. EPC normalize,
   3. unique/duplicate/invalid sanaladi.
4. Fayl yuborilgan user message o'chiriladi.
5. `/test` prompt habari `edit` bo'lib "Fayl qabul qilindi" ko'rinishiga o'tadi.
6. Reader EPC o'qiganda mos EPC uchun bir marta `O'qildi` habari yuboriladi.
7. `/test_stop` command message ham o'chiriladi.
8. `O'qildi` live habarlar tozalanadi va prompt `edit` bo'lib yakuniy natija chiqadi.

## 5. Algoritmik qarorlar va murakkablik
## 5.1 Discovery
Agar `H` host va `P` port bo'lsa, probing murakkabligi taxminan `O(H*P)`. Parallel workerlar (`Concurrency`) wall-clock vaqtni kamaytiradi.

## 5.2 Cache lookup
Draft EPC tekshiruvi hash-map asosida `O(1)` o'rtacha murakkablikda.

## 5.3 Queue deduplikatsiya
`queued` + `inflight` setlari bir EPCning parallel duplicate submit bo'lishini to'sadi.

## 5.4 Replay mexanizmi
`recentSeen` TTL oynasi qayta ishga tushirilgan scan paytida yo'qotilgan EPClarni recovery qilishga xizmat qiladi.

## 6. Konfiguratsiya (to'liq)
Eslatma: `internal/gobot/config.Load()` bo'yicha quyidagi 4 ta maydon majburiy:
1. `BOT_TOKEN`
2. `ERP_URL`
3. `ERP_API_KEY`
4. `ERP_API_SECRET`

## 6.1 Core service (`rfid-go-bot`)
| O'zgaruvchi | Default | Izoh |
|---|---:|---|
| `BOT_HTTP_ENABLED` | `1` | HTTP server yoqish/o'chirish |
| `BOT_HTTP_ADDR` | `:8098` | HTTP listen manzil |
| `BOT_IPC_ENABLED` | `1` | IPC socket server yoqish/o'chirish |
| `BOT_IPC_SOCKET` | `/tmp/rfid-go-bot.sock` | IPC socket path |
| `BOT_HTTP_TIMEOUT_MS` | `12000` | HTTP/ERP timeout |
| `BOT_CACHE_REFRESH_SEC` | `5` | Periodik cache refresh (min 5s) |
| `BOT_SUBMIT_RETRY` | `2` | Submit retry soni |
| `BOT_SUBMIT_RETRY_MS` | `300` | Retry oralig'i |
| `BOT_WORKER_COUNT` | `4` | Worker soni (min 1) |
| `BOT_QUEUE_SIZE` | `2048` | Queue sig'imi (min 64) |
| `BOT_RECENT_SEEN_TTL_SEC` | `600` | recentSeen TTL (min 30s) |
| `BOT_POLL_TIMEOUT_SEC` | `25` | Telegram poll timeout (5..55s clamp) |
| `BOT_SCAN_BACKEND` | `hybrid` | `ingest|sdk|hybrid` |
| `BOT_SCAN_DEFAULT_ACTIVE` | `1` | startup scan active holati |
| `BOT_AUTO_SCAN` | `0` | SDK auto-scan loop |
| `BOT_READER_HOST` | `` | reader hostni fixed qilish |
| `BOT_READER_PORT` | `0` | reader portni fixed qilish |
| `BOT_READER_CONNECT_TIMEOUT_SEC` | `25` | reader connect timeout (min 5s) |
| `BOT_READER_RETRY_SEC` | `2` | reconnect delay (min 500ms) |
| `BOT_WEBHOOK_SECRET` | `` | `/webhook/draft` secret |
| `BOT_CHAT_STORE_FILE` | `logs/telegram_chats.json` | Telegram chat registry |
| `BOT_CACHE_DUMP_DIR` | `BOT_LOG_DIR` yoki `logs` | `/cache` txt dump papkasi |
| `BOT_LOG_DIR` | `logs` | bot log papkasi |
| `BOT_SHOW_TUI` | `auto` | bot binary ichida TUI ko'rsatish (`0/1`) |

## 6.2 TUI sidecar va bot sync
| O'zgaruvchi | Default | Izoh |
|---|---:|---|
| `BOT_AUTOSTART` | `1` | TUI botni auto-start qiladimi |
| `BOT_AUTOSTART_CMD` | `` | botni custom command bilan ishga tushirish |
| `BOT_AUTOSTART_ROOT` | `` | bot root qidiruvini override qilish |
| `BOT_ENV_FILE` | `.env` | env fayl manzili |
| `BOT_SYNC_ENABLED` | `1` | TUI->bot IPC sync |
| `BOT_SYNC_SOCKET` | `BOT_IPC_SOCKET` | sync socket path |
| `BOT_SYNC_TIMEOUT_MS` | `1200` | sync timeout (min 200ms) |
| `BOT_SYNC_QUEUE_SIZE` | `4096` | EPC sync buffer (min 128) |
| `BOT_SYNC_SOURCE` | `st8508-tui` | source label |
| `BOT_SYNC_MODE` | n/a | hozirgi kodda aktiv ishlatilmaydi |

## 7. Linux-native o'rnatish (Docker'siz)
## 7.1 Talablar
1. Linux (Ubuntu yoki Arch)
2. Go 1.25+
3. LAN orqali readerga kirish
4. ERP + Telegram credentiallar

## 7.2 Dependency o'rnatish
```bash
# Ubuntu
sudo apt update
sudo apt install -y golang-go build-essential ca-certificates git

# Arch
sudo pacman -S --needed go base-devel ca-certificates git
```

## 7.3 Tez start
```bash
git clone <repo_url>
cd new_era_gos
cp .env.bot.example .env
# .env ni to'ldiring
make run
```

## 7.4 Ishga tushirish variantlari
```bash
# TUI + bot sidecar
make run

# faqat bot
make bot

# discovery diagnostika
make scan

# build
make build

# test+format check
make check
```

## 7.5 Systemd
Tayyor unit: `deploy/systemd/rfid-go-bot.service`

Standart ishga tushirish:
- WorkingDirectory: `/opt/new_era_gos`
- ExecStart: `/opt/new_era_gos/bin/rfid-go-bot`

## 8. IPC shartnomasi
Socket: `/tmp/rfid-go-bot.sock` (default).

Request namunalar:
```json
{"type":"status","source":"st8508-tui"}
{"type":"scan_start","source":"st8508-tui"}
{"type":"scan_stop","source":"st8508-tui"}
{"type":"epc","epc":"E200...","source":"st8508-tui"}
{"type":"draft_epcs","epcs":["E200..."],"source":"erp"}
```

Response umumiy shakli:
```json
{
  "ok": true,
  "action": "scan_start",
  "replayed_seen": 2,
  "warning": "",
  "stats": {"cache_size": 120, "scan_active": true}
}
```

## 9. HTTP API shartnomasi
## 9.1 Health/Stats
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

## 9.4 Scan boshqaruvi
```bash
curl -s -X POST http://127.0.0.1:8098/scan/start
curl -s -X POST http://127.0.0.1:8098/scan/stop
```

## 9.5 Turbo refresh
```bash
curl -s -X POST http://127.0.0.1:8098/turbo
```

## 10. Telegram bot buyruqlari
| Buyruq | Maqsad |
|---|---|
| `/start`, `/help` | chatni ro'yxatdan o'tkazish va yordam |
| `/scan` | scan start |
| `/read` | `/scan` alias |
| `/read stop` | `/stop` alias |
| `/stop` | scan stop |
| `/status` | service + reader status |
| `/cache` | `cache_draft_epcs.txt` va `cache_seen_epcs.txt` yuborish |
| `/range20 on/off/status` | long-range profil boshqaruvi |
| `/range20_on`, `/range20_off`, `/range20_status` | tez aliaslar |
| `/turbo` | darhol cache refresh |
| `/test` | EPC test session boshlash (txt kutish) |
| `/test_stop` | test yakuni va natijani chiqarish |

`/test` oqimida command va file message chatdan tozalanadi, natijalar prompt message edit orqali ko'rsatiladi.

## 11. TUI boshqaruv klavishlari
## 11.1 Global
- `q`: chiqish
- `m`: Home
- `b` yoki `0`: orqaga/Home

## 11.2 Home
- `1..7`: sahifa ochish
- `enter`: tanlangan item
- `j/k` yoki `up/down`: navigatsiya

## 11.3 Devices
- `s`: qayta scan
- `a`: quick connect
- `enter`: tanlangan endpointga ulanish

## 11.4 Control
- `enter`: action bajarish
- `/`: raw hex rejimi

## 11.5 Inventory Tune
- `h/l` yoki `left/right`: parametr o'zgartirish
- `enter`: apply/action

## 12. Diagnostika va observability
## 12.1 Loglar
- Sidecar bot log: `logs/rfid-go-bot.log`
- TUI ichida log panel mavjud (`Logs` sahifasi)

## 12.2 Scan diagnostika utilitasi
```bash
go run ./cmd/scancheck
```
Bu utilita:
1. local interfacelarni chiqaradi,
2. discovery duration va candidatelarni ko'rsatadi,
3. `verified`, `protocol`, `score`, `reason` maydonlarini beradi.

## 12.3 Runtime statistikalar
`service.Stats` maydonlari:
- `cache_size`, `draft_count`
- `seen_total`, `cache_hits`, `cache_misses`, `scan_inactive`
- `submitted_ok`, `submit_not_found`, `submit_errors`, `queue_dropped`
- `last_refresh_at`, `last_refresh_ok`

## 13. Testlash va sifat nazorati
Loyihada unit testlar mavjud (`45` ta `Test*` funksiya).

Qamrovga kiradigan asosiy yo'nalishlar:
1. Protocol packet build/parse va CRC.
2. Discovery defaultlar va host generation.
3. TUI update flow va unique tag hisob.
4. Service replay/logika va snapshot tartiblash.
5. Telegram command parse.
6. Testmode parser va session replace.

Ishga tushirish:
```bash
go test ./...
```

## 14. Xatolik holatlari va yechimlar
## 14.1 `BOT_TOKEN is required`
`.env` ichida `BOT_TOKEN`, `ERP_URL`, `ERP_API_KEY`, `ERP_API_SECRET` to'ldirilmagan.

## 14.2 Reader topilmayapti
1. `make scan` bilan networkni tekshiring.
2. Reader IP/portni `BOT_READER_HOST`, `BOT_READER_PORT` bilan fixed qiling.
3. Firewall/VLAN cheklovlarini tekshiring.

## 14.3 IPC ulanmayapti
1. `BOT_IPC_SOCKET` bir xil bo'lishi kerak.
2. Socket file eskirgan bo'lsa o'chirib qayta ishga tushiring.

## 14.4 Webhook 401
`X-Webhook-Secret` qiymati `BOT_WEBHOOK_SECRET` bilan bir xil emas.

## 15. Cheklovlar
1. Discovery asosan IPv4 va LAN segmentlarga yo'naltirilgan.
2. Cache/recentSeen default holatda process-memoryda saqlanadi (persistent DB yo'q).
3. Reader bilan aloqa TCP plain ko'rinishda (LAN darajasida himoya tavsiya etiladi).
4. Konfiguratsiya env-ga bog'liq, noto'g'ri env qiymatlari ishga tushishni to'xtatadi.

## 16. Kengaytirish yo'nalishlari
1. Persistent storage (SQLite/PostgreSQL) bilan queue/cacheni tiklash.
2. Structured metrics (Prometheus/OpenTelemetry) qo'shish.
3. Multi-reader orchestration (bir nechta readerni parallel boshqarish).
4. HTTPS + mTLS bilan tashqi API xavfsizligini oshirish.
5. /test natijalari uchun CSV/Excel export.

## 17. Loyihaning katalog tuzilmasi
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

## Xulosa
New Era Go platformasi ST-8508 readerlar bilan real ishlab chiqarish sharoitiga yaqin amaliy vazifalarni yechish uchun qurilgan: discovery, inventory, EPC deduplikatsiya, ERP submit, Telegram monitoring va Linux-native ekspluatatsiya. Arxitektura qatlamlarga ajratilganligi sabab tizimni institut amaliy ishidan keyin ham sanoat darajasiga bosqichma-bosqich olib chiqish mumkin.

## Litsenziya
Proprietary. All rights reserved.

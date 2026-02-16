.PHONY: run up down logs shell

up:
	@docker compose up -d --build rfid-go

run: up
	@docker compose exec -it \
		-e BOT_ENV_FILE=/workspace/new_era_go/.env \
		-e BOT_SHOW_TUI=0 \
		rfid-go bash -c '\
			set -euo pipefail; \
			cd /workspace/new_era_go; \
			mkdir -p /workspace/.cache/new-era-go/gocache /workspace/.cache/new-era-go/gomod; \
			export GOCACHE=/workspace/.cache/new-era-go/gocache; \
			export GOMODCACHE=/workspace/.cache/new-era-go/gomod; \
			if [[ -x /usr/local/go/bin/go ]]; then GO_BIN=/usr/local/go/bin/go; else GO_BIN=$$(command -v go || true); fi; \
			if [[ -z "$$GO_BIN" ]]; then echo "go binary topilmadi" >&2; exit 1; fi; \
			exec "$$GO_BIN" run ./cmd/st8508-tui \
		'

down:
	@docker compose down

logs:
	@docker compose logs -f rfid-go

shell: up
	@docker compose exec -it rfid-go bash

SHELL := /usr/bin/env bash

GO ?= go
ENV_FILE ?= .env
BINDIR ?= bin
PREFIX ?= /usr/local

.PHONY: run run-tui bot run-bot scan build build-all install fmt fmt-check test check clean

run: run-tui

run-tui:
	@BOT_ENV_FILE="$(ENV_FILE)" $(GO) run ./cmd/st8508-tui

bot: run-bot

run-bot:
	@BOT_ENV_FILE="$(ENV_FILE)" BOT_SHOW_TUI=0 $(GO) run ./cmd/rfid-go-bot

scan:
	@BOT_ENV_FILE="$(ENV_FILE)" $(GO) run ./cmd/scancheck

build: build-all

build-all:
	@mkdir -p "$(BINDIR)"
	@$(GO) build -o "$(BINDIR)/st8508-tui" ./cmd/st8508-tui
	@$(GO) build -o "$(BINDIR)/rfid-go-bot" ./cmd/rfid-go-bot
	@$(GO) build -o "$(BINDIR)/scancheck" ./cmd/scancheck

install: build-all
	@install -d "$(PREFIX)/bin"
	@install -m 0755 "$(BINDIR)/st8508-tui" "$(PREFIX)/bin/st8508-tui"
	@install -m 0755 "$(BINDIR)/rfid-go-bot" "$(PREFIX)/bin/rfid-go-bot"
	@install -m 0755 "$(BINDIR)/scancheck" "$(PREFIX)/bin/scancheck"

fmt:
	@files="$$(rg --files -g '*.go')"; \
	if [[ -n "$$files" ]]; then \
		gofmt -w $$files; \
	fi

fmt-check:
	@files="$$(rg --files -g '*.go')"; \
	if [[ -z "$$files" ]]; then \
		exit 0; \
	fi; \
	unformatted="$$(gofmt -l $$files)"; \
	if [[ -n "$$unformatted" ]]; then \
		echo "Unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

test:
	@$(GO) test ./...

check: fmt-check test

clean:
	@rm -rf "$(BINDIR)"

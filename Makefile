# SEO trend explorer
#
# Default: start a debuggable Chrome on port 9222, then run the app against it.
#   make            same as: make run
#   make run        Chrome (reuse or launch) + app with real Google Trends
#   make run-fake   app with synthetic data, no Chrome
#   make chrome     only start/verify Chrome
#   make test       go test ./...
#   make stop       close the debug Chrome and the app started by make

SHELL   := /bin/sh
.DEFAULT_GOAL := run
CHROME  ?= /Applications/Google Chrome.app/Contents/MacOS/Google Chrome
PROFILE ?= $(HOME)/.seo-chrome
PORT    ?= 9222
DB      ?= seo.db
ADDR    ?= 127.0.0.1:8080
APP     ?= ./cmd/seotrend
DEBUG   := http://127.0.0.1:$(PORT)
TRENDS  := https://trends.google.com/trends/explore
PIDFILE := .seotrend.pid

.PHONY: help run run-fake chrome app test vet build stop clean

help:
	@echo "make            start Chrome + app (real data)"
	@echo "make run        same as make"
	@echo "make run-fake   app with fake data, no Chrome"
	@echo "make chrome     start or verify the debug Chrome on $(PORT)"
	@echo "make test       go test ./..."
	@echo "make build      build ./bin/seotrend"
	@echo "make stop       stop the debug Chrome and the app"
	@echo "variables: PORT=$(PORT) DB=$(DB) ADDR=$(ADDR) PROFILE=$(PROFILE)"

run: chrome app

# Reuse a running debug Chrome, or launch one with a dedicated profile.
# First launch may show a consent or verification page; finish it by hand.
chrome:
	@if curl -sf $(DEBUG)/json/version >/dev/null 2>&1; then \
		echo "[chrome] already listening on $(PORT)"; \
	else \
		echo "[chrome] launching with profile $(PROFILE)"; \
		open -na "Google Chrome" --args \
			--remote-debugging-port=$(PORT) \
			--user-data-dir="$(PROFILE)" \
			--no-first-run \
			--no-default-browser-check \
			"$(TRENDS)" || { \
				echo "[chrome] could not launch Google Chrome. Set CHROME=<path>." >&2; exit 1; }; \
		n=0; \
		while [ $$n -lt 40 ]; do \
			if curl -sf $(DEBUG)/json/version >/dev/null 2>&1; then \
				echo "[chrome] ready on $(PORT)"; \
				break; \
			fi; \
			n=$$((n+1)); \
			sleep 0.5; \
		done; \
		if ! curl -sf $(DEBUG)/json/version >/dev/null 2>&1; then \
			echo "[chrome] port $(PORT) did not open. Close other Chrome windows or set PORT=<other>." >&2; \
			exit 1; \
		fi; \
	fi
	@echo "[chrome] if Google asks for verification, finish it in the opened window."

# Foreground app. Ctrl-C stops the app only; run 'make stop' for Chrome too.
app:
	@echo "[app] http://$(ADDR) (db=$(DB), gateway=browser)"
	@go run $(APP) -db "$(DB)" -addr "$(ADDR)" -gateway browser -chrome-remote $(DEBUG)

run-fake:
	@echo "[app] http://$(ADDR) (db=$(DB), gateway=fake)"
	@go run $(APP) -db "$(DB)" -addr "$(ADDR)" -gateway fake

test:
	@go test ./...

vet:
	@go vet ./...

build:
	@mkdir -p bin
	@go build -o bin/seotrend $(APP)
	@echo "[build] bin/seotrend"

# Only kills the Chrome instance that uses our debug port, and the app binary.
stop:
	@-pkill -f "remote-debugging-port=$(PORT)" 2>/dev/null && echo "[chrome] stopped" || true
	@-pkill -f "seotrend -db" 2>/dev/null && echo "[app] stopped" || true
	@-rm -f $(PIDFILE)

clean: stop
	@rm -rf bin
	@echo "[clean] bin removed"

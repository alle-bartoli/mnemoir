.PHONY: \
	help \
	build \
	install \
	uninstall \
	test \
	docker-up \
	docker-down \
	redis-ui \
	setup \
	setup-codex \
	mcp \
	mcp-global \
	mcp-codex \
	hook \
	hook-codex \
	specs \
	specs-codex \
	backup \
	restore \
	backup-json \
	restore-json \
	clean \
	clean-data

# Load .env if present so env-var checks work without manual export
-include .env
export

BINARY := mnemoir
BIN_DIR := bin
CMD_DIR := ./cmd/mnemoir
CONFIG_DIR := $(HOME)/.mnemoir
REDIS_UI_URL := http://localhost:8001

help:
	@echo "Available targets:"
	@echo "  make build         - Build binary to bin/$(BINARY)"
	@echo "  make install       - Install binary to \$$GOPATH/bin"
	@echo "  make uninstall     - Remove binary, MCP registration, and config"
	@echo "  make test          - Run all tests"
	@echo "  make docker-up     - Start Redis Stack (Redis + RedisInsight)"
	@echo "  make docker-down   - Stop Redis Stack"
	@echo "  make redis-ui      - Open RedisInsight web UI (http://localhost:8001)"
	@echo ""
	@echo "claude:"
	@echo "  make setup         - Full install for claude (docker + build + config + MCP + hook + specs)"
	@echo "  make mcp           - Register MCP server with claude (project-local)"
	@echo "  make mcp-global    - Register MCP server globally (all claude projects)"
	@echo "  make hook          - Install claude SessionEnd hook"
	@echo "  make specs         - Install agent specs into ~/.claude/memory/"
	@echo ""
	@echo "OpenAI Codex CLI:"
	@echo "  make setup-codex   - Full install for Codex CLI (docker + build + config + MCP + hook + specs)"
	@echo "  make mcp-codex     - Register MCP server with Codex CLI"
	@echo "  make hook-codex    - Install Codex CLI Stop hook"
	@echo "  make specs-codex   - Install agent specs into ~/.codex/AGENTS.md"
	@echo ""
	@echo "Backup / restore:"
	@echo "  make backup        - Hot backup Redis data dir via BGSAVE (OUTPUT=path/to/dir, default: ~/.mnemoir/backups/YYYYMMDD)"
	@echo "  make restore       - Cold restore Redis data dir (INPUT=path/to/dir)"
	@echo "  make backup-json   - Dump all memories to JSON (OUTPUT=path/to/file.json, default: ~/.mnemoir/backups/YYYYMMDD.json)"
	@echo "  make restore-json  - Restore memories from JSON (INPUT=path/to/file.json)"
	@echo ""
	@echo "  make clean         - Remove build artifacts"
	@echo "  make clean-data    - Stop Redis and wipe all stored memories (data/)"

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)

install:
	go install $(CMD_DIR)

uninstall:
	@echo "Removing binary from GOPATH..."
	rm -f $(shell go env GOPATH)/bin/$(BINARY)
	@echo "Removing claude MCP registration..."
	-claude mcp remove $(BINARY) 2>/dev/null
	@echo "Removing claude SessionEnd hook..."
	-$(CURDIR)/scripts/uninstall-hook.sh 2>/dev/null
	@echo "Removing agent specs from ~/.claude/..."
	-$(CURDIR)/scripts/uninstall-specs.sh 2>/dev/null
	@echo "Removing Codex CLI MCP registration..."
	-$(CURDIR)/scripts/uninstall-mcp-codex.sh 2>/dev/null
	@echo "Removing Codex CLI Stop hook..."
	-$(CURDIR)/scripts/uninstall-hook-codex.sh 2>/dev/null
	@echo "Removing agent specs from ~/.codex/..."
	-$(CURDIR)/scripts/uninstall-specs-codex.sh 2>/dev/null
	@echo "Removing config directory $(CONFIG_DIR)..."
	rm -rf $(CONFIG_DIR)
	@echo "Removing build artifacts..."
	rm -rf $(BIN_DIR)
	@echo "mnemoir uninstalled."

test:
	go test ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

redis-ui:
	@echo "Opening RedisInsight at $(REDIS_UI_URL)"
	@open $(REDIS_UI_URL) || xdg-open $(REDIS_UI_URL) || echo "Please open $(REDIS_UI_URL) manually"

setup: docker-up build
	@mkdir -p -m 700 $(CONFIG_DIR)
	@if [ ! -f $(CONFIG_DIR)/config.toml ]; then \
		cp config/default.toml $(CONFIG_DIR)/config.toml; \
		chmod 600 $(CONFIG_DIR)/config.toml; \
		echo "Config copied to $(CONFIG_DIR)/config.toml"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/config.toml"; \
	fi
	@if [ -z "$$MNEMOIR_REDIS_PASSWORD" ]; then \
		echo "WARNING: MNEMOIR_REDIS_PASSWORD is not set. Set it before starting Redis."; \
	fi
	@echo ""
	@echo "Registering MCP server (global)..."
	@$(MAKE) --no-print-directory mcp-global
	@echo ""
	@echo "Installing SessionEnd hook..."
	@$(MAKE) --no-print-directory hook
	@echo ""
	@echo "Installing agent specs..."
	@$(MAKE) --no-print-directory specs
	@echo ""
	@echo "Setup complete. Edit $(CONFIG_DIR)/config.toml to customize."

hook:
	@$(CURDIR)/scripts/install-hook.sh $(CURDIR)/scripts/session-end-hook.sh

hook-codex:
	@$(CURDIR)/scripts/install-hook-codex.sh $(CURDIR)/scripts/session-end-hook.sh

specs:
	@$(CURDIR)/scripts/install-specs.sh $(CURDIR)/docs/agent-specs.md

specs-codex:
	@$(CURDIR)/scripts/install-specs-codex.sh $(CURDIR)/docs/agent-specs.md

mcp: build
	@if [ -z "$$MNEMOIR_REDIS_PASSWORD" ]; then \
		echo "ERROR: MNEMOIR_REDIS_PASSWORD is not set. Set it in .env or export it."; \
		exit 1; \
	fi
	claude mcp add $(BINARY) -s local -t stdio -e MNEMOIR_REDIS_PASSWORD="$$MNEMOIR_REDIS_PASSWORD" -- $(CURDIR)/$(BIN_DIR)/$(BINARY) --config $(CONFIG_DIR)/config.toml

mcp-global: build
	@if [ -z "$$MNEMOIR_REDIS_PASSWORD" ]; then \
		echo "ERROR: MNEMOIR_REDIS_PASSWORD is not set. Set it in .env or export it."; \
		exit 1; \
	fi
	claude mcp add $(BINARY) -s user -t stdio -e MNEMOIR_REDIS_PASSWORD="$$MNEMOIR_REDIS_PASSWORD" -- $(CURDIR)/$(BIN_DIR)/$(BINARY) --config $(CONFIG_DIR)/config.toml

mcp-codex: build
	@$(CURDIR)/scripts/install-mcp-codex.sh $(CURDIR)/$(BIN_DIR)/$(BINARY)

setup-codex: docker-up build
	@mkdir -p -m 700 $(CONFIG_DIR)
	@if [ ! -f $(CONFIG_DIR)/config.toml ]; then \
		cp config/default.toml $(CONFIG_DIR)/config.toml; \
		chmod 600 $(CONFIG_DIR)/config.toml; \
		echo "Config copied to $(CONFIG_DIR)/config.toml"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/config.toml"; \
	fi
	@if [ -z "$$MNEMOIR_REDIS_PASSWORD" ]; then \
		echo "WARNING: MNEMOIR_REDIS_PASSWORD is not set. Set it before starting Redis."; \
	fi
	@echo ""
	@echo "Registering MCP server (Codex CLI)..."
	@$(MAKE) --no-print-directory mcp-codex
	@echo ""
	@echo "Installing Stop hook (Codex CLI)..."
	@$(MAKE) --no-print-directory hook-codex
	@echo ""
	@echo "Installing agent specs (Codex CLI)..."
	@$(MAKE) --no-print-directory specs-codex
	@echo ""
	@echo "Setup complete. Edit $(CONFIG_DIR)/config.toml to customize."

backup:
	@$(CURDIR)/scripts/backup-native.sh "$(or $(OUTPUT),$(CONFIG_DIR)/backups/$(shell date +%Y%m%d))"

restore:
	@if [ -z "$(INPUT)" ]; then \
		echo "Usage: make restore INPUT=path/to/backup-dir"; \
		exit 1; \
	fi
	@$(CURDIR)/scripts/restore-native.sh $(INPUT)

backup-json: build
	$(BIN_DIR)/$(BINARY) backup --config $(CONFIG_DIR)/config.toml --output "$(or $(OUTPUT),$(CONFIG_DIR)/backups/$(shell date +%Y%m%d).json)"

restore-json: build
	@if [ -z "$(INPUT)" ]; then \
		echo "Usage: make restore-json INPUT=path/to/backup.json"; \
		exit 1; \
	fi
	$(BIN_DIR)/$(BINARY) restore --config $(CONFIG_DIR)/config.toml --input $(INPUT)

clean:
	rm -rf $(BIN_DIR)

clean-data: docker-down
	rm -rf data/
	@echo "Redis data wiped. Run 'make docker-up' to start fresh."

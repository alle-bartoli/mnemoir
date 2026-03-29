.PHONY: help build install uninstall test docker-up docker-down redis-ui setup mcp-register clean clean-data

BINARY := agentmem
BIN_DIR := bin
CMD_DIR := ./cmd/agentmem
CONFIG_DIR := $(HOME)/.agentmem
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
	@echo "  make setup         - Full setup (docker + build + config)"
	@echo "  make mcp-register  - Register MCP server with Claude Code"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make clean-data    - Stop Redis and wipe all stored memories (data/)"

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)

install:
	go install $(CMD_DIR)

uninstall:
	@echo "Removing binary from GOPATH..."
	rm -f $(shell go env GOPATH)/bin/$(BINARY)
	@echo "Removing MCP registration..."
	-claude mcp remove $(BINARY) 2>/dev/null
	@echo "Removing config directory $(CONFIG_DIR)..."
	rm -rf $(CONFIG_DIR)
	@echo "Removing build artifacts..."
	rm -rf $(BIN_DIR)
	@echo "agentmem uninstalled."

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
	@if [ -z "$$AGENTMEM_REDIS_PASSWORD" ]; then \
		echo "WARNING: AGENTMEM_REDIS_PASSWORD is not set. Set it before starting Redis."; \
	fi

mcp-register: build
	claude mcp add --transport stdio $(BINARY) -- $(CURDIR)/$(BIN_DIR)/$(BINARY) --config $(CONFIG_DIR)/config.toml

clean:
	rm -rf $(BIN_DIR)

clean-data: docker-down
	rm -rf data/
	@echo "Redis data wiped. Run 'make docker-up' to start fresh."

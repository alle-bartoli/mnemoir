.PHONY: build install test docker-up docker-down setup mcp-register clean

BINARY := agentmem
BIN_DIR := bin
CMD_DIR := ./cmd/agentmem
CONFIG_DIR := $(HOME)/.agentmem

build:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/$(BINARY) $(CMD_DIR)

install:
	go install $(CMD_DIR)

test:
	go test ./...

docker-up:
	docker compose up -d

docker-down:
	docker compose down

setup: docker-up build
	@mkdir -p $(CONFIG_DIR)
	@if [ ! -f $(CONFIG_DIR)/config.toml ]; then \
		cp config/default.toml $(CONFIG_DIR)/config.toml; \
		echo "Config copied to $(CONFIG_DIR)/config.toml"; \
	else \
		echo "Config already exists at $(CONFIG_DIR)/config.toml"; \
	fi

mcp-register: build
	claude mcp add --transport stdio $(BINARY) -- $(CURDIR)/$(BIN_DIR)/$(BINARY) --config $(CONFIG_DIR)/config.toml

clean:
	rm -rf $(BIN_DIR)

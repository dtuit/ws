.PHONY: all build test fmt install clean

GO ?= $(shell command -v go 2>/dev/null || echo /usr/local/go/bin/go)
BINARY := ws
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
OUTPUT ?= bin/$(BINARY)
INSTALL_DIR ?= $(HOME)/.local/bin
LDFLAGS := -X github.com/dtuit/ws/internal/version.version=$(VERSION) -X github.com/dtuit/ws/internal/version.commit=$(COMMIT) -X github.com/dtuit/ws/internal/version.date=$(DATE)

all: fmt test build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(OUTPUT) ./cmd/ws

test:
	$(GO) test -race -count=1 ./...

fmt:
	$(GO) fmt ./...

install: build
	install -d "$(INSTALL_DIR)"
	install -m 0755 "$(OUTPUT)" "$(INSTALL_DIR)/$(BINARY)"
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

clean:
	rm -rf bin/

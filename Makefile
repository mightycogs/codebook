.PHONY: build test lint clean install check help
.DEFAULT_GOAL := help

BINARY=codebase-memory-mcp
MODULE=github.com/mightycogs/codebase-memory-mcp

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: clean  ## Build binary to bin/
	go build -o bin/$(BINARY) ./cmd/codebase-memory-mcp/

test:  ## Run tests
	go test ./... -v

check: lint test  ## Run lint + tests

lint:  ## Run golangci-lint
	golangci-lint run --timeout=5m ./...

clean:  ## Remove build artifacts
	rm -rf bin/

install: build  ## Build and copy binary to ~/.local/bin/
	mkdir -p ~/.local/bin
	cp bin/$(BINARY) ~/.local/bin/$(BINARY)

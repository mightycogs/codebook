.PHONY: build test coverage report lint clean install check help
.DEFAULT_GOAL := help

BINARY=codebase-memory-mcp
MODULE=github.com/mightycogs/codebase-memory-mcp

PKG ?= ./...
COVER_OUT ?= coverage.out
COVER_MIN ?= 85

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

build: clean  ## Build binary to bin/
	go build -o bin/$(BINARY) ./cmd/codebase-memory-mcp/

test:  ## Run tests (PKG=./internal/pipeline/ VERBOSE=1)
	CGO_ENABLED=1 go test $(PKG) $(if $(VERBOSE),-v)

coverage:  ## Show coverage summary (PKG=... COVER_MIN=85)
	@CGO_ENABLED=1 go test $(PKG) -coverprofile=$(COVER_OUT) -count=1 $(if $(VERBOSE),-v)
	@echo ""
	@go tool cover -func=$(COVER_OUT) | tail -1
	@TOTAL=$$(go tool cover -func=$(COVER_OUT) | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	if awk -v got="$$TOTAL" -v min="$(COVER_MIN)" 'BEGIN { exit !(got < min) }'; then \
		echo "FAIL: coverage $$TOTAL% < $(COVER_MIN)% minimum"; exit 1; \
	else \
		echo "OK: coverage $$TOTAL% >= $(COVER_MIN)%"; \
	fi

report:  ## Generate JSON report + coverage (for CI)
	CGO_ENABLED=1 go test $(PKG) -coverprofile=$(COVER_OUT) -json > test-report.json
	go tool cover -func=$(COVER_OUT) > coverage.txt
	go tool cover -html=$(COVER_OUT) -o coverage.html

check: lint test  ## Run lint + tests

lint:  ## Run golangci-lint
	golangci-lint run --timeout=5m ./...

clean:  ## Remove build artifacts
	rm -rf bin/

install: build  ## Build and copy binary to ~/.local/bin/
	mkdir -p ~/.local/bin
	cp bin/$(BINARY) ~/.local/bin/$(BINARY)

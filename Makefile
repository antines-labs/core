GO ?= go
GOLANGCI_LINT ?= golangci-lint
GOFUMPT ?= gofumpt

BINARY ?= antines
OUT_DIR ?= dist

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "unknown")

LDFLAGS := -s -w
LDFLAGS += -X github.com/antines-labs/core/internal/version.Version=$(VERSION)
LDFLAGS += -X github.com/antines-labs/core/internal/version.Commit=$(COMMIT)
LDFLAGS += -X github.com/antines-labs/core/internal/version.Date=$(DATE)

.PHONY: build test lint vet fmt clean run version

build:
	@mkdir -p $(OUT_DIR)
	$(GO) build -ldflags="$(LDFLAGS)" -o $(OUT_DIR)/$(BINARY) ./cmd/antines

test:
	$(GO) test ./... -count=1 -race -shuffle=on -timeout=60s -v

lint:
	$(GOLANGCI_LINT) run ./...

vet:
	$(GO) vet ./...

fmt:
	$(GOFUMPT) -l -w .

clean:
	rm -rf $(OUT_DIR)

run: build
	./$(OUT_DIR)/$(BINARY)

version:
	@echo "$(VERSION) (commit $(COMMIT), built $(DATE))"

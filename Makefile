## === Research Tree — Makefile ===
SHELL := /bin/bash

# --- Toolchain ---
GO       ?= go
OS       ?= $(shell $(GO) env GOOS)
ARCH     ?= $(shell $(GO) env GOARCH)
BIN_EXT  :=
ifeq ($(OS),windows)
  BIN_EXT := .exe
endif

# --- Binary ---
BIN_DIR := ./build
BIN     := $(BIN_DIR)/rt$(BIN_EXT)
LIBSO   := $(BIN_DIR)/libretree.so
DLL_AMD := $(BIN_DIR)/libretree-amd64.dll
DLL_ARM := $(BIN_DIR)/libretree-arm64.dll

# --- Lint ---
GOLANGCI_LINT_VER ?= latest
LINT_TIMEOUT      ?= 3m

# --- Cache isolation (sandbox-safe builds) ---
export GOCACHE            := $(abspath ./.gocache)
export GOMODCACHE         := $(abspath ./.gomodcache)
export GOLANGCI_LINT_CACHE := $(abspath ./.golangci-cache)

# --- Build dependencies ---
GO_SOURCES := $(shell find . -type f -name "*.go" -not -path "./build/*" -not -path "./.gocache/*" -not -path "./.gomodcache/*" -not -path "./.golangci-cache/*" -print)

.PHONY: all build fmt vet tidy commentlint lint test test-race clean tools help

# --- Primary target ---
all: build

# --- Build (disciplined: runs all checks first) ---
build: fmt vet tidy commentlint lint $(BIN)
	@echo "✅ build complete"

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

$(BIN): $(GO_SOURCES) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	@$(GO) build -gcflags 'all=-e' -ldflags '-X github.com/frudas24/research-tree/cmd/rt/cmds.Version=$(VERSION)' -o $@ ./cmd/rt
	@echo "  binary: $@"

# --- Shared library Linux (C ABI bridge for FFI) ---
libretree.so: $(LIBSO)

$(LIBSO): $(GO_SOURCES) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	@CGO_ENABLED=1 $(GO) build -buildmode=c-shared -o $@ ./cmd/rt-bridge/
	@echo "  shared library: $@"

# --- Windows DLL (C ABI bridge for FFI) ---
dll: $(DLL_AMD)
	@echo "✅ dll complete"

$(DLL_AMD): $(GO_SOURCES) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	@CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 $(GO) build -buildmode=c-shared -o $@ ./cmd/rt-bridge/
	@echo "  dll: $@"

$(DLL_ARM): $(GO_SOURCES) go.mod go.sum
	@mkdir -p $(BIN_DIR)
	@CC=aarch64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=arm64 $(GO) build -buildmode=c-shared -o $@ ./cmd/rt-bridge/ 2>/dev/null || echo "  ⚠️  aarch64 MinGW not available — skipping arm64 DLL"
	@echo "  dll: $@"

# --- Quality pipeline (runs all checks, no build) ---
check: fmt vet tidy commentlint lint
	@echo "✅ all checks passed"

# --- Format ---
fmt:
	@echo "🧹 gofmt -s"
	@find . \
	  -path './build' -prune -o \
	  -path './.gocache' -prune -o \
	  -path './.gomodcache' -prune -o \
	  -path './.golangci-cache' -prune -o \
	  -path './third_party' -prune -o \
	  -name "*.go" -print0 | xargs -0 gofmt -s -w
	@echo "🧹 go fmt ./..."
	@$(GO) fmt ./...

# --- Static analysis ---
vet:
	@echo "🔍 go vet"
	@$(GO) vet ./...

# --- Module hygiene ---
tidy:
	@echo "📦 go mod tidy"
	@$(GO) mod tidy
	@$(GO) mod verify

# --- Lint ---
lint: commentlint
	@echo "🔎 golangci-lint"
	@GOLINT=$$(command -v golangci-lint 2>/dev/null || echo "$$(go env GOPATH)/bin/golangci-lint"); \
	if [ ! -x "$$GOLINT" ]; then \
	  echo "❌ golangci-lint not installed — run: make tools"; exit 1; \
	fi; \
	$$GOLINT run --timeout $(LINT_TIMEOUT) ./...

# --- Doc comment lint (always runs, uses in-tree tool) ---
commentlint:
	@echo "📝 commentlint (doc comments)"
	@$(GO) run ./third_party/commentlint ./...

# --- Install lint tool ---
tools:
	@echo "⬇️  installing golangci-lint@$(GOLANGCI_LINT_VER)"
	@$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VER)

# --- Tests ---
test:
	@echo "🧪 go test -count=1 ./..."
	@$(GO) test -count=1 ./...

test-race:
	@echo "🧪 go test -race -count=1 ./..."
	@$(GO) test -race -count=1 ./...

# --- Cleanup ---
clean:
	@echo "🗑  removing $(BIN_DIR)"
	@rm -rf $(BIN_DIR) $(LIBSO) $(LIBSO:.so=.h) $(DLL_AMD) $(DLL_ARM) $(DLL_AMD:.dll=.h) $(DLL_ARM:.dll=.h)

# --- Help ---
help:
	@echo "Research Tree — Makefile targets"
	@echo ""
	@echo "  make build      Run all checks + build binary"
	@echo "  make libretree.so Build C shared library for FFI (Linux)"
	@echo "  make dll        Cross-compile Windows DLL (amd64)"
	@echo "  make check      Run fmt + vet + tidy + commentlint + lint (no build)"
	@echo "  make fmt        Format all Go sources"
	@echo "  make vet        Static analysis"
	@echo "  make tidy       go mod tidy + verify"
	@echo "  make commentlint Enforce doc comments on all functions"
	@echo "  make lint       golangci-lint (requires installation)"
	@echo "  make test       Run all tests"
	@echo "  make test-race  Run all tests with race detector"
	@echo "  make tools      Install golangci-lint"
	@echo "  make clean      Remove build artifacts"

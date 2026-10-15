SHELL := /bin/bash

BIN := $(CURDIR)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Pinned tool versions. Check for the latest release before bumping.
GOLANGCI_LINT_VERSION := v2.14.0
GOFUMPT_VERSION := v0.12.0
GOVULNCHECK_VERSION := v1.9.0
GO_LICENSES_VERSION := v2.0.1
ZIZMOR_VERSION := 1.31.0

# Build tools with the toolchain go.mod targets. `go install pkg@version` otherwise
# picks the tool's own minimum Go, and tools built with an older Go refuse to
# analyze this module.
GO_TOOLCHAIN := go$(shell sed -n 's/^go //p' go.mod)
TOOL_INSTALL := GOTOOLCHAIN=$(GO_TOOLCHAIN) GOBIN=$(BIN) go install

ALLOWED_LICENSES := Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC

GOLANGCI_LINT := $(BIN)/golangci-lint
GOFUMPT := $(BIN)/gofumpt
GOVULNCHECK := $(BIN)/govulncheck
GO_LICENSES := $(BIN)/go-licenses

.PHONY: all build test lint fmt fmt-check vuln licenses zizmor check ci-local tools clean

all: check

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN)/riskinator ./cmd/riskinator

test:
	go test -race -count=1 ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

fmt: $(GOFUMPT)
	$(GOFUMPT) -w .

fmt-check: $(GOFUMPT)
	@out="$$($(GOFUMPT) -l .)"; if [ -n "$$out" ]; then echo "gofumpt needed:"; echo "$$out"; exit 1; fi

vuln: $(GOVULNCHECK)
	$(GOVULNCHECK) ./...

licenses: $(GO_LICENSES)
	$(GO_LICENSES) check ./... --allowed_licenses=$(ALLOWED_LICENSES)

zizmor:
	zizmor --offline .github/workflows

check: fmt-check lint test

# Run every CI job locally and write a provenance record (scripts/local-ci.sh).
ci-local:
	scripts/local-ci.sh

tools: $(GOLANGCI_LINT) $(GOFUMPT) $(GOVULNCHECK) $(GO_LICENSES)

$(GOLANGCI_LINT):
	$(TOOL_INSTALL) github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOFUMPT):
	$(TOOL_INSTALL) mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOVULNCHECK):
	$(TOOL_INSTALL) golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GO_LICENSES):
	$(TOOL_INSTALL) github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)

clean:
	rm -rf $(BIN)

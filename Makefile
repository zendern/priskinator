SHELL := /bin/bash

BIN := $(CURDIR)/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

# Pinned tool versions. Check for the latest release before bumping.
GOLANGCI_LINT_VERSION := v2.14.0
GOFUMPT_VERSION := v0.12.0
GOVULNCHECK_VERSION := v1.9.0
GO_LICENSES_VERSION := v2.0.1

ALLOWED_LICENSES := Apache-2.0,MIT,BSD-2-Clause,BSD-3-Clause,ISC

GOLANGCI_LINT := $(BIN)/golangci-lint
GOFUMPT := $(BIN)/gofumpt
GOVULNCHECK := $(BIN)/govulncheck
GO_LICENSES := $(BIN)/go-licenses

.PHONY: all build test lint fmt fmt-check vuln licenses zizmor check tools clean

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

tools: $(GOLANGCI_LINT) $(GOFUMPT) $(GOVULNCHECK) $(GO_LICENSES)

$(GOLANGCI_LINT):
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOFUMPT):
	GOBIN=$(BIN) go install mvdan.cc/gofumpt@$(GOFUMPT_VERSION)

$(GOVULNCHECK):
	GOBIN=$(BIN) go install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(GO_LICENSES):
	GOBIN=$(BIN) go install github.com/google/go-licenses/v2@$(GO_LICENSES_VERSION)

clean:
	rm -rf $(BIN)

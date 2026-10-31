VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/pipetower/pipetower-cli/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build install test lint fmt golden release-snapshot clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/pt ./cmd/pt

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/pt

test:
	go test -race ./...

GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0

lint:
	$(GOLANGCI_LINT) run ./...

fmt:
	$(GOLANGCI_LINT) fmt ./...

# Update the golden files after an intended change of a view, then review the diff
golden:
	go test ./... -update

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist

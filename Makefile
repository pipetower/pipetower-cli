# Local builds are development builds (dev-<git describe>), only releases offer updates
BUILD_VERSION ?= dev-$(shell git describe --tags --always --dirty 2>/dev/null || echo unknown)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
PKG     := github.com/pipetower/pipetower-cli/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(BUILD_VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build install test lint fmt golden release release-snapshot clean

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

# Tag a release on main and push the tag, the release workflow publishes it: make release VERSION=1.2.0
release:
	@echo "$(VERSION)" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$$' || \
		{ echo "Usage: make release VERSION=1.2.0 (a semantic version without v)"; exit 2; }
	@test "$$(git rev-parse --abbrev-ref HEAD)" = main || { echo "Release from main."; exit 1; }
	@test -z "$$(git status --porcelain)" || { echo "Commit or stash your changes first."; exit 1; }
	@git fetch --quiet --tags origin main
	@test "$$(git rev-parse HEAD)" = "$$(git rev-parse origin/main)" || { echo "main differs from origin/main, push or pull first."; exit 1; }
	@! git rev-parse --quiet --verify "refs/tags/v$(VERSION)" >/dev/null || { echo "v$(VERSION) exists already."; exit 1; }
	@latest=$$(git tag --list 'v*.*.*' --sort=-version:refname | head -n 1); \
		if [ -n "$$latest" ] && [ "$$(printf '%s\n%s\n' "$$latest" "v$(VERSION)" | sort -V | tail -n 1)" != "v$(VERSION)" ]; then \
			echo "v$(VERSION) is not newer than $$latest."; exit 1; fi
	git tag --annotate "v$(VERSION)" --message "pt $(VERSION)"
	git push origin "v$(VERSION)"

release-snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist

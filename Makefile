# Version is derived from the git tag — `git describe` gives v0.1.0 at the tag
# and v0.1.0-3-gabc1234 three commits later. There is no version constant to
# hand-edit; `git tag vX.Y.Z` is the single source of truth.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

.PHONY: install build test lint check version wire pin-checksums

# Install to $GOBIN/$GOPATH/bin with the version baked in.
install:
	go install -ldflags "$(LDFLAGS)" ./cmd/onsetter

# Build a local ./onsetter binary with the version baked in.
build:
	go build -ldflags "$(LDFLAGS)" -o onsetter ./cmd/onsetter

# Install AND wire. The settings edit is part of the install, not a step in the
# README for someone to skip — that is how a sibling project's whole hook set
# sat dark for months while its installer exited 0.
wire: install
	$(shell go env GOPATH)/bin/onsetter install

test:
	go test ./...

lint:
	golangci-lint run

check: test lint

version:
	@echo $(VERSION)

# Pin a published release's checksums.txt hash so the plugin's fetch.sh will
# trust it. Run after `gh release view vX.Y.Z` shows real assets.
# Usage: make pin-checksums VERSION=0.8.0
pin-checksums:
	./scripts/pin-checksums.sh $(VERSION)

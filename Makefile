GO ?= go
BINARY := wsl-netguard
MAIN := ./cmd/wsl-netguard
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/shanlvb/wsl-netguard/internal/version.Version=$(VERSION) \
	-X github.com/shanlvb/wsl-netguard/internal/version.Commit=$(COMMIT) \
	-X github.com/shanlvb/wsl-netguard/internal/version.Date=$(DATE)

.PHONY: build test release-snapshot clean

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(MAIN)

test:
	$(GO) test ./...

release-snapshot: clean
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_$(VERSION)_linux_amd64 $(MAIN)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)_$(VERSION)_linux_arm64 $(MAIN)
	cd dist && sha256sum $(BINARY)_$(VERSION)_linux_* > checksums.txt

clean:
	rm -rf bin dist

VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# Release tags may include v; runtime versions never do.
BUILD_VERSION := $(patsubst v%,%,$(VERSION))
LDFLAGS := -s -w -X github.com/vincentsch/tabslip/internal/version.Version=$(BUILD_VERSION) -X github.com/vincentsch/tabslip/internal/version.Commit=$(COMMIT)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
NATIVE_SUFFIX := $(if $(filter windows,$(shell go env GOOS)),.exe)

.PHONY: build build-all test race lint conformance docs docs-check clean

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/tabslip$(NATIVE_SUFFIX) ./cmd/tabslip

build-all:
	mkdir -p dist
	rm -f dist/tabslip-* dist/checksums.txt
	@set -e; for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		asset=tabslip-$$os-$$arch; \
		if [ "$$os" = windows ]; then asset=$$asset.exe; fi; \
		echo "Building $$asset"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -ldflags "$(LDFLAGS)" -o "dist/$$asset" ./cmd/tabslip; \
	done
	@cd dist && if command -v sha256sum >/dev/null 2>&1; then \
		sha256sum tabslip-* > checksums.txt; \
	else shasum -a 256 tabslip-* > checksums.txt; fi

test:
	go test ./...

race:
	go test -race ./...

lint:
	@test -z "$$(gofmt -l cmd internal scripts)" || { gofmt -l cmd internal scripts; exit 1; }
	go vet ./...

conformance:
	./scripts/conformance.sh

docs:
	go run ./cmd/generate-command-docs

docs-check:
	go run ./cmd/generate-command-docs --check

clean:
	rm -f bin/tabslip bin/tabslip.exe dist/tabslip-* dist/checksums.txt

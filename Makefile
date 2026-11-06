# storagebench — build and deployment helpers.
#
# The platform runs on Linux (SLES). This Makefile is meant to be run on a Linux
# build host or in CI. It cross-compiles static Linux binaries for amd64 by
# default; override GOOS/GOARCH as needed.

SHELL := /bin/bash

# Version metadata injected into the binaries.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0-dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

PKG     := github.com/storagebench/storagebench/internal/version
LDFLAGS := -s -w \
	-X '$(PKG).Version=$(VERSION)' \
	-X '$(PKG).Commit=$(COMMIT)' \
	-X '$(PKG).BuildDate=$(DATE)'

GOOS   ?= linux
GOARCH ?= amd64

# CGO is required by the sqlite3 driver; the controller build enables it.
# The agent has no CGO dependency and is built fully static.
BIN_DIR := bin

.PHONY: all tidy build build-agent build-broker build-controller build-controller-embed web-build test vet fmt clean deps-check

all: build

## tidy: resolve and pin module dependencies (writes go.sum)
tidy:
	go mod tidy

## build: build all binaries for the target platform
build: build-agent build-broker build-controller

## build-agent: static agent binary (no CGO)
build-agent:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/storagebench-agent ./cmd/agent

## build-broker: static privilege-broker binary (no CGO). Runs as root on each
## host and applies/reverts network faults via tc on behalf of the agent.
build-broker:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/storagebench-broker ./cmd/broker

## build-controller: controller binary (CGO on for sqlite3)
build-controller:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/storagebench-controller ./cmd/controller

## web-build: build the frontend bundle into cmd/controller/webdist.
## Uses `npm install` so an updated package.json always resolves (and refreshes
## package-lock.json, which CI commits back for reproducibility).
web-build:
	cd web && npm install && npm run build

## build-controller-embed: controller with the built frontend embedded.
## Run `make web-build` first so cmd/controller/webdist holds the real bundle.
build-controller-embed:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -tags embedui -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/storagebench-controller ./cmd/controller

## test: run unit tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format the code
fmt:
	gofmt -s -w .

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR)

## deps-check: verify go.mod/go.sum are tidy (CI gate)
deps-check:
	go mod verify

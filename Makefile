# ---- CI Image Config ----
CI_IMAGE := ghcr.io/rancher/ci-image/go1.26:latest
WORKDIR := /workspace

# Detect CI environment (common env var used by many CI systems)
CI ?= false

# Docker run wrapper (only used locally)
DOCKER_RUN = docker run --rm -i \
	-v $(PWD):$(WORKDIR) \
	-w $(WORKDIR) \
	$(CI_IMAGE)

# Command runner:
# - In CI: run commands directly
# - Locally: run via Docker
ifeq ($(CI),true)
	RUN =
else
	RUN = $(DOCKER_RUN)
endif

# ---- Build Config ----
BINARY  := rancher-deployer
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "\
	-X github.com/mallardduck/rancher-deployer/internal/version.Version=$(VERSION) \
	-X github.com/mallardduck/rancher-deployer/internal/version.Commit=$(COMMIT) \
	-X github.com/mallardduck/rancher-deployer/internal/version.Date=$(DATE) \
	-s -w"

# Build for the current platform (dev use)
.PHONY: build
build:
	$(RUN) go build $(LDFLAGS) -o $(BINARY) .

.PHONY: install
install:
	go install $(LDFLAGS) .

.PHONY: test
test:
	$(RUN) go test ./... -v

.PHONY: lint
lint:
	$(RUN) golangci-lint run ./...

.PHONY: tidy
tidy:
	$(RUN) go mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -rf dist/

.PHONY: deps
deps:
	$(RUN) go mod download

# Cut a release — requires a clean working tree and a pushed tag.
.PHONY: release
release:
	goreleaser release --clean

# Local snapshot build across all platforms without publishing (no tag required).
.PHONY: snapshot
snapshot:
	goreleaser release --snapshot --clean

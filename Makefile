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
	go build $(LDFLAGS) -o $(BINARY) .

.PHONY: install
install:
	go install $(LDFLAGS) .

.PHONY: test
test:
	go test ./... -v

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	rm -f $(BINARY)
	rm -rf dist/

.PHONY: deps
deps:
	go mod download

# Cut a release — requires a clean working tree and a pushed tag.
.PHONY: release
release:
	goreleaser release --clean

# Local snapshot build across all platforms without publishing (no tag required).
.PHONY: snapshot
snapshot:
	goreleaser release --snapshot --clean

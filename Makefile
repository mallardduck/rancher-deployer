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

# ---- Docker Config ----
IMAGE_PREFIX ?= rancher-deployer
IMAGE_TAG    ?= $(VERSION)
ARCH         ?= amd64

# Docker build arguments
DOCKER_BUILD_ARGS := --build-arg ARCH=$(ARCH)

# ---- Docker Build Targets ----
# Build k3s-base: minimal k3s-in-docker foundation
.PHONY: docker-build-k3s-base
docker-build-k3s-base:
	docker build $(DOCKER_BUILD_ARGS) \
		--target k3s-base \
		-t $(IMAGE_PREFIX)/k3s-base:$(IMAGE_TAG) \
		-f package/Dockerfile .

# Build k3s-tools: k3s-base + debugging/client tools (k9s, helm)
.PHONY: docker-build-k3s-tools
docker-build-k3s-tools:
	docker build $(DOCKER_BUILD_ARGS) \
		--target k3s-tools \
		-t $(IMAGE_PREFIX)/k3s-tools:$(IMAGE_TAG) \
		-f package/Dockerfile .

# Build rancher-deployer: full deployment stack
.PHONY: docker-build-rancher-deployer
docker-build-rancher-deployer:
	docker build $(DOCKER_BUILD_ARGS) \
		--target rancher-deployer \
		-t $(IMAGE_PREFIX)/rancher-deployer:$(IMAGE_TAG) \
		-f package/Dockerfile .

# Build all Docker images
.PHONY: docker-build-all
docker-build-all: docker-build-k3s-base docker-build-k3s-tools docker-build-rancher-deployer

# Default docker-build target (builds the full stack)
.PHONY: docker-build
docker-build: docker-build-rancher-deployer

# ---- Docker Push Targets ----
.PHONY: docker-push-k3s-base
docker-push-k3s-base: docker-build-k3s-base
	docker push $(IMAGE_PREFIX)/k3s-base:$(IMAGE_TAG)

.PHONY: docker-push-k3s-tools
docker-push-k3s-tools: docker-build-k3s-tools
	docker push $(IMAGE_PREFIX)/k3s-tools:$(IMAGE_TAG)

.PHONY: docker-push-rancher-deployer
docker-push-rancher-deployer: docker-build-rancher-deployer
	docker push $(IMAGE_PREFIX)/rancher-deployer:$(IMAGE_TAG)

.PHONY: docker-push-all
docker-push-all: docker-push-k3s-base docker-push-k3s-tools docker-push-rancher-deployer

# ---- Docker Run Helpers ----
.PHONY: docker-run-k3s-base
docker-run-k3s-base:
	docker run -d --privileged --name k3s-base $(IMAGE_PREFIX)/k3s-base:$(IMAGE_TAG)

.PHONY: docker-run-k3s-tools
docker-run-k3s-tools:
	docker run -d --privileged --name k3s-tools $(IMAGE_PREFIX)/k3s-tools:$(IMAGE_TAG)

.PHONY: docker-run-rancher-deployer
docker-run-rancher-deployer:
	docker run -d --privileged -p 80:80 -p 443:443 --name rancher-deployer $(IMAGE_PREFIX)/rancher-deployer:$(IMAGE_TAG)

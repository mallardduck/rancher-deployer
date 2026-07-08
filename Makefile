# Include logic that can be reused across projects.
include hack/make/build.mk

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
build: ## Build rancher-deployer binary for current platform
	$(RUN) go build $(LDFLAGS) -o $(BINARY) .

.PHONY: install
install: ## Install rancher-deployer binary to $GOPATH/bin
	go install $(LDFLAGS) .

.PHONY: test
test: ## Run Go tests
	$(RUN) go test ./... -v

.PHONY: lint
lint: ## Run golangci-lint
	$(RUN) golangci-lint run ./...

.PHONY: tidy
tidy: ## Tidy Go modules
	$(RUN) go mod tidy

.PHONY: clean
clean: ## Clean build artifacts
	rm -f $(BINARY)
	rm -rf dist/

.PHONY: deps
deps: ## Download Go dependencies
	$(RUN) go mod download

# Cut a release — requires a clean working tree and a pushed tag.
.PHONY: release
release: ## Cut a release with GoReleaser (requires clean tree and pushed tag)
	goreleaser release --clean

# Local snapshot build across all platforms without publishing (no tag required).
.PHONY: snapshot
snapshot: ## Build snapshot release locally without publishing
	goreleaser release --snapshot --clean

# ---- Docker Config ----
IMAGE_PREFIX ?= rancher-deployer
IMAGE_TAG    ?= $(VERSION)
TARGETARCH   ?= amd64

# Docker build arguments for local single-arch builds
# (buildx auto-injects TARGETARCH, but plain 'docker build' doesn't)
DOCKER_BUILD_ARGS := --build-arg TARGETARCH=$(TARGETARCH)

# ---- Docker Build Targets (local single-arch builds) ----
# For multi-arch builds, use: make build-image
# For local development, these build for a single architecture (default: amd64)

# Build k3s-base: minimal k3s-in-docker foundation
.PHONY: docker-build-k3s-base
docker-build-k3s-base: ## Build k3s-base image (minimal k3s-in-docker foundation)
	docker build $(DOCKER_BUILD_ARGS) \
		--target k3s-base \
		-t $(IMAGE_PREFIX)/k3s-base:$(IMAGE_TAG) \
		-f package/Dockerfile .

# Build k3s-tools: k3s-base + helm and debug scripts
.PHONY: docker-build-k3s-tools
docker-build-k3s-tools: ## Build k3s-tools image (k3s-base + helm, debug scripts)
	docker build $(DOCKER_BUILD_ARGS) \
		--target k3s-tools \
		-t $(IMAGE_PREFIX)/k3s-tools:$(IMAGE_TAG) \
		-f package/Dockerfile .

# Build rancher-demo: full deployment stack
.PHONY: docker-build-rancher-demo
docker-build-rancher-demo: ## Build rancher-demo image (full deployment stack, single-arch)
	docker build $(DOCKER_BUILD_ARGS) \
		--target rancher-demo \
		-t $(IMAGE_PREFIX)/rancher-demo:$(IMAGE_TAG) \
		-f package/Dockerfile .

# Build all Docker images
.PHONY: docker-build-all
docker-build-all: docker-build-k3s-base docker-build-k3s-tools docker-build-rancher-demo ## Build all Docker images (single-arch)

# Default docker-build target (builds the full stack)
.PHONY: docker-build
docker-build: docker-build-rancher-demo ## Build rancher-demo Docker image (alias for docker-build-rancher-demo)

# ---- Docker Buildx Targets (multi-arch builds) ----
# These use buildx and don't need TARGETARCH passed (auto-injected per platform)
# Note: --load only works with single platform; use --push for multi-platform or remove --load
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: docker-buildx-rancher-demo
docker-buildx-rancher-demo:
	docker buildx build \
		--platform $(PLATFORMS) \
		--target rancher-demo \
		-t $(IMAGE_PREFIX)/rancher-demo:$(IMAGE_TAG) \
		-f package/Dockerfile \
		.

# ---- Buildx Machine Pattern (Rancher ecosystem style) ----

.PHONY: build-image
build-image: buildx-machine ## Build (and load) the container image targeting the current platform
	$(IMAGE_BUILDER) build -f package/Dockerfile \
		--builder $(MACHINE) $(IMAGE_ARGS) \
		--target rancher-demo \
		--platform=$(TARGET_PLATFORMS) \
		-t "$(FULL_IMAGE_TAG)" $(BUILD_ACTION) .
	@echo "Built $(FULL_IMAGE_TAG)"

.PHONY: build-image-single
build-image-single: ## Build single-arch image for current platform (faster, loads into docker)
	@$(MAKE) build-image TARGET_PLATFORMS=linux/$$(go env GOHOSTARCH) BUILD_ACTION=--load

.PHONY: build-image-push
build-image-push: buildx-machine ## Build and push multi-platform image to registry
	@$(MAKE) build-image BUILD_ACTION=--push

# ---- Docker Push Targets ----
.PHONY: docker-push-k3s-base
docker-push-k3s-base: docker-build-k3s-base ## Build and push k3s-base image
	docker push $(IMAGE_PREFIX)/k3s-base:$(IMAGE_TAG)

.PHONY: docker-push-k3s-tools
docker-push-k3s-tools: docker-build-k3s-tools ## Build and push k3s-tools image
	docker push $(IMAGE_PREFIX)/k3s-tools:$(IMAGE_TAG)

.PHONY: docker-push-rancher-demo
docker-push-rancher-demo: docker-build-rancher-demo ## Build and push rancher-demo image (single-arch)
	docker push $(IMAGE_PREFIX)/rancher-demo:$(IMAGE_TAG)

.PHONY: docker-push-all
docker-push-all: docker-push-k3s-base docker-push-k3s-tools docker-push-rancher-demo ## Build and push all Docker images

# ---- Docker Run Helpers ----
.PHONY: docker-run-k3s-base
docker-run-k3s-base: ## Run k3s-base container in privileged mode
	docker run -d --privileged --name k3s-base $(IMAGE_PREFIX)/k3s-base:$(IMAGE_TAG)

.PHONY: docker-run-k3s-tools
docker-run-k3s-tools: ## Run k3s-tools container in privileged mode
	docker run -d --privileged --name k3s-tools $(IMAGE_PREFIX)/k3s-tools:$(IMAGE_TAG)

.PHONY: docker-run-rancher-demo
docker-run-rancher-demo: docker-buildx-rancher-demo ## Run rancher-demo container with exposed ports 80,443
	docker run -d --privileged \
		-p 80:80 -p 443:443 -p 8080:30080 -p 8443:30443 -p 6443:6443 \
		--name rancher-demo $(IMAGE_PREFIX)/rancher-demo:$(IMAGE_TAG)

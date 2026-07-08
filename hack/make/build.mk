# Build configuration for rancher-deployer
# This file contains reusable build logic for Docker images

ifeq ($(VERSION),)
VERSION := $(shell ./scripts/version --short VERSION)
endif

ifeq ($(TAG),)
TAG := $(shell ./scripts/version --short TAG)
endif

RUNNER := docker
IMAGE_BUILDER := $(RUNNER) buildx
MACHINE := rancher

# Define the target platforms that can be used across the ecosystem.
# Note that what would actually be used for a given project will be
# defined in TARGET_PLATFORMS, and must be a subset of the below:
DEFAULT_PLATFORMS := linux/amd64,linux/arm64
TARGET_PLATFORMS ?= $(DEFAULT_PLATFORMS)

# Image configuration
REPO ?= ghcr.io/mallardduck
IMAGE_NAME ?= rancher-demo
FULL_IMAGE_TAG := $(REPO)/$(IMAGE_NAME):$(TAG)

# Build action: --load for local, --push for registry
BUILD_ACTION ?= --load

.PHONY: help
help: ## Display Makefile's help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: buildx-machine
buildx-machine: ## Create rancher docker buildx machine targeting platforms defined by DEFAULT_PLATFORMS
	@docker buildx ls | grep $(MACHINE) || \
		docker buildx create --name=$(MACHINE) --platform=$(DEFAULT_PLATFORMS)

.PHONY: version
version: ## Print version information
	@./scripts/version

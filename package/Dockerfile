# Unified Dockerfile with multiple build targets
# Build targets:
#   - k3s-base: Minimal k3s-in-docker foundation
#   - k3s-tools: k3s-base + helm and debug scripts
#   - rancher-demo: Full deployment stack

# ============================================================================
# Stage: Go builder for rancher-deployer binary
# ============================================================================
FROM rancher/hardened-build-base:v1.26.4b1 AS go-builder
# TARGETARCH is automatically provided by buildx for multi-platform builds
ARG TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -ldflags="-w -s" -o rancher-deployer .

# ============================================================================
# Stage: BCI micro base
# ============================================================================
FROM registry.suse.com/bci/bci-micro:15.7 AS final
RUN : # No-op to create layer (buildkit bug workaround)

# ============================================================================
# Stage: Install core packages via zypper
# ============================================================================
FROM registry.suse.com/bci/bci-base:15.7 AS chroot-builder
COPY --from=final / /chroot/
RUN zypper refresh && \
    zypper --installroot /chroot -n in --no-recommends \
    curl util-linux ca-certificates ca-certificates-mozilla xz gzip tar gawk vim-small \
    openssh-clients openssl && \
    zypper --installroot /chroot clean -a && \
    rm -rf /chroot/var/cache/zypp/* /chroot/var/log/zypp/* /chroot/tmp/* /chroot/var/tmp/* /chroot/usr/share/doc/packages/*

# ============================================================================
# Stage: Server-specific packages
# ============================================================================
FROM chroot-builder AS chroot-builder-server
COPY --from=final / /chroot/
RUN zypper refresh && \
    zypper --installroot /chroot -n in --no-recommends \
    git-core unzip sed shadow netcat-openbsd mkisofs && \
    zypper --installroot /chroot clean -a && \
    rm -rf /chroot/var/cache/zypp/* /chroot/var/log/zypp/* /chroot/tmp/* /chroot/var/tmp/* /chroot/usr/share/doc/packages/*

# ============================================================================
# Stage: Base image with packages
# ============================================================================
FROM final AS base
COPY --from=chroot-builder-server /chroot/ /

# Verify required utilities are available
RUN /usr/bin/unshare --version && \
    /usr/bin/mount --version && \
    /usr/bin/umount --version && \
    /usr/bin/nsenter --version

# Create kubeconfig symlinks
RUN mkdir -p /root/.kube && \
    ln -s /etc/rancher/k3s/k3s.yaml /root/.kube/k3s.yaml  && \
    ln -s /etc/rancher/k3s/k3s.yaml /root/.kube/config

# Set working directory
WORKDIR /var/lib/rancher

# Define persistent volumes
VOLUME /var/lib/rancher
VOLUME /var/lib/kubelet
VOLUME /var/lib/cni
VOLUME /var/log

# ============================================================================
# Target 1: k3s-base - Minimal k3s runtime foundation
# ============================================================================
FROM base AS k3s-base

# K3s layer defaults (runtime configurable)
ENV CATTLE_K3S_VERSION=v1.33.12+k3s1
ENV KUBECONFIG=/etc/rancher/k3s/k3s.yaml
ENV HOSTNAME=k3s-local
ENV CONTAINER=true

# Copy k3s entrypoint
COPY package/k3s-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose Kubernetes API port
EXPOSE 6443

ENTRYPOINT ["/entrypoint.sh"]

# ============================================================================
# Target 2: k3s-tools - k3s-base + helm and debug scripts
# ============================================================================
FROM k3s-base AS k3s-tools

# TARGETARCH must be redeclared per build stage (ARG scope is stage-local)
ARG TARGETARCH

# Override hostname for tools layer
ENV HOSTNAME=k3s-tools

# Install helm for chart management
ARG HELM_VERSION=v4.2.2
RUN curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-${TARGETARCH}.tar.gz" \
    | tar -xz -C /usr/local/bin --strip-components=1 --wildcards '*/helm' && \
    chmod +x /usr/local/bin/helm

# Add debug scripts
COPY package/debug-cert-manager.sh /usr/local/bin/debug-cert-manager
COPY package/debug-rancher.sh /usr/local/bin/debug-rancher
RUN chmod +x /usr/local/bin/debug-cert-manager /usr/local/bin/debug-rancher

# Expose Kubernetes API port (inherited from k3s-base, but documented here)
EXPOSE 6443

# Inherit k3s-base entrypoint
# ENTRYPOINT is inherited

# ============================================================================
# Target 3: rancher-demo - Full rancher demo deployment stack
# ============================================================================
FROM k3s-tools AS rancher-demo

# Copy rancher-deployer binary from builder
COPY --from=go-builder /build/rancher-deployer /usr/local/bin/rancher-deployer
RUN chmod +x /usr/local/bin/rancher-deployer

# Rancher layer defaults (can be overridden)
ENV RANCHER_VERSION=2.14.2
ENV CATTLE_K3S_VERSION=""
ENV HOSTNAME=rancher-local
ENV CATTLE_NAMESPACE=cattle-system

# Copy rancher-deployer wrapper entrypoint
COPY package/rancher-entrypoint.sh /rancher-entrypoint.sh
RUN chmod +x /rancher-entrypoint.sh

# Expose Rancher ports:
# - 80/443: Traefik ingress (hostname-based access)
# - 8080/8443: NodePort direct access (any hostname/IP)
# - 6443: Kubernetes API
EXPOSE 80 443 8080 8443 6443

# Override entrypoint to run deployment logic
ENTRYPOINT ["/rancher-entrypoint.sh"]

# ============================================================================
# Build Instructions:
# ============================================================================
# Build specific targets:
#   docker build --target=k3s-base -t k3s-base:latest .
#   docker build --target=k3s-tools -t k3s-tools:latest .
#   docker build --target=rancher-demo -t rancher-demo:latest .
#
# Multi-platform builds (TARGETARCH auto-injected by buildx):
#   docker buildx build --platform=linux/amd64,linux/arm64 --target=rancher-demo -t rancher-demo:latest .
#
# Single architecture (specify TARGETARCH manually for non-buildx):
#   docker build --build-arg TARGETARCH=arm64 --target=rancher-demo -t rancher-demo:latest .
#
# Run targets:
#   docker run -d --privileged k3s-base:latest
#   docker run -d --privileged k3s-tools:latest
#   docker run -d --privileged -p 80:80 -p 443:443 rancher-demo:latest

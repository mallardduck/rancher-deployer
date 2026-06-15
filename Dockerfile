# Dockerfile for rancher-demo - Option 2 (Direct k3s, no DinD)
# This image runs rancher-deployer in container mode to deploy Rancher as a pod in k3s

# Build stage: compile rancher-deployer binary
FROM rancher/hardened-build-base:v1.26.4b1 AS builder

WORKDIR /build

# Copy go modules first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary (CGO_ENABLED=0 for portability)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o rancher-deployer .

# Runtime stage: SUSE BCI base (NO Docker daemon)
FROM registry.suse.com/bci/bci-base:16.0

# Install runtime dependencies (curl, bash, ca-certs, util-linux for nsenter/unshare)
RUN zypper --non-interactive install --no-recommends \
    curl \
    bash \
    ca-certificates \
    ca-certificates-mozilla \
    util-linux \
    && zypper clean --all

# Install kubectl from official Kubernetes release
ARG KUBECTL_VERSION=v1.36.0
ARG KUBECTL_CHECKSUM_amd64=123d8c8844f46b1244c547fffb3c17180c0c26dac9890589fe7e67763298748e
RUN curl -fsSL "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" -o /usr/bin/kubectl && \
    echo "${KUBECTL_CHECKSUM_amd64}  /usr/bin/kubectl" | sha256sum -c - && \
    chmod +x /usr/bin/kubectl

# Install helm from official Helm release
ARG HELM_VERSION=v3.16.4
RUN curl -fsSL "https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz" | \
    tar xz -C /tmp && \
    mv /tmp/linux-amd64/helm /usr/bin/helm && \
    chmod +x /usr/bin/helm && \
    rm -rf /tmp/linux-amd64

# Copy compiled binary from build stage
COPY --from=builder /build/rancher-deployer /usr/local/bin/rancher-deployer
RUN chmod +x /usr/local/bin/rancher-deployer

# Create kubeconfig symlink directory (matches current Rancher setup)
RUN mkdir -p /root/.kube && \
    ln -s /etc/rancher/k3s/k3s.yaml /root/.kube/config

# k3s binary will be dynamically fetched by rancher-deployer at runtime
# Rancher KDM dynamically fetched from releases.rancher.com
# Rancher deployed via Helm (fetched at runtime)

# Expose Rancher ingress ports
EXPOSE 80 443

# Volumes - match current rancher/rancher for compatibility
VOLUME /var/lib/rancher
VOLUME /var/lib/kubelet
VOLUME /var/lib/cni
VOLUME /var/log

# Copy entrypoint script
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Set working directory
WORKDIR /var/lib/rancher

# Default Rancher version (can be overridden via ENV)
ENV RANCHER_VERSION=2.8.5

# Entrypoint handles privileged mode check, cgroup setup, and rancher-deployer execution
ENTRYPOINT ["/entrypoint.sh"]
CMD ["--rancher-version", "2.8.5"]

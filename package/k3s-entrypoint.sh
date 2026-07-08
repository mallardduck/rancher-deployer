#!/bin/bash
# Minimal k3s-in-docker entrypoint

set -e

# Privilege check
if [ ! -e /run/secrets/kubernetes.io/serviceaccount ] && [ ! -e /dev/kmsg ]; then
    echo "ERROR: Rancher must be run with the --privileged flag when running outside of Kubernetes"
    exit 1
fi

#########################################################################################################################################
# DISCLAIMER                                                                                                                            #
# Copied from https://github.com/moby/moby/blob/ed89041433a031cafc0a0f19cfe573c31688d377/hack/dind#L28-L37                              #
# Permission granted by Akihiro Suda <akihiro.suda.cz@hco.ntt.co.jp> (https://github.com/rancher/k3d/issues/493#issuecomment-827405962) #
# Moby License Apache 2.0: https://github.com/moby/moby/blob/ed89041433a031cafc0a0f19cfe573c31688d377/LICENSE                           #
#########################################################################################################################################
# Setup cgroups for k3s
if [ ! -e /run/secrets/kubernetes.io/serviceaccount ] && [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  mkdir -p /sys/fs/cgroup/init
  xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || :
  sed -e 's/ / +/g' -e 's/^/+/' <"/sys/fs/cgroup/cgroup.controllers" >"/sys/fs/cgroup/cgroup.subtree_control"
fi

# Update CA certificates
if [ -x "$(command -v update-ca-certificates)" ]; then
  update-ca-certificates
fi
if [ -x "$(command -v c_rehash)" ]; then
  c_rehash
fi

echo "=== K3s Runtime Installation ==="
echo "K3s version: ${CATTLE_K3S_VERSION}"

# Detect architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64) ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    armv7l) ARCH="arm" ;;
    s390x) ARCH="s390x" ;;
    *) echo "ERROR: Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Download k3s binary if not present (runtime version selection)
if [ ! -f /usr/local/bin/k3s ]; then
    echo "Downloading k3s ${CATTLE_K3S_VERSION} for ${ARCH}..."

    K3S_BINARY="k3s"
    if [ "$ARCH" != "amd64" ]; then
        K3S_BINARY="k3s-${ARCH}"
    fi

    curl -sfL "https://github.com/k3s-io/k3s/releases/download/${CATTLE_K3S_VERSION}/${K3S_BINARY}" -o /usr/local/bin/k3s
    if [ $? -ne 0 ]; then
        echo "ERROR: Failed to download k3s ${CATTLE_K3S_VERSION}"
        exit 1
    fi
    chmod +x /usr/local/bin/k3s
    echo "k3s binary downloaded successfully"
fi

# Create k3s symlinks
echo "Creating k3s symlinks..."
ln -sf /usr/local/bin/k3s /usr/local/bin/kubectl
ln -sf /usr/local/bin/k3s /usr/local/bin/crictl
ln -sf /usr/local/bin/k3s /usr/local/bin/ctr
ln -sf /usr/local/bin/k3s /usr/local/bin/k3s-server
ln -sf /usr/local/bin/k3s /usr/local/bin/k3s-agent
ln -sf /usr/local/bin/k3s /usr/local/bin/k3s-etcd-snapshot

# Create necessary directories
mkdir -p /etc/rancher/k3s
mkdir -p /var/lib/rancher/k3s/server
mkdir -p /var/lib/kubelet
mkdir -p /var/lib/cni
mkdir -p /var/log
mkdir -p /run/k3s

# Link etcd if needed
if [ -e /var/lib/rancher/management-state/etcd ] && [ ! -e /var/lib/rancher/k3s/server/db/etcd ]; then
  mkdir -p /var/lib/rancher/k3s/server/db
  ln -sf /var/lib/rancher/management-state/etcd /var/lib/rancher/k3s/server/db/etcd
  echo -n 'default' > /var/lib/rancher/k3s/server/db/etcd/name
fi

# Reset etcd if needed
if [ -e /var/lib/rancher/k3s/server/db/etcd ]; then
  echo "INFO: Running k3s server --cluster-init --cluster-reset"
  set +e
  k3s server --cluster-init --cluster-reset &> /var/log/k3s-cluster-reset.log
  K3S_CR_CODE=$?
  if [ "${K3S_CR_CODE}" -ne 0 ]; then
    echo "ERROR:" && cat /var/log/k3s-cluster-reset.log
    rm -f /var/lib/rancher/k3s/server/db/reset-flag
    exit ${K3S_CR_CODE}
  fi
  set -e
fi

echo "=== Starting k3s ==="

# Start k3s server
# Traefik is enabled by default (needed for Rancher ingress)
exec k3s server \
  --write-kubeconfig-mode=644 \
  "$@"

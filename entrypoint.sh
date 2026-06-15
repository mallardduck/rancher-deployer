#!/bin/bash
# Entrypoint for rancher-demo container
# Adapted from rancher/rancher package/entrypoint.sh

set -e

# Check for --privileged mode (required for k3s to run)
# Copied from current Rancher Docker mode entrypoint.sh
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
# Setup cgroups for k3s to run inside container (cgroup v2)
# Only run this if rancher-demo is not running in a Kubernetes cluster
if [ ! -e /run/secrets/kubernetes.io/serviceaccount ] && [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  # Move the processes from the root group to the /init group,
  # otherwise writing subtree_control fails with EBUSY.
  mkdir -p /sys/fs/cgroup/init
  xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || :
  # Enable controllers
  sed -e 's/ / +/g' -e 's/^/+/' <"/sys/fs/cgroup/cgroup.controllers" >"/sys/fs/cgroup/cgroup.subtree_control"
fi

# Update CA certificates if available
if [ -x "$(command -v update-ca-certificates)" ]; then
  update-ca-certificates
fi
if [ -x "$(command -v c_rehash)" ]; then
  c_rehash
fi

# Run rancher-deployer in k3s mode
# It will:
# 1. Download k3s binary if not present
# 2. Start k3s server
# 3. Wait for k3s to be ready
# 4. Deploy cert-manager
# 5. Deploy Rancher via Helm
# 6. Wait for Rancher to be ready

echo "Starting rancher-deployer in container mode..."
echo "Rancher version: ${RANCHER_VERSION}"

# Execute rancher-deployer with container mode flag
# Use exec to replace the shell process (proper signal handling)
exec rancher-deployer deploy \
  --mode k3s \
  --rancher-version "${RANCHER_VERSION}" \
  --yes \
  "$@"

# Examples

```bash
# Minimal — everything auto-resolved
rancher-deployer deploy --rancher-version 2.8.5

# Rancher Prime, pinned to k8s 1.27, force k3d
rancher-deployer deploy \
  --rancher-version 2.8.5 \
  --prime \
  --k8s-version 1.27 \
  --mode k3d

# Custom hostname + values file + individual overrides
rancher-deployer deploy \
  --rancher-version 2.8.5 \
  --hostname rancher.example.com \
  --values-file ./production-values.yaml \
  --set replicas=3 \
  --set auditLog.level=1 \
  --set ingress.tls.source=letsEncrypt \
  --set letsEncrypt.email=admin@example.com

# Publish through a Pangolin tunnel (see docs/deployer/expose.md)
rancher-deployer deploy \
  --rancher-version 2.12 \
  --expose pangolin \
  --hostname rancher.example.com

# Dry run — see the full plan before touching anything
rancher-deployer deploy --rancher-version 2.8.5 --dry-run

# Just check what versions would be used
rancher-deployer resolve --rancher-version 2.8.5
rancher-deployer resolve --rancher-version 2.8.5 --k8s-version 1.27

# Upgrade to a new version
rancher-deployer upgrade --rancher-version 2.8.6

# Upgrade with dry-run to see the plan first
rancher-deployer upgrade --rancher-version 2.9.0 --dry-run

# Clean up everything
rancher-deployer teardown
rancher-deployer teardown --yes  # skip confirmation
```

# Idempotency & Cleanup

The `deploy` command is intentionally **not idempotent** — it **fails loudly** if:

- A k3d cluster with the given name already exists
- k3s is already running on the node
- The `cert-manager` namespace exists
- A `rancher` Helm release exists in the target namespace

This is by design: partial state is dangerous, and silent re-runs can mask problems.

**To clean up a failed or unwanted deployment, use:**

```bash
rancher-deployer teardown
```

Or manually remove individual components:

```bash
# Remove k3d cluster
k3d cluster delete rancher-local

# Uninstall k3s
sudo /usr/local/bin/k3s-uninstall.sh

# Remove Rancher Helm release
helm uninstall rancher -n cattle-system

# Remove cert-manager
kubectl delete namespace cert-manager
```

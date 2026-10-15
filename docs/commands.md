# Commands

## `deploy` — full install

```
rancher-deployer deploy --rancher-version <version> [flags]
```

| Flag                     | Default           | Description |
|--------------------------|-------------------|-------------|
| `--rancher-version`      | *(required)*      | Rancher version, e.g. `2.8.5` or `v2.8.5` |
| `--k8s-version`          | *(auto)*          | Target k8s major.minor, e.g. `1.28` |
| `--prime`                | `false`           | Use Rancher Prime instead of community edition |
| `--mode`                 | *(auto-detect)*   | Force `k3s` or `k3d` |
| `--hostname`             | `<ip>.sslip.io`   | Rancher ingress hostname |
| `--namespace`            | `cattle-system`   | Kubernetes namespace |
| `--values-file`          | *(none)*          | Path to Helm values YAML |
| `--set key=value`        | *(none)*          | Set individual Helm values (repeatable) |
| `--cluster-name`         | `rancher-local`   | k3d cluster name (k3d mode only) |
| `--dry-run`              | `false`           | Print plan without executing |

## `resolve` — inspect resolved versions (no install)

```
rancher-deployer resolve --rancher-version <version> [--k8s-version <minor>] [--mode k3s|k3d]
```

Useful for scripting or verifying what versions would be selected before committing to a full deploy.

## `upgrade` — upgrade an existing Rancher installation

```
rancher-deployer upgrade --rancher-version <version> [flags]
```

| Flag                     | Default           | Description |
|--------------------------|-------------------|-------------|
| `--rancher-version`      | *(required)*      | Target Rancher version to upgrade to |
| `--namespace`            | `cattle-system`   | Namespace where Rancher is installed |
| `--prime`                | `false`           | Use Rancher Prime chart |
| `--channel`              | `stable`          | Release channel: `stable` (GA), `latest` (RC), `alpha` |
| `--values-file`          | *(none)*          | Path to Helm values YAML |
| `--set key=value`        | *(none)*          | Override Helm chart value (repeatable) |
| `--dry-run`              | `false`           | Print resolved plan without executing |
| `--yes`                  | `false`           | Skip confirmation prompt |
| `--force`                | `false`           | Skip upgrade-path and k8s-compatibility checks |

**Upgrade validation:**
- Prevents downgrades
- Prevents skipping minor versions (must upgrade one minor at a time)
- Validates cluster k8s version is compatible with target Rancher version
- Use `--force` to bypass checks (e.g., for pre-release testing)

## `teardown` — remove a Rancher deployment and cluster

```
rancher-deployer teardown [flags]
```

| Flag                     | Default           | Description |
|--------------------------|-------------------|-------------|
| `--mode`                 | *(auto-detect)*   | Force `k3s` or `k3d` |
| `--cluster-name`         | `rancher-local`   | k3d cluster name (k3d mode only) |
| `--namespace`            | `cattle-system`   | Kubernetes namespace Rancher was installed into |
| `--yes`                  | `false`           | Skip confirmation prompt |

**Warning:** This is destructive and irreversible. It will:
1. Uninstall the Rancher Helm release
2. Remove cert-manager
3. Delete the entire k3s/k3d cluster

## `self-update` — update the binary itself

```
rancher-deployer self-update [flags]
```

| Flag        | Default | Description |
|-------------|---------|-------------|
| `--check`   | `false` | Only check for an update; don't prompt or apply |
| `--yes, -y` | `false` | Skip confirmation prompt |

Checks GitHub for a newer release of `rancher-deployer` and, on confirmation, replaces
the running binary with it. Installs managed by a package manager (Homebrew, apt,
Scoop) or otherwise not writable by the current user are detected and skipped, with
guidance on how to update through that channel instead.

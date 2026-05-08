# rancher-deployer

A Go CLI that deploys Rancher on top of k3s or k3d with automatic version resolution.
Single binary, no runtime dependencies beyond the target tooling (`helm`, `kubectl`, `k3s`/`k3d`).

---

## How it works

```
rancher-deployer deploy --rancher-version 2.8.5
```

1. **Fetches Rancher's KDM** (Kontainer Driver Metadata) for the requested Rancher minor version from `releases.rancher.com`, with a GitHub fallback.
2. **Resolves a compatible k8s version** — latest patch in the newest supported minor, or validates the `--k8s-version` you provide against the support matrix.
3. **Resolves a k3s version** — finds the latest stable k3s release tag on GitHub for that k8s version.
4. **Auto-detects install mode**:
   - macOS → **k3d** (k3s doesn't run natively)
   - Linux + Docker/Podman → **k3d**
   - Linux bare → **k3s**
   - Override with `--mode k3s` or `--mode k3d`
5. **Installs the cluster** (fails loud if already present).
6. **Installs cert-manager** (fails loud if already present).
7. **Deploys Rancher via Helm**, merging your `--values-file` and `--set` flags.
8. **Waits for Rancher** to become ready and prints the access URL.

---

## Installation

### From source (requires Go 1.25+)

```bash
git clone https://github.com/mallardduck/rancher-deployer
cd rancher-deployer
make build          # produces ./rancher-deployer
make install        # installs to $GOPATH/bin
```

### Cross-compile

```bash
make build-all      # builds linux-amd64, linux-arm64, darwin-amd64, darwin-arm64
ls dist/
```

---

## Prerequisites

The binary shells out to these tools — they must be in `PATH` on the target node:

| Tool      | Required for        |
|-----------|---------------------|
| `helm`    | Always              |
| `kubectl` | Always              |
| `k3d`     | k3d mode (auto-installed if missing) |
| `k3s`     | k3s mode (installed via official script) |
| `curl`    | k3s install script  |

---

## Commands

### `deploy` — full install

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

### `resolve` — inspect resolved versions (no install)

```
rancher-deployer resolve --rancher-version <version> [--k8s-version <minor>] [--mode k3s|k3d]
```

Useful for scripting or verifying what versions would be selected before committing to a full deploy.

---

## Examples

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

# Dry run — see the full plan before touching anything
rancher-deployer deploy --rancher-version 2.8.5 --dry-run

# Just check what versions would be used
rancher-deployer resolve --rancher-version 2.8.5
rancher-deployer resolve --rancher-version 2.8.5 --k8s-version 1.27
```

---

## Helm values

Values are applied in this order (last wins):

1. Rancher chart defaults
2. `--values-file <file>` — your base values YAML
3. `--set key=value` flags — individual overrides

The `hostname` value is always injected automatically unless you set it yourself via `--hostname` or `--set hostname=...`.

**Example `values.yaml`:**

```yaml
replicas: 3
bootstrapPassword: "change-me-on-first-login"
auditLog:
  level: 1
ingress:
  tls:
    source: letsEncrypt
letsEncrypt:
  email: admin@example.com
  environment: production
```

---

## Idempotency

The tool is intentionally **not idempotent** — it **fails loudly** if:

- A k3d cluster with the given name already exists
- k3s is already running on the node
- The `cert-manager` namespace exists
- A `rancher` Helm release exists in the target namespace

This is by design: partial state is dangerous, and silent re-runs can mask problems. To retry a failed deployment, clean up the relevant component first:

```bash
# Remove k3d cluster
k3d cluster delete rancher-local

# Uninstall k3s
sudo /usr/local/bin/k3s-uninstall.sh

# Remove Rancher Helm release
helm uninstall rancher -n cattle-system

# Remove cert-manager
kubectl delete -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
```

---

## Project structure

```
rancher-deployer/
├── main.go                     # Entrypoint
├── cmd/deploy/
│   ├── root.go                 # Cobra root command + Execute()
│   ├── deploy.go               # `deploy` subcommand — orchestration
│   ├── resolve.go              # `resolve` subcommand — version inspection
│   └── print.go                # CLI output helpers
└── internal/
    ├── detect/
    │   └── detect.go           # k3s vs k3d auto-detection
    ├── kdm/
    │   └── kdm.go              # Rancher KDM fetching + support matrix parsing
    ├── version/
    │   └── version.go          # k8s + k3s version resolution from GitHub API
    ├── k3s/
    │   └── k3s.go              # k3s install + readiness
    ├── k3d/
    │   └── k3d.go              # k3d install + cluster creation
    ├── rancher/
    │   └── rancher.go          # Helm chart ref, values, cert-manager, deploy
    └── runner/
        └── runner.go           # os/exec wrapper (print + run + capture)
```

---

## Extending
**Adding SSH/remote mode:**
The `runner` package is the only place that shells out. A `runner.Remote` that wraps `golang.org/x/crypto/ssh` (or shells to the system `ssh` binary) would slot in cleanly — the rest of the codebase doesn't need to change.

**Adding upgrade support:**
Each package's `EnsureNotInstalled` / `ensureAbsent` guard is the only thing blocking re-runs. An `--upgrade` flag that skips those guards and swaps `helm install` → `helm upgrade` is a contained change to `rancher.go` and `k3s.go`/`k3d.go`.

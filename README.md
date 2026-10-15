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

## Install

Prebuilt binaries for `linux` and `darwin` on `amd64` and `arm64` are published on the
[Releases page](https://github.com/mallardduck/rancher-deployer/releases).

**Quick install** — detects your OS and architecture, downloads the latest release, and
installs it to `/usr/local/bin`:

```bash
os=$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/'); \
v=$(curl -fsSL https://api.github.com/repos/mallardduck/rancher-deployer/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1); \
curl -fsSL "https://github.com/mallardduck/rancher-deployer/releases/download/${v}/rancher-deployer_${v#v}_${os}_${arch}.tar.gz" \
  | sudo tar -xz -C /usr/local/bin rancher-deployer
```

Check it worked:

```bash
rancher-deployer version
```

Keep it current with the built-in updater:

```bash
rancher-deployer self-update
```

`deploy` also needs `helm`, `kubectl`, and `k3s` or `k3d` on your `PATH`; run
`rancher-deployer doctor` to check. For checksum verification, building from source, and
the full prerequisites list, see the [Installation guide](docs/install.md).

## Quick start

```bash
# See what would be installed, without touching anything
rancher-deployer deploy --rancher-version 2.8.5 --dry-run

# Install
rancher-deployer deploy --rancher-version 2.8.5

# Upgrade later
rancher-deployer upgrade --rancher-version 2.8.6

# Remove everything
rancher-deployer teardown
```

## Commands

| Command       | Purpose |
|---------------|---------|
| `deploy`      | Install a cluster, cert-manager, and Rancher |
| `resolve`     | Show the versions that would be used, without installing |
| `upgrade`     | Upgrade an existing Rancher installation, with safety checks |
| `teardown`    | Remove Rancher, cert-manager, and the cluster |
| `doctor`      | Check that required tools are available |
| `self-update` | Update the `rancher-deployer` binary itself |

Flags and details are in [Commands](docs/commands.md).

## Documentation

- [Installation](docs/install.md)
- [Commands](docs/commands.md)
- [Examples](docs/examples.md)
- [Helm values](docs/helm-values.md)
- [Idempotency & cleanup](docs/idempotency-and-cleanup.md)
- [Roadmap](docs/roadmap.md)
- [Design decisions](docs/decisions/)
- [Demo image](docs/demo/README.md)

Contributing? See [CONTRIBUTING.md](CONTRIBUTING.md).

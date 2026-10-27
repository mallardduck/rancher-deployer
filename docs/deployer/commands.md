# Commands

## `deploy` — full install

```
rancher-deployer deploy --rancher-version <version> [flags]
```

| Flag                     | Default           | Description |
|--------------------------|-------------------|-------------|
| `--rancher-version`      | *(required\*)*    | Rancher version, e.g. `2.8.5` or `v2.8.5`. A bare minor (`2.8`) resolves the newest patch |
| `--k8s-version`          | *(auto)*          | Target k8s major.minor, e.g. `1.28` |
| `--prime`                | `false`           | Use Rancher Prime instead of community edition |
| `--channel`              | `stable`          | Release channel: `stable` (GA), `latest` (RC), `alpha`, `head` (see [Head builds](#head-builds)) |
| `--commit`               | *(none)*          | With `--channel head`, pin to the head build whose commit starts with this |
| `--chart-path`           | *(none)*          | Install from a local chart directory or `.tgz` instead of a Helm repo; the version is read from `Chart.yaml`. Replaces `--rancher-version` |
| `--mode`                 | *(auto-detect)*   | Force `k3s` or `k3d` |
| `--expose`               | `sslip`           | How Rancher is reached: `sslip` or `pangolin` (see [Exposing Rancher](expose.md)) |
| `--hostname`             | *(per `--expose`)* | Rancher ingress hostname. Default `rancher.<ip>.sslip.io`; required with `--expose pangolin` |
| `--namespace`            | `cattle-system`   | Kubernetes namespace |
| `--bootstrap-password`   | `letsmein`        | Initial admin password for Rancher |
| `--values-file`          | *(none)*          | Path to Helm values YAML |
| `--set key=value`        | *(none)*          | Set individual Helm values (repeatable) |
| `--cluster-name`         | `rancher-local`   | k3d cluster name (k3d mode only) |
| `--failure-policy`       | `abort`           | If the install fails: `abort` (leave it for inspection) or `reinstall` (roll back/clean up automatically) |
| `--dry-run`              | `false`           | Print plan without executing |
| `--yes, -y`              | `false`           | Skip confirmation prompt (for CI/non-interactive use) |

\* Not required with `--chart-path` or `--prime --channel head --commit`.

## `resolve` — inspect resolved versions (no install)

```
rancher-deployer resolve [--rancher-version <version>] [flags]
```

Useful for scripting or verifying what versions would be selected before committing to a full deploy.
With no `--rancher-version`, the latest version in the chosen channel is resolved.

| Flag                | Default         | Description |
|---------------------|-----------------|-------------|
| `--rancher-version` | *(latest)*      | Rancher version; a bare minor (`2.8`) resolves the newest patch |
| `--k8s-version`     | *(auto)*        | Target k8s major.minor |
| `--prime`           | `false`         | Use the Rancher Prime repository |
| `--channel`         | `stable`        | `stable`, `latest`, `alpha`, or `head` |
| `--commit`          | *(none)*        | With `--channel head`, pin to a head build by commit prefix. With `--prime`, `--rancher-version` can be omitted |
| `--mode`            | *(auto-detect)* | `k3s` or `k3d` |
| `--output`          | *(human-readable)* | `k3s-version`, `k3d-version`, `k8s-version`, or `json` |

## `upgrade` — upgrade an existing Rancher installation

```
rancher-deployer upgrade --rancher-version <version> [flags]
```

| Flag                     | Default           | Description |
|--------------------------|-------------------|-------------|
| `--rancher-version`      | *(required\*)*    | Target Rancher version to upgrade to. A bare minor (`2.9`) resolves the newest patch |
| `--namespace`            | `cattle-system`   | Namespace where Rancher is installed |
| `--prime`                | `false`           | Use Rancher Prime chart |
| `--channel`              | `stable`          | Release channel: `stable` (GA), `latest` (RC), `alpha`, `head` (see [Head builds](#head-builds)) |
| `--commit`               | *(none)*          | With `--channel head`, pin to the head build whose commit starts with this |
| `--chart-path`           | *(none)*          | Install from a local chart directory or `.tgz` instead of a Helm repo; the version is read from `Chart.yaml`. Replaces `--rancher-version` |
| `--expose`               | *(none)*          | Change how Rancher is reached: `sslip` or `pangolin`. Unset keeps the release's current hostname and TLS settings. See [Exposing Rancher](expose.md) |
| `--hostname`             | *(none)*          | Hostname for the ingress; only valid with `--expose` |
| `--mode`                 | *(auto-detect)*   | Cluster type (`k3s`, `k3d`, `existing`) used by `--expose` to configure ingress |
| `--values-file`          | *(none)*          | Path to Helm values YAML |
| `--set key=value`        | *(none)*          | Override Helm chart value (repeatable) |
| `--failure-policy`       | `abort`           | If the upgrade fails: `abort` (leave it for inspection) or `reinstall` (roll back automatically) |
| `--dry-run`              | `false`           | Print resolved plan without executing |
| `--yes, -y`              | `false`           | Skip confirmation prompt |
| `--force`                | `false`           | Skip upgrade-path and k8s-compatibility checks |

\* Not required with `--chart-path` or `--prime --channel head --commit`.

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
| `--yes, -y`              | `false`           | Skip confirmation prompt |

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
| `--pre`     | `true` on a prerelease, else `false` | Also consider prereleases |

Checks GitHub for a newer release of `rancher-deployer` and, on confirmation, replaces
the running binary with it. The release's checksum manifest must carry a Sigstore
signature from this repository's release workflow, so a swapped binary and checksum
file are rejected.

Only stable releases are offered by default. With `--pre`, prereleases are considered
too, and moving onto one asks for an extra confirmation; `--pre --yes` together skip
both prompts for scripts. An install already on a prerelease considers prereleases by default (so rc.2 follows
rc.3 and then the stable release); pass `--pre=false` to look only for stable releases. Installs managed by a package manager (Homebrew, apt,
Scoop) or otherwise not writable by the current user are detected and skipped, with
guidance on how to update through that channel instead.

## `doctor` — check system health

```
rancher-deployer doctor [flags]
```

Validates that the system meets the requirements for `rancher-deployer`: required and
optional binaries, runtime environment (OS, container runtime), configuration, network
connectivity to required endpoints, and the state of any existing installation.

| Flag                | Default         | Description |
|---------------------|-----------------|-------------|
| `--mode`            | *(auto-detect)* | Force `k3s` or `k3d` |
| `--skip-network`    | `false`         | Skip network connectivity checks (for offline environments) |
| `--skip-state`      | `false`         | Skip existing-installation state checks |
| `--network-timeout` | `10s`           | Timeout for network checks |

Exits `0` if all checks pass (warnings are OK) and `1` if any critical check fails.

## Head builds

`--channel head` installs continuously-published, unreleased Rancher builds from
`deploy`, `upgrade`, and `resolve`. It must be scoped to a bare minor
(`--rancher-version 2.15`), which selects the newest head build. To reproduce a bug
reported against a specific build, pin it with `--commit <sha-prefix>`:

```bash
rancher-deployer deploy --channel head --rancher-version 2.15 --commit b03c4de
```

Community and Prime publish head builds differently; see the
[head builds design notes](../decisions/head-builds.md) for details.

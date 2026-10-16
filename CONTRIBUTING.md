# Contributing to rancher-deployer

Thanks for your interest in contributing!

## Getting Started

1. Fork and clone the repository
2. Ensure you have Go 1.25+ installed
3. Run tests to verify your setup:
   ```bash
   make test
   ```

## Making Changes

1. Create a branch for your changes
2. Make your changes
3. Run the linter:
   ```bash
   make lint
   ```
4. Run tests:
   ```bash
   make test
   ```
5. Commit with a clear message describing what and why
6. Push and open a PR

## Project Structure

```
main.go                 - rancher-deployer entrypoint
cmd/deploy/             - rancher-deployer commands (Cobra): deploy, upgrade, resolve, teardown, doctor, self-update, version
cmd/demo/               - rancher-demo entrypoint (the binary in the demo image): resolve, deploy, version
internal/
  cli/                  - Output helpers, prompts and shared commands (resolve) used by both binaries
  clusters/             - Cluster backends (existing/validate checks an already-running cluster)
  deployflow/           - The deploy workflow shared by both binaries
  detect/               - Deployment mode detection (k3s, k3d, existing)
  doctor/               - Dependency and health checks
  helm/                 - Helm backend interface and CLI backend
  helm/controller/      - Backend that installs through k3s/rke2's built-in helm-controller
  k8sresolver/          - k8s and k3s/k3d version resolution
  kdm/                  - Rancher KDM fetching and support-matrix parsing
  kubectl/              - Shared kubectl calls (server version, apply, rollout, ...)
  provider/             - Provider interface each cluster backend implements
  rancher/              - Chart references, values and cert-manager version resolution
  runner/               - os/exec wrapper (print + run + capture)
  selfupdate/           - Self-update via binup against GitHub releases
  upgrade/              - Upgrade validation logic
  version/              - Build-time version info
```

## Extending

**Adding SSH/remote mode:** the `runner` package is the only place that shells out.
A `runner.Remote` that wraps `golang.org/x/crypto/ssh` (or shells to the system `ssh`
binary) would slot in cleanly — the rest of the codebase doesn't need to change.

## Testing

- Write tests for business logic in `internal/` packages
- Table-driven tests preferred
- Run specific package: `go test ./internal/upgrade/`
- Check coverage: `go test -cover ./...`

## Building Locally

```bash
make build          # Build rancher-deployer
./rancher-deployer version

make build-demo     # Build rancher-demo (the binary used in the demo image)
./rancher-demo version
```

## Questions?

Open an issue and ask.

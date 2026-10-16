# Documentation

## rancher-deployer

- [Installation](deployer/install.md) — release binaries, building from source, prerequisites
- [Commands](deployer/commands.md) — every command and flag, plus head builds
- [Examples](deployer/examples.md) — common invocations
- [Exposing Rancher](deployer/expose.md) — `--expose` modes: sslip (default) and Pangolin tunnels
- [Helm values](deployer/helm-values.md) — how `--values-file` and `--set` are merged
- [Idempotency & cleanup](deployer/idempotency-and-cleanup.md) — why `deploy` fails loudly, and how to clean up

## rancher-demo image

- [Demo image guide](demo/README.md) — running the Rancher demo container
- [Access methods](demo/access-methods.md) — localhost ingress, hostname ingress, and NodePort

## Project and contributing

- [Roadmap](roadmap.md) — ideas to pick up when users ask for them
- [Design decisions](decisions/) — records of why things work the way they do
  - [Head builds](decisions/head-builds.md)
- [Contributing](../CONTRIBUTING.md)

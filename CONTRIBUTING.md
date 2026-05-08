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
cmd/deploy/     - CLI commands (Cobra)
internal/       - Implementation packages
```

## Testing

- Write tests for business logic in `internal/` packages
- Table-driven tests preferred
- Run specific package: `go test ./internal/upgrade/`
- Check coverage: `go test -cover ./...`

## Building Locally

```bash
make build          # Build binary
./bin/rancher-deployer version
```

## Questions?

Open an issue and ask.

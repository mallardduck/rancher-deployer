// Package version provides build-time version information for the CLI.
package version

import (
	"fmt"
	"runtime"
)

// Build information. These variables are set via ldflags at build time.
var (
	// Version is the semantic version (e.g., "1.0.0", "v1.2.3").
	// Set via: -ldflags "-X github.com/mallardduck/rancher-deployer/internal/version.Version=v1.2.3"
	Version = "dev"

	// Commit is the git commit SHA.
	// Set via: -ldflags "-X github.com/mallardduck/rancher-deployer/internal/version.Commit=$(git rev-parse HEAD)"
	Commit = "unknown"

	// Date is the build date in RFC3339 format.
	// Set via: -ldflags "-X github.com/mallardduck/rancher-deployer/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	Date = "unknown"
)

// Info returns a formatted version string with all build information.
func Info() string {
	return fmt.Sprintf(
		"rancher-deployer %s\n"+
			"  commit: %s\n"+
			"  built:  %s\n"+
			"  go:     %s",
		Version,
		Commit,
		Date,
		runtime.Version(),
	)
}

// Short returns just the version string (e.g., "v1.2.3" or "dev").
func Short() string {
	return Version
}

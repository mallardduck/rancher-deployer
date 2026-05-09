// Package doctor provides health checks and dependency validation for rancher-deployer.
package doctor

import (
	"context"
	"runtime"
	"time"

	"github.com/mallardduck/rancher-deployer/internal/detect"
)

// CheckStatus represents the result of a validation check.
type CheckStatus int

const (
	// StatusPass indicates the check passed successfully.
	StatusPass CheckStatus = iota
	// StatusWarn indicates a non-critical issue was found.
	StatusWarn
	// StatusFail indicates a critical failure.
	StatusFail
)

// CheckResult contains the outcome of a single validation check.
type CheckResult struct {
	Name        string      // Human-readable check name
	Category    string      // "dependencies", "environment", "configuration", "network", "state"
	Status      CheckStatus // Pass, warn, or fail
	Message     string      // Details about the check
	Remediation string      // How to fix if failed/warned (optional)
}

// Checker interface - all validation checks implement this.
type Checker interface {
	Name() string
	Category() string
	Check(ctx context.Context, opts *CheckOptions) CheckResult
}

// ExecutionContext indicates where commands should be executed.
type ExecutionContext string

const (
	// ContextLocal means commands run on the machine where rancher-deployer executes.
	ContextLocal ExecutionContext = "local"

	// ContextRemote will be used for future SSH mode - commands run on remote target.
	// (Reserved for future use - not implemented in initial version)
	// ContextRemote ExecutionContext = "remote"
)

// CheckOptions configures the doctor run.
type CheckOptions struct {
	Mode           string           // "k3s", "k3d", or "" for auto-detect
	Context        ExecutionContext // "local" for now, "remote" reserved for SSH mode
	SkipNetwork    bool             // Skip network connectivity checks
	SkipState      bool             // Skip existing installation state checks
	NetworkTimeout time.Duration    // Timeout for network checks

	// Future SSH mode fields (not used in initial implementation):
	// SSHTarget      string           // "user@host" for SSH mode
	// SSHConfig      string           // Path to SSH config file
}

// Doctor orchestrates all health checks.
type Doctor struct {
	checkers []Checker
	opts     *CheckOptions
}

// NewDoctor creates a new Doctor with mode-appropriate checkers.
func NewDoctor(opts *CheckOptions) *Doctor {
	if opts == nil {
		opts = &CheckOptions{}
	}

	// Set defaults
	if opts.Context == "" {
		opts.Context = ContextLocal
	}
	if opts.NetworkTimeout == 0 {
		opts.NetworkTimeout = 10 * time.Second
	}

	// Auto-detect mode if not specified
	mode := opts.Mode
	if mode == "" {
		mode, _ = detect.InstallMode()
	}

	d := &Doctor{
		checkers: make([]Checker, 0),
		opts:     opts,
	}

	// Register dependency checkers (always required)
	d.registerDependencyCheckers(mode)

	// Register environment checkers
	d.registerEnvironmentCheckers(mode)

	// Register configuration checkers
	d.registerConfigurationCheckers()

	// Register network checkers (unless skipped)
	if !opts.SkipNetwork {
		d.registerNetworkCheckers()
	}

	// Register state checkers (unless skipped)
	if !opts.SkipState {
		d.registerStateCheckers(mode)
	}

	return d
}

// registerDependencyCheckers registers binary dependency checks.
func (d *Doctor) registerDependencyCheckers(mode string) {
	// Always required binaries
	d.addChecker(&BinaryChecker{
		binary:      "kubectl",
		displayName: "kubectl",
		required:    true,
		location:    LocationLocal,
	})
	d.addChecker(&BinaryChecker{
		binary:      "helm",
		displayName: "helm",
		required:    true,
		location:    LocationLocal,
	})

	// Mode-specific binaries
	switch mode {
	case "k3s":
		d.addChecker(&BinaryChecker{
			binary:      "k3s",
			displayName: "k3s",
			required:    false, // Can be auto-installed
			modes:       []string{"k3s"},
			location:    LocationRemote,
		})

		// Only check systemctl on Linux
		if runtime.GOOS == "linux" {
			d.addChecker(&BinaryChecker{
				binary:      "systemctl",
				displayName: "systemctl",
				required:    true,
				modes:       []string{"k3s"},
				location:    LocationRemote,
			})
		}

		d.addChecker(&BinaryChecker{
			binary:      "sudo",
			displayName: "sudo",
			required:    false, // Warn if missing
			modes:       []string{"k3s"},
			location:    LocationRemote,
		})
		d.addChecker(&BinaryChecker{
			binary:      "curl",
			displayName: "curl",
			required:    false,
			modes:       []string{"k3s"},
			location:    LocationRemote,
		})
		d.addChecker(&BinaryChecker{
			binary:      "sh",
			displayName: "sh",
			required:    true,
			modes:       []string{"k3s"},
			location:    LocationRemote,
		})
	case "k3d":
		d.addChecker(&BinaryChecker{
			binary:      "k3d",
			displayName: "k3d",
			required:    false, // Can be auto-installed
			modes:       []string{"k3d"},
			location:    LocationRemote,
		})
		d.addChecker(&BinaryChecker{
			binary:      "curl",
			displayName: "curl",
			required:    false,
			modes:       []string{"k3d"},
			location:    LocationRemote,
		})
		d.addChecker(&BinaryChecker{
			binary:      "bash",
			displayName: "bash",
			required:    true,
			modes:       []string{"k3d"},
			location:    LocationRemote,
		})

		// Docker or Podman is required for k3d
		d.addChecker(&ContainerRuntimeChecker{})
	}
}

// registerEnvironmentCheckers registers environment validation checks.
func (d *Doctor) registerEnvironmentCheckers(mode string) {
	d.addChecker(&RuntimeChecker{mode: mode})
	d.addChecker(&EnvVarChecker{
		varName:     "GH_TOKEN",
		fallback:    "GITHUB_TOKEN",
		displayName: "GitHub API token",
		purpose:     "Increases GitHub API rate limit from 60 to 5000 requests/hour",
		required:    false,
	})
}

// registerConfigurationCheckers registers configuration file checks.
func (d *Doctor) registerConfigurationCheckers() {
	d.addChecker(&ConfigFileChecker{
		displayName: "kubeconfig",
		required:    false, // Expected to be missing for new installations
	})
	d.addChecker(&CacheDirectoryChecker{})
}

// registerNetworkCheckers registers network connectivity checks.
func (d *Doctor) registerNetworkCheckers() {
	d.addChecker(&EndpointChecker{
		url:         "https://releases.rancher.com/kontainer-driver-metadata/",
		displayName: "Rancher releases",
		purpose:     "Required for version resolution and compatibility checks",
	})
	d.addChecker(&EndpointChecker{
		url:         "https://api.github.com",
		displayName: "GitHub API",
		purpose:     "Required for k3s releases and cert-manager version resolution",
	})
	d.addChecker(&EndpointChecker{
		url:         "https://get.k3s.io",
		displayName: "k3s install script",
		purpose:     "Required for automated k3s installation",
	})
	d.addChecker(&EndpointChecker{
		url:         "https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh",
		displayName: "k3d install script",
		purpose:     "Required for automated k3d installation",
	})
	d.addChecker(&GitHubRateLimitChecker{})
}

// registerStateCheckers registers installation state checks.
func (d *Doctor) registerStateCheckers(mode string) {
	d.addChecker(&ClusterAccessChecker{})

	switch mode {
	case "k3s":
		if runtime.GOOS == "linux" {
			d.addChecker(&K3sServiceChecker{})
		}
	case "k3d":
		d.addChecker(&K3dClusterChecker{})
	}

	d.addChecker(&RancherInstallationChecker{})
}

// addChecker adds a checker to the doctor.
func (d *Doctor) addChecker(c Checker) {
	d.checkers = append(d.checkers, c)
}

// RunAll executes all registered checkers and returns results.
func (d *Doctor) RunAll(ctx context.Context) []CheckResult {
	results := make([]CheckResult, 0, len(d.checkers))

	for _, checker := range d.checkers {
		result := checker.Check(ctx, d.opts)
		results = append(results, result)
	}

	return results
}

// HasCriticalFailures checks if any results have StatusFail.
func HasCriticalFailures(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == StatusFail {
			return true
		}
	}
	return false
}

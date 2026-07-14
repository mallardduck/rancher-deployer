// Package doctor provides health checks and dependency validation for rancher-deployer.
//
// TODO: Future enhancement - Auto-fix missing dependencies
// Add a --fix flag that automatically downloads missing binaries using github.com/mallardduck/ghreleases
// and stores them in a local .bin/ directory. See TODO.md for full implementation plan.
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
	Message     string      // Details about the check
	Remediation string      // How to fix if failed/warned (optional)
	Status      CheckStatus // Pass, warn, or fail
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
	opts     *CheckOptions
	checkers []Checker
}

// NewDoctor creates a new Doctor with the given checkers.
// providerCheckers should come from provider.Checkers() and supply all
// binary/runtime prerequisite checks for the active deployment mode.
func NewDoctor(opts *CheckOptions, providerCheckers ...Checker) *Doctor {
	if opts == nil {
		opts = &CheckOptions{}
	}

	if opts.Context == "" {
		opts.Context = ContextLocal
	}
	if opts.NetworkTimeout == 0 {
		opts.NetworkTimeout = 10 * time.Second
	}

	mode := opts.Mode
	if mode == "" {
		mode, _ = detect.InstallMode()
	}

	d := &Doctor{
		checkers: make([]Checker, 0),
		opts:     opts,
	}

	for _, c := range providerCheckers {
		d.addChecker(c)
	}

	d.registerConfigurationCheckers()

	if !opts.SkipNetwork {
		d.registerNetworkCheckers()
	}

	if !opts.SkipState {
		d.registerStateCheckers(mode)
	}

	return d
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

package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/detect"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// ExecutionLocation indicates where a binary needs to exist.
type ExecutionLocation string

const (
	// LocationLocal - checked locally (kubectl, helm, ssh).
	LocationLocal ExecutionLocation = "local"
	// LocationRemote - checked on remote target (k3s, docker, systemctl) - reserved for future SSH mode.
	LocationRemote ExecutionLocation = "remote"
	// LocationBoth - needed in both places (curl, bash) - reserved for future.
	LocationBoth ExecutionLocation = "both"
)

// BinaryChecker validates that a binary exists in PATH.
// TODO: When --fix flag is implemented, this checker should return remediation
// with downloadable binaries using github.com/mallardduck/ghreleases
type BinaryChecker struct {
	binary      string
	displayName string
	required    bool              // false = warn, true = fail
	modes       []string          // empty = all modes, ["k3s"] = k3s only
	location    ExecutionLocation // where to check for this binary
}

func (c *BinaryChecker) Name() string {
	return c.displayName
}

func (c *BinaryChecker) Category() string {
	return "dependencies"
}

func (c *BinaryChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	// For initial implementation (local context only), all checks run locally
	// When SSH mode is added, checkers with LocationRemote will run over SSH

	if runner.Exists(c.binary) {
		// Try to get version
		path, _ := exec.LookPath(c.binary)
		msg := fmt.Sprintf("found at %s", path)

		// Attempt to get version (best effort, don't fail if version command fails)
		if version := getVersion(c.binary); version != "" {
			msg = fmt.Sprintf("%s (%s)", msg, version)
		}

		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  msg,
		}
	}

	// Binary not found
	status := StatusWarn
	if c.required {
		status = StatusFail
	}

	remediation := getInstallRemediation(c.binary)

	return CheckResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Status:      status,
		Message:     "not found in PATH",
		Remediation: remediation,
	}
}

// getVersion attempts to get the version of a binary (best effort).
func getVersion(binary string) string {
	// Try common version flags
	versionArgs := [][]string{
		{"version", "--client", "--short"}, // kubectl
		{"version", "--short"},             // helm
		{"--version"},                      // most tools
		{"version"},                        // some tools
	}

	for _, args := range versionArgs {
		cmd := exec.Command(binary, args...)
		out, err := cmd.CombinedOutput()
		if err == nil && len(out) > 0 {
			version := strings.TrimSpace(string(out))
			// Clean up common prefixes
			version = strings.TrimPrefix(version, "v")
			version = strings.TrimPrefix(version, "version ")
			// Take first line if multi-line
			if idx := strings.Index(version, "\n"); idx > 0 {
				version = version[:idx]
			}
			// Limit length
			if len(version) > 50 {
				version = version[:50] + "..."
			}
			return version
		}
	}

	return ""
}

// getInstallRemediation provides installation instructions for common binaries.
func getInstallRemediation(binary string) string {
	// OS-specific package manager
	pm := getPackageManager()

	switch binary {
	case "kubectl":
		switch pm {
		case "brew":
			return "Install: brew install kubectl"
		default:
			return "Install: https://kubernetes.io/docs/tasks/tools/install-kubectl/"
		}
	case "helm":
		switch pm {
		case "brew":
			return "Install: brew install helm"
		default:
			return "Install: https://helm.sh/docs/intro/install/"
		}
	case "k3s":
		return "k3s will be automatically installed during deployment"
	case "k3d":
		return "k3d will be automatically installed during deployment"
	case "docker":
		switch pm {
		case "brew":
			return "Install: brew install --cask docker"
		default:
			return "Install: https://docs.docker.com/get-docker/"
		}
	case "podman":
		switch pm {
		case "brew":
			return "Install: brew install podman"
		default:
			return "Install: https://podman.io/getting-started/installation"
		}
	case "curl":
		switch pm {
		case "brew":
			return "Install: brew install curl"
		case "apt":
			return "Install: sudo apt-get install curl"
		case "yum":
			return "Install: sudo yum install curl"
		default:
			return "Install curl using your package manager"
		}
	case "systemctl":
		return "systemctl is part of systemd - ensure you're running on a systemd-based Linux distribution"
	case "sudo":
		switch pm {
		case "apt":
			return "Install: apt-get install sudo"
		default:
			return "Install sudo using your package manager"
		}
	case "bash":
		switch pm {
		case "brew":
			return "Install: brew install bash"
		default:
			return "bash should be pre-installed on most systems"
		}
	case "sh":
		return "sh should be pre-installed on all Unix-like systems"
	default:
		return fmt.Sprintf("Install %s using your package manager", binary)
	}
}

// getPackageManager detects the likely package manager for the OS.
func getPackageManager() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew"
	case "linux":
		// Try to detect Linux package manager
		if runner.Exists("apt-get") {
			return "apt"
		}
		if runner.Exists("yum") {
			return "yum"
		}
		if runner.Exists("dnf") {
			return "dnf"
		}
		if runner.Exists("pacman") {
			return "pacman"
		}
		return "unknown"
	default:
		return "unknown"
	}
}

// RuntimeChecker validates OS and container runtime compatibility.
type RuntimeChecker struct {
	mode string
}

func (c *RuntimeChecker) Name() string {
	return "runtime environment"
}

func (c *RuntimeChecker) Category() string {
	return "environment"
}

func (c *RuntimeChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	mode := c.mode
	if mode == "" {
		mode, _ = detect.InstallMode()
	}

	osName := runtime.GOOS
	osDisplay := osName

	// Validate OS compatibility with mode
	if mode == "k3s" && osName != "linux" {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusFail,
			Message:     fmt.Sprintf("OS is %s, but k3s mode requires Linux", osName),
			Remediation: "Use k3d mode on macOS, or switch to a Linux system for k3s mode",
		}
	}

	msg := fmt.Sprintf("OS is %s (compatible with %s mode)", osDisplay, mode)

	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   StatusPass,
		Message:  msg,
	}
}

// ContainerRuntimeChecker validates that Docker or Podman is available and running.
type ContainerRuntimeChecker struct{}

func (c *ContainerRuntimeChecker) Name() string {
	return "container runtime"
}

func (c *ContainerRuntimeChecker) Category() string {
	return "dependencies"
}

func (c *ContainerRuntimeChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	// Try Docker first
	if runner.Exists("docker") {
		cmd := exec.Command("docker", "info", "--format", "{{.ServerVersion}}")
		out, err := cmd.CombinedOutput()
		if err == nil {
			version := strings.TrimSpace(string(out))
			return CheckResult{
				Name:     c.Name(),
				Category: c.Category(),
				Status:   StatusPass,
				Message:  fmt.Sprintf("Docker is running (v%s)", version),
			}
		}
		// Docker binary exists but daemon not running
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusFail,
			Message:     "Docker binary found but daemon is not running",
			Remediation: "Start Docker: open Docker Desktop or run 'systemctl start docker'",
		}
	}

	// Try Podman
	if runner.Exists("podman") {
		cmd := exec.Command("podman", "info", "--format", "{{.Version.Version}}")
		out, err := cmd.CombinedOutput()
		if err == nil {
			version := strings.TrimSpace(string(out))
			return CheckResult{
				Name:     c.Name(),
				Category: c.Category(),
				Status:   StatusPass,
				Message:  fmt.Sprintf("Podman is running (v%s)", version),
			}
		}
	}

	// Neither Docker nor Podman found
	pm := getPackageManager()
	remediation := "Install Docker or Podman"
	if pm == "brew" {
		remediation = "Install Docker: brew install --cask docker, or Podman: brew install podman"
	}

	return CheckResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Status:      StatusFail,
		Message:     "Neither Docker nor Podman found - k3d requires a container runtime",
		Remediation: remediation,
	}
}

// EnvVarChecker checks for optional environment variables.
type EnvVarChecker struct {
	varName     string
	fallback    string // Optional fallback variable name
	displayName string
	purpose     string // Why this var is useful
	required    bool
}

func (c *EnvVarChecker) Name() string {
	return c.displayName
}

func (c *EnvVarChecker) Category() string {
	return "environment"
}

func (c *EnvVarChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	value := os.Getenv(c.varName)
	if value == "" && c.fallback != "" {
		value = os.Getenv(c.fallback)
	}

	if value != "" {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  "set",
		}
	}

	// Variable not set
	status := StatusWarn
	if c.required {
		status = StatusFail
	}

	remediation := fmt.Sprintf("Set %s environment variable - %s", c.varName, c.purpose)

	return CheckResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Status:      status,
		Message:     "not set",
		Remediation: remediation,
	}
}

// ConfigFileChecker validates expected configuration files.
type ConfigFileChecker struct {
	displayName string
	required    bool
}

func (c *ConfigFileChecker) Name() string {
	return c.displayName
}

func (c *ConfigFileChecker) Category() string {
	return "configuration"
}

func (c *ConfigFileChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	// Determine kubeconfig path
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return CheckResult{
				Name:     c.Name(),
				Category: c.Category(),
				Status:   StatusWarn,
				Message:  "could not determine home directory",
			}
		}
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	// Clean path to prevent traversal
	kubeconfigPath = filepath.Clean(kubeconfigPath)

	if _, err := os.Stat(kubeconfigPath); err == nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  fmt.Sprintf("found at %s", kubeconfigPath),
		}
	}

	// Kubeconfig not found
	status := StatusWarn
	if c.required {
		status = StatusFail
	}

	return CheckResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Status:      status,
		Message:     fmt.Sprintf("not found at %s", kubeconfigPath),
		Remediation: "This is normal for new installations - kubeconfig will be created during deployment",
	}
}

// CacheDirectoryChecker validates the cache directory is accessible.
type CacheDirectoryChecker struct{}

func (c *CacheDirectoryChecker) Name() string {
	return "cache directory"
}

func (c *CacheDirectoryChecker) Category() string {
	return "configuration"
}

func (c *CacheDirectoryChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     "could not determine cache directory",
			Remediation: "Set XDG_CACHE_HOME environment variable",
		}
	}

	appCacheDir := filepath.Join(cacheDir, "rancher-deployer")

	// Try to create cache directory if it doesn't exist
	if err := os.MkdirAll(appCacheDir, 0750); err != nil {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     fmt.Sprintf("cannot create cache directory at %s: %v", appCacheDir, err),
			Remediation: "Ensure you have write permissions to your cache directory",
		}
	}

	// Test writability
	testFile := filepath.Join(appCacheDir, ".write-test")
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     fmt.Sprintf("cache directory at %s is not writable", appCacheDir),
			Remediation: "Ensure you have write permissions to your cache directory",
		}
	}
	_ = os.Remove(testFile) // Clean up (ignore error)

	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   StatusPass,
		Message:  fmt.Sprintf("writable at %s", appCacheDir),
	}
}

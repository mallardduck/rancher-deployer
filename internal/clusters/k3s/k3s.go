// Package k3s implements the provider.Provider interface for bare-metal k3s installations.
package k3s

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/provider"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

var _ provider.Provider = (*Provider)(nil)

const (
	installScriptURL = "https://get.k3s.io"
	kubeconfigPath   = "/etc/rancher/k3s/k3s.yaml"
)

// Provider wraps bare-metal k3s installation and satisfies provider.Provider.
type Provider struct{}

// NewProvider creates a k3s provider.
func NewProvider() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "k3s" }

func (p *Provider) ResolveClusterVersion(_ context.Context, k8sVersion string) (string, error) {
	return k8sresolver.ResolveClusterVersion("k3s", k8sVersion)
}

func (p *Provider) Setup(_ context.Context, opts provider.SetupOptions) error {
	if err := install(opts.ClusterVersion); err != nil {
		return err
	}
	// Set KUBECONFIG before waitReady so kubectl can reach the cluster
	// while the kubeconfig still lives at the k3s system path.
	if err := os.Setenv("KUBECONFIG", kubeconfigPath); err != nil {
		return fmt.Errorf("could not set KUBECONFIG: %w", err)
	}
	if err := waitReady(); err != nil {
		return fmt.Errorf("k3s node did not become ready: %w", err)
	}
	return exportKubeconfig()
}

func (p *Provider) Teardown(_ context.Context, _ provider.TeardownOptions) error {
	return runner.RunSudo("/usr/local/bin/k3s-uninstall.sh")
}

func (p *Provider) KubeconfigPath() string { return kubeconfigPath }

func (p *Provider) Helm() helm.Backend {
	return helm.NewController()
}

func (p *Provider) Checkers() []doctor.Checker {
	checkers := []doctor.Checker{
		doctor.NewRequiredBinaryChecker("kubectl", "kubectl"),
		doctor.NewOptionalBinaryCheckerWithRemediation("helm", "helm",
			"not required — k3s uses the built-in helm-controller for chart installation"),
		doctor.NewOptionalBinaryChecker("k3s", "k3s"),
		doctor.NewOptionalBinaryChecker("sudo", "sudo"),
		doctor.NewOptionalBinaryChecker("curl", "curl"),
		doctor.NewRequiredBinaryChecker("sh", "sh"),
		doctor.NewRuntimeChecker("k3s"),
		doctor.NewGitHubTokenChecker(),
	}
	if runtime.GOOS == "linux" {
		checkers = append(checkers, doctor.NewRequiredBinaryChecker("systemctl", "systemctl"))
	}
	return checkers
}

// ── k3s cluster operations ────────────────────────────────────────────────────

// install downloads and runs the official k3s install script pinned to the given version.
func install(version string) error {
	if isRunning() {
		return fmt.Errorf(
			"k3s is already running on this node\n" +
				"  Re-runs are not supported to avoid partial state.\n" +
				"  To reinstall, first run: sudo /usr/local/bin/k3s-uninstall.sh",
		)
	}

	if err := runner.MustExist("curl"); err != nil {
		return err
	}

	fmt.Printf("  Installing k3s %s via official install script\n", version)

	// The install script respects INSTALL_K3S_VERSION env var.
	script := fmt.Sprintf(
		"curl -sfL %s | INSTALL_K3S_VERSION=%s sh -s - --write-kubeconfig-mode 644",
		installScriptURL, version,
	)
	return runner.RunSudo("sh", "-c", script)
}

// waitReady blocks until the k3s node reports Ready.
func waitReady() error {
	fmt.Println("  Waiting for k3s node to become Ready...")
	return runner.Kubectl("wait",
		"--for=condition=Ready",
		"node", "--all",
		"--timeout=120s",
	)
}

// exportKubeconfig copies the k3s kubeconfig to ~/.kube/config so that
// helm and kubectl work without extra flags.
func exportKubeconfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}
	kubeDir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(kubeDir, 0700); err != nil {
		return fmt.Errorf("could not create %s: %w", kubeDir, err)
	}
	dest := filepath.Join(kubeDir, "config")
	fmt.Printf("  Copying k3s kubeconfig → %s\n", dest)
	return runner.RunSudo("cp", kubeconfigPath, dest)
}

// isRunning returns true if the k3s service or process is active.
func isRunning() bool {
	if runner.Exists("systemctl") {
		out, err := exec.Command("systemctl", "is-active", "k3s").Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return true
		}
	}
	if runner.Exists("k3s") {
		out, err := exec.Command("k3s", "kubectl", "get", "nodes").CombinedOutput()
		if err == nil && len(out) > 0 {
			return true
		}
	}
	return false
}

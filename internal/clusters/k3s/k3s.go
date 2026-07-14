package k3s

import (
	"context"
	"fmt"
	"os"
	"runtime"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	legacyk3s "github.com/mallardduck/rancher-deployer/internal/k3s"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// Provider wraps bare-metal k3s installation and satisfies provider.Provider.
type Provider struct{}

// NewProvider creates a k3s provider.
func NewProvider() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "k3s" }

func (p *Provider) ResolveClusterVersion(_ context.Context, k8sVersion string) (string, error) {
	return k8sresolver.ResolveClusterVersion("k3s", k8sVersion)
}

func (p *Provider) Setup(_ context.Context, opts provider.SetupOptions) error {
	if err := legacyk3s.Install(opts.ClusterVersion); err != nil {
		return err
	}
	// Set KUBECONFIG before WaitReady so kubectl can reach the cluster
	// while the kubeconfig still lives at the k3s system path.
	if err := os.Setenv("KUBECONFIG", legacyk3s.KubeconfigPath()); err != nil {
		return fmt.Errorf("could not set KUBECONFIG: %w", err)
	}
	if err := legacyk3s.WaitReady(); err != nil {
		return fmt.Errorf("k3s node did not become ready: %w", err)
	}
	return legacyk3s.ExportKubeconfig()
}

func (p *Provider) Teardown(_ context.Context, _ provider.TeardownOptions) error {
	return legacyk3s.Uninstall()
}

func (p *Provider) KubeconfigPath() string {
	return legacyk3s.KubeconfigPath()
}

func (p *Provider) Helm() helm.Backend {
	return helm.NewCLI()
}

func (p *Provider) Checkers() []doctor.Checker {
	checkers := []doctor.Checker{
		doctor.NewRequiredBinaryChecker("kubectl", "kubectl"),
		doctor.NewRequiredBinaryChecker("helm", "helm"),
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

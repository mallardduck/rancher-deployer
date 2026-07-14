package k3d

import (
	"context"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	legacyk3d "github.com/mallardduck/rancher-deployer/internal/k3d"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// Provider wraps k3d cluster operations and satisfies provider.Provider.
type Provider struct {
	clusterName string
}

// NewProvider creates a k3d provider for the given cluster name.
func NewProvider(clusterName string) *Provider {
	return &Provider{clusterName: clusterName}
}

func (p *Provider) Name() string { return "k3d" }

func (p *Provider) ResolveClusterVersion(_ context.Context, k8sVersion string) (string, error) {
	return k8sresolver.ResolveClusterVersion("k3d", k8sVersion)
}

func (p *Provider) Setup(_ context.Context, opts provider.SetupOptions) error {
	if err := legacyk3d.EnsureInstalled(); err != nil {
		return err
	}
	if err := legacyk3d.CreateCluster(p.clusterName, opts.ClusterVersion); err != nil {
		return err
	}
	return legacyk3d.KubeconfigMerge(p.clusterName)
}

func (p *Provider) Teardown(_ context.Context, _ provider.TeardownOptions) error {
	return legacyk3d.DeleteCluster(p.clusterName)
}

// KubeconfigPath returns "" because k3d merges into the default ~/.kube/config.
func (p *Provider) KubeconfigPath() string { return "" }

func (p *Provider) Helm() helm.Backend {
	return helm.NewCLI()
}

func (p *Provider) Checkers() []doctor.Checker {
	return []doctor.Checker{
		doctor.NewRequiredBinaryChecker("kubectl", "kubectl"),
		doctor.NewRequiredBinaryChecker("helm", "helm"),
		doctor.NewOptionalBinaryChecker("k3d", "k3d"),
		doctor.NewOptionalBinaryChecker("curl", "curl"),
		doctor.NewRequiredBinaryChecker("bash", "bash"),
		doctor.NewContainerRuntimeChecker(),
		doctor.NewRuntimeChecker("k3d"),
		doctor.NewGitHubTokenChecker(),
	}
}

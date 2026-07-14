package existing

import (
	"context"
	"fmt"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	legacyexisting "github.com/mallardduck/rancher-deployer/internal/existing"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	"github.com/mallardduck/rancher-deployer/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// Provider wraps validation of a pre-existing Kubernetes cluster.
type Provider struct {
	kubeconf string
}

// NewProvider creates an existing cluster provider. Pass "" to use the default
// kubeconfig location (~/.kube/config or $KUBECONFIG).
func NewProvider(kubeconf string) *Provider {
	return &Provider{kubeconf: kubeconf}
}

func (p *Provider) Name() string { return "existing" }

// ResolveClusterVersion validates kubectl connectivity and verifies that the
// cluster's k8s minor version matches what Rancher requires. Unlike k3d/k3s
// providers, this does real I/O — it is the validation step for existing clusters.
func (p *Provider) ResolveClusterVersion(_ context.Context, k8sVersion string) (string, error) {
	clusterVersion, err := legacyexisting.ValidateCluster()
	if err != nil {
		return "", err
	}
	if err := legacyexisting.ValidateVersion(clusterVersion, k8sVersion); err != nil {
		return "", err
	}
	return clusterVersion, nil
}

func (p *Provider) Setup(_ context.Context, _ provider.SetupOptions) error {
	fmt.Println("  Using existing Kubernetes cluster (kubectl already configured)")
	return nil
}

func (p *Provider) Teardown(_ context.Context, _ provider.TeardownOptions) error {
	fmt.Println("  Existing cluster not removed (managed externally)")
	return nil
}

func (p *Provider) KubeconfigPath() string { return p.kubeconf }

func (p *Provider) Helm() helm.Backend {
	// Phase 2: detect distro via k8sDistro field and return HelmController or HelmCLI.
	panic("helm backends not implemented until Phase 2")
}

// Checkers returns static pre-connection prerequisite checks.
// Distro-specific checks (e.g. helm-controller availability on k3s/rke2 clusters)
// are added as state checks after ResolveClusterVersion identifies the distro.
func (p *Provider) Checkers() []doctor.Checker {
	return []doctor.Checker{
		doctor.NewRequiredBinaryChecker("kubectl", "kubectl"),
		doctor.NewRequiredBinaryChecker("helm", "helm"),
		doctor.NewRuntimeChecker("existing"),
		doctor.NewGitHubTokenChecker(),
	}
}

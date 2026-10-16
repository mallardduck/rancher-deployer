// Package existing implements the provider.Provider interface for pre-existing Kubernetes clusters.
package existing

import (
	"context"
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/clusters/existing/validate"
	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	"github.com/mallardduck/rancher-deployer/internal/helm/controller"
	"github.com/mallardduck/rancher-deployer/internal/kubectl"
	"github.com/mallardduck/rancher-deployer/internal/provider"
)

var _ provider.Provider = (*Provider)(nil)

// Provider wraps validation of a pre-existing Kubernetes cluster.
type Provider struct {
	kubeconf string
	distro   string // "k3s", "rke2", or "other" — resolved lazily by Helm()
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
	clusterVersion, err := validate.Cluster()
	if err != nil {
		return "", err
	}
	if err := validate.Version(clusterVersion, k8sVersion); err != nil {
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

// Helm detects the cluster distro on first call and returns the appropriate backend.
// k3s and rke2 clusters get HelmController (no helm CLI needed on the host);
// all other clusters get HelmCLI.
func (p *Provider) Helm() helm.Backend {
	if p.distro == "" {
		p.distro = p.detectDistro()
	}
	if p.distro == "k3s" || p.distro == "rke2" {
		return controller.New()
	}
	return helm.NewCLI()
}

// detectDistro inspects the server gitVersion suffix to identify the cluster distro.
// Falls back to "other" if the cluster is unreachable or the version is unparseable.
func (p *Provider) detectDistro() string {
	raw, err := kubectl.ServerVersion()
	if err != nil {
		return "other"
	}
	switch {
	case strings.Contains(raw, "+k3s"):
		return "k3s"
	case strings.Contains(raw, "+rke2"):
		return "rke2"
	default:
		return "other"
	}
}

// Checkers returns static pre-connection prerequisite checks.
// helm is optional because k3s/rke2 clusters use helm-controller; the actual
// backend selection happens lazily in Helm() once the distro is known.
func (p *Provider) Checkers() []doctor.Checker {
	return []doctor.Checker{
		doctor.NewRequiredBinaryChecker("kubectl", "kubectl"),
		doctor.NewOptionalBinaryCheckerWithRemediation("helm", "helm",
			"may not be required — k3s and rke2 clusters use the built-in helm-controller"),
		doctor.NewRuntimeChecker("existing"),
		doctor.NewGitHubTokenChecker(),
	}
}

// ── cluster validation ────────────────────────────────────────────────────────

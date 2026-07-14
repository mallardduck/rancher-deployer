// Package k3d implements the provider.Provider interface for k3d-managed clusters.
package k3d

import (
	"context"
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/provider"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

var _ provider.Provider = (*Provider)(nil)

const (
	k3dInstallScript = "https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh"
	k3sImageRepo     = "rancher/k3s"
)

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
	if err := ensureInstalled(); err != nil {
		return err
	}
	if err := createCluster(p.clusterName, opts.ClusterVersion); err != nil {
		return err
	}
	return runner.K3d("kubeconfig", "merge", p.clusterName, "--kubeconfig-merge-default")
}

func (p *Provider) Teardown(_ context.Context, _ provider.TeardownOptions) error {
	return runner.K3d("cluster", "delete", p.clusterName)
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

// ── k3d cluster operations ────────────────────────────────────────────────────

// ensureInstalled checks for k3d and installs it if missing.
func ensureInstalled() error {
	if runner.Exists("k3d") {
		out, err := runner.Output("k3d", "version")
		if err == nil {
			fmt.Printf("  k3d already installed: %s\n", firstLine(out))
			return nil
		}
	}

	fmt.Println("  k3d not found — installing via official script...")
	if err := runner.MustExist("curl"); err != nil {
		return err
	}
	script := fmt.Sprintf("curl -s %s | bash", k3dInstallScript)
	return runner.Run("bash", "-c", script)
}

// createCluster creates a new k3d cluster. Fails if a cluster with the same
// name already exists (fail-loud idempotency).
func createCluster(name, k3sVersion string) error {
	out, err := runner.Output("k3d", "cluster", "list", "--no-headers", "-o", "name")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) == name {
				return fmt.Errorf(
					"k3d cluster %q already exists\n"+
						"  Re-runs are not supported to avoid partial state.\n"+
						"  To remove it, run: k3d cluster delete %s",
					name, name,
				)
			}
		}
	}

	image := k3sVersionToImage(k3sVersion)
	fmt.Printf("  Creating k3d cluster %q with image %s\n", name, image)

	return runner.K3d("cluster", "create", name,
		"--image", image,
		"--wait",
		"-p", "80:80@loadbalancer",
		"-p", "443:443@loadbalancer",
	)
}

// k3sVersionToImage converts a k3s version tag to a k3d-compatible image ref.
// k3s tags use '+' as separator (v1.28.10+k3s1) but Docker requires '-' (v1.28.10-k3s1).
func k3sVersionToImage(k3sVersion string) string {
	tag := strings.ReplaceAll(k3sVersion, "+", "-")
	return fmt.Sprintf("%s:%s", k3sImageRepo, tag)
}

func firstLine(s string) string {
	lines := strings.SplitN(s, "\n", 2)
	return strings.TrimSpace(lines[0])
}

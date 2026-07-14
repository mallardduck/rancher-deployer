package existing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
	"github.com/mallardduck/rancher-deployer/internal/provider"
	"github.com/mallardduck/rancher-deployer/internal/runner"
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
	clusterVersion, err := validateCluster()
	if err != nil {
		return "", err
	}
	if err := validateVersion(clusterVersion, k8sVersion); err != nil {
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
		return helm.NewController()
	}
	return helm.NewCLI()
}

// detectDistro inspects the server gitVersion suffix to identify the cluster distro.
// Falls back to "other" if the cluster is unreachable or the version is unparseable.
func (p *Provider) detectDistro() string {
	raw, err := rawServerVersion()
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

type kubectlVersionOutput struct {
	ServerVersion *versionInfo `json:"serverVersion"`
}

type versionInfo struct {
	GitVersion string `json:"gitVersion"`
}

// rawServerVersion returns the unmodified server gitVersion string, including
// any distro suffix (e.g. "v1.28.10+k3s1", "v1.29.4+rke2r1").
func rawServerVersion() (string, error) {
	out, err := runner.Output("kubectl", "version", "--output=json")
	if err != nil {
		return "", fmt.Errorf("could not query cluster version: %w", err)
	}
	var v kubectlVersionOutput
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.ServerVersion == nil {
		return "", fmt.Errorf("could not parse kubectl version output")
	}
	return v.ServerVersion.GitVersion, nil
}

// validateCluster checks that kubectl can reach the cluster and returns the k8s version.
func validateCluster() (string, error) {
	if err := runner.MustExist("kubectl"); err != nil {
		return "", fmt.Errorf("kubectl not found: %w\nPlease ensure kubectl is installed and configured", err)
	}

	out, err := runner.Output("kubectl", "version", "--output=json")
	if err != nil {
		return "", fmt.Errorf("cannot reach Kubernetes cluster via kubectl: %w\nPlease ensure KUBECONFIG is set correctly and the cluster is accessible", err)
	}

	version := extractVersion(out)
	if version == "" {
		return "", fmt.Errorf("could not parse Kubernetes version from kubectl output:\n%s", out)
	}

	fmt.Printf("  Kubernetes cluster found: %s\n", version)
	return version, nil
}

// validateVersion checks that the cluster's k8s version matches the expected version.
func validateVersion(clusterVersion, expectedVersion string) error {
	cluster := strings.TrimPrefix(clusterVersion, "v")
	expected := strings.TrimPrefix(expectedVersion, "v")

	clusterMinor := extractMinor(cluster)
	expectedMinor := extractMinor(expected)

	if clusterMinor != expectedMinor {
		return fmt.Errorf(
			"kubernetes version mismatch:\n"+
				"  cluster has: %s (minor: %s)\n"+
				"  rancher requires: %s (minor: %s)\n\n"+
				"Please use a cluster with Kubernetes %s.x",
			clusterVersion, clusterMinor, expectedVersion, expectedMinor, expectedMinor,
		)
	}

	fmt.Printf("  Kubernetes version validated: %s matches required %s\n", clusterVersion, expectedVersion)
	return nil
}

// extractVersion parses the base k8s version from kubectl version JSON output,
// stripping any distro suffix (e.g. "v1.28.10+k3s1" → "v1.28.10").
func extractVersion(output string) string {
	var v kubectlVersionOutput
	if err := json.Unmarshal([]byte(output), &v); err != nil || v.ServerVersion == nil {
		return ""
	}
	full := v.ServerVersion.GitVersion
	if full == "" {
		return ""
	}
	if i := strings.Index(full, "+"); i != -1 {
		return full[:i]
	}
	return full
}

// extractMinor returns the "major.minor" portion of a version string.
func extractMinor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

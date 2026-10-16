// Package validate checks that a pre-existing Kubernetes cluster is reachable
// and compatible with the Kubernetes version Rancher requires. It depends only
// on kubectl, so binaries that deploy onto an existing cluster can use it
// without pulling in the full existing-cluster provider.
package validate

import (
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/kubectl"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// Cluster checks that kubectl can reach the cluster and returns the k8s version.
func Cluster() (string, error) {
	if err := runner.MustExist("kubectl"); err != nil {
		return "", fmt.Errorf("kubectl not found: %w\nPlease ensure kubectl is installed and configured", err)
	}

	raw, err := kubectl.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("cannot reach Kubernetes cluster via kubectl: %w\nPlease ensure KUBECONFIG is set correctly and the cluster is accessible", err)
	}
	version := kubectl.BaseVersion(raw)

	fmt.Printf("  Kubernetes cluster found: %s\n", version)
	return version, nil
}

// Version checks that the cluster's k8s version matches the expected version.
func Version(clusterVersion, expectedVersion string) error {
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

// extractMinor returns the "major.minor" portion of a version string.
func extractMinor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

// Package existing handles validation of existing Kubernetes clusters.
package existing

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// kubectlVersionOutput represents the JSON output from kubectl version --output=json
type kubectlVersionOutput struct {
	ServerVersion *versionInfo `json:"serverVersion"`
}

// versionInfo contains the git version string
type versionInfo struct {
	GitVersion string `json:"gitVersion"`
}

// ValidateCluster checks that kubectl can reach the cluster and returns the k8s version.
// Returns an error if the cluster is not accessible.
func ValidateCluster() (string, error) {
	// Check that kubectl is available
	if err := runner.MustExist("kubectl"); err != nil {
		return "", fmt.Errorf("kubectl not found: %w\nPlease ensure kubectl is installed and configured", err)
	}

	// Try to get server version
	out, err := runner.Output("kubectl", "version", "--output=json")
	if err != nil {
		return "", fmt.Errorf("cannot reach Kubernetes cluster via kubectl: %w\nPlease ensure KUBECONFIG is set correctly and the cluster is accessible", err)
	}

	// Parse version from output (kubectl version returns gitVersion field)
	// Example: {"serverVersion":{"gitVersion":"v1.28.10+k3s1"}}
	version := extractVersion(out)
	if version == "" {
		return "", fmt.Errorf("could not parse Kubernetes version from kubectl output:\n%s", out)
	}

	fmt.Printf("  Kubernetes cluster found: %s\n", version)
	return version, nil
}

// extractVersion parses the Kubernetes version from kubectl version JSON output.
// Handles both standard k8s versions (v1.28.10) and k3s versions (v1.28.10+k3s1).
// Strips the +k3s suffix to return the base k8s version.
func extractVersion(output string) string {
	var versionOutput kubectlVersionOutput
	if err := json.Unmarshal([]byte(output), &versionOutput); err != nil {
		return ""
	}

	if versionOutput.ServerVersion == nil {
		return ""
	}

	fullVersion := versionOutput.ServerVersion.GitVersion
	if fullVersion == "" {
		return ""
	}

	// Strip +k3s suffix if present to get base k8s version
	if plusIdx := strings.Index(fullVersion, "+"); plusIdx != -1 {
		return fullVersion[:plusIdx] // e.g., v1.28.10
	}

	return fullVersion
}

// ValidateVersion checks that the cluster's k8s version matches the expected version.
// Returns an error if there's a mismatch (with a helpful message).
func ValidateVersion(clusterVersion, expectedVersion string) error {
	// Normalize versions (strip leading 'v')
	cluster := strings.TrimPrefix(clusterVersion, "v")
	expected := strings.TrimPrefix(expectedVersion, "v")

	// Extract major.minor for comparison
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

// extractMinor extracts the major.minor portion of a version string.
// e.g., "1.28.10" -> "1.28", "1.29.0" -> "1.29"
func extractMinor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

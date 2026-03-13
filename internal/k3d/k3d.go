// Package k3d handles detection, installation, and cluster creation for k3d.
package k3d

import (
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/runner"
)

const (
	k3dInstallScript = "https://raw.githubusercontent.com/k3d-io/k3d/main/install.sh"
	// k3d uses k3s images tagged as rancher/k3s:v1.28.10-k3s1
	// Note the dash instead of plus — Docker tags don't allow '+'
	k3sImageRepo = "rancher/k3s"
)

// EnsureInstalled checks for k3d and installs it if missing.
func EnsureInstalled() error {
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

// CreateCluster creates a new k3d cluster. Fails if a cluster with the same
// name already exists (fail-loud idempotency).
func CreateCluster(name, k3sVersion string) error {
	if err := ensureClusterAbsent(name); err != nil {
		return err
	}

	image := k3sVersionToImage(k3sVersion)
	fmt.Printf("  Creating k3d cluster %q with image %s\n", name, image)

	return runner.Run("k3d", "cluster", "create", name,
		"--image", image,
		"--wait",
		// Expose ports needed for Rancher ingress (Traefik loadbalancer)
		"-p", "80:80@loadbalancer",
		"-p", "443:443@loadbalancer",
	)
}

// KubeconfigMerge merges the k3d cluster kubeconfig into the default location.
func KubeconfigMerge(clusterName string) error {
	return runner.Run("k3d", "kubeconfig", "merge", clusterName, "--kubeconfig-merge-default")
}

// ensureClusterAbsent returns an error if a cluster with the given name exists.
func ensureClusterAbsent(name string) error {
	out, err := runner.Output("k3d", "cluster", "list", "--no-headers", "-o", "name")
	if err != nil {
		// k3d list failing is fine if no clusters exist yet (empty node)
		return nil //nolint:nilerr
	}
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
	return nil
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

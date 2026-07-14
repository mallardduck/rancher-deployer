// Package k3s handles detection and installation of k3s on bare Linux systems.
package k3s

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/runner"
)

const installScriptURL = "https://get.k3s.io"

// EnsureNotInstalled errors if k3s is already running, enforcing fail-loud
// idempotency semantics.
func EnsureNotInstalled() error {
	if isRunning() {
		return fmt.Errorf(
			"k3s is already running on this node\n" +
				"  Re-runs are not supported to avoid partial state.\n" +
				"  To reinstall, first run: sudo /usr/local/bin/k3s-uninstall.sh",
		)
	}
	return nil
}

// Install downloads and runs the official k3s install script, pinned to the
// given version tag (e.g. "v1.28.10+k3s1").
func Install(version string) error {
	if err := EnsureNotInstalled(); err != nil {
		return err
	}

	if err := runner.MustExist("curl"); err != nil {
		return err
	}

	fmt.Printf("  Installing k3s %s via official install script\n", version)

	// The install script respects INSTALL_K3S_VERSION env var.
	// We inline it as an env-prefixed command via sh -c.
	script := fmt.Sprintf(
		"curl -sfL %s | INSTALL_K3S_VERSION=%s sh -s - --write-kubeconfig-mode 644",
		installScriptURL, version,
	)
	return runner.RunSudo("sh", "-c", script)
}

// WaitReady blocks until the k3s node reports Ready.
func WaitReady() error {
	fmt.Println("  Waiting for k3s node to become Ready...")
	return runner.Kubectl("wait",
		"--for=condition=Ready",
		"node", "--all",
		"--timeout=120s",
	)
}

// KubeconfigPath returns the default kubeconfig written by k3s.
func KubeconfigPath() string {
	return "/etc/rancher/k3s/k3s.yaml"
}

// ExportKubeconfig copies the k3s kubeconfig to ~/.kube/config so that
// helm and kubectl work without extra flags.
func ExportKubeconfig() error {
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
	return runner.RunSudo("cp", KubeconfigPath(), dest)
}

// Uninstall runs the official k3s uninstall script, removing k3s and all its data.
func Uninstall() error {
	return runner.RunSudo("/usr/local/bin/k3s-uninstall.sh")
}

// isRunning returns true if the k3s service or process is active.
func isRunning() bool {
	// Check via systemd first
	if runner.Exists("systemctl") {
		out, err := exec.Command("systemctl", "is-active", "k3s").Output()
		if err == nil && strings.TrimSpace(string(out)) == "active" {
			return true
		}
	}
	// Fallback: check if the binary is present and a kubeconfig exists
	if runner.Exists("k3s") {
		out, err := exec.Command("k3s", "kubectl", "get", "nodes").CombinedOutput()
		if err == nil && len(out) > 0 {
			return true
		}
	}
	return false
}

// Package detect determines whether to use k3s or k3d based on the runtime
// environment: OS type and container runtime availability.
package detect

import (
	"os/exec"
	"runtime"
)

// goos and the check functions are vars so tests can substitute them.
var (
	goos        = runtime.GOOS
	checkDocker = defaultHasDocker
	checkPodman = defaultHasPodman
)

// InstallMode returns the recommended install mode ("k3s" or "k3d") and a
// human-readable reason for the selection.
//
// Detection rules (in priority order):
//  1. macOS → k3d (k3s doesn't run natively on macOS)
//  2. Linux + Docker/Podman available → k3d
//  3. Linux bare → k3s
func InstallMode() (mode, reason string) {
	if goos == "darwin" {
		return "k3d", "macOS detected — k3s requires Linux kernel"
	}

	if goos == "linux" {
		if checkDocker() {
			return "k3d", "Linux + Docker detected"
		}
		if checkPodman() {
			return "k3d", "Linux + Podman detected"
		}
		return "k3s", "Linux bare-metal/VM — no container runtime detected"
	}

	// Windows or unknown — best effort
	if checkDocker() {
		return "k3d", "container runtime detected"
	}
	return "k3d", "non-Linux OS — falling back to k3d"
}

func defaultHasDocker() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	err := exec.Command("docker", "info", "--format", "{{.ServerVersion}}").Run()
	return err == nil
}

func defaultHasPodman() bool {
	if _, err := exec.LookPath("podman"); err != nil {
		return false
	}
	err := exec.Command("podman", "info", "--format", "{{.Version.Version}}").Run()
	return err == nil
}

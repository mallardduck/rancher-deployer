package detect

import (
	"strings"
	"testing"
)

// TestInstallModeReturnsValidMode checks that InstallMode always returns a
// recognised mode and a non-empty reason string.
func TestInstallModeReturnsValidMode(t *testing.T) {
	mode, reason := InstallMode()
	if mode != "k3s" && mode != "k3d" {
		t.Errorf("InstallMode() mode = %q, want k3s or k3d", mode)
	}
	if reason == "" {
		t.Error("InstallMode() returned empty reason")
	}
}

// TestInstallModeDarwin verifies that darwin always maps to k3d.
func TestInstallModeDarwin(t *testing.T) {
	orig := goos
	defer func() { goos = orig }()
	goos = "darwin"

	mode, reason := InstallMode()
	if mode != "k3d" {
		t.Errorf("darwin: mode = %q, want k3d", mode)
	}
	if !strings.Contains(reason, "macOS") {
		t.Errorf("darwin: reason = %q, should mention macOS", reason)
	}
}

// TestInstallModeLinuxBare verifies that Linux without any container runtime
// selects k3s.
func TestInstallModeLinuxBare(t *testing.T) {
	origGOOS := goos
	origDocker := checkDocker
	origPodman := checkPodman
	defer func() {
		goos = origGOOS
		checkDocker = origDocker
		checkPodman = origPodman
	}()

	goos = "linux"
	checkDocker = func() bool { return false }
	checkPodman = func() bool { return false }

	mode, _ := InstallMode()
	if mode != "k3s" {
		t.Errorf("Linux bare: mode = %q, want k3s", mode)
	}
}

// TestInstallModeLinuxDocker verifies that Linux + Docker selects k3d.
func TestInstallModeLinuxDocker(t *testing.T) {
	origGOOS := goos
	origDocker := checkDocker
	defer func() {
		goos = origGOOS
		checkDocker = origDocker
	}()

	goos = "linux"
	checkDocker = func() bool { return true }

	mode, reason := InstallMode()
	if mode != "k3d" {
		t.Errorf("Linux+Docker: mode = %q, want k3d", mode)
	}
	if !strings.Contains(reason, "Docker") {
		t.Errorf("Linux+Docker: reason = %q, should mention Docker", reason)
	}
}

// TestInstallModeLinuxPodman verifies that Linux + Podman (no Docker) selects k3d.
func TestInstallModeLinuxPodman(t *testing.T) {
	origGOOS := goos
	origDocker := checkDocker
	origPodman := checkPodman
	defer func() {
		goos = origGOOS
		checkDocker = origDocker
		checkPodman = origPodman
	}()

	goos = "linux"
	checkDocker = func() bool { return false }
	checkPodman = func() bool { return true }

	mode, reason := InstallMode()
	if mode != "k3d" {
		t.Errorf("Linux+Podman: mode = %q, want k3d", mode)
	}
	if !strings.Contains(reason, "Podman") {
		t.Errorf("Linux+Podman: reason = %q, should mention Podman", reason)
	}
}

// TestInstallModeWindows verifies that non-Linux/non-macOS falls back to k3d.
func TestInstallModeWindows(t *testing.T) {
	origGOOS := goos
	origDocker := checkDocker
	defer func() {
		goos = origGOOS
		checkDocker = origDocker
	}()

	goos = "windows"
	checkDocker = func() bool { return false }

	mode, _ := InstallMode()
	if mode != "k3d" {
		t.Errorf("windows: mode = %q, want k3d", mode)
	}
}

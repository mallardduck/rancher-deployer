package detect

import (
	"strings"
	"testing"
)

// ── InstallMode ───────────────────────────────────────────────────────────────

func TestInstallModeReturnsValidMode(t *testing.T) {
	mode, reason := InstallMode()
	if mode != "k3s" && mode != "k3d" {
		t.Errorf("InstallMode() mode = %q, want k3s or k3d", mode)
	}
	if reason == "" {
		t.Error("InstallMode() returned empty reason")
	}
}

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

// ── ResolveMode ───────────────────────────────────────────────────────────────

func TestResolveMode(t *testing.T) {
	tests := []struct {
		name        string
		flag        string
		wantMode    string
		wantErr     bool
		errContains string
	}{
		{name: "explicit k3s", flag: "k3s", wantMode: "k3s"},
		{name: "explicit k3d", flag: "k3d", wantMode: "k3d"},
		{name: "explicit existing", flag: "existing", wantMode: "existing"},
		{name: "case insensitive K3S", flag: "K3S", wantMode: "k3s"},
		{name: "case insensitive K3D", flag: "K3D", wantMode: "k3d"},
		{name: "empty flag auto-detects", flag: ""},
		{name: "invalid mode", flag: "docker", wantErr: true, errContains: "invalid mode"},
		{name: "invalid mode typo", flag: "k3", wantErr: true, errContains: "invalid mode"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, reason, err := ResolveMode(tt.flag)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveMode(%q) error = %v, wantErr %v", tt.flag, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ResolveMode(%q) error = %v, want error containing %q", tt.flag, err, tt.errContains)
				}
				return
			}
			if tt.wantMode != "" && mode != tt.wantMode {
				t.Errorf("ResolveMode(%q) mode = %q, want %q", tt.flag, mode, tt.wantMode)
			}
			if tt.flag == "" {
				if mode != "k3s" && mode != "k3d" {
					t.Errorf("ResolveMode(\"\") auto-detected mode = %q, want k3s or k3d", mode)
				}
				if reason == "" {
					t.Error("ResolveMode(\"\") should return a reason for auto-detection")
				}
			}
			if tt.flag != "" && reason != "" {
				t.Errorf("ResolveMode(%q) returned unexpected reason: %q", tt.flag, reason)
			}
		})
	}
}

func TestResolveMode_AutoDetectConsistency(t *testing.T) {
	mode1, reason1, err1 := ResolveMode("")
	if err1 != nil {
		t.Fatalf("first auto-detect failed: %v", err1)
	}
	mode2, reason2, err2 := ResolveMode("")
	if err2 != nil {
		t.Fatalf("second auto-detect failed: %v", err2)
	}
	if mode1 != mode2 {
		t.Errorf("auto-detect not consistent: first=%q, second=%q", mode1, mode2)
	}
	if reason1 != reason2 {
		t.Errorf("auto-detect reason not consistent: first=%q, second=%q", reason1, reason2)
	}
}

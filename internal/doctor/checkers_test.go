package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBinaryChecker_Check(t *testing.T) {
	tests := []struct {
		name         string
		binary       string
		required     bool
		expectStatus CheckStatus
	}{
		{
			name:         "existing binary (sh)",
			binary:       "sh",
			required:     true,
			expectStatus: StatusPass,
		},
		{
			name:         "non-existent binary required",
			binary:       "nonexistent-binary-12345",
			required:     true,
			expectStatus: StatusFail,
		},
		{
			name:         "non-existent binary optional",
			binary:       "nonexistent-binary-12345",
			required:     false,
			expectStatus: StatusWarn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &BinaryChecker{
				binary:      tt.binary,
				displayName: tt.binary,
				required:    tt.required,
			}

			result := checker.Check(context.Background(), &CheckOptions{})

			if result.Status != tt.expectStatus {
				t.Errorf("Expected status %v, got %v", tt.expectStatus, result.Status)
			}

			if result.Name != tt.binary {
				t.Errorf("Expected name %q, got %q", tt.binary, result.Name)
			}

			if result.Category != "dependencies" {
				t.Errorf("Expected category 'dependencies', got %q", result.Category)
			}

			if result.Message == "" {
				t.Error("Expected non-empty message")
			}

			// Non-existent binaries should have remediation
			if result.Status != StatusPass && result.Remediation == "" {
				t.Error("Expected remediation for non-passing check")
			}
		})
	}
}

func TestBinaryChecker_Name(t *testing.T) {
	checker := &BinaryChecker{
		binary:      "kubectl",
		displayName: "kubectl",
	}

	if checker.Name() != "kubectl" {
		t.Errorf("Expected Name() to return 'kubectl', got %q", checker.Name())
	}
}

func TestBinaryChecker_Category(t *testing.T) {
	checker := &BinaryChecker{
		binary: "kubectl",
	}

	if checker.Category() != "dependencies" {
		t.Errorf("Expected Category() to return 'dependencies', got %q", checker.Category())
	}
}

func TestRuntimeChecker_Check(t *testing.T) {
	tests := []struct {
		name         string
		mode         string
		expectStatus CheckStatus
	}{
		{
			name:         "k3d mode on macOS",
			mode:         "k3d",
			expectStatus: StatusPass, // k3d works on all platforms
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := &RuntimeChecker{mode: tt.mode}
			result := checker.Check(context.Background(), &CheckOptions{})

			if result.Name != "runtime environment" {
				t.Errorf("Expected name 'runtime environment', got %q", result.Name)
			}

			if result.Category != "environment" {
				t.Errorf("Expected category 'environment', got %q", result.Category)
			}

			// On macOS, k3s mode should fail
			if runtime.GOOS == "darwin" && tt.mode == "k3s" {
				if result.Status != StatusFail {
					t.Error("Expected k3s mode to fail on macOS")
				}
			}
		})
	}
}

func TestRuntimeChecker_K3sOnMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("This test only runs on macOS")
	}

	checker := &RuntimeChecker{mode: "k3s"}
	result := checker.Check(context.Background(), &CheckOptions{})

	if result.Status != StatusFail {
		t.Errorf("Expected StatusFail for k3s on macOS, got %v", result.Status)
	}

	if result.Remediation == "" {
		t.Error("Expected remediation message")
	}
}

func TestEnvVarChecker_Check(t *testing.T) {
	tests := []struct {
		name         string
		varName      string
		setValue     bool
		required     bool
		expectStatus CheckStatus
	}{
		{
			name:         "set variable",
			varName:      "TEST_VAR_EXISTS",
			setValue:     true,
			required:     false,
			expectStatus: StatusPass,
		},
		{
			name:         "unset optional variable",
			varName:      "TEST_VAR_MISSING",
			setValue:     false,
			required:     false,
			expectStatus: StatusWarn,
		},
		{
			name:         "unset required variable",
			varName:      "TEST_VAR_REQUIRED",
			setValue:     false,
			required:     true,
			expectStatus: StatusFail,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setValue {
				os.Setenv(tt.varName, "test-value")
				defer os.Unsetenv(tt.varName)
			} else {
				os.Unsetenv(tt.varName)
			}

			checker := &EnvVarChecker{
				varName:     tt.varName,
				displayName: "test variable",
				purpose:     "testing",
				required:    tt.required,
			}

			result := checker.Check(context.Background(), &CheckOptions{})

			if result.Status != tt.expectStatus {
				t.Errorf("Expected status %v, got %v", tt.expectStatus, result.Status)
			}

			if result.Category != "environment" {
				t.Errorf("Expected category 'environment', got %q", result.Category)
			}
		})
	}
}

func TestEnvVarChecker_Fallback(t *testing.T) {
	// Test that fallback variable is checked
	primaryVar := "PRIMARY_VAR_TEST"
	fallbackVar := "FALLBACK_VAR_TEST"

	os.Unsetenv(primaryVar)
	os.Setenv(fallbackVar, "fallback-value")
	defer os.Unsetenv(fallbackVar)

	checker := &EnvVarChecker{
		varName:     primaryVar,
		fallback:    fallbackVar,
		displayName: "test variable",
		purpose:     "testing",
		required:    false,
	}

	result := checker.Check(context.Background(), &CheckOptions{})

	if result.Status != StatusPass {
		t.Errorf("Expected StatusPass when fallback is set, got %v", result.Status)
	}
}

func TestConfigFileChecker_Check(t *testing.T) {
	t.Run("existing kubeconfig", func(t *testing.T) {
		// Create a temporary kubeconfig
		tmpDir := t.TempDir()
		kubeconfigPath := filepath.Join(tmpDir, "config")
		err := os.WriteFile(kubeconfigPath, []byte("test"), 0600)
		if err != nil {
			t.Fatal(err)
		}

		// Set KUBECONFIG env var
		os.Setenv("KUBECONFIG", kubeconfigPath)
		defer os.Unsetenv("KUBECONFIG")

		checker := &ConfigFileChecker{
			displayName: "kubeconfig",
			required:    false,
		}

		result := checker.Check(context.Background(), &CheckOptions{})

		if result.Status != StatusPass {
			t.Errorf("Expected StatusPass for existing kubeconfig, got %v", result.Status)
		}
	})

	t.Run("missing kubeconfig", func(t *testing.T) {
		// Set KUBECONFIG to non-existent path
		os.Setenv("KUBECONFIG", "/nonexistent/path/to/kubeconfig")
		defer os.Unsetenv("KUBECONFIG")

		checker := &ConfigFileChecker{
			displayName: "kubeconfig",
			required:    false,
		}

		result := checker.Check(context.Background(), &CheckOptions{})

		if result.Status != StatusWarn {
			t.Errorf("Expected StatusWarn for missing kubeconfig, got %v", result.Status)
		}

		if result.Remediation == "" {
			t.Error("Expected remediation message")
		}
	})
}

func TestCacheDirectoryChecker_Check(t *testing.T) {
	checker := &CacheDirectoryChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	if result.Name != "cache directory" {
		t.Errorf("Expected name 'cache directory', got %q", result.Name)
	}

	if result.Category != "configuration" {
		t.Errorf("Expected category 'configuration', got %q", result.Category)
	}

	// Should pass or warn, but not fail
	if result.Status == StatusFail {
		t.Error("Cache directory check should not fail")
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}
}

func TestGetPackageManager(t *testing.T) {
	pm := getPackageManager()

	// Should return a valid package manager string
	validPMs := []string{"brew", "apt", "yum", "dnf", "pacman", "unknown"}
	found := false
	for _, valid := range validPMs {
		if pm == valid {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("getPackageManager() returned unexpected value: %q", pm)
	}

	// On macOS, should be brew
	if runtime.GOOS == "darwin" && pm != "brew" {
		t.Errorf("Expected 'brew' on macOS, got %q", pm)
	}
}

func TestGetInstallRemediation(t *testing.T) {
	tests := []struct {
		name   string
		binary string
	}{
		{"kubectl", "kubectl"},
		{"helm", "helm"},
		{"k3s", "k3s"},
		{"k3d", "k3d"},
		{"docker", "docker"},
		{"curl", "curl"},
		{"unknown binary", "some-unknown-tool"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remediation := getInstallRemediation(tt.binary)

			if remediation == "" {
				t.Errorf("Expected non-empty remediation for %q", tt.binary)
			}

			// k3s and k3d should mention automatic installation
			if tt.binary == "k3s" || tt.binary == "k3d" {
				if remediation != "k3s will be automatically installed during deployment" &&
					remediation != "k3d will be automatically installed during deployment" {
					t.Errorf("Expected automatic installation message for %q", tt.binary)
				}
			}
		})
	}
}

func TestContainerRuntimeChecker_Category(t *testing.T) {
	checker := &ContainerRuntimeChecker{}

	if checker.Category() != "dependencies" {
		t.Errorf("Expected category 'dependencies', got %q", checker.Category())
	}

	if checker.Name() != "container runtime" {
		t.Errorf("Expected name 'container runtime', got %q", checker.Name())
	}
}

func TestGetVersion(t *testing.T) {
	// Test with a binary that exists (sh is on all Unix-like systems)
	version := getVersion("sh")

	// Version might be empty if sh doesn't support --version
	// but getVersion should not panic
	t.Logf("sh version: %q", version)

	// Test with non-existent binary
	version = getVersion("nonexistent-binary-xyz")
	if version != "" {
		t.Errorf("Expected empty version for non-existent binary, got %q", version)
	}
}

func TestExecutionLocation_Constants(t *testing.T) {
	tests := []struct {
		name     string
		location ExecutionLocation
		want     string
	}{
		{"LocationLocal", LocationLocal, "local"},
		{"LocationRemote", LocationRemote, "remote"},
		{"LocationBoth", LocationBoth, "both"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.location) != tt.want {
				t.Errorf("Expected %q, got %q", tt.want, tt.location)
			}
		})
	}
}

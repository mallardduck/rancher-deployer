package doctor

import (
	"context"
	"testing"
)

func TestClusterAccessChecker_Name(t *testing.T) {
	checker := &ClusterAccessChecker{}

	if checker.Name() != "Kubernetes cluster access" {
		t.Errorf("Expected name 'Kubernetes cluster access', got %q", checker.Name())
	}
}

func TestClusterAccessChecker_Category(t *testing.T) {
	checker := &ClusterAccessChecker{}

	if checker.Category() != "state" {
		t.Errorf("Expected category 'state', got %q", checker.Category())
	}
}

func TestClusterAccessChecker_Check(t *testing.T) {
	checker := &ClusterAccessChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	// Should return some result (pass or warn)
	if result.Name == "" {
		t.Error("Expected non-empty name")
	}

	if result.Category != "state" {
		t.Errorf("Expected category 'state', got %q", result.Category)
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Should not fail (at worst, warn about no cluster)
	if result.Status == StatusFail {
		t.Error("ClusterAccessChecker should not return StatusFail")
	}
}

func TestK3sServiceChecker_Name(t *testing.T) {
	checker := &K3sServiceChecker{}

	if checker.Name() != "k3s service status" {
		t.Errorf("Expected name 'k3s service status', got %q", checker.Name())
	}
}

func TestK3sServiceChecker_Category(t *testing.T) {
	checker := &K3sServiceChecker{}

	if checker.Category() != "state" {
		t.Errorf("Expected category 'state', got %q", checker.Category())
	}
}

func TestK3sServiceChecker_Check(t *testing.T) {
	checker := &K3sServiceChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	if result.Name == "" {
		t.Error("Expected non-empty name")
	}

	if result.Category != "state" {
		t.Errorf("Expected category 'state', got %q", result.Category)
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Should not fail (systemctl might not be available)
	if result.Status == StatusFail {
		t.Error("K3sServiceChecker should not return StatusFail")
	}
}

func TestK3dClusterChecker_Name(t *testing.T) {
	checker := &K3dClusterChecker{}

	if checker.Name() != "k3d clusters" {
		t.Errorf("Expected name 'k3d clusters', got %q", checker.Name())
	}
}

func TestK3dClusterChecker_Category(t *testing.T) {
	checker := &K3dClusterChecker{}

	if checker.Category() != "state" {
		t.Errorf("Expected category 'state', got %q", checker.Category())
	}
}

func TestK3dClusterChecker_Check(t *testing.T) {
	checker := &K3dClusterChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	if result.Name == "" {
		t.Error("Expected non-empty name")
	}

	if result.Category != "state" {
		t.Errorf("Expected category 'state', got %q", result.Category)
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Should pass (either k3d not installed or no clusters found)
	if result.Status == StatusFail {
		t.Error("K3dClusterChecker should not return StatusFail")
	}
}

func TestRancherInstallationChecker_Name(t *testing.T) {
	checker := &RancherInstallationChecker{}

	if checker.Name() != "Rancher installation" {
		t.Errorf("Expected name 'Rancher installation', got %q", checker.Name())
	}
}

func TestRancherInstallationChecker_Category(t *testing.T) {
	checker := &RancherInstallationChecker{}

	if checker.Category() != "state" {
		t.Errorf("Expected category 'state', got %q", checker.Category())
	}
}

func TestRancherInstallationChecker_Check(t *testing.T) {
	checker := &RancherInstallationChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	if result.Name == "" {
		t.Error("Expected non-empty name")
	}

	if result.Category != "state" {
		t.Errorf("Expected category 'state', got %q", result.Category)
	}

	if result.Message == "" {
		t.Error("Expected non-empty message")
	}

	// Should pass (either helm not available or Rancher not installed)
	if result.Status == StatusFail {
		t.Error("RancherInstallationChecker should not return StatusFail")
	}
}

func TestStateCheckers_NoKubectl(t *testing.T) {
	// Test that ClusterAccessChecker handles missing kubectl gracefully
	checker := &ClusterAccessChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	// Should not panic or fail hard
	if result.Status == StatusFail {
		t.Error("Should not fail when kubectl is missing")
	}
}

func TestStateCheckers_NoHelm(t *testing.T) {
	// Test that RancherInstallationChecker handles missing helm gracefully
	checker := &RancherInstallationChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	// Should not panic or fail hard
	if result.Status == StatusFail {
		t.Error("Should not fail when helm is missing")
	}
}

func TestStateCheckers_NoK3d(t *testing.T) {
	// Test that K3dClusterChecker handles missing k3d gracefully
	checker := &K3dClusterChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	// Should not panic
	if result.Message == "" {
		t.Error("Expected non-empty message even when k3d is not installed")
	}
}

func TestStateCheckers_NoSystemctl(t *testing.T) {
	// Test that K3sServiceChecker handles missing systemctl gracefully
	checker := &K3sServiceChecker{}
	result := checker.Check(context.Background(), &CheckOptions{})

	// Should not panic or fail hard
	if result.Status == StatusFail {
		t.Error("Should not fail when systemctl is missing")
	}
}

func TestStateCheckers_AllReturnValidResults(t *testing.T) {
	checkers := []Checker{
		&ClusterAccessChecker{},
		&K3sServiceChecker{},
		&K3dClusterChecker{},
		&RancherInstallationChecker{},
	}

	for _, checker := range checkers {
		t.Run(checker.Name(), func(t *testing.T) {
			result := checker.Check(context.Background(), &CheckOptions{})

			if result.Name == "" {
				t.Error("Expected non-empty name")
			}

			if result.Category == "" {
				t.Error("Expected non-empty category")
			}

			if result.Message == "" {
				t.Error("Expected non-empty message")
			}

			// All state checkers should return valid status
			validStatus := result.Status == StatusPass ||
				result.Status == StatusWarn ||
				result.Status == StatusFail

			if !validStatus {
				t.Errorf("Invalid status: %v", result.Status)
			}
		})
	}
}

func TestStateCheckers_ContextCancellation(t *testing.T) {
	// Test that state checkers respect context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	checkers := []Checker{
		&ClusterAccessChecker{},
		&K3sServiceChecker{},
		&K3dClusterChecker{},
		&RancherInstallationChecker{},
	}

	for _, checker := range checkers {
		t.Run(checker.Name(), func(t *testing.T) {
			// Should not panic even with cancelled context
			result := checker.Check(ctx, &CheckOptions{})

			if result.Name == "" {
				t.Error("Expected non-empty name even with cancelled context")
			}
		})
	}
}

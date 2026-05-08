package k3s

import (
	"testing"
)

// ── KubeconfigPath ────────────────────────────────────────────────────────────

func TestKubeconfigPath(t *testing.T) {
	got := KubeconfigPath()
	want := "/etc/rancher/k3s/k3s.yaml"

	if got != want {
		t.Errorf("KubeconfigPath() = %q, want %q", got, want)
	}

	// Verify it returns a consistent value
	got2 := KubeconfigPath()
	if got != got2 {
		t.Error("KubeconfigPath() should return consistent value")
	}
}

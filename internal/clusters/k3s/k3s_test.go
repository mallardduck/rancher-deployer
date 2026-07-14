package k3s

import (
	"testing"
)

func TestKubeconfigPath(t *testing.T) {
	p := NewProvider()
	got := p.KubeconfigPath()
	want := "/etc/rancher/k3s/k3s.yaml"
	if got != want {
		t.Errorf("KubeconfigPath() = %q, want %q", got, want)
	}
}

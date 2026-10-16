package expose

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	for _, name := range []string{"", "sslip", "SSLIP"} {
		m, err := New(name)
		if err != nil || m.Name() != SSLIP {
			t.Errorf("New(%q) = %v, %v; want sslip", name, m, err)
		}
	}
	if _, err := New("bogus"); err == nil {
		t.Error("New should reject unknown modes")
	}
}

func TestResolve_HostnamePrecedence(t *testing.T) {
	m, _ := New("")
	tests := []struct {
		name     string
		flag     string
		setFlags []string
		want     string // exact, or substring when wantSub
		wantSub  bool
	}{
		{"flag wins over --set", "a.example.com", []string{"hostname=b.com"}, "a.example.com", false},
		{"--set hostname used", "", []string{"foo=bar", "hostname=b.com"}, "b.com", false},
		{"sslip derived", "", []string{"foo=bar"}, ".sslip.io", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := Resolve(m, tt.flag, tt.setFlags)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantSub && !strings.Contains(e.Hostname, tt.want) || !tt.wantSub && e.Hostname != tt.want {
				t.Errorf("Hostname = %q, want %q", e.Hostname, tt.want)
			}
		})
	}
}

func TestMergeSets(t *testing.T) {
	e := Exposure{SetFlags: []string{"tls=external", "ingress.x=1"}}
	got := e.MergeSets([]string{"tls=ingress", "foo=bar"})
	want := []string{"tls=ingress", "foo=bar", "ingress.x=1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeSets() = %v, want %v", got, want)
	}
}

func TestSSLIP_NoStepsOrVerify(t *testing.T) {
	m, _ := New("")
	if steps := m.Steps(Exposure{}, Env{}); len(steps) != 0 {
		t.Errorf("sslip Steps() = %v, want none", steps)
	}
	if err := m.Verify(context.Background(), Exposure{}); err != nil {
		t.Errorf("sslip Verify() = %v, want nil", err)
	}
}

func TestSSLIP_NoClusterConfig(t *testing.T) {
	m, _ := New("")
	if cfg := m.ClusterConfig(Env{ClusterMode: "k3s"}); len(cfg) != 0 {
		t.Errorf("sslip ClusterConfig() = %v, want none", cfg)
	}
}

func TestNew_NgrokNotImplemented(t *testing.T) {
	_, err := New("ngrok")
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("New(ngrok) error = %v, want a not-implemented error", err)
	}
}

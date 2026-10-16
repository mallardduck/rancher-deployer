package expose

import (
	"context"
	"errors"
	"gopkg.in/yaml.v3"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPangolinResolve(t *testing.T) {
	m, _ := New(Pangolin)
	if _, err := m.Resolve(""); err == nil {
		t.Error("want error without a hostname")
	}
	for _, bad := range []string{"https://rancher.example.com", "rancher.example.com/x"} {
		if _, err := m.Resolve(bad); err == nil {
			t.Errorf("Resolve(%q) should be rejected", bad)
		}
	}
	e, err := m.Resolve("rancher.example.com")
	if err != nil || e.Hostname != "rancher.example.com" {
		t.Fatalf("Resolve = %+v, %v", e, err)
	}
	if got := e.MergeSets([]string{"tls=ingress"}); len(got) != 1 || got[0] != "tls=ingress" {
		t.Errorf("user tls setting must win, got %v", got)
	}
	if got := e.MergeSets(nil); len(got) != 1 || got[0] != "tls=external" {
		t.Errorf("default sets = %v, want tls=external", got)
	}
}

func TestPangolinSteps(t *testing.T) {
	m := newPangolin()
	steps := m.Steps(Exposure{Hostname: "rancher.example.com"}, Env{ClusterMode: "k3s"})
	if len(steps) != 2 || steps[0].Check != nil || steps[1].Check == nil {
		t.Fatalf("unexpected steps: %+v", steps)
	}
	if !strings.Contains(strings.Join(steps[1].Lines, "\n"), "http://127.0.0.1:80") {
		t.Error("resource step should state the k3s target")
	}
	existing := m.Steps(Exposure{Hostname: "h"}, Env{ClusterMode: "existing"})
	if strings.Contains(strings.Join(existing[1].Lines, "\n"), "127.0.0.1") {
		t.Error("existing mode must not claim a loopback target")
	}

	m.lookupHost = func(context.Context, string) ([]string, error) { return nil, errors.New("nxdomain") }
	failing := m.Steps(Exposure{Hostname: "h"}, Env{})
	if err := failing[1].Check(context.Background()); err == nil {
		t.Error("DNS failure should fail the check")
	}
	m.lookupHost = func(context.Context, string) ([]string, error) { return []string{"1.2.3.4"}, nil }
	if err := m.Steps(Exposure{Hostname: "h"}, Env{})[1].Check(context.Background()); err != nil {
		t.Errorf("resolving host should pass: %v", err)
	}
}

func TestPangolinVerify(t *testing.T) {
	status := http.StatusBadGateway
	pong := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status == http.StatusOK {
			body := "pong"
			if !pong {
				body = "<html>login</html>"
			}
			_, _ = w.Write([]byte(body))
		}
	}))
	defer srv.Close()

	m := newPangolin()
	m.client = srv.Client()
	m.pingURL = func(string) string { return srv.URL + "/ping" }
	m.timeout, m.interval = 50*time.Millisecond, 10*time.Millisecond
	e := Exposure{Hostname: "rancher.example.com"}

	for code, want := range map[int]string{
		http.StatusBadGateway: "resource target",
		http.StatusNotFound:   "Host header",
		http.StatusTeapot:     "unexpected HTTP 418",
	} {
		status = code
		err := m.Verify(context.Background(), e)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("status %d: err = %v, want containing %q", code, err, want)
		}
	}
	status = http.StatusUnauthorized
	if err := m.Verify(context.Background(), e); err == nil || !strings.Contains(err.Error(), "SSO") {
		t.Errorf("401 must fail with the SSO hint, got %v", err)
	}
	status, pong = http.StatusOK, false
	if err := m.Verify(context.Background(), e); err == nil || !strings.Contains(err.Error(), "pong") {
		t.Errorf("a 200 that is not pong must fail, got %v", err)
	}
	status, pong = http.StatusOK, true
	if err := m.Verify(context.Background(), e); err != nil {
		t.Errorf("200 pong should verify: %v", err)
	}
}

func TestPangolinClusterConfig(t *testing.T) {
	m := newPangolin()
	for _, mode := range []string{"k3s", "k3d"} {
		cfg := m.ClusterConfig(Env{ClusterMode: mode})
		if len(cfg) != 1 || !strings.Contains(cfg[0], "forwardedHeaders") || !strings.Contains(cfg[0], "kind: HelmChartConfig") {
			t.Errorf("%s: want the Traefik forwardedHeaders config, got %v", mode, cfg)
		}
	}
	if cfg := m.ClusterConfig(Env{ClusterMode: "existing"}); len(cfg) != 0 {
		t.Errorf("existing clusters must not be modified, got %v", cfg)
	}
	existing := m.Steps(Exposure{Hostname: "h"}, Env{ClusterMode: "existing"})
	if !strings.Contains(strings.Join(existing[1].Lines, "\n"), "X-Forwarded-Proto") {
		t.Error("existing mode should tell the user about X-Forwarded-Proto")
	}
}

func TestTraefikManifestIsValid(t *testing.T) {
	var doc struct {
		Kind string `yaml:"kind"`
		Spec struct {
			ValuesContent string `yaml:"valuesContent"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal([]byte(traefikTrustForwarded), &doc); err != nil || doc.Kind != "HelmChartConfig" {
		t.Fatalf("manifest invalid: kind=%q err=%v", doc.Kind, err)
	}
	var values struct {
		Ports struct {
			Web struct {
				ForwardedHeaders struct {
					Insecure bool `yaml:"insecure"`
				} `yaml:"forwardedHeaders"`
			} `yaml:"web"`
		} `yaml:"ports"`
	}
	if err := yaml.Unmarshal([]byte(doc.Spec.ValuesContent), &values); err != nil || !values.Ports.Web.ForwardedHeaders.Insecure {
		t.Fatalf("valuesContent invalid: %+v err=%v", values, err)
	}
}

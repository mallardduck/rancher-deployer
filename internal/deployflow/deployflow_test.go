package deployflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func baseOptions() Options {
	return Options{
		RancherVersion: "2.8.5",
		Channel:        "stable",
		FailurePolicy:  "abort",
		Title:          "Test",
		SelectCluster: func() (Cluster, string, string, error) {
			return nil, "", "", errors.New("select cluster reached")
		},
	}
}

func TestRun_ValidationFailsBeforeAnyStep(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{"bad channel", func(o *Options) { o.Channel = "nope" }, "channel"},
		{"bad failure policy", func(o *Options) { o.FailurePolicy = "nope" }, "failure"},
		{"missing rancher version", func(o *Options) { o.RancherVersion = "" }, "--rancher-version is required"},
		{"head without prime needs version", func(o *Options) {
			o.RancherVersion, o.Channel, o.Commit = "", "head", "abc1234"
		}, "--rancher-version is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := baseOptions()
			tt.mutate(&o)
			err := Run(context.Background(), o)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "select cluster reached") {
				t.Error("validation should fail before the cluster is selected")
			}
		})
	}
}

func TestRun_PreflightFailureAbortsBeforeClusterSelection(t *testing.T) {
	o := baseOptions()
	o.Preflight = func(context.Context) error { return errors.New("preflight failed") }
	err := Run(context.Background(), o)
	if err == nil || err.Error() != "preflight failed" {
		t.Fatalf("err = %v, want preflight failed", err)
	}
}

func TestRun_PreflightPassesThenSelectsCluster(t *testing.T) {
	o := baseOptions()
	called := false
	o.Preflight = func(context.Context) error { called = true; return nil }
	err := Run(context.Background(), o)
	if !called {
		t.Error("preflight was not called")
	}
	if err == nil || err.Error() != "select cluster reached" {
		t.Fatalf("err = %v, want select cluster reached", err)
	}
}

// Package provider defines the Provider interface that each cluster backend must implement.
package provider

import (
	"context"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm"
)

// SetupOptions carries the resolved cluster version needed by Setup.
type SetupOptions struct {
	ClusterVersion string
}

// TeardownOptions is reserved for future teardown configuration.
type TeardownOptions struct{}

// Provider is the interface each cluster backend must satisfy.
// It owns cluster lifecycle, the helm mechanism, and its own prerequisite checks.
type Provider interface {
	Name() string

	// ResolveClusterVersion resolves (or validates, for existing clusters) the
	// concrete cluster release tag for the given k8s version string.
	// For k3d/k3s this calls the GitHub release resolver; for existing clusters
	// it connects via kubectl and validates compatibility.
	ResolveClusterVersion(ctx context.Context, k8sVersion string) (string, error)

	Setup(ctx context.Context, opts SetupOptions) error
	Teardown(ctx context.Context, opts TeardownOptions) error

	// KubeconfigPath returns the kubeconfig path used by this provider, or ""
	// when the provider relies on the default ~/.kube/config location.
	KubeconfigPath() string

	Helm() helm.Backend

	// Checkers returns the static prerequisite checks for this provider.
	// These are the binary/environment checks that can run before any cluster
	// connection is established (i.e. what the doctor command needs up front).
	Checkers() []doctor.Checker
}

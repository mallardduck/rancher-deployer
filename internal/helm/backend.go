// Package helm provides the Backend interface and its implementations for installing Helm charts.
package helm

import "github.com/mallardduck/rancher-deployer/internal/rancher"

// Backend abstracts how Helm charts are installed, allowing different
// implementations (CLI binary vs in-cluster helm-controller CRDs).
type Backend interface {
	EnsureRepo(name, url string, yes bool) error
	Install(namespace string, chart rancher.Chart, values rancher.HelmValues) error
	Upgrade(namespace string, chart rancher.Chart, values rancher.HelmValues) error
	GetValues(release, namespace string) (string, error)
	IsInstalled(release, namespace string) (bool, error)
	InstalledVersion(namespace string) (string, error)
	InstallCertManager(version string) error
	WaitReady(namespace string) error
}

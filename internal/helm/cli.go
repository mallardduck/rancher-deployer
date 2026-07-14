package helm

import (
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/rancher"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

var _ Backend = (*CLI)(nil)

// CLI implements Backend using the helm binary on the host.
type CLI struct{}

// NewCLI returns a helm Backend that delegates to the helm CLI.
func NewCLI() *CLI { return &CLI{} }

func (c *CLI) EnsureRepo(name, url string, yes bool) error {
	return rancher.EnsureHelmRepo(name, url, yes)
}

func (c *CLI) Install(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	return rancher.Install(namespace, chart, values)
}

func (c *CLI) Upgrade(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	return rancher.Upgrade(namespace, chart, values)
}

func (c *CLI) GetValues(release, namespace string) (string, error) {
	return runner.Output("helm", "get", "values", release,
		"--namespace", namespace, "--output", "yaml")
}

func (c *CLI) IsInstalled(release, namespace string) (bool, error) {
	out, _ := runner.Output("helm", "list", "-n", namespace, "--short")
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == release {
			return true, nil
		}
	}
	return false, nil
}

func (c *CLI) InstalledVersion(namespace string) (string, error) {
	return rancher.InstalledVersion(namespace)
}

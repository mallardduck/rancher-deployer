package helm

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mallardduck/rancher-deployer/internal/kubectl"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// rancherRelease is the Helm release name Rancher is installed under.
const rancherRelease = "rancher"

var _ Backend = (*CLI)(nil)

// CLI implements Backend using the helm binary on the host.
type CLI struct{}

// NewCLI returns a helm Backend that delegates to the helm CLI.
func NewCLI() *CLI { return &CLI{} }

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

func (c *CLI) WaitReady(namespace string) error {
	return rancher.WaitReady(namespace)
}

// InstallCertManager applies cert-manager at the given version and waits for
// its deployments to roll out.
func (c *CLI) InstallCertManager(version string) error {
	// Check if already installed — fail loud
	if exists, _ := kubectl.Exists("namespace", "cert-manager", ""); exists {
		return fmt.Errorf(
			"cert-manager namespace already exists\n" +
				"  Re-runs are not supported. Remove cert-manager before retrying",
		)
	}

	url := fmt.Sprintf(
		"https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml",
		version,
	)
	if err := kubectl.ApplyURL(url); err != nil {
		return fmt.Errorf("failed to apply cert-manager: %w", err)
	}

	// Wait for all three cert-manager deployments. The webhook must be fully
	// ready before Rancher's Helm install can succeed — it validates CRDs and
	// will 503 if the endpoint isn't up yet.
	for _, deploy := range []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"} {
		if err := kubectl.RolloutStatus("cert-manager", deploy, 120*time.Second); err != nil {
			return fmt.Errorf("cert-manager deployment %q did not become ready: %w", deploy, err)
		}
	}
	return nil
}

// EnsureRepo makes the named Helm repo available with the correct URL:
//   - Not present → add it and fetch the index.
//   - Present with matching URL → fetch the index (no-op on the registration).
//   - Present with a different URL → prompt the user (or auto-confirm when yes=true),
//     then re-register with --force-update and fetch the index.
func (c *CLI) EnsureRepo(repoName, repoURL string, yes bool) error {
	out, err := runner.Output("helm", "repo", "list", "-o", "json")
	if err != nil {
		// helm repo list exits non-zero when no repos are configured yet.
		out = "[]"
	}

	var repos []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &repos); err != nil {
		return fmt.Errorf("could not parse helm repo list: %w", err)
	}

	for _, r := range repos {
		if r.Name != repoName {
			continue
		}
		if r.URL == repoURL {
			fmt.Printf("    Helm repo %q already registered — updating index\n", repoName)
			return runner.Helm("repo", "update", repoName)
		}
		// Repo exists but points to a different URL.
		fmt.Printf("    Warning: Helm repo %q is registered with a different URL:\n", repoName)
		fmt.Printf("      current : %s\n", r.URL)
		fmt.Printf("      wanted  : %s\n", repoURL)
		if !yes {
			fmt.Printf("    Update repo to new URL? [y/N]: ")
			var answer string
			_, _ = fmt.Fscanln(os.Stdin, &answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				return fmt.Errorf(
					"helm repo %q has a conflicting URL — update it manually or re-run with --yes",
					repoName,
				)
			}
		}
		if err := runner.Helm("repo", "add", "--force-update", repoName, repoURL); err != nil {
			return fmt.Errorf("helm repo update failed: %w", err)
		}
		return runner.Helm("repo", "update", repoName)
	}

	// Repo not registered yet.
	if err := runner.Helm("repo", "add", repoName, repoURL); err != nil {
		return fmt.Errorf("helm repo add failed: %w", err)
	}
	return runner.Helm("repo", "update", repoName)
}

// Install installs Rancher via Helm. The repo must already be registered —
// call EnsureHelmRepo before calling this.
func (c *CLI) Install(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	if err := runner.MustExist("helm"); err != nil {
		return err
	}

	// Check Rancher is not already installed
	if err := ensureRancherAbsent(namespace); err != nil {
		return err
	}

	// Create namespace
	if err := kubectl.CreateNamespace(namespace); err != nil {
		return fmt.Errorf("namespace creation failed: %w", err)
	}

	// Assemble helm install command (no --wait, we'll monitor progress separately)
	args := []string{
		"install", chart.ChartName,
		fmt.Sprintf("%s/%s", chart.RepoName, chart.ChartName),
		"--namespace", namespace,
		"--version", chart.Version,
	}

	if chart.IsPrerelease {
		args = append(args, "--devel")
	}

	if values.ValuesFile != "" {
		args = append(args, "--values", values.ValuesFile)
	}
	for _, s := range values.SetFlags {
		args = append(args, "--set", s)
	}
	if values.FailurePolicy == rancher.FailurePolicyReinstall {
		args = append(args, "--atomic")
	}

	if err := runner.Helm(args...); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  Helm release created. Monitoring deployment progress...")
	fmt.Println()

	return nil
}

// InstalledVersion returns the currently installed Rancher app version from
// Helm (e.g. "2.8.5"). Returns an error if Rancher is not installed.
func (c *CLI) InstalledVersion(namespace string) (string, error) {
	out, err := runner.Output("helm", "list", "-n", namespace, "-o", "json")
	if err != nil {
		return "", fmt.Errorf("could not query helm releases: %w", err)
	}

	var releases []struct {
		Name       string `json:"name"`
		AppVersion string `json:"app_version"`
	}
	if err := json.Unmarshal([]byte(out), &releases); err != nil {
		return "", fmt.Errorf("could not parse helm list output: %w", err)
	}

	for _, r := range releases {
		if r.Name == rancherRelease {
			return strings.TrimPrefix(r.AppVersion, "v"), nil
		}
	}
	return "", fmt.Errorf("rancher is not installed in namespace %q — nothing to upgrade", namespace)
}

// Upgrade runs helm upgrade for an existing Rancher installation.
// Previous values are fetched from the existing release and applied via --values.
// This avoids issues with --reuse-values where new chart defaults might be missing.
// Explicit --set flags and a values file (if provided) are applied on top.
// The repo must already be registered — call EnsureRepo before this.
func (c *CLI) Upgrade(namespace string, chart rancher.Chart, values rancher.HelmValues) error {
	if err := runner.MustExist("helm"); err != nil {
		return err
	}

	// 1. Fetch current values to a temporary file
	tmpFile, err := os.CreateTemp("", "rancher-values-*.yaml")
	if err != nil {
		return fmt.Errorf("could not create temporary values file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }() //nolint:gosec // path comes from os.CreateTemp, not user input

	out, err := runner.Output("helm", "get", "values", chart.ChartName, "--namespace", namespace, "--output", "yaml")
	if err != nil {
		return fmt.Errorf("could not fetch current helm values: %w", err)
	}

	if _, err := tmpFile.WriteString(out); err != nil {
		return fmt.Errorf("could not write current values to temporary file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("could not close temporary values file: %w", err)
	}

	// 2. Build upgrade command (no --wait, caller will monitor progress)
	args := []string{
		"upgrade", chart.ChartName,
		fmt.Sprintf("%s/%s", chart.RepoName, chart.ChartName),
		"--namespace", namespace,
		"--version", chart.Version,
		"--values", tmpFile.Name(), // applied first
	}

	if chart.IsPrerelease {
		args = append(args, "--devel")
	}

	if values.ValuesFile != "" {
		args = append(args, "--values", values.ValuesFile)
	}
	for _, s := range values.SetFlags {
		args = append(args, "--set", s)
	}
	if values.FailurePolicy == rancher.FailurePolicyReinstall {
		args = append(args, "--atomic")
	}

	if err := runner.Helm(args...); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  Helm release upgraded. Monitoring rollout progress...")
	fmt.Println()

	return nil
}

// ensureRancherAbsent returns an error if a Rancher Helm release already exists.
func ensureRancherAbsent(namespace string) error {
	out, _ := runner.Output("helm", "list", "-n", namespace, "--short")
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == rancherRelease {
			return fmt.Errorf(
				"rancher is already installed in namespace %q\n"+
					"  Re-runs are not supported. To remove:\n"+
					"    helm uninstall rancher -n %s",
				namespace, namespace,
			)
		}
	}
	return nil
}

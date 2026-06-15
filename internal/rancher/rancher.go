// Package rancher handles everything Rancher-specific: chart references,
// Helm values construction, cert-manager installation, and the Rancher
// Helm install/upgrade itself.
package rancher

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// ── Chart reference ──────────────────────────────────────────────────────────

// Channel selects the release channel (and therefore the Helm repo) to use.
const (
	ChannelStable = "stable" // GA releases only
	ChannelLatest = "latest" // RC + latest GA  (alias: "rc")
	ChannelAlpha  = "alpha"  // Alpha builds

	chartName = "rancher"
)

// chart repos indexed by [prime][channel]
var repoURLs = map[bool]map[string]string{
	false: {
		ChannelStable: "https://releases.rancher.com/server-charts/stable",
		ChannelLatest: "https://releases.rancher.com/server-charts/latest",
		ChannelAlpha:  "https://releases.rancher.com/server-charts/alpha",
	},
	true: {
		ChannelStable: "https://charts.rancher.com/server-charts/prime",
		ChannelLatest: "https://charts.optimus.rancher.io/server-charts/latest",
		ChannelAlpha:  "https://charts.optimus.rancher.io/server-charts/alpha",
	},
}

var repoNames = map[bool]map[string]string{
	false: {
		ChannelStable: "rancher-stable",
		ChannelLatest: "rancher-latest",
		ChannelAlpha:  "rancher-alpha",
	},
	true: {
		ChannelStable: "rancher-prime",
		ChannelLatest: "rancher-prime-latest",
		ChannelAlpha:  "rancher-prime-alpha",
	},
}

// Chart holds the Helm repository and chart reference for a Rancher edition.
type Chart struct {
	RepoName     string
	RepoURL      string
	ChartName    string
	Version      string
	IsPrerelease bool
}

func (c Chart) String() string {
	return fmt.Sprintf("%s/%s @ %s", c.RepoName, c.ChartName, c.Version)
}

// NormaliseChannel maps aliases to canonical channel names and returns an error
// for unknown values.
func NormaliseChannel(channel string) (string, error) {
	switch strings.ToLower(channel) {
	case ChannelStable, "ga":
		return ChannelStable, nil
	case ChannelLatest, "rc":
		return ChannelLatest, nil
	case ChannelAlpha:
		return ChannelAlpha, nil
	default:
		return "", fmt.Errorf("unknown channel %q: must be stable, latest, rc, or alpha", channel)
	}
}

// ChartRef returns the appropriate Chart for the given edition and channel.
func ChartRef(prime, prerelease bool, channel, rancherVersion string) Chart {
	return Chart{
		RepoName:     repoNames[prime][channel],
		RepoURL:      repoURLs[prime][channel],
		ChartName:    chartName,
		Version:      rancherVersion,
		IsPrerelease: prerelease,
	}
}

// ── Helm values ──────────────────────────────────────────────────────────────

// HelmValues holds everything needed to construct the Helm install command.
type HelmValues struct {
	ValuesFile string   // --values <file>
	SetFlags   []string // --set key=value entries
	Hostname   string   // resolved hostname
}

// BuildHelmValues validates the values file (if given) and assembles the
// full set of --set flags, injecting hostname and bootstrapPassword if not
// already set by the caller.
func BuildHelmValues(valuesFile string, setFlags []string, hostname, namespace, bootstrapPassword string) (HelmValues, error) {
	if valuesFile != "" {
		if _, err := os.Stat(valuesFile); err != nil {
			return HelmValues{}, fmt.Errorf("--values-file %q: %w", valuesFile, err)
		}
	}

	resolvedHostname, err := resolveHostname(hostname, setFlags)
	if err != nil {
		return HelmValues{}, err
	}

	sets := injectHostname(setFlags, resolvedHostname)
	sets = injectIfAbsent(sets, "bootstrapPassword", bootstrapPassword)

	return HelmValues{
		ValuesFile: valuesFile,
		SetFlags:   sets,
		Hostname:   resolvedHostname,
	}, nil
}

// injectIfAbsent appends key=value to sets if no entry already starts with "key=".
func injectIfAbsent(sets []string, key, value string) []string {
	prefix := key + "="
	for _, s := range sets {
		if strings.HasPrefix(s, prefix) {
			return sets
		}
	}
	return append(sets, prefix+value)
}

// resolveHostname returns a usable Rancher hostname in this priority order:
//  1. Explicit --hostname flag
//  2. hostname= found in --set flags
//  3. Auto-detect via local interface IP + sslip.io
//  4. Fallback to "rancher.127.0.0.1.sslip.io" with a warning
func resolveHostname(explicit string, setFlags []string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	for _, s := range setFlags {
		if strings.HasPrefix(s, "hostname=") {
			return strings.TrimPrefix(s, "hostname="), nil
		}
	}
	ip, err := outboundIP()
	if err != nil {
		// Non-fatal: warn and use loopback so the deploy can still proceed
		fmt.Printf("    Warning: could not detect node IP (%v) — using 127.0.0.1\n", err)
		fmt.Printf("    Set --hostname to override.\n")
		return "rancher.127.0.0.1.sslip.io", nil
	}
	return fmt.Sprintf("rancher.%s.sslip.io", ip), nil
}

// injectHostname adds hostname=<h> to sets if not already present.
func injectHostname(sets []string, hostname string) []string {
	for _, s := range sets {
		if strings.HasPrefix(s, "hostname=") {
			return sets
		}
	}
	return append(sets, fmt.Sprintf("hostname=%s", hostname))
}

// outboundIP returns the preferred outbound IP of the local machine.
// In containers, always returns 127.0.0.1 since the container's bridge IP
// (172.17.0.x) is not accessible from the host.
func outboundIP() (string, error) {
	// Detect if running in a container
	if inContainer() {
		return "127.0.0.1", nil
	}

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", err
	}
	defer conn.Close()
	addr := conn.LocalAddr().(*net.UDPAddr)
	return addr.IP.String(), nil
}

// inContainer detects if we're running inside a container.
// Checks for /.dockerenv file or CONTAINER environment variable.
func inContainer() bool {
	// Docker creates /.dockerenv in containers
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	// Also check for explicit CONTAINER env var
	if os.Getenv("CONTAINER") != "" {
		return true
	}
	return false
}

// ── cert-manager ─────────────────────────────────────────────────────────────

const (
	certManagerGHLatest = "https://api.github.com/repos/cert-manager/cert-manager/releases/latest"
	certManagerFallback = "v1.14.4"
	certManagerTimeout  = 15 * time.Second
)

// githubGet performs an authenticated GET if GH_TOKEN or GITHUB_TOKEN is set,
// raising the GitHub API rate limit from 60 to 5,000 requests/hour.
func githubGet(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return client.Do(req) //nolint:gosec // URL is constructed from known GitHub API constants
}

// githubToken returns the first GitHub token found in the environment.
// GH_TOKEN (GitHub CLI) takes precedence over GITHUB_TOKEN (Actions/ecosystem).
func githubToken() string {
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GITHUB_TOKEN")
}

// ResolveCertManagerVersion fetches the latest stable cert-manager release tag
// from GitHub. If the API is unreachable it falls back to a known-good version
// and prints a warning rather than failing — a slightly stale cert-manager is
// better than blocking the whole deployment.
func ResolveCertManagerVersion() (string, error) {
	client := &http.Client{Timeout: certManagerTimeout}
	resp, err := githubGet(client, certManagerGHLatest)
	if err != nil {
		fmt.Printf("    Warning: could not fetch cert-manager version (%v) — using fallback %s\n", err, certManagerFallback)
		return certManagerFallback, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Printf("    Warning: GitHub returned HTTP %d for cert-manager — using fallback %s\n", resp.StatusCode, certManagerFallback)
		return certManagerFallback, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("    Warning: could not read cert-manager release response — using fallback %s\n", certManagerFallback)
		return certManagerFallback, nil
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &release); err != nil || release.TagName == "" {
		fmt.Printf("    Warning: could not parse cert-manager release response — using fallback %s\n", certManagerFallback)
		return certManagerFallback, nil
	}

	return release.TagName, nil
}

// InstallCertManager applies cert-manager at the given version and waits for
// its deployments to roll out.
func InstallCertManager(version string) error {
	// Check if already installed — fail loud
	out, _ := runner.Output("kubectl", "get", "namespace", "cert-manager",
		"--ignore-not-found", "-o", "name")
	if strings.Contains(out, "cert-manager") {
		return fmt.Errorf(
			"cert-manager namespace already exists\n" +
				"  Re-runs are not supported. Remove cert-manager before retrying",
		)
	}

	url := fmt.Sprintf(
		"https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml",
		version,
	)
	if err := runner.Run("kubectl", "apply", "-f", url); err != nil {
		return fmt.Errorf("failed to apply cert-manager: %w", err)
	}

	// Wait for all three cert-manager deployments. The webhook must be fully
	// ready before Rancher's Helm install can succeed — it validates CRDs and
	// will 503 if the endpoint isn't up yet.
	for _, deploy := range []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"} {
		if err := runner.Run("kubectl", "rollout", "status",
			"deployment/"+deploy,
			"-n", "cert-manager",
			"--timeout=120s",
		); err != nil {
			return fmt.Errorf("cert-manager deployment %q did not become ready: %w", deploy, err)
		}
	}
	return nil
}

// ── Helm repo management ─────────────────────────────────────────────────────

// EnsureHelmRepo makes the named Helm repo available with the correct URL:
//   - Not present → add it and fetch the index.
//   - Present with matching URL → fetch the index (no-op on the registration).
//   - Present with a different URL → prompt the user (or auto-confirm when yes=true),
//     then re-register with --force-update and fetch the index.
func EnsureHelmRepo(repoName, repoURL string, yes bool) error {
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
			return runner.Run("helm", "repo", "update", repoName)
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
		if err := runner.Run("helm", "repo", "add", "--force-update", repoName, repoURL); err != nil {
			return fmt.Errorf("helm repo update failed: %w", err)
		}
		return runner.Run("helm", "repo", "update", repoName)
	}

	// Repo not registered yet.
	if err := runner.Run("helm", "repo", "add", repoName, repoURL); err != nil {
		return fmt.Errorf("helm repo add failed: %w", err)
	}
	return runner.Run("helm", "repo", "update", repoName)
}

// ── Rancher install ──────────────────────────────────────────────────────────

// Install installs Rancher via Helm. The repo must already be registered —
// call EnsureHelmRepo before calling this.
func Install(namespace string, chart Chart, values HelmValues) error {
	if err := runner.MustExist("helm"); err != nil {
		return err
	}

	// Check Rancher is not already installed
	if err := ensureRancherAbsent(namespace); err != nil {
		return err
	}

	// Create namespace
	if err := runner.Run("kubectl", "create", "namespace", namespace); err != nil {
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

	if err := runner.Run("helm", args...); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("  Helm release created. Monitoring deployment progress...")
	fmt.Println()

	return nil
}

// WaitReady waits for the Rancher deployment to report available replicas,
// showing progress updates along the way.
func WaitReady(namespace string) error {
	fmt.Println("  Waiting for Rancher pods to start...")

	// Show pod status updates every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	done := make(chan error, 1)

	// Run rollout status in background
	go func() {
		done <- runner.Run("kubectl", "rollout", "status",
			"deployment/rancher",
			"-n", namespace,
			"--timeout=600s",
		)
	}()

	lastStatus := ""
	for {
		select {
		case err := <-done:
			// Rollout completed (or timed out)
			if err == nil {
				fmt.Println()
				fmt.Println("  ✓ Rancher deployment ready")
			}
			return err

		case <-ticker.C:
			// Show current pod status
			status := getPodStatus(namespace)
			if status != "" && status != lastStatus {
				fmt.Printf("  %s\n", status)
				lastStatus = status
			}
		}
	}
}

// getPodStatus returns a human-readable summary of Rancher pod status.
func getPodStatus(namespace string) string {
	// Get deployment status
	out, err := runner.Output("kubectl", "get", "deployment", "rancher",
		"-n", namespace,
		"-o", "jsonpath={.status.replicas}/{.status.readyReplicas}/{.status.availableReplicas}",
	)
	if err != nil || out == "" {
		return "Waiting for deployment to be created..."
	}

	parts := strings.Split(out, "/")
	if len(parts) < 3 {
		return "Deployment initializing..."
	}

	total := parts[0]
	ready := parts[1]
	available := parts[2]

	if ready == "" {
		ready = "0"
	}
	if available == "" {
		available = "0"
	}

	// Also get pod phase info for more detail
	podOut, _ := runner.Output("kubectl", "get", "pods",
		"-n", namespace,
		"-l", "app=rancher",
		"--no-headers",
	)

	var detail string
	if podOut != "" {
		lines := strings.Split(strings.TrimSpace(podOut), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				status := fields[2] // STATUS column
				if status == "ContainerCreating" || status == "PodInitializing" {
					detail = " (pulling images)"
					break
				} else if status == "Init:0/1" || strings.HasPrefix(status, "Init:") {
					detail = " (initializing)"
					break
				}
			}
		}
	}

	return fmt.Sprintf("Pods: %s/%s ready%s", ready, total, detail)
}

// InstalledVersion returns the currently installed Rancher app version from
// Helm (e.g. "2.8.5"). Returns an error if Rancher is not installed.
func InstalledVersion(namespace string) (string, error) {
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
		if r.Name == chartName {
			return strings.TrimPrefix(r.AppVersion, "v"), nil
		}
	}
	return "", fmt.Errorf("rancher is not installed in namespace %q — nothing to upgrade", namespace)
}

// ClusterK8sVersion returns the server-side k8s version (e.g. "v1.28.10+k3s1").
func ClusterK8sVersion() (string, error) {
	out, err := runner.Output("kubectl", "version", "-o", "json")
	if err != nil {
		return "", fmt.Errorf("could not query cluster version: %w", err)
	}

	var v struct {
		ServerVersion struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return "", fmt.Errorf("could not parse kubectl version output: %w", err)
	}
	if v.ServerVersion.GitVersion == "" {
		return "", fmt.Errorf("kubectl version returned an empty server version")
	}
	return v.ServerVersion.GitVersion, nil
}

// Upgrade runs helm upgrade for an existing Rancher installation.
// Previous values are fetched from the existing release and applied via --values.
// This avoids issues with --reuse-values where new chart defaults might be missing.
// Explicit --set flags and a values file (if provided) are applied on top.
// The repo must already be registered — call EnsureHelmRepo before this.
func Upgrade(namespace string, chart Chart, values HelmValues) error {
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

	if err := runner.Run("helm", args...); err != nil {
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
		if strings.TrimSpace(line) == chartName {
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

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
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

// ── Chart reference ──────────────────────────────────────────────────────────

// Channel selects the release channel (and therefore the Helm repo) to use.
const (
	ChannelStable = "stable" // GA releases only
	ChannelLatest = "latest" // RC + latest GA  (alias: "rc")
	ChannelAlpha  = "alpha"  // Alpha builds
	ChannelHead   = "head"   // Continuously-published head builds for a minor branch

	chartName = "rancher"
)

// chart repos indexed by [prime][channel].
//
// Community head builds are intentionally absent here: they're served from a
// dedicated per-minor path (see headRepoURL) rather than a single fixed URL,
// so they can't be represented as a static map entry.
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
		// Prime head builds are already mixed into the "latest" repo today
		// (GA + RC + head, filtered by resolveHeadEntry). Kept as its own
		// entry — not aliased to ChannelLatest — so it can be pointed at a
		// dedicated repo independently if Prime ever splits one out.
		ChannelHead: "https://charts.optimus.rancher.io/server-charts/latest",
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
		ChannelHead:   "rancher-prime-head",
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
	case ChannelHead:
		return ChannelHead, nil
	default:
		return "", fmt.Errorf("unknown channel %q: must be stable, latest, rc, alpha, or head", channel)
	}
}

// headRepoURL returns the Helm repo URL that serves head builds for the
// given edition and minor version (e.g. "2.15").
//
// Community head builds live in a dedicated per-minor repo. Prime head
// builds are not on a separate path at all today — they're already mixed
// into the Prime repo configured under ChannelHead (currently the same repo
// as "latest": GA + RC + head), filtered down by resolveHeadEntry.
func headRepoURL(prime bool, minor string) string {
	if prime {
		return repoURLs[true][ChannelHead]
	}
	return fmt.Sprintf("https://charts.optimus.rancher.io/server-charts/release-%s", minor)
}

// headRepoName returns the Helm repo alias to register for head builds.
func headRepoName(prime bool, minor string) string {
	if prime {
		return repoNames[true][ChannelHead]
	}
	return fmt.Sprintf("rancher-head-%s", minor)
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

// helmIndexEntry is the subset of a Helm repository index entry needed to
// resolve chart versions, including by recency for head builds (whose
// version strings carry a git hash rather than a sortable sequence).
type helmIndexEntry struct {
	Version string `yaml:"version"`
	Created string `yaml:"created"`
}

// helmIndex is the minimal subset of a Helm repository index file needed to
// find the latest chart version.
type helmIndex struct {
	Entries map[string][]helmIndexEntry `yaml:"entries"`
}

// fetchIndex downloads and parses the Helm repository index at repoURL,
// returning the "rancher" chart's entries.
func fetchIndex(repoURL string) ([]helmIndexEntry, error) {
	indexURL := repoURL + "/index.yaml"
	client := &http.Client{Timeout: certManagerTimeout}
	resp, err := client.Get(indexURL) //nolint:gosec // URL built from internal map of known Helm repo constants
	if err != nil {
		return nil, fmt.Errorf("could not fetch Helm index from %s: %w", indexURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching Helm index from %s", resp.StatusCode, indexURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("could not read Helm index: %w", err)
	}

	var index helmIndex
	if err := yaml.Unmarshal(body, &index); err != nil {
		return nil, fmt.Errorf("could not parse Helm index: %w", err)
	}

	entries, ok := index.Entries[chartName]
	if !ok || len(entries) == 0 {
		return nil, fmt.Errorf("no %q chart entries in Helm index from %s", chartName, indexURL)
	}
	return entries, nil
}

// FetchLatestVersion queries the Helm repository for the given prime/channel
// combination and returns the latest available Rancher version (without a leading "v").
func FetchLatestVersion(prime bool, channel string) (string, error) {
	repoURL, ok := repoURLs[prime][channel]
	if !ok {
		return "", fmt.Errorf("no repository configured for prime=%v channel=%q", prime, channel)
	}

	entries, err := fetchIndex(repoURL)
	if err != nil {
		return "", err
	}

	sort.Slice(entries, func(i, j int) bool {
		return compareRancherVersions(entries[i].Version, entries[j].Version) > 0
	})

	return strings.TrimPrefix(entries[0].Version, "v"), nil
}

// minorOnlyRe matches a bare "MAJOR.MINOR" version input (no patch, no
// pre-release suffix) — the signal that the caller wants auto-resolution
// within that minor rather than an exact pin.
var minorOnlyRe = regexp.MustCompile(`^\d+\.\d+$`)

// minorPrefixRe extracts the "MAJOR.MINOR" prefix from a version string,
// tolerating trailing content that doesn't parse as a plain patch number —
// e.g. head build versions like "2.15-9f0d030...-head" or
// "2.15.2-fbf2130-head".
var minorPrefixRe = regexp.MustCompile(`^(\d+\.\d+)`)

// versionMinor extracts the "MAJOR.MINOR" prefix from a version string.
// Returns "" if the string doesn't start with a recognisable major.minor.
func versionMinor(v string) string {
	v = strings.TrimPrefix(v, "v")
	m := minorPrefixRe.FindStringSubmatch(v)
	if m == nil {
		return ""
	}
	return m[1]
}

// resolveMinorEntry picks the newest entry in entries whose version falls in
// the given minor, using ordinary semver-ish comparison.
func resolveMinorEntry(entries []helmIndexEntry, minor string) (helmIndexEntry, error) {
	var candidates []helmIndexEntry
	for _, e := range entries {
		if versionMinor(e.Version) == minor {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return helmIndexEntry{}, fmt.Errorf("no chart entries found for Rancher %s in this repository", minor)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareRancherVersions(candidates[i].Version, candidates[j].Version) > 0
	})
	return candidates[0], nil
}

// resolveHeadEntry picks the newest head build for the given minor. Head
// build version strings carry a git hash rather than a sortable sequence
// (e.g. "2.16.0-b03c4de-head" vs "2.16.0-de47bc4-head"), so recency is
// determined by each entry's "created" timestamp instead of the version
// string itself.
func resolveHeadEntry(entries []helmIndexEntry, minor string) (helmIndexEntry, error) {
	var candidates []helmIndexEntry
	for _, e := range entries {
		if versionMinor(e.Version) == minor && strings.HasSuffix(e.Version, "-head") {
			candidates = append(candidates, e)
		}
	}
	if len(candidates) == 0 {
		return helmIndexEntry{}, fmt.Errorf("no head build found for Rancher %s minor in this repository", minor)
	}
	sort.Slice(candidates, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339Nano, candidates[i].Created)
		tj, _ := time.Parse(time.RFC3339Nano, candidates[j].Created)
		return ti.After(tj)
	})
	return candidates[0], nil
}

// ResolveChart determines the full Chart (repo, name, version, prerelease)
// for a given edition/channel/version input.
//
// versionInput may be:
//   - empty — resolve the absolute newest version in the channel (not valid
//     for the head channel, which is always minor-scoped).
//   - a bare minor, e.g. "2.15" — resolve the newest patch in that minor
//     (stable/latest/alpha), or the newest head build for that minor (head).
//   - an exact version, e.g. "2.8.5" or "2.9.0-rc1" — used as-is with no
//     network call (not valid for head, whose build hashes aren't
//     user-guessable).
func ResolveChart(prime bool, channel, versionInput string) (Chart, error) {
	versionInput = strings.TrimPrefix(versionInput, "v")

	if channel == ChannelHead {
		minor := versionMinor(versionInput)
		if minor == "" {
			return Chart{}, fmt.Errorf(
				"head channel requires a Rancher minor version, e.g. --rancher-version 2.15 (got %q)",
				versionInput,
			)
		}
		repoURL := headRepoURL(prime, minor)
		entries, err := fetchIndex(repoURL)
		if err != nil {
			return Chart{}, err
		}
		entry, err := resolveHeadEntry(entries, minor)
		if err != nil {
			return Chart{}, err
		}
		version := strings.TrimPrefix(entry.Version, "v")
		return Chart{
			RepoName:     headRepoName(prime, minor),
			RepoURL:      repoURL,
			ChartName:    chartName,
			Version:      version,
			IsPrerelease: true,
		}, nil
	}

	repoURL, ok := repoURLs[prime][channel]
	if !ok {
		return Chart{}, fmt.Errorf("no repository configured for prime=%v channel=%q", prime, channel)
	}
	repoName := repoNames[prime][channel]

	var version string
	switch {
	case versionInput == "":
		latest, err := FetchLatestVersion(prime, channel)
		if err != nil {
			return Chart{}, err
		}
		version = latest
	case minorOnlyRe.MatchString(versionInput):
		entries, err := fetchIndex(repoURL)
		if err != nil {
			return Chart{}, err
		}
		entry, err := resolveMinorEntry(entries, versionInput)
		if err != nil {
			return Chart{}, err
		}
		version = strings.TrimPrefix(entry.Version, "v")
	default:
		version = versionInput
	}

	return Chart{
		RepoName:     repoName,
		RepoURL:      repoURL,
		ChartName:    chartName,
		Version:      version,
		IsPrerelease: k8sresolver.IsPrerelease(version),
	}, nil
}

// compareRancherVersions compares two Rancher version strings numerically.
// Returns >0 if a is newer, <0 if b is newer, 0 if equal.
// Stable releases sort above pre-release versions at the same base (e.g. 2.9.0 > 2.9.0-rc1).
func compareRancherVersions(a, b string) int {
	parseVer := func(v string) (maj, min, pat int, pre string) {
		v = strings.TrimPrefix(v, "v")
		if idx := strings.IndexByte(v, '-'); idx >= 0 {
			pre = v[idx:]
			v = v[:idx]
		}
		parts := strings.SplitN(v, ".", 3)
		if len(parts) > 0 {
			_, _ = fmt.Sscanf(parts[0], "%d", &maj)
		}
		if len(parts) > 1 {
			_, _ = fmt.Sscanf(parts[1], "%d", &min)
		}
		if len(parts) > 2 {
			_, _ = fmt.Sscanf(parts[2], "%d", &pat)
		}
		return
	}

	aMaj, aMin, aPat, aPre := parseVer(a)
	bMaj, bMin, bPat, bPre := parseVer(b)

	if aMaj != bMaj {
		return aMaj - bMaj
	}
	if aMin != bMin {
		return aMin - bMin
	}
	if aPat != bPat {
		return aPat - bPat
	}
	if aPre == "" && bPre != "" {
		return 1 // a is stable, b is pre-release — a wins
	}
	if aPre != "" && bPre == "" {
		return -1 // a is pre-release, b is stable — b wins
	}
	return strings.Compare(aPre, bPre)
}

// ── Helm values ──────────────────────────────────────────────────────────────

// HelmValues holds everything needed to construct the Helm install command.
type HelmValues struct {
	ValuesFile string   // --values <file>
	Hostname   string   // resolved hostname
	SetFlags   []string // --set key=value entries
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

	resolvedHostname := resolveHostnameWithFallback(hostname, setFlags)

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

// resolveHostname returns a hostname derived from the explicit flag, a hostname=
// entry in setFlags, or the machine's outbound IP formatted as an sslip.io address.
// Returns an error only when all three sources fail to produce a value.
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
		return "", err
	}
	return fmt.Sprintf("rancher.%s.sslip.io", ip), nil
}

// resolveHostnameWithFallback wraps resolveHostname and falls back to
// "rancher.127.0.0.1.sslip.io" when IP detection fails, printing a warning
// instead of propagating the error.
func resolveHostnameWithFallback(explicit string, setFlags []string) string {
	hostname, err := resolveHostname(explicit, setFlags)
	if err != nil {
		fmt.Printf("    Warning: could not detect node IP (%v) — using 127.0.0.1\n", err)
		fmt.Printf("    Set --hostname to override.\n")
		return "rancher.127.0.0.1.sslip.io"
	}
	return hostname
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
	if err := runner.Kubectl("apply", "-f", url); err != nil {
		return fmt.Errorf("failed to apply cert-manager: %w", err)
	}

	// Wait for all three cert-manager deployments. The webhook must be fully
	// ready before Rancher's Helm install can succeed — it validates CRDs and
	// will 503 if the endpoint isn't up yet.
	for _, deploy := range []string{"cert-manager", "cert-manager-cainjector", "cert-manager-webhook"} {
		if err := runner.Kubectl("rollout", "status",
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
	if err := runner.Kubectl("create", "namespace", namespace); err != nil {
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

	if err := runner.Helm(args...); err != nil {
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
		done <- runner.Kubectl("rollout", "status",
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

	if ready == "" {
		ready = "0"
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

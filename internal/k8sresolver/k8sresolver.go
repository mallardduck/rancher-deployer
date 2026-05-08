// Package k8sresolver resolves the Kubernetes and cluster (k3s/k3d) versions to
// use, given a Rancher support matrix and optional user constraints.
package k8sresolver

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mallardduck/rancher-deployer/internal/kdm"
)

const (
	// k3s has many releases; paginate to ensure we find older minors.
	k3sReleasesURLFmt   = "https://api.github.com/repos/k3s-io/k3s/releases?per_page=100&page=%d"
	k3sMaxPages         = 10 // cap at 1000 releases — covers all published versions
	githubClientTimeout = 30 * time.Second

	cacheFileName = "k3s-releases.json"
	cacheTTL      = 24 * time.Hour
)

// releasesCache is the on-disk cache format.
type releasesCache struct {
	FetchedAt time.Time   `json:"fetched_at"`
	Releases  []ghRelease `json:"releases"`
}

func cacheFilePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rancher-deployer", cacheFileName), nil
}

func loadCache() ([]ghRelease, bool) {
	path, err := cacheFilePath()
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from os.UserCacheDir(), not user input
	if err != nil {
		return nil, false
	}
	var c releasesCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, false
	}
	if time.Since(c.FetchedAt) > cacheTTL {
		return nil, false
	}
	return c.Releases, true
}

func saveCache(releases []ghRelease) {
	path, err := cacheFilePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return
	}
	data, err := json.Marshal(releasesCache{FetchedAt: time.Now(), Releases: releases})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}

// ResolveK8s determines the full k8s version (e.g. "v1.28.10") to target.
//
//   - If userMinor is empty, the newest version from the support matrix is used.
//   - If userMinor is set (e.g. "1.28"), it is validated against the matrix and
//     the latest patch in that minor is returned.
func ResolveK8s(userMinor string, matrix *kdm.SupportMatrix) (string, error) {
	if userMinor == "" {
		latest := matrix.LatestSupported()
		if latest == "" {
			return "", fmt.Errorf("support matrix returned no k8s versions")
		}
		return latest, nil
	}

	// User specified a minor — validate + get latest patch
	userMinor = strings.TrimPrefix(userMinor, "v")
	full, err := matrix.LatestPatchFor(userMinor)
	if err != nil {
		return "", err
	}
	return full, nil
}

// ResolveClusterVersion finds the latest stable k3s release tag for the given
// full k8s version (e.g. "v1.28.10"). Both k3s and k3d use k3s image tags
// internally, so we always resolve a k3s tag regardless of mode.
//
// Resolution strategy:
//  1. Exact patch match: find the latest k3sN build for that exact patch
//     (e.g. v1.28.10+k3s1, v1.28.10+k3s2 → v1.28.10+k3s2)
//  2. Minor fallback: if the exact patch has no k3s release yet, find the
//     newest available patch in that minor series
func ResolveClusterVersion(_ string, k8sVersion string) (string, error) {
	minor := extractMinor(k8sVersion) // "1.28"
	patch := extractFull(k8sVersion)  // "v1.28.10"

	releases, err := fetchK3sReleases(minor)
	if err != nil {
		return "", fmt.Errorf("could not fetch k3s releases: %w", err)
	}

	if patch != "" {
		if tag := latestTagForPatch(releases, patch); tag != "" {
			return tag, nil
		}
		fmt.Printf("    Note: no k3s release for %s exactly — using latest in %s\n", patch, minor)
	}

	tag := latestTagForMinor(releases, minor)
	if tag == "" {
		return "", fmt.Errorf(
			"no stable k3s release found for Kubernetes %s\n"+
				"  See: https://github.com/k3s-io/k3s/releases",
			k8sVersion,
		)
	}
	return tag, nil
}

// ── GitHub release fetching ──────────────────────────────────────────────────

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
}

// githubToken returns a GitHub personal access token from the environment, if
// one is set. GH_TOKEN (used by the GitHub CLI) takes precedence over the
// more generic GITHUB_TOKEN (used by GitHub Actions and many other tools).
func githubToken() string {
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GITHUB_TOKEN")
}

// githubGet performs an authenticated GET if a token is available, falling
// back to unauthenticated. Using a token raises the rate limit from 60 to
// 5,000 requests per hour.
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

// fetchK3sReleases returns k3s GitHub releases, using a 24-hour disk cache to
// avoid GitHub API rate limits. When a cache miss forces a live fetch, pagination
// stops early once releases older than targetMinor are reached — typically 1–3
// pages instead of the theoretical maximum of 10.
func fetchK3sReleases(targetMinor string) ([]ghRelease, error) {
	if releases, ok := loadCache(); ok {
		return releases, nil
	}

	client := &http.Client{Timeout: githubClientTimeout}
	var all []ghRelease

	for page := 1; page <= k3sMaxPages; page++ {
		url := fmt.Sprintf(k3sReleasesURLFmt, page)
		resp, err := githubGet(client, url)
		if err != nil {
			return nil, fmt.Errorf("GitHub API request failed (page %d): %w", page, err)
		}

		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("GitHub API returned HTTP %d (page %d)", resp.StatusCode, page)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading GitHub response (page %d): %w", page, err)
		}

		var pageReleases []ghRelease
		if err := json.Unmarshal(body, &pageReleases); err != nil {
			return nil, fmt.Errorf("failed to parse GitHub releases (page %d): %w", page, err)
		}

		all = append(all, pageReleases...)

		// Stop early: GitHub returns releases newest-first. Once the last release
		// on this page is from a minor older than our target, we have everything
		// we need and can avoid further API calls.
		if targetMinor != "" && len(pageReleases) > 0 {
			lastMinor := extractMinor(pageReleases[len(pageReleases)-1].TagName)
			if compareMinors(lastMinor, targetMinor) < 0 {
				break
			}
		}

		// GitHub returns fewer than per_page items on the last page.
		if len(pageReleases) < 100 {
			break
		}
	}

	if len(all) == 0 {
		return nil, fmt.Errorf("no k3s releases returned from GitHub API")
	}

	saveCache(all)
	return all, nil
}

// compareMinors compares two "MAJOR.MINOR" strings. Returns >0 if a is newer.
func compareMinors(a, b string) int {
	return compareSemverStr(a+".0", b+".0")
}

// latestTagForPatch returns the newest k3s tag for an exact patch version.
// e.g. patch="v1.28.10" → "v1.28.10+k3s2" (if k3s2 is newer than k3s1)
func latestTagForPatch(releases []ghRelease, patch string) string {
	// Normalise: ensure "v" prefix
	patch = "v" + strings.TrimPrefix(patch, "v")
	prefix := patch + "+"

	var candidates []string
	for _, r := range releases {
		if r.Prerelease || r.Draft {
			continue
		}
		if strings.HasPrefix(r.TagName, prefix) {
			candidates = append(candidates, r.TagName)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareK3sTag(candidates[i], candidates[j]) > 0
	})
	return candidates[0]
}

// latestTagForMinor returns the newest stable k3s tag for a given minor version.
// e.g. minor="1.28" → "v1.28.13+k3s1"
func latestTagForMinor(releases []ghRelease, minor string) string {
	minor = strings.TrimPrefix(minor, "v")
	prefix := "v" + minor + "."

	var candidates []string
	for _, r := range releases {
		if r.Prerelease || r.Draft {
			continue
		}
		if strings.HasPrefix(r.TagName, prefix) {
			candidates = append(candidates, r.TagName)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		return compareK3sTag(candidates[i], candidates[j]) > 0
	})
	return candidates[0]
}

// compareK3sTag compares two k3s tags like "v1.28.10+k3s1" vs "v1.28.10+k3s2".
// Returns >0 if a is newer.
func compareK3sTag(a, b string) int {
	// Split into semver part and k3s build number
	semA, buildA := splitK3sTag(a)
	semB, buildB := splitK3sTag(b)
	if cmp := compareSemverStr(semA, semB); cmp != 0 {
		return cmp
	}
	// Same semver — compare build number (k3s1 < k3s2)
	var na, nb int
	_, _ = fmt.Sscanf(buildA, "k3s%d", &na)
	_, _ = fmt.Sscanf(buildB, "k3s%d", &nb)
	return na - nb
}

func splitK3sTag(tag string) (semver, build string) {
	parts := strings.SplitN(tag, "+", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return tag, "k3s0"
}

func compareSemverStr(a, b string) int {
	pa := parseVerParts(a)
	pb := parseVerParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func parseVerParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		_, _ = fmt.Sscanf(parts[i], "%d", &out[i])
	}
	return out
}

// IsPrerelease returns true if the version string contains a hyphen, which
// indicates a pre-release version (e.g. "2.8.5-rc1").
func IsPrerelease(v string) bool {
	return strings.Contains(v, "-")
}

func extractMinor(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

func extractFull(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return ""
	}
	return "v" + strings.Join(parts[:3], ".")
}

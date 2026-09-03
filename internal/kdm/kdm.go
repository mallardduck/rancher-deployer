// Package kdm fetches and parses Rancher's Kontainer Driver Metadata (KDM),
// which is the authoritative source for the k8s versions supported by each
// Rancher release.
package kdm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	// Release KDM source: Rancher's releases CDN, tied to an official release branch.
	kdmURLTemplate = "https://releases.rancher.com/kontainer-driver-metadata/release-v%s/data.json"
	// Dev KDM source: GitHub raw, tied to the in-development branch for a minor.
	kdmFallbackTemplate = "https://raw.githubusercontent.com/rancher/kontainer-driver-metadata/refs/heads/dev-v%s/data/data.json"

	httpTimeout = 30 * time.Second
)

// KDMFlavor identifies which KDM branch a support matrix was actually built
// from: an official release branch, or the in-development branch for a
// minor that doesn't have one yet.
type KDMFlavor string

const (
	KDMFlavorRelease KDMFlavor = "release"
	KDMFlavorDev     KDMFlavor = "dev"
)

// releaseFirst and devFirst are the two flavor-preference orders callers
// choose between: releaseFirst for ordinary (non-head) installs, which are
// tied to an official release; devFirst for head-channel installs, which
// are inherently dev/bleeding-edge rather than tied to a release branch.
var (
	releaseFirst = []KDMFlavor{KDMFlavorRelease, KDMFlavorDev}
	devFirst     = []KDMFlavor{KDMFlavorDev, KDMFlavorRelease}
)

// kdmURL returns the KDM data.json URL for the given flavor and minor.
func kdmURL(flavor KDMFlavor, minor string) string {
	if flavor == KDMFlavorDev {
		return fmt.Sprintf(kdmFallbackTemplate, minor)
	}
	return fmt.Sprintf(kdmURLTemplate, minor)
}

// SupportMatrix holds the k8s versions supported by a specific Rancher release.
type SupportMatrix struct {
	RancherVersion string
	// k8sVersions is the full set of supported versions, e.g. ["v1.28.10", "v1.27.14", ...]
	k8sVersions []string
}

// SupportedMinors returns the unique minor versions in the matrix, sorted
// descending (newest first), e.g. ["1.28", "1.27", "1.26"].
func (m *SupportMatrix) SupportedMinors() []string {
	seen := map[string]bool{}
	var minors []string
	for _, v := range m.k8sVersions {
		minor := toMinor(v)
		if !seen[minor] {
			seen[minor] = true
			minors = append(minors, minor)
		}
	}
	sort.Slice(minors, func(i, j int) bool {
		return compareMinor(minors[i], minors[j]) > 0
	})
	return minors
}

// LatestPatchFor returns the newest patch release for the given minor version
// (e.g. "1.28" → "v1.28.10"). Returns an error if the minor isn't supported.
// The input may be a full version string (e.g. "v1.34.4+k3s1") — only the
// major.minor portion is used for matching.
func (m *SupportMatrix) LatestPatchFor(minor string) (string, error) {
	minor = strings.TrimPrefix(minor, "v")
	// Strip build metadata and patch so "1.34.4+k3s1" → "1.34"
	if idx := strings.IndexByte(minor, '+'); idx >= 0 {
		minor = minor[:idx]
	}
	parts := strings.Split(minor, ".")
	if len(parts) >= 2 {
		minor = parts[0] + "." + parts[1]
	}

	var matches []string
	for _, v := range m.k8sVersions {
		if toMinor(v) == minor {
			matches = append(matches, v)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf(
			"k8s version %s is not supported by Rancher v%s\n  Supported: %s",
			minor, m.RancherVersion, strings.Join(m.SupportedMinors(), ", "),
		)
	}
	sort.Slice(matches, func(i, j int) bool {
		return compareFull(matches[i], matches[j]) > 0
	})
	return matches[0], nil
}

// LatestSupported returns the newest fully-supported k8s version across all
// supported minors.
func (m *SupportMatrix) LatestSupported() string {
	if len(m.k8sVersions) == 0 {
		return ""
	}
	sorted := make([]string, len(m.k8sVersions))
	copy(sorted, m.k8sVersions)
	sort.Slice(sorted, func(i, j int) bool {
		return compareFull(sorted[i], sorted[j]) > 0
	})
	return sorted[0]
}

// kdmData mirrors the parts of the KDM JSON we care about.
// The real schema has many more fields; we only decode what we need.
//
// KDM is structured around the downstream distros Rancher can provision
// (k3s and rke2). Each distro section lists all release versions that
// Rancher knows about. We use this as a proxy for "what k8s version should
// I run Rancher on?" — any version listed here is a safe choice.
type kdmData struct {
	K3sInfo  distroInfo `json:"k3s"`
	Rke2Info distroInfo `json:"rke2"`
}

type distroInfo struct {
	AppDefaults []appDefaultIndex `json:"appDefaults"`
	// appDefaults has info about Rancher stuff - inside there is where the rancher app version stuff can be found - and it maps to "defaultVersion" to use for k8s.
	Releases []distroRelease `json:"releases"`
}

type appDefaultIndex struct {
	AppName  string        `json:"appName"`
	Defaults []appDefaults `json:"defaults"`
}

type appDefaults struct {
	AppVersion     string `json:"appVersion"`
	DefaultVersion string `json:"defaultVersion"`
}

type distroRelease struct {
	// also has max and min channel server version...not super helpful.
	Version string `json:"version"`
}

// FetchSupportMatrix downloads and parses the KDM for the given Rancher
// version, preferring the official release branch and falling back to the
// dev branch if that minor has no release branch yet.
func FetchSupportMatrix(rancherVersion string) (*SupportMatrix, error) {
	matrix, _, err := fetchSupportMatrixFlavored(rancherVersion, releaseFirst)
	return matrix, err
}

// SupportMatrixResult is what FetchSupportMatrixWithFallback resolves for a
// head-channel install: the support matrix itself, which KDM branch it
// actually came from, and whether the previous minor's data had to be used.
type SupportMatrixResult struct {
	Matrix            *SupportMatrix
	Flavor            KDMFlavor
	UsedFallbackMinor bool
}

// FetchSupportMatrixWithFallback resolves KDM data for a head-channel
// install. Head builds are inherently dev/bleeding-edge rather than tied to
// an official release, so the dev branch is checked before the release
// branch — for the target minor, and (if neither has data for it yet) for
// the previous minor too, tried exactly once. Result.Flavor reports which
// branch the returned data actually came from, and
// Result.UsedFallbackMinor reports whether the previous minor's data had to
// be used, so callers can warn that k8s compatibility isn't guaranteed for
// it.
func FetchSupportMatrixWithFallback(rancherVersion string) (*SupportMatrixResult, error) {
	matrix, flavor, err := fetchSupportMatrixFlavored(rancherVersion, devFirst)
	if err == nil {
		return &SupportMatrixResult{Matrix: matrix, Flavor: flavor}, nil
	}
	primaryErr := err

	fallbackVersion, ok := previousMinorVersion(rancherVersion)
	if !ok {
		return nil, primaryErr
	}

	matrix, flavor, err = fetchSupportMatrixFlavored(fallbackVersion, devFirst)
	if err != nil {
		return nil, fmt.Errorf(
			"no KDM data for Rancher %s (release or dev), and fallback to %s also failed:\n  primary:  %w\n  fallback: %w",
			rancherVersion, fallbackVersion, primaryErr, err,
		)
	}
	return &SupportMatrixResult{Matrix: matrix, Flavor: flavor, UsedFallbackMinor: true}, nil
}

// fetchSupportMatrixFlavored downloads and parses the KDM for rancherVersion,
// trying each flavor in order and returning whichever one succeeds first.
func fetchSupportMatrixFlavored(rancherVersion string, order []KDMFlavor) (*SupportMatrix, KDMFlavor, error) {
	minor := majorMinor(rancherVersion)

	data, flavor, err := fetchKDM(minor, order)
	if err != nil {
		return nil, "", err
	}

	versions := extractVersions(data, rancherVersion)
	if len(versions) == 0 {
		return nil, "", fmt.Errorf(
			"KDM data for Rancher v%s contained no k8s versions — "+
				"the format may have changed; check https://www.suse.com/suse-rancher/support-matrix/",
			rancherVersion,
		)
	}

	return &SupportMatrix{
		RancherVersion: rancherVersion,
		k8sVersions:    versions,
	}, flavor, nil
}

// previousMinorVersion computes "MAJOR.(MINOR-1).0" from a version string,
// used as the KDM fallback target. Returns ok=false if the minor can't be
// parsed or is already 0 (nothing to fall back to).
func previousMinorVersion(v string) (string, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return "", false
	}
	var major, minor int
	if _, err := fmt.Sscanf(parts[0], "%d", &major); err != nil {
		return "", false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minor); err != nil {
		return "", false
	}
	if minor == 0 {
		return "", false
	}
	return fmt.Sprintf("%d.%d.0", major, minor-1), true
}

// fetchKDM tries each flavor in order for rancherMinor, returning the data
// from whichever succeeds first, along with which flavor that was.
func fetchKDM(rancherMinor string, order []KDMFlavor) (*kdmData, KDMFlavor, error) {
	client := &http.Client{Timeout: httpTimeout}

	var tried []string
	for _, flavor := range order {
		url := kdmURL(flavor, rancherMinor)
		if data, err := doFetch(client, url); err == nil {
			return data, flavor, nil
		}
		tried = append(tried, fmt.Sprintf("  %s: %s", flavor, url))
	}

	return nil, "", fmt.Errorf(
		"could not fetch KDM data for Rancher v%s from any source:\n%s\n"+
			"Check connectivity or specify --k8s-version manually",
		rancherMinor, strings.Join(tried, "\n"),
	)
}

func doFetch(client *http.Client, url string) (*kdmData, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var d kdmData
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("failed to parse KDM JSON: %w", err)
	}
	return &d, nil
}

// extractVersions pulls unique, normalised k8s version strings from the KDM
// payload for the given Rancher version. KDM files contain data for all
// Rancher versions, so we must filter using appDefaults:
//  1. For each distro section (k3s, rke2), find appDefaults entries whose
//     AppVersion range (e.g. ">= 2.8.0-0 < 2.9.0-0") includes rancherVersion.
//  2. Extract the supported k8s minor from DefaultVersion (e.g. "1.28.x" → "1.28").
//  3. Collect all actual patch releases from the releases array for those minors.
//
// Build metadata (+k3s1, +rke2r1) is stripped so versions deduplicate cleanly.
func extractVersions(d *kdmData, rancherVersion string) []string {
	seen := map[string]bool{}
	var out []string

	add := func(v string) {
		v = normaliseVersion(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}

	collectFromDistro := func(info distroInfo) {
		// Find the k8s minors supported for this Rancher version.
		supportedMinors := map[string]bool{}
		for _, idx := range info.AppDefaults {
			for _, def := range idx.Defaults {
				if matchesRange(def.AppVersion, rancherVersion) {
					minor := defaultVersionToMinor(def.DefaultVersion)
					if minor != "" {
						supportedMinors[minor] = true
					}
				}
			}
		}
		// Collect actual patch releases for those minors.
		for _, r := range info.Releases {
			if supportedMinors[toMinor(r.Version)] {
				add(r.Version)
			}
		}
	}

	collectFromDistro(d.K3sInfo)
	collectFromDistro(d.Rke2Info)
	return out
}

// matchesRange checks whether version satisfies a space-separated semver
// constraint string such as ">= 2.8.0-0 < 2.9.0-0". Pre-release suffixes
// (e.g. "-0") are stripped before numeric comparison.
//
// The data.json on GitHub is generated by a Go program that calls json.Marshal,
// which HTML-escapes < and > as \u003c and \u003e. Although json.Unmarshal
// normally decodes these back to < and >, when the raw bytes pass through
// certain paths they can survive as literal 6-character sequences. We
// normalise them here so operator parsing is always reliable.
func matchesRange(rangeStr, version string) bool {
	rangeStr = strings.ReplaceAll(rangeStr, `\u003e`, ">")
	rangeStr = strings.ReplaceAll(rangeStr, `\u003c`, "<")
	version = stripPreRelease(version)
	parts := strings.Fields(rangeStr)
	for i := 0; i+1 < len(parts); i += 2 {
		op := parts[i]
		cv := stripPreRelease(parts[i+1])
		cmp := compareSemver(version, cv)
		switch op {
		case ">=":
			if cmp < 0 {
				return false
			}
		case ">":
			if cmp <= 0 {
				return false
			}
		case "<=":
			if cmp > 0 {
				return false
			}
		case "<":
			if cmp >= 0 {
				return false
			}
		case "=", "==":
			if cmp != 0 {
				return false
			}
		}
	}
	return true
}

// stripPreRelease removes the pre-release portion (e.g. "-0", "-alpha.1") and
// the leading "v" from a semver string, leaving a plain "MAJOR.MINOR.PATCH".
func stripPreRelease(v string) string {
	v = strings.TrimPrefix(v, "v")
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	return v
}

// defaultVersionToMinor converts a KDM DefaultVersion pattern like "1.28.x"
// into a plain minor string "1.28" suitable for comparison with toMinor().
func defaultVersionToMinor(defaultVersion string) string {
	v := strings.TrimPrefix(defaultVersion, "v")
	v = strings.TrimSuffix(v, ".x")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// ── Version helpers ──────────────────────────────────────────────────────────

// normaliseVersion ensures a version string is in "vMAJOR.MINOR.PATCH" form.
// Build metadata (e.g. "+k3s1") is stripped.
// Returns "" if the string doesn't look like a semver.
func normaliseVersion(v string) string {
	v = strings.TrimPrefix(v, "v")
	// Strip build metadata (e.g. "+k3s1")
	if idx := strings.IndexByte(v, '+'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return ""
	}
	return "v" + strings.Join(parts[:3], ".")
}

// majorMinor extracts the "MAJOR.MINOR" portion of a version string.
func majorMinor(v string) string {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return v
	}
	return parts[0] + "." + parts[1]
}

// toMinor extracts "MAJOR.MINOR" from a full version string.
func toMinor(v string) string {
	return majorMinor(v)
}

// compareMinor compares two "MAJOR.MINOR" strings numerically.
// Returns >0 if a is newer, <0 if b is newer, 0 if equal.
func compareMinor(a, b string) int {
	return compareSemver(a+".0", b+".0")
}

// compareFull compares two full "vMAJOR.MINOR.PATCH" strings.
func compareFull(a, b string) int {
	return compareSemver(a, b)
}

func compareSemver(a, b string) int {
	pa := parseParts(a)
	pb := parseParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] - pb[i]
		}
	}
	return 0
}

func parseParts(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		_, _ = fmt.Sscanf(parts[i], "%d", &out[i])
	}
	return out
}

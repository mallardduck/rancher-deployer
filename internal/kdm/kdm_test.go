package kdm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

// ── Unit tests for pure version helpers ──────────────────────────────────────

func TestNormaliseVersion(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"v1.28.10", "v1.28.10"},
		{"1.28.10", "v1.28.10"},
		{"v1.28.10+k3s1", "v1.28.10"}, // build suffix stripped at dot-split
		{"1.27", ""},                  // only major.minor — invalid
		{"bad", ""},
	}
	for _, c := range cases {
		got := normaliseVersion(c.input)
		if got != c.want {
			t.Errorf("normaliseVersion(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestMajorMinor(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"v1.28.10", "1.28"},
		{"1.28.10", "1.28"},
		{"2.8.5", "2.8"},
		{"1.27", "1.27"},
	}
	for _, c := range cases {
		got := majorMinor(c.input)
		if got != c.want {
			t.Errorf("majorMinor(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestCompareFull(t *testing.T) {
	cases := []struct {
		a, b string
		sign int // -1, 0, +1
	}{
		{"v1.28.10", "v1.28.9", 1},
		{"v1.28.9", "v1.28.10", -1},
		{"v1.28.10", "v1.28.10", 0},
		{"v1.29.0", "v1.28.99", 1},
		{"v1.27.5", "v1.28.0", -1},
	}
	for _, c := range cases {
		got := compareFull(c.a, c.b)
		gotSign := sign(got)
		if gotSign != c.sign {
			t.Errorf("compareFull(%q, %q) sign = %d, want %d", c.a, c.b, gotSign, c.sign)
		}
	}
}

// ── SupportMatrix tests ──────────────────────────────────────────────────────

func makeSupportMatrix(versions []string) *SupportMatrix {
	return &SupportMatrix{
		RancherVersion: "2.8.5",
		k8sVersions:    versions,
	}
}

func TestSupportedMinors(t *testing.T) {
	m := makeSupportMatrix([]string{
		"v1.28.10", "v1.28.9", "v1.27.14", "v1.27.13", "v1.26.17",
	})
	minors := m.SupportedMinors()
	want := []string{"1.28", "1.27", "1.26"}
	if len(minors) != len(want) {
		t.Fatalf("SupportedMinors() = %v, want %v", minors, want)
	}
	for i := range want {
		if minors[i] != want[i] {
			t.Errorf("SupportedMinors()[%d] = %q, want %q", i, minors[i], want[i])
		}
	}
}

func TestLatestPatchFor(t *testing.T) {
	m := makeSupportMatrix([]string{
		"v1.28.10", "v1.28.9", "v1.28.8",
		"v1.27.14", "v1.27.13",
	})

	t.Run("returns latest patch for valid minor", func(t *testing.T) {
		got, err := m.LatestPatchFor("1.28")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "v1.28.10" {
			t.Errorf("got %q, want v1.28.10", got)
		}
	})

	t.Run("strips v prefix from input", func(t *testing.T) {
		got, err := m.LatestPatchFor("v1.27")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "v1.27.14" {
			t.Errorf("got %q, want v1.27.14", got)
		}
	})

	t.Run("errors on unsupported minor", func(t *testing.T) {
		_, err := m.LatestPatchFor("1.25")
		if err == nil {
			t.Error("expected error for unsupported minor, got nil")
		}
	})
}

func TestLatestSupported(t *testing.T) {
	m := makeSupportMatrix([]string{
		"v1.27.14", "v1.28.10", "v1.26.17", "v1.28.9",
	})
	got := m.LatestSupported()
	if got != "v1.28.10" {
		t.Errorf("LatestSupported() = %q, want v1.28.10", got)
	}
}

func TestLatestSupportedEmpty(t *testing.T) {
	m := makeSupportMatrix(nil)
	got := m.LatestSupported()
	if got != "" {
		t.Errorf("LatestSupported() with no versions = %q, want empty", got)
	}
}

// ── matchesRange tests ────────────────────────────────────────────────────────

func TestMatchesRange(t *testing.T) {
	cases := []struct {
		rangeStr string
		version  string
		want     bool
	}{
		{">= 2.8.0-0 < 2.9.0-0", "2.8.5", true},
		{">= 2.8.0-0 < 2.9.0-0", "2.8.0", true},
		{">= 2.8.0-0 < 2.9.0-0", "2.9.0", false},
		{">= 2.8.0-0 < 2.9.0-0", "2.7.9", false},
		{">= 2.6.0-0 < 2.6.5-0", "2.6.4", true},
		{">= 2.6.0-0 < 2.6.5-0", "2.6.5", false},
		{"= 2.8.5", "2.8.5", true},
		{"= 2.8.5", "2.8.4", false},
	}
	for _, c := range cases {
		got := matchesRange(c.rangeStr, c.version)
		if got != c.want {
			t.Errorf("matchesRange(%q, %q) = %v, want %v", c.rangeStr, c.version, got, c.want)
		}
	}
}

// ── HTTP integration test with a mock server ─────────────────────────────────

func TestFetchSupportMatrix_MockServer(t *testing.T) {
	// Minimal KDM JSON mirroring the real schema.
	// AppVersion is a semver range; DefaultVersion is a minor pattern like "1.28.x".
	// Actual patch versions come from the releases array.
	// The target Rancher version is "2.8.5", which falls in ">= 2.8.0-0 < 2.9.0-0".
	fakeKDM := map[string]interface{}{
		"k3s": map[string]interface{}{
			"appDefaults": []map[string]interface{}{
				{
					"appName": "rancher",
					"defaults": []map[string]interface{}{
						{"appVersion": ">= 2.8.0-0 < 2.9.0-0", "defaultVersion": "1.28.x"},
						{"appVersion": ">= 2.8.0-0 < 2.9.0-0", "defaultVersion": "1.27.x"},
						{"appVersion": ">= 2.6.0-0 < 2.7.0-0", "defaultVersion": "1.25.x"}, // different range — excluded
					},
				},
			},
			"releases": []map[string]interface{}{
				{"version": "v1.28.10+k3s1"},
				{"version": "v1.28.9+k3s1"},
				{"version": "v1.27.14+k3s1"},
				{"version": "v1.25.16+k3s1"}, // minor not in supported set for 2.8.5
			},
		},
		"rke2": map[string]interface{}{
			"appDefaults": []map[string]interface{}{
				{
					"appName": "rancher",
					"defaults": []map[string]interface{}{
						{"appVersion": ">= 2.8.0-0 < 2.9.0-0", "defaultVersion": "1.26.x"},
					},
				},
			},
			"releases": []map[string]interface{}{
				{"version": "v1.26.17+rke2r1"},
			},
		},
	}
	body, _ := json.Marshal(fakeKDM)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body) //nolint:errcheck
	}))
	defer ts.Close()

	// Test doFetch directly since the URL constants can't be overridden.
	client := &http.Client{}
	data, err := doFetch(client, ts.URL)
	if err != nil {
		t.Fatalf("doFetch() error: %v", err)
	}

	versions := extractVersions(data, "2.8.5")
	if len(versions) == 0 {
		t.Fatal("extractVersions() returned no versions")
	}

	sort.Strings(versions)
	wantVersions := []string{"v1.26.17", "v1.27.14", "v1.28.10", "v1.28.9"}
	sort.Strings(wantVersions)

	if len(versions) != len(wantVersions) {
		t.Errorf("extractVersions() = %v, want %v", versions, wantVersions)
	}
	for i := range wantVersions {
		if versions[i] != wantVersions[i] {
			t.Errorf("versions[%d] = %q, want %q", i, versions[i], wantVersions[i])
		}
	}
}

func TestExtractVersionsDeduplicates(t *testing.T) {
	// The same k8s base version (v1.28.10) appears in both k3s and rke2 releases —
	// after stripping build metadata they should collapse to a single entry.
	d := &kdmData{
		K3sInfo: distroInfo{
			AppDefaults: []appDefaultIndex{
				{
					AppName: "rancher",
					Defaults: []appDefaults{
						{AppVersion: ">= 2.8.0-0 < 2.9.0-0", DefaultVersion: "1.28.x"},
					},
				},
			},
			Releases: []distroRelease{
				{Version: "v1.28.10+k3s1"},
				{Version: "v1.28.10+k3s2"}, // same base, different build
			},
		},
		Rke2Info: distroInfo{
			AppDefaults: []appDefaultIndex{
				{
					AppName: "rancher",
					Defaults: []appDefaults{
						{AppVersion: ">= 2.8.0-0 < 2.9.0-0", DefaultVersion: "1.28.x"},
					},
				},
			},
			Releases: []distroRelease{
				{Version: "v1.28.10+rke2r1"},
			},
		},
	}
	versions := extractVersions(d, "2.8.5")
	if len(versions) != 1 {
		t.Errorf("expected 1 deduplicated version, got %d: %v", len(versions), versions)
	}
	if versions[0] != "v1.28.10" {
		t.Errorf("got %q, want v1.28.10", versions[0])
	}
}

func TestExtractVersionsFiltersRancherVersion(t *testing.T) {
	// Releases for a minor outside the supported range must not appear in output.
	d := &kdmData{
		K3sInfo: distroInfo{
			AppDefaults: []appDefaultIndex{
				{
					AppName: "rancher",
					Defaults: []appDefaults{
						{AppVersion: ">= 2.8.0-0 < 2.9.0-0", DefaultVersion: "1.28.x"},
						{AppVersion: ">= 2.7.0-0 < 2.8.0-0", DefaultVersion: "1.25.x"}, // different Rancher line
					},
				},
			},
			Releases: []distroRelease{
				{Version: "v1.28.10+k3s1"},
				{Version: "v1.25.16+k3s1"}, // must be excluded for 2.8.5
			},
		},
	}
	versions := extractVersions(d, "2.8.5")
	if len(versions) != 1 || versions[0] != "v1.28.10" {
		t.Errorf("extractVersions filtered incorrectly: got %v, want [v1.28.10]", versions)
	}
}

func TestFetchSupportMatrix_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	client := &http.Client{}
	_, err := doFetch(client, ts.URL)
	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}

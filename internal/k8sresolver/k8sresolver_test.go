package k8sresolver

import (
	"testing"
)

// ── compareK3sTag ─────────────────────────────────────────────────────────────

func TestCompareK3sTag(t *testing.T) {
	cases := []struct {
		a, b string
		sign int // -1 = a older, 0 = equal, +1 = a newer
	}{
		// Same semver, different build number
		{"v1.28.10+k3s2", "v1.28.10+k3s1", 1},
		{"v1.28.10+k3s1", "v1.28.10+k3s2", -1},
		{"v1.28.10+k3s1", "v1.28.10+k3s1", 0},
		// Different patch, same build number
		{"v1.28.11+k3s1", "v1.28.10+k3s1", 1},
		{"v1.28.10+k3s1", "v1.28.11+k3s1", -1},
		// Different minor
		{"v1.29.0+k3s1", "v1.28.99+k3s1", 1},
		// Tag without build suffix defaults to k3s0
		{"v1.28.10+k3s1", "v1.28.10", 1},
		{"v1.28.10", "v1.28.10+k3s1", -1},
	}
	for _, c := range cases {
		got := compareK3sTag(c.a, c.b)
		gotSign := signOf(got)
		if gotSign != c.sign {
			t.Errorf("compareK3sTag(%q, %q) sign = %d, want %d", c.a, c.b, gotSign, c.sign)
		}
	}
}

// ── latestTagForPatch ─────────────────────────────────────────────────────────

func TestLatestTagForPatch(t *testing.T) {
	releases := []ghRelease{
		{TagName: "v1.28.10+k3s1"},
		{TagName: "v1.28.10+k3s2"},
		{TagName: "v1.28.9+k3s1"},
		{TagName: "v1.28.10+k3s3-rc1", Prerelease: true}, // excluded
	}

	t.Run("returns highest k3sN for exact patch", func(t *testing.T) {
		got := latestTagForPatch(releases, "v1.28.10")
		if got != "v1.28.10+k3s2" {
			t.Errorf("got %q, want v1.28.10+k3s2", got)
		}
	})

	t.Run("adds v prefix if missing", func(t *testing.T) {
		got := latestTagForPatch(releases, "1.28.10")
		if got != "v1.28.10+k3s2" {
			t.Errorf("got %q, want v1.28.10+k3s2", got)
		}
	})

	t.Run("returns empty when no match", func(t *testing.T) {
		got := latestTagForPatch(releases, "v1.27.0")
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("excludes prereleases", func(t *testing.T) {
		// Only prerelease for this patch — should return empty
		got := latestTagForPatch(releases, "v1.28.10+k3s3-rc1")
		if got != "" {
			t.Errorf("got %q, want empty (only prerelease available)", got)
		}
	})
}

// ── latestTagForMinor ─────────────────────────────────────────────────────────

func TestLatestTagForMinor(t *testing.T) {
	releases := []ghRelease{
		{TagName: "v1.28.10+k3s1"},
		{TagName: "v1.28.11+k3s1"},
		{TagName: "v1.28.11+k3s2"},
		{TagName: "v1.27.14+k3s1"},
		{TagName: "v1.28.12+k3s1-rc1", Prerelease: true},
	}

	t.Run("returns newest patch+build in minor", func(t *testing.T) {
		got := latestTagForMinor(releases, "1.28")
		if got != "v1.28.11+k3s2" {
			t.Errorf("got %q, want v1.28.11+k3s2", got)
		}
	})

	t.Run("strips v prefix from minor input", func(t *testing.T) {
		got := latestTagForMinor(releases, "v1.27")
		if got != "v1.27.14+k3s1" {
			t.Errorf("got %q, want v1.27.14+k3s1", got)
		}
	})

	t.Run("returns empty when no match", func(t *testing.T) {
		got := latestTagForMinor(releases, "1.26")
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// ── latestTagForPatch → latestTagForMinor fallback ───────────────────────────

// TestFallbackToMinor confirms that when there is no k3s release for an exact
// patch, latestTagForMinor is used instead (the ResolveClusterVersion strategy).
func TestFallbackToMinor(t *testing.T) {
	releases := []ghRelease{
		// No release for v1.28.10, but v1.28.9 exists.
		{TagName: "v1.28.9+k3s1"},
		{TagName: "v1.28.8+k3s1"},
	}

	// Exact patch lookup returns empty — fallback logic should kick in.
	exactTag := latestTagForPatch(releases, "v1.28.10")
	if exactTag != "" {
		t.Fatalf("expected no exact match for v1.28.10, got %q", exactTag)
	}

	fallbackTag := latestTagForMinor(releases, "1.28")
	if fallbackTag != "v1.28.9+k3s1" {
		t.Errorf("minor fallback got %q, want v1.28.9+k3s1", fallbackTag)
	}
}

func TestIsPrerelease(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"2.8.5", false},
		{"2.8.5-rc1", true},
		{"2.9.0-alpha1", true},
		{"v2.8.5", false},
		{"v2.8.5-rc1", true},
	}
	for _, c := range cases {
		if got := IsPrerelease(c.v); got != c.want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func signOf(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}

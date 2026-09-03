package rancher

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ── NormaliseChannel ──────────────────────────────────────────────────────────

func TestNormaliseChannel(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "stable", want: ChannelStable, wantErr: false},
		{input: "Stable", want: ChannelStable, wantErr: false},
		{input: "STABLE", want: ChannelStable, wantErr: false},
		{input: "ga", want: ChannelStable, wantErr: false},
		{input: "GA", want: ChannelStable, wantErr: false},
		{input: "latest", want: ChannelLatest, wantErr: false},
		{input: "Latest", want: ChannelLatest, wantErr: false},
		{input: "rc", want: ChannelLatest, wantErr: false},
		{input: "RC", want: ChannelLatest, wantErr: false},
		{input: "alpha", want: ChannelAlpha, wantErr: false},
		{input: "Alpha", want: ChannelAlpha, wantErr: false},
		{input: "ALPHA", want: ChannelAlpha, wantErr: false},
		{input: "invalid", want: "", wantErr: true},
		{input: "beta", want: "", wantErr: true},
		{input: "", want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormaliseChannel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("NormaliseChannel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("NormaliseChannel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormaliseChannelHead(t *testing.T) {
	got, err := NormaliseChannel("HEAD")
	if err != nil {
		t.Fatalf("NormaliseChannel(\"HEAD\") unexpected error: %v", err)
	}
	if got != ChannelHead {
		t.Errorf("NormaliseChannel(\"HEAD\") = %q, want %q", got, ChannelHead)
	}
}

// ── headRepoURL / headRepoName ─────────────────────────────────────────────

func TestHeadRepoURL(t *testing.T) {
	t.Run("community is per-minor", func(t *testing.T) {
		got := headRepoURL(false, "2.15")
		want := "https://charts.optimus.rancher.io/server-charts/release-2.15"
		if got != want {
			t.Errorf("headRepoURL(false, \"2.15\") = %q, want %q", got, want)
		}
	})
	t.Run("prime uses the configured head repo regardless of minor", func(t *testing.T) {
		got215 := headRepoURL(true, "2.15")
		got216 := headRepoURL(true, "2.16")
		if got215 != got216 {
			t.Errorf("headRepoURL(true, ...) should not vary by minor, got %q vs %q", got215, got216)
		}
		if got215 != repoURLs[true][ChannelHead] {
			t.Errorf("headRepoURL(true, ...) = %q, want repoURLs[true][ChannelHead] = %q", got215, repoURLs[true][ChannelHead])
		}
	})
}

func TestHeadRepoName(t *testing.T) {
	if got := headRepoName(false, "2.15"); got != "rancher-head-2.15" {
		t.Errorf("headRepoName(false, \"2.15\") = %q, want %q", got, "rancher-head-2.15")
	}
	if got := headRepoName(true, "2.15"); got != repoNames[true][ChannelHead] {
		t.Errorf("headRepoName(true, \"2.15\") = %q, want %q", got, repoNames[true][ChannelHead])
	}
}

// ── versionMinor ──────────────────────────────────────────────────────────────

func TestVersionMinor(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"2.8.5", "2.8"},
		{"v2.9.0-rc1", "2.9"},
		{"2.15-9f0d0301586ef2c690062d3a42bb3a91edfd3e12-head", "2.15"},
		{"2.15.2-fbf2130-head", "2.15"},
		{"2.15", "2.15"},
		{"bogus", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := versionMinor(c.input)
		if got != c.want {
			t.Errorf("versionMinor(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ── resolveMinorEntry / resolveHeadEntry ─────────────────────────────────────

func TestResolveMinorEntry(t *testing.T) {
	entries := []helmIndexEntry{
		{Version: "2.14.3"},
		{Version: "2.14.2"},
		{Version: "2.15.0"},
		{Version: "2.14.3-rc1"},
	}

	t.Run("picks newest in requested minor", func(t *testing.T) {
		got, err := resolveMinorEntry(entries, "2.14")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Version != "2.14.3" {
			t.Errorf("resolveMinorEntry() = %q, want %q", got.Version, "2.14.3")
		}
	})

	t.Run("errors for unknown minor", func(t *testing.T) {
		_, err := resolveMinorEntry(entries, "9.99")
		if err == nil {
			t.Error("expected error for unknown minor, got nil")
		}
	})
}

func TestResolveHeadEntry(t *testing.T) {
	entries := []helmIndexEntry{
		{Version: "2.15.2-fbf2130-head", Created: "2026-09-01T10:00:00Z"},
		{Version: "2.15.2-a90f264-head", Created: "2026-09-02T21:02:39Z"}, // newest
		{Version: "2.15.1-rc2", Created: "2026-08-20T00:00:00Z"},          // not a head build
		{Version: "2.16.0-de47bc4-head", Created: "2026-09-03T00:00:00Z"}, // different minor
	}

	t.Run("picks newest head build by created timestamp within minor", func(t *testing.T) {
		got, err := resolveHeadEntry(entries, "2.15")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Version != "2.15.2-a90f264-head" {
			t.Errorf("resolveHeadEntry() = %q, want %q", got.Version, "2.15.2-a90f264-head")
		}
	})

	t.Run("errors when no head build exists for the minor", func(t *testing.T) {
		_, err := resolveHeadEntry(entries, "9.99")
		if err == nil {
			t.Error("expected error for minor with no head build, got nil")
		}
	})

	t.Run("ignores non-head entries in the same minor", func(t *testing.T) {
		single := []helmIndexEntry{
			{Version: "2.15.1-rc2", Created: "2026-08-20T00:00:00Z"},
		}
		_, err := resolveHeadEntry(single, "2.15")
		if err == nil {
			t.Error("expected error since no entry has a -head suffix, got nil")
		}
	})
}

// ── ResolveChart ──────────────────────────────────────────────────────────────

func TestResolveChartExactVersionPassthrough(t *testing.T) {
	got, err := ResolveChart(false, ChannelStable, "2.8.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != "2.8.5" {
		t.Errorf("Version = %q, want %q", got.Version, "2.8.5")
	}
	if got.RepoURL != repoURLs[false][ChannelStable] {
		t.Errorf("RepoURL = %q, want %q", got.RepoURL, repoURLs[false][ChannelStable])
	}
	if got.IsPrerelease {
		t.Error("IsPrerelease = true, want false for a plain patch version")
	}
}

func TestResolveChartHeadRequiresMinor(t *testing.T) {
	_, err := ResolveChart(false, ChannelHead, "")
	if err == nil {
		t.Error("expected error when head channel is given no version, got nil")
	}

	_, err = ResolveChart(false, ChannelHead, "not-a-version")
	if err == nil {
		t.Error("expected error when head channel is given an unparseable version, got nil")
	}
}

// ── ResolveHeadChartByCommit validation (no network needed — these all
// short-circuit before fetching an index) ───────────────────────────────────

func TestResolveHeadChartByCommitRequiresCommit(t *testing.T) {
	_, err := ResolveHeadChartByCommit(true, "2.15", "")
	if err == nil {
		t.Error("expected error when commit is empty, got nil")
	}
}

func TestResolveHeadChartByCommitCommunityRequiresMinor(t *testing.T) {
	_, err := ResolveHeadChartByCommit(false, "", "b03c4de")
	if err == nil {
		t.Error("expected error for community with no minor, got nil")
	}
}

// ── Realistic mixed-index fixtures ───────────────────────────────────────────
//
// primeLatestFixture is a frozen, hand-written snapshot — not fetched live —
// shaped after the real Prime "latest" repo index
// (charts.optimus.rancher.io/server-charts/latest) as observed on 2026-09-03.
// It will keep passing unchanged regardless of what Rancher ships later
// (e.g. once 2.16 GAs and `main` moves on to 2.17): it isn't asserting
// anything about *current* upstream state, only that the resolver handles
// two minor shapes correctly:
//   - "2.15": an established minor with GA + RC + head entries all present
//     in the same index (mirrors how Prime's "latest" repo mixes them).
//   - "2.16": a brand-new minor with head builds only and no GA/RC at all —
//     the shape any not-yet-released minor has, modeled here on 2.16 because
//     that's what `main` looked like at the time this was written.
func primeLatestFixture() []helmIndexEntry {
	return []helmIndexEntry{
		{Version: "2.15.0", Created: "2026-07-01T00:00:00Z"},
		{Version: "2.15.0-rc1", Created: "2026-06-20T00:00:00Z"},
		{Version: "2.15.1", Created: "2026-08-01T00:00:00Z"},
		{Version: "2.15.1-rc2", Created: "2026-07-25T00:00:00Z"},
		{Version: "2.15.2-fbf2130-head", Created: "2026-09-01T10:00:00Z"},
		{Version: "2.15.2-a90f264-head", Created: "2026-09-02T21:02:39Z"},
		// No GA/RC entries at all for this minor — only head builds.
		{Version: "2.16.0-de47bc4-head", Created: "2026-09-02T18:00:00Z"},
		{Version: "2.16.0-ff28ae6aea8dcb820bcc2ff8bfd1bc45d4e2ee65-head", Created: "2026-09-03T09:26:05Z"},
	}
}

func TestResolveMinorEntry_IgnoresHeadBuildsInSharedIndex(t *testing.T) {
	// A bare-minor "latest" resolution for 2.15 must land on the actual GA
	// (2.15.1), never on a 2.15.2-*-head entry — even though a head build's
	// patch number sorts numerically above the current GA.
	got, err := resolveMinorEntry(primeLatestFixture(), "2.15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Version != "2.15.1" {
		t.Errorf("resolveMinorEntry(\"2.15\") = %q, want %q (must not pick a head build)", got.Version, "2.15.1")
	}
}

func TestResolveMinorEntry_MinorWithOnlyHeadEntries(t *testing.T) {
	// A minor with no GA/RC in the index — only head builds. resolveMinorEntry
	// (used by non-head channels) must fail clearly rather than accidentally
	// falling through to a head entry.
	_, err := resolveMinorEntry(primeLatestFixture(), "2.16")
	if err == nil {
		t.Error("expected error resolving a non-head version for a minor with only head entries, got nil")
	}
}

func TestResolveHeadEntry_MinorWithOnlyHeadEntries(t *testing.T) {
	// The head channel is exactly the path that *should* work when a minor
	// has no stable release yet: pick the newest head build by created
	// timestamp.
	got, err := resolveHeadEntry(primeLatestFixture(), "2.16")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2.16.0-ff28ae6aea8dcb820bcc2ff8bfd1bc45d4e2ee65-head"
	if got.Version != want {
		t.Errorf("resolveHeadEntry(\"2.16\") = %q, want %q", got.Version, want)
	}
}

func TestResolveHeadEntry_EstablishedMinorPicksNewestHeadNotGA(t *testing.T) {
	// For a minor that already has a GA (2.15), the head channel must still
	// resolve to the newest *head* build, not the GA/RC entries.
	got, err := resolveHeadEntry(primeLatestFixture(), "2.15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "2.15.2-a90f264-head"
	if got.Version != want {
		t.Errorf("resolveHeadEntry(\"2.15\") = %q, want %q", got.Version, want)
	}
}

// ── headBuildCommit / resolveHeadEntryByCommit ───────────────────────────────

func TestHeadBuildCommit(t *testing.T) {
	cases := []struct {
		version string
		want    string
	}{
		{"2.16.0-b03c4de-head", "b03c4de"}, // prime, short SHA
		{"2.15-9f0d0301586ef2c690062d3a42bb3a91edfd3e12-head", "9f0d0301586ef2c690062d3a42bb3a91edfd3e12"}, // community, full SHA
		{"2.15.1", ""},     // GA, not a head build
		{"2.15.1-rc2", ""}, // RC, not a head build
		{"bogus", ""},
	}
	for _, c := range cases {
		got := headBuildCommit(c.version)
		if got != c.want {
			t.Errorf("headBuildCommit(%q) = %q, want %q", c.version, got, c.want)
		}
	}
}

// TestCommitMatches covers the length-mismatch cases discovered live:
// Prime's head index used full 40-char SHAs until 2026-08-24, then switched
// to short 7-char ones — so matching has to work in both directions, not
// just "does the index entry start with what the user typed."
func TestCommitMatches(t *testing.T) {
	cases := []struct {
		name, entrySHA, input string
		want                  bool
	}{
		{"short input, short entry, exact", "de47bc4", "de47bc4", true},
		{"short input, short entry, prefix", "de47bc4", "de47", true},
		{"short input, full entry, prefix", "7ee3371f7d4458b53ca126d7a180db1148633f4c", "7ee3371", true},
		{"full input, short entry", "de47bc4", "de47bc4a4171597c4744f82a77b5adb94f46ce3a", true}, // the exact case found live
		{"full input, full entry, exact", "7ee3371f7d4458b53ca126d7a180db1148633f4c", "7ee3371f7d4458b53ca126d7a180db1148633f4c", true},
		{"no relation", "de47bc4", "b03c4de", false},
		{"empty entry", "", "de47bc4", false},
		{"empty input", "de47bc4", "", false},
		{"both empty", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := commitMatches(c.entrySHA, c.input)
			if got != c.want {
				t.Errorf("commitMatches(%q, %q) = %v, want %v", c.entrySHA, c.input, got, c.want)
			}
		})
	}
}

func TestResolveHeadEntryByCommit_ScopedToMinor(t *testing.T) {
	entries := primeLatestFixture()

	t.Run("matches within the given minor", func(t *testing.T) {
		got, err := resolveHeadEntryByCommit(entries, "2.15", "fbf2130")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Version != "2.15.2-fbf2130-head" {
			t.Errorf("got %q, want %q", got.Version, "2.15.2-fbf2130-head")
		}
	})

	t.Run("full commit hash matches a short-hash index entry", func(t *testing.T) {
		// "2.16.0-de47bc4-head" only has the short form in the index (see
		// primeLatestFixture); a user pasting the full 40-char SHA (e.g.
		// straight from GitHub) must still find it.
		got, err := resolveHeadEntryByCommit(entries, "2.16", "de47bc4a4171597c4744f82a77b5adb94f46ce3a")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Version != "2.16.0-de47bc4-head" {
			t.Errorf("got %q, want %q", got.Version, "2.16.0-de47bc4-head")
		}
	})

	t.Run("case-insensitive prefix match", func(t *testing.T) {
		got, err := resolveHeadEntryByCommit(entries, "2.16", "DE47")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Version != "2.16.0-de47bc4-head" {
			t.Errorf("got %q, want %q", got.Version, "2.16.0-de47bc4-head")
		}
	})

	t.Run("commit belongs to a different minor", func(t *testing.T) {
		// de47bc4 is a 2.16 build; asking for it under 2.15 must not match.
		_, err := resolveHeadEntryByCommit(entries, "2.15", "de47")
		if err == nil {
			t.Error("expected error when commit doesn't belong to the given minor, got nil")
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, err := resolveHeadEntryByCommit(entries, "2.15", "0000000")
		if err == nil {
			t.Error("expected error for unmatched commit, got nil")
		}
	})
}

func TestResolveHeadEntryByCommit_AnyMinor(t *testing.T) {
	entries := primeLatestFixture()

	t.Run("unique match across minors", func(t *testing.T) {
		// de47bc4 only appears under 2.16; searching with no minor filter
		// (minor == "") must still find it uniquely.
		got, err := resolveHeadEntryByCommit(entries, "", "de47")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Version != "2.16.0-de47bc4-head" {
			t.Errorf("got %q, want %q", got.Version, "2.16.0-de47bc4-head")
		}
	})

	t.Run("ambiguous prefix errors and lists candidates", func(t *testing.T) {
		// Both "fbf2130" (2.15) and "ff28ae6aea8dcb820bcc2ff8bfd1bc45d4e2ee65"
		// (2.16) start with "f" — a short prefix searched across all minors
		// must fail rather than silently pick one.
		_, err := resolveHeadEntryByCommit(entries, "", "f")
		if err == nil {
			t.Fatal("expected error for an ambiguous commit prefix, got nil")
		}
		if !strings.Contains(err.Error(), "2.15.2-fbf2130-head") || !strings.Contains(err.Error(), "2.16.0-ff28ae") {
			t.Errorf("error should list both candidates, got: %v", err)
		}
	})
}

func TestFetchLatestVersion_ExcludesHeadBuilds(t *testing.T) {
	// FetchLatestVersion backs the no-version-given auto-resolve path
	// (e.g. `resolve --prime`, default channel latest). Even though 2.16
	// head builds are numerically "newer" than the 2.15.x GA/RC entries in
	// the shared index, a non-head channel must never resolve to one.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_ = yaml.NewEncoder(w).Encode(struct {
			Entries map[string][]helmIndexEntry `yaml:"entries"`
		}{
			Entries: map[string][]helmIndexEntry{
				chartName: primeLatestFixture(),
			},
		})
	}))
	defer server.Close()

	orig := repoURLs[true][ChannelLatest]
	repoURLs[true][ChannelLatest] = server.URL
	defer func() { repoURLs[true][ChannelLatest] = orig }()

	got, err := FetchLatestVersion(true, ChannelLatest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "2.15.1" {
		t.Errorf("FetchLatestVersion() = %q, want %q (must not pick a 2.16 head build)", got, "2.15.1")
	}
}

// ── ChartRef ──────────────────────────────────────────────────────────────────

func TestChartRef(t *testing.T) {
	tests := []struct {
		name         string
		prime        bool
		prerelease   bool
		channel      string
		version      string
		wantRepoName string
		wantRepoURL  string
		wantVersion  string
	}{
		{
			name:         "community stable",
			prime:        false,
			prerelease:   false,
			channel:      ChannelStable,
			version:      "2.8.5",
			wantRepoName: "rancher-stable",
			wantRepoURL:  "https://releases.rancher.com/server-charts/stable",
			wantVersion:  "2.8.5",
		},
		{
			name:         "community latest",
			prime:        false,
			prerelease:   true,
			channel:      ChannelLatest,
			version:      "2.9.0-rc1",
			wantRepoName: "rancher-latest",
			wantRepoURL:  "https://releases.rancher.com/server-charts/latest",
			wantVersion:  "2.9.0-rc1",
		},
		{
			name:         "prime stable",
			prime:        true,
			prerelease:   false,
			channel:      ChannelStable,
			version:      "2.8.5",
			wantRepoName: "rancher-prime",
			wantRepoURL:  "https://charts.rancher.com/server-charts/prime",
			wantVersion:  "2.8.5",
		},
		{
			name:         "prime alpha",
			prime:        true,
			prerelease:   true,
			channel:      ChannelAlpha,
			version:      "2.9.0-alpha1",
			wantRepoName: "rancher-prime-alpha",
			wantRepoURL:  "https://charts.optimus.rancher.io/server-charts/alpha",
			wantVersion:  "2.9.0-alpha1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChartRef(tt.prime, tt.prerelease, tt.channel, tt.version)
			if got.RepoName != tt.wantRepoName {
				t.Errorf("ChartRef().RepoName = %q, want %q", got.RepoName, tt.wantRepoName)
			}
			if got.RepoURL != tt.wantRepoURL {
				t.Errorf("ChartRef().RepoURL = %q, want %q", got.RepoURL, tt.wantRepoURL)
			}
			if got.ChartName != chartName {
				t.Errorf("ChartRef().ChartName = %q, want %q", got.ChartName, chartName)
			}
			if got.Version != tt.wantVersion {
				t.Errorf("ChartRef().Version = %q, want %q", got.Version, tt.wantVersion)
			}
			if got.IsPrerelease != tt.prerelease {
				t.Errorf("ChartRef().IsPrerelease = %v, want %v", got.IsPrerelease, tt.prerelease)
			}
		})
	}
}

func TestChartString(t *testing.T) {
	chart := Chart{
		RepoName:  "rancher-stable",
		ChartName: "rancher",
		Version:   "2.8.5",
	}
	want := "rancher-stable/rancher @ 2.8.5"
	got := chart.String()
	if got != want {
		t.Errorf("Chart.String() = %q, want %q", got, want)
	}
}

// ── injectIfAbsent ────────────────────────────────────────────────────────────

func TestInjectIfAbsent(t *testing.T) {
	tests := []struct {
		name     string
		sets     []string
		key      string
		value    string
		wantLen  int
		wantLast string // expected last element if injected
	}{
		{
			name:     "inject when absent",
			sets:     []string{"foo=bar"},
			key:      "hostname",
			value:    "test.local",
			wantLen:  2,
			wantLast: "hostname=test.local",
		},
		{
			name:     "do not inject when present",
			sets:     []string{"hostname=existing.com", "foo=bar"},
			key:      "hostname",
			value:    "test.local",
			wantLen:  2,
			wantLast: "foo=bar",
		},
		{
			name:     "inject into empty slice",
			sets:     []string{},
			key:      "test",
			value:    "value",
			wantLen:  1,
			wantLast: "test=value",
		},
		{
			name:     "respect partial key matches",
			sets:     []string{"hostname2=other"},
			key:      "hostname",
			value:    "test.local",
			wantLen:  2,
			wantLast: "hostname=test.local",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectIfAbsent(tt.sets, tt.key, tt.value)
			if len(got) != tt.wantLen {
				t.Errorf("injectIfAbsent() len = %d, want %d", len(got), tt.wantLen)
			}
			if len(got) > 0 && got[len(got)-1] != tt.wantLast {
				t.Errorf("injectIfAbsent() last = %q, want %q", got[len(got)-1], tt.wantLast)
			}
		})
	}
}

// ── injectHostname ────────────────────────────────────────────────────────────

func TestInjectHostname(t *testing.T) {
	tests := []struct {
		name     string
		sets     []string
		hostname string
		want     []string
	}{
		{
			name:     "inject when absent",
			sets:     []string{"foo=bar"},
			hostname: "rancher.local",
			want:     []string{"foo=bar", "hostname=rancher.local"},
		},
		{
			name:     "do not inject when present",
			sets:     []string{"hostname=existing.com", "foo=bar"},
			hostname: "rancher.local",
			want:     []string{"hostname=existing.com", "foo=bar"},
		},
		{
			name:     "inject into empty slice",
			sets:     []string{},
			hostname: "test.local",
			want:     []string{"hostname=test.local"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectHostname(tt.sets, tt.hostname)
			if len(got) != len(tt.want) {
				t.Fatalf("injectHostname() len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("injectHostname()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ── resolveHostname ───────────────────────────────────────────────────────────

func TestResolveHostname(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		setFlags []string
		wantErr  bool
		contains string // substring that should be in the result
	}{
		{
			name:     "explicit hostname takes precedence",
			explicit: "rancher.example.com",
			setFlags: []string{"hostname=other.com"},
			wantErr:  false,
			contains: "rancher.example.com",
		},
		{
			name:     "use hostname from set flags",
			explicit: "",
			setFlags: []string{"foo=bar", "hostname=from-set.com"},
			wantErr:  false,
			contains: "from-set.com",
		},
		{
			name:     "auto-detect when no explicit or set hostname",
			explicit: "",
			setFlags: []string{"foo=bar"},
			wantErr:  false,
			contains: ".sslip.io", // should auto-generate sslip.io hostname
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveHostname(tt.explicit, tt.setFlags)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveHostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == "" {
				t.Error("resolveHostname() returned empty string")
			}
			if tt.contains != "" && !contains(got, tt.contains) {
				t.Errorf("resolveHostname() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

// ── BuildHelmValues ───────────────────────────────────────────────────────────

func TestBuildHelmValues(t *testing.T) {
	t.Run("validates values file exists", func(t *testing.T) {
		_, err := BuildHelmValues("/nonexistent/file.yaml", nil, "", "default", "password")
		if err == nil {
			t.Error("BuildHelmValues should error for nonexistent values file")
		}
	})

	t.Run("accepts empty values file", func(t *testing.T) {
		_, err := BuildHelmValues("", nil, "test.local", "default", "password")
		if err != nil {
			t.Errorf("BuildHelmValues with empty values file should not error: %v", err)
		}
	})

	t.Run("accepts existing values file", func(t *testing.T) {
		// Create a temp file
		tmpDir := t.TempDir()
		valuesFile := filepath.Join(tmpDir, "values.yaml")
		if err := os.WriteFile(valuesFile, []byte("foo: bar\n"), 0644); err != nil {
			t.Fatalf("Failed to create test values file: %v", err)
		}

		got, err := BuildHelmValues(valuesFile, nil, "test.local", "default", "password")
		if err != nil {
			t.Errorf("BuildHelmValues should accept existing file: %v", err)
		}
		if got.ValuesFile != valuesFile {
			t.Errorf("BuildHelmValues().ValuesFile = %q, want %q", got.ValuesFile, valuesFile)
		}
	})

	t.Run("injects hostname and bootstrapPassword", func(t *testing.T) {
		got, err := BuildHelmValues("", nil, "test.local", "default", "mypass")
		if err != nil {
			t.Fatalf("BuildHelmValues failed: %v", err)
		}

		if !containsSetFlag(got.SetFlags, "hostname=test.local") {
			t.Error("BuildHelmValues should inject hostname")
		}
		if !containsSetFlag(got.SetFlags, "bootstrapPassword=mypass") {
			t.Error("BuildHelmValues should inject bootstrapPassword")
		}
	})

	t.Run("does not override user-provided values", func(t *testing.T) {
		setFlags := []string{"hostname=user.com", "bootstrapPassword=userpass"}
		got, err := BuildHelmValues("", setFlags, "default.local", "default", "defaultpass")
		if err != nil {
			t.Fatalf("BuildHelmValues failed: %v", err)
		}

		if !containsSetFlag(got.SetFlags, "hostname=user.com") {
			t.Error("BuildHelmValues should preserve user hostname")
		}
		if !containsSetFlag(got.SetFlags, "bootstrapPassword=userpass") {
			t.Error("BuildHelmValues should preserve user bootstrapPassword")
		}
	})
}

// ── githubToken ───────────────────────────────────────────────────────────────

func TestGithubToken(t *testing.T) {
	// Save original env vars
	oldGH := os.Getenv("GH_TOKEN")
	oldGitHub := os.Getenv("GITHUB_TOKEN")
	defer func() {
		os.Setenv("GH_TOKEN", oldGH)
		os.Setenv("GITHUB_TOKEN", oldGitHub)
	}()

	tests := []struct {
		name      string
		ghToken   string
		ghubToken string
		want      string
	}{
		{
			name:      "prefers GH_TOKEN",
			ghToken:   "gh-token-123",
			ghubToken: "github-token-456",
			want:      "gh-token-123",
		},
		{
			name:      "falls back to GITHUB_TOKEN",
			ghToken:   "",
			ghubToken: "github-token-456",
			want:      "github-token-456",
		},
		{
			name:      "returns empty when neither set",
			ghToken:   "",
			ghubToken: "",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GH_TOKEN", tt.ghToken)
			os.Setenv("GITHUB_TOKEN", tt.ghubToken)

			got := githubToken()
			if got != tt.want {
				t.Errorf("githubToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ── Test helpers ──────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr))))
}

func containsSetFlag(flags []string, target string) bool {
	for _, f := range flags {
		if f == target {
			return true
		}
	}
	return false
}

package rancher

import (
	"os"
	"path/filepath"
	"testing"
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

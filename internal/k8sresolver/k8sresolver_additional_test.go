package k8sresolver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── extractMinor ──────────────────────────────────────────────────────────────

func TestExtractMinor(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "v1.28.10", want: "1.28"},
		{input: "1.28.10", want: "1.28"},
		{input: "v1.29.0", want: "1.29"},
		{input: "1.30.5+k3s1", want: "1.30"},
		{input: "v1.27", want: "1.27"},
		{input: "1.26", want: "1.26"},
		{input: "1", want: "1"},
		{input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractMinor(tt.input)
			if got != tt.want {
				t.Errorf("extractMinor(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── extractFull ───────────────────────────────────────────────────────────────

func TestExtractFull(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "v1.28.10+k3s1", want: "v1.28.10+k3s1"}, // splits on dots, not plus
		{input: "1.28.10+k3s2", want: "v1.28.10+k3s2"},
		{input: "v1.29.0", want: "v1.29.0"},
		{input: "1.30.5", want: "v1.30.5"},
		{input: "v1.27", want: ""}, // not enough parts
		{input: "1.26", want: ""},  // not enough parts
		{input: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractFull(tt.input)
			if got != tt.want {
				t.Errorf("extractFull(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ── compareMinors ─────────────────────────────────────────────────────────────

func TestCompareMinors(t *testing.T) {
	tests := []struct {
		a, b string
		want int // -1 = a<b, 0 = equal, 1 = a>b
	}{
		{a: "1.28", b: "1.27", want: 1},
		{a: "1.27", b: "1.28", want: -1},
		{a: "1.28", b: "1.28", want: 0},
		{a: "1.30", b: "1.29", want: 1},
		{a: "2.0", b: "1.99", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := compareMinors(tt.a, tt.b)
			gotSign := sign(got)
			wantSign := sign(tt.want)
			if gotSign != wantSign {
				t.Errorf("compareMinors(%q, %q) sign = %d, want %d", tt.a, tt.b, gotSign, wantSign)
			}
		})
	}
}

// ── ResolveK8s ────────────────────────────────────────────────────────────────

func TestResolveK8s(t *testing.T) {
	// This function requires a real kdm.SupportMatrix
	// We'll skip this for now and test it through integration tests
	t.Skip("Requires kdm.SupportMatrix setup - will test via integration tests")
}

// ── githubToken ───────────────────────────────────────────────────────────────

func TestGithubTokenK8s(t *testing.T) {
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
			ghToken:   "gh-123",
			ghubToken: "github-456",
			want:      "gh-123",
		},
		{
			name:      "falls back to GITHUB_TOKEN",
			ghToken:   "",
			ghubToken: "github-456",
			want:      "github-456",
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

// ── fetchK3sReleases with HTTP mocking ────────────────────────────────────────

func TestFetchK3sReleases(t *testing.T) {
	t.Run("fetches releases from GitHub API", func(t *testing.T) {
		// Create mock GitHub API server
		page1 := []ghRelease{
			{TagName: "v1.28.10+k3s1", Prerelease: false, Draft: false},
			{TagName: "v1.28.9+k3s2", Prerelease: false, Draft: false},
			{TagName: "v1.28.9+k3s1", Prerelease: false, Draft: false},
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check for auth header if token is set
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page1)
		}))
		defer server.Close()

		// This test requires refactoring fetchK3sReleases to accept a base URL
		t.Skip("Requires refactoring fetchK3sReleases to accept URL parameter")
	})

	t.Run("uses cache when fresh", func(t *testing.T) {
		// Create a temp cache file
		tmpDir := t.TempDir()
		oldCacheDir := os.Getenv("HOME")
		defer os.Setenv("HOME", oldCacheDir)
		os.Setenv("HOME", tmpDir)

		// Create cache directory and file
		cacheDir := filepath.Join(tmpDir, ".cache", "rancher-deployer")
		os.MkdirAll(cacheDir, 0755)

		cache := releasesCache{
			FetchedAt: time.Now(),
			Releases: []ghRelease{
				{TagName: "v1.28.10+k3s1", Prerelease: false, Draft: false},
			},
		}
		data, _ := json.Marshal(cache)
		os.WriteFile(filepath.Join(cacheDir, cacheFileName), data, 0644)

		// This test requires refactoring to inject cache path
		t.Skip("Requires refactoring to test cache behavior")
	})

	t.Run("handles pagination", func(t *testing.T) {
		t.Skip("Requires HTTP server mocking")
	})

	t.Run("stops pagination early when old versions reached", func(t *testing.T) {
		t.Skip("Requires HTTP server mocking")
	})
}

// ── ResolveClusterVersion ─────────────────────────────────────────────────────

func TestResolveClusterVersion(t *testing.T) {
	// This function calls fetchK3sReleases which does real HTTP calls
	// For unit testing, we'd need to refactor to inject the release fetcher
	t.Run("placeholder for integration test", func(t *testing.T) {
		t.Skip("Requires refactoring to inject fetchK3sReleases or HTTP mocking")
	})
}

// ── Test helpers ──────────────────────────────────────────────────────────────

func sign(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}

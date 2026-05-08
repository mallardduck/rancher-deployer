package rancher

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

// ── ResolveCertManagerVersion with HTTP mocking ───────────────────────────────

func TestResolveCertManagerVersion(t *testing.T) {
	t.Run("fetches latest version from GitHub API", func(t *testing.T) {
		// Create a mock server that returns a valid GitHub release
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Error("Should work without auth token")
			}
			release := struct {
				TagName string `json:"tag_name"`
			}{
				TagName: "v1.15.0",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(release)
		}))
		defer server.Close()

		// Temporarily override the GitHub API URL for testing
		// We'll need to modify the function to accept a URL parameter or use a global
		// For now, this shows the pattern
		t.Skip("Requires refactoring ResolveCertManagerVersion to accept URL parameter")
	})

	t.Run("falls back on HTTP error", func(t *testing.T) {
		// Create a mock server that returns an error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		t.Skip("Requires refactoring ResolveCertManagerVersion to accept URL parameter")
	})

	t.Run("falls back on timeout", func(t *testing.T) {
		// Create a mock server that hangs
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Sleep longer than the timeout
			// This will cause client timeout
		}))
		defer server.Close()

		t.Skip("Requires refactoring ResolveCertManagerVersion to accept URL parameter")
	})

	t.Run("uses GitHub token when available", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token-123" {
				t.Errorf("Expected Bearer token, got: %s", auth)
			}
			release := struct {
				TagName string `json:"tag_name"`
			}{
				TagName: "v1.15.0",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(release)
		}))
		defer server.Close()

		t.Skip("Requires refactoring ResolveCertManagerVersion to accept URL parameter")
	})
}

// ── githubGet testing ─────────────────────────────────────────────────────────

func TestGithubGet(t *testing.T) {
	t.Run("makes unauthenticated request when no token", func(t *testing.T) {
		// Save and clear env vars
		oldGH := os.Getenv("GH_TOKEN")
		oldGitHub := os.Getenv("GITHUB_TOKEN")
		os.Unsetenv("GH_TOKEN")
		os.Unsetenv("GITHUB_TOKEN")
		defer func() {
			os.Setenv("GH_TOKEN", oldGH)
			os.Setenv("GITHUB_TOKEN", oldGitHub)
		}()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Error("Should not send auth header when no token set")
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &http.Client{}
		resp, err := githubGet(client, server.URL)
		if err != nil {
			t.Fatalf("githubGet failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("sends auth header when token available", func(t *testing.T) {
		os.Setenv("GH_TOKEN", "test-token-456")
		defer os.Unsetenv("GH_TOKEN")

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token-456" {
				t.Errorf("Expected Bearer test-token-456, got: %s", auth)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		client := &http.Client{}
		resp, err := githubGet(client, server.URL)
		if err != nil {
			t.Fatalf("githubGet failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("returns error for invalid URL", func(t *testing.T) {
		client := &http.Client{}
		_, err := githubGet(client, "://invalid-url")
		if err == nil {
			t.Error("githubGet should error on invalid URL")
		}
	})
}

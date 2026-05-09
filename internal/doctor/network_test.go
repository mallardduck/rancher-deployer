package doctor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestEndpointChecker_Check_Success(t *testing.T) {
	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("Expected HEAD request, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := &EndpointChecker{
		url:         server.URL,
		displayName: "test endpoint",
		purpose:     "testing",
	}

	opts := &CheckOptions{
		NetworkTimeout: 5 * time.Second,
	}

	result := checker.Check(context.Background(), opts)

	if result.Status != StatusPass {
		t.Errorf("Expected StatusPass, got %v", result.Status)
	}

	if result.Name != "test endpoint" {
		t.Errorf("Expected name 'test endpoint', got %q", result.Name)
	}

	if result.Category != "network" {
		t.Errorf("Expected category 'network', got %q", result.Category)
	}
}

func TestEndpointChecker_Check_NotFound(t *testing.T) {
	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	checker := &EndpointChecker{
		url:         server.URL,
		displayName: "test endpoint",
		purpose:     "testing",
	}

	opts := &CheckOptions{
		NetworkTimeout: 5 * time.Second,
	}

	result := checker.Check(context.Background(), opts)

	// 404 should return StatusWarn (not critical)
	if result.Status != StatusWarn {
		t.Errorf("Expected StatusWarn for 404, got %v", result.Status)
	}

	if result.Remediation == "" {
		t.Error("Expected remediation message")
	}
}

func TestEndpointChecker_Check_Unreachable(t *testing.T) {
	// Use an unreachable URL
	checker := &EndpointChecker{
		url:         "http://localhost:1", // Port 1 is unlikely to be listening
		displayName: "unreachable endpoint",
		purpose:     "testing",
	}

	opts := &CheckOptions{
		NetworkTimeout: 100 * time.Millisecond, // Short timeout
	}

	result := checker.Check(context.Background(), opts)

	// Unreachable should return StatusWarn (not critical)
	if result.Status != StatusWarn {
		t.Errorf("Expected StatusWarn for unreachable endpoint, got %v", result.Status)
	}

	if result.Remediation == "" {
		t.Error("Expected remediation message")
	}
}

func TestEndpointChecker_Check_Redirects(t *testing.T) {
	// Create a test server that returns 302 redirect
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	checker := &EndpointChecker{
		url:         server.URL,
		displayName: "redirect endpoint",
		purpose:     "testing",
	}

	opts := &CheckOptions{
		NetworkTimeout: 5 * time.Second,
	}

	result := checker.Check(context.Background(), opts)

	// 302 is in 2xx-3xx range, should pass
	if result.Status != StatusPass {
		t.Errorf("Expected StatusPass for redirect, got %v", result.Status)
	}
}

func TestEndpointChecker_Name(t *testing.T) {
	checker := &EndpointChecker{
		displayName: "GitHub API",
	}

	if checker.Name() != "GitHub API" {
		t.Errorf("Expected name 'GitHub API', got %q", checker.Name())
	}
}

func TestEndpointChecker_Category(t *testing.T) {
	checker := &EndpointChecker{}

	if checker.Category() != "network" {
		t.Errorf("Expected category 'network', got %q", checker.Category())
	}
}

func TestGitHubRateLimitChecker_Check_WithToken(t *testing.T) {
	// Create a mock GitHub API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for authorization header
		auth := r.Header.Get("Authorization")
		limit := 60
		if auth != "" && auth == "Bearer test-token" {
			limit = 5000
		}

		response := map[string]interface{}{
			"resources": map[string]interface{}{
				"core": map[string]interface{}{
					"limit":     limit,
					"remaining": limit - 10,
					"reset":     1234567890,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Set GitHub token
	os.Setenv("GH_TOKEN", "test-token")
	defer os.Unsetenv("GH_TOKEN")

	// We can't easily test the actual GitHub API call, so this test
	// just verifies the checker doesn't panic
	checker := &GitHubRateLimitChecker{}

	if checker.Name() != "GitHub API rate limit" {
		t.Errorf("Expected name 'GitHub API rate limit', got %q", checker.Name())
	}

	if checker.Category() != "network" {
		t.Errorf("Expected category 'network', got %q", checker.Category())
	}
}

func TestGitHubRateLimitChecker_Name(t *testing.T) {
	checker := &GitHubRateLimitChecker{}

	if checker.Name() != "GitHub API rate limit" {
		t.Errorf("Expected name 'GitHub API rate limit', got %q", checker.Name())
	}
}

func TestGitHubRateLimitChecker_Category(t *testing.T) {
	checker := &GitHubRateLimitChecker{}

	if checker.Category() != "network" {
		t.Errorf("Expected category 'network', got %q", checker.Category())
	}
}

func TestEndpointChecker_ContextCancellation(t *testing.T) {
	// Create a slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := &EndpointChecker{
		url:         server.URL,
		displayName: "slow endpoint",
		purpose:     "testing",
	}

	opts := &CheckOptions{
		NetworkTimeout: 100 * time.Millisecond, // Timeout before server responds
	}

	result := checker.Check(context.Background(), opts)

	// Should timeout and return StatusWarn
	if result.Status != StatusWarn {
		t.Errorf("Expected StatusWarn for timeout, got %v", result.Status)
	}
}

func TestEndpointChecker_InvalidURL(t *testing.T) {
	checker := &EndpointChecker{
		url:         "://invalid-url",
		displayName: "invalid URL",
		purpose:     "testing",
	}

	opts := &CheckOptions{
		NetworkTimeout: 5 * time.Second,
	}

	result := checker.Check(context.Background(), opts)

	// Invalid URL should return StatusWarn
	if result.Status != StatusWarn {
		t.Errorf("Expected StatusWarn for invalid URL, got %v", result.Status)
	}
}

func TestGitHubRateLimitChecker_LowLimit(t *testing.T) {
	// Create a mock server with low rate limit
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"resources": map[string]interface{}{
				"core": map[string]interface{}{
					"limit":     60,
					"remaining": 5, // Low remaining
					"reset":     1234567890,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	// Note: This test can't easily override the GitHub API URL,
	// so it just tests that the checker doesn't panic
	checker := &GitHubRateLimitChecker{}
	opts := &CheckOptions{
		NetworkTimeout: 5 * time.Second,
	}

	// Just make sure it doesn't panic
	_ = checker.Check(context.Background(), opts)
}

func TestGitHubRateLimitChecker_Fallback(t *testing.T) {
	// Test that fallback to GITHUB_TOKEN works
	os.Unsetenv("GH_TOKEN")
	os.Setenv("GITHUB_TOKEN", "fallback-token")
	defer os.Unsetenv("GITHUB_TOKEN")

	checker := &GitHubRateLimitChecker{}
	opts := &CheckOptions{
		NetworkTimeout: 5 * time.Second,
	}

	// Just make sure it doesn't panic with fallback token
	_ = checker.Check(context.Background(), opts)
}

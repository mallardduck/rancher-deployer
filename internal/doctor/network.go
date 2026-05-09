package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// EndpointChecker validates network connectivity to required endpoints.
type EndpointChecker struct {
	url         string
	displayName string
	purpose     string // What this endpoint is used for
}

func (c *EndpointChecker) Name() string {
	return c.displayName
}

func (c *EndpointChecker) Category() string {
	return "network"
}

func (c *EndpointChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	client := &http.Client{
		Timeout: opts.NetworkTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "HEAD", c.url, nil)
	if err != nil {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     fmt.Sprintf("invalid URL: %v", err),
			Remediation: "This may indicate a configuration issue",
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     fmt.Sprintf("unreachable: %v", err),
			Remediation: fmt.Sprintf("Check network connectivity and firewall settings - %s", c.purpose),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusPass,
			Message:  fmt.Sprintf("accessible (HTTP %d)", resp.StatusCode),
		}
	}

	return CheckResult{
		Name:        c.Name(),
		Category:    c.Category(),
		Status:      StatusWarn,
		Message:     fmt.Sprintf("returned HTTP %d", resp.StatusCode),
		Remediation: fmt.Sprintf("Endpoint may be temporarily unavailable - %s", c.purpose),
	}
}

// GitHubRateLimitChecker checks GitHub API rate limit status.
type GitHubRateLimitChecker struct{}

func (c *GitHubRateLimitChecker) Name() string {
	return "GitHub API rate limit"
}

func (c *GitHubRateLimitChecker) Category() string {
	return "network"
}

func (c *GitHubRateLimitChecker) Check(ctx context.Context, opts *CheckOptions) CheckResult {
	client := &http.Client{
		Timeout: opts.NetworkTimeout,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/rate_limit", nil)
	if err != nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "could not create request",
		}
	}

	// Add GitHub token if available
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     "GitHub API unreachable",
			Remediation: "Check network connectivity - GitHub API is used for version resolution",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  fmt.Sprintf("GitHub API returned HTTP %d", resp.StatusCode),
		}
	}

	// Parse rate limit response
	var rateLimitResp struct {
		Resources struct {
			Core struct {
				Limit     int `json:"limit"`
				Remaining int `json:"remaining"`
				Reset     int `json:"reset"`
			} `json:"core"`
		} `json:"resources"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rateLimitResp); err != nil {
		return CheckResult{
			Name:     c.Name(),
			Category: c.Category(),
			Status:   StatusWarn,
			Message:  "could not parse rate limit response",
		}
	}

	limit := rateLimitResp.Resources.Core.Limit
	remaining := rateLimitResp.Resources.Core.Remaining

	// Determine if authenticated
	authenticated := limit > 60

	if remaining < 10 {
		status := StatusWarn
		msg := fmt.Sprintf("%d/%d requests remaining (nearly exhausted)", remaining, limit)
		var remediation string

		if !authenticated {
			remediation = "Set GH_TOKEN or GITHUB_TOKEN environment variable to increase limit to 5000/hour"
		} else {
			remediation = "Wait for rate limit to reset, or use a different GitHub token"
		}

		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      status,
			Message:     msg,
			Remediation: remediation,
		}
	}

	authStatus := "unauthenticated"
	if authenticated {
		authStatus = "authenticated"
	}

	msg := fmt.Sprintf("%d/%d requests remaining (%s)", remaining, limit, authStatus)

	// Warn if unauthenticated (but not critical)
	if !authenticated {
		return CheckResult{
			Name:        c.Name(),
			Category:    c.Category(),
			Status:      StatusWarn,
			Message:     msg,
			Remediation: "Set GH_TOKEN or GITHUB_TOKEN to increase rate limit from 60 to 5000 requests/hour",
		}
	}

	return CheckResult{
		Name:     c.Name(),
		Category: c.Category(),
		Status:   StatusPass,
		Message:  msg,
	}
}

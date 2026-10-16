package cli

import (
	"fmt"

	"github.com/mallardduck/rancher-deployer/internal/doctor"
)

// DisplayDoctorResults prints doctor results grouped by category.
func DisplayDoctorResults(results []doctor.CheckResult) {
	categories := []struct {
		key   string
		title string
	}{
		{"dependencies", "Binary Dependencies"},
		{"environment", "Runtime Environment"},
		{"configuration", "Configuration"},
		{"network", "Network Connectivity"},
		{"state", "Installation State"},
	}

	for _, cat := range categories {
		catResults := filterDoctorResults(results, cat.key)
		if len(catResults) == 0 {
			continue
		}

		fmt.Println()
		fmt.Printf("%s%s%s%s\n", ColorBold, ColorCyan, cat.title, ColorReset)

		for _, r := range catResults {
			displayDoctorResult(r)
		}
	}
}

func displayDoctorResult(r doctor.CheckResult) {
	switch r.Status {
	case doctor.StatusPass:
		fmt.Printf("  %s✔%s %s: %s\n", ColorGreen, ColorReset, r.Name, r.Message)
	case doctor.StatusWarn:
		fmt.Printf("  %s⚠%s %s: %s\n", ColorYellow, ColorReset, r.Name, r.Message)
		if r.Remediation != "" {
			fmt.Printf("    %s→%s %s\n", ColorYellow, ColorReset, r.Remediation)
		}
	case doctor.StatusFail:
		fmt.Printf("  %s✗%s %s: %s\n", ColorRed, ColorReset, r.Name, r.Message)
		if r.Remediation != "" {
			fmt.Printf("    %s→%s %s\n", ColorRed, ColorReset, r.Remediation)
		}
	}
}

func filterDoctorResults(results []doctor.CheckResult, category string) []doctor.CheckResult {
	filtered := make([]doctor.CheckResult, 0)
	for _, r := range results {
		if r.Category == category {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// CountDoctorResults tallies results by status.
func CountDoctorResults(results []doctor.CheckResult) (passed, warned, failed int) {
	for _, r := range results {
		switch r.Status {
		case doctor.StatusPass:
			passed++
		case doctor.StatusWarn:
			warned++
		case doctor.StatusFail:
			failed++
		}
	}
	return
}

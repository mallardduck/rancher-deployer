package deploy

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/detect"
	"github.com/mallardduck/rancher-deployer/internal/doctor"
)

type doctorFlags struct {
	mode           string
	skipNetwork    bool
	skipState      bool
	networkTimeout time.Duration
}

func newDoctorCmd() *cobra.Command {
	flags := &doctorFlags{}

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system health and validate dependencies",
		Long: `Validates that your system meets all requirements for rancher-deployer.

This command checks:
- Required and optional binary dependencies
- Runtime environment compatibility (OS, container runtime)
- Configuration files and environment variables
- Network connectivity to required endpoints
- Current installation state (if applicable)

Exit codes:
  0 - All checks passed (warnings are OK)
  1 - One or more critical checks failed`,
		Example: `  # Run all checks
  rancher-deployer doctor

  # Skip network checks (for offline environments)
  rancher-deployer doctor --skip-network

  # Force specific mode
  rancher-deployer doctor --mode k3d

  # Skip state checks
  rancher-deployer doctor --skip-state`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(flags)
		},
	}

	cmd.Flags().StringVar(&flags.mode, "mode", "", "Deployment mode: k3s or k3d (auto-detected if not specified)")
	cmd.Flags().BoolVar(&flags.skipNetwork, "skip-network", false, "Skip network connectivity checks")
	cmd.Flags().BoolVar(&flags.skipState, "skip-state", false, "Skip existing installation state checks")
	cmd.Flags().DurationVar(&flags.networkTimeout, "network-timeout", 10*time.Second, "Timeout for network checks")

	return cmd
}

func runDoctor(flags *doctorFlags) error {
	printBanner()

	// Prepare options
	opts := &doctor.CheckOptions{
		Mode:           flags.mode,
		Context:        doctor.ContextLocal,
		SkipNetwork:    flags.skipNetwork,
		SkipState:      flags.skipState,
		NetworkTimeout: flags.networkTimeout,
	}

	// Auto-detect and display mode
	mode := flags.mode
	if mode == "" {
		mode, _ = detect.InstallMode()
	}
	fmt.Printf("Detected mode: %s\n\n", mode)

	// Create doctor with mode-appropriate checkers
	d := doctor.NewDoctor(opts)

	// Run all checks
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	results := d.RunAll(ctx)

	// Group and display results by category
	displayResults(results)

	// Summary
	passed, warned, failed := countResults(results)
	fmt.Println()
	printInfo("Summary: %d passed, %d warnings, %d failed", passed, warned, failed)

	// Final message and exit
	fmt.Println()
	if doctor.HasCriticalFailures(results) {
		printWarning("Critical checks failed. Please address the errors above before deploying.")
		// Return error to exit with code 1 (but avoid os.Exit to allow defers to run)
		return fmt.Errorf("health check failed")
	}

	if warned > 0 {
		printInfo("Your system has some warnings but is ready to deploy.")
	} else {
		printSuccess("Your system is ready to deploy Rancher!")
	}

	return nil
}

func displayResults(results []doctor.CheckResult) {
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
		catResults := filterByCategory(results, cat.key)
		if len(catResults) == 0 {
			continue
		}

		fmt.Println()
		fmt.Printf("%s%s%s%s\n", colorBold, colorCyan, cat.title, colorReset)

		for _, r := range catResults {
			displayResult(r)
		}
	}
}

func displayResult(r doctor.CheckResult) {
	switch r.Status {
	case doctor.StatusPass:
		fmt.Printf("  %s✔%s %s: %s\n", colorGreen, colorReset, r.Name, r.Message)
	case doctor.StatusWarn:
		fmt.Printf("  %s⚠%s %s: %s\n", colorYellow, colorReset, r.Name, r.Message)
		if r.Remediation != "" {
			fmt.Printf("    %s→%s %s\n", colorYellow, colorReset, r.Remediation)
		}
	case doctor.StatusFail:
		fmt.Printf("  %s✗%s %s: %s\n", colorRed, colorReset, r.Name, r.Message)
		if r.Remediation != "" {
			fmt.Printf("    %s→%s %s\n", colorRed, colorReset, r.Remediation)
		}
	}
}

func filterByCategory(results []doctor.CheckResult, category string) []doctor.CheckResult {
	filtered := make([]doctor.CheckResult, 0)
	for _, r := range results {
		if r.Category == category {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func countResults(results []doctor.CheckResult) (passed, warned, failed int) {
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

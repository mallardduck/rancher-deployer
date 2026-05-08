package deploy

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/kdm"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

type upgradeFlags struct {
	rancherVersion string
	namespace      string
	prime          bool
	channel        string
	valuesFile     string
	helmSet        []string
	dryRun         bool
	yes            bool
	force          bool
}

func newUpgradeCmd() *cobra.Command {
	f := &upgradeFlags{}

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade an existing Rancher installation",
		Example: `  # Upgrade to a new patch release
  rancher-deployer upgrade --rancher-version 2.8.6

  # Upgrade to the next minor
  rancher-deployer upgrade --rancher-version 2.9.0

  # Dry run — validate and print plan without executing
  rancher-deployer upgrade --rancher-version 2.9.0 --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(f)
		},
	}

	cmd.Flags().StringVar(&f.rancherVersion, "rancher-version", "", "Target Rancher version to upgrade to (required)")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Namespace where Rancher is installed")
	cmd.Flags().BoolVar(&f.prime, "prime", false, "Use Rancher Prime chart")
	cmd.Flags().StringVar(&f.channel, "channel", "stable", "Release channel: stable (GA), latest (RC), alpha")
	cmd.Flags().StringVar(&f.valuesFile, "values-file", "", "Path to YAML file with additional Helm values")
	cmd.Flags().StringArrayVar(&f.helmSet, "set", nil, "Override Helm chart value (repeatable): --set key=value")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print resolved plan without executing")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&f.force, "force", false, "Skip upgrade-path and k8s-compatibility checks (useful for pre-release testing)")

	_ = cmd.MarkFlagRequired("rancher-version")

	return cmd
}

func runUpgrade(f *upgradeFlags) error {
	f.rancherVersion = strings.TrimPrefix(f.rancherVersion, "v")
	isPrerelease := k8sresolver.IsPrerelease(f.rancherVersion)

	fmt.Println()
	printBanner()

	// ── Step 1: Detect installed version ─────────────────────────────────────
	printStep(1, "Detecting installed Rancher version")
	currentVersion, err := rancher.InstalledVersion(f.namespace)
	if err != nil {
		return err
	}
	printInfo("Installed: v%s", currentVersion)

	// ── Step 2: Validate upgrade path ─────────────────────────────────────────
	printStep(2, "Validating upgrade path")
	if f.force {
		printWarning("--force set: skipping upgrade-path validation")
	} else if err := validateUpgradePath(currentVersion, f.rancherVersion); err != nil {
		return err
	} else {
		printInfo("v%s → v%s is a valid upgrade path", currentVersion, f.rancherVersion)
	}

	// ── Step 3: Fetch support matrix for target version ───────────────────────
	printStep(3, "Fetching support matrix for Rancher v"+f.rancherVersion)
	clusterK8s, err := rancher.ClusterK8sVersion()
	if err != nil {
		return err
	}
	printInfo("Cluster k8s: %s", clusterK8s)

	matrix, err := kdm.FetchSupportMatrix(f.rancherVersion)
	if err != nil {
		if !f.force {
			return fmt.Errorf("support matrix lookup failed: %w", err)
		}
		printWarning("support matrix unavailable (%v) — skipping k8s compatibility check", err)
	} else {
		printInfo("Rancher v%s supports k8s: %s",
			f.rancherVersion, strings.Join(matrix.SupportedMinors(), ", "))

		// ── Step 4: Check current k8s version compatibility ───────────────────
		printStep(4, "Checking cluster k8s compatibility")
		if _, err := matrix.LatestPatchFor(clusterK8s); err != nil {
			if !f.force {
				return fmt.Errorf(
					"cluster k8s version %s is not supported by Rancher v%s\n"+
						"  You must upgrade k8s before upgrading Rancher, or choose a compatible Rancher version\n"+
						"  Supported minors: %s",
					clusterK8s, f.rancherVersion, strings.Join(matrix.SupportedMinors(), ", "),
				)
			}
			printWarning("k8s %s not in support matrix for v%s — continuing anyway (--force)", clusterK8s, f.rancherVersion)
		} else {
			printInfo("Cluster k8s %s is compatible with Rancher v%s", clusterK8s, f.rancherVersion)
		}
	}

	// ── Step 5: Resolve chart ─────────────────────────────────────────────────
	printStep(5, "Resolving Helm chart")
	channel, err := rancher.NormaliseChannel(f.channel)
	if err != nil {
		return err
	}
	chartRef := rancher.ChartRef(f.prime, isPrerelease, channel, f.rancherVersion)
	printInfo("Chart: %s", chartRef.String())

	// ── Step 6: Build Helm values (no bootstrapPassword on upgrade) ───────────
	printStep(6, "Building Helm values")
	helmValues := rancher.HelmValues{
		ValuesFile: f.valuesFile,
		SetFlags:   f.helmSet,
	}
	if f.valuesFile != "" {
		printInfo("  --values %s", f.valuesFile)
	}
	for _, s := range f.helmSet {
		printInfo("  --set %s", s)
	}

	// ── Print plan ────────────────────────────────────────────────────────────
	fmt.Println()
	printUpgradePlan(f, currentVersion, clusterK8s, chartRef)

	if f.dryRun {
		fmt.Println()
		printWarning("Dry run — no changes made.")
		return nil
	}

	// ── Confirm ───────────────────────────────────────────────────────────────
	if !f.yes && !promptConfirm("Proceed with upgrade?") {
		fmt.Println("Aborted.")
		return nil
	}

	// ── Step 7: Ensure Helm repo ─────────────────────────────────────────────
	printStep(7, "Configuring Helm repo")
	if err := rancher.EnsureHelmRepo(chartRef.RepoName, chartRef.RepoURL, f.yes); err != nil {
		return err
	}

	// ── Step 8: Upgrade Rancher ───────────────────────────────────────────────
	printStep(8, "Upgrading Rancher via Helm")
	if err := rancher.Upgrade(f.namespace, chartRef, helmValues); err != nil {
		return err
	}

	// ── Step 9: Wait for rollout ──────────────────────────────────────────────
	printStep(9, "Waiting for Rancher to become ready")
	if err := rancher.WaitReady(f.namespace); err != nil {
		return err
	}

	fmt.Println()
	printSuccess("Rancher upgraded to v%s successfully!", f.rancherVersion)
	fmt.Println()

	return nil
}

// validateUpgradePath enforces Rancher's upgrade rules:
//   - Downgrades are not supported
//   - Minor version skips are not supported (must upgrade one minor at a time)
//   - Patch-level upgrades within the same minor are always valid
func validateUpgradePath(current, target string) error {
	cv := parseMinorParts(current)
	tv := parseMinorParts(target)

	if cv[0] != tv[0] {
		return fmt.Errorf(
			"cross-major upgrades are not supported (v%s → v%s)",
			current, target,
		)
	}

	minorDelta := tv[1] - cv[1]
	switch {
	case minorDelta < 0:
		return fmt.Errorf(
			"downgrade not supported: v%s is older than installed v%s",
			target, current,
		)
	case minorDelta > 1:
		return fmt.Errorf(
			"cannot skip minor versions: v%s → v%s skips %d minor release(s)\n"+
				"  Upgrade to v%d.%d.x first",
			current, target, minorDelta-1, cv[0], cv[1]+1,
		)
	}

	// Same minor: target patch must be >= current patch
	if minorDelta == 0 {
		cp := parsePatchPart(current)
		tp := parsePatchPart(target)
		if tp < cp {
			return fmt.Errorf(
				"downgrade not supported: v%s is older than installed v%s",
				target, current,
			)
		}
	}

	return nil
}

// parseMinorParts returns [major, minor] as ints from a version string.
func parseMinorParts(v string) [2]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	var out [2]int
	for i := 0; i < 2 && i < len(parts); i++ {
		_, _ = fmt.Sscanf(parts[i], "%d", &out[i])
	}
	return out
}

// parsePatchPart returns the patch integer from a version string.
func parsePatchPart(v string) int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	if len(parts) < 3 {
		return 0
	}
	var patch int
	_, _ = fmt.Sscanf(parts[2], "%d", &patch)
	return patch
}

func printUpgradePlan(f *upgradeFlags, currentVersion, clusterK8s string, chart rancher.Chart) {
	edition := "Community"
	if f.prime {
		edition = "Prime"
	}
	edition += " " + f.channel
	fmt.Printf("%s━━ Upgrade Plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
	fmt.Printf("  From             : v%s\n", currentVersion)
	fmt.Printf("  To               : v%s (%s)\n", f.rancherVersion, edition)
	fmt.Printf("  Cluster k8s      : %s\n", clusterK8s)
	fmt.Printf("  Helm chart       : %s\n", chart.String())
	fmt.Printf("  Namespace        : %s\n", f.namespace)
	if f.force {
		fmt.Printf("  Checks           : FORCED (path + k8s compat skipped)\n")
	}
	if f.dryRun {
		fmt.Printf("  Mode             : DRY RUN\n")
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
}

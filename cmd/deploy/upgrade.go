package deploy

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/kdm"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
	"github.com/mallardduck/rancher-deployer/internal/upgrade"
)

type upgradeFlags struct {
	rancherVersion string
	namespace      string
	channel        string
	commit         string // head channel only — pin to a specific head build
	valuesFile     string
	helmSet        []string
	failurePolicy  string
	prime          bool
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
  rancher-deployer upgrade --rancher-version 2.9.0 --dry-run

  # Upgrade to a specific reported head build
  rancher-deployer upgrade --channel head --rancher-version 2.15 --commit b03c4de`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(f)
		},
	}

	cmd.Flags().StringVar(&f.rancherVersion, "rancher-version", "", "Target Rancher version to upgrade to, e.g. 2.8.6. A bare minor (e.g. 2.9) auto-resolves the newest patch (or newest head build for --channel head). Required, unless using --prime --channel head --commit")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Namespace where Rancher is installed")
	cmd.Flags().BoolVar(&f.prime, "prime", false, "Use Rancher Prime chart")
	cmd.Flags().StringVar(&f.channel, "channel", "stable", "Release channel: stable (GA), latest (RC), alpha, head (continuously-published head builds — requires a minor, e.g. --rancher-version 2.15)")
	cmd.Flags().StringVar(&f.commit, "commit", "", "Pin --channel head to the head build whose commit starts with this (instead of the newest one) — e.g. to reproduce a bug reported against a specific build")
	cmd.Flags().StringVar(&f.valuesFile, "values-file", "", "Path to YAML file with additional Helm values")
	cmd.Flags().StringArrayVar(&f.helmSet, "set", nil, "Override Helm chart value (repeatable): --set key=value")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print resolved plan without executing")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&f.force, "force", false, "Skip upgrade-path and k8s-compatibility checks (useful for pre-release testing)")
	cmd.Flags().StringVar(&f.failurePolicy, "failure-policy", rancher.FailurePolicyAbort, "What to do if the upgrade fails: abort (leave it for inspection) or reinstall (roll back automatically)")

	return cmd
}

func runUpgrade(f *upgradeFlags) error {
	f.rancherVersion = strings.TrimPrefix(f.rancherVersion, "v")

	channel, err := rancher.NormaliseChannel(f.channel)
	if err != nil {
		return err
	}

	failurePolicy, err := rancher.ParseFailurePolicy(f.failurePolicy)
	if err != nil {
		return err
	}

	// --rancher-version is required, except for the one case where a commit
	// alone is enough to resolve a chart: --prime --channel head --commit.
	if f.rancherVersion == "" && (channel != rancher.ChannelHead || !f.prime || f.commit == "") {
		return fmt.Errorf("--rancher-version is required (unless using --prime --channel head --commit)")
	}

	fmt.Println()
	printBanner()

	// ── Step 1: Detect installed version ─────────────────────────────────────
	printStep(1, "Detecting installed Rancher version")
	currentVersion, err := rancher.InstalledVersion(f.namespace)
	if err != nil {
		return err
	}
	printInfo("Installed: v%s", currentVersion)

	// ── Step 2: Resolve Helm chart ────────────────────────────────────────────
	// Resolved before upgrade-path validation and the support matrix because
	// commit-only resolution (--prime --channel head --commit, no
	// --rancher-version) doesn't know its minor until the chart is resolved.
	printStep(2, "Resolving Helm chart")
	chartRef, err := resolveChart(f.prime, channel, f.rancherVersion, f.commit)
	if err != nil {
		return err
	}
	printInfo("Chart: %s", chartRef.String())

	// ── Step 3: Validate upgrade path ─────────────────────────────────────────
	printStep(3, "Validating upgrade path")
	if f.force {
		printWarning("--force set: skipping upgrade-path validation")
	} else {
		if pathErr := upgrade.ValidatePath(currentVersion, chartRef.Version); pathErr != nil {
			return pathErr
		}
		printInfo("v%s → v%s is a valid upgrade path", currentVersion, chartRef.Version)
	}

	// ── Step 4: Fetch support matrix for target version ───────────────────────
	printStep(4, "Fetching support matrix for Rancher v"+chartRef.Version)
	clusterK8s, err := rancher.ClusterK8sVersion()
	if err != nil {
		return err
	}
	printInfo("Cluster k8s: %s", clusterK8s)

	kdmTarget := f.rancherVersion
	if kdmTarget == "" {
		// Commit-only resolution: derive just the minor from the resolved
		// chart version instead, so the KDM plan line shows a clean minor
		// rather than the full head build string. Safe because community
		// always requires an explicit minor and can never reach this branch
		// with an empty f.rancherVersion — see ResolveChart.
		kdmTarget = minorOf(chartRef.Version)
	}
	var matrix *kdm.SupportMatrix
	var kdmFlavor kdm.KDMFlavor
	var usedFallbackKDM bool
	if channel == rancher.ChannelHead {
		var result *kdm.SupportMatrixResult
		result, err = kdm.FetchSupportMatrixWithFallback(kdmTarget)
		if err == nil {
			matrix, kdmFlavor, usedFallbackKDM = result.Matrix, result.Flavor, result.UsedFallbackMinor
			if usedFallbackKDM {
				printWarning("No KDM data for Rancher %s yet — using the previous minor's support matrix as a best-effort approximation; k8s compatibility isn't guaranteed", kdmTarget)
			}
		}
	} else {
		matrix, err = kdm.FetchSupportMatrix(kdmTarget)
	}
	if err != nil {
		if !f.force {
			return fmt.Errorf("support matrix lookup failed: %w", err)
		}
		printWarning("support matrix unavailable (%v) — skipping k8s compatibility check", err)
	} else {
		printInfo("Rancher v%s supports k8s: %s",
			chartRef.Version, strings.Join(matrix.SupportedMinors(), ", "))

		// ── Step 5: Check current k8s version compatibility ───────────────────
		printStep(5, "Checking cluster k8s compatibility")
		if _, compatErr := matrix.LatestPatchFor(clusterK8s); compatErr != nil {
			if !f.force {
				return fmt.Errorf(
					"cluster k8s version %s is not supported by Rancher v%s\n"+
						"  You must upgrade k8s before upgrading Rancher, or choose a compatible Rancher version\n"+
						"  Supported minors: %s",
					clusterK8s, chartRef.Version, strings.Join(matrix.SupportedMinors(), ", "),
				)
			}
			printWarning("k8s %s not in support matrix for v%s — continuing anyway (--force)", clusterK8s, chartRef.Version)
		} else {
			printInfo("Cluster k8s %s is compatible with Rancher v%s", clusterK8s, chartRef.Version)
		}
	}

	// ── Step 6: Build Helm values (no bootstrapPassword on upgrade) ───────────
	printStep(6, "Building Helm values")
	helmValues := rancher.HelmValues{
		ValuesFile:    f.valuesFile,
		SetFlags:      f.helmSet,
		FailurePolicy: failurePolicy,
	}
	if f.valuesFile != "" {
		printInfo("  --values %s", f.valuesFile)
	}
	for _, s := range f.helmSet {
		printInfo("  --set %s", s)
	}

	// ── Print plan ────────────────────────────────────────────────────────────
	fmt.Println()
	printUpgradePlan(f, currentVersion, clusterK8s, kdmLine(matrix, kdmFlavor, usedFallbackKDM), chartRef)

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
	printSuccess("Rancher upgraded to v%s successfully!", chartRef.Version)
	fmt.Println()

	return nil
}

func printUpgradePlan(f *upgradeFlags, currentVersion, clusterK8s, kdmVer string, chart rancher.Chart) { //nolint:revive // 5 args are all distinct plan fields; a wrapper struct would add noise without clarity
	edition := "Community"
	if f.prime {
		edition = "Prime"
	}
	edition += " " + f.channel
	fmt.Printf("%s━━ Upgrade Plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
	fmt.Printf("  From             : v%s\n", currentVersion)
	fmt.Printf("  To               : %s\n", versionLine(f.rancherVersion, chart.Version, edition, f.commit))
	fmt.Printf("  KDM              : %s\n", kdmVer)
	fmt.Printf("  Cluster k8s      : %s\n", clusterK8s)
	fmt.Printf("  Helm chart       : %s\n", chart.String())
	fmt.Printf("  Namespace        : %s\n", f.namespace)
	fmt.Printf("  Failure policy   : %s\n", f.failurePolicy)
	if f.force {
		fmt.Printf("  Checks           : FORCED (path + k8s compat skipped)\n")
	}
	if f.dryRun {
		fmt.Printf("  Mode             : DRY RUN\n")
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
}

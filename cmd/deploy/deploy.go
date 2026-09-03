// Package deploy wires together the CLI commands and orchestrates the deployment workflow.
package deploy

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	clusterexisting "github.com/mallardduck/rancher-deployer/internal/clusters/existing"
	clusterk3d "github.com/mallardduck/rancher-deployer/internal/clusters/k3d"
	clusterk3s "github.com/mallardduck/rancher-deployer/internal/clusters/k3s"
	"github.com/mallardduck/rancher-deployer/internal/detect"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/kdm"
	"github.com/mallardduck/rancher-deployer/internal/provider"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

type deployFlags struct {
	rancherVersion    string
	k8sVersion        string
	channel           string
	commit            string // head channel only — pin to a specific head build
	mode              string // "", "k3s", "k3d"
	hostname          string
	namespace         string
	valuesFile        string
	clusterName       string // k3d only
	bootstrapPassword string
	helmSet           []string
	prime             bool
	dryRun            bool
	yes               bool // skip confirmation prompt
}

func newDeployCmd() *cobra.Command {
	f := &deployFlags{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Install k3s/k3d and deploy Rancher",
		Example: `  # Minimal — auto-selects k8s + k3s versions
  rancher-deployer deploy --rancher-version 2.8.5

  # Force k3d, target k8s 1.28, Rancher Prime
  rancher-deployer deploy --rancher-version 2.8.5 --mode k3d --k8s-version 1.28 --prime

  # With a values file and individual overrides
  rancher-deployer deploy --rancher-version 2.8.5 \
    --values-file ./values.yaml \
    --set replicas=1 \
    --set auditLog.level=1

  # Dry run — print resolved plan without executing
  rancher-deployer deploy --rancher-version 2.8.5 --dry-run

  # Reproduce a bug reported against a specific head build
  rancher-deployer deploy --channel head --rancher-version 2.15 --commit b03c4de`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(f)
		},
	}

	cmd.Flags().StringVar(&f.rancherVersion, "rancher-version", "", "Rancher version to install, e.g. 2.8.5. A bare minor (e.g. 2.8) auto-resolves the newest patch (or newest head build for --channel head). Required, unless using --prime --channel head --commit")
	cmd.Flags().StringVar(&f.k8sVersion, "k8s-version", "", "Target k8s major.minor, e.g. 1.28 (default: auto-select from support matrix)")
	cmd.Flags().BoolVar(&f.prime, "prime", false, "Use Rancher Prime instead of community edition")
	cmd.Flags().StringVar(&f.channel, "channel", "stable", "Release channel: stable (GA), latest (RC), alpha, head (continuously-published head builds — requires a minor, e.g. --rancher-version 2.15)")
	cmd.Flags().StringVar(&f.commit, "commit", "", "Pin --channel head to the head build whose commit starts with this (instead of the newest one) — e.g. to reproduce a bug reported against a specific build")
	cmd.Flags().StringVar(&f.mode, "mode", "", "Force install mode: k3s or k3d (default: auto-detect)")
	cmd.Flags().StringVar(&f.hostname, "hostname", "", "Hostname for Rancher ingress (default: <node-ip>.sslip.io)")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Kubernetes namespace for Rancher")
	cmd.Flags().StringVar(&f.valuesFile, "values-file", "", "Path to YAML file with Helm chart values")
	cmd.Flags().StringArrayVar(&f.helmSet, "set", nil, "Set Helm chart value (repeatable): --set key=value")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print resolved plan and commands without executing")
	cmd.Flags().StringVar(&f.clusterName, "cluster-name", "rancher-local", "k3d cluster name (k3d mode only)")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt (for CI/non-interactive use)")
	cmd.Flags().StringVar(&f.bootstrapPassword, "bootstrap-password", "letsmein", "Initial admin password for Rancher")

	return cmd
}

func runDeploy(f *deployFlags) error {
	// Normalise version — strip leading 'v'
	f.rancherVersion = strings.TrimPrefix(f.rancherVersion, "v")

	channel, err := rancher.NormaliseChannel(f.channel)
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

	// ── Step 1: Detect install mode ─────────────────────────────────────────
	printStep(1, "Detecting install mode")
	mode, reason, err := detect.ResolveMode(f.mode)
	if err != nil {
		return err
	}
	if reason != "" {
		printInfo("Auto-detected: %s (%s)", mode, reason)
	} else {
		printInfo("Mode: %s", mode)
	}
	clusterProvider, err := buildProvider(mode, f.clusterName)
	if err != nil {
		return err
	}

	// ── Step 2: Resolve Helm chart details ──────────────────────────────────
	// Resolved before the support matrix because commit-only resolution
	// (--prime --channel head --commit, no --rancher-version) doesn't know
	// its minor until the chart is resolved.
	printStep(2, "Resolving Helm chart")
	chartRef, err := resolveChart(f.prime, channel, f.rancherVersion, f.commit)
	if err != nil {
		return err
	}
	printInfo("Chart: %s", chartRef.String())

	// ── Step 3: Resolve Rancher support matrix ───────────────────────────────
	printStep(3, "Fetching Rancher support matrix")
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
		result, resultErr := kdm.FetchSupportMatrixWithFallback(kdmTarget)
		if resultErr != nil {
			return fmt.Errorf("support matrix lookup failed: %w", resultErr)
		}
		matrix, kdmFlavor, usedFallbackKDM = result.Matrix, result.Flavor, result.UsedFallbackMinor
		if usedFallbackKDM {
			printWarning("No KDM data for Rancher %s yet — using the previous minor's support matrix as a best-effort approximation; k8s compatibility isn't guaranteed", kdmTarget)
		}
	} else {
		matrix, err = kdm.FetchSupportMatrix(kdmTarget)
		if err != nil {
			return fmt.Errorf("support matrix lookup failed: %w", err)
		}
	}
	printInfo("Rancher v%s supports k8s versions: %s",
		chartRef.Version, strings.Join(matrix.SupportedMinors(), ", "))

	// ── Step 4: Resolve k8s version ─────────────────────────────────────────
	printStep(4, "Resolving Kubernetes version")
	resolvedK8s, err := k8sresolver.ResolveK8s(f.k8sVersion, matrix)
	if err != nil {
		return err
	}
	printInfo("Target k8s version: %s", resolvedK8s)

	// ── Step 5: Resolve cluster version ──────────────────────────────────────
	if mode == "existing" {
		printStep(5, "Validating existing cluster")
	} else {
		printStep(5, "Resolving k3s/k3d version")
	}
	var clusterVersion string
	clusterVersion, err = clusterProvider.ResolveClusterVersion(context.Background(), resolvedK8s)
	if err != nil {
		return err
	}
	if mode != "existing" {
		printInfo("Cluster version: %s", clusterVersion)
	}

	// ── Step 6: Resolve cert-manager version ─────────────────────────────────
	printStep(6, "Resolving cert-manager version")
	certManagerVersion, err := rancher.ResolveCertManagerVersion()
	if err != nil {
		return err
	}
	printInfo("cert-manager version: %s", certManagerVersion)

	// ── Step 7: Build Helm values ────────────────────────────────────────────
	printStep(7, "Building Helm values")
	helmValues, err := rancher.BuildHelmValues(f.valuesFile, f.helmSet, f.hostname, f.namespace, f.bootstrapPassword)
	if err != nil {
		return err
	}
	if len(helmValues.SetFlags) > 0 {
		for _, s := range helmValues.SetFlags {
			printInfo("  --set %s", s)
		}
	}
	if helmValues.ValuesFile != "" {
		printInfo("  --values %s", helmValues.ValuesFile)
	}

	// ── Print plan ───────────────────────────────────────────────────────────
	fmt.Println()
	printPlan(f, mode, resolvedK8s, clusterVersion, certManagerVersion, kdmLine(matrix, kdmFlavor, usedFallbackKDM), chartRef, helmValues)

	if f.dryRun {
		fmt.Println()
		printWarning("Dry run — no changes made.")
		return nil
	}

	// ── Confirm ──────────────────────────────────────────────────────────────
	if !f.yes && !promptConfirm("Proceed with deployment?") {
		fmt.Println("Aborted.")
		return nil
	}

	// ── Step 8: Install cluster ───────────────────────────────────────────────
	printStep(8, "Installing cluster")
	if err := clusterProvider.Setup(context.Background(), provider.SetupOptions{ClusterVersion: clusterVersion}); err != nil {
		return err
	}

	// ── Step 9: Install cert-manager ─────────────────────────────────────────
	printStep(9, "Installing cert-manager")
	if err := clusterProvider.Helm().InstallCertManager(certManagerVersion); err != nil {
		return err
	}

	// ── Step 10: Ensure Helm repo ────────────────────────────────────────────
	printStep(10, "Configuring Helm repo")
	if err := clusterProvider.Helm().EnsureRepo(chartRef.RepoName, chartRef.RepoURL, f.yes); err != nil {
		return err
	}

	// ── Step 11: Deploy Rancher ───────────────────────────────────────────────
	printStep(11, "Deploying Rancher via Helm")
	if err := clusterProvider.Helm().Install(f.namespace, chartRef, helmValues); err != nil {
		return err
	}

	// ── Step 12: Wait for Rancher ────────────────────────────────────────────
	printStep(12, "Waiting for Rancher to become ready")
	if err := clusterProvider.Helm().WaitReady(f.namespace); err != nil {
		return err
	}

	fmt.Println()
	printSuccess("Rancher v%s deployed successfully!", chartRef.Version)
	printInfo("Access URL : https://%s", helmValues.Hostname)
	printInfo("Username   : admin")
	printInfo("Password   : %s", f.bootstrapPassword)
	fmt.Println()

	return nil
}

// buildProvider constructs the cluster provider for the resolved mode.
// clusterName is only meaningful for k3d; it is ignored by k3s and existing.
func buildProvider(mode, clusterName string) (provider.Provider, error) {
	switch mode {
	case "k3d":
		return clusterk3d.NewProvider(clusterName), nil
	case "k3s":
		return clusterk3s.NewProvider(), nil
	case "existing":
		return clusterexisting.NewProvider(""), nil
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
}

// minorOf extracts the "MAJOR.MINOR" prefix from a version string — used to
// derive a clean KDM lookup target from a fully-resolved chart version when
// no minor was given directly (commit-only resolution).
func minorOf(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// resolveChart dispatches to rancher.ResolveChart or
// rancher.ResolveHeadChartByCommit depending on whether a commit was given,
// and validates that --commit is only used with --channel head.
func resolveChart(prime bool, channel, versionInput, commit string) (rancher.Chart, error) {
	if commit == "" {
		return rancher.ResolveChart(prime, channel, versionInput)
	}
	if channel != rancher.ChannelHead {
		return rancher.Chart{}, fmt.Errorf("--commit only applies to --channel head")
	}
	return rancher.ResolveHeadChartByCommit(prime, versionInput, commit)
}

// versionLine formats the Rancher version line for a plan/upgrade summary,
// noting the original request alongside the resolved version whenever a bare
// minor (or head channel) caused them to differ — e.g.
// "v2.15.2-fbf2130-head (requested: 2.15, Prime head)". When no minor was
// requested at all (commit-only resolution), notes the commit instead — e.g.
// "v2.16.0-b03c4de-head (Prime head, commit: b03c4de)".
func versionLine(requested, resolved, edition, commit string) string {
	switch {
	case requested == "" && commit != "":
		return fmt.Sprintf("v%s (%s, commit: %s)", resolved, edition, commit)
	case requested == resolved:
		return fmt.Sprintf("v%s (%s)", resolved, edition)
	default:
		return fmt.Sprintf("v%s (requested: %s, %s)", resolved, requested, edition)
	}
}

// kdmLine formats the "which Rancher version's KDM data are we trusting"
// plan line — the data used to bridge the requested Rancher version and the
// resolved Kubernetes version. Notes which branch flavor it came from
// (release vs. dev — only shown when known, i.e. head-channel installs),
// flags when the head channel's minor-1 fallback was used, and handles the
// --force-skipped case (upgrade only, where matrix can be nil).
func kdmLine(matrix *kdm.SupportMatrix, flavor kdm.KDMFlavor, usedFallback bool) string {
	if matrix == nil {
		return "unavailable (--force)"
	}
	var notes []string
	if flavor != "" {
		notes = append(notes, string(flavor))
	}
	if usedFallback {
		notes = append(notes, "fallback — no KDM data published yet")
	}
	if len(notes) == 0 {
		return fmt.Sprintf("v%s", matrix.RancherVersion)
	}
	return fmt.Sprintf("v%s (%s)", matrix.RancherVersion, strings.Join(notes, ", "))
}

func printPlan(f *deployFlags, mode, k8sVer, clusterVer, certMgrVer, kdmVer string, chart rancher.Chart, hv rancher.HelmValues) { //nolint:revive // 8 args are all distinct plan fields; a wrapper struct would add noise without clarity
	edition := "Community"
	if f.prime {
		edition = "Prime"
	}
	edition += " " + f.channel
	fmt.Printf("%s━━ Deployment Plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
	fmt.Printf("  Rancher version  : %s\n", versionLine(f.rancherVersion, chart.Version, edition, f.commit))
	fmt.Printf("  KDM              : %s\n", kdmVer)
	fmt.Printf("  Kubernetes       : %s\n", k8sVer)
	fmt.Printf("  Cluster tool     : %s @ %s\n", mode, clusterVer)
	fmt.Printf("  cert-manager     : %s\n", certMgrVer)
	fmt.Printf("  Helm chart       : %s\n", chart.String())
	fmt.Printf("  Namespace        : %s\n", f.namespace)
	fmt.Printf("  Hostname         : %s\n", hv.Hostname)
	fmt.Printf("  Bootstrap PW     : %s\n", f.bootstrapPassword)
	if f.dryRun {
		fmt.Printf("  Mode             : DRY RUN\n")
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
}

// promptConfirm asks for a yes/no answer on stdin.
func promptConfirm(prompt string) bool {
	fmt.Printf("\n%s%s [y/N]: %s", colorYellow, prompt, colorReset)
	var answer string
	_, _ = fmt.Fscanln(os.Stdin, &answer)
	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}

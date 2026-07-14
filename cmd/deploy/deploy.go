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
	prime             bool
	channel           string
	mode              string // "", "k3s", "k3d"
	hostname          string
	namespace         string
	valuesFile        string
	helmSet           []string
	dryRun            bool
	clusterName       string // k3d only
	yes               bool   // skip confirmation prompt
	bootstrapPassword string
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
  rancher-deployer deploy --rancher-version 2.8.5 --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(f)
		},
	}

	cmd.Flags().StringVar(&f.rancherVersion, "rancher-version", "", "Rancher version to install, e.g. 2.8.5 (required)")
	cmd.Flags().StringVar(&f.k8sVersion, "k8s-version", "", "Target k8s major.minor, e.g. 1.28 (default: auto-select from support matrix)")
	cmd.Flags().BoolVar(&f.prime, "prime", false, "Use Rancher Prime instead of community edition")
	cmd.Flags().StringVar(&f.channel, "channel", "stable", "Release channel: stable (GA), latest (RC), alpha")
	cmd.Flags().StringVar(&f.mode, "mode", "", "Force install mode: k3s or k3d (default: auto-detect)")
	cmd.Flags().StringVar(&f.hostname, "hostname", "", "Hostname for Rancher ingress (default: <node-ip>.sslip.io)")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Kubernetes namespace for Rancher")
	cmd.Flags().StringVar(&f.valuesFile, "values-file", "", "Path to YAML file with Helm chart values")
	cmd.Flags().StringArrayVar(&f.helmSet, "set", nil, "Set Helm chart value (repeatable): --set key=value")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print resolved plan and commands without executing")
	cmd.Flags().StringVar(&f.clusterName, "cluster-name", "rancher-local", "k3d cluster name (k3d mode only)")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt (for CI/non-interactive use)")
	cmd.Flags().StringVar(&f.bootstrapPassword, "bootstrap-password", "letsmein", "Initial admin password for Rancher")

	_ = cmd.MarkFlagRequired("rancher-version")

	return cmd
}

func runDeploy(f *deployFlags) error {
	// Normalise version — strip leading 'v'
	f.rancherVersion = strings.TrimPrefix(f.rancherVersion, "v")
	isPrerelease := k8sresolver.IsPrerelease(f.rancherVersion)

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

	// ── Step 2: Resolve Rancher support matrix ───────────────────────────────
	printStep(2, "Fetching Rancher support matrix")
	matrix, err := kdm.FetchSupportMatrix(f.rancherVersion)
	if err != nil {
		return fmt.Errorf("support matrix lookup failed: %w", err)
	}
	printInfo("Rancher v%s supports k8s versions: %s",
		f.rancherVersion, strings.Join(matrix.SupportedMinors(), ", "))

	// ── Step 3: Resolve k8s version ─────────────────────────────────────────
	printStep(3, "Resolving Kubernetes version")
	resolvedK8s, err := k8sresolver.ResolveK8s(f.k8sVersion, matrix)
	if err != nil {
		return err
	}
	printInfo("Target k8s version: %s", resolvedK8s)

	// ── Step 4: Resolve cluster version ──────────────────────────────────────
	if mode == "existing" {
		printStep(4, "Validating existing cluster")
	} else {
		printStep(4, "Resolving k3s/k3d version")
	}
	var clusterVersion string
	clusterVersion, err = clusterProvider.ResolveClusterVersion(context.Background(), resolvedK8s)
	if err != nil {
		return err
	}
	if mode != "existing" {
		printInfo("Cluster version: %s", clusterVersion)
	}

	// ── Step 5: Resolve cert-manager version ─────────────────────────────────
	printStep(5, "Resolving cert-manager version")
	certManagerVersion, err := rancher.ResolveCertManagerVersion()
	if err != nil {
		return err
	}
	printInfo("cert-manager version: %s", certManagerVersion)

	// ── Step 6: Resolve Helm chart details ──────────────────────────────────
	printStep(6, "Resolving Helm chart")
	channel, err := rancher.NormaliseChannel(f.channel)
	if err != nil {
		return err
	}
	chartRef := rancher.ChartRef(f.prime, isPrerelease, channel, f.rancherVersion)
	printInfo("Chart: %s", chartRef.String())

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
	printPlan(f, mode, resolvedK8s, clusterVersion, certManagerVersion, chartRef, helmValues)

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
	if err := rancher.InstallCertManager(certManagerVersion); err != nil {
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
	if err := rancher.WaitReady(f.namespace); err != nil {
		return err
	}

	fmt.Println()
	printSuccess("Rancher v%s deployed successfully!", f.rancherVersion)
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

func printPlan(f *deployFlags, mode, k8sVer, clusterVer, certMgrVer string, chart rancher.Chart, hv rancher.HelmValues) {
	edition := "Community"
	if f.prime {
		edition = "Prime"
	}
	edition += " " + f.channel
	fmt.Printf("%s━━ Deployment Plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
	fmt.Printf("  Rancher version  : v%s (%s)\n", f.rancherVersion, edition)
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

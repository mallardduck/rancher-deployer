// Package deployflow is the Rancher deploy workflow shared by the
// rancher-deployer and rancher-demo binaries: resolve versions, build values,
// show the plan, then install cert-manager and Rancher through a backend.
// It depends only on the narrow Cluster and Installer interfaces so each
// binary links just the cluster and helm implementations it uses.
package deployflow

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/cli"
	"github.com/mallardduck/rancher-deployer/internal/expose"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/kdm"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

// Installer is the subset of a helm backend the deploy flow uses.
type Installer interface {
	InstallCertManager(version string) error
	EnsureRepo(name, url string, yes bool) error
	Install(namespace string, chart rancher.Chart, values rancher.HelmValues) error
	WaitReady(namespace string) error
}

// Cluster is the cluster the flow deploys onto.
type Cluster interface {
	// ResolveClusterVersion resolves (or validates, for existing clusters) the
	// concrete cluster version for the given k8s version.
	ResolveClusterVersion(ctx context.Context, k8sVersion string) (string, error)
	// Setup prepares the cluster using the resolved cluster version.
	Setup(ctx context.Context, clusterVersion string) error
	// Installer returns the helm backend used to install charts.
	Installer() Installer
}

// Options are the inputs to Run.
type Options struct {
	// SelectCluster resolves the install mode and builds the cluster for it.
	// reason is shown to the user when the mode was auto-detected.
	SelectCluster func() (cluster Cluster, mode, reason string, err error)

	// Preflight, if set, runs right after the banner and before any step.
	// Returning an error aborts the deploy.
	Preflight func(ctx context.Context) error

	// Title is shown under the banner.
	Title string

	RancherVersion    string
	K8sVersion        string
	Channel           string
	Commit            string // head channel only — pin to a specific head build
	Hostname          string
	Expose            string // how Rancher is reached; "" means sslip
	Namespace         string
	ValuesFile        string
	BootstrapPassword string
	FailurePolicy     string

	HelmSet []string

	Prime  bool
	DryRun bool
	Yes    bool // skip confirmation prompt
}

// Run executes the deploy workflow.
func Run(ctx context.Context, f Options) error {
	// Normalise version — strip leading 'v'
	f.RancherVersion = strings.TrimPrefix(f.RancherVersion, "v")

	channel, err := rancher.NormaliseChannel(f.Channel)
	if err != nil {
		return err
	}

	exposeMode, err := expose.New(f.Expose)
	if err != nil {
		return err
	}

	failurePolicy, err := rancher.ParseFailurePolicy(f.FailurePolicy)
	if err != nil {
		return err
	}

	// --rancher-version is required, except for the one case where a commit
	// alone is enough to resolve a chart: --prime --channel head --commit.
	if f.RancherVersion == "" && (channel != rancher.ChannelHead || !f.Prime || f.Commit == "") {
		return fmt.Errorf("--rancher-version is required (unless using --prime --channel head --commit)")
	}

	fmt.Println()
	cli.PrintBanner(f.Title)

	if f.Preflight != nil {
		if perr := f.Preflight(ctx); perr != nil {
			return perr
		}
	}

	// ── Step 1: Detect install mode ─────────────────────────────────────────
	cli.PrintStep(1, "Detecting install mode")
	cluster, mode, reason, err := f.SelectCluster()
	if err != nil {
		return err
	}
	if reason != "" {
		cli.PrintInfo("Auto-detected: %s (%s)", mode, reason)
	} else {
		cli.PrintInfo("Mode: %s", mode)
	}

	// ── Step 2: Resolve Helm chart details ──────────────────────────────────
	// Resolved before the support matrix because commit-only resolution
	// (--prime --channel head --commit, no --rancher-version) doesn't know
	// its minor until the chart is resolved.
	cli.PrintStep(2, "Resolving Helm chart")
	chartRef, err := cli.ResolveChart(f.Prime, channel, f.RancherVersion, f.Commit)
	if err != nil {
		return err
	}
	cli.PrintInfo("Chart: %s", chartRef.String())

	// ── Step 3: Resolve Rancher support matrix ───────────────────────────────
	cli.PrintStep(3, "Fetching Rancher support matrix")
	kdmTarget := f.RancherVersion
	if kdmTarget == "" {
		// Commit-only resolution: derive just the minor from the resolved
		// chart version instead, so the KDM plan line shows a clean minor
		// rather than the full head build string. Safe because community
		// always requires an explicit minor and can never reach this branch
		// with an empty f.RancherVersion — see ResolveChart.
		kdmTarget = cli.MinorOf(chartRef.Version)
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
			cli.PrintWarning("No KDM data for Rancher %s yet — using the previous minor's support matrix as a best-effort approximation; k8s compatibility isn't guaranteed", kdmTarget)
		}
	} else {
		matrix, err = kdm.FetchSupportMatrix(kdmTarget)
		if err != nil {
			return fmt.Errorf("support matrix lookup failed: %w", err)
		}
	}
	cli.PrintInfo("Rancher v%s supports k8s versions: %s",
		chartRef.Version, strings.Join(matrix.SupportedMinors(), ", "))

	// ── Step 4: Resolve k8s version ─────────────────────────────────────────
	cli.PrintStep(4, "Resolving Kubernetes version")
	resolvedK8s, err := k8sresolver.ResolveK8s(f.K8sVersion, matrix)
	if err != nil {
		return err
	}
	cli.PrintInfo("Target k8s version: %s", resolvedK8s)

	// ── Step 5: Resolve cluster version ──────────────────────────────────────
	if mode == "existing" {
		cli.PrintStep(5, "Validating existing cluster")
	} else {
		cli.PrintStep(5, "Resolving k3s/k3d version")
	}
	var clusterVersion string
	clusterVersion, err = cluster.ResolveClusterVersion(ctx, resolvedK8s)
	if err != nil {
		return err
	}
	if mode != "existing" {
		cli.PrintInfo("Cluster version: %s", clusterVersion)
	}

	// ── Step 6: Resolve cert-manager version ─────────────────────────────────
	cli.PrintStep(6, "Resolving cert-manager version")
	certManagerVersion, err := rancher.ResolveCertManagerVersion()
	if err != nil {
		return err
	}
	cli.PrintInfo("cert-manager version: %s", certManagerVersion)

	// ── Step 7: Build Helm values ────────────────────────────────────────────
	cli.PrintStep(7, "Building Helm values")
	exposePlan, err := cli.NewExposePlan(exposeMode, f.Hostname, f.HelmSet, expose.Env{ClusterMode: mode, Namespace: f.Namespace})
	if err != nil {
		return err
	}
	exposure := exposePlan.Exposure
	helmValues, err := rancher.BuildHelmValues(f.ValuesFile, exposure.MergeSets(f.HelmSet), exposure.Hostname, f.Namespace, f.BootstrapPassword)
	if err != nil {
		return err
	}
	helmValues.FailurePolicy = failurePolicy
	if len(helmValues.SetFlags) > 0 {
		for _, s := range helmValues.SetFlags {
			cli.PrintInfo("  --set %s", s)
		}
	}
	if helmValues.ValuesFile != "" {
		cli.PrintInfo("  --values %s", helmValues.ValuesFile)
	}

	// ── Print plan ───────────────────────────────────────────────────────────
	fmt.Println()
	printPlan(&f, mode, resolvedK8s, clusterVersion, certManagerVersion, cli.KDMLine(matrix, kdmFlavor, usedFallbackKDM), chartRef, helmValues)

	if f.DryRun {
		exposePlan.PrintDryRun()
		fmt.Println()
		cli.PrintWarning("Dry run — no changes made.")
		return nil
	}

	// ── Confirm ──────────────────────────────────────────────────────────────
	if !f.Yes && !cli.PromptConfirm("Proceed with deployment?") {
		fmt.Println("Aborted.")
		return nil
	}

	if err := cli.RunExposeSteps(ctx, exposePlan.Steps, f.Yes, os.Stdin); err != nil {
		return err
	}

	// ── Step 8: Install cluster ───────────────────────────────────────────────
	cli.PrintStep(8, "Installing cluster")
	if err := cluster.Setup(ctx, clusterVersion); err != nil {
		return err
	}

	// Cluster-level config the --expose mode needs, applied before Rancher exists.
	if err := exposePlan.ApplyConfig(); err != nil {
		return err
	}

	// ── Step 9: Install cert-manager ─────────────────────────────────────────
	cli.PrintStep(9, "Installing cert-manager")
	if err := cluster.Installer().InstallCertManager(certManagerVersion); err != nil {
		return err
	}

	// ── Step 10: Ensure Helm repo ────────────────────────────────────────────
	cli.PrintStep(10, "Configuring Helm repo")
	if err := cluster.Installer().EnsureRepo(chartRef.RepoName, chartRef.RepoURL, f.Yes); err != nil {
		return err
	}

	// ── Step 11: Deploy Rancher ───────────────────────────────────────────────
	cli.PrintStep(11, "Deploying Rancher via Helm")
	if err := cluster.Installer().Install(f.Namespace, chartRef, helmValues); err != nil {
		return err
	}

	// ── Step 12: Wait for Rancher ────────────────────────────────────────────
	cli.PrintStep(12, "Waiting for Rancher to become ready")
	if err := cluster.Installer().WaitReady(f.Namespace); err != nil {
		return err
	}

	exposePlan.Verify(ctx)

	fmt.Println()
	cli.PrintSuccess("Rancher v%s deployed successfully!", chartRef.Version)
	cli.PrintInfo("Access URL : https://%s", helmValues.Hostname)
	cli.PrintInfo("Username   : admin")
	cli.PrintInfo("Password   : %s", f.BootstrapPassword)
	fmt.Println()

	return nil
}

func printPlan(f *Options, mode, k8sVer, clusterVer, certMgrVer, kdmVer string, chart rancher.Chart, hv rancher.HelmValues) { //nolint:revive // 8 args are all distinct plan fields; a wrapper struct would add noise without clarity
	edition := "Community"
	if f.Prime {
		edition = "Prime"
	}
	edition += " " + f.Channel
	fmt.Printf("%s━━ Deployment Plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", cli.ColorCyan, cli.ColorReset)
	fmt.Printf("  Rancher version  : %s\n", cli.VersionLine(f.RancherVersion, chart.Version, edition, f.Commit))
	fmt.Printf("  KDM              : %s\n", kdmVer)
	fmt.Printf("  Kubernetes       : %s\n", k8sVer)
	fmt.Printf("  Cluster tool     : %s @ %s\n", mode, clusterVer)
	fmt.Printf("  cert-manager     : %s\n", certMgrVer)
	fmt.Printf("  Helm chart       : %s\n", chart.String())
	fmt.Printf("  Namespace        : %s\n", f.Namespace)
	fmt.Printf("  Hostname         : %s\n", hv.Hostname)
	fmt.Printf("  Bootstrap PW     : %s\n", f.BootstrapPassword)
	fmt.Printf("  Failure policy   : %s\n", hv.FailurePolicy)
	if f.DryRun {
		fmt.Printf("  Mode             : DRY RUN\n")
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", cli.ColorCyan, cli.ColorReset)
}

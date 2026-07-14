package deploy

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/detect"
	"github.com/mallardduck/rancher-deployer/internal/provider"
	"github.com/mallardduck/rancher-deployer/internal/runner"
)

type teardownFlags struct {
	mode        string
	clusterName string
	namespace   string
	yes         bool
}

func newTeardownCmd() *cobra.Command {
	f := &teardownFlags{}

	cmd := &cobra.Command{
		Use:   "teardown",
		Short: "Remove a Rancher deployment and its cluster (reverse of deploy)",
		Example: `  rancher-deployer teardown
  rancher-deployer teardown --mode k3d --cluster-name rancher-local
  rancher-deployer teardown --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTeardown(f)
		},
	}

	cmd.Flags().StringVar(&f.mode, "mode", "", "k3s or k3d (default: auto-detect)")
	cmd.Flags().StringVar(&f.clusterName, "cluster-name", "rancher-local", "k3d cluster name (k3d mode only)")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Kubernetes namespace Rancher was installed into")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

func runTeardown(f *teardownFlags) error {
	fmt.Println()

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

	fmt.Printf("%s━━ Teardown Plan ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)
	fmt.Printf("  Rancher namespace : %s\n", f.namespace)
	fmt.Printf("  Cluster tool      : %s\n", mode)
	if mode == "k3d" {
		fmt.Printf("  Cluster name      : %s\n", f.clusterName)
	}
	fmt.Printf("%s━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━%s\n", colorCyan, colorReset)

	if !f.yes && !promptConfirm("Proceed with teardown? This is destructive and irreversible.") {
		fmt.Println("Aborted.")
		return nil
	}

	// ── Step 1: Uninstall Rancher ────────────────────────────────────────────
	printStep(1, "Uninstalling Rancher")
	if err := runner.Helm("uninstall", "rancher", "-n", f.namespace); err != nil {
		printWarning("helm uninstall rancher failed (may not be installed): %v", err)
	}

	// ── Step 2: Remove cert-manager ──────────────────────────────────────────
	printStep(2, "Removing cert-manager")
	if err := runner.Kubectl("delete", "namespace", "cert-manager", "--ignore-not-found"); err != nil {
		printWarning("cert-manager namespace removal failed: %v", err)
	}

	// ── Step 3: Remove cluster ───────────────────────────────────────────────
	printStep(3, "Removing cluster")
	if err := clusterProvider.Teardown(context.Background(), provider.TeardownOptions{}); err != nil {
		return fmt.Errorf("cluster teardown failed: %w", err)
	}

	fmt.Println()
	printSuccess("Teardown complete.")
	fmt.Println()
	return nil
}

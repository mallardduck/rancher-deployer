// Package deploy wires together the CLI commands and orchestrates the deployment workflow.
package deploy

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	clusterexisting "github.com/mallardduck/rancher-deployer/internal/clusters/existing"
	clusterk3d "github.com/mallardduck/rancher-deployer/internal/clusters/k3d"
	clusterk3s "github.com/mallardduck/rancher-deployer/internal/clusters/k3s"
	"github.com/mallardduck/rancher-deployer/internal/deployflow"
	"github.com/mallardduck/rancher-deployer/internal/detect"
	"github.com/mallardduck/rancher-deployer/internal/expose"
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
	expose            string // "", "sslip"
	namespace         string
	valuesFile        string
	clusterName       string // k3d only
	bootstrapPassword string
	failurePolicy     string
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
	cmd.Flags().StringVar(&f.expose, "expose", expose.SSLIP, "How Rancher is reached: sslip (node IP via sslip.io)")
	cmd.Flags().StringVar(&f.hostname, "hostname", "", "Hostname for Rancher ingress (default: derived from --expose, e.g. rancher.<node-ip>.sslip.io)")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Kubernetes namespace for Rancher")
	cmd.Flags().StringVar(&f.valuesFile, "values-file", "", "Path to YAML file with Helm chart values")
	cmd.Flags().StringArrayVar(&f.helmSet, "set", nil, "Set Helm chart value (repeatable): --set key=value")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print resolved plan and commands without executing")
	cmd.Flags().StringVar(&f.clusterName, "cluster-name", "rancher-local", "k3d cluster name (k3d mode only)")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt (for CI/non-interactive use)")
	cmd.Flags().StringVar(&f.bootstrapPassword, "bootstrap-password", "letsmein", "Initial admin password for Rancher")
	cmd.Flags().StringVar(&f.failurePolicy, "failure-policy", rancher.FailurePolicyAbort, "What to do if the install fails: abort (leave it for inspection) or reinstall (roll back/clean up automatically)")

	return cmd
}

// providerCluster adapts a provider.Provider to deployflow.Cluster.
type providerCluster struct{ provider.Provider }

func (p providerCluster) Setup(ctx context.Context, clusterVersion string) error {
	return p.Provider.Setup(ctx, provider.SetupOptions{ClusterVersion: clusterVersion})
}

func (p providerCluster) Installer() deployflow.Installer { return p.Helm() }

func runDeploy(f *deployFlags) error {
	return deployflow.Run(context.Background(), deployflow.Options{
		RancherVersion:    f.rancherVersion,
		K8sVersion:        f.k8sVersion,
		Channel:           f.channel,
		Commit:            f.commit,
		Hostname:          f.hostname,
		Expose:            f.expose,
		Namespace:         f.namespace,
		ValuesFile:        f.valuesFile,
		BootstrapPassword: f.bootstrapPassword,
		FailurePolicy:     f.failurePolicy,
		HelmSet:           f.helmSet,
		Prime:             f.prime,
		DryRun:            f.dryRun,
		Yes:               f.yes,
		Title:             "Deployer",
		SelectCluster: func() (deployflow.Cluster, string, string, error) {
			mode, reason, err := detect.ResolveMode(f.mode)
			if err != nil {
				return nil, "", "", err
			}
			p, err := buildProvider(mode, f.clusterName)
			if err != nil {
				return nil, "", "", err
			}
			return providerCluster{p}, mode, reason, nil
		},
	})
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

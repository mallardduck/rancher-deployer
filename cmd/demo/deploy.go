package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/cli"
	"github.com/mallardduck/rancher-deployer/internal/clusters/existing/validate"
	"github.com/mallardduck/rancher-deployer/internal/deployflow"
	"github.com/mallardduck/rancher-deployer/internal/doctor"
	"github.com/mallardduck/rancher-deployer/internal/helm/controller"
	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

// preflightTimeout bounds the inline doctor checks that run before a deploy.
const preflightTimeout = 60 * time.Second

type deployFlags struct {
	rancherVersion    string
	k8sVersion        string
	channel           string
	hostname          string
	namespace         string
	valuesFile        string
	bootstrapPassword string
	helmSet           []string
	prime             bool
	yes               bool
}

func newDeployCmd() *cobra.Command {
	f := &deployFlags{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy Rancher onto the running k3s cluster",
		Long: `Deploys Rancher onto the Kubernetes cluster kubectl is already pointed at,
installing cert-manager and Rancher through k3s's built-in helm-controller.`,
		Example: `  rancher-demo deploy --rancher-version 2.8.5 --yes
  rancher-demo deploy --rancher-version 2.8.5 --values-file ./values.yaml --set replicas=1 --yes`,
		RunE: func(*cobra.Command, []string) error {
			return runDeploy(f)
		},
	}

	cmd.Flags().StringVar(&f.rancherVersion, "rancher-version", "", "Rancher version to install, e.g. 2.8.5 (a bare minor such as 2.8 resolves the newest patch)")
	cmd.Flags().StringVar(&f.k8sVersion, "k8s-version", "", "Target k8s major.minor, e.g. 1.28 (default: auto-select from support matrix)")
	cmd.Flags().BoolVar(&f.prime, "prime", false, "Use Rancher Prime instead of community edition")
	cmd.Flags().StringVar(&f.channel, "channel", "stable", "Release channel: stable (GA), latest (RC), alpha, head (requires a minor, e.g. --rancher-version 2.15)")
	cmd.Flags().StringVar(&f.hostname, "hostname", "", "Hostname for Rancher ingress (default: <node-ip>.sslip.io)")
	cmd.Flags().StringVar(&f.namespace, "namespace", "cattle-system", "Kubernetes namespace for Rancher")
	cmd.Flags().StringVar(&f.valuesFile, "values-file", "", "Path to YAML file with Helm chart values")
	cmd.Flags().StringArrayVar(&f.helmSet, "set", nil, "Set Helm chart value (repeatable): --set key=value")
	cmd.Flags().StringVar(&f.bootstrapPassword, "bootstrap-password", "letsmein", "Initial admin password for Rancher")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

func runDeploy(f *deployFlags) error {
	return deployflow.Run(context.Background(), deployflow.Options{
		RancherVersion:    f.rancherVersion,
		K8sVersion:        f.k8sVersion,
		Channel:           f.channel,
		Hostname:          f.hostname,
		Namespace:         f.namespace,
		ValuesFile:        f.valuesFile,
		BootstrapPassword: f.bootstrapPassword,
		FailurePolicy:     rancher.FailurePolicyAbort,
		HelmSet:           f.helmSet,
		Prime:             f.prime,
		Yes:               f.yes,
		Title:             "Demo",
		Preflight:         preflight,
		SelectCluster: func() (deployflow.Cluster, string, string, error) {
			return existingCluster{}, "existing", "", nil
		},
	})
}

// preflight runs the demo doctor checks inline and aborts on critical failures.
func preflight(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, preflightTimeout)
	defer cancel()

	results := doctor.NewDemoDoctor(nil).RunAll(ctx)
	cli.DisplayDoctorResults(results)

	passed, warned, failed := cli.CountDoctorResults(results)
	fmt.Println()
	cli.PrintInfo("Preflight: %d passed, %d warnings, %d failed", passed, warned, failed)
	fmt.Println()

	if doctor.HasCriticalFailures(results) {
		return fmt.Errorf("preflight checks failed")
	}
	return nil
}

// existingCluster is the pre-existing k3s cluster the demo image starts
// before deploying. Charts are installed through the helm-controller backend,
// since the image has no helm CLI.
type existingCluster struct{}

func (existingCluster) ResolveClusterVersion(_ context.Context, k8sVersion string) (string, error) {
	clusterVersion, err := validate.Cluster()
	if err != nil {
		return "", err
	}
	if err := validate.Version(clusterVersion, k8sVersion); err != nil {
		return "", err
	}
	return clusterVersion, nil
}

func (existingCluster) Setup(context.Context, string) error {
	fmt.Println("  Using existing Kubernetes cluster (kubectl already configured)")
	return nil
}

func (existingCluster) Installer() deployflow.Installer { return controller.New() }

package deploy

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/detect"
	"github.com/mallardduck/rancher-deployer/internal/k8sresolver"
	"github.com/mallardduck/rancher-deployer/internal/kdm"
)

func newResolveCmd() *cobra.Command {
	var rancherVersion string
	var k8sVersion string
	var mode string

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve and print version information without installing anything",
		Example: `  rancher-deployer resolve --rancher-version 2.8.5
  rancher-deployer resolve --rancher-version 2.8.5 --k8s-version 1.27`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rancherVersion = strings.TrimPrefix(rancherVersion, "v")

			fmt.Println()
			printStep(1, "Fetching Rancher support matrix")
			matrix, err := kdm.FetchSupportMatrix(rancherVersion)
			if err != nil {
				return fmt.Errorf("support matrix lookup failed: %w", err)
			}
			printInfo("Rancher v%s supports: %s",
				rancherVersion, strings.Join(matrix.SupportedMinors(), ", "))

			printStep(2, "Resolving Kubernetes version")
			resolvedK8s, err := k8sresolver.ResolveK8s(k8sVersion, matrix)
			if err != nil {
				return err
			}
			printInfo("Resolved k8s: %s", resolvedK8s)

			printStep(3, "Resolving cluster version")
			if mode == "" {
				mode, _ = detect.InstallMode()
			}
			clusterVer, err := k8sresolver.ResolveClusterVersion(mode, resolvedK8s)
			if err != nil {
				return err
			}
			printInfo("Resolved %s version: %s", mode, clusterVer)

			printStep(4, "Prerelease status")
			if k8sresolver.IsPrerelease(rancherVersion) {
				printInfo("Version %s is a prerelease", rancherVersion)
			} else {
				printInfo("Version %s is a stable release", rancherVersion)
			}

			fmt.Println()
			return nil
		},
	}

	cmd.Flags().StringVar(&rancherVersion, "rancher-version", "", "Rancher version (required)")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "", "Target k8s major.minor (optional)")
	cmd.Flags().StringVar(&mode, "mode", "", "k3s or k3d (default: auto-detect)")
	_ = cmd.MarkFlagRequired("rancher-version")

	return cmd
}

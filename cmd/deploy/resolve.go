package deploy

import (
	"encoding/json"
	"fmt"
	"os"
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
	var outputFormat string

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve and print version information without installing anything",
		Example: `  rancher-deployer resolve --rancher-version 2.8.5
  rancher-deployer resolve --rancher-version 2.8.5 --k8s-version 1.27
  rancher-deployer resolve --rancher-version 2.8.5 --mode k3s --output k3s-version
  rancher-deployer resolve --rancher-version 2.8.5 --output json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			rancherVersion = strings.TrimPrefix(rancherVersion, "v")
			isPrerelease := k8sresolver.IsPrerelease(rancherVersion)

			// Resolve all versions (needed for all output formats)
			matrix, err := kdm.FetchSupportMatrix(rancherVersion)
			if err != nil {
				return fmt.Errorf("support matrix lookup failed: %w", err)
			}

			resolvedK8s, err := k8sresolver.ResolveK8s(k8sVersion, matrix)
			if err != nil {
				return err
			}

			if mode == "" {
				mode, _ = detect.InstallMode()
			}
			clusterVer, err := k8sresolver.ResolveClusterVersion(mode, resolvedK8s)
			if err != nil {
				return err
			}

			// Handle machine-readable output formats
			switch outputFormat {
			case "k3s-version", "k3d-version", "recommended":
				// Just print the cluster version (k3s or k3d)
				// "recommended" is an alias for k3s-version (latest compatible version)
				fmt.Println(clusterVer)
				return nil
			case "k8s-version":
				// Just print the k8s version
				fmt.Println(resolvedK8s)
				return nil
			case "json":
				// JSON output with all resolved values and k3s versions for all supported minors
				supportedMinors := matrix.SupportedMinors()

				// Resolve k3s versions for all supported k8s minors
				k3sVersions := make(map[string]string)
				for _, minor := range supportedMinors {
					k8sVer, err := k8sresolver.ResolveK8s(minor, matrix)
					if err != nil {
						continue // Skip if can't resolve
					}
					clusterVer, err := k8sresolver.ResolveClusterVersion(mode, k8sVer)
					if err != nil {
						continue // Skip if can't resolve
					}
					k3sVersions[minor] = clusterVer
				}

				output := map[string]interface{}{
					"rancher":              rancherVersion,
					"supported_k8s_minors": supportedMinors,
					"k3s_versions":         k3sVersions,
					"recommended":          clusterVer, // Latest compatible version
					"mode":                 mode,
					"prerelease":           isPrerelease,
				}
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(output)
			case "":
				// Human-readable output (default)
				fmt.Println()
				printStep(1, "Fetching Rancher support matrix")
				printInfo("Rancher v%s supports: %s",
					rancherVersion, strings.Join(matrix.SupportedMinors(), ", "))

				printStep(2, "Resolving Kubernetes version")
				printInfo("Resolved k8s: %s", resolvedK8s)

				printStep(3, "Resolving cluster version")
				printInfo("Resolved %s version: %s", mode, clusterVer)

				printStep(4, "Prerelease status")
				if isPrerelease {
					printInfo("Version %s is a prerelease", rancherVersion)
				} else {
					printInfo("Version %s is a stable release", rancherVersion)
				}

				fmt.Println()
				return nil
			default:
				return fmt.Errorf("invalid output format %q: must be 'k3s-version', 'k3d-version', 'k8s-version', 'json', or empty for human-readable", outputFormat)
			}
		},
	}

	cmd.Flags().StringVar(&rancherVersion, "rancher-version", "", "Rancher version (required)")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "", "Target k8s major.minor (optional)")
	cmd.Flags().StringVar(&mode, "mode", "", "k3s or k3d (default: auto-detect)")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format: k3s-version, k3d-version, k8s-version, json (default: human-readable)")
	_ = cmd.MarkFlagRequired("rancher-version")

	return cmd
}

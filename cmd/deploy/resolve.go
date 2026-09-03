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
	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

func newResolveCmd() *cobra.Command {
	var rancherVersion string
	var k8sVersion string
	var mode string
	var outputFormat string
	var prime bool
	var channel string
	var commit string

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve and print version information without installing anything",
		Example: `  # Auto-resolve latest stable Rancher version
  rancher-deployer resolve

  # Auto-resolve latest from a specific repo
  rancher-deployer resolve --prime --channel latest

  # Resolve a specific version
  rancher-deployer resolve --rancher-version 2.8.5
  rancher-deployer resolve --rancher-version 2.8.5 --k8s-version 1.27
  rancher-deployer resolve --rancher-version 2.8.5 --mode k3s --output k3s-version
  rancher-deployer resolve --rancher-version 2.8.5 --output json

  # Auto-resolve the newest patch in a minor
  rancher-deployer resolve --rancher-version 2.8

  # Auto-resolve the newest head build for a minor
  rancher-deployer resolve --channel head --rancher-version 2.15

  # Resolve a specific reported head build by commit
  rancher-deployer resolve --channel head --rancher-version 2.15 --commit b03c4de

  # Prime head builds share one repo across minors, so a commit alone is enough
  rancher-deployer resolve --prime --channel head --commit b03c4de`,
		RunE: func(cmd *cobra.Command, args []string) error {
			normalizedChannel, err := rancher.NormaliseChannel(channel)
			if err != nil {
				return err
			}

			requestedVersion := strings.TrimPrefix(rancherVersion, "v")

			chart, err := resolveChart(prime, normalizedChannel, requestedVersion, commit)
			if err != nil {
				return fmt.Errorf("could not resolve Rancher version: %w", err)
			}
			rancherVersion = chart.Version
			autoResolved := requestedVersion != rancherVersion
			isPrerelease := chart.IsPrerelease

			// Fast path: rancher-version output only needs the chart resolution above.
			if outputFormat == "rancher-version" {
				fmt.Println(rancherVersion)
				return nil
			}

			// KDM only understands major.minor. The resolved chart version is
			// safe to use for non-head channels (always a plain patch version),
			// but head builds carry a git hash that majorMinor() can't parse
			// reliably (community head, in particular, has no patch segment at
			// all — e.g. "2.15-<hash>-head"). Use whatever minor the user
			// actually asked for instead; fall back to the resolved version only
			// when no version was given at all (auto-latest, non-head only).
			kdmVersion := requestedVersion
			if kdmVersion == "" {
				kdmVersion = rancherVersion
			}

			// Resolve all versions (needed for remaining output formats)
			matrix, err := kdm.FetchSupportMatrix(kdmVersion)
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
					ver, err := k8sresolver.ResolveClusterVersion(mode, k8sVer)
					if err != nil {
						continue // Skip if can't resolve
					}
					k3sVersions[minor] = ver
					clusterVer = ver
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
				if autoResolved {
					switch {
					case requestedVersion == "" && commit != "":
						printInfo("Resolved v%s from commit %s (channel: %s)", rancherVersion, commit, normalizedChannel)
					case requestedVersion == "":
						printInfo("Auto-resolved latest Rancher version: v%s (channel: %s)", rancherVersion, normalizedChannel)
					default:
						printInfo("Auto-resolved v%s from requested %s (channel: %s)", rancherVersion, requestedVersion, normalizedChannel)
					}
					fmt.Println()
				}
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
				return fmt.Errorf("invalid output format %q: must be 'rancher-version', 'k3s-version', 'k3d-version', 'k8s-version', 'json', or empty for human-readable", outputFormat)
			}
		},
	}

	cmd.Flags().StringVar(&rancherVersion, "rancher-version", "", "Rancher version (omit to auto-resolve latest). A bare minor (e.g. 2.8) auto-resolves the newest patch (or newest head build for --channel head, which requires one)")
	cmd.Flags().StringVar(&k8sVersion, "k8s-version", "", "Target k8s major.minor (optional)")
	cmd.Flags().StringVar(&mode, "mode", "", "k3s or k3d (default: auto-detect)")
	cmd.Flags().StringVar(&outputFormat, "output", "", "Output format: k3s-version, k3d-version, k8s-version, json (default: human-readable)")
	cmd.Flags().BoolVar(&prime, "prime", false, "Use Rancher Prime repository for version resolution")
	cmd.Flags().StringVar(&channel, "channel", "stable", "Release channel: stable (GA), latest (RC), alpha, head (continuously-published head builds — requires a minor, e.g. --rancher-version 2.15)")
	cmd.Flags().StringVar(&commit, "commit", "", "Pin --channel head to the head build whose commit starts with this (instead of the newest one). With --prime, --rancher-version can be omitted — Prime head builds share one repo across minors")

	return cmd
}

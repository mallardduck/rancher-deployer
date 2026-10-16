package deploy

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/cli"
)

// binaryName is the executable name shown in help and version output.
const binaryName = "rancher-deployer"

var rootCmd = &cobra.Command{
	Use:   binaryName,
	Short: "Deploy Rancher on k3s/k3d with automatic version resolution",
	Long: `rancher-deployer installs Rancher on top of k3s or k3d.

It automatically resolves a compatible Kubernetes version from Rancher's
support matrix, then installs the appropriate k3s/k3d version before
deploying Rancher via Helm.

Auto-detection logic:
  macOS              → k3d
  Linux + Docker     → k3d
  Linux (bare)       → k3s

Override with --mode=k3d or --mode=k3s.
`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(newUpgradeCmd())
	rootCmd.AddCommand(cli.NewResolveCmd(binaryName))
	rootCmd.AddCommand(newTeardownCmd())
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newSelfUpdateCmd())
}

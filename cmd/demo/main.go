// Command rancher-demo is the slim binary baked into the rancher-demo container image.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/cli"
	"github.com/mallardduck/rancher-deployer/internal/version"
)

const binaryName = "rancher-demo"

func main() {
	rootCmd := &cobra.Command{
		Use:   binaryName,
		Short: "Resolve versions and deploy Rancher inside the rancher-demo image",
	}
	rootCmd.AddCommand(cli.NewResolveCmd(binaryName))
	rootCmd.AddCommand(newDeployCmd())
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(*cobra.Command, []string) {
			fmt.Println(version.Info(binaryName))
		},
	})

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

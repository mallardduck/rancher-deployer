package deploy

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/version"
)

func newVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print version information including the build version, commit, and build date.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version.Info())
		},
	}

	return cmd
}

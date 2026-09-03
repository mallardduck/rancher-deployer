package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mallardduck/rancher-deployer/internal/selfupdate"
	"github.com/mallardduck/rancher-deployer/internal/version"
)

type selfUpdateFlags struct {
	check bool
	yes   bool
}

func newSelfUpdateCmd() *cobra.Command {
	f := &selfUpdateFlags{}

	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update rancher-deployer to the latest release",
		Long: `Checks GitHub for a newer rancher-deployer release and, on confirmation,
replaces the running binary with it.

Installs managed by a package manager (Homebrew, apt, Scoop) or otherwise
not writable by the current user are detected and skipped with guidance on
how to update through that channel instead.`,
		Example: `  # Check and, if available, apply an update
  rancher-deployer self-update

  # Only check, don't prompt or apply
  rancher-deployer self-update --check

  # Apply without prompting (e.g. in scripts)
  rancher-deployer self-update --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSelfUpdate(f)
		},
	}

	cmd.Flags().BoolVar(&f.check, "check", false, "Only check for an update; don't prompt or apply")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

func runSelfUpdate(f *selfUpdateFlags) error {
	u, err := selfupdate.New()
	if err != nil {
		return err
	}

	fmt.Printf("Current version: %s\n", displayVersion(version.Short()))

	ctx := context.Background()
	res, err := u.Check(ctx)
	if err != nil {
		return fmt.Errorf("check for update: %w", err)
	}
	if !res.UpdateAvailable {
		printSuccess("Already up to date.")
		return nil
	}

	if skip := u.CanApply(); skip != nil {
		printInfo("%s is available — %s", displayVersion(res.Latest), skip.Hint)
		return nil
	}

	if f.check {
		printInfo("%s is available. Run without --check to apply it.", displayVersion(res.Latest))
		return nil
	}

	if res.Release.Notes != "" {
		fmt.Printf("\n%s\n\n", res.Release.Notes)
	}

	if !f.yes && !promptConfirm(fmt.Sprintf("Update to %s?", displayVersion(res.Latest))) {
		fmt.Println("Aborted.")
		return nil
	}

	applied, err := u.Apply(ctx, res.Release)
	if err != nil {
		return fmt.Errorf("apply update: %w", err)
	}
	if applied.Skipped != nil {
		printInfo("Not applied: %s", applied.Skipped.Hint)
		return nil
	}

	printSuccess("Updated to %s.", displayVersion(applied.Version))
	return nil
}

// displayVersion prefixes v with "v" for display, unless it already has one
// or isn't a real version (e.g. a "dev" build).
func displayVersion(v string) string {
	if v == "" || v == "dev" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

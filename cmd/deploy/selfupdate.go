package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mallardduck/binup"
	"github.com/mallardduck/rancher-deployer/internal/cli"
	"github.com/mallardduck/rancher-deployer/internal/selfupdate"
	"github.com/mallardduck/rancher-deployer/internal/version"
)

type selfUpdateFlags struct {
	check bool
	yes   bool
	pre   bool
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
how to update through that channel instead.

Only stable releases are offered by default. Pass --pre to also consider
prereleases; moving onto one asks for an extra confirmation. An install that
is already a prerelease (e.g. v1.1.0-rc.2) considers prereleases by default,
so it follows later release candidates and the stable release that supersedes
them; pass --pre=false to look only for stable releases.`,
		Example: `  # Check and, if available, apply an update
  rancher-deployer self-update

  # Only check, don't prompt or apply
  rancher-deployer self-update --check

  # Apply without prompting (e.g. in scripts)
  rancher-deployer self-update --yes

  # Include prereleases when checking or updating
  rancher-deployer self-update --check --pre

  # Install a prerelease non-interactively (both flags are required)
  rancher-deployer self-update --pre --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("pre") {
				f.pre = isPrerelease(version.Short())
			}
			return runSelfUpdate(f)
		},
	}

	cmd.Flags().BoolVar(&f.check, "check", false, "Only check for an update; don't prompt or apply")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "Skip confirmation prompt")

	cmd.Flags().BoolVar(&f.pre, "pre", false, "Also consider prereleases (default: true when running a prerelease)")

	return cmd
}

func runSelfUpdate(f *selfUpdateFlags) error {
	u, err := selfupdate.New(selfupdate.WithPrereleases(f.pre))
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
		cli.PrintSuccess("Already up to date.")
		return nil
	}

	if skip := u.CanApply(); skip != nil {
		cli.PrintInfo("%s is available — %s", displayVersion(res.Latest), skip.Hint)
		return nil
	}

	if f.check {
		kind := ""
		if res.Release.Prerelease {
			kind = " (prerelease)"
		}
		cli.PrintInfo("%s%s is available. Run without --check to apply it.", displayVersion(res.Latest), kind)
		return nil
	}

	if res.Release.Notes != "" {
		fmt.Printf("\n%s\n\n", res.Release.Notes)
	}

	for _, q := range confirmPrompts(f, res.Release) {
		if !cli.PromptConfirm(q) {
			fmt.Println("Aborted.")
			return nil
		}
	}

	applied, err := u.Apply(ctx, res.Release)
	if err != nil {
		return fmt.Errorf("apply update: %w", err)
	}
	if applied.Skipped != nil {
		cli.PrintInfo("Not applied: %s", applied.Skipped.Hint)
		return nil
	}

	cli.PrintSuccess("Updated to %s.", displayVersion(applied.Version))
	return nil
}

// confirmPrompts returns the questions to ask, in order, before applying rel.
// Stable releases need one confirmation, which --yes skips. A prerelease adds
// a second one, and only --pre together with --yes skips both, so a script
// has to opt in to prereleases explicitly.
func confirmPrompts(f *selfUpdateFlags, rel *binup.Release) []string {
	if f.yes && (!rel.Prerelease || f.pre) {
		return nil
	}
	v := displayVersion(rel.Version)
	var prompts []string
	if !f.yes {
		prompts = append(prompts, fmt.Sprintf("Update to %s?", v))
	}
	if rel.Prerelease {
		prompts = append(prompts, fmt.Sprintf("%s is a prerelease and may be unstable. Install it anyway?", v))
	}
	return prompts
}

// isPrerelease reports whether v carries a semver prerelease suffix
// (e.g. "v1.1.0-rc.2"). Build metadata after "+" is ignored, and non-version
// strings such as "dev" are not prereleases.
func isPrerelease(v string) bool {
	core, _, _ := strings.Cut(v, "+")
	return strings.Contains(core, "-")
}

// displayVersion prefixes v with "v" for display, unless it already has one
// or isn't a real version (e.g. a "dev" build).
func displayVersion(v string) string {
	if v == "" || v == "dev" || strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

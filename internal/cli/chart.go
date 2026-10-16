package cli

import (
	"fmt"

	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

// ResolveChart dispatches to rancher.ResolveChart or
// rancher.ResolveHeadChartByCommit depending on whether a commit was given,
// and validates that --commit is only used with --channel head.
func ResolveChart(prime bool, channel, versionInput, commit string) (rancher.Chart, error) {
	if commit == "" {
		return rancher.ResolveChart(prime, channel, versionInput)
	}
	if channel != rancher.ChannelHead {
		return rancher.Chart{}, fmt.Errorf("--commit only applies to --channel head")
	}
	return rancher.ResolveHeadChartByCommit(prime, versionInput, commit)
}

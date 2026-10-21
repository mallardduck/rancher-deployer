package cli

import (
	"fmt"

	"github.com/mallardduck/rancher-deployer/internal/rancher"
)

// ResolveChartSource returns the chart to install: the local chart at
// chartPath when given, otherwise the repo chart from ResolveChart. A local
// chart carries its own version, so it cannot be combined with
// --rancher-version or --commit.
func ResolveChartSource(chartPath string, prime bool, channel, versionInput, commit string) (rancher.Chart, error) {
	if chartPath == "" {
		return ResolveChart(prime, channel, versionInput, commit)
	}
	if versionInput != "" || commit != "" {
		return rancher.Chart{}, fmt.Errorf("--chart-path cannot be combined with --rancher-version or --commit; the version comes from the chart's Chart.yaml")
	}
	return rancher.LoadLocalChart(chartPath)
}

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

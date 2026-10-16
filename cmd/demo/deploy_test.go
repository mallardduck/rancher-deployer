package main

import (
	"sort"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// The entrypoint only uses these flags; the demo deploy command should not grow
// deployer-only options (mode selection, dry runs, head-build pinning, ...).
func TestDeployCmdFlags(t *testing.T) {
	want := []string{
		"bootstrap-password", "channel", "hostname", "k8s-version", "namespace",
		"prime", "rancher-version", "set", "values-file", "yes",
	}

	var got []string
	newDeployCmd().Flags().VisitAll(func(f *pflag.Flag) { got = append(got, f.Name) })
	sort.Strings(got)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("flags = %v, want %v", got, want)
	}
}

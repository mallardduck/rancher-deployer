package cli

import (
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/kdm"
)

// MinorOf extracts the "MAJOR.MINOR" prefix from a version string — used to
// derive a clean KDM lookup target from a fully-resolved chart version when
// no minor was given directly (commit-only resolution).
func MinorOf(version string) string {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return version
	}
	return parts[0] + "." + parts[1]
}

// VersionLine formats the Rancher version line for a plan/upgrade summary,
// noting the original request alongside the resolved version whenever a bare
// minor (or head channel) caused them to differ — e.g.
// "v2.15.2-fbf2130-head (requested: 2.15, Prime head)". When no minor was
// requested at all (commit-only resolution), notes the commit instead — e.g.
// "v2.16.0-b03c4de-head (Prime head, commit: b03c4de)".
func VersionLine(requested, resolved, edition, commit string) string {
	switch {
	case requested == "" && commit == "":
		return fmt.Sprintf("v%s (%s)", resolved, edition)
	case requested == "" && commit != "":
		return fmt.Sprintf("v%s (%s, commit: %s)", resolved, edition, commit)
	case requested == resolved:
		return fmt.Sprintf("v%s (%s)", resolved, edition)
	default:
		return fmt.Sprintf("v%s (requested: %s, %s)", resolved, requested, edition)
	}
}

// KDMLine formats the "which Rancher version's KDM data are we trusting"
// plan line — the data used to bridge the requested Rancher version and the
// resolved Kubernetes version. Notes which branch flavor it came from
// (release vs. dev — only shown when known, i.e. head-channel installs),
// flags when the head channel's minor-1 fallback was used, and handles the
// --force-skipped case (upgrade only, where matrix can be nil).
func KDMLine(matrix *kdm.SupportMatrix, flavor kdm.KDMFlavor, usedFallback bool) string {
	if matrix == nil {
		return "unavailable (--force)"
	}
	var notes []string
	if flavor != "" {
		notes = append(notes, string(flavor))
	}
	if usedFallback {
		notes = append(notes, "fallback — no KDM data published yet")
	}
	if len(notes) == 0 {
		return fmt.Sprintf("v%s", matrix.RancherVersion)
	}
	return fmt.Sprintf("v%s (%s)", matrix.RancherVersion, strings.Join(notes, ", "))
}

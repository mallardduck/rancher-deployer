// Package deployment provides deployment workflow utilities.
package deployment

import (
	"fmt"
	"strings"

	"github.com/mallardduck/rancher-deployer/internal/detect"
)

// ResolveMode returns the effective install mode ("k3s" or "k3d").
// If flag is empty, it auto-detects the appropriate mode based on OS and available tools.
// Valid flag values: "", "k3s", "k3d" (case-insensitive).
func ResolveMode(flag string, autoMessage bool) (string, string, error) {
	switch strings.ToLower(flag) {
	case "k3s":
		return "k3s", "", nil
	case "k3d":
		return "k3d", "", nil
	case "":
		mode, reason := detect.InstallMode()
		return mode, reason, nil
	default:
		return "", "", fmt.Errorf("invalid mode %q: must be 'k3s' or 'k3d'", flag)
	}
}
